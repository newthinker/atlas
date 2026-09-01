package hestia

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
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

// Context Checkpoint: done_criteria → test mapping (M1c-3b 的 TASK-006)
// functional[1]  mergeGroup 必须把 Parts 传到 Obs.Parts（单篇与多篇两条路径）
//                                        → TestMergeGroupPropagatesPartsToObservation
// error_handling[4] 跨接缝端到端：mergeByBusinessKey → Validate 后 completeness 被真正求值
//                                        → TestMergedObservationCompletenessIsEvaluatedEndToEnd
// boundary[0]    DBPath 已存在 ⇒ 报错          → TestBackfillLoadRefusesExistingDB
// boundary[1]    四道恒等式成立                → TestBackfillLoadIdentitiesHold
// boundary[2]    CompletedAt 缺失的两条分支    → TestBackfillLoadRequiresCompletedAt
// boundary[3]    os.Stat 必须先于 NewStore     → TestBackfillLoadDoesNotCreateDBBeforeChecking
// functional[5]  Out==nil ⇒ error，不退化 Discard → TestBackfillLoadRequiresOut

// loadFixture 复用 calibrate_test.go 的 writeCalibrateFixture（**不新写一份**：
// 两份 fixture 会分叉）。三篇同期报告——月报 + 社融存量 + 社融增量，正是本任务
// 要合并的那种组。
//
// ⚠️ 真签名是 (t, m Manifest, files map[string]string) string，它自己建 TempDir 并
// 返回目录；需求文档写的 (t, dir) 是错的，照抄编译不过。
func loadFixture(t *testing.T) string {
	t.Helper()
	return writeCalibrateFixture(t, Manifest{
		From:        "2025-08",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "id-monthly", Title: "2025年8月金融统计数据报告", File: "articles/m.html",
				SHA256: testdataSHA(t, "pboc-2025-08-monthly.html")},
			{ID: "id-stock", Title: "2025年8月社会融资规模存量统计数据报告", File: "articles/s.html",
				SHA256: testdataSHA(t, "pboc-2025-08-tsf-stock.html")},
			{ID: "id-flow", Title: "2025年8月社会融资规模增量统计数据报告", File: "articles/f.html",
				SHA256: testdataSHA(t, "pboc-2025-08-tsf-flow.html")},
		},
	}, map[string]string{
		"articles/m.html": "pboc-2025-08-monthly.html",
		"articles/s.html": "pboc-2025-08-tsf-stock.html",
		"articles/f.html": "pboc-2025-08-tsf-flow.html",
	})
}

// TestMergeGroupPropagatesPartsToObservation 钉住 A-1 的接缝修复（functional[1]）。
//
// 缺陷形态：M1c-3b 的 TASK-003 把 Parts 写在**包装结构** MergedObservation 上，而
// M1c-3b 的 TASK-011 的 gateCompleteness 读的是 Observation.Parts —— 它只拿得到 Obs，
// 看不见包装。两个任务各自的测试都绿，接缝无人认领。
func TestMergeGroupPropagatesPartsToObservation(t *testing.T) {
	mk := func(ex, id string, fs []string) parsedArticle {
		v := make(map[string]float64, len(fs))
		for i, f := range fs {
			v[f] = float64(i + 1)
		}
		return parsedArticle{obs: Observation{
			Meta:   Meta{Period: "2020-01", PeriodType: "monthly", PublishedAt: "2020-02-15", ArticleID: id, CaliberVersion: "2015-01", Extractor: ex},
			Values: v,
		}}
	}

	t.Run("多篇合并", func(t *testing.T) {
		m := mergeGroup([]parsedArticle{
			mk(extractorTSFStock, "a1", tsfStockFields()),
			mk(extractorTSFFlow, "a2", tsfFlowFields()),
		})
		require.Equal(t, extractorMerged, m.Obs.Meta.Extractor)
		assert.Equal(t, m.Parts, m.Obs.Parts,
			"Obs.Parts 必须与包装上的 Parts 一致——gateCompleteness 只读得到 Obs")
		assert.NotEmpty(t, m.Obs.Parts, "空 Parts 会让整道 completeness 恒 skipped")
	})

	// 🔴 单篇那条路径也必须赋。96 个观测里 54 个是单篇，它们今天走
	// requiredFields(原 extractor) 那条路碰巧也对，但一旦将来单篇也改走合并路径
	// 就静默失效——「碰巧对」不是可依赖的性质。
	t.Run("单篇也必须赋", func(t *testing.T) {
		m := mergeGroup([]parsedArticle{mk(extractorTSFStock, "solo", tsfStockFields())})
		require.Equal(t, extractorTSFStock, m.Obs.Meta.Extractor, "单篇不改写 extractor")
		assert.Equal(t, m.Parts, m.Obs.Parts, "单篇路径漏赋 Obs.Parts 是静默的")
		assert.Equal(t, []string{extractorTSFStock}, m.Obs.Parts)
	})
}

