package hestia

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// —— 按业务键合并 Observation 的测试（M1c-3b 的 TASK-003）——
//
// Context Checkpoint: done_criteria → test mapping
// functional[1] 「三篇同键 ⇒ 合成一条，字段取并集」  → TestMergeByBusinessKeyUnionsFields
// functional[2] 「article_id 优先月报 / 否则字典序最小」→ TestMergePrefersMonthlyArticleID
//                                                        TestMergeWithoutMonthlyFallsBackToSmallestID
// functional[3] 「按 period 升序、同期按 period_type 升序」→ TestMergeIsStableAndSorted
// boundary[0]   「单篇不得改写成 merged@v1」          → TestMergeSingleArticleKeepsOriginalExtractor
// error_handling[0] 「冲突不静默取值」                → TestMergeRecordsFieldConflict
// error_handling[1] 「同值不算冲突」                  → TestMergeSameValueIsNotAConflict

func mkParsed(period, pt, pub, id, ext string, vals map[string]float64) parsedArticle {
	return parsedArticle{
		item: calibrateItem{period: period},
		obs: Observation{
			Meta: Meta{
				Period: period, PeriodType: pt, PublishedAt: pub,
				ArticleID: id, CaliberVersion: "2023-01", Extractor: ext,
			},
			Values: vals,
		},
	}
}

// TestMergeByBusinessKeyUnionsFields：同键多篇合并成一条，字段取并集。
func TestMergeByBusinessKeyUnionsFields(t *testing.T) {
	ps := []parsedArticle{
		mkParsed("2019-12", "annual", "2020-01-16", "id-A", extractorV1,
			map[string]float64{FieldM2: 198.65}),
		mkParsed("2019-12", "annual", "2020-01-16", "id-B", extractorTSFStock,
			map[string]float64{FieldTSFStock: 251.31}),
		mkParsed("2019-12", "annual", "2020-01-16", "id-C", extractorTSFFlow,
			map[string]float64{FieldTSFFlowYTD: 2437}),
	}
	got := mergeByBusinessKey(ps)
	require.Len(t, got, 1, "三篇同键必须合成一条")
	m := got[0]
	require.Equal(t, extractorMerged, m.Obs.Meta.Extractor)
	require.Equal(t, 198.65, m.Obs.Values[FieldM2])
	require.Equal(t, 251.31, m.Obs.Values[FieldTSFStock])
	require.Equal(t, 2437.0, m.Obs.Values[FieldTSFFlowYTD])
	require.ElementsMatch(t, []string{"id-A", "id-B", "id-C"}, m.SourceIDs)
	require.Empty(t, m.Conflicts, "三个 extractor 的字段集不相交，冲突理应为 0")
}

// TestMergeSingleArticleKeepsOriginalExtractor：只有一篇的键**不得**被改写成 merged@v1。
//
// 改写会让 96 个观测里那 54 个单篇也走 mergedRequiredFields 那条路，
// 而它们的必填集本来就由自己的 extractor 说得清 —— 多绕一圈只多一个出错的机会。
func TestMergeSingleArticleKeepsOriginalExtractor(t *testing.T) {
	ps := []parsedArticle{
		mkParsed("2026-07", "monthly", "2026-08-14", "id-X", extractorMonthlyV2,
			map[string]float64{FieldM2: 356.71}),
	}
	got := mergeByBusinessKey(ps)
	require.Len(t, got, 1)
	require.Equal(t, extractorMonthlyV2, got[0].Obs.Meta.Extractor,
		"单篇不合并，extractor 保持原样")
	require.Equal(t, "id-X", got[0].Obs.Meta.ArticleID)
	require.Empty(t, got[0].DroppedIDs)
}

// TestMergePrefersMonthlyArticleID：合并组的 article_id 取月报那篇。
//
// 月报是这一期的主报告，运维按它拼回 URL 的概率最高。被丢弃的必须记进
// DroppedIDs —— 丢弃一个能拼回 URL 的定位符有代价，代价可以付，但不能不出声。
func TestMergePrefersMonthlyArticleID(t *testing.T) {
	ps := []parsedArticle{
		mkParsed("2025-07", "monthly", "2025-08-13", "id-tsfstock", extractorTSFStock,
			map[string]float64{FieldTSFStock: 430.0}),
		mkParsed("2025-07", "monthly", "2025-08-13", "id-monthly", extractorMonthlyV1,
			map[string]float64{FieldM2: 320.0}),
	}
	got := mergeByBusinessKey(ps)
	require.Len(t, got, 1)
	require.Equal(t, "id-monthly", got[0].Obs.Meta.ArticleID)
	require.Equal(t, []string{"id-tsfstock"}, got[0].DroppedIDs)
}

