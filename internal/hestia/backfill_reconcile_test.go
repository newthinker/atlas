package hestia

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping（M1c-1 的 TASK-008）
//
//	functional[0] 逐期对账表、稳定排序   → TestReconcileBackfillPerPeriodTable
//	functional[1] 结构规则判缺篇         → TestReconcileBackfillMissingByStructuralRule
//	functional[1] 来源差异 ≠ 缺篇        → TestReconcileBackfillSourceDiffIsNotMissing
//	functional[2] 总数期望：告警 vs 硬失败 → TestReconcileBackfillExpectations
//	functional[3] 期次表述四种形态归组    → TestReconcileBackfillGroupsFourPeriodForms
//	boundary[0]   空输入必须可分          → TestReconcileBackfillEmptyInputIsMarked
//	boundary[1]   切换点前/当/后          → TestReconcileBackfillCutoverBoundary
//	boundary[2]   跳页缺口必须报出        → TestReconcileBackfillDetectsSilentlySkippedArticles
//	（自补）不静默丢无法归类的标题        → TestReconcileBackfillDoesNotSilentlyDropUnclassified

// rcItems 把标题列表变成对账输入。id 用序号编——对账只按标题归组，
// id 仅用于把「是哪一篇」报回去。
func rcItems(titles ...string) []backfillReconcileItem {
	out := make([]backfillReconcileItem, 0, len(titles))
	for i, t := range titles {
		out = append(out, backfillReconcileItem{ID: rcID(i), Title: t})
	}
	return out
}

func rcID(i int) string { return fmt.Sprintf("20250922125500%05d", i) }

// rcTrio 造某一期齐全的三篇（v1 形态：金融统计 + 社融存量 + 社融增量）。
// segFin / segStock / segFlow 分开传，因为**三者的期次表述可以不同**——
// 这正是 functional[3] 那张表要覆盖的事。
func rcTrio(year, segFin, segStock, segFlow string) []string {
	return []string{
		year + "年" + segFin + "金融统计数据报告",
		year + "年" + segStock + "社会融资规模存量统计数据报告",
		year + "年" + segFlow + "社会融资规模增量统计数据报告",
	}
}

func rcPeriods(r backfillReconcile) []string {
	out := make([]string, 0, len(r.Rows))
	for _, row := range r.Rows {
		out = append(out, row.Period)
	}
	return out
}

func rcRow(t *testing.T, r backfillReconcile, period string) backfillPeriodRow {
	t.Helper()
	for _, row := range r.Rows {
		if row.Period == period {
			return row
		}
	}
	t.Fatalf("对账表里没有期次 %s，实得期次: %v", period, rcPeriods(r))
	return backfillPeriodRow{}
}

// 🔴 functional[0]：按 (年, 期次表述) 归组，逐期一行，含实得篇数与**分别是哪几种**。
//
// 「哪几种」不能省：只报篇数时，一期里有两篇「存量」+ 零篇「增量」会被算成 2/3，
// 与「存量 1 + 增量 1、缺金融统计」在表上长得一样，而两者的缺口完全不同。
//
// 排序必须稳定且可 diff —— 这份表是 spec §10 Q3 的答案，也是 M1c-3 的直接输入，
// 会被人反复跑、反复比对。输入顺序变一下就重排的表，diff 出来全是噪声。
func TestReconcileBackfillPerPeriodTable(t *testing.T) {
	items := rcItems(append(slices.Concat(
		rcTrio("2020", "2月", "2月", "2月"),
		rcTrio("2020", "一季度", "3月", "一季度"),
	), "2020年4月金融统计数据报告")...)

	r := reconcileBackfill(items, "2025-09", backfillExpect{})

	assert.Equal(t, []string{"2020-02", "2020-03", "2020-04"}, rcPeriods(r), "按期次升序，每期一行")
	assert.False(t, r.Empty)
	assert.Equal(t, 3, r.Periods)
	assert.Equal(t, 7, r.Articles)

	feb := rcRow(t, r, "2020-02")
	assert.Equal(t, 3, feb.Got)
	assert.Equal(t, 3, feb.Want, "切换点之前 ⇒ rule@v1 ⇒ 应有 3 篇")
	assert.Empty(t, feb.Missing)
	assert.Equal(t, []string{backfillKindFinance, backfillKindStock, backfillKindFlow}, feb.Kinds,
		"「分别是哪几种」必须出现在表里，且种类顺序稳定")

	apr := rcRow(t, r, "2020-04")
	assert.Equal(t, 1, apr.Got)
	assert.Equal(t, []string{backfillKindStock, backfillKindFlow}, apr.Missing, "缺的是哪几种要点名")

	// 输入顺序不影响输出：把同一批条目倒过来喂，表必须逐字相同。
	rev := slices.Clone(items)
	slices.Reverse(rev)
	assert.Equal(t, r.Rows, reconcileBackfill(rev, "2025-09", backfillExpect{}).Rows,
		"排序必须稳定 —— 输入顺序变了就重排的表，diff 出来全是噪声")
}