// TestMergedObservationCompletenessIsEvaluatedEndToEnd 是本任务**独有**的验收点
// （error_handling[4]）：一条同时经过 M1c-3b 的 TASK-003（产出）与 M1c-3b 的 TASK-011（消费）的断言。
//
// 为什么两边的守卫都没抓到接缝缺陷：M1c-3b 的 TASK-003 断的是 MergedObservation.Parts（自己的
// 产物），M1c-3b 的 TASK-011 **直接构造** Observation{Parts: …} 喂闸门（其 DoD 就是这么要求的）
// —— 两边都对，两边都没跨过接缝。凡「A 产出、B 消费」的字段，都要有一条同时经过
// A 和 B 的断言。
func TestMergedObservationCompletenessIsEvaluatedEndToEnd(t *testing.T) {
	mk := func(ex, id string, fs []string) parsedArticle {
		v := make(map[string]float64, len(fs))
		for i, f := range fs {
			v[f] = float64(i + 1)
		}
		return parsedArticle{obs: Observation{
			Meta:   Meta{Period: "2020-01", PeriodType: "monthly", PublishedAt: "2020-02-15", ArticleID: id, CaliberVersion: "2015-01", Extractor: ex},
			Values: v,
		}}
	}

	t.Run("字段齐全_completeness 必须 passed", func(t *testing.T) {
		ms := mergeByBusinessKey([]parsedArticle{
			mk(extractorTSFStock, "a1", tsfStockFields()),
			mk(extractorTSFFlow, "a2", tsfFlowFields()),
		})
		require.Len(t, ms, 1)

		rep, err := Validate(context.Background(), ms[0].Obs, NoHistory, DefaultThresholds())
		require.NoError(t, err)
		c := findCheck(t, rep, "completeness")

		// 🔴 钉**具体的 reason**，不能只断「不是 skipped」：空 Parts 与「真的缺字段」
		// 都会让它非 passed，两者必须可分。
		assert.NotContainsf(t, c.Reason, "unknown_extractor",
			"completeness=%s reason=%q —— 仍报 unknown_extractor 说明 Obs.Parts 是空的，"+
				"即 mergeGroup 没把 Parts 传进 Obs（缺口 A-1 原样保留）", c.Status, c.Reason)
		assert.Equal(t, CheckPassed, c.Status,
			"stock ∪ flow 齐全时 completeness 该 passed；若为 failed，看 reason 里缺的是哪些字段")
	})

	// 反向一格：真的缺字段时必须 failed 且 reason 说得出缺几个 —— 用它证明上面那格的
	// passed 不是因为闸门根本没在比对。
	t.Run("真缺字段_completeness 必须 failed 且可与空 Parts 区分", func(t *testing.T) {
		full := tsfStockFields()
		ms := mergeByBusinessKey([]parsedArticle{
			mk(extractorTSFStock, "a1", full[1:]), // 少一个
			mk(extractorTSFFlow, "a2", tsfFlowFields()),
		})
		require.Len(t, ms, 1)

		rep, err := Validate(context.Background(), ms[0].Obs, NoHistory, DefaultThresholds())
		require.NoError(t, err)
		c := findCheck(t, rep, "completeness")
		assert.Equal(t, CheckFailed, c.Status)
		assert.Contains(t, c.Reason, "missing 1", "缺 1 个就该报 missing 1")
		assert.NotContains(t, c.Reason, "unknown_extractor",
			"两种非 passed 必须可分：这条是「真缺字段」，不是「Parts 为空」")
	})
}

// TestBackfillLoadRefusesExistingDB：DBPath 已存在 ⇒ 报错（boundary[0]）。
//
// 回填是一次性动作。追加写会让四道恒等式失去意义，且掩盖「上一趟跑到哪」。
func TestBackfillLoadRefusesExistingDB(t *testing.T) {
	db := filepath.Join(t.TempDir(), "exists.db")
	require.NoError(t, os.WriteFile(db, []byte("x"), 0o644))

	_, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: loadFixture(t), DBPath: db, Cfg: DefaultThresholds(),
		Out: io.Discard, AllowIncomplete: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已存在")
	assert.Contains(t, err.Error(), "删", "错误串要说明重跑要删文件，否则人不知道下一步做什么")
}

// TestBackfillLoadDoesNotCreateDBBeforeChecking 钉住 boundary[3]：
// os.Stat(DBPath) 必须在 NewStore **之前**。
//
// 🔴 为什么 TestBackfillLoadRefusesExistingDB 抓不到顺序错：它用**已存在**的文件测，
// 两种实现都会拒。顺序错的后果是**第一次跑自己造出库、第二次被自己拒**，且第一次
// 那份库是半成品。故这里用**不存在**的路径正向跑一次，断言库被正确建立且内容完整。
func TestBackfillLoadDoesNotCreateDBBeforeChecking(t *testing.T) {
	db := filepath.Join(t.TempDir(), "nested", "new.db")
	require.NoFileExists(t, db)

	res, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: loadFixture(t), DBPath: db, Cfg: DefaultThresholds(),
		Out: io.Discard, AllowIncomplete: true})
	require.NoError(t, err)
	require.NotNil(t, res)

	require.FileExists(t, db, "正常路径必须把库建出来")
	s, err := NewStore(db)
	require.NoError(t, err, "建出来的库必须是完整可用的，不是半成品")
	t.Cleanup(func() { _ = s.Close() })
	got, err := s.RecentObservations(context.Background(), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, got, "跑完一趟该有观测入库")
}

