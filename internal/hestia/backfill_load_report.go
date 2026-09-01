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
// 🔴 **先打印、后校验恒等式，顺序不能反**（M1c-3b 的 TASK-006，C-2）。
//
// ⚠️ 此处原本写的是**相反**的话（「恒等式先校验、后打印：报告不能在数字对不上时照样
// 打印一份好看的表格」）。那句话在 C-2 之前完全正确，**被 C-2 推翻之后没人回来改它** ——
// dev 与 Leader 都没注意到，验证者是第三个看的人才发现。留着它的危害不是静默回归
// （TestWriteLoadReportPropagatesWriteError 的注释点名了 C-2、会拦住误改），
// 而是**误导 + 一次无效往返**：它带 🔴 强语气，会让下一个人真的去动手改回去。
//
// # 为什么必须先打印
//
// 恒等式一失败的**头号成因**是 Unclassified 非空（站点改了期次表述），而那批标题原文
// **只在报告里**。先校验就直接 return ⇒ stdout 0 字节，运维只拿到「有 N 条」这个数字，
// 得自己回去翻 manifest。⇒ 「不给看」并不能阻止错误发生，只是让排查的人少一份线索。
//
// 打印出来的那份表格**带着 error 一起交出**，不会被误当成验收通过 —— 原注释担心的
// 「自洽的表格让人停止追问」由此不成立：调用方拿到的是非 nil error，退出码非 0。
//
// ⇒ 与 collectSamples 遵守的是同一条原则（那里的原话：「先渲染再判错 —— 即使下面拒绝了，
// 看终端的人也该知道那 N 篇都去哪了」）。load 侧此前反过来了。
//
// 分节顺序固定（稳定才能逐次 diff）：顺序会飘的报告，每次跑出来的 diff 都是噪声，
// 而噪声会让人不再逐行看。由 TestLoadReportSectionsAreInFixedOrder 钉住。
func writeLoadReport(w io.Writer, dir string, res *BackfillLoadResult) error {
	if err := renderLoadReport(w, dir, res); err != nil {
		return err
	}
	return checkLoadIdentities(res)
}

// renderLoadReport 只渲染并写出，**不校验**。
//
// 🔴 拆出它是为了让「先渲染再报错」成为**结构性质**而不是调用顺序上的约定
// （M1c-3b 的 TASK-006，C-2）：恒等式一在 NewStore 之前就可能失败（C-3），
// 那条路径上没有 writeLoadReport 可用 —— 若不拆，为了满足 C-3 就会重新把 C-2 弄坏。
// 两个要求本身是冲突的（一个要求提前失败、一个要求失败也得有输出），
// **拆开之后才同时成立**。
func renderLoadReport(w io.Writer, dir string, res *BackfillLoadResult) error {
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
		k := groupKey(g)
		miss := res.MissingFields[k]
		// 🔴 打的是**缺了多少个字段**（W-1 的判据），族只作补充说明。
		// 缺的字段可能有三十几个，全列会把这一节淹掉 ⇒ 报条数 + 前几个名字。
		head := miss
		if len(head) > 6 {
			head = head[:6]
		}
		fmt.Fprintf(&b, "  %-18s 缺 %2d 个字段: %s", k, len(miss), strings.Join(head, ", "))
		if len(miss) > len(head) {
			b.WriteString(" …等")
		}
		if fams := missingFamilies(g.Parts); len(fams) > 0 {
			fmt.Fprintf(&b, "（缺族: %s）", strings.Join(fams, "、"))
		}
		b.WriteString("\n")
	}

	// W-7：sha256 未校验出声。写权威表的路径上，完整性风险比 calibrate 侧更重。
	if res.SHAUnverified > 0 {
		fmt.Fprintf(&b, "\n⚠️ %d 篇的 manifest 没有 sha256，未做完整性校验\n"+
			"   被截断的 HTML 仍可能 Parse 成功但少抽字段，那一期会带着残缺值进权威表，\n"+
			"   在库里与「央行本来就没发那几个字段」完全同形。\n", res.SHAUnverified)
	}

	// W-8：fetch 阶段就没抓到的篇目。它们不在 articles 里，故**不参与四道恒等式**，
	// 但必须出声 —— 不带出来的话，读者会以为「解析失败 N 篇」就是全部损失。
	fmt.Fprintf(&b, "\nfetch 阶段未抓到（%d，不计入上面四道恒等式）\n", len(res.FetchFailed))
	if len(res.FetchFailed) == 0 {
		b.WriteString("  （无）\n")
	}
	for _, f := range res.FetchFailed {
		fmt.Fprintf(&b, "  %s\n", f.ID)
	}

	// W-6：三族必填集互不相交这个**前提**破了要出声。它排在字段冲突之前，
	// 因为它会改变下面那一节的含义：前提破了之后，冲突不再是「归属表错了」的证据。
	if len(res.PartOverlaps) > 0 {
		fmt.Fprintf(&b, "\n⚠️ 前提已破：以下字段被同一组内一个以上 part 同时要求（%d）\n"+
			"   三族的必填集设计上不相交，这一节非空说明字段归属表变了；\n"+
			"   在此之后，「字段冲突」一节的含义也随之改变，不能再当成归属表出错的证据。\n",
			len(res.PartOverlaps))
		for _, f := range res.PartOverlaps {
			fmt.Fprintf(&b, "  %s\n", f)
		}
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

	// 🔴 **先把报告写出去，再校验恒等式**（M1c-3b 的 TASK-006，C-2）。
	//
	// 此前校验写在函数开头、不成立就直接 return ⇒ **一个字节都不写**。而恒等式一失败的
	// 头号成因正是 Unclassified 非空（站点改了期次表述），上面那段逐条打印标题原文的代码
	// 因此在**生产路径上不可达** —— 它自己的注释说那是「最需要被人看见的变化」。
	// 实测：改一篇标题后 exit=1、stdout **0 字节**，stderr 只给「有 1 条」这个数字，
	// 运维一个标题都看不到，只能自己回去翻 manifest。
	//
	// ⇒ 与 collectSamples 遵守的是同一条原则（那里的原话：「先渲染再判错 —— 即使下面
	// 拒绝了，看终端的人也该知道那 N 篇都去哪了」）。load 侧此前反过来了。
	//
	// ⚠️ 写失败时调用方**不再往下查恒等式**：那时 w 已经不可用，再返回一个恒等式错误
	// 会把真正的成因（写不出去）盖掉。
	_, err := io.WriteString(w, b.String())
	return err
}

