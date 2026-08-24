package hestia

import (
	"bufio"
	"fmt"
	"io"
	"slices"
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
// ⚠️ 用**整数**运算，不写 math.Ceil(q*float64(n))：后者要依赖 0.05 / 0.95 这类
// 不可精确表示的常量在乘法之后恰好舍入到整数上。n=100 时它碰巧成立，换个 n 就可能
// 差一名 —— 而**差一名不会有任何东西报错**，只会让报告上的分位数悄悄错一格。
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
// 这句话变成假话** —— 而失败表的用途正是「M1c-3 入库前要清零」。
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
	fmt.Fprintf(bw, "%-28s %4s %12s %12s %12s %12s %12s  %s\n",
		"字段", "n", "min", "p5", "median", "p95", "max", "建议区间")

	// 遍历 fieldOrder 而不是 res.Samples：map 迭代序随机，同一份数据两次跑会打出
	// 不同的行序，无法逐次 diff；且零样本字段会整行消失 —— 而「这个字段一个样本都
	// 没有」恰恰是读者最需要看见的一件事。
	for _, f := range fieldOrder {
		writeFieldRow(bw, computeFieldStats(f, res.Samples[f]))
	}
	fmt.Fprintln(bw)

	writeFailureSection(bw, res)
	return bw.Flush()
}

// writeFieldRow 渲染一行。零样本打 noValueMark，n<3 的建议列打 noSuggestionMark。
func writeFieldRow(w io.Writer, s FieldStats) {
	if s.N == 0 {
		fmt.Fprintf(w, "%-28s %4d %12s %12s %12s %12s %12s  %s\n",
			s.Field, 0, noValueMark, noValueMark, noValueMark, noValueMark, noValueMark, noValueMark)
		return
	}
	// 建议区间不留空格：报告按列解析（测试与人都是），`[a, b]` 会被切成两列。
	sugg := noSuggestionMark
	if s.HasSuggestion {
		sugg = fmt.Sprintf("[%g,%g]", s.SuggestMin, s.SuggestMax)
	}
	fmt.Fprintf(w, "%-28s %4d %12g %12g %12g %12g %12g  %s\n",
		s.Field, s.N, s.Min, s.P5, s.Median, s.P95, s.Max, sugg)
}

// writeFailureSection 渲染四种去向里的后三种，外加 fetch 阶段的失败。
//
// ⚠️ 「本迭代不解析」与「解析失败」**分两段**，不是排版洁癖：真语料里前者 193 篇
// （社融两篇 69+69 + monthly 55），混进失败表就是 193 条假失败，真失败被淹没在里面。
func writeFailureSection(w io.Writer, res *CalibrateResult) {
	if len(res.Failures) == 0 && len(res.FetchFailed) == 0 {
		fmt.Fprintf(w, "%s\n", noFailureClaim)
	}

	writeParseFailures(w, "解析失败（该支持却失败了，M1c-3 入库前要清零）", res.Failures)

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
