package hestia

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
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

	t.Run("三族齐全时不算部分覆盖", func(t *testing.T) {
		var out2 bytes.Buffer
		res2, err := BackfillLoad(context.Background(), BackfillLoadDeps{
			Dir: loadFixture(t), DBPath: filepath.Join(t.TempDir(), "q.db"),
			Cfg: DefaultThresholds(), Out: &out2, AllowIncomplete: true})
		require.NoError(t, err)
		assert.Empty(t, res2.PartialCoverage)
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
	reason := res.PendingReasons["2025-08/monthly"]
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
