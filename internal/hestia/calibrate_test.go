package hestia

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping（TASK-002）
//
//	functional[0]      collectSamples 签名/类型、按期次归组、只解析金融统计报告、汇总 Samples
//	                   → TestCollectSamplesCountsDifferPerField
//	functional[1]      社融存量/增量两篇不计入失败表（文件根本不存在）
//	                   → TestCollectSamplesIgnoresNonFinanceKinds
//	functional[2]      三类（本迭代不解析 / 真失败 / 解析不出期次）各有一格、计数分别可见
//	                   → TestCollectSamplesSeparatesThreeCategories
//	                   → TestCollectSamplesRendersEveryCategoryToOut
//	boundary[0]        CompletedAt 缺失 ⇒ 报错；AllowIncomplete ⇒ 放行
//	                   → TestCollectSamplesRejectsIncompleteManifest
//	                   → TestCollectSamplesAllowsIncompleteWhenAsked
//	                   三条错误路径的 want 互不相同（换成另一条的 want 必须变红）
//	                   → TestCollectSamplesErrorPathsAreDistinguishable
//	                   「空」的判据是可用样本数为 0，不是 len(Articles)==0
//	                   → TestCollectSamplesRejectsZeroUsableSamples
//	boundary[1]        Dir 不存在 / manifest 不可读 ⇒ 报错，不产出空结果
//	                   → TestCollectSamplesErrorPathsAreDistinguishable（missing 一格）
//	                   → TestCollectSamplesRejectsUnreadableManifest
//	                   → TestCollectSamplesRejectsUnstatableDir
//	error_handling[0]  单篇失败 ⇒ 记 Failures 并继续；四字段都填；断言 Err 内容
//	                   → TestCollectSamplesRecordsMissingFileAndContinues
//	                   → TestCollectSamplesRecordsParseFailure
//	                   → TestCollectSamplesFailuresCarryDistinctReasons
//	error_handling[1]  manifest 的四个可信度字段被消费 + 期次交叉校验
//	                   → TestCollectSamplesSurfacesSearchSkippedReason（SearchSkippedReason）
//	                   → TestCollectSamplesSurfacesFetchFailed（Manifest.Failed）
//	                   → TestCollectSamplesSurfacesMissingPeriods（Reconcile.MissingPeriods）
//	                   → TestCollectSamplesVerifiesArticleSHA256（Article.SHA256，含缺失时出声）
//	                   → TestCollectSamplesCrossChecksPeriodAgainstTitle（免费的交叉校验）
//	                   → TestCollectSamplesDoesNotPrejudgeUnparseableTitles
//	                     （单靠标题正则判归属会静默吞掉分行报告，见 backfill_scan.go 的订正）

// writeCalibrateFixture 用 testdata 里的真实报告拼一个 manifest 目录。
//
// files 的键是**相对 dir** 的落盘路径（与 Article.File 同一口径），值是 testdata 下的源文件名。
func writeCalibrateFixture(t *testing.T, m Manifest, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, articlesDirName), 0o755))

	for rel, src := range files {
		raw, err := os.ReadFile(filepath.Join("testdata", src))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), raw, 0o600))
	}

	raw, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifestFileName), raw, 0o600))
	return dir
}

// testdataSHA 返回 testdata 下某份快照的 sha256，用来把夹具的 manifest 填成
// **与真实产物同形**（真跑的 218 条每条都有 sha256）。
func testdataSHA(t *testing.T, src string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", src))
	require.NoError(t, err)
	return articleSHA256(raw)
}