// 🔴 functional[1]：缺篇判定用**结构规则**，切换点作为**参数**传入。
//
// 不依赖任何推算出来的总数：rule@v1 期次不足 3 篇即缺、rule@v2 期次不足 1 篇即缺。
// 这条同时是 spec §10 Q3「哪些期缺篇」的答案，也是 M1c-3「哪些期会缺字段」的直接输入。
func TestReconcileBackfillMissingByStructuralRule(t *testing.T) {
	items := rcItems(append(rcTrio("2020", "2月", "2月", "2月"), // 齐全
		"2020年3月金融统计数据报告", // 只有 1/3
		"2020年3月社会融资规模存量统计数据报告",
		"2025年10月金融统计数据报告", // 切换点之后，1/1 齐全
	)...)

	r := reconcileBackfill(items, "2025-09", backfillExpect{})
	oct := rcRow(t, r, "2025-10")

	assert.Equal(t, []string{"2020-03"}, r.MissingPeriods, "只有 2020-03 缺篇")
	assert.Equal(t, backfillRuleV1, rcRow(t, r, "2020-02").Rule)
	assert.Equal(t, backfillRuleV2, oct.Rule)
	assert.Equal(t, 1, oct.Want, "切换点之后每期只应有一篇")
	assert.Empty(t, oct.Missing)
	assert.Equal(t, []string{backfillKindFlow}, rcRow(t, r, "2020-03").Missing)

	// 切换点是**参数**：同一批输入换一个切换点，判定必须跟着变。
	// 写死 2025-09 的实现能让上面全绿，只有这一条会红。
	r2 := reconcileBackfill(items, "2020-03", backfillExpect{})
	assert.Equal(t, backfillRuleV2, rcRow(t, r2, "2020-03").Rule, "切换点挪到 2020-03 ⇒ 该期起只需 1 篇")
	assert.Empty(t, r2.MissingPeriods, "此时 2020-03 的 2 篇已超过 v2 要求的 1 篇 ⇒ 不缺")
}

// 🔴 functional[1] 的一个**按篇数判会漏掉**的形态：篇数够了，但缺的那种还是缺。
//
// DoD 写的是「不足 3 篇即缺篇」。照字面实现（`Got < Want`）时，下面这一期
// **Got = 3 == Want = 3 ⇒ 放行**，而它实际缺了《金融统计数据报告》：
//
//	2 篇社融存量（同期修订重发 / 跨页重复未去净）+ 1 篇增量 + 0 篇金融统计
//
// 按种类判是按篇数判的**严格加强**（篇数不够 ⇒ 必有种类缺席 ⇒ 同样被抓到），
// 所以换成种类判不会放过 DoD 要求抓的任何一种，只会多抓这一类。
//
// ⚠️ 这条与 TestReconcileBackfillPerPeriodTable 里那句「只报篇数时两种情况长得一样」
// 是**同一个观察的两半**：那里说的是表**展示**上分不开，这里说的是判据**判**不出来。
func TestReconcileBackfillCountEqualButKindMissing(t *testing.T) {
	r := reconcileBackfill(rcItems(
		"2020年2月社会融资规模存量统计数据报告",
		"2020年2月社会融资规模存量统计数据报告", // 重复一篇：篇数凑够 3，种类只有 2
		"2020年2月社会融资规模增量统计数据报告",
	), "2025-09", backfillExpect{})

	row := rcRow(t, r, "2020-02")
	require.Equal(t, 3, row.Got, "前置锚点：篇数确实凑够了 3 —— 按篇数判会在这里放行")
	assert.Equal(t, 3, row.Want)
	assert.Equal(t, []string{backfillKindFinance}, row.Missing, "缺的是金融统计那一种")
	assert.Equal(t, []string{"2020-02"}, r.MissingPeriods,
		"篇数够不等于不缺 —— 判据必须看种类")
}