// TestBackfillLoadRequiresOut：Out 为 nil ⇒ 报错，**不退化成 io.Discard**（functional[5]）。
//
// ⚠️ 这刻意背离需求文档（它写「nil 等价于 io.Discard」），沿用同包 Calibrate 的
// 相反契约（calibrate.go:571）：本函数的产出**就是那份报告**，默认丢弃会把调用方的
// 疏漏变成合法配置 —— cmd 层漏填 Out ⇒ 命令静默打印零字节、退出码 0，而「子命令
// 注册了吗」「flag 解析对吗」这类测试全部通过。
func TestBackfillLoadRequiresOut(t *testing.T) {
	_, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: loadFixture(t), DBPath: filepath.Join(t.TempDir(), "x.db"),
		Cfg: DefaultThresholds(), AllowIncomplete: true}) // Out 未填
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Out")
}

// TestBackfillLoadRequiresCompletedAt：CompletedAt 缺失的两条分支（boundary[2]）。
func TestBackfillLoadRequiresCompletedAt(t *testing.T) {
	fixture := func(t *testing.T) string {
		t.Helper()
		return writeCalibrateFixture(t, Manifest{
			From: "2025-08",
			Articles: []Article{{ID: "id-monthly", Title: "2025年8月金融统计数据报告",
				File: "articles/m.html", SHA256: testdataSHA(t, "pboc-2025-08-monthly.html")}},
		}, map[string]string{"articles/m.html": "pboc-2025-08-monthly.html"})
	}

	t.Run("未传 AllowIncomplete ⇒ 报错", func(t *testing.T) {
		_, err := BackfillLoad(context.Background(), BackfillLoadDeps{
			Dir: fixture(t), DBPath: filepath.Join(t.TempDir(), "a.db"),
			Cfg: DefaultThresholds(), Out: io.Discard})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "completed_at")
	})

	t.Run("传了 ⇒ 放行并置 IncompleteAccepted", func(t *testing.T) {
		res, err := BackfillLoad(context.Background(), BackfillLoadDeps{
			Dir: fixture(t), DBPath: filepath.Join(t.TempDir(), "b.db"),
			Cfg: DefaultThresholds(), Out: io.Discard, AllowIncomplete: true})
		require.NoError(t, err)
		assert.True(t, res.IncompleteAccepted,
			"放行必须留痕，否则报告读者无从知道这批数据是在缺完成标记的前提下算出来的")
	})
}

// TestBackfillLoadIdentitiesHold：四道恒等式在 fixture 上成立（boundary[1]）。
func TestBackfillLoadIdentitiesHold(t *testing.T) {
	res, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: loadFixture(t), DBPath: filepath.Join(t.TempDir(), "new.db"),
		Cfg: DefaultThresholds(), Out: io.Discard, AllowIncomplete: true})
	require.NoError(t, err)

	assert.Equal(t, res.Total, res.Attempted+res.Unsupported, "恒等式一")
	assert.Equal(t, res.Attempted, res.ParsedOK+res.ParseFailed, "恒等式二")
	assert.Equal(t, res.Merged, res.SingleArticle+res.MergedGroups, "恒等式三")
	assert.Equal(t, res.Merged, res.ToObservations+res.ToPending, "恒等式四")

	// 三篇同期同 published_at ⇒ 合成 1 组，且必须真的走了合并那条路。
	require.Equal(t, 1, res.Merged, "三篇同期该合成一条观测")
	assert.Equal(t, 1, res.MergedGroups)
	assert.Equal(t, 0, res.SingleArticle)
	require.Len(t, res.Groups, 1)
	assert.Equal(t, extractorMerged, res.Groups[0].Obs.Meta.Extractor)
	assert.Equal(t, res.Groups[0].Parts, res.Groups[0].Obs.Parts,
		"编排产出的观测同样必须带上 Obs.Parts")
	assert.Empty(t, res.Conflicts, "三个 extractor 的字段集设计上不相交，冲突理应恒为 0")
}

