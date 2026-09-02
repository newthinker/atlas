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

// Context Checkpoint: done_criteria → test mapping（M1c-2 的 TASK-002）
//
// ⚠️ **这是 M1c-2 的 TASK-002 定稿那一刻的映射，逐字保留**：它记的是那一轮的 done_criteria，
// 改了就不再是那一轮的记录。但里面有两句在今天已经不成立，**读它要带上这个前提**：
//
//   · functional[0] 的「只解析金融统计报告」—— M1c-3a 的 TASK-010 删掉 kind 硬过滤后不再成立；
//   · functional[1] 指的 TestCollectSamplesIgnoresNonFinanceKinds 已在同一任务里**转正例**
//     并改名为 TestCollectSamplesParsesNonFinanceKinds（来历见下面 functional[1] 那一段）。
//     判据是机械的：把本文件注释里引用的每个 Test 名逐个比对实际存在的函数。
//     ⚠️ **这里不写「共 N 处」**——那种自证数字下一次改动就过期，而它过期时不会报错。
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

// TestCollectSamplesParsesNonFinanceKinds —— 由 TestCollectSamplesIgnoresNonFinanceKinds
// **转正例**而来（M1c-3a 的 TASK-010）。
//
// # 来历：它此前断言的行为如今恰好相反，**别把它当过时测试删掉**
//
// 原测试断言社融两种「不被读文件、直接进本迭代不解析」，夹具因此刻意给它们**不存在的文件**
// ——那时 `classifyArticles` 硬过滤掉它们，理由写着「社融存量/增量的解析器是 M1c-3 的活」。
// **而本迭代就是 M1c-3**：解析器（M1c-3a 的 TASK-002/003/007）已经做好并接进 `Parse` 的
// 三路分派，calibrate 再不喂给它，报告里社融字段的 n 就永远停在原地。
//
// 转换后它守的东西没变——「社融两篇不得从表上消失」——只是从「不解析所以进另一格」
// 变成「照常解析，文件缺失就是真失败」。这与 M1c-3a 的 TASK-007 对月报那次转换是同一手法。
func TestCollectSamplesParsesNonFinanceKinds(t *testing.T) {
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

	assert.Equal(t, 3, got.Periods, "三种报告都进入解析流程，不再只数金融统计")
	assert.Empty(t, got.Unsupported,
		"社融两种不再是「本迭代不解析」——那句理由（解析器是 M1c-3 的活）在本迭代已过期")

	// 文件不存在 ⇒ 现在是**真失败**，而不是「不解析」。关键是它们**仍在表上**。
	require.Len(t, got.Failures, 2, "两篇社融的文件缺失是真失败，但一篇都不能消失")
	kinds := []string{got.Failures[0].Kind, got.Failures[1].Kind}
	assert.ElementsMatch(t, []string{backfillKindStock, backfillKindFlow}, kinds)
	for _, f := range got.Failures {
		assert.NotEmpty(t, f.Period, "期次要填，否则看不出是哪一期")
		assert.Contains(t, f.Err, "读文件", "理由要写明是文件读不到，不是别的")
	}
}

// —— functional[2] ——

// threeCategoryFixture 一次覆盖三类跳过/失败成因，外加一期正常样本。
//
// 🔴 **monthly 那篇已从「本迭代不解析」改为「正常样本」**（M1c-3a 的 TASK-007）。
//
// 原来它的文件刻意不存在，因为 `checkPeriodTypeSupported` 对 monthly 显式返回 error，
// 它与社融两篇同类。M1c-3a 的 TASK-007 删掉那个分支后**月报是受支持的期次**，于是这里给它
// 一份真实月报，让本夹具直接体现这一点——而不是只把「不解析」的期望值从 3 改成 2。
//
// 原注释里那句「若实现把它当成待解析的，真语料上会产生约 53 条同一句
// "monthly is not supported yet" 的假失败」**已随分支删除而失效**，留在这里是因为
// 它记录了当初为什么要让那篇不可读：**那个理由消失了，不是那个判断错了**。
func threeCategoryFixture(t *testing.T) string {
	t.Helper()
	return writeCalibrateFixture(t, Manifest{
		From:        "2020-01",
		CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html",
				SHA256: testdataSHA(t, "pboc-2025-12-annual.html")},
			// 月报（M1c-3a 的 TASK-007 起受支持）：给真实文件，走正常解析
			{ID: "m", Title: "2026年7月金融统计数据报告", File: "articles/m.html",
				SHA256: testdataSHA(t, "pboc-2026-07-monthly.html")},
			// 社融两种（M1c-3a 的 TASK-010 起受支持）：给真实文件，走正常解析
			{ID: "s", Title: "2025年8月社会融资规模存量统计数据报告", File: "articles/stock.html",
				SHA256: testdataSHA(t, "pboc-2025-08-tsf-stock.html")},
			{ID: "f", Title: "2025年8月社会融资规模增量统计数据报告", File: "articles/flow.html",
				SHA256: testdataSHA(t, "pboc-2025-08-tsf-flow.html")},
			// 类 1：本迭代不解析 —— 按现有期次前缀表找不到任何累计口径合计句
			//
			// ⚠️ **住户换过第三轮**（M1c-4 的 TASK-005）：原来放的是真实
			// pboc-2020-04-monthly.html，而路由化之后**它解析成功了**（只有当月数的报告
			// 不再被拒，值全落 _mom 列）—— 那正是本迭代的目标，见
			// TestCollectSamplesSplitsUnsupportedFromFailureByWhetherDataExists 里的正面确认。
			// 本格测的是**三类可分**，需要仍然合格的住户，故改用从同一篇派生的
			// -broken-ordinals：只把首节序号「一」改成「五」，Parse 因板块序号不连续失败，
			// 而存贷款合计句一个字没动 ⇒ onlyCurrentMonthFlowSentences 仍为 true。
			{ID: "c", Title: "2020年4月金融统计数据报告", File: "articles/c.html",
				SHA256: testdataSHA(t, "pboc-2020-04-monthly-broken-ordinals.html")},
			// 类 2：该支持却失败了 —— 正文给一个 index 页，Parse 必然失败
			{ID: "bad", Title: "2024年金融统计数据报告", File: "articles/bad.html",
				SHA256: testdataSHA(t, "pboc-index-p1.html")},
			// 类 3：标题解析不出期次（央行只发「一季度」，「三季度」是站点表述变了）
			{ID: "u", Title: "2024年三季度金融统计数据报告", File: "articles/u.html"},
		},
	}, map[string]string{
		"articles/ok.html":    "pboc-2025-12-annual.html",
		"articles/m.html":     "pboc-2026-07-monthly.html",
		"articles/stock.html": "pboc-2025-08-tsf-stock.html",
		"articles/flow.html":  "pboc-2025-08-tsf-flow.html",
		"articles/c.html":     "pboc-2020-04-monthly-broken-ordinals.html",
		"articles/bad.html":   "pboc-index-p1.html",
	})
}

