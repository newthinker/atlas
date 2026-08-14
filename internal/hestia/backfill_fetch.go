package hestia

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 回填抓取层（M1c-1 的 TASK-006，design-spec §7）。
//
// 本文件是整条回填链路的编排者：index 扫描（TASK-002）→ 站内搜索（TASK-004）→
// 交叉校验（TASK-005）→ 逐篇下载并落进 manifest（TASK-003）。上游各层都是纯逻辑或
// 单页取回，**限速、落盘、断点续抓、两类错误的不同处置全在这里**。

// backfillInterval 是两次请求之间的最小间隔。
//
// 央行站点没有公开的速率限制，1 req/s 是**自我约束**：回填是一次性动作，
// 约 480 次请求跑 9 分钟完全可以接受，没有理由更快。
const backfillInterval = time.Second

// backfillLimiter 给任意 Fetcher 套上「每次请求前先等一秒」。
//
// 做成 Fetcher 装饰器而不是在每个调用点插 sleep，是为了让
// **「sleep 次数 == 请求次数 − 1」由构造保证**：index 翻页与搜索翻页的循环都在上游
// 各自的包里，逐个调用点去插 sleep 既漏得掉、也没法在测试里数清楚。
//
// sleep 可注入（`func(time.Duration)`），测试注入记录器；**不引入 fake clock** ——
// 被测的是「调了几次、每次多久」，不是时间本身。
type backfillLimiter struct {
	inner Fetcher
	sleep func(time.Duration)
	n     int // 已发出的请求数
}

func (l *backfillLimiter) Get(ctx context.Context, url string) ([]byte, error) {
	if l.n > 0 { // 第一次不等：等在请求**之间**，不是请求之前
		l.sleep(backfillInterval)
	}
	l.n++
	return l.inner.Get(ctx, url)
}

// BackfillConfig 是一次回填的参数。
//
// Out 是产物根目录（design-spec §8：manifest.json + index/ + search/ + articles/），
// 必须是仓库外的绝对路径——那三个理由是 worktree 拆掉产物仍在、不可能误提交、
// 会话外备份直接 cp -r。
type BackfillConfig struct {
	IndexURL string
	From     time.Time // 语义是**发布日期**不是期次（设计附录 §A1）
	To       time.Time // 搜索侧的区间上界；零值时用 time.Now()
	Out      string
	MaxPages int           // <= 0 时用 backfillMaxPages
	Timeout  time.Duration // 仅 BackfillFetch 用：单次 HTTP 的超时

	// —— 完整性对账（M1c-1 的 TASK-009 接入）——

	// Report 是对账报告的去向。**nil ⇒ os.Stdout**。
	//
	// 默认可见而不是默认丢弃：这个机制的全部意义就是不让缺口静默消失，
	// 一个忘了设 Report 的调用方若因此什么都看不到，等于把它又关掉了一次。
	Report io.Writer

	// Cutover 是模板切换点（期次 YYYY-MM）。空 ⇒ 用 backfillCutover。
	//
	// 它是**参数**不是常量：真跑时要能核对、能随实测调整（caller_contract 第 4 条
	// 「别在调用方那侧再写死一份」——所以默认值放在本包的 backfillCutover 一处，
	// 命令层不传即用它，而不是命令层自己写一个 "2025-09"）。
	Cutover string

	// ExpectPeriods / ExpectArticles 是总数期望值，对应 CLI 的 --expect-periods /
	// --expect-articles。**零值 = 未显式传入 ⇒ 走推算值告警路径**（TASK-008 的
	// notes_for_downstream 已定死这个语义）。
	//
	// 摊平成两个 int 而不是内嵌 backfillExpect：后者是**非导出**类型，
	// 内嵌进导出结构体会让 cmd/atlas 根本没法给它赋值。
	ExpectPeriods  int
	ExpectArticles int
}

// backfillCutover 是模板切换点的**唯一**定义处（实测 2025-09）。
//
// 实测依据：社融存量/增量的最后一期是「2025年8月…」，同日发布于 2025-09-12；
// 自 2025-09 起每期恰好一篇。⚠️ 它是**对照值**——真跑时核对它，别把它当判据。
const backfillCutover = "2025-09"