// completedFixture 是一份正常完成的 manifest，含两期真实报告。
func completedFixture(t *testing.T) string {
	t.Helper()
	return writeCalibrateFixture(t, Manifest{
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
}

// —— functional[0] ——

// 两期真实报告：2025 年报 54 个字段、2020 上半年 27 个。
//
// 社融字段只出现在前者 ⇒ 它们的样本数必然小于非社融字段。这个差异是 M1c-2 的核心事实
// （v1 期次的社融在另外两篇里，本迭代没有解析器），T3 报告的 n 列专门为它而设。
func TestCollectSamplesCountsDifferPerField(t *testing.T) {
	got, err := collectSamples(CalibrateDeps{Dir: completedFixture(t)})
	require.NoError(t, err)

	require.Equal(t, 2, got.Periods)
	assert.Empty(t, got.Failures)
	assert.Empty(t, got.Unsupported)
	assert.Empty(t, got.Unclassified)

	assert.Len(t, got.Samples[FieldM2], 2, "非社融字段两期都有")
	assert.Len(t, got.Samples[FieldTSFStock], 1, "社融字段只有 rule@v2 那期有")
}

// —— functional[1] ——

// 社融存量/增量两篇不计入失败表。
//
// 构造是刻意的：那两篇的**文件根本不存在**。若实现把它们当成待解析的，会产生两条
// 「读文件失败」的假失败 —— 而 v1 期次每期两篇，68 期就是 136 条假失败，M1c-4 的
// 工作量凭空翻倍。
func TestCollectSamplesIgnoresNonFinanceKinds(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "a1", Title: "2025年金融统计数据报告", File: "articles/a1.html"},
			{ID: "a2", Title: "2020年3月社会融资规模存量统计数据报告", File: "articles/nope-stock.html"},
			{ID: "a3", Title: "2020年一季度社会融资规模增量统计数据报告", File: "articles/nope-flow.html"},
		},
	}, map[string]string{"articles/a1.html": "pboc-2025-12-annual.html"})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err)

	assert.Equal(t, 1, got.Periods, "只数金融统计数据报告")
	assert.Empty(t, got.Failures,
		"社融两篇的文件根本不存在，若把它们当成待解析的就会产生两条假失败")

	require.Len(t, got.Unsupported, 2, "但它们也不能消失——进「本迭代不解析」那一格")
	kinds := []string{got.Unsupported[0].Kind, got.Unsupported[1].Kind}
	assert.ElementsMatch(t, []string{backfillKindStock, backfillKindFlow}, kinds)
	for _, u := range got.Unsupported {
		assert.NotEmpty(t, u.Period, "期次要填，否则看不出是哪一期没解析")
		assert.NotEmpty(t, u.Err, "要写明为什么没解析")
	}
}

// —— functional[2] ——

// threeCategoryFixture 一次覆盖三类跳过/失败成因，外加一期正常样本。
//
// 🔴 monthly 那篇的**文件同样不存在**：`parse.go` 的 checkPeriodTypeSupported 对 monthly
// 显式返回 error（TestParseRejectsMonthlyUntilSampled 把这个决定钉成了契约），所以它与
// 社融两篇同类——本迭代不解析。若实现把它当成待解析的，真语料上会产生**约 53 条**
// 同一句 "monthly is not supported yet" 的假失败，把真失败淹没。
func threeCategoryFixture(t *testing.T) string {
	t.Helper()
	return writeCalibrateFixture(t, Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html",
				SHA256: testdataSHA(t, "pboc-2025-12-annual.html")},
			// 类 1：本迭代不解析（monthly + 社融两篇），三篇的文件都不存在
			{ID: "m", Title: "2026年7月金融统计数据报告", File: "articles/nope-m.html"},
			{ID: "s", Title: "2020年3月社会融资规模存量统计数据报告", File: "articles/nope-stock.html"},
			{ID: "f", Title: "2020年一季度社会融资规模增量统计数据报告", File: "articles/nope-flow.html"},
			// 类 2：该支持却失败了 —— 正文给一个 index 页，Parse 必然失败
			{ID: "bad", Title: "2024年金融统计数据报告", File: "articles/bad.html",
				SHA256: testdataSHA(t, "pboc-index-p1.html")},
			// 类 3：标题解析不出期次（央行只发「一季度」，「三季度」是站点表述变了）
			{ID: "u", Title: "2024年三季度金融统计数据报告", File: "articles/u.html"},
		},
	}, map[string]string{
		"articles/ok.html":  "pboc-2025-12-annual.html",
		"articles/bad.html": "pboc-index-p1.html",
	})
}

