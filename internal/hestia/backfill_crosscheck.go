package hestia

import "fmt"

// 两侧交叉校验（M1c-1 的 TASK-005，design-spec §5）。
//
// index 翻页是主路径，站内搜索是**独立第二来源**（ADR-M1c1-01）。两侧筛完栏目前缀后
// URL 同形态 ⇒ **以 article_id 为键**直接比对：
//
//	抓取集 = A ∪ B          ← 宁可多抓，不漏
//	A \ B  → 搜索没索到的   ← 实测已知会发生（137 条 vs 理论 158 条，差 21 条）
//	B \ A  → index 没翻到的 ← 这才是真风险信号
//
// `B \ A` 非空是**告警而非失败**：它意味着 index 翻页漏了，但并集已把这些收进抓取集，
// 交付物不缺。
//
// 本文件是**纯函数**：不碰网络、不碰磁盘。取回两侧候选是调用方的事（index 侧
// `scanBackfillIndex`、搜索侧 `fetchBackfillSearchPage`），落盘是 TASK-006 的事。
const (
	backfillSourceIndex  = "index"
	backfillSourceSearch = "search"
	backfillSourceBoth   = "both"
)

// backfillCandidate 是一条待抓的报告，外加它的来源。
//
// Source 的三个取值就是 manifest 里 `Article.Source` 的口径（TASK-003 的契约）。
// 放在这一层给出是因为**只有交叉校验知道答案** —— 不给，TASK-006 就得把交集算法
// 再实现一遍，而两份实现迟早会不一致。
type backfillCandidate struct {
	backfillItem
	Source string
}

// backfillCrossCheck 是交叉校验的结果。
//
// 两个差集都写进 manifest（`only_in_index[]` / `only_in_search[]`）并在 final-report
// 报告；`SearchSkippedReason` 非空表示**这一轮根本没做校验**。
type backfillCrossCheck struct {
	Fetch        []backfillCandidate
	OnlyInIndex  []string
	OnlyInSearch []string

	// SearchSkippedReason 非空 ⇒ 搜索侧失效、本轮跳过了交叉校验。
	//
	// ⚠️ **有声跳过，不静默**（ADR-M1c1-02）：没有这个字段时，「这次没做校验」与
	// 「校验通过」在读者看来完全一样 —— 那正是本机制要防的失效模式本身。
	SearchSkippedReason string
}

// crossCheckBackfill 求两侧的并集与两个差集。
//
// searchErr 非 nil ⇒ **fail-open**：跳过校验、抓取集退化为完整的 index 侧、
// 两个差集留空、把原因写进 SearchSkippedReason，**返回值里没有 error**。
// 理由（ADR-M1c1-02）：交叉校验是附加防线，让一个不可控的第三方检索服务有权否决
// 主路径交付，是把交付可用性挂在它身上。
//
// ⚠️ **「搜索侧空集」与「搜索侧失效」是两种情形，处置不同**，别合并：
//
//	空集（searchErr == nil, search == nil）→ 照常校验，全部 A 落进 OnlyInIndex，reason 为空
//	失效（searchErr != nil）              → 跳过校验，两个差集为空，reason 非空
//
// 合并的后果很具体：「关键词写错导致 0 条」会被当成「服务挂了」而静默放过。
//
// 输出**定序**（index 侧原序在前、搜索侧独有的按其原序追加），不是随手的：manifest
// 要能逐次比对，顺序随机会让每次重跑都产生一堆假 diff。
func crossCheckBackfill(index []backfillItem, search []backfillSearchHit, searchErr error) backfillCrossCheck {
	if searchErr != nil {
		// 差集**必须留空**。把 A 全塞进 OnlyInIndex 是一句谎报：它宣称「搜索没索到
		// 这几百篇」，而事实是根本没问过搜索，读 manifest 的人无从分辨。
		return backfillCrossCheck{
			Fetch:               taggedCandidates(index, backfillSourceIndex),
			SearchSkippedReason: fmt.Sprintf("search side failed, cross-check skipped: %v", searchErr),
		}
	}

	inSearch := make(map[string]bool, len(search))
	for _, h := range search {
		inSearch[h.ArticleID] = true
	}

	fetch := make([]backfillCandidate, 0, len(index)+len(search))
	inIndex := make(map[string]bool, len(index))
	var onlyIndex []string
	for _, it := range index {
		if inIndex[it.ArticleID] {
			continue // 上游已去重，这里只是不让重复穿透到 manifest
		}
		inIndex[it.ArticleID] = true

		source := backfillSourceIndex
		if inSearch[it.ArticleID] {
			source = backfillSourceBoth
		} else {
			onlyIndex = append(onlyIndex, it.ArticleID)
		}
		fetch = append(fetch, backfillCandidate{backfillItem: it, Source: source})
	}

	var onlySearch []string
	seenSearch := make(map[string]bool, len(search))
	for _, h := range search {
		if inIndex[h.ArticleID] || seenSearch[h.ArticleID] {
			// 两侧都有的那条，字段取 **index 侧**：它的标题来自列表项的 `title=`
			// 属性（原文），URL 经 resolveURL 归一；搜索侧的标题是剥过 <font> 高亮的
			// 重建值。两者通常一致，不一致时以主路径为准 —— 交叉校验是附加防线，
			// 不该反过来改写主路径的数据。
			continue
		}
		seenSearch[h.ArticleID] = true
		onlySearch = append(onlySearch, h.ArticleID)
		fetch = append(fetch, backfillCandidate{
			backfillItem: backfillItem{
				ArticleID: h.ArticleID,
				URL:       h.URL,
				Title:     h.Title,
				Published: h.Published,
			},
			Source: backfillSourceSearch,
		})
	}

	return backfillCrossCheck{Fetch: fetch, OnlyInIndex: onlyIndex, OnlyInSearch: onlySearch}
}

// taggedCandidates 把 index 侧条目按同一来源打标，用于 fail-open 的退化路径。
func taggedCandidates(items []backfillItem, source string) []backfillCandidate {
	out := make([]backfillCandidate, 0, len(items))
	for _, it := range items {
		out = append(out, backfillCandidate{backfillItem: it, Source: source})
	}
	return out
}
