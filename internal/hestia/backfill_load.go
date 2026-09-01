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
)

// —— 为什么必须合并（M1c-3b 的 TASK-003）——
//
// 央行在 2025-10 之前把同一期的数据**分三篇发**：《金融统计数据报告》《社会融资规模
// 存量统计数据报告》《社会融资规模增量统计数据报告》，三篇的 period / period_type /
// published_at **完全相同**，而字段互补（27 / 18 / 9）。
//
// 实测（M1c-3b 设计阶段，语料 data/hestia-backfill-2026-08-14）：
//     161 篇解析成功 → 双时态主键唯一值 96 → 42 组冲突、涉及 107 篇
//
// 🔴 不合并直接逐篇 Save 的后果是具体的：Save 按 published_at 判重 ⇒
// 107 − 42 = 65 篇判 Duplicate ⇒ 走 refreshArticleID ⇒ **只刷 article_id、
// 新抽出来的 Values 一个都不写**，且返回 nil、退出码 0。运维看到「N 期处理完毕、
// 零错误」，而库里缺掉几乎全部社融字段。
//
// required.go 的 requiredFields 里 M1c-3a 已写下这条伏笔（搜「合并成一个完整观测
// 是 M1c-3b 的事」）；本文件是它的兑现。

// MergeConflict 是同一业务键上两篇对同一字段给出不同值的记录。
type MergeConflict struct {
	Period, PeriodType, Field string
	A, B                      float64
	FromA, FromB              string
}

// MergedObservation 是一个业务键上合并后的观测，外加合并的取证信息。
//
// Parts / SourceIDs / DroppedIDs 三样都是**取证**，不是装饰：
// 必填集要靠 Parts 算（见 mergedRequiredFields），而 DroppedIDs 是被丢弃的定位符 ——
// 丢弃可以，不出声不行。
type MergedObservation struct {
	// PartOverlaps 是被组内一个以上 part 同时要求的字段（M1c-3b 的 TASK-006，W-6）。
	// 恒应为空 —— 非空表示「三族必填集互不相交」这个前提破了。
	PartOverlaps []string

	Obs        Observation
	Parts      []string
	SourceIDs  []string
	DroppedIDs []string
	Conflicts  []MergeConflict
}

// mergeByBusinessKey 把多篇按 (period, period_type, published_at) 归组合并。
//
// 输出按 period 升序、同期按 period_type 升序 —— 稳定才能逐次 diff，
// 与 classifyArticles「返回的 items 按期次升序」同一个理由。
func mergeByBusinessKey(ps []parsedArticle) []MergedObservation {
	type key struct{ period, periodType, publishedAt string }
	order := make([]key, 0, len(ps))
	groups := make(map[key][]parsedArticle, len(ps))
	for _, p := range ps {
		k := key{p.obs.Meta.Period, p.obs.Meta.PeriodType, p.obs.Meta.PublishedAt}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p)
	}
	slices.SortStableFunc(order, func(a, b key) int {
		if c := strings.Compare(a.period, b.period); c != 0 {
			return c
		}
		return strings.Compare(a.periodType, b.periodType)
	})

	out := make([]MergedObservation, 0, len(order))
	for _, k := range order {
		out = append(out, mergeGroup(groups[k]))
	}
	return out
}

