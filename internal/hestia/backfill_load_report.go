package hestia

import (
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

// —— 回填入库的核对报告（M1c-3b 的 TASK-006）——

// caliberMedianMinSamples 是族内量级核对每侧所需的最少样本数（M1c-4 的 TASK-011）。
//
// 取 3 而不是更大：门槛越高，「样本不足未判」越多，而未判的对**不受任何自动判据保护**
// —— 那正是本任务要消除的盲区。取 3 是「中位数还有意义」的下界。
const caliberMedianMinSamples = 3

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

	// —— 口径路由核对（M1c-4 的 TASK-011）——
	//
	// 🔴 **四个数都要印。** 本迭代唯一的新风险是「当月值进了 _ytd 列」，而它量级完全
	// 合理、下游没有别的闸门拦得住。这一节就是那个闸门，而**闸门自己也需要被看见**：
	//   · 违反 V —— 判据本身（预期 0）
	//   · 单侧跳过 S —— 整族被误判成同一口径时**每对都单侧**，若不印，报告会输出
	//     「共 0 对，违反 0」，**与「一切正常」逐字相同**
	//   · 无上期 P —— 恒等式要 ytd_{n-1}，取不到时**没查**，与「查过没问题」必须可区分
	//   · 可判 N —— 与上面两个加上两侧皆空构成划分，是这些数字的自证
	//
	// TASK-012 的消费判据是两条，缺一不可：V == 0、**N ≠ 0 且逐对清单里 0 的那几对
	// 已在 CONTRACTS 登记为已知开口**。只看 V == 0 是不够的——N 接近 0 时它恒真。
	pairs := caliberFamilies()
	viol, st := checkCaliberRouting(res.Groups)
	fmt.Fprintf(&b, "\n口径路由核对（判据 ytd_n == ytd_n-1 + mom_n，容差 ±%g 亿元；预期违反 0，共 %d 违反）\n"+
		"  可判 %d 对 / 单侧跳过 %d / 无上期 %d / 两侧皆空 %d（合计 %d = %d 对 × %d 观测）\n",
		caliberIdentityTolerance, len(viol), st.Comparable, st.SingleSided, st.NoPrior, st.Absent,
		st.Comparable+st.SingleSided+st.NoPrior+st.Absent,
		len(pairs), len(res.Groups))
	if len(viol) == 0 {
		b.WriteString("  （无违反）\n")
	}
	for _, v := range viol {
		// 差额同时给绝对值与相对值：路由错（两列写反）的偏差是 ~99%，而发布取整与
		// 央行数据修订造成的噪声是千分之几 —— 两者相差三个数量级，印出来才标得出容差。
		d := math.Abs(v.YTD - v.Expected)
		var rel float64
		if v.Expected != 0 {
			rel = d / math.Abs(v.Expected) * 100
		}
		// %.0f 而非 %g：值由 万亿元×10000 得来，浮点尾巴（241700.00000000003）会原样
		// 印进 TASK-012 的验收报告。单位是亿元，亚亿精度在本判据下没有意义。
		fmt.Fprintf(&b, "  %s/%s  %s=%.0f ≠ 上期 %.0f + %s=%.0f = %.0f（差 %.0f，%.3f%%）\n",
			v.Period, v.PeriodType, v.YTDField, v.YTD, v.PrevYTD, v.MoMField, v.MoM, v.Expected, d, rel)
	}

	// 逐对印被比较过的观测数。🔴 「异号跳过数 ≠ 总对数」只发现得了「取号写反」，
	// 发现不了「比较对数接近零」—— 而后者正是整族位移的表现。某一对恒为 0，
	// 只有逐对列出来才看得见。
	b.WriteString("  逐对可判观测数（0 表示这一对从未被比较过）:\n")
	for i, p := range pairs {
		fmt.Fprintf(&b, "    %-28s %d\n", strings.TrimSuffix(p[0], "_ytd"), st.ByPair[i])
	}

	// 跨语料族内不等式：不要求两列在同一观测里共存，是单侧盲区的唯一自动判据。
	famViol, famInsuf := checkCaliberFamilyMedians(res.Groups, caliberMedianMinSamples)
	fmt.Fprintf(&b, "\n族内量级核对（按 period_type，预期违反 0，共 %d 违反；样本不足未判 %d）\n",
		len(famViol), len(famInsuf))
	if len(famViol) == 0 {
		b.WriteString("  （无违反）\n")
	}
	for _, f := range famViol {
		fmt.Fprintf(&b, "  %s %s: median|ytd|=%g (n=%d) < median|mom|=%g (n=%d)  ← 整族可能写错列\n",
			f.PeriodType, strings.TrimSuffix(f.YTDField, "_ytd"),
			f.YTDMedian, f.YTDCount, f.MoMMedian, f.MoMCount)
	}
	for _, f := range famInsuf {
		fmt.Fprintf(&b, "  %s %s: 样本不足未判（ytd n=%d, mom n=%d，门槛 %d）\n",
			f.PeriodType, strings.TrimSuffix(f.YTDField, "_ytd"),
			f.YTDCount, f.MoMCount, caliberMedianMinSamples)
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

// —— 口径路由核对（M1c-4 的 TASK-011）——

// caliberRouteViolation 是一条「当月值可能被写进累计列」的疑点。
//
// ⚠️ **刻意用非导出名。** 需求文档写的是导出的 CaliberRouteViolation，但本类型的
// 消费者只有同包的 checkCaliberRouting 与报告渲染，没有任何外部调用方；Global
// Constraint 要求「包的导出面精确相等」，而 TestPackageExposesNoWriteFunctions
// **只遍历 *ast.FuncDecl、不覆盖导出的 type**（store_test.go）⇒ 导出它会**静默**
// 进入包的公开契约而不撞红任何东西。导出面越小越好，这里没有理由付那个代价。
type caliberRouteViolation struct {
	Period, PeriodType, YTDField, MoMField string
	YTD, MoM, PrevYTD, Expected            float64
}

// caliberRouteStats 是路由核对的四类计数，外加逐对的可判观测数。
//
// 🔴 四类**构成一个划分**：每个 (观测, 成对列) 组合恰好落进一类，故
// Comparable + SingleSided + NoPrior + Absent == len(caliberFamilies()) × len(obs)。
// 这条恒等式是「共 N 对」这个数字的**自证** —— 少了它，N 变小既可能是语料形态变了、
// 也可能是判定逻辑漏了一整类，而两者在报告上无法区分。
// 由 TestCaliberRoutingCountsArePartition 钉住。
type caliberRouteStats struct {
	Comparable  int // 三个值齐备，恒等式真的算过
	SingleSided int // 本期只有一侧在场（真语料常态，不是异常）
	NoPrior     int // 本期双侧在场，但取不到上一期的 ytd
	Absent      int // 两侧都不在场

	// ByPair 与 caliberFamilies() **同序**，记每一对被判过的观测数。
	// 🔴 它存在的理由：整族位移的表现是「某一对从未被判过」，而总数上看不出来 ——
	// 逐对印出来，恒为 0 的那一对才看得见。
	ByPair []int
}

// caliberIdentityTolerance 是累计恒等式的容差，单位亿元（M1c-4 的 TASK-011）。
//
// 取绝对值 ±1 而不是相对容差：真语料的值是**取整到亿元**的整数，误差来源是取整
// 而非测量。实测 2020-02 未贴现票据 1403+(-3961)=-2558 与报告值**逐位相等**，
// 信托 432+(-540)=-108 vs -109 差 1 —— 正是取整。相对容差会让大数上的真错漏网。
const caliberIdentityTolerance = 1.0

// prevPeriodKey 返回同年上一个月的 period（"2020-02" → "2020-01"）。
// 1 月或格式不可解析时返回 false —— 跨年不接（上一年 12 月的 ytd 是**上一年**的累计，
// 接上去会得到一个毫无意义的期望值）。
func prevPeriodKey(period string) (string, bool) {
	var y, m int
	if _, err := fmt.Sscanf(period, "%d-%d", &y, &m); err != nil || m <= 1 || m > 12 {
		return "", false
	}
	return fmt.Sprintf("%04d-%02d", y, m-1), true
}

// checkCaliberRouting 找「当月值被写进累计列」的疑点（M1c-4 的 TASK-011）。
//
// 判据是**精确恒等式** `ytd_n == ytd_{n-1} + mom_n`（容差 ±1 亿元，取整误差）。
// 1 月退化成 `ytd_1 == mom_1` —— 年初至今只包含 1 月这一个月。
//
// 🔴 这是本迭代头号风险的可执行形式。TASK-005 之前，解析器宁可拒绝整篇也不猜口径，
// 理由是「两者都在合法量级内、下游没有任何闸门拦得住」；把拒绝改成路由之后，
// 这条断言就是那个「拦得住的闸门」。
//
// # ⚠️ 为什么**不用** |ytd| >= |mom|（DoD 原判据，实测证否）
//
// 「累计值不可能小于它自己的某一个月」这条前提**只在年内各月同号时成立**。真语料
// 2020-02 实测两条假阳，两条都是合法数据：
//
//	信托:       432 + (-540)  = -108  vs 报告值 -109（取整差 1）
//	未贴现票据: 1403 + (-3961) = -2558 vs 报告值 -2558（**逐位相等**）
//
// 社融分项年内正负交替是常态 ⇒ 累计穿越零点后 |累计| 合法地小于 |某月|。
// **异号跳过挡不住它** —— 这两例 ytd 与 mom **同号**（都是负），是被跳过条件放行
// 之后才判的。⇒ 那条判据在这类字段上不可用，而恒等式对它们逐位成立。
//
// ⚠️ 随之**取消了异号跳过**：恒等式与符号无关，异号的对现在能被精确验证。保留跳过
// 只会让本可以判的对静默漏判 —— 那正是本任务要消除的失明。（真语料实测异号 = 0，
// 故这一改动不影响当前数字，但它决定将来。）
//
// 🔴 **四类都要计数，单侧尤其**：路由错误最典型的产物恰恰是单侧（整族被误判成同一
// 口径 ⇒ 整族只写进一侧）。若单侧静默跳过，报告会输出「共 0 对，违反 0」，
// **与「一切正常」逐字相同** —— 这条防线就在最需要它的地方失明。
func checkCaliberRouting(obs []MergedObservation) ([]caliberRouteViolation, caliberRouteStats) {
	fams := caliberFamilies()
	st := caliberRouteStats{ByPair: make([]int, len(fams))}
	var out []caliberRouteViolation

	// 批内索引：(period_type, period) → 该观测的值。恒等式要取**同族同档**的上一期，
	// 故键必须含 period_type —— 2020-03/q1 与 2020-03/monthly 是两条不同的序列。
	//
	// ⚠️ 刻意**不查库**（不碰 TASK-008 的 PrecedingAll）：那是 validate 阶段回答
	// 「近期发生过什么」的，定义域不同；在报告里调它会让 load 报告依赖 Store。
	index := make(map[string]map[string]float64, len(obs))
	for _, m := range obs {
		pt := m.Obs.Meta.PeriodType
		if index[pt] == nil {
			index[pt] = map[string]float64{}
		}
		for f, v := range m.Obs.Values {
			index[pt][m.Obs.Meta.Period+"|"+f] = v
		}
	}

	for _, m := range obs {
		pt := m.Obs.Meta.PeriodType
		for i, p := range fams {
			ytd, okY := m.Obs.Values[p[0]]
			mom, okM := m.Obs.Values[p[1]]
			switch {
			case !okY && !okM:
				st.Absent++
				continue
			case !okY || !okM:
				st.SingleSided++
				continue
			}

			// 1 月退化：年初至今 == 本月，不需要上一期
			prev, expected := 0.0, mom
			if pk, ok := prevPeriodKey(m.Obs.Meta.Period); ok {
				pv, found := index[pt][pk+"|"+p[0]]
				if !found {
					st.NoPrior++
					continue
				}
				prev, expected = pv, pv+mom
			}

			st.Comparable++
			st.ByPair[i]++
			if math.Abs(ytd-expected) > caliberIdentityTolerance {
				out = append(out, caliberRouteViolation{
					Period: m.Obs.Meta.Period, PeriodType: pt,
					YTDField: p[0], MoMField: p[1],
					YTD: ytd, MoM: mom, PrevYTD: prev, Expected: expected,
				})
			}
		}
	}
	return out, st
}

// caliberFamilyShift 是一条「整族可能被写错列」的疑点（M1c-4 的 TASK-011）。
//
// 🔴 它与 caliberRouteViolation 的分工是本任务的核心：后者要求两列**在同一观测里
// 共存**，而路由错误最典型的产物是**单侧**（整族被误判成同一口径 ⇒ 整族只写进一侧），
// 那时逐观测判据一条都判不了。本判据**跨观测**取中位数，不要求共存 —— 这是
// 它存在的全部理由，也是唯一能抓住整族位移的自动判据。
type caliberFamilyShift struct {
	PeriodType, YTDField, MoMField string
	YTDMedian, MoMMedian           float64
	YTDCount, MoMCount             int
}

// checkCaliberFamilyMedians 按 period_type 分档比较每一对成对列的绝对值中位数，
// 返回「违反」与「样本不足未判」两组。
//
// 判据：median(|x_ytd|) >= median(|x_mom|)。累计口径覆盖的月份数不少于当月口径，
// 故整体量级不应更小；显著更小说明整族写错了列。
//
// ⚠️ **相等不算违反**（判据是 >= 不是 >）。理由与逐观测判据同源：1 月的累计**就等于**
// 当月，某个分档若恰好全由 1 月构成，两侧中位数会相等 —— 那是合法形态，判成违反
// 会制造假阳。而整族位移的表现是**显著小于**，用 >= 检出力几乎不损失。
// ⚠️ DoD 原文写的是 `median(|x_ytd|) > median(|x_mom|)`（严格大于），已订正进
// done_criteria，理由同上。
//
// ⚠️ **样本不足的对必须报出来，不得静默跳过** —— 静默跳过正是本任务要修的那个毛病
// （「没查」与「查过没问题」在报告上不可区分）。
func checkCaliberFamilyMedians(obs []MergedObservation, minSamples int) (violations, insufficient []caliberFamilyShift) {
	// 分档收集：periodType → 对序号 → 两侧的绝对值样本
	type bucket struct{ ytd, mom []float64 }
	fams := caliberFamilies()
	byType := map[string][]bucket{}
	var types []string

	for _, m := range obs {
		pt := m.Obs.Meta.PeriodType
		if byType[pt] == nil {
			byType[pt] = make([]bucket, len(fams))
			types = append(types, pt)
		}
		for i, p := range fams {
			if v, ok := m.Obs.Values[p[0]]; ok {
				byType[pt][i].ytd = append(byType[pt][i].ytd, math.Abs(v))
			}
			if v, ok := m.Obs.Values[p[1]]; ok {
				byType[pt][i].mom = append(byType[pt][i].mom, math.Abs(v))
			}
		}
	}

	// 排序输出：map 迭代顺序随机，两次跑报出不同顺序会让逐次 diff 失效
	// —— 与本文件里 PendingReasons 的排序同一个理由。
	sort.Strings(types)
	for _, pt := range types {
		for i, b := range byType[pt] {
			s := caliberFamilyShift{
				PeriodType: pt, YTDField: fams[i][0], MoMField: fams[i][1],
				YTDCount: len(b.ytd), MoMCount: len(b.mom),
			}
			if len(b.ytd) < minSamples || len(b.mom) < minSamples {
				// 两侧都是 0 样本的对不报——那是「这一档根本没有这一族」，
				// 逐档报 22 条全空会把这一节淹掉，真正的信号反而看不见。
				if len(b.ytd) > 0 || len(b.mom) > 0 {
					insufficient = append(insufficient, s)
				}
				continue
			}
			s.YTDMedian, s.MoMMedian = medianOf(b.ytd), medianOf(b.mom)
			if s.YTDMedian < s.MoMMedian {
				violations = append(violations, s)
			}
		}
	}
	return violations, insufficient
}

// medianOf 返回中位数；就地排序副本，不动调用方的切片。
func medianOf(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}
