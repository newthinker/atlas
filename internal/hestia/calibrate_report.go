package hestia

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"
	"strings"
)

// M1c-2 的 TASK-003：把 collectSamples 的原料变成一份**给人看的**分布报告。
//
// # 这份报告的读者会拿它做决定
//
// 标定的输出是**证据**不是**决定**（工具不改 configs/hestia.yaml）。但读者会照着它填
// MagnitudeRanges —— 所以报告的每一处含糊都会变成一个填错的区间，而填错的区间在
// M1c-3 回填时会批量误拦，那时人只会怀疑数据，不会怀疑区间。
//
// 本文件**不碰文件系统、不碰网络**：全部输入由调用方注入（io.Writer 也是）。

// minSamplesForSuggestion 是给出建议区间所需的最少样本数。
//
// 少于它就不给建议（HasSuggestion=false），而不是给一个窄的。理由：n 很小时 span
// 往往极小甚至为 0，任何余量规则都会退化成一个几乎没有宽度的区间 ——
// **过窄的建议比没有建议更危险：它看起来是个结论。**
//
// ⚠️ 判据是 **n**，不是 `span > 0`。后者在 n=2 且两值不同时照样给建议，
// 本常量的存在理由就此落空（TestSuggestionWithheldBelowMinSamples 的 n=2 那格
// 刻意用两个**不同**的值，就是为了让这种写法红）。
const minSamplesForSuggestion = 3

// FieldStats 是单个字段的取值分布，外加一个**建议**区间。
//
// SuggestMin/SuggestMax 只在 HasSuggestion 为 true 时有意义。两者分开而不是用
// 零值表达「没有建议」：0 是一个合法的区间端点，用它兼职会让「建议 0」与「不建议」
// 不可分 —— 与 Thresholds.CaliberExemptions 那条「空切片不默认全部」同源。
type FieldStats struct {
	Field string
	N     int

	Min, P5, Median, P95, Max float64

	SuggestMin, SuggestMax float64
	HasSuggestion          bool
}

// computeFieldStats 统计一个字段的样本分布。
//
// # 分位数用 nearest-rank，不插值
//
// rank = ceil(pct*n/100)，取第 rank 小。**不做线性插值**：这份报告是给人看分布、
// 据以决定「区间该定多宽」的，插值带来的那点精度对那个决定没有任何影响，
// 却多一处能算错的地方 —— 而算错了不会有任何东西报错，只会让报告上多一个
// 看起来完全正常的数。
//
// ⚠️ **不就地排序**：入参是 CalibrateResult.Samples 里的切片本身，而它的顺序是
// **期次升序**（TASK-002 discovery 明写），tsf_stock 的环比变化率正依赖那个顺序。
// 就地排序会毁掉它，而**本函数自己的全部断言照样绿** —— 受害的是另一个函数。
func computeFieldStats(field string, samples []float64) FieldStats {
	s := FieldStats{Field: field, N: len(samples)}
	if s.N == 0 {
		return s // 分位数无定义；渲染层据 N==0 打 "—"
	}

	sorted := slices.Clone(samples)
	slices.Sort(sorted)

	s.Min, s.Max = sorted[0], sorted[len(sorted)-1]
	s.P5 = nearestRank(sorted, 5)
	s.Median = nearestRank(sorted, 50)
	s.P95 = nearestRank(sorted, 95)

	if s.N < minSamplesForSuggestion {
		return s
	}

	// 加性余量 [min-span, max+span]，**不是乘性**。
	//
	// 乘性规则在可为负的字段上会把已观测到的真实值排除在外：deposit_household_ytd
	// 的 min=-8200 时 min*0.5 = -4100 > -8200 ⇒ 建议区间把实测最小值挡在外面，
	// 而它看起来完全像一个正常区间。加性余量与符号无关，不会犯这个错。
	span := s.Max - s.Min
	s.SuggestMin, s.SuggestMax = s.Min-span, s.Max+span
	s.HasSuggestion = true
	return s
}