// 对账报告里的段落标签。**提成常量是为了让测试钉住具体文案而不是钉住「非空」**：
// 「输出非空」对「只打印了 Rows」同样为真，而本 sprint 已在弱断言上翻车多次。
//
// ⚠️ **任何两个标签都不许互为子串**。第一版把 Complete 写成「缺篇期次: 无（…）」，
// 它**字面包含** Missing 的「缺篇期次:」⇒ `NotContains(报告, Missing)` 在「全部齐全」
// 的报告上**永不成立**，而那条断言看起来完全合理。是我自己写的用例当场撞上的。
// ⇒ 共享前缀的标签会让所有子串断言变得不可靠，而失败方式是「断言恒假」不是「漏判」。
const (
	backfillReconcileHeader            = "=== 完整性对账 ==="
	backfillReconcileMissingLabel      = "缺篇期次:"
	backfillReconcileCompleteLabel     = "各期均满足结构规则（无缺篇）"
	backfillReconcileEmptyLabel        = "零期次: 对账表为空"
	backfillReconcileUnclassifiedLabel = "无法归类的标题:"
	backfillReconcileWarningLabel      = "告警:"
	backfillReconcileViolationLabel    = "违规（显式期望值不符）:"
)

// BackfillFetch 跑一次完整回填。**本包对外的唯一入口**（cmd/atlas 接 cobra 用）。
//
// 返回非 nil 不等于「什么都没抓到」：单篇失败会记进 manifest 的 failed[] 并继续，
// 跑完才返回非零 —— 产物仍然可用，调用方据此决定要不要重跑补齐。
func BackfillFetch(ctx context.Context, cfg BackfillConfig) error {
	return runBackfill(ctx, NewPBOCFetcher(cfg.Timeout), time.Sleep, cfg)
}

