package hestia

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// M1c-2 的 TASK-002：批量解析 M1c-1 抓下来的快照，产出**字段样本**与**失败清单**。
//
// 样本喂给 T3 的分布报告（人据此填 MagnitudeRanges）；失败清单是 M1c-4（LLM 兜底）的
// 工作量依据，也是 M1c-3 入库前要清零或显式豁免的东西。
//
// # 本文件反复出现的一条原则：不让东西静默消失
//
// 一篇文章走到这里有四种去向，**每一种都要能被数出来**：
//
//	① 出样本      —— 解析成功
//	② 本迭代不解析 —— 社融存量/增量两篇（M1c-3 的活）、monthly 期次（parse.go 显式拒绝）
//	③ 解析失败    —— 该支持却失败了，M1c-4 要兜的就是这批
//	④ 解析不出期次 —— 标题形态变了
//
// ②③ 混成一格的代价是**具体**的：v1 期次每期两篇社融，68 期就是 136 条假失败；
// monthly 在本轮语料里约 53 篇，混进去会是 53 条一模一样的
// "monthly is not supported yet" —— 真失败被淹没在里面。
// ④ 若 continue 掉就彻底消失了，backfill_reconcile.go:196 对同一处写过这条理由。

// CalibrateDeps 是标定所需的输入。
//
// Dir 是 M1c-1 的产出目录（fetch 的 --out），**不是** manifest.json 的路径：
// loadManifest 收目录，且 Article.File 存的是相对该目录的路径 —— 收文件路径就要在这里
// 再 filepath.Dir() 一次，凭空多一处可能出错的地方。
//
// Out 用来把「这一趟看到了什么」写给人看。
//
// 🔴 **同一个字段，两套契约，刻意不同**：
//   - `collectSamples` 收 **nil 合法** —— 纯取数的调用方（只要 Samples/Records，不要报告）；
//   - 导出入口 `Calibrate` 收 **nil 报错** —— 见 Calibrate 的注释。
//
// 理由：`collectSamples` 的产出是**数据**，打印是副产品；`Calibrate` 的产出**就是那份报告**。
// 一个「把报告写出来」的函数默认丢弃输出，等于把调用方的疏漏变成合法配置。
// ⚠️ 这条区别由 TestCalibrateRejectsNilOutWhileCollectSamplesAllowsIt 钉住 ——
// 光写在注释里的区别，下一个人重构时不会知道它是有意的。
type CalibrateDeps struct {
	Dir             string
	Out             io.Writer
	AllowIncomplete bool
}

// ParseFailure 是一篇的去向记录：Failures 里是「该支持却失败了」，
// Unsupported 里是「本迭代不解析」。两格共用同一形状，因为要填的四样完全相同 ——
// 期次、种类、文件、以及**为什么**。
type ParseFailure struct {
	Period, Kind, File, Err string
}