// mergeGroup 合并一个业务键上的若干篇。
func mergeGroup(g []parsedArticle) MergedObservation {
	// 组内按 article_id 升序：Parts / SourceIDs 的顺序、以及冲突记录里
	// 谁是 A 谁是 B，都不该随 manifest 的排列而变。
	slices.SortStableFunc(g, func(a, b parsedArticle) int {
		return strings.Compare(a.obs.Meta.ArticleID, b.obs.Meta.ArticleID)
	})

	m := MergedObservation{Obs: g[0].obs}
	// 单篇：原样返回，**不改写 extractor**。改写会让 96 个观测里那 54 个单篇
	// 也走 mergedRequiredFields 那条路，而它们的必填集本来就由自己的 extractor
	// 说得清 —— 多绕一圈只多一个出错的机会。
	if len(g) == 1 {
		m.Parts = []string{g[0].obs.Meta.Extractor}
		m.SourceIDs = []string{g[0].obs.Meta.ArticleID}
		m.Obs.Parts = m.Parts // 见下面 mergeGroup 尾部那段注释：单篇这条也必须赋
		return m
	}

	vals := make(map[string]float64, len(fieldOrder))
	from := make(map[string]string, len(fieldOrder))
	for _, p := range g {
		m.Parts = append(m.Parts, p.obs.Meta.Extractor)
		m.SourceIDs = append(m.SourceIDs, p.obs.Meta.ArticleID)
		// 遍历 fieldOrder 而不是 p.obs.Values：map 迭代顺序随机，
		// 同一份数据两次跑会报出不同的冲突顺序，让排查变成猜谜。
		for _, f := range fieldOrder {
			v, ok := p.obs.Values[f]
			if !ok {
				continue
			}
			prev, seen := vals[f]
			switch {
			case !seen:
				vals[f], from[f] = v, p.obs.Meta.ArticleID
			case prev != v:
				// 🔴 不做静默取值。三个 extractor 的字段集设计上不相交
				// （27 / 18 / 9 = 54），冲突理应恒为 0；一旦出现就说明字段
				// 归属表错了，那是必须响亮失败的事，而「取第一个」会让一张
				// 错的归属表永远不被发现。
				m.Conflicts = append(m.Conflicts, MergeConflict{
					Period: g[0].obs.Meta.Period, PeriodType: g[0].obs.Meta.PeriodType,
					Field: f, A: prev, B: v, FromA: from[f], FromB: p.obs.Meta.ArticleID,
				})
			}
		}
	}

	// W-6：**合并组不限 extractor 是既有前提，而这个前提由语料守着、不由代码守着**
	// （M1c-3b 的 TASK-006）。三族的必填集设计上不相交（27/18/9 = 54），冲突才恒为 0。
	// 本条不改合并键（那是人类裁决过的），只让前提破掉时**出声** ——
	// 否则新增一个 extractor 让两族必填集相交时，症状会表现成「字段冲突突然出现」，
	// 而排查的人会去查语料，不会想到是归属表变了。
	m.PartOverlaps = overlappingRequiredFields(m.Parts)

	m.Obs.Values = vals
	m.Obs.Meta.Extractor = extractorMerged
	// 🔴 **必须把 Parts 传进 Obs**（M1c-3b 的 TASK-006 补的接缝）。
	//
	// Parts 记在**包装结构**上，而 gateCompleteness 只拿得到 Observation（它的入参是
	// gateInput{obs, prior, cfg}，看不见包装）。不传的后果实测过：
	//
	//	MergedObservation.Parts     = [tsf-stock@v1 tsf-flow@v1]
	//	MergedObservation.Obs.Parts = []          <<< 闸门读的是这个
	//	⇒ completeness = skipped  reason="unknown_extractor:merged@v1"
	//
	// 即缺口 A-1 原样保留：42 个合并观测的 completeness 谁都不查，带着「零告警」
	// 进权威表。⚠️ 这个缺陷**两侧的测试都抓不到** —— M1c-3b 的 TASK-003 断的是包装上的 Parts，
	// M1c-3b 的 TASK-011 直接构造 Observation{Parts: …} 喂闸门，两边都对、都没跨过接缝。
	// 守它的是 TestMergedObservationCompletenessIsEvaluatedEndToEnd（同时经过两侧）。
	m.Obs.Parts = m.Parts
	m.Obs.Meta.ArticleID = pickArticleID(g)
	for _, id := range m.SourceIDs {
		if id != m.Obs.Meta.ArticleID {
			m.DroppedIDs = append(m.DroppedIDs, id)
		}
	}
	return m
}

// pickArticleID 选合并组的代表 article_id：优先月报那篇，否则字典序最小。
//
// 月报是这一期的主报告，运维按它拼回 URL 的概率最高。g 进来时已按 article_id
// 升序，故「没有月报」时取 g[0] 即字典序最小。
func pickArticleID(g []parsedArticle) string {
	for _, p := range g {
		if isFinanceFamily(p.obs.Meta.Extractor) {
			return p.obs.Meta.ArticleID
		}
	}
	return g[0].obs.Meta.ArticleID
}