// 三类必须可分，且**各类的计数分别可见** —— 只断「有 N 条失败」的话，把三类混成一堆的
// 实现照样绿，而那正是要防的形态。
func TestCollectSamplesSeparatesThreeCategories(t *testing.T) {
	got, err := collectSamples(CalibrateDeps{Dir: threeCategoryFixture(t)})
	require.NoError(t, err)

	// 类 1：本迭代不解析 —— 3 篇，且**没有一篇被读过文件**（它们的文件都不存在，
	// 若被读过就会变成「读文件失败」跑进 Failures）
	require.Len(t, got.Unsupported, 3)
	var monthly int
	for _, u := range got.Unsupported {
		assert.NotContains(t, u.Err, "读文件", "不解析的篇目不该被读")
		if strings.Contains(u.Err, "monthly") {
			monthly++
		}
	}
	assert.Equal(t, 1, monthly, "monthly 与社融两篇同类，但理由要写得出来")

	// 类 2：该支持却失败了 —— 只有 index 页那一篇
	require.Len(t, got.Failures, 1)
	assert.Equal(t, "2024-12", got.Failures[0].Period)

	// 类 3：标题解析不出期次 —— 原文进 Unclassified，不得 continue 丢弃
	assert.Equal(t, []string{"2024年三季度金融统计数据报告"}, got.Unclassified)

	// 正常那期照常出样本
	assert.Len(t, got.Samples[FieldM2], 1)
	assert.Equal(t, 2, got.Periods, "Periods 只数受支持的金融统计报告（含失败的那篇）")
}

// 三类都要**渲染出来**：一个只把它们记在结构体里、不写给人看的实现，与「静默消失」
// 在终端上没有区别（backfill_reconcile.go:196 对同一处写过这条理由）。
func TestCollectSamplesRendersEveryCategoryToOut(t *testing.T) {
	var out bytes.Buffer
	_, err := collectSamples(CalibrateDeps{Dir: threeCategoryFixture(t), Out: &out})
	require.NoError(t, err)

	s := out.String()
	assert.Contains(t, s, "2024年三季度金融统计数据报告", "解析不出期次的标题原文必须出现")
	assert.Contains(t, s, "articles/bad.html", "失败要定位到文件")
	assert.Contains(t, s, backfillKindStock, "不解析的种类要写出来")
	// ⚠️ 夹具里 monthly 那篇的文件名**刻意不含 "monthly"**（articles/nope-m.html）：
	// 叫 nope-monthly.html 的话，即使实现把它错记成「读文件失败」，这条断言也会被
	// 失败行里的文件名平凡满足 —— 消融实测过，那时本用例不红。
	assert.Contains(t, s, "monthly", "不解析的 period_type 要写出来")
}

// —— boundary[0] ——

// CompletedAt 缺失 ⇒ 报错。
//
// M1c-1 的注释写明：进程在第 218 篇被杀与跑完 400 篇正常退出，产出的 manifest
// **结构上无法区分**，下游做的一切闭合性检查在夭折的产物上同样全绿。⇒ 判据只能是
// 那个字段在不在，不能用「产物内部自洽」去替代。
func TestCollectSamplesRejectsIncompleteManifest(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From: "2020-01", // 没有 CompletedAt
		Articles: []Article{
			{ID: "a2025", Title: "2025年金融统计数据报告", File: "articles/a2025.html"},
		},
	}, map[string]string{"articles/a2025.html": "pboc-2025-12-annual.html"})

	_, err := collectSamples(CalibrateDeps{Dir: dir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "completed_at")
	assert.Contains(t, err.Error(), "allow-incomplete", "错误信息要说清怎么绕过")
}

// 显式放行时可以用缺标记的 manifest —— 那是人的决定，不是默认行为。
func TestCollectSamplesAllowsIncompleteWhenAsked(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From: "2020-01",
		Articles: []Article{
			{ID: "a2025", Title: "2025年金融统计数据报告", File: "articles/a2025.html"},
		},
	}, map[string]string{"articles/a2025.html": "pboc-2025-12-annual.html"})

	got, err := collectSamples(CalibrateDeps{Dir: dir, AllowIncomplete: true})
	require.NoError(t, err)
	assert.Equal(t, 1, got.Periods)
}