// checkLoadIdentities 校验四道恒等式。
//
// 四道分开报而不是合成一句「对不上」：它们指向的成因完全不同，合并成一句会让
// 排查从「看哪一道断了」退化成「重新数一遍全部」。
// checkInputIdentities 只校验**输入侧**的恒等式一、二（M1c-3b 的 TASK-006，C-3）。
//
// 拆出来的理由是**时机**：这两道的四个加数在 loadParsedArticles 返回时就已全部确定，
// 而它们此前要等到末尾写报告时才校验 —— 那时库已经建出来、96 次 Save 已经写进去了。
// 提到 NewStore 之前调用，账对不上时**一个不可逆副作用都还没发生**。
//
// ⚠️ 它与 checkLoadIdentities 刻意**重叠**（后者仍查全部四道）：前者是提前失败，
// 后者背书报告里印的那句「四道恒等式: 全部成立 ✓」。少了后者，那句话就只是声称。
func checkInputIdentities(res *BackfillLoadResult) error {
	var bad []string
	bad = appendInputIdentityFailures(bad, res)
	if len(bad) == 0 {
		return nil
	}
	return errors.New("回填的输入侧恒等式不成立，账对不上；**尚未建库、尚未写入任何数据**：\n  " +
		strings.Join(bad, "\n  "))
}

// appendInputIdentityFailures 是恒等式一、二的**唯一定义处**，两个调用方共用。
func appendInputIdentityFailures(bad []string, res *BackfillLoadResult) []string {
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
	return bad
}

func checkLoadIdentities(res *BackfillLoadResult) error {
	bad := appendInputIdentityFailures(nil, res)
	// 🔴 恒等式三是**异源交叉校验**，不是自洽求和（M1c-3b 的 TASK-006，C-4）。
	//
	// 原判据 `Merged == SingleArticle + MergedGroups` **恒真**：Merged = len(groups)，
	// 而循环里每组必增且只增那两个计数器之一。实测把 `len(g.SourceIDs) > 1` 改成 `>= 1`，
	// 报告打印「单篇 0 + 合并组 96」这个假数，而这一行照样报「成立 ✓」。
	//
	// 现在拿**库里数出来的** merged@v1 行数比：一边来自内存里的分组结构，一边来自入库结果，
	// 两条路径独立 ⇒ 计数器写错时它会红。
	//
	// ⚠️ MergedRowsCounted 为 false 表示这一趟有组入库失败、异源计数没测
	//（那时 MergedGroups 必然大于库里的行数，比了就是假红）⇒ 跳过，而不是假装成立。
	if res.MergedRowsCounted && res.MergedGroups != res.DBMergedRows {
		bad = append(bad, fmt.Sprintf(
			"三：MergedGroups(%d) ≠ 库里 %s 的行数(%d，hestia_observations + hestia_pending 两表合计)"+
				" —— 分组计数与入库结果对不上",
			res.MergedGroups, extractorMerged, res.DBMergedRows))
	}
	if res.Merged != res.ToObservations+res.ToPending {
		bad = append(bad, fmt.Sprintf("四：Merged(%d) ≠ ToObservations(%d) + ToPending(%d)",
			res.Merged, res.ToObservations, res.ToPending))
	}
	if len(bad) == 0 {
		return nil
	}
	// ⚠️ 措辞随 C-2 一起改：报告**已经输出**了，此处只说账对不上。
	// 原文写的是「报告不予输出」——那句话在 C-2 之后就是假的，而错误串是运维唯一读到的东西。
	return errors.New("核对报告的恒等式不成立，本次回填的账对不上（报告已输出在上方，请对照查）：\n  " +
		strings.Join(bad, "\n  "))
}