// isFinanceFamily 是「月报族（含季报）」这张清单的**唯一定义处**（M1c-3b 的 TASK-006，W-9）。
//
// 🔴 此前同一份四常量清单在本文件里手抄了两份（pickArticleID 与 extractorFamilies 各一份）
// 且没有任何测试比对两者 —— 加第五个月报类 extractor 时只改一处会让两处**静默分叉**：
// 代表 article_id 还认得它、而族归属不认得（或反过来），两种错都不会红。
// ⇒ 合并成一处后，分叉在结构上不可能，而不是靠人记得改两处。
//
// ⚠️ **不要与 extract.go 的 isMonthlyFamily 混淆，两者含义不同**（本函数原本就叫那个名字，
// 撞名后改成现名，把区别记在这里）：
//
//	extract.go 的 isMonthlyFamily  —— 只含 extractorMonthlyV1/V2 两个，问的是「版式轴上是不是月报」
//	本函数 isFinanceFamily         —— 含上面两个 + extractorV1/V2 共四个，问的是
//	                                  「三族划分里属不属于金融统计报告这一族」（对立面是社融存量/增量）
//
// 两者在**当前**取值域上是真包含关系，但它们回答的是不同的问题，会各自演化。
func isFinanceFamily(e string) bool {
	switch e {
	case extractorV1, extractorV2, extractorMonthlyV1, extractorMonthlyV2:
		return true
	}
	return false
}

// —— BackfillLoad 编排（M1c-3b 的 TASK-006）——

// BackfillLoadDeps 是一次批量入库所需的全部输入。
type BackfillLoadDeps struct {
	Dir    string // M1c-1 的产物目录
	DBPath string // 必须不存在
	Cfg    Thresholds

	// Out 是核对报告的去处。
	//
	// 🔴 **为 nil 时报错，不退化成 io.Discard。**（⚠️ 刻意背离需求文档 Task 6 的
	// 「nil 等价于 io.Discard」，沿用同包 Calibrate 的相反契约，见 calibrate.go 的
	// Calibrate 头注释。）本函数的产出**就是那份报告** —— 它还是 DroppedIDs 的唯一
	// 载体，被丢弃的 article_id 除了这里没有第二个地方能看到。默认丢弃输出会把
	// 调用方的疏漏变成合法配置：cmd 层装配时漏填 Out ⇒ 命令静默打印零字节、退出码 0，
	// 而「子命令注册了吗」「flag 解析对吗」这类测试**全部通过**。
	Out io.Writer

	AllowIncomplete bool
}

// BackfillLoadResult 是核对报告的原料。
type BackfillLoadResult struct {
	Total, Attempted, Unsupported int
	ParsedOK, ParseFailed         int
	Merged                        int
	SingleArticle, MergedGroups   int
	ToObservations, ToPending     int

	// DBMergedRows 是**入库之后**从库里数出来的 merged@v1 行数（跨 hestia_observations
	// 与 hestia_pending 两张表），MergedRowsCounted 表示它是否真被测过。
	//
	// 🔴 它存在的唯一理由是给恒等式三一个**异源** oracle（M1c-3b 的 TASK-006，C-4）。
	// 原先的恒等式三是 `Merged == SingleArticle + MergedGroups`，而 Merged = len(groups)、
	// 循环里每组必增且只增那两个计数器之一 ⇒ **它恒真，检查不出任何东西**。
	// 实测：把 `len(g.SourceIDs) > 1` 改成 `>= 1`，全套测试 rc=0、真语料 exit=0，
	// 报告打印「单篇 0 + 合并组 96」这个假数，而「四道恒等式全部成立 ✓」照常打印。
	//
	// ⚠️ 同一个文件早就为恒等式二否掉过这个形状（「一个由两个加数派生出来的和，
	// 再拿去和这两个加数比，检查不出任何东西」）—— **那段推理逐字适用于三号，
	// 只是当时没有被应用过去。**
	//
	// ⚠️ 依据 merged@v1 ⟺ SourceIDs > 1（M1c-3b 的 TASK-003 的契约：单篇不改写 extractor）。
	// ⚠️ **必须跨两张表数**：落 pending 的组同样带 merged@v1，只查权威表会得 28 而非 42。
	DBMergedRows      int
	MergedRowsCounted bool

	Conflicts      []MergeConflict
	Groups         []MergedObservation
	PendingReasons map[string]string // period/period_type → 判因

	// MissingFields 是 PartialCoverage 里每条观测缺掉的字段（period/period_type → 字段名）。
	// 与 PartialCoverage 同源同时机写入，报告直接读它，不在渲染时重算
	// —— 重算等于把同一个判据写两遍，两份会各自演化（M1c-3b 的 TASK-006，W-1）。
	MissingFields map[string][]string

	// PartOverlaps 汇总各组「被一个以上 part 同时要求」的字段（M1c-3b 的 TASK-006，W-6）。
	// 恒应为空：三族必填集设计上不相交，非空即前提已破。
	PartOverlaps       []string
	IncompleteAccepted bool

	// PartialCoverage 是「入了权威表、但不是由全部三族报告合成」的期次
	// （M1c-3b 的 TASK-006，人类 2026-09-01 在 dod-gate 裁决，缺口 A-3）。
	//
	// 🔴 为什么必须单列：需求文档称三篇的 period/period_type/published_at 完全相同，
	// 实测**为假** —— 17 个发布事件的三篇被 period_type 拆开（《社融存量报告》标题恒为
	// 「N月…」⇒ monthly，而同日发的另两篇在季末写「一季度/上半年/前三季度」）。
	// 实测 96 个观测里只有 33 个字段完整，42 个只带 18 个社融存量字段、其余 36 列全 NULL，
	// **而它们的 completeness 会通过**（tsf-stock@v1 的必填集就是那 18 个）。
	//
	// ⚠️ 理由与 IncompleteAccepted 同类：这批期次在库里与「央行本来就没发」**完全同形**。
	// 缺期与缺字段都是静默的洞，工具不替人合并（人类裁决：合并键维持不变），但必须出声。
	PartialCoverage []MergedObservation

	// Unclassified 是标题解析不出期次的篇目，原文照录。
	//
	// 它既不在 Attempted 也不在 Unsupported ⇒ 非空时**恒等式一必然不成立**，
	// 而那正是想要的：这批篇目无处可去，四道恒等式的用途就是把这种「账对不上」
	// 变成响亮失败。writeLoadReport 的错误串会点名它是成因。
	Unclassified []string

	// SHAUnverified 是 manifest 里没有 sha256、因而未做完整性校验的篇数（W-7）。
	// 恒应为 0；非 0 时那几篇的截断风险对本次回填是**不可见**的。
	SHAUnverified int

	// FetchFailed 是 manifest.failed：fetch 阶段就没抓到的篇目（W-8）。
	// ⚠️ 不参与四道恒等式——它们不在 articles 里，混进去会造出假失败。
	FetchFailed []Failed
}