// 三条错误路径的 want 必须互不相同 —— 计划原来的空-manifest 用例断言 `Contains(err, "没有")`，
// 而 CompletedAt 的错误里也含「没有」⇒ 同一个 want 命中两条完全不同的失败。
//
// 本用例把「把任一条的 want 换成另一条的，该用例必须变红」机制化：正向断言自己的 want
// 命中，反向断言**其余每一条**的 want 都不命中。
func TestCollectSamplesErrorPathsAreDistinguishable(t *testing.T) {
	cases := []struct {
		name string
		dir  func(t *testing.T) string
		want string
	}{
		{
			name: "缺 completed_at",
			dir: func(t *testing.T) string {
				return writeCalibrateFixture(t, Manifest{
					From: "2020-01",
					Articles: []Article{
						{ID: "a", Title: "2025年金融统计数据报告", File: "articles/a.html"},
					},
				}, map[string]string{"articles/a.html": "pboc-2025-12-annual.html"})
			},
			want: "completed_at",
		},
		{
			name: "目录或 manifest 不存在",
			dir:  func(t *testing.T) string { return filepath.Join(t.TempDir(), "nope") },
			want: "manifest.json 不存在",
		},
		{
			name: "有 manifest 但可用样本为 0",
			dir: func(t *testing.T) string {
				return writeCalibrateFixture(t, Manifest{
					From:        "2020-01",
					CompletedAt: "2026-08-24T10:00:00Z",
					Articles: []Article{
						{ID: "s", Title: "2020年3月社会融资规模存量统计数据报告", File: "articles/s.html"},
						{ID: "f", Title: "2020年一季度社会融资规模增量统计数据报告", File: "articles/f.html"},
					},
				}, nil)
			},
			want: "可用样本为 0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := collectSamples(CalibrateDeps{Dir: tc.dir(t)})
			require.Error(t, err)
			assert.Nil(t, res, "报错时不产出结果，免得调用方拿一份空的当真")
			assert.Contains(t, err.Error(), tc.want)

			for _, other := range cases {
				if other.name == tc.name {
					continue
				}
				assert.NotContains(t, err.Error(), other.want,
					"「%s」的错误串命中了「%s」的 want ⇒ 两条路径分不开", tc.name, other.name)
			}
		})
	}
}

// 「空」的判据是**可用样本数为 0**，不是 len(Articles)==0：一个含 400 篇却一个样本都
// 产不出的目录，若放行就会打印 54 行全 `—`、退出码 0。
//
// 本例的 400 篇全部解析失败（文件都不存在）—— 失败表非空、样本仍为 0，同样必须拒。
func TestCollectSamplesRejectsZeroUsableSamples(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "g1", Title: "2024年金融统计数据报告", File: "articles/g1.html"},
			{ID: "g2", Title: "2025年金融统计数据报告", File: "articles/g2.html"},
		},
	}, nil)

	res, err := collectSamples(CalibrateDeps{Dir: dir})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "可用样本为 0")
	assert.Contains(t, err.Error(), "解析失败 2", "错误信息要带上各格的计数，否则看不出样本去哪了")
}

// —— boundary[1] ——

// manifest 存在但读不出来 ⇒ 报错。
//
// loadManifest 对**文件不存在**返回空 Manifest + nil error（那是回填首跑的正常路径），
// 所以标定必须自己先确认它在；而内容坏掉时 loadManifest 会报错，这里确认那条错误
// 被原样带出，不被吞成一份空结果。
func TestCollectSamplesRejectsUnreadableManifest(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifestFileName), []byte("{不是 JSON"), 0o600))

	res, err := collectSamples(CalibrateDeps{Dir: dir})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), manifestFileName)
}

// —— error_handling[0] ——

// 快照文件不存在 ⇒ 记失败并继续，不中断整批。
func TestCollectSamplesRecordsMissingFileAndContinues(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "gone", Title: "2024年金融统计数据报告", File: "articles/gone.html"},
			{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html"},
		},
	}, map[string]string{"articles/ok.html": "pboc-2025-12-annual.html"})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err, "单期失败不该让整次标定失败")

	require.Len(t, got.Failures, 1)
	f := got.Failures[0]
	assert.Equal(t, "2024-12", f.Period)
	assert.Equal(t, backfillKindFinance, f.Kind)
	assert.Equal(t, "articles/gone.html", f.File)
	assert.Contains(t, f.Err, "读文件")
	assert.NotEmpty(t, got.Samples[FieldM2], "好的那期照常收集")
}