// 🔴 functional[1] 后半：**来源差异不是缺篇**，两者是两回事。
//
// `OnlyInIndex` / `OnlyInSearch` 回答「这一篇是**哪一侧发现**的」；缺篇回答
// 「这一期**该有 3 篇却只有 2 篇**」。两个方向都要钉：
//   - 一篇只被搜索侧发现 ⇒ 它**在**抓取集里 ⇒ **不构成缺篇**
//   - 某期真缺篇时，两个差集**可能都是空的**（两侧都没发现那一篇）⇒ 差集看不出来
//
// ⇒ 对账**只看抓取集本身**，不读差集。这条用例把「不读差集」变成可观测的：
// 一个把 OnlyInSearch 当缺篇的实现会在前半段红，一个指望差集非空才报缺的实现会在后半段红。
func TestReconcileBackfillSourceDiffIsNotMissing(t *testing.T) {
	t.Run("只被搜索侧发现的一篇，不算缺篇", func(t *testing.T) {
		// 三篇齐全，其中「增量」那篇假设只有搜索侧索到 —— 对账层看到的是同一个抓取集。
		r := reconcileBackfill(rcItems(rcTrio("2020", "2月", "2月", "2月")...), "2025-09", backfillExpect{})

		assert.Empty(t, r.MissingPeriods, "它被抓到了，只是 index 侧没发现它 —— 数据不缺")
		assert.Equal(t, 3, rcRow(t, r, "2020-02").Got)
	})

	t.Run("两侧都没发现的那一篇，差集是空的，但必须报缺篇", func(t *testing.T) {
		// 抓取集里只有两篇 —— 第三篇两侧都没发现，所以它不在任何差集里，
		// 也不在抓取集里。**唯一能发现它的就是结构规则。**
		r := reconcileBackfill(rcItems(
			"2020年2月金融统计数据报告",
			"2020年2月社会融资规模存量统计数据报告",
		), "2025-09", backfillExpect{})

		require.Equal(t, []string{"2020-02"}, r.MissingPeriods)
		assert.Equal(t, []string{backfillKindFlow}, rcRow(t, r, "2020-02").Missing,
			"缺的那一篇从未出现在任何差集里 —— 结构规则是唯一的检测手段")
	})
}

// 🔴 functional[2]：总数期望值**默认告警、显式传入才硬失败**。
//
// ⚠️ 默认值（79 期 / 217 篇）是**推算不是实测**，且与 `--from` 语义直接耦合。
// 让一个未经实测的数有权阻断交付，会在推算偏差时卡死整个 sprint。
// ⇒ 首跑报差异，人类确认后把实测值固化进 CONTRACTS，后续跑才用硬失败。
func TestReconcileBackfillExpectations(t *testing.T) {
	items := rcItems(rcTrio("2020", "2月", "2月", "2月")...) // 1 期 / 3 篇

	t.Run("未显式传入 ⇒ 与推算值不符只告警，不构成失败", func(t *testing.T) {
		r := reconcileBackfill(items, "2025-09", backfillExpect{})

		assert.NotEmpty(t, r.Warnings, "1 期 3 篇远少于推算的 79 期 217 篇，必须出声")
		assert.Empty(t, r.Violations, "但默认值无权阻断 —— 它是推算不是实测")
		assert.False(t, r.Failed())
	})

	t.Run("显式传入且不符 ⇒ 硬失败", func(t *testing.T) {
		r := reconcileBackfill(items, "2025-09", backfillExpect{Periods: 79, Articles: 217})

		require.NotEmpty(t, r.Violations)
		assert.True(t, r.Failed(), "显式期望值不符必须非零退出")
		assert.Empty(t, r.Warnings, "显式传入时同一件事只报一次，不该既告警又失败")
	})

	t.Run("显式传入且相符 ⇒ 既不告警也不失败", func(t *testing.T) {
		r := reconcileBackfill(items, "2025-09", backfillExpect{Periods: 1, Articles: 3})

		assert.Empty(t, r.Violations)
		assert.Empty(t, r.Warnings)
		assert.False(t, r.Failed())
	})

	// 两个 flag 可以只传一个：只传 periods 时 articles 仍走告警路径。
	t.Run("只传其中一个 ⇒ 另一个仍是告警", func(t *testing.T) {
		r := reconcileBackfill(items, "2025-09", backfillExpect{Periods: 1})

		assert.Empty(t, r.Violations, "periods 相符")
		assert.NotEmpty(t, r.Warnings, "articles 没显式传 ⇒ 与推算值 217 不符 ⇒ 告警")
	})
}

