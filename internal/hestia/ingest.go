package hestia

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/hestia/sheets"
	"github.com/newthinker/atlas/internal/macro/bitemporal"
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
	// OnlyPeriod 非空时只处理 Period 等于它的候选（M1d 的 TASK-006）。**只与 Force 同用**：
	// Force 会翻满 MaxPages 并重新处理每个候选、每个都发一条 Duplicate 的 P2；
	// 运行时切换清单里要的是「限定一期跑一次」来实测整条链路（spec §5 第 6 步）。
	// 过滤发生在 Discover 之后、ingestOne 之前——Discover 不动。
	OnlyPeriod string
	// ProjectSheets 把本期投影到 Google Sheets（M2b）。nil = 不投影：
	// 没配 credentials_file 就是静默降级，与 Notify 同形。
	//
	// 🔴 失败**不阻断、不改 outcome、不进错误链、不发通知**（spec §6.2）。
	// 比契约写失败还弱一档——契约失败仍进 errors.Join，因为 M3 在等它；
	// 投影没有下游，且按 ADR-0004 可再生，下次 ingest 或 sheets push 会补上。
	//
	// 发现漏推靠 `atlas hestia sheets push --all`（默认 dry-run）巡检，
	// 不攒「上次投影到哪」的状态文件——那份状态自己会过期。
	ProjectSheets func(context.Context, []sheets.Row) error
}

// buildSheetRows 是投影组装的注入点（同 sheets.createTabs 的手法）。正常库上 BuildSheetRows
// 构造不出失败——视图按 (period, period_type) 唯一、累计期次期末月互异——C8 的「组装失败」
// 分支只能经这里注入来测。
var buildSheetRows = BuildSheetRows

// notifyError 标记「入库成功但通知没发出去」。循环遇到它**不再发 P1**——
// 那条同样发不出去，只会无限套娃；而错误链里保留底层 err，errors.Is 仍能穿透。
type notifyError struct{ err error }

func (e notifyError) Error() string { return "notify: " + e.err.Error() }
func (e notifyError) Unwrap() error { return e.err }

// contractError 标记「入库成功但契约没写出去」（M2a 的 TASK-005，AD-4b）。与 notifyError
// 同形态，但 Error() 不带前缀：阶段前缀由 ingestOne 的 wrap 给（stage=contract），带了会
// 打成 "contract: contract: …"。runRow 靠它把 outcome 保持为 ingested——数据确实在库，
// HealthSummary 的「最近入库」应当推进；哪里断的由 stage+error 说。循环不认它 ⇒ P1 照发：
// P1 是失败通知，不会造成「Telegram 说入库了、队列里却没有」，反而是运维知道要
// `contract emit` 补发的唯一即时信号。
type contractError struct{ err error }

func (e contractError) Error() string { return e.err.Error() }
func (e contractError) Unwrap() error { return e.err }

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

// runResult 是 ingestOne 交给 Ingest 记 hestia_runs 用的事实（M1.5 的 TASK-002）。
// processed=false 表示候选根本没被处理（HasArticle 跳过），不记行。
type runResult struct {
	processed    bool
	outcome      RunOutcome
	extractor    string
	blockedCheck string
	stage        string // 失败时的阶段名；成功为空
	notified     bool
	notifyErr    string
}

// firstLineOf 取错误的第一行：hestia_runs 的 error / notify_error 列只放一行。
// 需求原文叫 firstLine，本仓库 status_test.go 已有同名（不同签名）的测试 helper，故改名。
func firstLineOf(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return first
}

// isNotifyError 判断错误是不是「通知失败」而非「处理失败」。两处消费它、且必须同判：
// 循环靠它决定发不发 P1，runRow 靠它决定 outcome 是不是 failed。
func isNotifyError(err error) bool {
	var ne notifyError
	return errors.As(err, &ne)
}