// 解析失败进失败表，并**原样带上 Parse 的错误**。
//
// 断言的是 Err 的内容而不是「有一条失败」：只断条数的话，一个把所有失败都写成同一句话
// 的实现照样绿。期望值不写死措辞，而是现场调一次 Parse 取 —— 措辞改了这里跟着走，
// 但「有没有原样带出来」这件事守得住。
func TestCollectSamplesRecordsParseFailure(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			// 正文给一个 index 页 —— 不是报告，Parse 必然失败
			{ID: "bad", Title: "2024年金融统计数据报告", File: "articles/bad.html"},
			{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html"},
		},
	}, map[string]string{
		"articles/bad.html": "pboc-index-p1.html",
		"articles/ok.html":  "pboc-2025-12-annual.html",
	})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join("testdata", "pboc-index-p1.html"))
	require.NoError(t, err)
	_, wantErr := Parse(raw)
	require.Error(t, wantErr)

	require.Len(t, got.Failures, 1)
	f := got.Failures[0]
	assert.Equal(t, "2024-12", f.Period)
	assert.Equal(t, backfillKindFinance, f.Kind)
	assert.Equal(t, "articles/bad.html", f.File, "要能定位到具体文件")
	assert.Equal(t, wantErr.Error(), f.Err, "Parse 的错误要原样带出，不能换成一句通用话")
}

// 两种成因的失败，Err 必须不同 —— 这是「一个把所有失败都写成同一句话的实现照样绿」
// 的直接反例。
func TestCollectSamplesFailuresCarryDistinctReasons(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "gone", Title: "2023年金融统计数据报告", File: "articles/gone.html"},
			{ID: "bad", Title: "2024年金融统计数据报告", File: "articles/bad.html"},
			{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html"},
		},
	}, map[string]string{
		"articles/bad.html": "pboc-index-p1.html",
		"articles/ok.html":  "pboc-2025-12-annual.html",
	})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err)

	require.Len(t, got.Failures, 2)
	assert.NotEqual(t, got.Failures[0].Err, got.Failures[1].Err)
	assert.Equal(t, []string{"2023-12", "2024-12"}, []string{got.Failures[0].Period, got.Failures[1].Period},
		"失败清单按期次排序，才能逐次 diff")
}

// —— error_handling[1]：manifest 里判断「这份产物可不可信」的四个字段 ——

// SearchSkippedReason 非空 ⇒ 这一轮**根本没做**交叉校验。
// manifest 注释原话：「没有这个字段时『这次没做校验』与『校验通过』在他们看来完全一样」
// —— 那个「他们」就是本迭代。
func TestCollectSamplesSurfacesSearchSkippedReason(t *testing.T) {
	m := Manifest{
		From:                "2020-01",
		CompletedAt:         "2026-08-24T10:00:00Z",
		SearchSkippedReason: "搜索接口连续 3 页返回 0 条",
		Articles: []Article{
			{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html"},
		},
	}
	dir := writeCalibrateFixture(t, m, map[string]string{"articles/ok.html": "pboc-2025-12-annual.html"})

	var out bytes.Buffer
	got, err := collectSamples(CalibrateDeps{Dir: dir, Out: &out})
	require.NoError(t, err)

	assert.Contains(t, strings.Join(got.Warnings, "\n"), "搜索接口连续 3 页返回 0 条")
	assert.Contains(t, out.String(), "搜索接口连续 3 页返回 0 条", "看终端的人也要看得见")

	// 阴性对照：没有这个字段时不该凭空报警，否则告警会退化成背景噪音。
	clean := m
	clean.SearchSkippedReason = ""
	got2, err := collectSamples(CalibrateDeps{Dir: writeCalibrateFixture(t, clean,
		map[string]string{"articles/ok.html": "pboc-2025-12-annual.html"})})
	require.NoError(t, err)
	assert.NotContains(t, strings.Join(got2.Warnings, "\n"), "交叉校验")
}

// Manifest.Failed 是 fetch 阶段没抓到的篇目：它们既不在 Articles 也不在解析失败表，
// 不读就会让报告显示「失败：无」—— 而失败表的用途正是「M1c-3 入库前要清零」。
func TestCollectSamplesSurfacesFetchFailed(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html"},
		},
		Failed: []Failed{
			{ID: "2021xxxx", URL: "https://example.invalid/a", Error: "502 Bad Gateway"},
		},
	}, map[string]string{"articles/ok.html": "pboc-2025-12-annual.html"})

	var out bytes.Buffer
	got, err := collectSamples(CalibrateDeps{Dir: dir, Out: &out})
	require.NoError(t, err)

	require.Len(t, got.FetchFailed, 1, "fetch 阶段的失败要带进结果，下游报告才列得出来")
	assert.Equal(t, "2021xxxx", got.FetchFailed[0].ID)
	assert.Contains(t, out.String(), "2021xxxx")
}