// CalibrateResult 是分布统计的原料。
//
// Periods 数**全部受支持种类**里被真正尝试解析的那些篇（含解析失败的）—— 它是「本轮
// 尝试了多少篇」，不是「manifest 里有多少篇」。
//
// ⚠️ **它一度只数《金融统计数据报告》**，M1c-3a 的 TASK-010 删掉 classifyArticles 的
// kind 硬过滤后就不是了。同一句过期声称还留在
// 渲染表头上，直到 QA 查出来 —— 别再按「只有金融统计」去读这个数。
type CalibrateResult struct {
	Periods int
	Samples map[string][]float64 // field → 各期实测值，未排序

	// Records 是逐期的原始观测，**带 period 与 period_type**（M1c-2 的 TASK-003 加）。
	//
	// 为什么必须有它：Samples 把 Meta 丢掉了（下面那个循环原本只取 obs.Values），
	// 而报告要回答两个问题，两个都需要 period_type：
	//   ① fieldOrder 里相当比例是 *_ytd **累计量** —— q1 与 annual 的量纲根本不同，
	//      混池算出的 min/max 会横跨整个范围，得到一个宽到拦不住任何东西的区间。
	//      MagnitudeRanges 只有 field 一维（不在本迭代改），所以工具**不替人解决**，
	//      但必须**让人看见样本混了哪几种**。
	//   ② tsf_stock 的环比变化率必须**在同一 period_type 内**算 —— Preceding 按
	//      period_type 隔离序列，annual 的「上一期」是去年的 annual。
	//
	// ⚠️ Samples **由 Records 派生**（见 samplesFromRecords），不是各写一份：
	// 同一事实的两个副本，改一处不会让另一处变红。
	Records []SampleRecord

	Failures     []ParseFailure // ③ 该支持却失败了
	Unsupported  []ParseFailure // ② 本迭代不解析（社融两篇 + monthly）
	Unclassified []string       // ④ 标题解析不出期次，原文照录

	// FetchFailed 是 manifest.failed 的转录：fetch 阶段就没抓到的篇目。
	// 它们既不在 articles 里也不在上面任何一格里，不带出来的话，报告会显示「失败：无」
	// —— 而失败表的用途正是「M1c-3 入库前要清零」。
	FetchFailed []Failed

	// Warnings 是「这份产物可不可信」的存疑项。不阻断：它们说的是语料的性质，
	// 不是本次标定出了错。
	Warnings []string

	// IncompleteAccepted 记「这份 manifest 没有 completed_at，是靠 AllowIncomplete 放行的」。
	//
	// 只有 collectSamples 知道这件事（它消费 d.AllowIncomplete 与 m.CompletedAt），而
	// Calibrate 要据此打印一句说明 —— 不带出来的话，放行就是**静默**的，报告的读者
	// 无从知道这批数据是在「缺完成标记」的前提下算出来的。
	//
	// ⚠️ 它**不表示「已确认完整」**：工具分辨不了「缺标记但完整」与「确实夭折」
	// （见上面 CompletedAt 那段——闭合性检查在夭折产物上同样全绿）。
	IncompleteAccepted bool
}

// collectSamples 读 manifest、解析**全部受支持种类**的报告、汇总各字段的取值。
//
// ⚠️ **这里一度只取《金融统计数据报告》**，理由是「Parse 只认它的格式，社融字段在另外
// 两篇独立报告里、本迭代没有解析器」。M1c-3a 的 TASK-010 做了社融两种的解析器并删掉了
// 那道硬过滤，理由随之失效 —— 现在三种报告都进来，各自贡献自己有的字段。
func collectSamples(d CalibrateDeps) (*CalibrateResult, error) {
	// loadManifest 对**文件不存在**返回空 Manifest + nil error —— 那是回填首跑的正常路径，
	// 在那边是对的。但在这里「目录里没有 manifest」意味着 --dir 指错了，若沿用那条语义，
	// 标定会拿一份零篇的 manifest 一路走下去。故先自己确认它在。
	p := filepath.Join(d.Dir, manifestFileName)
	switch _, err := os.Stat(p); {
	case os.IsNotExist(err):
		return nil, fmt.Errorf("%s 不存在：--dir 要指向 backfill 的产物目录（内含 %s 与 %s/），不是 manifest 文件本身",
			p, manifestFileName, articlesDirName)
	case err != nil:
		return nil, fmt.Errorf("查看 %s: %w", p, err)
	}

	st, err := loadManifest(d.Dir)
	if err != nil {
		return nil, err
	}
	m := st.Manifest

	// 夭折的 manifest 与正常完成的**结构上无法区分**（backfill_manifest.go 的 CompletedAt
	// 注释）：两者都是合法 JSON、sha256 全对、articles[] 与磁盘完全闭合，下游做的一切
	// 闭合性检查在夭折的产物上同样全绿。⇒ 判据只能是那个字段在不在，不能用「产物内部
	// 自洽」去替代。用半份数据标定会得出**偏窄**的区间，而偏窄的区间会在 M1c-3 回填时
	// 批量误拦 —— 那时人只会怀疑数据，不会怀疑区间。
	if m.CompletedAt == "" && !d.AllowIncomplete {
		return nil, errors.New("manifest 里没有 completed_at：这份产物可能是中途夭折的，" +
			"而夭折与正常完成在结构上无法区分；用半份数据标定会得出偏窄的区间。" +
			"若你有产物之外的证据能证明这趟跑完了，传 --allow-incomplete")
	}

	res := &CalibrateResult{Samples: map[string][]float64{}}
	// 走到这里而 CompletedAt 为空 ⇒ 必然是 AllowIncomplete 放行的（上面那个 if 已排除另一支）。
	res.IncompleteAccepted = m.CompletedAt == ""
	res.FetchFailed = append(res.FetchFailed, m.Failed...)
	res.Warnings = manifestWarnings(m)

	items := classifyArticles(res, m.Articles)

	shaUnverified := eachParsedArticle(d.Dir, res, items, func(p parsedArticle) {
		// 连 Meta 一起留下。此前这里只取 obs.Values，period_type 在采集的这一刻
		// 就在手边、却被丢掉了 —— 而下游报告恰恰需要它（见 CalibrateResult.Records）。
		res.Records = append(res.Records, SampleRecord{
			Period: p.obs.Meta.Period, PeriodType: p.obs.Meta.PeriodType, Values: p.obs.Values,
		})
	})

	// Samples 由 Records 派生，不在上面的循环里另写一份：两份副本会各自演化。
	// 顺序不变（items 按期次升序 ⇒ Records 升序 ⇒ 每个字段的切片也升序）。
	res.Samples = samplesFromRecords(res.Records)

	if shaUnverified > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"⚠ %d 篇的 manifest 没有 sha256，未做完整性校验：被截断的 HTML 仍可能 Parse 成功但少抽字段",
			shaUnverified))
	}

	// 先渲染再判错：即使下面拒绝了，看终端的人也该知道那 N 篇都去哪了。
	writeCollectSummary(d.Out, d.Dir, res)

	// 「空」的判据是**可用样本数为 0**，不是 len(Articles)==0：一份 400 篇全是社融、
	// 或标题形态全变了的目录，len(Articles) 不为 0 却一个样本都产不出 —— 那时报告会打印
	// 54 行全 `—`，退出码 0。
	if len(res.Samples) == 0 {
		return nil, fmt.Errorf("这份产物可用样本为 0（尝试解析 %d 篇、解析失败 %d 篇、"+
			"本迭代不解析 %d 篇、标题解析不出期次 %d 条）：没有样本标不出分布，"+
			"放行只会产出一份每格都是 — 的报告",
			res.Periods, len(res.Failures), len(res.Unsupported), len(res.Unclassified))
	}
	return res, nil
}