// 三类必须可分，且**各类的计数分别可见** —— 只断「有 N 条失败」的话，把三类混成一堆的
// 实现照样绿，而那正是要防的形态。
func TestCollectSamplesSeparatesThreeCategories(t *testing.T) {
	got, err := collectSamples(CalibrateDeps{Dir: threeCategoryFixture(t)})
	require.NoError(t, err)

	// 类 1：本迭代不解析 —— **1 篇**。
	//
	// 这一格的住户换过两轮，来历都留着：最初是 monthly + 社融两种；M1c-3a 的 TASK-007
	// 让月报受支持、M1c-3a 的 TASK-010 让社融两种受支持，如今只剩「报告本身没有累计数据」这一类。
	// ⚠️ 下面三条 NotContains 是**前两轮住户的墓碑**：它们回到这一格，就说明对应的解除被撤销了。
	require.Len(t, got.Unsupported, 1)
	u := got.Unsupported[0]
	assert.Equal(t, "2020-04", u.Period, "只有当月数、没有累计数的那一篇")
	assert.Contains(t, u.Err, "只有当月数", "理由要说清是数据不存在，不是解析器不支持")
	assert.NotContains(t, u.Err, "M1c-3 的活",
		"那句理由在本迭代已过期——社融解析器就是本迭代做的")
	assert.NotContains(t, u.Err, "monthly is not supported",
		"月报已受支持，它若回到这一格说明 checkPeriodTypeSupported 的分支被人加了回去")
	for _, k := range []string{backfillKindStock, backfillKindFlow} {
		assert.NotEqualf(t, k, u.Kind,
			"社融 %s 已受支持，它若回到这一格说明 classifyArticles 的硬过滤被人加了回去", k)
	}

	// 类 2：该支持却失败了 —— 只有 index 页那一篇
	require.Len(t, got.Failures, 1)
	assert.Equal(t, "2024-12", got.Failures[0].Period)
	assert.NotEqual(t, u.Err, got.Failures[0].Err,
		"两格的理由必须不同——B 类（写法问题）与 C 类（数据不存在）后续动作完全不同")

	// 类 3：标题解析不出期次 —— 原文进 Unclassified，不得 continue 丢弃
	assert.Equal(t, []string{"2024年三季度金融统计数据报告"}, got.Unclassified)

	// 正常那几期照常出样本（年报 + 7 节月报 + 社融两种）。
	//
	// 🔴 **两个 n 不相等，这正是 functional[1] 要的「独立统计」**（数字按实跑填，不按臆测）：
	//   m2        = 2 —— 只有年报与月报产它；社融两种不含货币板块
	//   tsf_stock = 3 —— 社融存量报告，**外加**年报与 7 节月报里自带的社融板块
	//
	// ⚠️ 我最初写的是 `tsf_stock == 1`（以为只有社融存量报告产它），实跑得 3 才发现
	// **年报与 rule-monthly@v2 月报本身就含社融两节**。留下这句是因为下一个人很可能
	// 做同样的假设：社融字段的来源**不止社融报告**。
	assert.Len(t, got.Samples[FieldM2], 2, "年报与月报各贡献一个 m2；社融两种不产 m2")
	assert.Len(t, got.Samples[FieldTSFStock], 3,
		"社融存量报告 + 年报 + 7 节月报三个来源 —— 与 m2 的 n 不同，正说明两类字段各数各的")
	assert.Len(t, got.Samples[FieldTSFFlowYTD], 3, "社融增量同理，三个来源")
}