// Reconcile.MissingPeriods：不读它，就看不出这份分布是算在一条**有洞的序列**上的。
func TestCollectSamplesSurfacesMissingPeriods(t *testing.T) {
	base := Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html"},
		},
	}
	files := map[string]string{"articles/ok.html": "pboc-2025-12-annual.html"}

	t.Run("有缺篇", func(t *testing.T) {
		m := base
		m.Reconcile = &ManifestReconcile{Periods: 80, Articles: 218, MissingPeriods: []string{"2021-03", "2021-06"}}
		var out bytes.Buffer
		got, err := collectSamples(CalibrateDeps{Dir: writeCalibrateFixture(t, m, files), Out: &out})
		require.NoError(t, err)
		w := strings.Join(got.Warnings, "\n")
		assert.Contains(t, w, "2021-03")
		assert.Contains(t, w, "2021-06")
		assert.Contains(t, out.String(), "2021-03")
	})

	t.Run("没有对账摘要也要出声", func(t *testing.T) {
		// 真跑用的那份产物出自 TASK-010 之前，**没有** reconcile 字段。静默略过会让
		// 「序列没有洞」与「压根没对过账」看起来一样 —— 与 SearchSkippedReason 同一失效。
		got, err := collectSamples(CalibrateDeps{Dir: writeCalibrateFixture(t, base, files)})
		require.NoError(t, err)
		assert.Contains(t, strings.Join(got.Warnings, "\n"), "reconcile")
	})

	t.Run("对过账且没有洞则不报警", func(t *testing.T) {
		m := base
		m.Reconcile = &ManifestReconcile{Periods: 80, Articles: 218}
		got, err := collectSamples(CalibrateDeps{Dir: writeCalibrateFixture(t, m, files)})
		require.NoError(t, err)
		assert.NotContains(t, strings.Join(got.Warnings, "\n"), "缺篇")
	})
}