// TestBackfillLoadAssignsArticleID：functional[2] 的要点 (4)。
//
// item.a.ID 是 manifest 的 id，而 Parse 看不到 URL（接缝①）⇒ 必须显式赋值，
// 否则合并组的代表 id 恒为空，报告里那一列全是空白而没有任何东西会红。
func TestBackfillLoadAssignsArticleID(t *testing.T) {
	res, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: loadFixture(t), DBPath: filepath.Join(t.TempDir(), "new.db"),
		Cfg: DefaultThresholds(), Out: io.Discard, AllowIncomplete: true})
	require.NoError(t, err)
	require.Len(t, res.Groups, 1)

	g := res.Groups[0]
	assert.NotEmpty(t, g.Obs.Meta.ArticleID, "代表 article_id 不能为空")
	assert.ElementsMatch(t, []string{"id-monthly", "id-stock", "id-flow"}, g.SourceIDs,
		"三篇的 manifest id 都要被记下来")
	assert.Equal(t, "id-monthly", g.Obs.Meta.ArticleID,
		"pickArticleID 优先月报那篇——运维按它拼回 URL 的概率最高")
	assert.ElementsMatch(t, []string{"id-stock", "id-flow"}, g.DroppedIDs)
}

// TestBackfillLoadFlagsPartialCoverage：functional[6] 的**计算**侧。
//
// backfill_load_report_test.go 覆盖的是渲染侧（给定 PartialCoverage 怎么打印），
// 这条覆盖的是「谁被判为部分覆盖」——两侧分开测，否则一个恒返回空的实现也能让
// 渲染那条全绿。
func TestBackfillLoadFlagsPartialCoverage(t *testing.T) {
	// 只放社融存量那一篇：它入权威表、completeness 也过（tsf-stock@v1 的必填集
	// 就是那 18 个），在库里与「央行本来就没发」完全同形——正是要让它出声的那 44%。
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2025-08",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "id-stock", Title: "2025年8月社会融资规模存量统计数据报告", File: "articles/s.html",
				SHA256: testdataSHA(t, "pboc-2025-08-tsf-stock.html")},
		},
	}, map[string]string{"articles/s.html": "pboc-2025-08-tsf-stock.html"})

	var out bytes.Buffer
	res, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: dir, DBPath: filepath.Join(t.TempDir(), "p.db"),
		Cfg: DefaultThresholds(), Out: &out, AllowIncomplete: true})
	require.NoError(t, err)

	require.Equal(t, 1, res.ToObservations, "它该入权威表——正是这一点让它变得危险")
	require.Len(t, res.PartialCoverage, 1, "只有存量一篇 ⇒ 必须被判为部分覆盖")
	assert.Equal(t, []string{"月报族", "社融增量"}, missingFamilies(res.PartialCoverage[0].Parts))
	assert.Contains(t, out.String(), "部分覆盖的期次（1）")

	// 🔴 **判据是「缺哪些字段」，不是「缺哪一族」**（M1c-3b 的 TASK-006，W-1）。
	// 钉住缺字段本身，否则实现可以退回按族判而本条照样绿。
	miss := res.MissingFields[groupKey(res.PartialCoverage[0])]
	assert.NotEmpty(t, miss, "只有存量一篇 ⇒ 月报族与社融增量的字段全都缺")
	assert.Contains(t, miss, FieldM2, "月报族字段必须在缺失清单里")
	assert.Contains(t, out.String(), "缺 ", "报告要给出缺了多少个字段，不能只说缺哪一族")

	// 🔴 阴性对照：三族齐全 ⇒ 不得被列入。
	//
	// ⚠️ **这一格是 W-1 的实际杀手**：把参照集从「完整三族覆盖会给出的字段」换成
	// 「全部 54 个字段」，本格立刻红 —— 月报正文本就没有外汇储备板块，三族齐全的
	// merged@v1 也只有 52 个字段（缺 fx_reserve / fx_rate），那不是洞。
	// 改 W-1 时我当场撞到过这个：参照集选错会造出一批新的假阳，方向与 W-1 要修的那批相同。
	t.Run("三族齐全时不算部分覆盖", func(t *testing.T) {
		var out2 bytes.Buffer
		res2, err := BackfillLoad(context.Background(), BackfillLoadDeps{
			Dir: loadFixture(t), DBPath: filepath.Join(t.TempDir(), "q.db"),
			Cfg: DefaultThresholds(), Out: &out2, AllowIncomplete: true})
		require.NoError(t, err)
		assert.Empty(t, res2.PartialCoverage,
			"三族齐全 ⇒ 字段没有洞。若此处红，多半是把参照集当成了全部 54 个字段")
		assert.Empty(t, res2.MissingFields)
	})
}