// runBackfill 是 BackfillFetch 的可注入版本：Fetcher 与 sleep 都由调用方给。
//
// # 两类错误的处置刻意相反（design-spec §7.2）
//
//   - **单篇文章抓取失败**（网络抖动、单篇 404）⇒ 记进 failed[]、**继续抓后面的**、
//     跑完返回非零。这是外部世界的事，一篇抓不到不该让另外两百篇也白跑。
//   - **落盘失败**（磁盘满、权限）⇒ **立刻中止**。这是本机的事——继续抓只会浪费请求，
//     而且后面每一篇都会失败。
//
// index 侧抓取/解析失败同样是硬失败（少一页 index = 静默少十几条候选）；
// 搜索侧失败则 fail-open（ADR-M1c1-02），由 crossCheckBackfill 退化处理。
func runBackfill(ctx context.Context, f Fetcher, sleep func(time.Duration), cfg BackfillConfig) error {
	store, err := loadManifest(cfg.Out)
	if err != nil {
		return err // 含「manifest 坏了」——绝不当成首跑，那会退化成全量重抓
	}
	to := cfg.To
	if to.IsZero() {
		to = time.Now()
	}
	lim := &backfillLimiter{inner: f, sleep: sleep}
	// 只解一次：落进 manifest 的那个值与 reconcileBackfill 实际用的那个值**必须是同一个**。
	// 分别写两遍 cmp.Or 时，改一处漏一处就会让 manifest 记下一个「这一趟没用过」的 cutover
	// ——而 manifest 里这个字段存在的全部理由就是「产出它时实际用的参数」（QA R2-8）。
	cutover := cmp.Or(cfg.Cutover, backfillCutover)

	scan, err := scanBackfillIndex(ctx, lim, backfillIndexCfg{
		IndexURL: cfg.IndexURL, From: cfg.From, MaxPages: cfg.MaxPages,
	})
	if err != nil {
		return fmt.Errorf("index 侧扫描: %w", err)
	}
	if err := saveBackfillIndexPages(cfg.Out, scan.Pages); err != nil {
		return err
	}

	hits, searchPages, searchErr := fetchBackfillSearchAll(ctx, lim, cfg.Out, cfg.From, to)
	if searchErr != nil && errors.Is(searchErr, errBackfillDisk) {
		return searchErr // 落盘失败不 fail-open：那是本机故障，不是检索服务的事
	}

	cc := crossCheckBackfill(scan.Reports, hits, searchErr)

	store.Manifest.From = cfg.From.Format("2006-01")
	store.Manifest.ScannedAt = time.Now().UTC().Format(time.RFC3339)
	store.Manifest.PagesScanned = len(scan.Pages)
	store.Manifest.SearchPagesScanned = searchPages
	store.Manifest.OnlyInIndex = cc.OnlyInIndex
	store.Manifest.OnlyInSearch = cc.OnlyInSearch
	store.Manifest.SearchSkippedReason = cc.SearchSkippedReason
	// 产出这份 manifest 时**实际用的参数**（QA R2-8）：下游被要求「只读这份文件」，
	// 而重算对账离不开这几样。cutover 尤其要紧——猜错一期就会凭空报出两条假缺篇。
	store.Manifest.Cutover = cutover
	store.Manifest.To = to.Format("2006-01-02")
	store.Manifest.Keywords = backfillSearchKeywords
	store.Manifest.KeepPrefix = backfillSearchKeepPrefix
	if err := store.Save(); err != nil {
		return err
	}

	var failed int
	for _, c := range cc.Fetch {
		// 「已抓」= manifest 有记录 **且** 文件在盘上。断点续抓的整个价值就在这一句：
		// 命中就**根本不发请求**，而不是发了再丢掉。
		if store.HasArticle(c.ArticleID) {
			continue
		}
		body, err := lim.Get(ctx, c.URL)
		if err != nil {
			failed++
			if err := store.AppendFailed(Failed{ID: c.ArticleID, URL: c.URL, Error: err.Error()}); err != nil {
				return err // 记不下失败本身就是落盘失败 ⇒ 中止
			}
			continue
		}
		rel := articleFile(c.ArticleID)
		if err := writeBackfillFile(filepath.Join(cfg.Out, rel), body); err != nil {
			return err
		}
		if err := store.AppendArticle(Article{
			ID:        c.ArticleID,
			Title:     c.Title,
			Published: c.Published,
			URL:       c.URL,
			File:      rel,
			SHA256:    articleSHA256(body),
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
			Source:    c.Source,
		}); err != nil {
			return err
		}
	}

	// —— 完整性对账（M1c-1 的 TASK-009）——
	//
	// 🔴 **位置在 `failed > 0` 判断之前**，不是之后：先 return 会让「抓取有失败」把
	// 对账违规**整个吞掉**，而那趟跑已经花了约 9 分钟，人拿到的错误信息里少一半。
	//
	// 入参取 **manifest 里已抓到的篇目**而不是 cc.Fetch：对账要回答的是
	// 「**手上这份产物完不完整**」，抓取失败造成的缺口同样要算进来。
	//
	// 🔴 为什么这一步非有不可：`reconcileBackfill` 有 12 个测试、12 个变异全 KILLED，
	// 而在此之前**生产路径一次都不碰它**。「整页被静默跳过」这个失效（翻页层对下架
	// 方向完全沉默，见 backfill_scan.go 的连续性守卫注释）在本 sprint 里**只有对账层
	// 能发现** —— 它不被调用，等于那条链根本没接上。
	rc := reconcileBackfill(
		backfillReconcileItemsFromArticles(store.Manifest.Articles),
		cutover,
		backfillExpect{Periods: cfg.ExpectPeriods, Articles: cfg.ExpectArticles},
	)
	writeBackfillReconcileReport(backfillReportWriter(cfg.Report), rc)

	// —— 完成标记 + 对账摘要落盘（M1c-1 的 TASK-010 / QA R2-7、R2-8）——
	//
	// 🔴 `CompletedAt` 只在这里写，且**必须在这一句之前的所有步骤都跑完**：它要回答的
	// 正是「这一趟结束了没有」。进程在第 218 篇被杀时，manifest 依然是合法 JSON、
	// sha256 全对、与磁盘完全闭合——**下游做的一切闭合性检查在夭折的产物上同样全绿**，
	// 唯一的差别就是这个字段在不在。
	//
	// ⚠️ 它写在 `failed > 0` / `rc.Failed()` 判断**之前**是刻意的：那两者是「跑完了但有问题」，
	// 不是「没跑完」。把完成标记和「无错」绑在一起，会让**有失败但确实跑到底**的那趟
	// 与**中途被杀**的那趟再次不可分辨——正是本字段要消掉的那种不可分辨。
	store.Manifest.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	store.Manifest.Reconcile = &ManifestReconcile{
		Periods:        rc.Periods,
		Articles:       rc.Articles,
		MissingPeriods: rc.MissingPeriods,
		Unclassified:   rc.Unclassified,
	}
	if err := store.Save(); err != nil {
		return err
	}

	// 两类问题**互不吞没**：抓取失败与对账违规同时发生时，errors.Join 让两条都在
	// Error() 文本里。缺篇本身**不**进这里 —— 它是要报告的事实不是失败
	// （caller_contract 第 1 条：历史上本来就有期次不齐，判成失败会让正常回填收不了工）。
	var errs []error
	if failed > 0 {
		errs = append(errs, fmt.Errorf("回填完成但有 %d 篇抓取失败（详见 %s 的 failed[]）", failed, store.Path()))
	}
	if rc.Failed() {
		errs = append(errs, fmt.Errorf("完整性对账不通过: %s", strings.Join(rc.Violations, "; ")))
	}
	return errors.Join(errs...)
}