// BackfillLoad 把 M1c-1 的产物目录批量灌进一个**新建**的库，并把核对报告写给 d.Out。
//
// 顺序：eachParsedArticle → mergeByBusinessKey → Validate → Save。
//
// 🔴 **按期次升序处理**（mergeByBusinessKey 已保证，本函数不再打乱）：顺着 manifest 的
// 顺序跑会让 stock_continuity / deposit_sum 的漂移检测**一次都不真正执行** —— 每期都成了
// 「首期」⇒ 恒 no_prior_period，而数据照进权威表、报告照样 Passed=true、**零告警**。
func BackfillLoad(ctx context.Context, d BackfillLoadDeps) (*BackfillLoadResult, error) {
	if d.Out == nil {
		return nil, errors.New("hestia: BackfillLoad 需要 Out：本函数的产出就是那份核对报告，" +
			"没有 Out 就没有产出（它还是 DroppedIDs 的唯一载体）")
	}

	// 🔴 **存在性检查必须先于 NewStore**：NewStore 会 MkdirAll + sql.Open **把文件建出来**
	// （store.go 的 NewStore）。写在它后面的后果不是「少一道防线」而是功能直接坏掉——
	// 第一次跑自己造出库、第二次被自己拒，且第一次那份库是半成品。
	// ⚠️ TestBackfillLoadRefusesExistingDB 用**已存在**的文件测，抓不到顺序错；
	// 守顺序的是 TestBackfillLoadDoesNotCreateDBBeforeChecking（用不存在的路径正向跑）。
	switch _, err := os.Stat(d.DBPath); {
	case err == nil:
		return nil, fmt.Errorf("%s 已存在：回填是一次性动作，追加写会让四道恒等式失去意义，"+
			"且掩盖「上一趟跑到哪」。重跑请先删掉该文件再来", d.DBPath)
	case !os.IsNotExist(err):
		return nil, fmt.Errorf("查看 %s: %w", d.DBPath, err)
	}

	res, ps, err := loadParsedArticles(d)
	if err != nil {
		return nil, err
	}

	// 🔴 **输入侧的账必须在 NewStore 之前对**（M1c-3b 的 TASK-006，C-3）。
	//
	// 恒等式一、二的四个加数在 loadParsedArticles 返回时就已全部确定，**再往后不会变**。
	// 而此前它们要等到末尾 writeLoadReport 才校验 —— 那时 NewStore 已经把库建出来、
	// 96 次 Save 已经写进去了。实测后果：失败后留下一个 90112 字节、observations=75、
	// pending=20 的半成品库，原样重跑被本函数自己的「--db 必须不存在」拒掉，
	// **而错误串里没有一个字提到它刚建了库**。
	//
	// ⇒ 判据是**「加数何时确定」而不是「校验写在哪」**：能提前的检查一律提到不可逆副作用之前。
	if idErr := checkInputIdentities(res); idErr != nil {
		// 🔴 **先把报告渲染出去再返回错误**（C-2）：恒等式一失败的头号成因是 Unclassified
		// 非空，而那批标题原文是运维唯一的线索 —— 账对不上不是「不给看」的理由。
		// 此刻尚未 NewStore ⇒ C-3 要的「不产生不可逆副作用」同时成立。
		if wErr := renderLoadReport(d.Out, d.Dir, res); wErr != nil {
			return res, errors.Join(idErr, wErr)
		}
		// ⚠️ 交出 res 而不是 nil：调用方要拿它看差在哪，只给一句错误串不够
		//（既有契约，由 TestBackfillLoadFailsLoudlyOnUnclassified 钉住）。
		return res, idErr
	}

	store, err := NewStore(d.DBPath)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	groups := mergeByBusinessKey(ps)
	res.Merged = len(groups)
	res.Groups = groups
	res.PendingReasons = map[string]string{}
	res.MissingFields = map[string][]string{}

	var errs []error
	for _, g := range groups {
		// 单篇不会被改写成 merged@v1（M1c-3b 的 TASK-003 的契约），故判据是 SourceIDs 的条数。
		if len(g.SourceIDs) > 1 {
			res.MergedGroups++
		} else {
			res.SingleArticle++
		}
		res.Conflicts = append(res.Conflicts, g.Conflicts...)
		res.PartOverlaps = append(res.PartOverlaps, g.PartOverlaps...)

		key := groupKey(g) // W-10：与 MissingFields 用同一个键，且是完整业务键
		rep, err := Validate(ctx, g.Obs, store, d.Cfg)
		if err != nil {
			// 单期失败不中断整批：逐期收集，末尾 errors.Join 一并返回。
			errs = append(errs, fmt.Errorf("%s 校验: %w", key, err))
			continue
		}
		out, err := store.Save(ctx, g.Obs, rep)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s 入库: %w", key, err))
			continue
		}
		if out.Table == TablePending {
			res.ToPending++
			res.PendingReasons[key] = pendingReason(rep)
			continue
		}
		res.ToObservations++
		// 🔴 **判据是「缺哪些字段」，不是「缺哪一族」**（M1c-3b 的 TASK-006，W-1）。
		//
		// 这一节的理由自始至终是**字段**上的洞（「42 个只带 18 个社融存量字段、
		// 其余 36 列全 NULL」），而判据此前问的是「Parts 里三族齐不齐」。两者在
		// 2025-10 之前**恰好**高度相关，之后完全脱钩：央行并篇后一篇就带满 52/54 个字段，
		// 而 Parts 长度恒为 1 ⇒ 按族判会把**字段全满**的观测列成部分覆盖。
		// 实测已产出 3 个 54/54 字段全非空却被列进来的假阳；对 2025-10 后的数据是 100% 假阳。
		//
		// ⇒ 改问「这条观测缺了哪些字段」。缺字段才是这一节要人看见的东西，
		// 「由几族合成」只是它在旧语料上的一个代理变量。
		if missing := missingFields(g); len(missing) > 0 {
			res.PartialCoverage = append(res.PartialCoverage, g)
			res.MissingFields[groupKey(g)] = missing
		}
	}

	// 🔴 **字段冲突必须让整趟失败**（M1c-3b 的 TASK-006，W-4）。
	//
	// mergeGroup 里那段注释早就写着「不做静默取值 …… 一旦出现就说明字段归属表错了，
	// 那是必须响亮失败的事」——**而实现只是把它记进 Conflicts，然后退出码 0**。
	// 实测后果：冲突非空、四道恒等式全成立、报告照常打印、`echo $?` 得 0。
	// 注释承诺的与代码做的不是一回事，而读注释的人会以为已经防住了。
	//
	// ⚠️ 排在写报告**之前** append 进 errs，但不 return —— 报告仍要出（C-2 同一条原则）：
	// 冲突明细只在报告里，直接返回等于让人看不见冲突在哪。
	if err := conflictError(res); err != nil {
		errs = append(errs, err)
	}

	// 异源 oracle 只在**这一趟全部入库成功**时才测（M1c-3b 的 TASK-006，C-4）。
	//
	// 🔴 有组 Validate/Save 失败时它必然对不上：那些组根本没进库，而 MergedGroups 已经
	// 把它们算进去了。C-2/C-3 的修法恰恰是让「有错误」的路径也能出报告 ——
	// 那条路径由此从「不可达」变成「常走」，不设这个前置条件就会在那里造出一片假红。
	if len(errs) == 0 {
		n, err := countMergedRows(ctx, store)
		if err != nil {
			errs = append(errs, fmt.Errorf("统计 merged@v1 行数（恒等式三的异源 oracle）: %w", err))
		} else {
			res.DBMergedRows, res.MergedRowsCounted = n, true
		}
	}

	if err := writeLoadReport(d.Out, d.Dir, res); err != nil {
		errs = append(errs, err)
	}
	return res, errors.Join(errs...)
}