// TestBackfillLoadRecordsPendingReasons：未过闸的期次落 pending，且**判因要说得出**
// （functional[3] 的 Outcome.Table 分流 + 报告的「落 pending 的期次」一节）。
//
// 用一条不可能成立的 magnitude_ranges 制造失败，而不是找一篇「本来就坏」的真报告：
// 后者会让这条测试绑死在某份 testdata 的内容上，语料一换就静默失去意义。
func TestBackfillLoadRecordsPendingReasons(t *testing.T) {
	cfg := DefaultThresholds()
	// tsf_stock 一定出现在存量报告里，且必为正；把区间钉在负半轴 ⇒ magnitude_sanity 必失败。
	// ⚠️ 不能写成 Min==Max（M1c-3b 的 TASK-004 加了倒置区间守卫，min>=max 直接报配置错，
	// 那会变成校验阶段的 error 而不是「未过闸落 pending」——两条路径完全不同）。
	cfg.MagnitudeRanges = map[string]Range{
		FieldTSFStock: {Min: -100, Max: -1, Unit: "万亿元"},
	}

	var out bytes.Buffer
	res, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: loadFixture(t), DBPath: filepath.Join(t.TempDir(), "pend.db"),
		Cfg: cfg, Out: &out, AllowIncomplete: true})
	require.NoError(t, err)

	assert.Equal(t, 1, res.ToPending)
	assert.Equal(t, 0, res.ToObservations)
	assert.Equal(t, res.Merged, res.ToObservations+res.ToPending, "恒等式四在 pending 路径上同样要成立")

	require.Len(t, res.PendingReasons, 1)
	// 🔴 用 groupKey 取而不是硬编码字符串（M1c-3b 的 TASK-006，W-10）：
	// 键现在是**完整业务键** period/period_type@published_at —— 只用前两段时，
	// 同一期次的第二次发布（修订）会静默覆盖前一条，报告上少一行而无人报警。
	require.Len(t, res.Groups, 1)
	key := groupKey(res.Groups[0])
	assert.Contains(t, key, "@", "键必须含 published_at，否则修订发布会互相覆盖")
	assert.Contains(t, key, res.Groups[0].Obs.Meta.PublishedAt)
	reason := res.PendingReasons[key]
	require.NotEmpty(t, reason, "判因不能为空——报告里那一列存在的全部意义就是「一眼看出为什么」")
	assert.Contains(t, reason, "magnitude_sanity", "判因要指名是哪道闸")

	body := out.String()
	assert.Contains(t, body, "落 pending 的期次（1）")
	assert.Contains(t, body, "2025-08/monthly")
	assert.Contains(t, body, "magnitude_sanity")

	// 落 pending 的期次**不**计入部分覆盖：那一节的定义是「入了权威表但不完整」，
	// 没进权威表的期次由 pending 那一节负责，两节混起来会重复报同一件事。
	assert.Empty(t, res.PartialCoverage)
}

// TestBackfillLoadRejectsBadDir：--dir 指错时要说清该指哪里（loadParsedArticles 的前置）。
func TestBackfillLoadRejectsBadDir(t *testing.T) {
	_, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "x.db"),
		Cfg: DefaultThresholds(), Out: io.Discard, AllowIncomplete: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), manifestFileName)
	assert.Contains(t, err.Error(), "--dir", "错误串要说清 --dir 该指向什么，否则人只知道「错了」")
}

// TestBackfillLoadCrossChecksPublishedAt：functional[2] 的要点 (5)。
//
// eachParsedArticle 已经校过 period（标题推 vs 正文自解），这条校的是**发布日**，
// 是另一条独立推导。published_at 是双时态的 revision 列 —— 错了会造出一条假修订，
// 而假修订在库里与真修订**完全同形**。
func TestBackfillLoadCrossChecksPublishedAt(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2025-08",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{{
			ID: "id-monthly", Title: "2025年8月金融统计数据报告", File: "articles/m.html",
			Published: "1999-01-01", // 与正文自解的发布日必然不符
			SHA256:    testdataSHA(t, "pboc-2025-08-monthly.html"),
		}},
	}, map[string]string{"articles/m.html": "pboc-2025-08-monthly.html"})

	var out bytes.Buffer
	res, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: dir, DBPath: filepath.Join(t.TempDir(), "x.db"),
		Cfg: DefaultThresholds(), Out: &out, AllowIncomplete: true})
	require.NoError(t, err)

	assert.Equal(t, 1, res.ParseFailed, "发布日对不上的那篇必须算失败、不入库")
	assert.Equal(t, 0, res.ParsedOK)
	assert.Equal(t, 0, res.Merged)
	assert.Equal(t, res.Attempted, res.ParsedOK+res.ParseFailed, "恒等式二在这条路径上同样要成立")
}

