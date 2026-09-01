package hestia

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// —— 回填入库的核对报告（M1c-3b 的 TASK-006）——

// writeLoadReport 把核对报告写给人看，并校验四道恒等式。任一不成立返回 error。
//
// 🔴 **恒等式先校验、后打印**：报告本身就是验收物，它不能在数字对不上时照样打印
// 一份好看的表格 —— 一份自洽的表格会让人停止追问，而那正是账对不上的时候最不该发生的事。
//
// 分节顺序固定（稳定才能逐次 diff）：顺序会飘的报告，每次跑出来的 diff 都是噪声，
// 而噪声会让人不再逐行看。由 TestLoadReportSectionsAreInFixedOrder 钉住。
func writeLoadReport(w io.Writer, dir string, res *BackfillLoadResult) error {
	if err := checkLoadIdentities(res); err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "标定输入: %s\n", dir)

	// 放行说明排在报告**之前**：它是读下面每一个数的前提，放在末尾等于让人读完才知道。
	// 与 Calibrate 的 incompleteNotice 同一位置、同一理由，但后果更重：calibrate 传
	// --allow-incomplete 的代价是「区间可能偏窄」，load 的代价是**历史序列直接缺期**。
	if res.IncompleteAccepted {
		b.WriteString("\n⚠️ 这份 manifest 没有 completed_at，是靠 --allow-incomplete 放行的。\n" +
			"   夭折与正常完成在结构上无法区分，若它确实夭折，本次回填会让历史序列**缺期**，\n" +
			"   而缺的那些期在库里与「央行本来就没发」完全同形。\n")
	}

	fmt.Fprintf(&b, `
  语料总篇数:   %5d
  待解析:       %5d
  本迭代不解析: %5d
  解析成功:     %5d
  解析失败:     %5d   ← 原封留给 M1c-4
  合并后观测:   %5d   （单篇 %d + 合并组 %d）
  入权威表:     %5d
  落 pending:   %5d
`, res.Total, res.Attempted, res.Unsupported, res.ParsedOK, res.ParseFailed,
		res.Merged, res.SingleArticle, res.MergedGroups, res.ToObservations, res.ToPending)

	if n := len(res.Unclassified); n > 0 {
		fmt.Fprintf(&b, "\n标题解析不出期次: %d 条（原文照录，它们不在上面任何一格里）\n", n)
		for _, t := range res.Unclassified {
			fmt.Fprintf(&b, "  %s\n", t)
		}
	}

	b.WriteString("\n四道恒等式: 全部成立 ✓\n")

	fmt.Fprintf(&b, "\n合并组明细（%d 组）\n", len(res.Groups))
	if len(res.Groups) == 0 {
		b.WriteString("  （无）\n")
	}
	for _, g := range res.Groups {
		fmt.Fprintf(&b, "  %s/%s  ← %s\n", g.Obs.Meta.Period, g.Obs.Meta.PeriodType,
			strings.Join(g.SourceIDs, " + "))
		fmt.Fprintf(&b, "      代表 article_id: %s", g.Obs.Meta.ArticleID)
		if len(g.DroppedIDs) > 0 {
			// 逐条列出，不是只报个数：丢弃可以，不出声不行 —— 报个数的话，
			// 想查「到底丢了哪篇」时无处可查，而这里是 DroppedIDs 的唯一载体。
			fmt.Fprintf(&b, "   丢弃: %s", strings.Join(g.DroppedIDs, ", "))
		}
		b.WriteString("\n")
	}

	// 部分覆盖：入了权威表、但不是由全部三族报告合成的期次（人类裁决，缺口 A-3）。
	// 它们的 completeness **会通过**（各自 extractor 的必填集就是那几个字段），
	// 在库里与「央行本来就没发」完全同形 —— 工具不替人合并，但必须让它出声。
	fmt.Fprintf(&b, "\n部分覆盖的期次（%d）\n", len(res.PartialCoverage))
	if len(res.PartialCoverage) == 0 {
		b.WriteString("  （无）\n")
	}
	for _, g := range res.PartialCoverage {
		fmt.Fprintf(&b, "  %s/%s  缺: %s\n", g.Obs.Meta.Period, g.Obs.Meta.PeriodType,
			strings.Join(missingFamilies(g.Parts), "、"))
	}

	fmt.Fprintf(&b, "\n字段冲突（预期 0，共 %d）\n", len(res.Conflicts))
	if len(res.Conflicts) == 0 {
		b.WriteString("  （无）\n")
	}
	for _, c := range res.Conflicts {
		// 冲突非空即表示**字段归属表有错**，不是数据问题（三个 extractor 的字段集
		// 设计上不相交）——所以这里报的是「哪张表错了」的线索，不是「哪个值对」。
		fmt.Fprintf(&b, "  %s/%s  %s: %g(%s) vs %g(%s)\n",
			c.Period, c.PeriodType, c.Field, c.A, c.FromA, c.B, c.FromB)
	}

	fmt.Fprintf(&b, "\n落 pending 的期次（%d）\n", len(res.PendingReasons))
	if len(res.PendingReasons) == 0 {
		b.WriteString("  （无）\n")
	}
	// 排序输出：map 迭代顺序随机，同一份数据两次跑报出不同顺序会让逐次 diff 失效
	// —— 与 gateMagnitudeSanity 遍历 fieldOrder 同一个理由。
	keys := make([]string, 0, len(res.PendingReasons))
	for k := range res.PendingReasons {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "  %-20s %s\n", k, res.PendingReasons[k])
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// checkLoadIdentities 校验四道恒等式。
//
// 四道分开报而不是合成一句「对不上」：它们指向的成因完全不同，合并成一句会让
// 排查从「看哪一道断了」退化成「重新数一遍全部」。
func checkLoadIdentities(res *BackfillLoadResult) error {
	var bad []string
	if res.Total != res.Attempted+res.Unsupported {
		msg := fmt.Sprintf("一：Total(%d) ≠ Attempted(%d) + Unsupported(%d)",
			res.Total, res.Attempted, res.Unsupported)
		// 点名最常见的成因：标题解析不出期次的篇目既不在 Attempted 也不在 Unsupported，
		// 它们无处可去 —— 而「站点改了期次表述」正是最需要被人看见的一类变化。
		if n := len(res.Unclassified); n > 0 {
			msg += fmt.Sprintf("（有 %d 条标题解析不出期次，它们不在任何一格里，很可能就是差额）", n)
		}
		bad = append(bad, msg)
	}
	if res.Attempted != res.ParsedOK+res.ParseFailed {
		bad = append(bad, fmt.Sprintf("二：Attempted(%d) ≠ ParsedOK(%d) + ParseFailed(%d)",
			res.Attempted, res.ParsedOK, res.ParseFailed))
	}
	if res.Merged != res.SingleArticle+res.MergedGroups {
		bad = append(bad, fmt.Sprintf("三：Merged(%d) ≠ SingleArticle(%d) + MergedGroups(%d)",
			res.Merged, res.SingleArticle, res.MergedGroups))
	}
	if res.Merged != res.ToObservations+res.ToPending {
		bad = append(bad, fmt.Sprintf("四：Merged(%d) ≠ ToObservations(%d) + ToPending(%d)",
			res.Merged, res.ToObservations, res.ToPending))
	}
	if len(bad) == 0 {
		return nil
	}
	return errors.New("核对报告的恒等式不成立，本次回填的账对不上，报告不予输出：\n  " +
		strings.Join(bad, "\n  "))
}