// parsedArticle 是共用管道交给两端的一条记录。
//
// 交出**整个 Observation** 而不只是 Values：calibrate 端只要 Values，而 M1c-3b 的
// TASK-003 的 load 端要靠 Meta 装配业务键 —— 只传 Values 的话那边会表现为
// 「合并组恒为 0」，看起来像语料问题、不像管道问题。
type parsedArticle struct {
	item calibrateItem
	obs  Observation
}

// eachParsedArticle 走「分类 → 读文件 → sha256 校验 → Parse」，
// 对每篇成功解析的调 fn；失败的记进 res.Failures/res.Unsupported。
// 返回 sha256 未校验的篇数（调用方汇总成 warning）。
//
// ⚠️ res.Periods++ 在 fail() **之前**：它数的是「全部受支持种类里被真正尝试解析的
// 篇数」，含解析失败的。挪到失败分流之后会让这个数静默变小，而单元测试全绿 ——
// 只有拿真语料背对背比对 calibrate 输出才抓得住。
func eachParsedArticle(dir string, res *CalibrateResult, items []calibrateItem,
	fn func(parsedArticle)) int {
	var shaUnverified int
	for _, it := range items {
		res.Periods++
		fail := func(reason string) {
			res.Failures = append(res.Failures, ParseFailure{
				Period: it.period, Kind: it.kind, File: it.a.File, Err: reason,
			})
		}

		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(it.a.File)))
		if err != nil {
			fail(fmt.Sprintf("读文件: %v", err))
			continue
		}

		// Article.SHA256 的用途（backfill_manifest.go:102）是「让下游验证本地文件未被
		// 篡改/截断」，而 calibrate 就是第一个下游。**必须在 Parse 之前查**：被截断的
		// HTML 可能仍 Parse 成功、只是少抽几个字段，那时该期会静默贡献一份残缺样本。
		if it.a.SHA256 == "" {
			shaUnverified++ // 出声跳过，收尾时汇总成一条 warning
		} else if got := articleSHA256(raw); got != it.a.SHA256 {
			fail(fmt.Sprintf("sha256 不符：manifest 记 %s，实际 %s —— 文件被改过或截断，"+
				"而截断的 HTML 仍可能 Parse 成功但少抽字段", it.a.SHA256, got))
			continue
		}

		obs, err := Parse(raw)
		if err != nil {
			// 🔴 **先分流「报告本身没有累计数据」，再记失败**（M1c-3a 的 TASK-010）。
			//
			// 这类报告（真语料 23 篇，全是 2020–2023 年间的月报）正文只有当月数、
			// 没有任何期内累计口径的合计句 —— 小标题都写成「三、4月份人民币存款增加…」。
			// 它们不是解析器不行，是**数据根本不存在**。
			//
			// 归 Failures 的代价是具体的：那一格在报告里的标题是「解析失败（该支持却
			// 失败了，M1c-3 入库前要清零）」、本文件顶部注释写着「M1c-4 要兜的就是这批」
			// —— 而 LLM 兜底也变不出不存在的数，等于给 M1c-4 加一批**永远清不了零**的工作量。
			//
			// ⚠️ 判据是**正向属性**而不是错误串匹配：问「这篇正文里有没有任何累计口径的
			// 合计句」，复用 cumulativePeriods 这张唯一真相源的表。错误文本会随实现措辞
			// 改动而失配，「有没有累计句」是报告本身的性质。
			//
			// ⚠️ **原始解析错误一并带上**：分类用的属性与「这篇为什么解析失败」是两件事。
			// 真语料里 2023-05 就是判为本类、而直接错误是「板块序号不连续」——不带上原错误，
			// 那个结构问题会被分类标签盖掉。
			if it.kind == backfillKindFinance && onlyCurrentMonthFlowSentences(stripHTML(raw)) {
				res.Unsupported = append(res.Unsupported, ParseFailure{
					Period: it.period, Kind: it.kind, File: it.a.File,
					Err: "本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，" +
						"*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）" +
						"；原始解析错误：" + err.Error(),
				})
				res.Periods-- // 它没被真正尝试解析成功过，不计入「本轮尝试了多少期」
				continue
			}
			// 原样带出 Parse 的错误：把它换成一句通用话，失败清单上 N 条就会长得一模一样，
			// 而 M1c-4 要按成因分工。
			fail(err.Error())
			continue
		}

		// 免费的交叉校验：Parse 从 HTML 自解期次，backfillPeriodOf 从 manifest 标题推期次
		// —— 两条独立推导。manifest 与文件错配时（AppendArticle 每篇立刻落盘，中途出错
		// 有窗口），没有这条则样本**静默来自错误期次**。
		if obs.Meta.Period != it.period {
			fail(fmt.Sprintf("期次交叉校验不一致：标题推出 %s，正文自解 %s —— manifest 与文件错配，"+
				"放行会让这一期的样本静默来自另一期", it.period, obs.Meta.Period))
			continue
		}

		fn(parsedArticle{item: it, obs: obs})
	}
	return shaUnverified
}