// TestCollectSamplesSplitsUnsupportedFromFailureByWhetherDataExists 钉住**分流的判据**：
// 一篇报告归「本迭代不解析」还是「解析失败」，取决于**正文里到底有没有累计口径的合计句**。
//
// 🔴 **这条是 M1c-3a 的 TASK-012 合入后才成立的活行为，不是待做项**（见本任务
// done_criteria 里那条转录）：2022-07/08/10/11 四篇此前被判成「只有当月数」进了
// `Unsupported`，而「1-8月，人民币贷款累计增加15.61万亿元」就在正文里 ——
// TASK-012 把 `1-N月` 一族补进 `periodAlt` 与 `cumulativePeriods` 之后，
// `onlyCurrentMonthFlowSentences` 对它们翻成 false，四篇随之转入 `Failures`。
//
// **机制上是必然而不是碰巧**：这个判据**复用 `cumulativePeriods` 与 `loanFlowRE` 这两处
// 唯一真相源**，而 TASK-012 恰好两处都改了。⇒ 改一个地方（表），下游判据自动跟上，
// **没有人需要记得去改第二处**。这正是当初把判据写成「查表」而不是「匹配错误串」的收益。
//
// ⚠️ **两格对下游的意义相反**：`Unsupported` 那格 CONTRACTS G 写着「**不是** M1c-4 的兜底
// 工作量」，`Failures` 那格写着「M1c-4 要兜的就是这批」。**归错格 = 真实可恢复的数据被
// 永久写销** —— 那正是 R3 的原始损害。
//
// ⚠️ **钉性质不钉取值**：真语料上的 `195/199/23/19/34/38` 会随语料变，这里断言的是
// 「哪一篇进哪一格」以及「移出 Unsupported ≠ 能抽出来」。
//
// ⚠️ 两篇都是**解析失败的金融统计月报**，差别**只有**「有没有累计合计句」这一个变量
// —— 最小对。只断言其中一篇的话，一个把两格合并的实现照样绿。
//
// 🔴 **已知缺口，本任务不修（M1c-3a 的 TASK-011 实测发现，待 team-lead 裁决落点）**：
// 真语料 **2022-05** 两侧的累计句写作「**今年前5个月**，人民币贷款累计增加10.87万亿元」，
// 这个前缀**不在 `periodAlt` 里**（TASK-012 补的是 `1-N月` 一族）⇒ 它此刻仍被本判据判成
// 「只有当月数」而归进 `Unsupported` —— **与 R3 是同一句假话，只是换了个前缀写法**。
// 修它要动 `profiles.go`（不在本任务 writes 内）。⚠️ 本测试**没有**为它加断言：
// 加了会立刻红，而红的理由不在本任务能改的范围内。
func TestCollectSamplesSplitsUnsupportedFromFailureByWhetherDataExists(t *testing.T) {
	periodsOf := func(fs []ParseFailure) []string {
		out := make([]string, 0, len(fs))
		for _, f := range fs {
			out = append(out, f.Period)
		}
		return out
	}

	// 🔴 **两篇真语料在 M1c-4 的 TASK-005 之后都解析成功了**，这是本迭代的正面成果，
	// 先把它钉住——本格随后用的是它们的派生版，不写这一段的话「为什么换住户」会失传。
	for _, f := range []string{"pboc-2020-04-monthly.html", "pboc-2022-08-monthly.html"} {
		obs, perr := Parse(readTestdata(t, f))
		require.NoErrorf(t, perr, "%s 路由化之后应当解析成功（当月值进 _mom 列）", f)
		assert.NotEmptyf(t, obs.Values, "%s 应当产出字段", f)
	}

	// —— 前提：两篇**派生版**构成最小对 ——
	// 两者都把首节序号「一」改成「五」⇒ Parse 因板块序号不连续失败（同一个理由），
	// **唯一的变量**是正文里有没有累计合计句。原来那对（真实 2022-08 vs 2020-04）
	// 差了两个变量（失败理由不同 + 有无累计句），这一版更干净。
	require.False(t, onlyCurrentMonthFlowSentences(stripHTML(readSample(t, "pboc-2022-08-monthly-broken-ordinals.html"))),
		"用例前提：2022-08 正文里有累计合计句（「1-8月，人民币贷款累计增加…」）")
	require.True(t, onlyCurrentMonthFlowSentences(stripHTML(readSample(t, "pboc-2020-04-monthly-broken-ordinals.html"))),
		"用例前提：2020-04 正文里确实一句累计合计句都没有")

	dir := writeCalibrateFixture(t, Manifest{
		From: "2020-01", CompletedAt: "2026-08-29T00:00:00Z",
		Articles: []Article{
			// 一篇正常样本：collectSamples 对「可用样本为 0」有既有守卫，全是失败的会先在那里报错
			{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html",
				SHA256: testdataSHA(t, "pboc-2025-12-annual.html")},
			// 有累计句、但解析失败 ⇒ 解析失败（M1c-4 的兜底工作量）
			{ID: "b", Title: "2022年8月金融统计数据报告", File: "articles/b.html",
				SHA256: testdataSHA(t, "pboc-2022-08-monthly-broken-ordinals.html")},
			// 一句累计合计句都没有 ⇒ 本迭代不解析
			{ID: "c", Title: "2020年4月金融统计数据报告", File: "articles/c.html",
				SHA256: testdataSHA(t, "pboc-2020-04-monthly-broken-ordinals.html")},
		},
	}, map[string]string{
		"articles/ok.html": "pboc-2025-12-annual.html",
		"articles/b.html":  "pboc-2022-08-monthly-broken-ordinals.html",
		"articles/c.html":  "pboc-2020-04-monthly-broken-ordinals.html",
	})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err)

	// ① 有累计句的那篇归「解析失败」——**不得**归「本迭代不解析」
	assert.Contains(t, periodsOf(got.Failures), "2022-08",
		"正文里有累计数据 ⇒ 是解析器抽不出来，属 M1c-4 的兜底工作量")
	assert.NotContains(t, periodsOf(got.Unsupported), "2022-08",
		"归到这一格等于说「该期报告没有这个数」——而数据就在正文里，那是 R3 的原始损害")

	// ② 没有累计句的那篇归「本迭代不解析」——**不得**归「解析失败」
	assert.Contains(t, periodsOf(got.Unsupported), "2020-04",
		"正文真的没有累计数据 ⇒ LLM 兜底也变不出来，正确的是标注")
	assert.NotContains(t, periodsOf(got.Failures), "2020-04",
		"归到这一格会给 M1c-4 加一批**永远清不了零**的工作量")

	// ③ **「移出 Unsupported」不等于「能抽出来」**：它仍然产不出样本（存款侧无源）。
	//    少了这一条，一个「把所有失败都当成可恢复」的实现照样绿。
	for _, r := range got.Records {
		assert.NotEqual(t, "2022-08", r.Period, "2022-08 不该贡献样本：存款侧仍无源")
	}
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
	assert.Contains(t, s, "只有当月数",
		"「本迭代不解析」那一格的**理由**必须渲染出来——只写篇数等于没说为什么")

	// ⚠️ **这条原本断言的是 "monthly"**（那时月报属「本迭代不解析」），夹具里那篇的文件名
	// 因此刻意不含 "monthly"——否则实现即使把它错记成「读文件失败」，断言也会被失败行里的
	// 文件名平凡满足（消融实测过，那时本用例不红）。
	//
	// M1c-3a 的 TASK-007 让月报受支持后换成了社融两种；**M1c-3a 的 TASK-010 让社融也受支持，于是
	// 那个对象又一次消失**——这是同一手法第二次失去对象。这次改锚在「只有当月数」这个
	// **分类理由**上：它只可能由 writeParseFailures 渲染那条 Err 时产生，
	// 文件名、期次、别的错误文本里都不会出现它。**换对象要留痕，别让第三个人以为它一直如此。**
	// 判据不是「输出里有没有这个词」，
	// 而是「这个词只可能由**我要验的那一行**产生」。
	assert.NotContains(t, s, "本迭代不解析该报告种类（社融存量/增量的解析器是 M1c-3 的活）: 2026-07",
		"月报不该再出现在「本迭代不解析」那一段")
}