// loadParsedArticles 走与 collectSamples 相同的前置（manifest 在不在 → 载入 →
// completed_at → 分类 → 逐篇解析），返回按期次升序的 parsedArticle。
//
// 与 collectSamples 并列而不是复用它：那边把结果塞进 Samples/Records 供标定用，
// 这边要的是 parsedArticle 本身。两边共用的**解析**部分已经收口在 eachParsedArticle 里。
func loadParsedArticles(d BackfillLoadDeps) (*BackfillLoadResult, []parsedArticle, error) {
	// loadManifest 对文件不存在返回空 Manifest + nil error（回填首跑的正常路径），
	// 在这里沿用那条语义会让 load 拿一份零篇的 manifest 一路走下去。故先自己确认它在。
	p := filepath.Join(d.Dir, manifestFileName)
	switch _, err := os.Stat(p); {
	case os.IsNotExist(err):
		return nil, nil, fmt.Errorf("%s 不存在：--dir 要指向 backfill 的产物目录（内含 %s 与 %s/），不是 manifest 文件本身",
			p, manifestFileName, articlesDirName)
	case err != nil:
		return nil, nil, fmt.Errorf("查看 %s: %w", p, err)
	}

	st, err := loadManifest(d.Dir)
	if err != nil {
		return nil, nil, err
	}
	m := st.Manifest

	// 夭折的 manifest 与正常完成的**结构上无法区分**，下游一切闭合性检查在夭折产物上
	// 同样全绿 ⇒ 判据只能是那个字段在不在。文案沿用 collectSamples 那句，只把后果换成
	// load 的：这里的代价不是「区间偏窄」，是**历史序列直接缺期**。
	if m.CompletedAt == "" && !d.AllowIncomplete {
		return nil, nil, errors.New("manifest 里没有 completed_at：这份产物可能是中途夭折的，" +
			"而夭折与正常完成在结构上无法区分；用半份数据回填会让历史序列缺期，" +
			"而缺的那些期在库里与「央行本来就没发」完全同形。" +
			"若你有产物之外的证据能证明这趟跑完了，传 --allow-incomplete")
	}

	cal := &CalibrateResult{Samples: map[string][]float64{}}
	items := classifyArticles(cal, m.Articles)

	ps := make([]parsedArticle, 0, len(items))
	// 🔴 **接住 sha256 未校验的篇数**（M1c-3b 的 TASK-006，W-7）。
	//
	// 此前这个返回值被丢弃（`eachParsedArticle(...)` 不取返回值）。calibrate 侧拿它汇总
	// 成一条 warning，而 load 侧**写的是权威表** —— 完整性风险在这边更重却更看不见：
	// 被截断的 HTML 仍可能 Parse 成功、只是少抽几个字段，那一期就带着残缺值进库，
	// 与「央行本来就没发那几个字段」完全同形。
	shaUnverified := eachParsedArticle(d.Dir, cal, items, func(pa parsedArticle) {
		// 要点 (4)：item.a.ID 是 manifest 的 id，而 Parse 看不到 URL（接缝①）——
		// 不显式赋值的话合并组的代表 id 恒为空，报告里那一列全是空白而无人报警。
		pa.obs.Meta.ArticleID = pa.item.a.ID

		// 要点 (5)：交叉校验 manifest 的 published 与正文自解的 PublishedAt。
		// eachParsedArticle 已经校过 period（标题推 vs 正文自解），这条校的是**日期**，
		// 是另一条独立推导：manifest 与文件错配时（AppendArticle 每篇立刻落盘，中途
		// 出错有窗口），没有这条则该期静默带着另一篇的发布日入库，而 published_at 是
		// 双时态的 revision 列 —— 错了会造出一条假修订。
		if pub := pa.item.a.Published; pub != "" && pa.obs.Meta.PublishedAt != pub {
			cal.Failures = append(cal.Failures, ParseFailure{
				Period: pa.item.period, Kind: pa.item.kind, File: pa.item.a.File,
				Err: fmt.Sprintf("发布日交叉校验不一致：manifest 记 %s，正文自解 %s —— "+
					"manifest 与文件错配，而 published_at 是双时态的 revision 列，"+
					"放行会造出一条假修订", pub, pa.obs.Meta.PublishedAt),
			})
			return
		}
		ps = append(ps, pa)
	})

	res := &BackfillLoadResult{
		Total:              len(m.Articles),
		Unsupported:        len(cal.Unsupported),
		ParseFailed:        len(cal.Failures),
		ParsedOK:           len(ps),
		IncompleteAccepted: m.CompletedAt == "",
		Unclassified:       cal.Unclassified,
		SHAUnverified:      shaUnverified,
		// W-8：manifest.failed 是 **fetch 阶段就没抓到**的篇目。它们既不在 articles 里、
		// 也不在上面任何一格里 —— 不带出来的话，报告会宣布「解析失败 N 篇」而实际
		// 少了这一批，读者据此以为语料是完整的。
		// ⚠️ 它们**不计入四道恒等式**：恒等式的分母是 len(m.Articles)，而这批根本不在
		// articles 里。混进去会让恒等式一凭空不成立，那是另一种假失败。
		FetchFailed: m.Failed,
	}
	// 🔴 Attempted 取 **eachParsedArticle 自己维护的计数器**（cal.Periods：循环首 ++、
	// Unsupported 分流处 --），**不是** ParsedOK + ParseFailed。
	//
	// 后者会让恒等式二 `Attempted = ParsedOK + ParseFailed` **恒真** —— 一个由两个加数
	// 派生出来的和，再拿去和这两个加数比，检查不出任何东西。取管道的独立计数器，那道
	// 恒等式才真的在交叉校验「管道认为尝试了几篇」与「我这边成功+失败各几篇」。
	res.Attempted = cal.Periods
	return res, ps, nil
}