// calibrateItem 是一篇**确定要解析**的文章：期次已定、种类已定。
type calibrateItem struct {
	period string
	kind   string
	a      Article
}

// classifyArticles 把 manifest 里的篇目分成「要解析的」与另外三格。
//
// 返回的 items 按期次升序 —— 失败清单与样本顺序由它决定，稳定才能逐次 diff。
func classifyArticles(res *CalibrateResult, articles []Article) []calibrateItem {
	var items []calibrateItem
	for _, a := range articles {
		period, kind, ok := backfillPeriodOf(a.Title)
		if !ok {
			// **不 continue 掉**：解析不出期次的标题若被丢弃，它就从这张表上彻底消失了
			// ——而「站点改了期次表述」正是最需要被人看见的一类变化。
			res.Unclassified = append(res.Unclassified, a.Title)
			continue
		}
		// 🔴 **社融两种不再被硬过滤**（M1c-3a 的 TASK-010）。
		//
		// 这里原先写着「本迭代不解析该报告种类（社融存量/增量的解析器是 M1c-3 的活）」
		// 并 `continue` —— 而**本迭代就是 M1c-3**：解析器由 M1c-3a 的 TASK-002/003/007
		// 做完并接进了 `Parse` 的三路分派，calibrate 却仍不喂给它，于是报告里社融字段的
		// n 原地不动。那句 Err 是它自己过期的证据。
		//
		// ⚠️ **社融两种的 Observation 不能 Save**：同一期的三篇报告共享 (period, period_type)
		// 业务键，入库会撞主键。但 **calibrate 只统计字段值、不入库**（本函数下游只往
		// Records / Samples 里塞数，没有任何 Store 调用），所以这里安全。
		// 合并成一个 Observation 是 M1c-3b 的事。**下一个读到这里的人会问同样的问题，
		// 所以答案写在这里而不是提交信息里。**

		if reason := unsupportedPeriodType(a.Title); reason != "" {
			res.Unsupported = append(res.Unsupported, ParseFailure{
				Period: period, Kind: kind, File: a.File, Err: reason,
			})
			continue
		}
		items = append(items, calibrateItem{period: period, kind: kind, a: a})
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].period < items[j].period })
	sort.SliceStable(res.Unsupported, func(i, j int) bool {
		return res.Unsupported[i].Period < res.Unsupported[j].Period
	})
	sort.Strings(res.Unclassified)
	return items
}