// TestBackfillLoadFailsLoudlyOnUnclassified 是上面那个设计决策的**端到端**证据：
// manifest 里混进一篇标题解析不出期次的，整趟回填必须以「账对不上」响亮失败。
//
// ⚠️ 它同时钉住「失败也要交出 res」：调用方要拿 res 去看究竟差在哪，
// 返回 nil 会让人只剩一句错误串。
func TestBackfillLoadFailsLoudlyOnUnclassified(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2025-08",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "id-monthly", Title: "2025年8月金融统计数据报告", File: "articles/m.html",
				SHA256: testdataSHA(t, "pboc-2025-08-monthly.html")},
			// 标题推不出期次：backfillPeriodOf 认不了它
			{ID: "id-qa", Title: "央行有关负责人就某事答记者问", File: "articles/q.html",
				SHA256: testdataSHA(t, "pboc-2025-08-monthly.html")},
		},
	}, map[string]string{
		"articles/m.html": "pboc-2025-08-monthly.html",
		"articles/q.html": "pboc-2025-08-monthly.html",
	})

	var out bytes.Buffer
	res, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: dir, DBPath: filepath.Join(t.TempDir(), "u.db"),
		Cfg: DefaultThresholds(), Out: &out, AllowIncomplete: true})

	require.Error(t, err, "有篇目无处可去 ⇒ 恒等式一不成立 ⇒ 整趟必须响亮失败")
	assert.Contains(t, err.Error(), "恒等式")
	require.NotNil(t, res, "失败也要交出 res——调用方要拿它看差在哪，只给一句错误串不够")
	assert.Equal(t, 2, res.Total)
	assert.Len(t, res.Unclassified, 1)

	// 🔴 **C-2 的端到端判据：报告必须真的写出去，且必须含那条标题的原文**
	//（M1c-3b 的 TASK-006）。
	//
	// 为什么这条不能用「手工把 res 凑平再调 writeLoadReport」来测：`len(Unclassified) > 0`
	// 与「恒等式一成立」由同一段代码保证**互斥**，那个组合在生产路径上根本不存在。
	// 用手工凑出来的 res 去测，测的是一个不存在的世界 —— 断言会红、外溢度正常、
	// 看起来什么都对，唯独证明不了生产路径上那段代码可达。
	// ⇒ 本条**走真实路径**：夹具里那篇「答记者问」的标题真的推不出期次。
	assert.NotEmpty(t, out.String(), "账对不上时更要出报告——此前这条路径上 stdout 是 0 字节")
	assert.Contains(t, out.String(), "央行有关负责人就某事答记者问",
		"必须逐条打印标题**原文**：运维要拿它回去对 manifest，只给「有 1 条」这个数字没法查")
}

// TestMergeConflictFailsTheRun：字段冲突必须让整趟失败（M1c-3b 的 TASK-006，W-4）。
//
// mergeGroup 里那段注释早就写着「不做静默取值 …… 一旦出现就说明字段归属表错了，
// 那是必须响亮失败的事」——**而实现只是记进 Conflicts 然后退出码 0**。
// 注释承诺的与代码做的不是一回事，而读注释的人会以为已经防住了。
//
// ⚠️ 本条同时钉住**报告仍要出**：冲突明细只在报告里，直接 return 等于让人看不见冲突在哪。
func TestMergeConflictFailsTheRun(t *testing.T) {
	res := &BackfillLoadResult{}
	res.Conflicts = append(res.Conflicts, MergeConflict{
		Period: "2025-08", PeriodType: "monthly", Field: FieldM2,
		A: 1, B: 2, FromA: "id-a", FromB: "id-b"})

	// ① 冲突必须变成一个 error。
	err := conflictError(res)
	require.Error(t, err, "冲突非空 ⇒ 必须失败。此前它只被记录，退出码 0")
	assert.Contains(t, err.Error(), "1 处字段冲突")

	// ② 阴性对照：没有冲突时不得凭空报错。缺这一半，实现可以恒返回 error 而本条照绿。
	assert.NoError(t, conflictError(&BackfillLoadResult{}))

	// ③ 报告仍要出（C-2 同一条原则）：冲突明细只在报告里，直接 return 等于让人看不见冲突在哪。
	var b bytes.Buffer
	require.NoError(t, writeLoadReport(&b, "/x", res),
		"空 result 的四道恒等式本就成立（0 == 0+0），此处只验报告内容")
	assert.Contains(t, b.String(), "字段冲突（预期 0，共 1）")
	assert.Contains(t, b.String(), FieldM2, "冲突字段名必须逐条打印")
}