// pendingReason 从报告里挑出把这一期打进 pending 的那道闸。
//
// 只报第一道失败的：报告里已有完整清单，这里要的是「一眼看出为什么」。
func pendingReason(rep ValidationReport) string {
	for _, c := range rep.Checks {
		if c.Status == CheckFailed {
			return c.ID + ": " + c.Reason
		}
	}
	return "未过闸（无 failed 明细）"
}

// countMergedRows 数库里 extractor = merged@v1 的行数，**两张表都数**。
//
// 🔴 只数 hestia_observations 会得到 28 而不是 42：96 个观测里有 21 个被闸门拦下落进
// pending，而闸门**不区分 extractor**。危险处在于 75 = 42+28+3+2 自洽 ——
// 少看一张表不会有任何东西提示你（M1c-3b 的 TASK-010 在验收时实撞，记于 CONTRACTS §A-2）。
func countMergedRows(ctx context.Context, s *Store) (int, error) {
	var n int
	err := s.DB().QueryRowContext(ctx,
		`select (select count(*) from hestia_observations where extractor = ?)
		      + (select count(*) from hestia_pending      where extractor = ?)`,
		extractorMerged, extractorMerged).Scan(&n)
	return n, err
}

// 三族报告：月报族（含季报）/ 社融存量 / 社融增量。
//
// 手写而不是从 validExtractors 派生：那张表是**取值域**，回答「合法吗」；这里问的是
// 「属于哪一族」，两者会分叉（llm-fallback@v1 与 merged@v1 都在取值域里却不属于任何一族）。
var extractorFamilies = []struct {
	name string
	has  func(string) bool
}{
	{"月报族", isFinanceFamily},
	{"社融存量", func(e string) bool { return e == extractorTSFStock }},
	{"社融增量", func(e string) bool { return e == extractorTSFFlow }},
}