// summaryLine 从 writeCollectSummary 的输出里切出以 prefix 开头的那一行。
//
// 断言整份输出的 Contains 会让「这句话出现在别的行里」也算数；而本文件要验的几条
// 恰恰是**某一行内部**的措辞，切出来再断言才是在验它。
func summaryLine(t *testing.T, s, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	require.FailNowf(t, "输出里没有以 "+prefix+" 开头的行", "完整输出:\n%s", s)
	return ""
}

// 表头不得点名一个它并不专指的报告种类。
//
// 🔴 **M1c-3a 的 TASK-010 返工（QA F1）**：这一行原本写死 `backfillKindFinance`
// ——「待解析（金融统计数据报告，受支持期次）: 195 篇」。而本任务删掉 `classifyArticles`
// 的 kind 硬过滤之后，这一格装的是 **195 = 57 金融统计 + 138 社融**，标称的那一种只占 29%。
// 这不是「文档不好看」：标定报告是本迭代唯一的人类可读交付物，那句话已经逐字进了验收产物。
//
// ⚠️ **成因留在这里**：删硬过滤时同时删掉了紧邻的一句过期 `Err`，而**过期的标签没删**
// ——同一形状修了一处漏了一处。所以这条钉的是「表头与这一格的实际住户相符」，不是那行的字面。
//
// ⚠️ **钉性质不钉取值**：195 换一份 manifest 就变，
// 「这一格跨多种报告时表头不得点名任何单一种类」不会变。
func TestCollectSummaryHeaderDoesNotNameASingleKind(t *testing.T) {
	dir := threeCategoryFixture(t)
	var out bytes.Buffer
	got, err := collectSamples(CalibrateDeps{Dir: dir, Out: &out})
	require.NoError(t, err)

	// —— 前提①：这一格的输入确实**跨多种报告**。
	// 少了它，下面那条否定式断言会在「夹具恰好只剩一种」时**平凡为真**，
	// 而那正是这条守卫失效却仍全绿的样子。
	st, err := loadManifest(dir)
	require.NoError(t, err)
	kinds := map[string]bool{}
	for _, a := range st.Manifest.Articles {
		if _, _, kind, err := parseTitle(a.Title); err == nil {
			kinds[kind] = true
		}
	}
	require.Len(t, kinds, 3,
		"前提：夹具输入须跨三种报告，否则「表头不得点名单一种类」无从谈起")

	// —— 前提②：表头那一行在。整行被删时，下面两条都会退化成平凡真。
	line := summaryLine(t, out.String(), "  待解析")

	// ① 篇数必须印出来。6 篇进这一格、C 类那篇被减掉 ⇒ 5。
	//    独家杀手：把数字印错或不印 —— 它对种类名一无所知，②照样绿。
	require.Equal(t, 5, got.Periods, "夹具 6 篇进这一格、C 类那篇被减掉")
	assert.Contains(t, line, "5 篇", "这一格的篇数必须印在表头上")

	// ② 表头不得点名任何单一种类。
	//    独家杀手：把种类名塞回表头 —— 那时 ①仍然绿（数字没动），只有这里红。
	for _, k := range backfillKindOrder {
		assert.NotContainsf(t, line, k,
			"表头点名了「%s」，而这一格装着 %d 种报告 —— 这是一句用户可见的假话",
			k, len(kinds))
	}
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
// 不读就会让报告显示「失败：无」—— 而失败表的用途正是**让该修的东西可见**。
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
		// 真跑用的那份产物出自 M1c-3a 的 TASK-010 之前，**没有** reconcile 字段。静默略过会让
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

// —— M1c-3a 的 TASK-010：放行社融两种 kind + 月报按段级口径分流 ——
//
// Context Checkpoint: done_criteria → test mapping（M1c-3a 的 TASK-010）
// functional[0]     社融两种进入解析流程、字段计入 Samples
//                                    → TestCollectSamplesParsesTSFReports
// functional[1]     社融字段与非社融字段的 n 独立统计
//                                    → TestCollectSamplesKeepsTSFAndFinanceFieldsIndependent
// functional[2]     无累计数据的月报归「本迭代不解析」而非「解析失败」
//                                    → TestCollectSamplesSeparatesNoDataFromParseFailure
// boundary[0]       四格加总恒等于总篇数（任何一篇都不得静默消失）
//                                    → TestCollectSamplesAccountsForEveryArticle
// boundary[1]       B 类（口径混杂）与 C 类（无累计数据）在报告里可区分
//                                    → TestCollectSamplesSeparatesNoDataFromParseFailure
// error_handling[0] 单篇失败仍继续，且断言 Err 内容而非只数条数
//                                    → TestCollectSamplesSeparatesNoDataFromParseFailure

// 社融两种报告必须**进入解析流程**，字段计入 Samples。
//
// 🔴 在此之前 `classifyArticles` 硬过滤掉它们（`kind != backfillKindFinance` 即 continue），
// 那句 Err 写的是「社融存量/增量的解析器是 M1c-3 的活」——**而本迭代就是 M1c-3**：
// 解析器（M1c-3a 的 TASK-002/003/007）已经做好，calibrate 仍不喂给它，
// 于是报告里社融字段的 n 原地不动。
//
// ⚠️ **社融两种的 Observation 不能 Save**（同期三篇共享 (period, period_type) 业务键），
// 但 calibrate **只统计字段值、不入库**，所以这里安全——见实现处注释。
func TestCollectSamplesParsesTSFReports(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From: "2025-08", CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "s", Title: "2025年8月社会融资规模存量统计数据报告", File: "articles/stock.html",
				SHA256: testdataSHA(t, "pboc-2025-08-tsf-stock.html")},
			{ID: "f", Title: "2025年8月社会融资规模增量统计数据报告", File: "articles/flow.html",
				SHA256: testdataSHA(t, "pboc-2025-08-tsf-flow.html")},
		},
	}, map[string]string{
		"articles/stock.html": "pboc-2025-08-tsf-stock.html",
		"articles/flow.html":  "pboc-2025-08-tsf-flow.html",
	})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err)

	assert.Empty(t, got.Unsupported, "社融两种不再是「本迭代不解析」——解析器就是本迭代做的")
	assert.Empty(t, got.Failures, "两份都是真实快照，应当解析成功")
	assert.NotEmpty(t, got.Samples[FieldTSFStock], "存量字段必须真的采到样本，不是「没报错」就算过")
	assert.NotEmpty(t, got.Samples[FieldTSFFlowYTD], "增量字段同理")
}