// TestPartOverlapsAreEmptyOnRealFamilies：三族必填集互不相交这个**前提**要被钉住
// （M1c-3b 的 TASK-006，W-6）。
//
// 🔴 本条不改合并键（人类裁决过），只保证前提破掉时**有东西会响**。
// 此前这个前提由语料守着、不由代码守着 —— 新增一个 extractor 让两族必填集相交时，
// 症状会表现成「字段冲突突然出现」，而排查的人会去查语料，不会想到是归属表变了。
func TestPartOverlapsAreEmptyOnRealFamilies(t *testing.T) {
	// 🔴 **参照组合必须是真语料上真实出现的那一个**：merged@v1 的 42 个组，
	// parts 恒为 [rule-monthly@v1, tsf-stock@v1, tsf-flow@v1] 这一族形态。
	assert.Empty(t, overlappingRequiredFields(
		[]string{extractorMonthlyV1, extractorTSFStock, extractorTSFFlow}),
		"v1 时代的三族必填集必须互不相交——相交则字段冲突那一节的含义就变了")

	// 🔴 **v2 系与社融两族是相交的**（M1c-3b 的 TASK-006 实测，本条是发现不是回归）：
	// rule-monthly@v2 必填 52 个、rule@v2 必填 54 个，**都已包含社融字段** ——
	// 央行 2025-10 起把社融并进月报，月报的必填集随之吞下了那 27 个字段。
	//
	// ⇒ 「三族字段集设计上不相交」这句话**只对 v1 时代成立**，而 mergeGroup 的注释
	// 把它写成了无条件的。今天不出事，是因为并篇后每个发布事件只剩一篇 ⇒
	// v2 月报不会与社融篇进同一个合并组。
	// ⚠️ **过渡月双发会让它们相遇**（与 M1c-3b 的 TASK-008 的 R9 是同一个场景），
	// 那时字段冲突会成片出现，而排查的人会去查语料 —— W-6 的告警就是为这一刻准备的。
	assert.NotEmpty(t, overlappingRequiredFields(
		[]string{extractorMonthlyV2, extractorTSFStock}),
		"并篇后的月报必填集已含社融字段——这条钉住的是「前提有时代性」，不是回归")

	// 阴性对照：同一个 extractor 出现两次 ⇒ 它的必填字段全部被要求两次。
	// 没有这一半，上面那条在 overlappingRequiredFields 恒返回 nil 时照样绿。
	dup := overlappingRequiredFields([]string{extractorTSFStock, extractorTSFStock})
	assert.NotEmpty(t, dup, "同一族出现两次必然相交——本条证明判据不是恒空")
	// ⚠️ 用 ElementsMatch 而不是 Equal：overlappingRequiredFields 按 **fieldOrder** 排序，
	// 而 requiredFields 有自己的顺序（tsf_stock/tsf_stock_yoy 在它那里排末尾、在 fieldOrder
	// 里排最前）。两者**元素相同、顺序不同**，Equal 必然红 —— 这与 M1c-3b 的 TASK-002
	// 撞的是同一个坑（`require.Equal(tsfStockFields(), got)` 与「按 fieldOrder 排序」
	// 不可能同时满足），记在这里免得下一个人再撞。
	assert.ElementsMatch(t, requiredFields(extractorTSFStock), dup,
		"相交集应恰好是它自己的必填集（只比元素，顺序另断）")
	assert.True(t, slices.IsSortedFunc(dup, func(a, b string) int {
		return slices.Index(fieldOrder, a) - slices.Index(fieldOrder, b)
	}), "输出必须按 fieldOrder 升序——顺序会飘的清单让逐次 diff 失效")
}

// TestLoadIdentityThreeIsCrossSourced：恒等式三必须**异源**，不能是自洽求和
// （M1c-3b 的 TASK-006，C-4）。
//
// # 这条断言存在的理由，以及它杀的是哪一个变异
//
// 旧判据 `Merged == SingleArticle + MergedGroups` **恒真**：`Merged = len(groups)`，
// 而循环里每组必增且只增那两个计数器之一。它抓不到的正是**生产代码里分类写错**的情形：
// 把 `len(g.SourceIDs) > 1` 改成 `>= 1`（QA 的 M17），全套测试 rc=0、真语料 exit=0，
// 报告打印「单篇 0 + 合并组 96」这个假数，而「四道恒等式全部成立 ✓」照常打印。
//
// 🔴 **本条用真实入库路径产生两个数**：MergedGroups 来自内存里的分组结构，
// DBMergedRows 来自库里 `select count(*) ... where extractor = 'merged@v1'`。
// 两条路径独立 ⇒ M17 只改分类那一处时，两边必然对不上。
//
// ⚠️ **夹具必须至少含一个单篇组**：`>1` 与 `>=1` 只在 `SourceIDs == 1` 的组上取值不同，
// 全是多篇组时两者同值、变异存活。本夹具两篇期次不同 ⇒ 两个单篇组，正好落在差异点上。
//
// ⚠️ 多篇组那一侧由真语料回填覆盖（42 个多篇组），写不成单元测试：合并需要同期三族快照，
// 而 testdata 里没有配套的社融存量/增量样本。**此处如实记明覆盖边界，不假装测了。**
func TestLoadIdentityThreeIsCrossSourced(t *testing.T) {
	// 2025-08 月报单独一篇 ⇒ 单篇组；2025-09 月报 + 社融存量同期 ⇒ 多篇组。
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2025-08",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "a-08", Title: "2025年8月金融统计数据报告", File: "articles/a08.html",
				SHA256: testdataSHA(t, "pboc-2025-08-monthly.html")},
			{ID: "a-09", Title: "2025年9月金融统计数据报告", File: "articles/a09.html",
				SHA256: testdataSHA(t, "pboc-2025-09-q3.html")},
		},
	}, map[string]string{
		"articles/a08.html": "pboc-2025-08-monthly.html",
		"articles/a09.html": "pboc-2025-09-q3.html",
	})

	var out bytes.Buffer
	res, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: dir, DBPath: filepath.Join(t.TempDir(), "x.db"),
		Cfg: DefaultThresholds(), Out: &out, AllowIncomplete: true})
	require.NoError(t, err)

	require.True(t, res.MergedRowsCounted,
		"全部入库成功时异源计数必须真的测过——没测过时那道闸被跳过，而**跳过不会红**")
	assert.Equal(t, res.MergedGroups, res.DBMergedRows,
		"分组计数与库里 merged@v1 行数必须相等；不等说明分类逻辑与入库结果分叉了")

	// 钉具体值：只断言「两边相等」的话，两边同时为 0 也满足。
	assert.Equal(t, 0, res.MergedGroups, "本夹具两篇期次不同 ⇒ 没有多篇组")
	assert.Equal(t, 2, res.SingleArticle, "两个单篇组")
	assert.Equal(t, 2, res.Merged)
}

