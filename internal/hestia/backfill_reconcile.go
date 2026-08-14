package hestia

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// M1c-1 的 TASK-008：完整性对账。
//
// # 为什么需要这一层
//
// 其余七个任务的守卫**全是否定式或局部判据**：「某页 0 条则报错」「翻满则报错」
// 「单篇失败记 failed」——每条守住一种**已知**成因。而「manifest 完备」是一个
// **全局、正向**的性质，在此之前**没有任何一条断言「应该有 N 篇，实际有 N 篇」**。
//
// 🔴 更要紧的是：**「整页被静默跳过」这个失效，在本 sprint 里只有这一层能发现。**
// TASK-002 的相邻页连续性守卫方向与它声称要防的失效相反（test-agent-27 构造观察）：
// 条目下架 ⇒ 后续前移 ⇒ 整页被略过 ⇒ p(N+1).newest 变**老** ⇒ 判据更满足 ⇒ **永不触发**。
// 被跳过的篇目既不在抓取集里、也不在任何差集里（两侧都没发现它），
// **唯一还能发现它的就是下面这条结构规则**。

// 三种报告种类。顺序即输出顺序（金融统计 → 社融存量 → 社融增量），
// 与 design-spec §2 列举 backfillTitleRE 三个分支的顺序一致。
const (
	backfillKindFinance = "金融统计数据报告"
	backfillKindStock   = "社会融资规模存量统计数据报告"
	backfillKindFlow    = "社会融资规模增量统计数据报告"
)

// backfillKindOrder 是「哪几种」列的稳定顺序，也是判缺篇时逐种检查的顺序。
var backfillKindOrder = []string{backfillKindFinance, backfillKindStock, backfillKindFlow}

// backfillWantedKinds 是某个模板版本下**应当存在**的报告种类。
//
// 🔴 v2 期次只应有《金融统计数据报告》一种 —— 社融存量/增量自 2025-09 起**央行不再发**
// （实测：两者的最后一期是「2025年8月…」，同日发布于 2025-09-12）。
// ⇒ 对 v2 期次列出「缺存量、缺增量」是**误导**：那两种在该期本来就不存在，不是缺了。
// 「缺」必须相对**该期应有的种类**算，不是相对全部三种算。
func backfillWantedKinds(rule string) []string {
	if rule == backfillRuleV2 {
		return backfillKindOrder[:1]
	}
	return backfillKindOrder
}

// 模板版本。切换点**之前**每期三篇，**之后**每期一篇。
const (
	backfillRuleV1 = "rule@v1"
	backfillRuleV2 = "rule@v2"
)

// 推算出来的总数期望值，**不是实测**。
//
// 79 期 / 217 篇 = v1 期次 68 期 × 3 篇 + v2 期次 10 期 × 1 篇，再加上按发布日期口径
// （设计附录 §A1）收进来的 2019 年度三篇。它与 `--from` 的语义直接耦合。
//
// ⚠️ 因为是推算，它只有**告警**的权力：让一个未经实测的数有权阻断交付，会在推算偏差时
// 卡死整个 sprint。首跑报出实得值与推算值的差异，人类确认后把实测值固化进 CONTRACTS，
// 后续跑才用 backfillExpect 显式传入的硬判据。
const (
	backfillExpectedPeriods  = 79
	backfillExpectedArticles = 217
)

// backfillReconcileItem 是对账的最小输入：一篇文章的 id 与标题。
//
// 只要这两样：归组与判缺篇**全部依据标题**，id 仅用于把「是哪一篇」报回去。
// 刻意不直接吃 backfillCandidate 或 Article，因为对账在两个位置都有意义——
// 抓取**之前**（对候选集对账，可在花掉 250 次请求前就发现缺口）与抓取**之后**
// （对 manifest 对账，同时覆盖抓取失败造成的缺口）。两个适配器见下。
type backfillReconcileItem struct {
	ID    string
	Title string
}

// backfillReconcileItemsFromCandidates 把交叉校验的并集（TASK-005 的 Fetch）转成对账输入。
func backfillReconcileItemsFromCandidates(cs []backfillCandidate) []backfillReconcileItem {
	out := make([]backfillReconcileItem, 0, len(cs))
	for _, c := range cs {
		out = append(out, backfillReconcileItem{ID: c.ArticleID, Title: c.Title})
	}
	return out
}

// backfillReconcileItemsFromArticles 把 manifest 里已抓到的篇目（TASK-003 的 Article）转成对账输入。
func backfillReconcileItemsFromArticles(as []Article) []backfillReconcileItem {
	out := make([]backfillReconcileItem, 0, len(as))
	for _, a := range as {
		out = append(out, backfillReconcileItem{ID: a.ID, Title: a.Title})
	}
	return out
}

// backfillExpect 是总数期望值。零值表示**未显式传入** ⇒ 走告警路径。
type backfillExpect struct {
	Periods  int
	Articles int
}