// 社融字段与非社融字段的 n **独立统计**，互不混淆。
//
// 只断「有样本」不够：一个把两类字段混进同一个池子的实现照样非空。
// 这里用三篇不同 kind 的真实快照，逐字段断言各自的 n。
func TestCollectSamplesKeepsTSFAndFinanceFieldsIndependent(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From: "2025-08", CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			{ID: "m", Title: "2025年8月金融统计数据报告", File: "articles/m.html",
				SHA256: testdataSHA(t, "pboc-2025-08-monthly.html")},
			{ID: "s", Title: "2025年8月社会融资规模存量统计数据报告", File: "articles/stock.html",
				SHA256: testdataSHA(t, "pboc-2025-08-tsf-stock.html")},
			{ID: "f", Title: "2025年8月社会融资规模增量统计数据报告", File: "articles/flow.html",
				SHA256: testdataSHA(t, "pboc-2025-08-tsf-flow.html")},
		},
	}, map[string]string{
		"articles/m.html":     "pboc-2025-08-monthly.html",
		"articles/stock.html": "pboc-2025-08-tsf-stock.html",
		"articles/flow.html":  "pboc-2025-08-tsf-flow.html",
	})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err)
	require.Empty(t, got.Failures, "三份都该解析成功")

	// m2 只来自金融统计报告；tsf_stock 只来自社融存量报告 —— 两个 n 互相独立。
	assert.Len(t, got.Samples[FieldM2], 1, "m2 的样本只应来自那一篇金融统计报告")
	assert.Len(t, got.Samples[FieldTSFStock], 1, "tsf_stock 的样本只应来自那一篇社融存量报告")

	// Records 必须**逐篇**记入（社融两种也在内）——否则报告里那 69 期看不出期次分布。
	//
	// ⚠️ 用「这条记录贡献了哪个字段」来区分三篇，而不是给 SampleRecord 加一个
	// Extractor 字段：本任务的 writes 只有 calibrate.go / calibrate_test.go，
	// 而三种报告产出的字段本就互不相交，已经够分辨了。
	var nStock, nFlow, nFinance int
	for _, r := range got.Records {
		assert.Equal(t, "2025-08", r.Period, "三篇同期")
		assert.NotEmpty(t, r.PeriodType, "period_type 必须填，社融两种也不例外")
		switch {
		case r.Values[FieldTSFStock] != 0:
			nStock++
		case r.Values[FieldTSFFlowYTD] != 0:
			nFlow++
		case r.Values[FieldM2] != 0:
			nFinance++
		}
	}
	assert.Equal(t, 1, nStock, "社融存量那篇必须记进 Records")
	assert.Equal(t, 1, nFlow, "社融增量那篇必须记进 Records")
	assert.Equal(t, 1, nFinance, "金融统计那篇照旧")
	assert.Len(t, got.Records, 3, "三篇都在，一篇都不能少")
}