// unsupportedPeriodType 回答「这个标题的 period_type，本迭代的解析器接不接」。
// 接 ⇒ 返回空串；不接 ⇒ 返回可读的理由。
//
// 判据取自 parse.go 的 checkPeriodTypeSupported，**不另写一份名单**：解除支持的方式是
// 删它的分支（monthly 那条在等月报样本），删掉后这里自动跟着放行。自己维护一份「哪些
// 不支持」会在解除时留下第二个必须同步的地方，而漏同步的表现是**静默少解析一批期次**。
//
// 只在 parseTitle 认得出标题时预判 —— 认不出的（如「山西省2024年8月金融统计数据报告」
// 这类分行报告，backfillTitleRE 不锚定起点会认下来）不猜，照常读文件走 Parse：Parse 看的
// 是 HTML 里的 ArticleTitle，报出的错误严格更多。
func unsupportedPeriodType(title string) string {
	_, periodType, _, err := parseTitle(title)
	if err != nil {
		return ""
	}
	if err := checkPeriodTypeSupported(periodType, title); err != nil {
		return fmt.Sprintf("本迭代解析器不支持 period_type=%s（parse.go 的 checkPeriodTypeSupported 显式拒绝，等样本）",
			periodType)
	}
	return ""
}

// manifestWarnings 把 manifest 里「这份产物可不可信」的三个字段读成人话。
//
// 三条都是**有声跳过**：不出声的话，「没做过这项检查」与「检查通过了」在读者看来完全
// 一样 —— 那正是 SearchSkippedReason 当初被要求落盘的理由（backfill_manifest.go:47，
// 而那句注释里的「读者」指的就是本迭代）。
func manifestWarnings(m Manifest) []string {
	var out []string
	if m.SearchSkippedReason != "" {
		out = append(out, fmt.Sprintf(
			"⚠ 这份产物没有做 index×search 交叉校验（原因：%s）：可能有整页被静默跳过，"+
				"那些篇目既不在 articles 里，也不在任何差集里", m.SearchSkippedReason))
	}
	if n := len(m.Failed); n > 0 {
		ids := make([]string, 0, n)
		for _, f := range m.Failed {
			ids = append(ids, f.ID)
		}
		out = append(out, fmt.Sprintf(
			"⚠ fetch 阶段有 %d 篇没抓到（manifest.failed）：它们既不在 articles 里，也不在下面的失败表里 —— %s",
			n, strings.Join(ids, " / ")))
	}
	switch {
	case m.Reconcile == nil:
		// 真跑用的那份产物出自 M1c-1 的 TASK-010 之前，**没有**这个字段。静默略过会让
		// 「序列没有洞」与「压根没对过账」看起来一样。
		out = append(out, "⚠ manifest 没有 reconcile 对账摘要（出自 TASK-010 之前）："+
			"看不出这份分布是不是算在一条有洞的序列上")
	case len(m.Reconcile.MissingPeriods) > 0:
		out = append(out, fmt.Sprintf("⚠ 对账记了 %d 个缺篇期次，这份分布算在一条有洞的序列上 —— %s",
			len(m.Reconcile.MissingPeriods), strings.Join(m.Reconcile.MissingPeriods, " / ")))
	}
	return out
}