// backfillPeriodRow 是对账表的一行：一个期次。
type backfillPeriodRow struct {
	Period  string   // 期末月 YYYY-MM，即归组键
	Rule    string   // rule@v1 / rule@v2
	Want    int      // 该期应有篇数：v1 = 3，v2 = 1
	Got     int      // 实得篇数
	Kinds   []string // 实得的报告种类，按 backfillKindOrder
	Missing []string // 缺的报告种类，同序
	Titles  []string // 实得的标题原文，稳定排序 —— 人工核对时要看的就是它
}

// backfillReconcile 是一次完整性对账的结果。
type backfillReconcile struct {
	Rows           []backfillPeriodRow // 按 Period 升序，每期一行
	Periods        int
	Articles       int      // 归类成功的篇数（不含 Unclassified）
	MissingPeriods []string // 不满足结构规则的期次，升序
	Unclassified   []string // 标题解析不出期次的原文 —— **不静默丢**
	Empty          bool     // 零期次的显式标记
	Warnings       []string // 告警：不阻断
	Violations     []string // 硬失败：显式期望值不符
}

// Failed 说明调用方是否该非零退出。
//
// **只有显式传入的期望值不符才算失败**（functional[2]）：缺篇本身是**要报告的事实**
// 而不是失败——「哪些期缺篇」正是 spec §10 Q3 要的答案，也是 M1c-3 的输入。
// 把缺篇判成失败会让一次正常的回填（历史上本来就有期次不齐）无法收工。
func (r backfillReconcile) Failed() bool { return len(r.Violations) > 0 }

// backfillPeriodOf 从标题解析出 (期末月, 报告种类)。
//
// # 归组的关键：按**期末月**归组，不按期次段的字面量
//
// 实测（比上游 spec §7 记的宽一倍）：累计期次里**只有社融存量退化成月**，
// 而年度期次**三者都无期次段**（存量不写「12月」）：
//
//	| 期次      | 金融统计   | 社融存量   | 社融增量   |
//	| 2020-03  | 一季度     | **3月**    | 一季度     |
//	| 2020-06  | 上半年     | **6月**    | 上半年     |
//	| 2019-09  | 前三季度   | **9月**    | 前三季度   |
//	| 2019 年度 | (无期次段) | (无期次段) | (无期次段) |
//
// 一季度→03 而 3月→03、上半年→06 而 6月→06、前三季度→09 而 9月→09、无段→12。
// ⇒ **四种形态自动坍缩到同一个键**，不需要维护一张「哪些说法算同一期」的映射表。
// 这也是为什么 3/6/9/12 月不会与累计期次撞车：央行那几个月**不单独发月报**，
// 由累计期次覆盖（设计附录 §E）。
//
// 期末月映射与 types.go 的 periodEndMonth、discover.go 的 parsePeriod 一致，
// **不新增第二份约定**。语义层的拒绝也照抄 parsePeriod：「一季度」是唯一真实存在的
// 季度形态（二/三/四季度分别由上半年/前三季度/全年覆盖），月份必须在 1..12。
// 被拒的标题不会消失，它们进 Unclassified —— 见 reconcileBackfill 的注释。
func backfillPeriodOf(title string) (period, kind string, ok bool) {
	m := backfillTitleRE.FindStringSubmatch(title)
	if m == nil {
		return "", "", false
	}
	year, seg, kind := m[1], m[2], m[3]
	switch {
	case seg == "":
		return year + "-12", kind, true
	case seg == "上半年":
		return year + "-06", kind, true
	case seg == "前三季度":
		return year + "-09", kind, true
	case strings.HasSuffix(seg, "季度"):
		if seg != "一季度" {
			return "", "", false
		}
		return year + "-03", kind, true
	default:
		n, err := strconv.Atoi(strings.TrimSuffix(seg, "月"))
		if err != nil || n < 1 || n > 12 {
			return "", "", false
		}
		return fmt.Sprintf("%s-%02d", year, n), kind, true
	}
}