// 🔴 **无累计数据的月报归「本迭代不解析」，不是「解析失败」** —— 这是一个显式的设计决策。
//
// # 为什么不归 Failures
//
// `Failures` 在报告里的标题是「解析失败（该支持却失败了，M1c-4 的兜底工作量）」，
// 而 calibrate.go 顶部注释写着「③ 解析失败 —— M1c-4 要兜的就是这批」。
// 这类报告**正文里根本没有累计数据**（小标题全是当月），LLM 兜底也变不出不存在的数——
// 归进去等于给 M1c-4 凭空加一批**永远清不了零**的工作量。
//
// # 判别用的是**正向属性**，不是错误串匹配
//
// 判据：这篇报告的正文里**没有任何期内累计口径的合计句**——复用 `cumulativePeriods`
// 这张唯一真相源的表去问，而不是去匹配「期内合计 not found」那句错误文本。
// 错误文本会随实现措辞改动而失配；「有没有累计句」是报告本身的性质。
//
// # B 类与 C 类必须可区分
//
// B 类（2023-07/08/10/11）**有累计合计句**、但分部门段紧邻的是当月句，被 M1c-3a 的 TASK-009 的
// 口径守卫拒绝 —— 那是站点某几期的写法问题，仍归 Failures。
// C 类（本例）是数据根本不存在。两者后续动作完全不同，所以必须分在两格。
func TestCollectSamplesSeparatesNoDataFromParseFailure(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From: "2020-04", CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			// C 类：只有当月数、没有累计数（正文零个累计前缀句）
			// ⚠️ M1c-4 的 TASK-005 起改用派生版：真实 2020-04 现在解析成功（值全落 _mom），
			// 不再进这一格；派生版只把首节序号「一」改成「五」，Parse 失败而合计句一字未动。
			{ID: "c", Title: "2020年4月金融统计数据报告", File: "articles/c.html",
				SHA256: testdataSHA(t, "pboc-2020-04-monthly-broken-ordinals.html")},
			// 该支持却失败了：正文给一个 index 页
			{ID: "bad", Title: "2024年金融统计数据报告", File: "articles/bad.html",
				SHA256: testdataSHA(t, "pboc-index-p1.html")},
			// 正常样本
			{ID: "ok", Title: "2025年8月金融统计数据报告", File: "articles/ok.html",
				SHA256: testdataSHA(t, "pboc-2025-08-monthly.html")},
		},
	}, map[string]string{
		"articles/c.html":   "pboc-2020-04-monthly-broken-ordinals.html",
		"articles/bad.html": "pboc-index-p1.html",
		"articles/ok.html":  "pboc-2025-08-monthly.html",
	})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err)

	// C 类进「本迭代不解析」，且理由**不得**沿用「解析器是 M1c-3 的活」（那句在本迭代已过期）
	require.Len(t, got.Unsupported, 1, "无累计数据的那篇归「本迭代不解析」")
	u := got.Unsupported[0]
	assert.Equal(t, "2020-04", u.Period)
	assert.NotContains(t, u.Err, "M1c-3 的活",
		"这句理由在本迭代已过期——解析器就是本迭代做的，不能再拿它当不解析的理由")
	assert.Contains(t, u.Err, "累计", "理由要说清是「报告里没有累计数据」")
	assert.NotEmpty(t, u.File, "四个字段都要填，否则看不出是哪一篇")
	assert.NotEmpty(t, u.Kind)

	// ⚠️ 原始解析错误必须**保留**在理由里：C 类的判别用的是「有没有累计句」这个正向属性，
	// 它与「这篇为什么解析失败」是两件事。真语料里 2023-05 就是判为 C 类、
	// 而直接错误是「板块序号不连续」——若不保留原错误，那个结构问题会被标签盖掉。
	assert.Contains(t, u.Err, "hestia:", "原始解析错误要一并带上，别让分类标签盖掉真实成因")

	// 该支持却失败的仍在 Failures，且 Err 是它自己的原因（不是同一句话）
	require.Len(t, got.Failures, 1)
	assert.Equal(t, "2024-12", got.Failures[0].Period)
	assert.NotEqual(t, u.Err, got.Failures[0].Err,
		"两格的理由必须不同——一个把所有失败写成同一句话的实现照样能让「有 N 条」通过")

	assert.Len(t, got.Samples[FieldM2], 1, "正常那篇照常出样本，失败不中止整趟")
}

// 🔴 **任何一篇都不得静默消失**：四格加总必须恰好等于 manifest 里的篇数。
//
// 上游纪律见 backfill_reconcile.go:196——「不 continue 掉：对账的全部意义是
// 『不让东西静默消失』」。这条断言是那条纪律在 calibrate 侧唯一的机械保证。
//
// ⚠️ 用**恒等式**而不是逐格数字：逐格数字会随语料与实现演进而过期，
// 而「加起来等于总数」在任何演进下都必须成立。
func TestCollectSamplesAccountsForEveryArticle(t *testing.T) {
	articles := []Article{
		{ID: "ok", Title: "2025年8月金融统计数据报告", File: "articles/ok.html",
			SHA256: testdataSHA(t, "pboc-2025-08-monthly.html")},
		{ID: "c", Title: "2020年4月金融统计数据报告", File: "articles/c.html",
			SHA256: testdataSHA(t, "pboc-2020-04-monthly.html")},
		{ID: "s", Title: "2025年8月社会融资规模存量统计数据报告", File: "articles/stock.html",
			SHA256: testdataSHA(t, "pboc-2025-08-tsf-stock.html")},
		{ID: "bad", Title: "2024年金融统计数据报告", File: "articles/bad.html",
			SHA256: testdataSHA(t, "pboc-index-p1.html")},
		{ID: "u", Title: "2024年三季度金融统计数据报告", File: "articles/u.html"},
	}
	dir := writeCalibrateFixture(t, Manifest{
		From: "2020-04", CompletedAt: "2026-08-24T10:00:00Z", Articles: articles,
	}, map[string]string{
		"articles/ok.html":    "pboc-2025-08-monthly.html",
		"articles/c.html":     "pboc-2020-04-monthly.html",
		"articles/stock.html": "pboc-2025-08-tsf-stock.html",
		"articles/bad.html":   "pboc-index-p1.html",
	})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err)

	parsed := len(got.Records)
	sum := parsed + len(got.Failures) + len(got.Unsupported) + len(got.Unclassified)
	assert.Equalf(t, len(articles), sum,
		"四格加总必须等于总篇数：已解析 %d + 解析失败 %d + 本迭代不解析 %d + 标题解析不出期次 %d = %d，"+
			"而 manifest 里有 %d 篇——差额就是被静默丢掉的那些",
		parsed, len(got.Failures), len(got.Unsupported), len(got.Unclassified), sum, len(articles))
}

