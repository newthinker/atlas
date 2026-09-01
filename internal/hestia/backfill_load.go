package hestia

import (
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

	m.Obs.Values = vals
	m.Obs.Meta.Extractor = extractorMerged
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
		switch p.obs.Meta.Extractor {
		case extractorV1, extractorV2, extractorMonthlyV1, extractorMonthlyV2:
			return p.obs.Meta.ArticleID
		}
	}
	return g[0].obs.Meta.ArticleID
}