// 🔴 functional[3]：期次表述的**四种形态**都要归到同一期。
//
// 实测（比上游 spec §7 记的宽一倍）：**累计期次里只有社融存量退化成月**，
// 而**年度期次三者都无期次段**（存量**不写**「12月」）。
//
//	| 期次      | 金融统计   | 社融存量 | 社融增量   |
//	| 2020-03  | 一季度     | **3月**  | 一季度     |
//	| 2020-06  | 上半年     | **6月**  | 上半年     |
//	| 2019-09  | 前三季度   | **9月**  | 前三季度   |
//	| 2019 年度 | (无期次段) | (无期次段) | (无期次段) |
//
// 归组能成立的关键：**按期末月归组**，而不是按期次段的字面量。
// 一季度→03、3月→03；上半年→06、6月→06；前三季度→09、9月→09；无段→12。
// 四种形态因此自动坍缩到同一个键，不需要维护一张「哪些说法算同一期」的映射表。
func TestReconcileBackfillGroupsFourPeriodForms(t *testing.T) {
	forms := []struct {
		name                            string
		year, segFin, segStock, segFlow string
		want                            string
	}{
		{"一季度：存量退化成 3月", "2020", "一季度", "3月", "一季度", "2020-03"},
		{"上半年：存量退化成 6月", "2020", "上半年", "6月", "上半年", "2020-06"},
		{"前三季度：存量退化成 9月", "2019", "前三季度", "9月", "前三季度", "2019-09"},
		{"年度：三者都无期次段（存量不写 12月）", "2019", "", "", "", "2019-12"},

		// 第二组实例：2025-06 与 2020-06 完全同构（同日 2025-07-14 发布，
		// 存量说「6月」而增量说「上半年」）⇒ 这不是 2020 年的个例，是稳定规律。
		{"2025-06：与 2020-06 同构，非个例", "2025", "上半年", "6月", "上半年", "2025-06"},
	}

	for _, tt := range forms {
		t.Run(tt.name, func(t *testing.T) {
			r := reconcileBackfill(rcItems(rcTrio(tt.year, tt.segFin, tt.segStock, tt.segFlow)...),
				"2026-01", backfillExpect{})

			require.Equal(t, []string{tt.want}, rcPeriods(r),
				"三种表述必须坍缩到同一期；分成两行说明归组键取的是字面量而不是期末月")
			row := rcRow(t, r, tt.want)
			assert.Equal(t, 3, row.Got)
			assert.Empty(t, row.Missing)
		})
	}
}

// 🔴 boundary[0]：空输入 ⇒ 对账表为空**且明确标记「零期次」**，不得静默返回空表。
//
// ⚠️ 这条挡的正是本任务要兜住的那条静默零通道：`--from` 打错一位 ⇒ 0 篇 + 退出码 0。
// 「一期都没有」与「全部齐全」在读者看来必须**可分** —— 两者的 MissingPeriods 都是空的。
func TestReconcileBackfillEmptyInputIsMarked(t *testing.T) {
	empty := reconcileBackfill(nil, "2025-09", backfillExpect{})

	assert.True(t, empty.Empty, "零期次必须有显式标记")
	assert.Empty(t, empty.Rows)
	assert.NotEmpty(t, empty.Warnings, "静默返回空表 = 把「什么都没抓到」伪装成正常")

	// 阴性对照：非空且齐全时 Empty 必须是 false，且两者的 MissingPeriods 都是空的
	// ——**只有 Empty 这个标记能把它们分开**，这正是本条存在的理由。
	full := reconcileBackfill(rcItems(rcTrio("2020", "2月", "2月", "2月")...), "2025-09", backfillExpect{})
	assert.False(t, full.Empty)
	assert.Empty(t, full.MissingPeriods)
	assert.Empty(t, empty.MissingPeriods, "两者在缺篇表上无法区分 —— 所以必须靠 Empty")
}