// 🔴 判别器的**两个半边各自承重** —— `any` 与 `!cumulative` 都不能少（M1c-3a 的 TASK-010）。
//
// 这条是直接喂构造串给 `onlyCurrentMonthFlowSentences`，不经 collectSamples。
// 理由是那两个半边**各自只在特定输入上显形**，而夹具里凑不齐这两种输入：
//
//   - `any` 那半：只有喂「压根没有合计句」的文本（如 index 页）才显形 —— 去掉它，
//     index 页会被判成「该期报告只有当月数」，**一句关于它的假话**，
//     还会把一条真失败从 M1c-4 的清单上抹掉。这是实撞出来的，不是设想。
//   - `!cumulative` 那半：只有喂「解析失败**且**含累计句」的报告才显形。真语料里那是
//     B 类（2023-07/08/10/11，合计句累计而分部门段当月），**但 testdata 里没有这四篇**
//     ⇒ 走 collectSamples 的用例杀不动它（实测 SURVIVED），只能在这一层钉。
//
// ⇒ 两个半边、两个独家杀手，缺一个都会让对应的消融存活。
func TestOnlyCurrentMonthFlowSentencesNeedsBothHalves(t *testing.T) {
	const cumulative = "前八个月人民币贷款增加13.46万亿元。8月份人民币存款增加1.26万亿元。"
	const currentOnly = "4月份人民币贷款增加1.7万亿元。4月份人民币存款增加1.27万亿元。"
	const noFlow = "首页 金融统计数据报告 2025年 更多>>"

	assert.True(t, onlyCurrentMonthFlowSentences(currentOnly),
		"只有当月合计句 ⇒ 这就是「报告有数据但全是当月口径」，本函数要认出它")

	assert.False(t, onlyCurrentMonthFlowSentences(cumulative),
		"含累计合计句 ⇒ 不属本类。杀 `!cumulative` 那半：把 cumulativePeriods 查表改成恒 false，"+
			"本条立刻红，而走 collectSamples 的用例杀不动它（testdata 里没有 B 类快照）")

	assert.False(t, onlyCurrentMonthFlowSentences(noFlow),
		"压根没有合计句 ⇒ 不属本类。杀 `any` 那半：去掉它，index 页会被判成"+
			"「该期报告只有当月数」，既是假话又会抹掉一条真失败")

	// 阴性对照：不是「含任意期次前缀就算」——当月与累计混在一起时以累计为准。
	assert.False(t, onlyCurrentMonthFlowSentences(cumulative+currentOnly),
		"混合文本里只要有一条累计句就不属本类，顺序无关")
}

// 「本迭代不解析」那一格必须**按期次排序** —— 多条时顺序不能随机。
//
// 真语料里这一格有 23 条（全是只有当月数的月报），报告要按期次读才看得出
// 「哪几年是这个形态」；顺序随机的话，人得自己在脑子里排一遍。
//
// ⚠️ **订正：本条并没有覆盖 `classifyArticles` 末尾那个排序比较器**（我最初以为它会）。
//
// 实测覆盖 profile：`sort.SliceStable(res.Unsupported, …)` 的**调用本身**已覆盖，
// 而**闭包体**（`296.56,298.3`）计数为 0 —— 比较器一次都没被调用过，本条却通过了。
//
// 查清的机制：C 类条目是在**解析循环里**追加进 `res.Unsupported` 的，而那次 sort 位于
// `classifyArticles` 末尾、**执行得更早**。排序结果之所以正确，是因为解析循环遍历的
// `items` **本身已按期次排好** —— 顺序是那条路径顺带保证的，不是这个比较器给的。
//
// ⇒ 本条钉住的是**性质**（这一格按期次升序），与它由谁保证无关；这才是它该断言的东西。
// 而那个比较器目前**没有生产者**（社融已受支持、五种 period_type 全部放行 ⇒
// `classifyArticles` 内部不再往 Unsupported 里塞东西），属于「配置变了之后暂时不可达、
// 但必须留着等下次启用」的分支，与 M1c-3a 的 TASK-007 里 `checkPeriodTypeSupported`
// 那条错误路径同类。**把它记在这里，免得后人看到闭包没覆盖就去删排序。**
//
// ⚠️ 夹具用同一份快照配两个不同标题：两篇都会在「没有累计数据」处失败，
// **期次交叉校验（标题 vs 正文自解）在 Parse 失败时根本走不到**，所以这里
// 期次取自标题、互不相同，正好用来验排序。
func TestCollectSamplesSortsUnsupportedByPeriod(t *testing.T) {
	dir := writeCalibrateFixture(t, Manifest{
		From: "2020-04", CompletedAt: "2026-08-24T10:00:00Z",
		Articles: []Article{
			// 刻意逆序放入，若实现不排序就会原样输出
			// ⚠️ M1c-4 的 TASK-005 起，原来的 pboc-2020-04-monthly.html **解析成功**了
			// （只有当月数的报告不再被拒，值全落 _mom 列）⇒ 它不再进这一格。
			// 本格测的是**排序**，需要仍然合格的住户，故改用从同一篇真语料派生的
			// pboc-2020-04-monthly-broken-ordinals.html：只把首节序号「一」改成「五」，
			// Parse 因板块序号不连续失败，而存贷款合计句一个字没动 ⇒
			// onlyCurrentMonthFlowSentences 仍为 true，分类结果不变。
			{ID: "later", Title: "2020年5月金融统计数据报告", File: "articles/b.html",
				SHA256: testdataSHA(t, "pboc-2020-04-monthly-broken-ordinals.html")},
			{ID: "earlier", Title: "2020年4月金融统计数据报告", File: "articles/a.html",
				SHA256: testdataSHA(t, "pboc-2020-04-monthly-broken-ordinals.html")},
			// 至少一篇正常样本：collectSamples 对「可用样本为 0」有既有守卫，
			// 只放两篇不解析的会在那里就报错、走不到排序。
			{ID: "ok", Title: "2025年金融统计数据报告", File: "articles/ok.html",
				SHA256: testdataSHA(t, "pboc-2025-12-annual.html")},
		},
	}, map[string]string{
		"articles/a.html":  "pboc-2020-04-monthly-broken-ordinals.html",
		"articles/b.html":  "pboc-2020-04-monthly-broken-ordinals.html",
		"articles/ok.html": "pboc-2025-12-annual.html",
	})

	got, err := collectSamples(CalibrateDeps{Dir: dir})
	require.NoError(t, err)

	require.Len(t, got.Unsupported, 2, "两篇都只有当月数，都该进这一格")
	assert.Equal(t, []string{"2020-04", "2020-05"},
		[]string{got.Unsupported[0].Period, got.Unsupported[1].Period},
		"必须按期次升序 —— 输入是逆序放的，不排序就会原样吐出来")
}