// TestMergeWithoutMonthlyFallsBackToSmallestID：无月报时取字典序最小。
//
// 实测 2020-01|monthly 就是这样 —— 月报那篇落在解析失败格里，只剩社融两篇。
func TestMergeWithoutMonthlyFallsBackToSmallestID(t *testing.T) {
	ps := []parsedArticle{
		mkParsed("2020-01", "monthly", "2020-02-20", "id-zzz", extractorTSFFlow,
			map[string]float64{FieldTSFFlowYTD: 5000}),
		mkParsed("2020-01", "monthly", "2020-02-20", "id-aaa", extractorTSFStock,
			map[string]float64{FieldTSFStock: 251.0}),
	}
	got := mergeByBusinessKey(ps)
	require.Len(t, got, 1)
	require.Equal(t, "id-aaa", got[0].Obs.Meta.ArticleID)
	require.Equal(t, []string{"id-zzz"}, got[0].DroppedIDs)
	require.ElementsMatch(t, []string{extractorTSFStock, extractorTSFFlow}, got[0].Parts)
}

// TestMergeRecordsFieldConflict：同字段两个不同值 ⇒ 记冲突，不静默取值。
//
// 三个 extractor 的字段集设计上不相交（27 / 18 / 9 = 54），冲突理应恒为 0。
// 正因如此，一旦出现就说明字段归属表错了 —— 那是必须响亮失败的事，
// 而「取第一个」会让一张错的归属表永远不被发现。
func TestMergeRecordsFieldConflict(t *testing.T) {
	ps := []parsedArticle{
		mkParsed("2021-06", "h1", "2021-07-09", "id-1", extractorV1,
			map[string]float64{FieldM2: 231.0}),
		mkParsed("2021-06", "h1", "2021-07-09", "id-2", extractorTSFStock,
			map[string]float64{FieldM2: 999.0}),
	}
	got := mergeByBusinessKey(ps)
	require.Len(t, got, 1)
	require.Len(t, got[0].Conflicts, 1)
	c := got[0].Conflicts[0]
	require.Equal(t, FieldM2, c.Field)
	require.Equal(t, 231.0, c.A)
	require.Equal(t, 999.0, c.B)
	require.Equal(t, "id-1", c.FromA)
	require.Equal(t, "id-2", c.FromB)
}

// TestMergeSameValueIsNotAConflict：同字段同值不算冲突。
func TestMergeSameValueIsNotAConflict(t *testing.T) {
	ps := []parsedArticle{
		mkParsed("2021-06", "h1", "2021-07-09", "id-1", extractorV1,
			map[string]float64{FieldM2: 231.0}),
		mkParsed("2021-06", "h1", "2021-07-09", "id-2", extractorTSFStock,
			map[string]float64{FieldM2: 231.0}),
	}
	got := mergeByBusinessKey(ps)
	require.Empty(t, got[0].Conflicts)
}

// TestMergeIsStableAndSorted：输出按 period 升序、同期按 period_type 升序。
//
// 稳定才能逐次 diff —— 与 classifyArticles 「返回的 items 按期次升序」同一个理由。
func TestMergeIsStableAndSorted(t *testing.T) {
	ps := []parsedArticle{
		mkParsed("2021-06", "h1", "2021-07-09", "b", extractorV1, map[string]float64{FieldM2: 1}),
		mkParsed("2020-01", "monthly", "2020-02-20", "a", extractorV1, map[string]float64{FieldM2: 2}),
		mkParsed("2021-06", "annual", "2021-07-09", "c", extractorV1, map[string]float64{FieldM2: 3}),
	}
	got := mergeByBusinessKey(ps)
	require.Len(t, got, 3)
	require.Equal(t, "2020-01", got[0].Obs.Meta.Period)
	require.Equal(t, "annual", got[1].Obs.Meta.PeriodType, "同期按 period_type 升序")
	require.Equal(t, "h1", got[2].Obs.Meta.PeriodType)
}
