package hestia

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// IngestDeps 是一次抓取所需的全部外部依赖。
//
// 编排层放在 internal/hestia 而不是 cmd/atlas：它是业务编排（发现→抓→解析→
// 校验→入库），和 Discover / Parse / Validate / Save 同层；而且测试要用同包的
// testdata 快照，放 cmd 就得写 ../../internal/hestia/testdata 或复制一份。
type IngestDeps struct {
	Store *Store
	Fetch Fetcher
	Cfg   Config
	Out   io.Writer // nil 等价于 io.Discard
	// Notify 是通知通道（M1d 的 TASK-005）。nil = 不发：没配置就是静默降级，
	// 不报错、不阻塞入库（方案报告 4.8.1）。
	//
	// 发送时机：Save 落 pending ⇒ P0；入权威表 ⇒ P2（任何 Verdict，Duplicate 不吞）；
	// ingestOne 任一阶段失败 ⇒ 循环里发 P1。空跑不发。
	//
	// 发送失败**不回滚入库**（数据已经 Save 了），但并进 errors.Join 让退出码非零、
	// err.log 留痕。同一篇不会二次处理，所以不会每次唤起都重复报错。
	Notify Sender
	// Force 绕过**两层**幂等（Discover 的判停 + ingestOne 的 article_id）。
	//
	// ⚠️ **它穿不透第三层**：对已在权威表的期次，同一篇的 `published_at` 不变
	// ⇒ `Save` 恒判 `Duplicate` ⇒ `refreshArticleID` **只刷 article_id，新抽出来的
	// Values 一个都不写**，且返回 nil、退出码 0。⇒ 「改了阈值后重跑」对**已入库**
	// 的期次在数据层面是 **no-op**；它真正能救回来的是**落在 pending 里**的那些。
	// 该取舍登记在 `refreshArticleID` 自己的注释里（搜 `只更新 article_id 意味着`）。
	Force bool
}

// notifyError 标记「入库成功但通知没发出去」。循环遇到它**不再发 P1**——
// 那条同样发不出去，只会无限套娃；而错误链里保留底层 err，errors.Is 仍能穿透。
type notifyError struct{ err error }

func (e notifyError) Error() string { return "notify: " + e.err.Error() }
func (e notifyError) Unwrap() error { return e.err }

// send 是唯一的发送点。Notify 为 nil 时是 no-op。
func (d IngestDeps) send(text string) error {
	if d.Notify == nil {
		return nil
	}
	if err := d.Notify.SendText(text); err != nil {
		return notifyError{err: err}
	}
	return nil
}

// neverSeen 是 Force 用的 ArticleChecker：什么都没见过，于是 Discover 不会提前返回。
//
// 不用 nil 表示「不检查」：nil 接口会让 Discover 在调用处 panic，而「Force 时跳过
// 判停」是一个**行为**，应当由一个说得出名字的实现承载，不是由一个特例分支承载。
type neverSeen struct{}

func (neverSeen) HasArticleInObservations(context.Context, string) (bool, error) {
	return false, nil
}