// nearestRank 返回 sorted 里第 ceil(pct*n/100) 小的值（rank 从 1 起）。
//
// ⚠️ 用**整数**运算，不写 math.Ceil(float64(pct)/100*float64(n))：后者依赖
// pct/100 这个不可精确表示的常量在乘法之后恰好舍入到整数上。
//
// 🔴 **脆弱性在 pct，不在 n** —— 这条理由订正过一次，原文写的是「n=100 时碰巧对，
// 换个 n 就可能差一名」，**方向反了**。实测（n ∈ [1,200000] 全扫，两种实现逐点比对）：
//
//	pct = 5 / 50 / 95（本报告实际用的三个） → 差异 0 处，换任何 n 都不差
//	pct = 7                                → 1612 处，首个恰在 n=100
//	pct = 14                               → 3222 处，首个 n=50
//	pct = 28                               → 6442 处，首个 n=25
//
// ⇒ 「首个差异在 n=100」这个现象真实存在，但它发生在 **pct=7** 上。
// 本报告现用的 5/50/95 恰好无差异 —— **那是这三个 pct 的性质，不是算法的性质**。
// ⚠️ 照订正前那句「换个 n 去验」的人会**一无所获**，可能因此判定这道防御多余而改掉它；
// 要复现差异得**换 pct**（加一档 p7 / p25 就会撞上）。而**差一名不会有任何东西报错**，
// 只会让报告上的分位数悄悄错一格。
func nearestRank(sorted []float64, pct int) float64 {
	rank := (pct*len(sorted) + 99) / 100 // 整数上取整
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

// 报告里的三个标记串。提成常量而不是散在 Fprintf 里：测试要拿它们做断言，
// 两处字面量对不上时会变成一条**红得莫名其妙**的用例。
const (
	// noValueMark 是「这一格没有数」。**不能用 0** —— 0 是一个合法的实测值，
	// 用它兼职会让「没样本」与「实测为 0」在报告上不可分，而读者据此填区间。
	noValueMark = "—"

	// noSuggestionMark 是「样本太少，不给建议」。必须**显式**打出来：
	// 留空会被读成「区间就是这么宽」，而那正是过窄建议的危害所在。
	noSuggestionMark = "n<3"

	// noFailureClaim 是「一件失败都没有」这句话。只有 Failures 与 FetchFailed
	// **都**为空时才允许出现 —— 见 TestRenderCalibrateReportCountsFetchFailures…。
	noFailureClaim = "失败：无"
)

// renderCalibrateReport 把标定结果写成一份给人看的报告。
//
// # 为什么四种去向都要渲染，而不只是 Samples
//
// 一篇文章走到 collectSamples 有四种去向（出样本 / 本迭代不解析 / 该支持却失败了 /
// 标题解析不出期次），外加 fetch 阶段就没抓到的。**只渲染 Samples 会让「失败：无」
// 这句话变成假话** —— 而失败表的用途正是**让该修的东西可见**。
//
// 不碰文件系统：w 由调用方注入。
func renderCalibrateReport(w io.Writer, res *CalibrateResult) error {
	bw := bufio.NewWriter(w)

	fmt.Fprintf(bw, "标定报告：尝试解析 %d 期\n\n", res.Periods)

	if len(res.Warnings) > 0 {
		fmt.Fprintf(bw, "存疑（%d 条，不阻断，说的是语料的性质）\n", len(res.Warnings))
		for _, s := range res.Warnings {
			fmt.Fprintf(bw, "  - %s\n", s)
		}
		fmt.Fprintln(bw)
	}

	// 表头里的期数是「尝试解析多少期」，**不是**任何一个字段的样本数：
	// 两者差着解析失败的那几篇。n 列才是逐字段的样本数，故不可省。
	fmt.Fprintf(bw, "字段分布（尝试 %d 期；n = 该字段实际取到的样本数）\n", res.Periods)
	fmt.Fprintf(bw, "%-28s %4s %-22s %12s %12s %12s %12s %12s  %s\n",
		"字段", "n", "期次类型", "min", "p5", "median", "p95", "max", "建议区间")

	// 遍历 fieldOrder 而不是 res.Samples：map 迭代序随机，同一份数据两次跑会打出
	// 不同的行序，无法逐次 diff；且零样本字段会整行消失 —— 而「这个字段一个样本都
	// 没有」恰恰是读者最需要看见的一件事。
	for _, f := range fieldOrder {
		writeFieldRowWithMix(bw, computeFieldStats(f, res.Samples[f]), periodTypeMix(res.Records, f))
	}
	fmt.Fprintln(bw)

	writeStockRateSection(bw, res.Records)
	writeFailureSection(bw, res)
	return bw.Flush()
}

// fieldCells 把一行的六个数值格算成字符串。
//
// 提出来是因为字段表与环比表**列数不同**（前者多一列「期次类型」），而两处若各写一遍
// 「零样本打什么、n<3 打什么」，改一处不会让另一处变红 —— 同一事实的两个副本。
func fieldCells(s FieldStats) (min, p5, med, p95, max, sugg string) {
	if s.N == 0 {
		m := noValueMark
		return m, m, m, m, m, m
	}
	sugg = noSuggestionMark
	if s.HasSuggestion {
		// 不留空格：报告按列解析（测试与人都是），`[a, b]` 会被切成两列。
		sugg = fmt.Sprintf("[%g,%g]", s.SuggestMin, s.SuggestMax)
	}
	g := func(v float64) string { return fmt.Sprintf("%g", v) }
	return g(s.Min), g(s.P5), g(s.Median), g(s.P95), g(s.Max), sugg
}

// writeFieldRow 渲染不带「期次类型」列的一行（环比一节用，行首是 period_type）。
func writeFieldRow(w io.Writer, s FieldStats) {
	min, p5, med, p95, max, sugg := fieldCells(s)
	fmt.Fprintf(w, "%-28s %4d %12s %12s %12s %12s %12s  %s\n",
		s.Field, s.N, min, p5, med, p95, max, sugg)
}

// writeFieldRowWithMix 渲染字段表的一行，第 3 列是样本来自哪几种 period_type。
func writeFieldRowWithMix(w io.Writer, s FieldStats, mix string) {
	min, p5, med, p95, max, sugg := fieldCells(s)
	fmt.Fprintf(w, "%-28s %4d %-22s %12s %12s %12s %12s %12s  %s\n",
		s.Field, s.N, mix, min, p5, med, p95, max, sugg)
}

// writeFailureSection 渲染四种去向里的后三种，外加 fetch 阶段的失败。
//
// ⚠️ 「本迭代不解析」与「解析失败」**分两段**，不是排版洁癖：真语料里前者 193 篇
// （社融两篇 69+69 + monthly 55），混进失败表就是 193 条假失败，真失败被淹没在里面。
func writeFailureSection(w io.Writer, res *CalibrateResult) {
	if len(res.Failures) == 0 && len(res.FetchFailed) == 0 {
		fmt.Fprintf(w, "%s\n", noFailureClaim)
	}

	// ⚠️ 归属措辞与摘要栏那行（calibrate.go 的 writeCollectSummary）**必须一致**：
	// 同一批篇目在一份报告里出现两个归属，读者会以为是两批不同的东西
	// （M1c-3b 的 TASK-008 统一为「M1c-4 的兜底工作量」）。
	writeParseFailures(w, "解析失败（该支持却失败了，M1c-4 的兜底工作量）", res.Failures)

	if len(res.FetchFailed) > 0 {
		fmt.Fprintf(w, "抓取失败（%d 篇，fetch 阶段就没抓到）\n", len(res.FetchFailed))
		for _, f := range res.FetchFailed {
			fmt.Fprintf(w, "  %s  %s  %s\n", f.ID, f.URL, f.Error)
		}
		fmt.Fprintln(w)
	}

	writeParseFailures(w, "本迭代不解析（不是失败）", res.Unsupported)

	if len(res.Unclassified) > 0 {
		fmt.Fprintf(w, "标题解析不出期次（%d 条，原文照录 —— 非 0 意味着站点改了期次表述）\n",
			len(res.Unclassified))
		for _, s := range res.Unclassified {
			fmt.Fprintf(w, "  %s\n", s)
		}
		fmt.Fprintln(w)
	}
}

func writeParseFailures(w io.Writer, title string, fs []ParseFailure) {
	if len(fs) == 0 {
		return
	}
	fmt.Fprintf(w, "%s（%d 篇）\n", title, len(fs))
	for _, f := range fs {
		fmt.Fprintf(w, "  %s  %s  %s  %s\n", f.Period, f.Kind, f.File, f.Err)
	}
	fmt.Fprintln(w)
}

// stockRateSectionTitle 是环比一节的标题。提成常量供测试断言，理由同上面三个标记串。
//
// ⚠️ **标题刻意不以字段名开头。** 原文是「tsf_stock 相邻期环比变化率…」，
// 于是这一行的首列恰好等于字段名 tsf_stock ⇒ 报告的按列解析把**标题行**也当成了
// 该字段的数据行（reportRow 的「恰好 1 行」断言当场逮住，实际 2 行）。
// 处置是**消掉共有词本身**（把字段名挪出行首），不是在断言那边绕开它 ——
// 绕开只会让下一个按列读这份报告的人（或人眼）再撞一次。
const stockRateSectionTitle = "环比变化率分布：tsf_stock 相邻期（按 period_type 分档）"

// periodTypeMix 把某字段的样本来源摊成 "annual×3,h1×2,q1×1"。
//
// # 为什么每行都要标这个
//
// fieldOrder 里相当比例是 *_ytd **累计量**，q1（3 个月）与 annual（12 个月）的量纲
// 根本不同。混池后 min/max 横跨整个范围，再加余量 ⇒ **一个宽到拦不住任何东西的区间**。
// MagnitudeRanges 只有 field 一维（本迭代不改它的类型）⇒ 工具**不替人解决**，
// 但必须让人看见「这一行的样本混了哪几种」。
//
// 定序输出（按 period_type 名排序）：map 迭代序随机，不排的话同一份数据两次跑打出的
// 顺序不同，报告无法逐次 diff。分隔符不带空格 —— 报告按列解析。
func periodTypeMix(recs []SampleRecord, field string) string {
	n := map[string]int{}
	for _, r := range recs {
		if _, ok := r.Values[field]; ok {
			n[r.PeriodType]++
		}
	}
	if len(n) == 0 {
		return noValueMark
	}
	parts := make([]string, 0, len(n))
	for _, pt := range slices.Sorted(maps.Keys(n)) {
		parts = append(parts, fmt.Sprintf("%s×%d", pt, n[pt]))
	}
	return strings.Join(parts, ",")
}

// stockContinuityRates 算 tsf_stock 逐 period_type 的**相邻期**环比变化率分布。
//
// # 这一节存在的理由
//
// 报告其余部分只给字段的**原始值**分布，而 StockContinuityMax 管的是**环比变化率**
// 的上限。没有这一节，这台专门产出标定依据的机器对「本 sprint 刚改的那个阈值」
// 一言不发 —— TASK-001 把一个拍脑袋的数（0.02）改成了两个（0.02 / 0.15），
// 而它俩至今没有任何经验依据。
//
// # 🔴「相邻期」= 排序后相邻的两个样本，**不是**「相差一个季度/一年」
//
// 真实序列**有洞**：2024 年只有年报/上半年/前三季度，没有一季度；另有 3 篇 Parse
// 失败（2019-12 / 2020-09 / 2022-09）各挖一个。若按「期次必须相差固定间隔」配对，
// **跨洞的那一对会被整个丢掉**，而丢掉不报错，只会让 n 悄悄变小、分布悄悄变窄
// —— 然后有人拿这份变窄的分布去定阈值。
//
// 取绝对值：存量下跌同样是跳变，用 cur-prev 会漏掉整个下跌方向，而社融存量骤降
// 恰恰是最该报警的情形（与 gateStockContinuity 同口径）。
// 上一期为 0 时**跳过该对**：Inf 会污染整段分位数，且报告上的 +Inf 会被读成
// 「这个字段疯了」，实际只是分母恰好为 0。
func stockContinuityRates(recs []SampleRecord) map[string]FieldStats {
	byType := map[string][]SampleRecord{}
	for _, r := range recs {
		if _, ok := r.Values[FieldTSFStock]; ok {
			byType[r.PeriodType] = append(byType[r.PeriodType], r)
		}
	}

	out := map[string]FieldStats{}
	for pt, rs := range byType {
		// Period 是 "YYYY-MM"，字典序即时间序。
		slices.SortFunc(rs, func(a, b SampleRecord) int { return strings.Compare(a.Period, b.Period) })

		var rates []float64
		for i := 1; i < len(rs); i++ {
			prev, cur := rs[i-1].Values[FieldTSFStock], rs[i].Values[FieldTSFStock]
			if prev == 0 {
				continue
			}
			rates = append(rates, math.Abs(cur-prev)/math.Abs(prev))
		}
		// 只有一期（或全被零分母跳过）⇒ 没有相邻对。**不产生一个 0** ——
		// 那会在报告上凭空多出一档「环比 0%」，读者据此以为该序列极其平稳。
		if len(rates) == 0 {
			continue
		}
		out[pt] = computeFieldStats(pt, rates)
	}
	return out
}

// writeStockRateSection 渲染环比一节，按 period_type 名定序，一档一行。
func writeStockRateSection(w io.Writer, recs []SampleRecord) {
	rates := stockContinuityRates(recs)
	fmt.Fprintf(w, "%s\n", stockRateSectionTitle)
	if len(rates) == 0 {
		fmt.Fprintf(w, "  %s（没有任何一档取到 >= 2 期 tsf_stock 样本）\n\n", noValueMark)
		return
	}
	fmt.Fprintf(w, "%-28s %4s %12s %12s %12s %12s %12s  %s\n",
		"period_type", "n", "min", "p5", "median", "p95", "max", "建议区间")
	for _, pt := range slices.Sorted(maps.Keys(rates)) {
		writeFieldRow(w, rates[pt]) // n<3 的规则照常适用，**不为「样本本来就少」破例**
	}
	fmt.Fprintln(w)
}