// 🔴 boundary[1]：切换点前一期 / 当期 / 后一期各一条，判定不得偏移一期。
//
// 边界数据（Leader 2026-08-14 亲自复核，对照值不是判据）：
//   - 切换点 = 期次 **2025-09**
//   - 前一期 2025-08：**三篇齐全**，社融存量/增量的最后一期，同日发布于 2025-09-12
//   - 2025-09 起每期恰好一篇（2025-10-15 前三季度 / 2025-11-13 10月 / …）
//
// ⚠️ 切换点当期用的是**累计表述**（「2025年前三季度」而不是「2025年9月」）——
// 归组按期末月，两者都落到 2025-09，所以这条同时守着「边界不偏移」与「表述不影响归组」。
func TestReconcileBackfillCutoverBoundary(t *testing.T) {
	items := rcItems(append(rcTrio("2025", "8月", "8月", "8月"), // 前一期：v1，3 篇齐全
		"2025年前三季度金融统计数据报告", // 当期：v2，1 篇
		"2025年10月金融统计数据报告",  // 后一期：v2，1 篇
	)...)

	r := reconcileBackfill(items, "2025-09", backfillExpect{})
	aug, sep := rcRow(t, r, "2025-08"), rcRow(t, r, "2025-09")

	assert.Equal(t, backfillRuleV1, aug.Rule, "切换点**之前**仍是 v1")
	assert.Equal(t, 3, aug.Want)

	assert.Equal(t, backfillRuleV2, sep.Rule, "切换点**当期**已是 v2")
	assert.Equal(t, 1, sep.Want)

	assert.Equal(t, backfillRuleV2, rcRow(t, r, "2025-10").Rule)
	assert.Empty(t, r.MissingPeriods, "三期都满足各自的规则")

	// 偏移一期的实现（用 `>` 而不是 `>=` 比切换点）会让当期仍按 v1 判 ⇒ 报它缺 2 篇。
	// 上面那条 Rule 断言已经钉住方向，这里再钉后果，两条都红才说明真的偏移了。
	assert.NotContains(t, r.MissingPeriods, "2025-09",
		"当期按 v2 只需 1 篇；判成 v1 会误报缺 2 篇")
}

// 🔴 boundary[2]：**跳页缺口必须报出缺篇** —— 这一层是整条链唯一能发现它的地方。
//
// TASK-002 的相邻页连续性守卫**方向与它声称要防的失效相反**（test-agent-27 构造观察）：
// 条目下架 ⇒ 后续前移 ⇒ 整页被略过 ⇒ `p(N+1).newest` 变**老** ⇒ 判据更满足 ⇒ **永不触发**。
// 实测：净下架 −2 时那两篇仍在列表却**从未出现在任何一页**。
//
// ⇒ 被跳过的篇目**不在抓取集里、也不在任何差集里**（两侧都没发现它）。
// 唯一还能发现它的就是结构规则：这一期该有 3 篇，实得 1 篇。
//
// ⚠️ 本条与 `TestReconcileBackfillSourceDiffIsNotMissing` 的后半段**不是重复**：
// 那条钉的是「判据不依赖差集」，本条钉的是「**这一层必须在跳页输入上报出缺篇**」——
// 前者是实现约束，后者是这条链的闭合点。删任一条都会让另一条的意义变模糊。
func TestReconcileBackfillDetectsSilentlySkippedArticles(t *testing.T) {
	// 模拟净下架导致 2020-03 期的「存量」「增量」两篇被整页跳过：
	// 它们从未被任何一页扫到 ⇒ 抓取集里只剩金融统计那一篇。
	items := rcItems(
		"2020年2月金融统计数据报告",
		"2020年2月社会融资规模存量统计数据报告",
		"2020年2月社会融资规模增量统计数据报告",
		"2020年一季度金融统计数据报告", // 2020-03 期只剩这一篇
	)

	r := reconcileBackfill(items, "2025-09", backfillExpect{})

	require.Equal(t, []string{"2020-03"}, r.MissingPeriods,
		"翻页层对下架方向完全沉默 ⇒ 这一层不报，整条链就没人报了")
	row := rcRow(t, r, "2020-03")
	assert.Equal(t, 1, row.Got)
	assert.Equal(t, 3, row.Want)
	assert.Equal(t, []string{backfillKindStock, backfillKindFlow}, row.Missing,
		"要点名缺的是哪两种，而不只是报个数字")
}