// Article.SHA256：「让下游验证本地文件未被篡改/截断」，而 calibrate 是第一个下游。
// 被截断的 HTML 可能仍 Parse 成功但少抽字段 ⇒ 该期静默贡献残缺样本。
func TestCollectSamplesVerifiesArticleSHA256(t *testing.T) {
	t.Run("不符则记失败且不贡献样本", func(t *testing.T) {
		dir := writeCalibrateFixture(t, Manifest{
			From:        "2020-01",
			CompletedAt: "2026-08-24T10:00:00Z",
			Articles: []Article{
				// sha256 填的是**另一份**报告的，模拟文件被换/被截断
				{ID: "tampered", Title: "2020年上半年金融统计数据报告", File: "articles/t.html",
					SHA256: testdataSHA(t, "pboc-2025-12-annual.html")},
				{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html",
					SHA256: testdataSHA(t, "pboc-2025-12-annual.html")},
			},
		}, map[string]string{
			"articles/t.html":  "pboc-2020-06-h1.html",
			"articles/ok.html": "pboc-2025-12-annual.html",
		})

		got, err := collectSamples(CalibrateDeps{Dir: dir})
		require.NoError(t, err)

		require.Len(t, got.Failures, 1)
		assert.Equal(t, "2020-06", got.Failures[0].Period)
		assert.Contains(t, got.Failures[0].Err, "sha256")
		assert.Len(t, got.Samples[FieldM2], 1, "校验不过的那期一个样本都不能贡献")
	})

	t.Run("字段缺失时出声跳过", func(t *testing.T) {
		dir := writeCalibrateFixture(t, Manifest{
			From:        "2020-01",
			CompletedAt: "2026-08-24T10:00:00Z",
			Articles: []Article{
				{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html"}, // 没有 SHA256
			},
		}, map[string]string{"articles/ok.html": "pboc-2025-12-annual.html"})

		got, err := collectSamples(CalibrateDeps{Dir: dir})
		require.NoError(t, err)
		assert.Empty(t, got.Failures, "缺字段不是文件坏了")
		assert.Contains(t, strings.Join(got.Warnings, "\n"), "sha256",
			"跳过校验必须出声，否则「校验过了」与「没校验」看起来一样")
	})
}

// 免费的交叉校验：Parse 从 HTML 自解 period，与 backfillPeriodOf(a.Title) 是两条独立推导。
// manifest 与文件错配时，没有这条则样本**静默来自错误期次**。
func TestCollectSamplesCrossChecksPeriodAgainstTitle(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			// 标题说 2024 年报，正文却是 2025 年报
			{ID: "mismatch", Title: "2024年金融统计数据报告", File: "articles/m.html"},
			{ID: "ok", Title: "2020年上半年金融统计数据报告", File: "articles/ok.html"},
		},
	}, map[string]string{
		"articles/m.html":  "pboc-2025-12-annual.html",
		"articles/ok.html": "pboc-2020-06-h1.html",
	})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err)

	require.Len(t, got.Failures, 1)
	assert.Contains(t, got.Failures[0].Err, "2024-12", "错配的两侧都要写出来")
	assert.Contains(t, got.Failures[0].Err, "2025-12")
	assert.Len(t, got.Samples[FieldM2], 1, "错配的那期不能贡献样本")
	assert.Empty(t, got.Samples[FieldTSFStock], "贡献样本的是 2020 上半年那期，它没有社融字段")
}

// manifest 所在路径压根 stat 不了（这里让 --dir 穿过一个普通文件）⇒ 也要报错。
//
// 与「不存在」分开报：`os.IsNotExist` 对 ENOTDIR 是 false，两者合并的话，一条真正的
// 文件系统错误会被说成「--dir 指错了」，排障方向正好反了。
func TestCollectSamplesRejectsUnstatableDir(t *testing.T) {
	root := t.TempDir()
	notADir := filepath.Join(root, "afile")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o600))

	res, err := collectSamples(CalibrateDeps{Dir: filepath.Join(notADir, "sub")})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "查看")
	assert.NotContains(t, err.Error(), "不存在", "ENOTDIR 不是「没这个文件」")
}

// 标题 parseTitle 认不出时**不预判**，照常读文件交给 Parse。
//
// 「山西省2024年8月金融统计数据报告」这类分行报告：backfillTitleRE 不锚定起点，会把它
// 认成 2024-08 的金融统计报告（backfill_scan.go 的订正说得很清楚——挡住分行报告的是栏目筛，
// 不是这条正则）。它的期次段是「8月」，若在这里照着 manifest 标题预判，就会被静默归进
// 「本迭代不解析 monthly」那一格 —— 一篇本不该在语料里的文章从此不再有人看。
func TestCollectSamplesDoesNotPrejudgeUnparseableTitles(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "branch", Title: "山西省2024年8月金融统计数据报告", File: "articles/branch.html"},
			{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html"},
		},
	}, map[string]string{
		"articles/branch.html": "pboc-index-p1.html",
		"articles/ok.html":     "pboc-2025-12-annual.html",
	})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err)

	assert.Empty(t, got.Unsupported, "认不出的标题不猜 period_type")
	require.Len(t, got.Failures, 1, "它要以「失败」的身份被人看见，而不是被归进不解析那一格")
	assert.Equal(t, "2024-08", got.Failures[0].Period)
}
