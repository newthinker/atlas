package hestia

// Context Checkpoint: done_criteria → test mapping（M1d 的 TASK-004）
// functional[0]     "P0 只列 failed 的闸，passed/skipped 不出现"     → TestRenderP0ListsOnlyFailedChecks
// functional[1]     "P1 只取错误首行"                                → TestRenderP1KeepsFirstLineOnly
// functional[2]     "P2 含 Verdict 原样、extractor、四锚带单位、发布日" → TestRenderP2CarriesVerdictAndAnchors
// boundary[0]       "锚缺失写 n/a 不写 0"                            → TestRenderP2MissingAnchorIsNA
//                   "≥1e6 不走 %g 指数记法"（C1，原 1.776e+05 判据恒过） → TestFmtNumKeepsPlainDigitsAboveMillion
// boundary[0]       "存款只有 mom 时取 mom；两者都缺 ⇒ n/a 与 -"      → TestRenderP2UsesMomWhenYtdAbsent / TestRenderP2BothFlowsAbsent
// boundary[0] R-004 "ytd 与 mom 同在时取 ytd 并标 ytd"               → TestRenderP2PrefersYtdWhenBothPresent
// error_handling[0] "Check.Value == nil 时写 n/a，不 panic"           → TestRenderP0NilValueIsNA

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/macro/bitemporal"
)

func notifyObs() Observation {
	return Observation{
		Meta: Meta{Period: "2026-08", PeriodType: "monthly", ArticleID: "2026091412345678901",
			Extractor: "rule-monthly@v2", PublishedAt: "2026-09-12"},
		Values: map[string]float64{
			FieldM2: 356.71, FieldM1: 118.48, FieldTSFStock: 462.06,
			FieldDepositFlowYTD: 177600,
		},
	}
}

func f64(v float64) *float64 { return &v }

// P0：只列 failed 的闸，带实测值与 Reason；passed / skipped 的闸不出现。
func TestRenderP0ListsOnlyFailedChecks(t *testing.T) {
	rep := ValidationReport{Passed: false, Checks: []Check{
		{ID: "monetary_hierarchy", Status: CheckPassed},
		{ID: "deposit_sum", Status: CheckFailed, Value: f64(0.2501),
			Reason: "tolerance_exceeded[ytd]: residual 0.2501 exceeds 0.1700"},
		{ID: "stock_continuity", Status: CheckSkipped, Reason: "no_prior_period"},
	}}
	got := renderP0(notifyObs(), rep)

	assert.True(t, strings.HasPrefix(got, "[P0]"), "紧急度由前缀承载：%q", got)
	assert.Contains(t, got, "2026-08/monthly")
	assert.Contains(t, got, "pending")
	assert.Contains(t, got, "deposit_sum")
	assert.Contains(t, got, "0.2501")
	assert.Contains(t, got, "tolerance_exceeded[ytd]")
	assert.Contains(t, got, "2026091412345678901", "article_id 是能直接拼回 URL 的那个")
	assert.Contains(t, got, "rule-monthly@v2", "extractor")
	assert.NotContains(t, got, "monetary_hierarchy", "passed 的闸不该出现，否则 P0 会被稀释成一张全表")
	assert.NotContains(t, got, "stock_continuity")
}

// P0 边界（本 DoD 补，需求原文没覆盖）：failed 闸的 Value 为 nil 时写 n/a，不得解引用 panic。
func TestRenderP0NilValueIsNA(t *testing.T) {
	rep := ValidationReport{Passed: false, Checks: []Check{
		{ID: "period_consistency", Status: CheckFailed, Value: nil, Reason: "meta_mismatch"},
	}}
	var got string
	require.NotPanics(t, func() { got = renderP0(notifyObs(), rep) })
	assert.Contains(t, got, "period_consistency")
	assert.Contains(t, got, "n/a")
	assert.Contains(t, got, "meta_mismatch")
}

// P1：只取错误的**首行**——errors.Join 出来的多行错误会把消息撑成一屏。
func TestRenderP1KeepsFirstLineOnly(t *testing.T) {
	c := Candidate{Period: "2026-08", PeriodType: "monthly", ArticleID: "2026091412345678901"}
	err := errors.New("hestia ingest 2026-08/monthly (2026091412345678901): parse: boom\nsecond line\nthird")
	got := renderP1(c, err)

	assert.True(t, strings.HasPrefix(got, "[P1]"))
	assert.Contains(t, got, "2026-08/monthly")
	assert.Contains(t, got, "2026091412345678901")
	assert.Contains(t, got, "parse: boom")
	assert.NotContains(t, got, "second line")
}