// 无法归类的标题**不得静默丢弃**。
//
// 这条不在 DoD 里，是我加的：对账的全部意义是「不让东西静默消失」，而一个
// `parsePeriod` 认不出的标题（站点改了表述、或出现了「三季度」这类语义非法的期次）
// 若被 `continue` 掉，它就从这张表上彻底消失了 —— 与本任务要防的失效**完全同形**。
// 把它收进 Unclassified 并计入告警，人才有机会发现「表述变了」。
func TestReconcileBackfillDoesNotSilentlyDropUnclassified(t *testing.T) {
	items := rcItems(
		"2020年2月金融统计数据报告",
		"2020年三季度金融统计数据报告",   // 语义非法：央行不发单季报，parsePeriod 拒它
		"央行有关负责人就金融统计数据答记者问", // 压根不是报告
		// 月份上下界：`\d{1,2}月` 匹配得上，必须由语义层挡下（同 discover.go 的 parsePeriod）。
		// 它们若被静默接受，会造出 "2020-13" / "2020-00" 这种期次键，把表污染得无人能看懂。
		"2020年13月金融统计数据报告",
		"2020年0月金融统计数据报告",
	)

	r := reconcileBackfill(items, "2025-09", backfillExpect{})

	assert.Len(t, r.Unclassified, 4, "认不出的标题要留痕，不能 continue 掉")
	assert.Contains(t, r.Unclassified, "2020年三季度金融统计数据报告")
	assert.Contains(t, r.Unclassified, "2020年13月金融统计数据报告")
	assert.Contains(t, r.Unclassified, "2020年0月金融统计数据报告")
	assert.NotEmpty(t, r.Warnings, "有认不出的标题时必须出声 —— 可能是站点改了表述")
	assert.Equal(t, 1, r.Articles, "计入总数的只有归类成功的那一篇")
	assert.Equal(t, []string{"2020-02"}, rcPeriods(r), "非法月份不得造出 2020-13 / 2020-00 这种期次键")
}

// 两个适配器：对账在**抓取前**（候选集）与**抓取后**（manifest）都有意义。
//
// ⚠️ 这条守的是**跨任务契约**，不是本任务内部逻辑：TASK-006 落完 manifest 要对账、
// TASK-007 的 CLI 也可能在抓取前先对一次账（能在花掉约 250 次请求之前就发现缺口）。
// 本 sprint 反复出问题的正是这种「两个任务各自以为对方会做」的接缝，
// 所以把两个入口都钉住，而不是留给下游各写一个循环。
//
// 两者必须给出**同一张表**——它们只是同一批文章的两种载体。
func TestReconcileBackfillItemAdapters(t *testing.T) {
	titles := rcTrio("2020", "一季度", "3月", "一季度")

	var cands []backfillCandidate
	var arts []Article
	for i, title := range titles {
		id := rcID(i)
		cands = append(cands, backfillCandidate{
			backfillItem: backfillItem{ArticleID: id, Title: title, Published: "2020-04-10"},
			Source:       backfillSourceIndex,
		})
		arts = append(arts, Article{ID: id, Title: title, Published: "2020-04-10"})
	}

	fromCands := backfillReconcileItemsFromCandidates(cands)
	fromArts := backfillReconcileItemsFromArticles(arts)

	assert.Equal(t, fromCands, fromArts, "同一批文章的两种载体必须转出同一份对账输入")
	require.Len(t, fromCands, 3)
	assert.Equal(t, rcID(0), fromCands[0].ID, "id 要带过来 —— 报缺篇时人要知道是哪一篇")
	assert.Equal(t, titles[0], fromCands[0].Title)

	// 端到端：两条入口喂进去的对账结果逐字相同。
	a := reconcileBackfill(fromCands, "2025-09", backfillExpect{Periods: 1, Articles: 3})
	b := reconcileBackfill(fromArts, "2025-09", backfillExpect{Periods: 1, Articles: 3})
	assert.Equal(t, a, b)
	assert.Equal(t, []string{"2020-03"}, rcPeriods(a), "一季度 + 3月 + 一季度 仍坍缩到同一期")
	assert.False(t, a.Failed())
}