// isContractError 判断错误是不是「契约写失败」：runRow 靠它决定 outcome 保持 ingested。
func isContractError(err error) bool {
	var ce contractError
	return errors.As(err, &ce)
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
	runAt := time.Now()
	// 配置错误在任何网络请求之前拦下（M1d 的 TASK-006）。
	if d.OnlyPeriod != "" && !d.Force {
		return errors.New("hestia ingest: OnlyPeriod requires Force (--only-period 只与 --force 同用)")
	}
	// 空 queue.dir 在任何 I/O 之前拦下（M2a 的 TASK-005，AD-5）：EnsureQueueDirs("") 等于
	// MkdirAll("pending") 等四个**相对进程 cwd** 的目录，测试跑起来会在包目录里留垃圾。
	if d.Cfg.Queue.Dir == "" {
		return errors.New("hestia ingest: queue.dir must not be empty")
	}
	// 建齐队列状态机（M2a 的 TASK-005）：消费者第一次来就看到 pending/ processing/ done/ failed/。
	if err := EnsureQueueDirs(d.Cfg.Queue.Dir); err != nil {
		return fmt.Errorf("hestia ingest: %w", err)
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
		return d.recordHeartbeat(ctx, runAt)
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

	// 过滤放在 Discover 之后、循环之前（M1d 的 TASK-006）：Discover 照常翻页与判停，
	// 计数行「kept k of n」让人一眼看到目标期在不在扫描窗口里。
	if d.OnlyPeriod != "" {
		total := len(cands)
		cands = slices.DeleteFunc(cands, func(c Candidate) bool { return c.Period != d.OnlyPeriod })
		fmt.Fprintf(d.Out, "only-period %s: kept %d of %d candidate(s)\n", d.OnlyPeriod, len(cands), total)
		if len(cands) == 0 {
			// 不走「no new reports」：期次写错与正常空跑必须分得开。
			return fmt.Errorf("hestia ingest: no candidate for period %s within max_pages %d",
				d.OnlyPeriod, d.Cfg.Discover.MaxPages)
		}
	}

	var failedPeriods []string
	var errs []error
	processed := 0
	for _, c := range cands {
		started := time.Now()
		res, err := d.ingestOne(ctx, c)
		if err != nil {
			fmt.Fprintf(d.Out, "%s FAILED: %v\n", c.Period, err)
			failedPeriods = append(failedPeriods, c.Period)
			errs = append(errs, err)
			// 通知本身失败时不再发 P1：发不出去的通道上再发一条只是套娃。
			if !isNotifyError(err) {
				// 处理失败 ⇒ P1；P1 的送达结果也进这一行（M1.5 的 TASK-002）。
				if nerr := d.send(renderP1(c, err)); nerr != nil {
					errs = append(errs, fmt.Errorf("hestia ingest %s/%s (%s): %w", c.Period, c.PeriodType, c.ArticleID, nerr))
					res.notifyErr = firstLineOf(nerr.Error())
				} else {
					res.notified = d.Notify != nil
				}
			}
		}
		if !res.processed {
			continue
		}
		processed++
		// 记行在 Save 与 Verdict 行之后：记不进去只影响健康度，不影响已入库的期次。
		if rerr := d.Store.RecordRun(ctx, d.runRow(runAt, started, c, res, err)); rerr != nil {
			fmt.Fprintf(d.Out, "%s run record FAILED: %v\n", c.Period, rerr)
			errs = append(errs, fmt.Errorf("hestia ingest %s/%s (%s): record run: %w", c.Period, c.PeriodType, c.ArticleID, rerr))
		}
	}
	// 心跳按 processed 计而不是按「记成功的行数」计（reviewer S2）：候选处理过而 RecordRun
	// 失败时，再补一行 no_new 会把「处理过一期」标成空跑；spec §2.1 的心跳定义是「无候选被处理」。
	if processed == 0 {
		// 全部候选都被跳过：本轮零行，仍要心跳。
		if err := d.recordHeartbeat(ctx, runAt); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		// 分子数的是**期**（failedPeriods），不是 errs 条数（QA A4）：P1 自身发送失败会往
		// errs 里再追加一条，按 errs 计会打出「2/1 期失败」。
		return fmt.Errorf("hestia ingest: %d/%d 期失败 (%s): %w",
			len(failedPeriods), len(cands), strings.Join(failedPeriods, ", "), errors.Join(errs...))
	}
	return nil
}

// runRow 把一次候选处理折成 hestia_runs 的一行（M1.5 的 TASK-002）。
// err 非 nil 且不是通知失败 ⇒ failed。
func (d IngestDeps) runRow(runAt, started time.Time, c Candidate, res runResult, err error) Run {
	r := Run{
		RunAt: runAt, FinishedAt: time.Now(), Duration: time.Since(started),
		Period: c.Period, PeriodType: c.PeriodType, ArticleID: c.ArticleID,
		Outcome: res.outcome, Extractor: res.extractor, BlockedCheck: res.blockedCheck, Stage: res.stage,
		Notified: res.notified, NotifyError: res.notifyErr,
	}
	switch {
	case err == nil, isNotifyError(err):
		// 通知失败不算处理失败：notify_error 列已由 res 带过来。
	case isContractError(err):
		// 契约写失败（M2a 的 TASK-005，AD-4b）：数据在库，outcome 保持 ingested；
		// Stage 已由 fail("contract") 填好，Error 列记首行让 status 能看到哪里断的。
		r.Error = firstLineOf(err.Error())
	default:
		r.Outcome = RunFailed
		r.Error = firstLineOf(err.Error())
	}
	return r
}

// recordHeartbeat 记一行 no_new：本轮没有处理任何候选，但管线跑了。
func (d IngestDeps) recordHeartbeat(ctx context.Context, runAt time.Time) error {
	beat := Run{RunAt: runAt, FinishedAt: time.Now(), Duration: time.Since(runAt), Outcome: RunNoNew}
	if err := d.Store.RecordRun(ctx, beat); err != nil {
		fmt.Fprintf(d.Out, "run record FAILED: %v\n", err)
		return fmt.Errorf("hestia ingest: record heartbeat: %w", err)
	}
	return nil
}

// ingestOne 处理一条候选。返回的错误一律带「期次 (article_id)」前缀——汇总之后
// 调用方看到的是一串错误，不指名是哪一期的话没法排障。
//
// 同时返回 runResult 供 Ingest 记 hestia_runs（M1.5 的 TASK-002）：stage 取的是
// 传给 fail 的原字符串（fetch 阶段带 URL，AD-14），不改既有错误串。
func (d IngestDeps) ingestOne(ctx context.Context, c Candidate) (runResult, error) {
	// 出错时统一加定位上下文。期次与 article_id 都给：期次是人认的，article_id
	// 是能直接拼回 URL 的那个。
	wrap := func(stage string, err error) error {
		return fmt.Errorf("hestia ingest %s/%s (%s): %s: %w",
			c.Period, c.PeriodType, c.ArticleID, stage, err)
	}
	res := runResult{processed: true}
	fail := func(stage string, err error) (runResult, error) {
		res.stage = stage
		return res, wrap(stage, err)
	}

	if !d.Force {
		seen, err := d.Store.HasArticle(ctx, c.ArticleID)
		if err != nil {
			return fail("has article", err)
		}
		if seen {
			fmt.Fprintf(d.Out, "%s already ingested (%s)\n", c.Period, c.ArticleID)
			return runResult{processed: false}, nil
		}
	}

	raw, err := d.Fetch.Get(ctx, c.URL)
	if err != nil {
		return fail("fetch "+c.URL, err)
	}
	// 快照在 Parse **之前**落盘（M1d 的 TASK-003）：解析失败的那篇恰恰最需要回溯，
	// 而此前 raw 只在内存里——方案报告 M1c 的 DoD「原始 HTML 快照已留存」在增量路径上
	// 一直是空的。写盘失败让该期失败：快照是 DoD 项，不是可选副作用。
	snap, err := saveSnapshot(d.Cfg.Storage.SnapshotDir, c.ArticleID, raw, time.Now())
	if err != nil {
		return fail("snapshot", err)
	}
	if snap.Kind == snapshotDiverged {
		// 央行改稿。不是错误，但必须说出来——两版都留了，运维要知道去看哪一份。
		fmt.Fprintf(d.Out, "%s snapshot diverged from %s: saved as %s\n", c.Period, c.ArticleID, snap.Path)
	}
	obs, err := Parse(raw)
	if err != nil {
		return fail("parse", err)
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
		res.stage = "mismatch"
		return res, fmt.Errorf("hestia ingest %s/%s (%s): 期次不一致：标题说 %s/%s，正文说 %s/%s（%s）",
			c.Period, c.PeriodType, c.ArticleID,
			c.Period, c.PeriodType, obs.Meta.Period, obs.Meta.PeriodType, c.URL)
	}

	rep, err := Validate(ctx, obs, d.Store, d.Cfg.Thresholds)
	if err != nil {
		return fail("validate", err)
	}
	out, err := d.Store.Save(ctx, obs, rep)
	if err != nil {
		return fail("save", err)
	}
	res.extractor = obs.Meta.Extractor
	switch {
	case out.Table == TablePending:
		res.outcome = RunPending
		for _, chk := range rep.Checks {
			if chk.Status == CheckFailed {
				res.blockedCheck = chk.ID
				break
			}
		}
	case out.Verdict == bitemporal.Duplicate:
		res.outcome = RunDuplicate
	default:
		res.outcome = RunIngested
	}

	// Verdict 与 Table 都打出来。Table 是当下就必须区分的（入权威表 vs 落 pending
	// 对运维的含义相反）。
	//
	// Verdict 也打，理由是**不要为当前的局限写断言** —— 让 Duplicate/Revision 自己
	// 显形，而不是悄悄混在「已入库」里。⚠️ 这条理由的实证是
	// `TestForceOnObservedPeriodIsDuplicate`（`ingest_test.go`）：它现在是**绿**的，
	// 而它**从未因为「那条局限被放开」而红过一次** —— 因为这里从一开始就没有假定它。
	fmt.Fprintf(d.Out, "%s %s → %s\n", obs.Meta.Period, out.Verdict, out.Table)

	// 契约在通知之前（M2a 的 TASK-005）：契约是 M3 的输入，通知是投影。写失败该期
	// stage=contract、P2 不发——避免「Telegram 说入库了、队列里却没有」；数据已在库，
	// 运维用 `hestia contract emit --period` 补发（错误类型 contractError，见其注释）。
	//
	// 只在 New / Revision 时写（AD-14）：OutOfOrder 同样入权威表但不是 current 行，
	// 若也写契约会用旧数据同名覆盖 pending/ 里可能尚未消费的更新契约，违背「最新的赢」；
	// 它与 Duplicate 同待遇——P2 照发、温度 0/0。
	var temp Temperature
	if out.Table == TableObservations && (out.Verdict == bitemporal.New || out.Verdict == bitemporal.Revision) {
		// extracted_at 要从库里回读（QA C1，M2a 的 TASK-005 返工 2）：Save 按值收 obs，只给
		// 自己那份副本填 IngestedAt 且 Outcome 不回传，这里手上的 obs 仍是入库前的、
		// IngestedAt 为空——直接建契约会写出 `"extracted_at": ""`，与回放路径（Current 读库）
		// 形状不一致。不动 Save 的签名与函数体（冻结），Save 之后经 Current 取回当前行即可；
		// 取不到（查询出错，或刚 Save 却不是 current 行）与写失败同待遇：contractError，
		// 数据已在库、P1 照发。
		cur, ok, err := d.Store.Current(ctx, obs.Meta.Period, obs.Meta.PeriodType)
		if err != nil || !ok {
			return fail("contract", contractError{err: cmp.Or(err, errors.New("just saved but not current"))})
		}
		obs.Meta.IngestedAt = cur.Meta.IngestedAt
		in := ContractInput{Obs: obs, Report: rep, SourceURL: c.URL, IsRevision: out.Verdict == bitemporal.Revision}
		if in.IsRevision {
			prior, err := d.Store.PriorPublishedAt(ctx, obs.Meta.Period, obs.Meta.PeriodType, obs.Meta.PublishedAt)
			if err != nil {
				return fail("contract", contractError{err: err})
			}
			in.Supersedes = prior
		}
		// 侧车在契约之前（M3 的 TASK-001）：消费者是看到契约才去读侧车的，反过来会有
		// 一个窗口——契约已在 pending/、侧车还没落盘，消费者读到半份输入。失败与契约
		// 写失败同待遇（contractError）：数据已在库，P1 照发、P2 不发，运维用
		// `hestia contract emit --period` 连侧车一起补发。
		hist, err := BuildHistory(ctx, d.Store, obs, contractGenerator)
		if err != nil {
			return fail("contract", contractError{err: err})
		}
		if _, err := WriteHistory(d.Cfg.Queue.Dir, hist); err != nil {
			return fail("contract", contractError{err: err})
		}
		path, err := WriteContract(d.Cfg.Queue.Dir, BuildContract(in, d.Cfg))
		if err != nil {
			return fail("contract", contractError{err: err})
		}
		fmt.Fprintf(d.Out, "%s contract → %s\n", obs.Meta.Period, path)
		// 投影在契约之后（M2b 的 TASK-010）：契约有下游在等，投影没有。
		// ⚠️ 两个分支都**只打印不返回**：写成 return err 或并进 errors.Join 都违反 C8。
		if d.ProjectSheets != nil {
			rows, berr := buildSheetRows(ctx, d.Store)
			if berr != nil {
				fmt.Fprintf(d.Out, "sheets: 组装失败（不影响入库）: %v\n", berr)
			} else if perr := d.ProjectSheets(ctx, rows); perr != nil {
				fmt.Fprintf(d.Out, "sheets: 投影失败（不影响入库）: %v\n", perr)
			}
		}
		temp = Evaluate(obs, d.Cfg.Signals)
	}

	// 通知放在打印之后：Out 是本地真相，Telegram 是它的投影；投影失败不该让本地少一行。
	// 阶段名写成 "send P2"/"send P0" 而不是 "notify"：notifyError.Error() 自带 "notify: "
	// 前缀，再用 notify 做阶段名会打成 "notify: notify: …"；阶段名说清没发出去的是哪一类
	// 消息，比重复一遍 notify 有用。
	msg, stage := renderP2(obs, out, temp), "send P2"
	if out.Table == TablePending {
		msg, stage = renderP0(obs, rep), "send P0"
	}
	if err := d.send(msg); err != nil {
		// 数据已在库：outcome 保持 ingested / pending，只记 notify_error（不走 fail，stage 留空）。
		res.notifyErr = firstLineOf(err.Error())
		return res, wrap(stage, err)
	}
	res.notified = d.Notify != nil
	return res, nil
}