// backfillReportWriter 决定对账报告写去哪：**nil ⇒ os.Stdout，不是 io.Discard**。
//
// 这是一个刻意的方向选择，不是随手填的默认值：本机制的全部意义是不让缺口静默消失，
// 而「调用方忘了设 Report ⇒ 什么都看不到」等于把它又关掉了一次——**默认值决定了
// 忘记的后果**，而忘记是一定会发生的。
//
// ⚠️ 由 TestBackfillReportWriterDefaultsToStdout 钉住。没有那条用例时，有人把它改成
// io.Discard 全包不会有任何反应——这正是本 sprint 反复出现的「注释里写着意图、
// 删掉却无一变红」的形状（我在 TASK-008 上因此返工过一轮）。
func backfillReportWriter(w io.Writer) io.Writer {
	return cmp.Or(w, io.Writer(os.Stdout))
}

// writeBackfillReconcileReport 把对账结果的**四个可见面**写出去。
//
// ⚠️ 四个面都要写（DoD functional[1]）：`Rows` 说每期什么情况、`MissingPeriods` 说哪几期缺、
// `Unclassified` 说哪些标题没认出来（可能是站点改了期次表述）、`Warnings` 说总数与推算值
// 差多少。少写任何一面，那一面的信息就只活在内存里。
//
// 「零期次」与「全部齐全」各有**自己的**特征文本：两者的 MissingPeriods 都是空的
// （caller_contract 第 3 条），只靠缺篇段读者分不出「什么都没抓到」和「抓全了」。
func writeBackfillReconcileReport(w io.Writer, rc backfillReconcile) {
	fmt.Fprintln(w, backfillReconcileHeader)
	fmt.Fprintf(w, "期数 %d / 篇数 %d\n", rc.Periods, rc.Articles)

	if rc.Empty {
		fmt.Fprintln(w, backfillReconcileEmptyLabel)
	}
	for _, row := range rc.Rows {
		line := fmt.Sprintf("  %s  %s  %d/%d", row.Period, row.Rule, row.Got, row.Want)
		if len(row.Missing) > 0 {
			line += "  缺: " + strings.Join(row.Missing, ", ")
		}
		fmt.Fprintln(w, line)
	}

	if len(rc.MissingPeriods) > 0 {
		fmt.Fprintf(w, "%s %s\n", backfillReconcileMissingLabel, strings.Join(rc.MissingPeriods, ", "))
	} else if !rc.Empty {
		fmt.Fprintln(w, backfillReconcileCompleteLabel)
	}
	if len(rc.Unclassified) > 0 {
		fmt.Fprintf(w, "%s %s\n", backfillReconcileUnclassifiedLabel, strings.Join(rc.Unclassified, " / "))
	}
	for _, s := range rc.Warnings {
		fmt.Fprintf(w, "%s %s\n", backfillReconcileWarningLabel, s)
	}
	for _, s := range rc.Violations {
		fmt.Fprintf(w, "%s %s\n", backfillReconcileViolationLabel, s)
	}
}

// errBackfillDisk 标记「本机落盘故障」，用来把它与搜索侧的取回/解析失败区分开：
// 后者 fail-open，前者必须中止。
var errBackfillDisk = errors.New("落盘失败")