// writeCollectSummary 把四格去向写给人看。w 为 nil ⇒ 不打印。
//
// 四格的**计数每次都打**（含 0），明细只在非空时展开：只打非空格子的话，读者无从判断
// 「这一格是 0」还是「这个实现根本没有这一格」。
func writeCollectSummary(w io.Writer, dir string, res *CalibrateResult) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "标定输入: %s\n", dir)
	// ⚠️ **表头不点名种类**（M1c-3a 的 TASK-010 返工，QA F1）：这一格原本写死
	// `backfillKindFinance`，而本任务删掉 classifyArticles 的 kind 硬过滤之后，
	// 它装的是全部受支持的种类，标称的那一种只占少数 —— 那句话是**用户可见的假话**，
	// 且已逐字进了标定验收报告。
	//
	// 实测构成（**条件与数字同行**：M1c-3a 标定验收那次真跑，manifest 218 篇，2026-08）：
	// `195 = 57 金融统计 + 138 社融（69 存量 + 69 增量）` ⇒ 标称种类占 29%。
	// ⚠️ 换一份 manifest 这三个数就变，**别拿它们当判据** —— 判据在测试里，钉的是性质。
	// 由 TestCollectSummaryHeaderDoesNotNameASingleKind 钉住（钉性质不钉取值）。
	fmt.Fprintf(w, "  待解析（受支持期次）: %d 篇\n", res.Periods)

	fmt.Fprintf(w, "  本迭代不解析: %d 篇\n", len(res.Unsupported))
	for _, line := range groupUnsupported(res.Unsupported) {
		fmt.Fprintf(w, "    - %s\n", line)
	}

	fmt.Fprintf(w, "  解析失败（M1c-4 的兜底工作量）: %d 篇\n", len(res.Failures))
	for _, f := range res.Failures {
		fmt.Fprintf(w, "    - %s  %s  %s\n", f.Period, f.File, f.Err)
	}

	fmt.Fprintf(w, "  标题解析不出期次: %d 条\n", len(res.Unclassified))
	for _, t := range res.Unclassified {
		fmt.Fprintf(w, "    - %s\n", t)
	}

	fmt.Fprintf(w, "  fetch 阶段未抓到: %d 篇\n", len(res.FetchFailed))
	for _, f := range res.FetchFailed {
		fmt.Fprintf(w, "    - %s  %s\n", f.ID, f.Error)
	}

	for _, warn := range res.Warnings {
		fmt.Fprintf(w, "  %s\n", warn)
	}
}

// groupUnsupported 把「本迭代不解析」按 (种类, 理由) 归并成每因一行。
//
// 不逐篇打：真语料上 monthly 约 53 篇，逐篇打就是 53 行同一句话，把另外几行淹掉
// ——而「淹掉真信号」正是这一格与失败表分开的理由，在渲染层再犯一次就白分了。
func groupUnsupported(us []ParseFailure) []string {
	type group struct {
		kind, reason string
		periods      []string
	}
	// groups 按首次出现的顺序存 *group（us 已按期次排过），byKey 只用来找已有的那一组
	// —— 存 key 再回查一次 map 是多绕一道。
	var groups []*group
	byKey := map[string]*group{}
	for _, u := range us {
		key := u.Kind + "\x00" + u.Err
		g := byKey[key]
		if g == nil {
			g = &group{kind: u.Kind, reason: u.Err}
			byKey[key] = g
			groups = append(groups, g)
		}
		g.periods = append(g.periods, u.Period)
	}

	out := make([]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, fmt.Sprintf("%d × [%s] %s: %s",
			len(g.periods), g.kind, g.reason, strings.Join(g.periods, ", ")))
	}
	return out
}

// SampleRecord 是**一期**的观测：期次、期次类型、以及该期抽出的全部字段值。
//
// 它比 Samples 多的就是 Meta 那两项 —— 而报告要按 period_type 分组/标注，全靠它们。
//
// ⚠️ **不要给它加导出方法**（如 String()）：store_test.go 的
// TestPackageExposesNoWriteFunctions 断言包的导出面**精确相等**，导出接收者上的
// 导出方法会被记作 SampleRecord.String 而让它变红，而那个文件不在本任务的 writes 里。
type SampleRecord struct {
	Period     string // "YYYY-MM"，字典序即时间序
	PeriodType string // monthly | q1 | h1 | q1_q3 | annual
	Values     map[string]float64
}