// Ingest 跑一轮发现与入库。
//
// 单期失败**不中断整批** —— 一期解析失败不该阻止其它期入库。逐期收集，最后
// 汇总返回非零：launchd 的 err.log 会留痕，launchctl list 也能看到。一期反复
// 失败会让每次唤起都非零，那本来就该被注意到。
//
// 汇总错误用 errors.Join 把各期的错误**包进去**，而不是只拼一句人话：调用方
// （以及测试）要能 errors.Is / errors.As 出底层判因，只留字符串等于把判因扔了。
func Ingest(ctx context.Context, d IngestDeps) error {
	if d.Out == nil {
		d.Out = io.Discard
	}

	// Force 必须同时穿透**两层**幂等（TASK-011 修回归）。
	//
	// TASK-011 之前 Discover 按**期次**判停、ingestOne 按 **article_id** 跳过，两层
	// 判据不同，Force 只需绕过后者。判停换成 article_id 之后两层同判据了：Discover
	// 会在那篇上直接停 ⇒ 候选清单里根本没有它 ⇒ **Force 无从生效**（实测：
	// TestIngestSkipsSeenArticleUnlessForce/Force 那条断言「应当真的被重新处理并
	// 入库」失败）。所以 Force 时喂一个「什么都没见过」的 checker。
	//
	// 代价是 Force 会翻满 MaxPages —— 那正是 Force 的语义（重来一遍），不是意外。
	var known ArticleChecker = d.Store
	if d.Force {
		known = neverSeen{}
	}
	// 🔴 **cwd 守卫**：先说清这一轮写的是哪个库（TASK-009 WARNING-4）。
	//
	// `db_path` 是相对路径、按进程 cwd 解析（约束 C8），而 `NewStore` 先 `MkdirAll`
	// 再建库 ⇒ **在错误的 cwd 下不报错，会新建一个空库**。之后 ingest 会翻满 MaxPages、
	// 全量入库、逐期打印正常、退出码 0，**而真库停在旧数据、没有任何提示**。
	// `status` 一直有这道守卫（`RenderStatus` 打印解析后的绝对路径），`ingest` 此前没有
	// —— 而 ingest 才是 launchd 每天三次唤起的那个。
	//
	// ⚠️ **打印放在这一层而不是 cmd 层**：`cmd/atlas/hestia.go` 有一条守卫明令它不得
	// import `path/filepath`（搜 `不该 import path/filepath`）——「db_path 的解析归
	// internal/hestia」是 TASK-008 定的分层。把解析塞回 cmd 层会打红那条守卫，
	// 而那条守卫是对的：路径语义只该有一个归属。
	// `filepath.Abs` 只在 `os.Getwd()` 失败时出错，而那种环境下「相对 db_path」这个
	// 设计本身已无从谈起 ⇒ 不为它单开一条分支（那条分支测不到，只会变成一块永不执行
	// 的未覆盖代码）。出错时 abs 为空串，退回打印原样路径，信息量不减。
	abs, _ := filepath.Abs(d.Cfg.Storage.DBPath)
	if abs == "" {
		abs = d.Cfg.Storage.DBPath
	}
	fmt.Fprintf(d.Out, "db: %s\n", abs)

	cands, stop, err := Discover(ctx, d.Fetch, known, d.Cfg.Discover)
	if err != nil {
		// 此刻还没有任何期次，能给的定位上下文只有 index URL。
		return fmt.Errorf("hestia ingest: discover %s: %w", d.Cfg.Discover.IndexURL, err)
	}
	// 🔴 **停止原因在有候选时也必须说**（TASK-011 WARNING-1）。
	//
	// 原先它只在 `len(cands) == 0` 时打印，而 **`max_pages` 且有候选恰恰是它的主要形态**：
	// 空库首跑必然如此。⇒ 那一轮之后 `MaxPages` 以外的历史**永久不可达**，
	// 而这条信息**在唯一会发生它的那一轮被静默吞掉**，退出码 0。
	//
	// `max_pages` 走 **stderr**：它是「可能还有没发现的期次」这个警告，不是正常输出。
	// ⚠️ **不改退出码** —— 首跑必然 `max_pages`，改退出码会产出**假红**，而假红会被
	// 训练成忽略；那比不报还糟。
	if stop == StopMaxPages {
		fmt.Fprintf(os.Stderr,
			"WARNING: discover stopped at max_pages (%d) with %d candidate(s): "+
				"periods older than the window are not reachable this run\n",
			d.Cfg.Discover.MaxPages, len(cands))
	}
	if len(cands) == 0 {
		// 「命中已见过的文章、正常停」与「翻满上限仍一无所获」在这一行上完全同形，
		// 靠 stop 才分得开。
		fmt.Fprintf(d.Out, "no new reports (stopped: %s)\n", stop)
		return nil
	}
	fmt.Fprintf(d.Out, "discover stopped: %s (%d candidate(s))\n", stop, len(cands))

	// 按期次**升序**处理。Discover 给的是「最近的排在前面」，顺着跑会让
	// stock_continuity / deposit_sum 的漂移检测一次都不真正执行（每一期都成了
	// 「首期」⇒ 恒 no_prior_period），而数据照进权威表、报告照样 Passed=true、
	// **零告警**——skipped 不拉低 Passed（validate.go:101）。
	//
	// 用排序而不是把切片倒过来：倒序依赖「Discover 一定按时间倒序返回」这个
	// 外部约定，而排序直接断言我们要的性质本身。period 是 YYYY-MM 定宽格式，
	// 字典序即时间序。稳定排序 ⇒ 同期次的不同 period_type（12 月月报与年报的
	// period 都是 YYYY-12）保持 Discover 给的相对顺序，它们本就是独立序列。
	slices.SortStableFunc(cands, func(a, b Candidate) int {
		return strings.Compare(a.Period, b.Period)
	})

	var failedPeriods []string
	var errs []error
	for _, c := range cands {
		err := d.ingestOne(ctx, c)
		if err == nil {
			continue
		}
		fmt.Fprintf(d.Out, "%s FAILED: %v\n", c.Period, err)
		failedPeriods = append(failedPeriods, c.Period)
		errs = append(errs, err)
		// 通知本身失败时不再发 P1：发不出去的通道上再发一条只是套娃。
		var ne notifyError
		if errors.As(err, &ne) {
			continue
		}
		if nerr := d.send(renderP1(c, err)); nerr != nil {
			errs = append(errs, fmt.Errorf("hestia ingest %s/%s (%s): %w", c.Period, c.PeriodType, c.ArticleID, nerr))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("hestia ingest: %d/%d 期失败 (%s): %w",
			len(errs), len(cands), strings.Join(failedPeriods, ", "), errors.Join(errs...))
	}
	return nil
}

// ingestOne 处理一条候选。返回的错误一律带「期次 (article_id)」前缀——汇总之后
// 调用方看到的是一串错误，不指名是哪一期的话没法排障。
func (d IngestDeps) ingestOne(ctx context.Context, c Candidate) error {
	// 出错时统一加定位上下文。期次与 article_id 都给：期次是人认的，article_id
	// 是能直接拼回 URL 的那个。
	wrap := func(stage string, err error) error {
		return fmt.Errorf("hestia ingest %s/%s (%s): %s: %w",
			c.Period, c.PeriodType, c.ArticleID, stage, err)
	}

	if !d.Force {
		seen, err := d.Store.HasArticle(ctx, c.ArticleID)
		if err != nil {
			return wrap("has article", err)
		}
		if seen {
			fmt.Fprintf(d.Out, "%s already ingested (%s)\n", c.Period, c.ArticleID)
			return nil
		}
	}

	raw, err := d.Fetch.Get(ctx, c.URL)
	if err != nil {
		return wrap("fetch "+c.URL, err)
	}
	// 快照在 Parse **之前**落盘（M1d 的 TASK-003）：解析失败的那篇恰恰最需要回溯，
	// 而此前 raw 只在内存里——方案报告 M1c 的 DoD「原始 HTML 快照已留存」在增量路径上
	// 一直是空的。写盘失败让该期失败：快照是 DoD 项，不是可选副作用。
	snap, err := saveSnapshot(d.Cfg.Storage.SnapshotDir, c.ArticleID, raw, time.Now())
	if err != nil {
		return wrap("snapshot", err)
	}
	if snap.Kind == snapshotDiverged {
		// 央行改稿。不是错误，但必须说出来——两版都留了，运维要知道去看哪一份。
		fmt.Fprintf(d.Out, "%s snapshot diverged from %s: saved as %s\n", c.Period, c.ArticleID, snap.Path)
	}
	obs, err := Parse(raw)
	if err != nil {
		return wrap("parse", err)
	}

	// 接缝 ①：Parse 只拿到 raw bytes，看不到 URL —— 让它编造一个 ArticleID 才是
	// 缺陷（parse.go 的原话）。这是两个包之间唯一的手工装配点。
	obs.Meta.ArticleID = c.ArticleID

	// 接缝 ②：标题与正文各解析出一次期次，本该一致。
	//
	// 不一致意味着央行把链接挂错了，或某一侧的解析有 bug —— 两种都该拦下而不是
	// 入库。这是两次独立解析白捡的校验，与 Parse 已有的「PubDate 与正文交叉校验」
	// 同一思路：同一事实的两个独立来源，对不上就是信号。
	//
	// 不拦的代价不只是「一期数据错了」：按正文键入库、按候选键问判停，Discover
	// 下次仍认为那期没入库 ⇒ **静默的永久循环**（Sprint 036 W6 实测）。
	if obs.Meta.Period != c.Period || obs.Meta.PeriodType != c.PeriodType {
		return fmt.Errorf("hestia ingest %s/%s (%s): 期次不一致：标题说 %s/%s，正文说 %s/%s（%s）",
			c.Period, c.PeriodType, c.ArticleID,
			c.Period, c.PeriodType, obs.Meta.Period, obs.Meta.PeriodType, c.URL)
	}

	rep, err := Validate(ctx, obs, d.Store, d.Cfg.Thresholds)
	if err != nil {
		return wrap("validate", err)
	}
	out, err := d.Store.Save(ctx, obs, rep)
	if err != nil {
		return wrap("save", err)
	}

	// Verdict 与 Table 都打出来。Table 是当下就必须区分的（入权威表 vs 落 pending
	// 对运维的含义相反）。
	//
	// Verdict 也打，理由是**不要为当前的局限写断言** —— 让 Duplicate/Revision 自己
	// 显形，而不是悄悄混在「已入库」里。⚠️ 这条理由的实证是
	// `TestForceOnObservedPeriodIsDuplicate`（`ingest_test.go`）：它现在是**绿**的，
	// 而它**从未因为「那条局限被放开」而红过一次** —— 因为这里从一开始就没有假定它。
	fmt.Fprintf(d.Out, "%s %s → %s\n", obs.Meta.Period, out.Verdict, out.Table)

	// 通知放在打印之后：Out 是本地真相，Telegram 是它的投影；投影失败不该让本地少一行。
	// 阶段名写成 "send P2"/"send P0" 而不是 "notify"：notifyError.Error() 自带 "notify: "
	// 前缀，再用 notify 做阶段名会打成 "notify: notify: …"；阶段名说清没发出去的是哪一类
	// 消息，比重复一遍 notify 有用。
	msg, stage := renderP2(obs, out), "send P2"
	if out.Table == TablePending {
		msg, stage = renderP0(obs, rep), "send P0"
	}
	if err := d.send(msg); err != nil {
		return wrap(stage, err)
	}
	return nil
}