// TestBackfillLoadRejectsBrokenManifest：manifest 坏了要原样报出，不要当成「零篇」。
func TestBackfillLoadRejectsBrokenManifest(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifestFileName), []byte("{ not json"), 0o600))

	_, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: dir, DBPath: filepath.Join(t.TempDir(), "b.db"),
		Cfg: DefaultThresholds(), Out: io.Discard, AllowIncomplete: true})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "恒等式",
		"该在载入阶段就失败，而不是拿一份零篇的 manifest 一路走到恒等式那里")
}

// TestBackfillLoadReportsUnstatableDBPath：--db 指到一个**普通文件底下**时要原样报出。
//
// 这条路径与「文件已存在」不同：os.Stat 返回的既不是 nil 也不是 NotExist（是 ENOTDIR），
// 若只判这两支，第三支会落进「不存在 ⇒ 继续」然后在 NewStore 里以另一个理由失败，
// 而那个理由说的是别的事。真实成因就是路径里打错一段。
func TestBackfillLoadReportsUnstatableDBPath(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a-regular-file")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))

	_, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: loadFixture(t), DBPath: filepath.Join(f, "nested.db"),
		Cfg: DefaultThresholds(), Out: io.Discard, AllowIncomplete: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "查看", "要说清是「查看这个路径就失败了」，不是「它已存在」")
	assert.NotContains(t, err.Error(), "已存在")
}

// TestPendingReasonFallsBackWhenNoFailedCheck：防御性分支也要说得出话。
//
// 当前 Save 只在 !rep.Passed 时落 pending，而 passed 只由 CheckFailed 翻转 ⇒ 走到这里
// 时理论上必有一条 failed。但「理论上必有」是**别人的实现细节**：validate.go 若将来
// 让 passed 因别的原因翻转，这里会拿到一份没有 failed 的报告。返回空串会让报告里
// 那一列是空白，而空白读起来像「没查出问题」——恰好与事实相反。
func TestPendingReasonFallsBackWhenNoFailedCheck(t *testing.T) {
	assert.Contains(t, pendingReason(ValidationReport{Checks: []Check{
		{ID: "completeness", Status: CheckPassed},
		{ID: "yoy_sanity", Status: CheckSkipped, Reason: "no_prior_period"},
	}}), "未过闸", "没有 failed 明细时也要给一句话，不能返回空串")

	assert.Contains(t, pendingReason(ValidationReport{Checks: []Check{
		{ID: "yoy_sanity", Status: CheckSkipped, Reason: "no_prior_period"},
		{ID: "completeness", Status: CheckFailed, Reason: "missing 3: a,b,c"},
	}}), "completeness", "有 failed 时要指名是哪道闸，skipped 的不算")
}

// TestBackfillLoadCollectsPerPeriodErrors：单期失败**不中断整批**（boundary[2]）。
//
// 判据是「两期的错都在」，不是「有错」：中断实现同样会返回非 nil error，只是里面
// 只有第一期。两者在「err != nil」这个粒度上完全同形 —— 必须逐个期次核。
func TestBackfillLoadCollectsPerPeriodErrors(t *testing.T) {
	// 倒置区间会让 Validate 直接返回 error（配置错，不是数据错）⇒ 每一期都在同一处失败。
	cfg := DefaultThresholds()
	cfg.MagnitudeRanges = map[string]Range{FieldM2: {Min: 1, Max: -1, Unit: "万亿元"}}

	dir := writeCalibrateFixture(t, Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "a2025", Title: "2025年金融统计数据报告", File: "articles/a2025.html",
				SHA256: testdataSHA(t, "pboc-2025-12-annual.html")},
			{ID: "a2020", Title: "2020年上半年金融统计数据报告", File: "articles/a2020.html",
				SHA256: testdataSHA(t, "pboc-2020-06-h1.html")},
		},
	}, map[string]string{
		"articles/a2025.html": "pboc-2025-12-annual.html",
		"articles/a2020.html": "pboc-2020-06-h1.html",
	})

	var out bytes.Buffer
	res, err := BackfillLoad(context.Background(), BackfillLoadDeps{
		Dir: dir, DBPath: filepath.Join(t.TempDir(), "j.db"),
		Cfg: cfg, Out: &out, AllowIncomplete: true})
	require.Error(t, err)
	require.NotNil(t, res)

	assert.Equal(t, 2, res.Merged, "两期都该被处理到，不是在第一期就停下")
	assert.Contains(t, err.Error(), "2025-12/annual")
	assert.Contains(t, err.Error(), "2020-06/h1",
		"第二期的错也要在——只有第一期的错说明整批被中断了")
}