// conflictError 把「有字段冲突」变成一个 error（M1c-3b 的 TASK-006，W-4）。
//
// 抽成函数只为一件事：让「冲突 ⇒ 整趟失败」这条性质**可以被单测直接钉住**。
// 留在 BackfillLoad 里的话，要构造它就得先造出一次真的冲突，而三族字段集不相交
// 使得真语料上造不出来 —— 那条性质就会变成没人测过的声称。
func conflictError(res *BackfillLoadResult) error {
	n := len(res.Conflicts)
	if n == 0 {
		return nil
	}
	return fmt.Errorf(
		"合并出现 %d 处字段冲突：三族的字段集设计上不相交，冲突理应恒为 0 —— "+
			"出现即说明字段归属表错了或语料形态变了。明细见报告的「字段冲突」一节", n)
}

// overlappingRequiredFields 返回**被一个以上 part 同时要求**的字段，按 fieldOrder 排序。
//
// 空切片 = 三族必填集互不相交这个前提仍然成立。非空即前提已破：那时字段冲突不再是
// 「归属表错了」的证据，而可能只是两族都该有这个字段 —— 判据的含义变了，必须让人知道。
func overlappingRequiredFields(parts []string) []string {
	count := map[string]int{}
	for _, p := range parts {
		for _, f := range requiredFields(p) {
			count[f]++
		}
	}
	var dup []string
	for _, f := range fieldOrder {
		if count[f] > 1 {
			dup = append(dup, f)
		}
	}
	return dup
}