// reconcileBackfill 按期次归组并用结构规则判缺篇。
//
// cutover 是模板切换点（期次 YYYY-MM，实测 "2025-09"）：**该期及其之后**按 rule@v2
// 每期一篇，之前按 rule@v1 每期三篇。它是**参数**不是常量——写死会让判定无法随实测调整，
// 也让「边界不偏移一期」这件事没法被测试单独证伪。
//
// 比较用字典序：期次是定长的 YYYY-MM，字典序与时序等价（同 backfill_scan.go 的日期比较）。
func reconcileBackfill(items []backfillReconcileItem, cutover string, expect backfillExpect) backfillReconcile {
	var r backfillReconcile

	byPeriod := map[string]map[string][]string{} // period → kind → titles
	for _, it := range items {
		period, kind, ok := backfillPeriodOf(it.Title)
		if !ok {
			// **不 continue 掉**：对账的全部意义是「不让东西静默消失」，而一个解析不出
			// 期次的标题若被丢弃，它就从这张表上彻底消失了——与本任务要防的失效完全同形。
			r.Unclassified = append(r.Unclassified, it.Title)
			continue
		}
		if byPeriod[period] == nil {
			byPeriod[period] = map[string][]string{}
		}
		byPeriod[period][kind] = append(byPeriod[period][kind], it.Title)
		r.Articles++
	}

	periods := make([]string, 0, len(byPeriod))
	for p := range byPeriod {
		periods = append(periods, p)
	}
	sort.Strings(periods) // 稳定且可 diff：这份表会被反复跑、反复比对

	for _, p := range periods {
		row := backfillPeriodRow{Period: p, Rule: backfillRuleV1}
		if p >= cutover {
			row.Rule = backfillRuleV2
		}
		wanted := backfillWantedKinds(row.Rule)
		row.Want = len(wanted)

		// Kinds / Got / Titles 走**全部三种**：v2 期次若真出现了社融存量，那是站点行为
		// 变了，必须在表上看得见，不能因为「v2 不该有它」就把它从实得里抹掉。
		// Missing 才走 wanted —— 「缺」相对应有的种类算。
		for _, kind := range backfillKindOrder {
			titles := byPeriod[p][kind]
			if len(titles) == 0 {
				continue
			}
			row.Kinds = append(row.Kinds, kind)
			row.Got += len(titles)
			row.Titles = append(row.Titles, titles...)
		}
		for _, kind := range wanted {
			if len(byPeriod[p][kind]) == 0 {
				row.Missing = append(row.Missing, kind)
			}
		}
		sort.Strings(row.Titles)

		// 判据是「**应有的种类里有没有缺的**」，不是「篇数够不够」。
		//
		// DoD 写的是「v1 期次不足 3 篇即缺篇」，而按种类判是它的**严格加强**，两者不冲突：
		// Got < Want ⇒ 篇数不够 ⇒ 至多覆盖 Got 个种类 ⇒ 必有应有种类缺席 ⇒ 同样被本判据抓到。
		// 反过来则不然，而那个差集正是按篇数判会漏掉的一类：
		//
		//	「2 篇社融存量 + 1 篇增量、0 篇金融统计」⇒ Got=3 == Want=3 ⇒ 按篇数判**放行**，
		//	而这一期实际缺了《金融统计数据报告》。
		//
		// 重复篇目在真实语料里会发生（同一期修订重发、跨页边界重复未被上游去净），
		// 而缺篇表是 M1c-3 判「哪些期会缺字段」的直接输入 —— 放行一期缺了金融统计的期次，
		// 下游会在入库时才发现，那时已经没有回填上下文了。
		if len(row.Missing) > 0 {
			r.MissingPeriods = append(r.MissingPeriods, p)
		}
		r.Rows = append(r.Rows, row)
	}
	r.Periods = len(r.Rows)

	if r.Periods == 0 {
		// 「一期都没有」与「全部齐全」在缺篇表上**无法区分**（两者 MissingPeriods 都空），
		// 所以零期次必须有独立标记 + 出声。这条挡的正是本任务要兜住的静默零通道：
		// `--from` 打错一位 ⇒ 0 篇 + 退出码 0。
		r.Empty = true
		r.Warnings = append(r.Warnings,
			"reconcile: 零期次——对账表为空。若这不是预期结果，先检查 --from 是否写错")
	}
	if n := len(r.Unclassified); n > 0 {
		r.Warnings = append(r.Warnings,
			fmt.Sprintf("reconcile: %d 条标题解析不出期次，未计入对账（可能是站点改了期次表述）: %s",
				n, strings.Join(r.Unclassified, " / ")))
	}

	// 总数期望：显式传入 ⇒ 硬失败；未传 ⇒ 与推算值比对后告警。
	// 同一件事只报一次——显式传入时不再走告警路径。
	//
	// 「显式传入」的判据（explicit > 0，即 backfillExpect 的零值表示未传）**只写这一处**：
	// 它原先同时写在取值处与调用处，两处一旦不同步，就会出现「拿推算值去判硬失败」——
	// 那正是本文件反复声明不许发生的事（推算值只有告警的权力）。
	checkTotal := func(name string, explicit, estimated, got int) {
		if explicit > 0 {
			if explicit != got {
				r.Violations = append(r.Violations,
					fmt.Sprintf("reconcile: %s 期望 %d，实得 %d", name, explicit, got))
			}
			return
		}
		if estimated != got {
			r.Warnings = append(r.Warnings,
				fmt.Sprintf("reconcile: %s 实得 %d，推算值 %d（推算不是实测，仅供核对）", name, got, estimated))
		}
	}
	checkTotal("期数", expect.Periods, backfillExpectedPeriods, r.Periods)
	checkTotal("篇数", expect.Articles, backfillExpectedArticles, r.Articles)

	return r
}