// onlyCurrentMonthFlowSentences 回答「这篇报告**有**存/贷款合计句，但**没有一条是累计口径**」。
//
// 用途只有一个：把「报告本身没有累计数据」与「该支持却失败了」分开（M1c-3a 的 TASK-010）。
//
// 🔴 **两个条件缺一不可，`any` 那半是实撞出来的**：最初只判「没有累计句」，
// 结果一个 index 页（正文里连合计句都没有、`Parse` 在 `missing <meta name="PubDate">`
// 就失败了）被判成「该期报告只有当月数」——**一句关于它的假话**，而且会把一条真失败
// 从 M1c-4 的清单上抹掉。「没有累计句」是必要条件，不是充分条件：
// **要先证明它确实是一篇有数据的报告**（至少有一条合计句），再说它的数据全是当月口径。
//
// 🔴 **复用唯一真相源，不另写判据**：期次前缀交给 loanFlowRE / depositFlowRE 捕获，
// 口径交给 cumulativePeriods 查表 —— 与 extract.go 的 selectRMBCumulativeFlow 问的是
// 同一个问题（「这个前缀算不算累计」），只是这里问「有没有」、那里问「是哪一条」。
// 自己在这里列一份「哪些前缀算累计」的名单，会在 profiles.go 加新前缀时静默分叉。
//
// ⚠️ 只对《金融统计数据报告》有意义：社融两种的正文没有存贷款合计句，对它们调用会
// 恒返回 false —— 调用点已用 kind 限定，这里不再重复判断，但改动调用点时要记得。
func onlyCurrentMonthFlowSentences(text string) bool {
	var any, cumulative bool
	for _, re := range []*regexp.Regexp{loanFlowRE, depositFlowRE} {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			any = true
			if cumulativePeriods[m[1]] {
				cumulative = true
			}
		}
	}
	return any && !cumulative
}

// samplesFromRecords 把逐期记录摊平成 field → 各期值。
//
// 保序：入参按期次升序时，每个字段的切片也按期次升序 —— tsf_stock 的环比要用这个顺序
// （而 computeFieldStats 刻意不就地排序，正是为了不毁掉它）。
func samplesFromRecords(recs []SampleRecord) map[string][]float64 {
	out := map[string][]float64{}
	for _, r := range recs {
		for f, v := range r.Values {
			out[f] = append(out[f], v)
		}
	}
	return out
}

// incompleteNotice 是「为什么放行了一份没有完成标记的 manifest」那句说明。
//
// 措辞刻意**不声称已确认完整**：工具分辨不了「缺标记但完整」（本次真跑用的那份产物
// 早于 M1c-1 的 TASK-010 引入 completed_at）与「确实夭折」（进程中途被杀）——
// CompletedAt 的注释写明闭合性检查在夭折产物上同样全绿。⇒ 只能把事实摆出来，
// 让读报告的人自己判断，不能替他下结论。
const incompleteNotice = "⚠ 该 manifest 无完成标记（completed_at），已按 --allow-incomplete 放行；" +
	"若它出自 TASK-010 之前的 fetch，属预期。**这不代表已确认完整** —— " +
	"夭折的产物与正常完成的在结构上无法区分。"

// Calibrate 是标定的导出入口：读产物目录、统计分布、把报告写给 d.Out。
//
// 形态与 BackfillFetch 类似（一个导出入口 + 一个 Deps 结构体 + 返回 error），
// 但**不收 ctx**：collectSamples 全是本地文件 IO，没有网络也没有可取消的长操作，
// 收一个用不到的 ctx 只会让调用方以为它能取消。
//
// 🔴 **d.Out 为 nil 时报错，不退化成 io.Discard。**
// 本函数的产出**就是那份报告**；默认丢弃输出会把调用方的疏漏变成合法配置 ——
// 具体的失效形态是：cmd 层装配时漏填 Out 字段 ⇒ 命令静默打印零字节、退出码 0，
// 而「子命令注册了吗」「flag 解析对吗」这类测试**全部通过**。
// ⚠️ 与 collectSamples 相反（那边 nil 合法），区别的理由见 CalibrateDeps.Out。
func Calibrate(d CalibrateDeps) error {
	if d.Out == nil {
		return errors.New("hestia: Calibrate 需要 Out：本函数的产出就是那份报告，" +
			"没有 Out 就没有产出。（collectSamples 允许 Out 为 nil，那是纯取数的路径）")
	}

	res, err := collectSamples(d)
	if err != nil {
		return err
	}

	// 放行说明排在报告**之前**：它是读下面每一个数的前提，放在末尾等于让人读完才知道。
	if res.IncompleteAccepted {
		fmt.Fprintf(d.Out, "%s\n\n", incompleteNotice)
	}

	return renderCalibrateReport(d.Out, res)
}