// P2：Verdict 原样写进去（Duplicate 不被吞）、extractor、四个锚字段带单位。
func TestRenderP2CarriesVerdictAndAnchors(t *testing.T) {
	got := renderP2(notifyObs(), Outcome{Verdict: bitemporal.Duplicate, Table: TableObservations})

	assert.True(t, strings.HasPrefix(got, "[P2]"))
	assert.Contains(t, got, "Duplicate", "Verdict 必须可见——Duplicate 与 New 对运维的含义不同")
	assert.Contains(t, got, "rule-monthly@v2")
	assert.Contains(t, got, "356.71")
	assert.Contains(t, got, "118.48")
	assert.Contains(t, got, "462.06")
	assert.Contains(t, got, "177600")
	assert.Contains(t, got, "ytd", "存款流量要标口径")
	assert.Contains(t, got, "万亿", "存量单位")
	assert.Contains(t, got, "亿元", "流量单位")
	assert.Contains(t, got, "2026091412345678901", "article")
	assert.Contains(t, got, "2026-09-12", "发布日")
}

// 锚字段缺失写 n/a，不写 0——0 是一个合法的数，n/a 才是「没有」。
func TestRenderP2MissingAnchorIsNA(t *testing.T) {
	obs := notifyObs()
	delete(obs.Values, FieldTSFStock)
	got := renderP2(obs, Outcome{Verdict: bitemporal.New, Table: TableObservations})
	assert.Contains(t, got, "n/a")
	assert.NotContains(t, got, "462.06")
}

// 存款只有当月口径时取 _mom 并标 mom。
func TestRenderP2UsesMomWhenYtdAbsent(t *testing.T) {
	obs := notifyObs()
	delete(obs.Values, FieldDepositFlowYTD)
	obs.Values[FieldDepositFlowMoM] = 447
	got := renderP2(obs, Outcome{Verdict: bitemporal.New, Table: TableObservations})
	assert.Contains(t, got, "447")
	assert.Contains(t, got, "mom")
	require.NotContains(t, got, "ytd")
}

// 两个口径都缺 ⇒ 值 n/a、口径 -。
func TestRenderP2BothFlowsAbsent(t *testing.T) {
	obs := notifyObs()
	delete(obs.Values, FieldDepositFlowYTD)
	got := renderP2(obs, Outcome{Verdict: bitemporal.New, Table: TableObservations})
	assert.Contains(t, got, "人民币存款 n/a 亿元 (-)")
	assert.NotContains(t, got, "ytd")
	assert.NotContains(t, got, "mom")
}

// reviewer 补（R-004）：ytd 与 mom 同时存在时取 ytd 并标 ytd，mom 不出现。
func TestRenderP2PrefersYtdWhenBothPresent(t *testing.T) {
	obs := notifyObs()
	obs.Values[FieldDepositFlowMoM] = 447
	got := renderP2(obs, Outcome{Verdict: bitemporal.New, Table: TableObservations})
	assert.Contains(t, got, "177600")
	assert.Contains(t, got, "(ytd)")
	assert.NotContains(t, got, "447")
	assert.NotContains(t, got, "mom")
}

// A7（QA 终审，Leader 拍板）：Force 重跑已入库期次时 Save 判 Duplicate、只刷 article_id、
// 本次新抽的 Values 一个都不写（ingest.go 的注释原话）。P2 把本次重抽的锚值写成「入库」
// 会让运维以为这些数进了库——Duplicate 时措辞改为「已在库（本次抽取值未写入）」，
// 其它 Verdict 不变。
func TestRenderP2DuplicateSaysValuesNotWritten(t *testing.T) {
	dup := renderP2(notifyObs(), Outcome{Verdict: bitemporal.Duplicate, Table: TableObservations})
	assert.Contains(t, dup, "Duplicate", "Verdict 仍要原样可见")
	assert.Contains(t, dup, "未写入")
	assert.NotContains(t, dup, "入库 Duplicate", "Duplicate 不能再说「入库」")

	for _, v := range []bitemporal.Verdict{bitemporal.New, bitemporal.Revision, bitemporal.OutOfOrder} {
		got := renderP2(notifyObs(), Outcome{Verdict: v, Table: TableObservations})
		assert.NotContains(t, got, "未写入", "%s 的措辞不变", v)
		assert.Contains(t, got, "入库 "+v.String(), "%s 仍是「入库」", v)
	}
}

// fmtNum 必须对 ≥1e6 的值仍给最短精确表示（CONTRACTS §C 的 C1）。
//
// 🔴 这条替代了原先那句 `NotContains(got, "1.776e+05")`——**那条断言恒过**：%g 对 177600
// 本就打 "177600"，切指数记法的阈值在 1e6。原判据抄自需求文档的一句假理由（§A4 已订正），
// 它在任何实现下都不可能失败，因此钉不住「不要换成 %g」这条约束。
// 判据：把 fmtNum 改成 fmt.Sprintf("%g", v) 后本测试必须转红。
func TestFmtNumKeepsPlainDigitsAboveMillion(t *testing.T) {
	// 1776000 是社融存量那一族真实会出现的量级（单位亿元时）；%g 打 "1.776e+06"。
	assert.Equal(t, "1776000", fmtNum(1776000), "≥1e6 仍须最短精确表示，不得走 %g 的指数记法")
	// 边界两侧各一个，钉住「阈值在 1e6 而不是 1e5」这个事实本身。
	assert.Equal(t, "177600", fmtNum(177600), "1e5 量级 %g 也不切指数——原判据正因此恒过")
	assert.Equal(t, "356.71", fmtNum(356.71), "小数不受影响")
}