// fetchBackfillSearchAll 遍历三个关键词、逐页取回搜索结果，顺带存快照。
//
// 返回的 error 一律交给 crossCheckBackfill 走 fail-open（ADR-M1c1-02：不让一个不可控的
// 第三方检索服务有权否决主路径交付），**唯独** errBackfillDisk 例外，由调用方中止。
//
// 🔴 **生产路径就走 fetchBackfillSearchPage**（QA C-1）。上一版在这里复制了一份逐字相同的
// 取回→解析→区间校验→栏目筛，于是那个函数**零生产调用方**：删掉它里面的区间校验、
// 或把区间校验挪到栏目筛之后（注释最强调的那条顺序），**全包一条测试都不会红** ——
// 6 条测试守着一段没人跑的代码。复制的理由是「那个函数把原始 HTML 丢了」，
// 而正确的解法是**让它把 HTML 交出来**，不是再写一遍。
//
// ⚠️ **单个关键词失败不连坐**（QA R2-6 第 2 步）：上一版任一关键词出错即 `return nil, …`，
// 把**已经收到的其它关键词的命中一起丢掉**。而触发条件是业务事实——社融存量/增量自
// 2025-09 停发，`--from ≥ 2025-09` 时后两个关键词必然零命中。现在逐关键词收集错误、
// 跑完全部，errors.Join 后交给 crossCheckBackfill 走 fail-open。
//
// errBackfillDisk 仍然立刻中止：落盘故障是本机问题，继续跑只会写出更多半截产物。
func fetchBackfillSearchAll(ctx context.Context, f Fetcher, out string, from, to time.Time) ([]backfillSearchHit, int, error) {
	var all []backfillSearchHit
	var errs []error
	pages := 0
	for i, kw := range backfillSearchKeywords {
		total := 1
		for page := 1; page <= total; page++ {
			res, err := fetchBackfillSearchPage(ctx, f, kw, from, to, page)
			// 快照先落盘再判错：解析失败那一页的原始 HTML 恰恰是事后查因唯一的材料。
			// res.HTML 为空只可能是 Fetcher 自己失败（那时没有响应体可存）。
			if len(res.HTML) > 0 {
				pages++
				name := fmt.Sprintf("%s-p%02d.html", backfillSearchSlug(kw, i), page)
				if werr := writeBackfillFile(filepath.Join(out, "search", name), res.HTML); werr != nil {
					return nil, pages, werr // 落盘故障：立刻中止，不 join
				}
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("搜索 %q 第 %d 页: %w", kw, page, err))
				break // 这个关键词到此为止，换下一个——不连坐
			}
			all = append(all, res.Hits...)
			total = res.TotalPages
		}
	}
	return all, pages, errors.Join(errs...)
}

// backfillSearchSlug 是关键词在快照文件名里的短名。
//
// 用 ASCII 短名而不是关键词原文：文件名要能被人和工具直接用，而中文文件名在
// macOS 上还会被规范化成 NFD。序号兜底保证**任何**关键词都有唯一名字——
// 表漏了一项也只是名字难看，不会让两个关键词写进同一个文件（那会静默互相覆盖）。
func backfillSearchSlug(keyword string, i int) string {
	if s, ok := backfillSearchSlugs[keyword]; ok {
		return s
	}
	return fmt.Sprintf("kw%d", i+1)
}

var backfillSearchSlugs = map[string]string{
	"金融统计数据报告":       "finstats",
	"社会融资规模存量统计数据报告": "tsf-stock",
	"社会融资规模增量统计数据报告": "tsf-flow",
}

// saveBackfillIndexPages 存 index 快照。
//
// 命名按设计附录 §A3 `<抓取序号>-<该页最新条目日期>_<该页最老条目日期>.html`：
// **页码不作唯一键**，因为页码随新文章上架而漂移，重跑时同一个文件名会对应不同内容的页
// ——而存快照的全部目的正是「重跑与 M1c-2 的核对都能离线做」。
func saveBackfillIndexPages(out string, pages []backfillIndexPage) error {
	for i, p := range pages {
		name := fmt.Sprintf("%03d-%s_%s.html", i+1, p.Newest, p.Oldest)
		if err := writeBackfillFile(filepath.Join(out, "index", name), p.HTML); err != nil {
			return err
		}
	}
	return nil
}

// writeBackfillFile 建目录并写文件，错误一律裹上 errBackfillDisk。
func writeBackfillFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("%w: 建目录 %s: %v", errBackfillDisk, filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("%w: 写 %s: %v", errBackfillDisk, path, err)
	}
	return nil
}