// Context Checkpoint: done_criteria → test mapping（M1c-3b 的 TASK-001）
//
//	functional[2]  管道交出的是整个 Observation 而不只是 Values
//	               → TestEachParsedArticleYieldsFullObservation
//	functional[0]  eachParsedArticle 返回「sha256 未校验的篇数」，由调用方汇总成 warning
//	               → TestCollectSamplesVerifiesArticleSHA256（既有，钉住那条 warning）
//	functional[1]  collectSamples 改为调用管道，外部行为零变化
//	boundary[0]    res.Periods++ 在 fail() 之前（含解析失败的篇数）
//	               → TestCollectSamplesRecordsMissingFileAndContinues（既有，读文件失败仍计入 Periods）
//	               ⚠️ 这两条的**真**判据是背对背基线比对（error_handling[0]，verify_by: manual）——
//	                  单元测试全绿也可能 199 变 161，见 boundary[0] 原文。

// TestEachParsedArticleYieldsFullObservation：共用管道必须交出**整个** Observation，
// 而不只是 Values。
//
// 这条断言存在的理由：collectSamples 原先只取 Values，Meta 被就地丢弃。
// 若重构时图省事仍只传 Values，M1c-3b 的 TASK-003 就无从装配业务键——
// 而那时错误会表现为「合并组恒为 0」，看起来像语料问题，不像管道问题。
//
// ⚠️ 夹具沿用 completedFixture / writeCalibrateFixture，**不新写一份**：两份 fixture 会分叉。
func TestEachParsedArticleYieldsFullObservation(t *testing.T) {
	dir := completedFixture(t)

	res := &CalibrateResult{Samples: map[string][]float64{}}
	st, err := loadManifest(dir)
	require.NoError(t, err)
	items := classifyArticles(res, st.Manifest.Articles)
	require.NotEmpty(t, items)

	var got []parsedArticle
	eachParsedArticle(dir, res, items, func(p parsedArticle) { got = append(got, p) })

	require.NotEmpty(t, got)
	for _, p := range got {
		// 用 assert 而不是 require：多篇时第一篇失败也要看完其余篇 —— 「只有某一篇缺
		// Meta」与「全部都缺」是两种不同的故障，require 会把它们压成同一个现象。
		// 每条都带上文件名，否则只知道有一篇不对、不知道是哪一篇。
		assert.NotEmpty(t, p.obs.Meta.Period, "Meta.Period 不得为空：%s", p.item.a.File)
		assert.NotEmpty(t, p.obs.Meta.PeriodType, "Meta.PeriodType 不得为空：%s", p.item.a.File)
		assert.NotEmpty(t, p.obs.Meta.PublishedAt, "Meta.PublishedAt 不得为空：%s", p.item.a.File)
		assert.NotEmpty(t, p.obs.Meta.Extractor, "Meta.Extractor 不得为空：%s", p.item.a.File)

		// 🔴 这条钉住的是「**item 与产出它的那次迭代的 obs 是配对的**」，**不是**
		// 「分类算出的期次与正文自解的期次一致」—— 后者由 eachParsedArticle 内部那道
		// 交叉校验保证、由 TestCollectSamplesCrossChecksPeriodAgainstTitle 守着。
		//
		// ⚠️ 措辞是变异实测定下来的，别改回去（M1c-3b 的 TASK-001，隔离副本上跑）：
		//   · 删掉管道里那道交叉校验 guard ⇒ **本测试仍绿**，红的是上面那条既有测试；
		//   · 把 fn 调用改成 `parsedArticle{obs: obs}`（不传 item）⇒ **本测试红**，
		//     且全包只红这一条。
		// ⇒ 它守的是配对关系。写成「期次必须一致」会让下一个人拿着这条红去查分类逻辑，
		// 而真正坏掉的是 item 的传递 —— 断言红得对、给的理由却指错方向。
		assert.Equal(t, p.item.period, p.obs.Meta.Period,
			"item 与 obs 必须来自同一次迭代（item 未传或传错时这里会先炸）：%s", p.item.a.File)
	}
}