// groupKey 是一条合并观测在报告里的显示键，也是 MissingFields / PendingReasons 的键。
//
// 🔴 **必须是完整业务键 `period/period_type@published_at`**（M1c-3b 的 TASK-006，W-10）。
//
// 此前只用 `period/period_type`，而合并键是三元组 —— 同一期次同一 period_type 若出现
// 第二个 published_at（一次修订发布），两条会**撞进同一个 map 键，后写的静默覆盖先写的**，
// 报告上就少一行而没有任何东西报错。这与 M1c-3b 的 TASK-008 的 R9 是同一个形状：
// 「今天不撞」靠的是央行的发布习惯，不是代码。
func groupKey(g MergedObservation) string {
	return g.Obs.Meta.Period + "/" + g.Obs.Meta.PeriodType + "@" + g.Obs.Meta.PublishedAt
}

// missingFields 返回这条观测相对**完整三族覆盖**缺掉的字段，按 fieldOrder 的固定顺序。
//
// 🔴 与 missingFamilies 的区别就是 W-1 的全部内容：本函数问的是「字段有没有洞」，
// 那才是 PartialCoverage 这一节存在的理由；missingFamilies 问的是「由几族合成」，
// 那只是它在 2025-10 之前的一个代理变量，并篇之后完全失效。
//
// 🔴 **参照集不是全部 54 个字段，而是「完整三族覆盖会给出的那些」**。
// 直接拿 54 个比会把**完整**的观测判成部分覆盖：月报正文本就没有外汇储备板块，
// 三族齐全的 merged@v1 也只有 52 个字段（缺 fx_reserve / fx_rate）——
// 那不是洞，是那类报告本来就没有的东西。⚠️ 这个错误我在改本条时**当场撞到**：
// 子测试「三族齐全时不算部分覆盖」立刻转红，参照集选错会造出一批新的假阳，
// 方向与 W-1 要修的那批恰好相同。
//
// 月报那一族用**这条观测自己带的**月报 extractor（v1/v2 的必填集不同：只有 rule@v2
// 有外汇节）；没有月报部分时退回 extractorMonthlyV2 作代表 —— 它是 2025-10 之后的形态，
// 也是「本该有月报却没有」时应当参照的那一个。
func missingFields(g MergedObservation) []string {
	monthly := extractorMonthlyV2
	for _, p := range g.Parts {
		if isFinanceFamily(p) {
			monthly = p
			break
		}
	}
	var missing []string
	for _, f := range mergedRequiredFields([]string{monthly, extractorTSFStock, extractorTSFFlow}) {
		if _, ok := g.Obs.Values[f]; !ok {
			missing = append(missing, f)
		}
	}
	return missing
}

// missingFamilies 返回 parts 里缺掉的报告族，按 extractorFamilies 的固定顺序。
func missingFamilies(parts []string) []string {
	var missing []string
	for _, fam := range extractorFamilies {
		if !slices.ContainsFunc(parts, fam.has) {
			missing = append(missing, fam.name)
		}
	}
	return missing
}
