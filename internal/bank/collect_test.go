package bank

// Context Checkpoint: done_criteria → test mapping（TASK-004，collect 部分）
// functional[0]     A+H 去重 / 主名 / Aliases / 顺序 / MissingFields / 只配 H 股 → TestCollectDedupsAH, TestCollectOrderFirstAppearance, TestCollectHOnly
// error_handling[0] 单主体失败不影响其他，Err 原样、Latest 零、Ind 全 NaN      → TestCollectKeepsErrors

import (
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSource struct {
	series map[string]Series
	errs   map[string]error
	calls  map[string]int
}

func (f *fakeSource) Fetch(symbol string) (Series, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[symbol]++
	return f.series[symbol], f.errs[symbol] // 出错时也原样返回数据，检验 Collect 不采信部分数据
}

var psbc = []Observation{obs("2026-06-30", 1.0, 214.93, 10.04)}

func TestCollectDedupsAH(t *testing.T) {
	src := &fakeSource{series: map[string]Series{
		"600036.SH": {Obs: slices.Clone(cmb)},
		"601658.SH": {Obs: slices.Clone(psbc), MissingFields: []string{"HXYJBCZL"}},
	}}
	banks := []BankCfg{
		{Market: "CN_A", Symbol: "600036.SH", Name: "招商银行"},
		{Market: "HK", Symbol: "3968.HK", Name: "招商银行H", AShareRef: "600036.SH"},
		{Market: "CN_A", Symbol: "601658.SH", Name: "邮储银行"},
	}
	rs := Collect(banks, src)
	require.Len(t, rs, 2, "A+H 同一主体只计一次")
	assert.Equal(t, map[string]int{"600036.SH": 1, "601658.SH": 1}, src.calls, "每个主体只拉一次")
	assert.Equal(t, "600036.SH", rs[0].Symbol)
	assert.Equal(t, "招商银行", rs[0].Name)
	assert.Equal(t, []string{"招商银行H"}, rs[0].Aliases)
	assert.Equal(t, day("2026-06-30"), rs[0].Latest)
	assert.Equal(t, Change{385.1, -2.66, -25.83, day("2026-06-30")}, rs[0].Ind[IndCoverage])
	assert.Empty(t, rs[0].MissingFields)
	assert.Equal(t, "邮储银行", rs[1].Name)
	assert.Empty(t, rs[1].Aliases)
	assert.Equal(t, []string{"HXYJBCZL"}, rs[1].MissingFields, "MissingFields 透传")
}

func TestCollectOrderFirstAppearance(t *testing.T) {
	src := &fakeSource{series: map[string]Series{
		"600036.SH": {Obs: slices.Clone(cmb)},
		"601658.SH": {Obs: slices.Clone(psbc)},
	}}
	rs := Collect([]BankCfg{
		{Market: "CN_A", Symbol: "601658.SH", Name: "邮储银行"},
		{Market: "HK", Symbol: "3968.HK", Name: "招商银行H", AShareRef: "600036.SH"},
		{Market: "CN_A", Symbol: "600036.SH", Name: "招商银行"},
	}, src)
	require.Len(t, rs, 2)
	assert.Equal(t, []string{"邮储银行", "招商银行H"}, names(rs), "按首次出现排序，主名取首个条目")
	assert.Equal(t, []string{"招商银行"}, rs[1].Aliases)
	assert.Equal(t, "600036.SH", rs[1].Symbol)
}

func TestCollectHOnly(t *testing.T) {
	src := &fakeSource{series: map[string]Series{"600036.SH": {Obs: slices.Clone(cmb)}}}
	rs := Collect([]BankCfg{{Market: "HK", Symbol: "3968.HK", Name: "招商银行H", AShareRef: "600036.SH"}}, src)
	require.Len(t, rs, 1)
	assert.Equal(t, "招商银行H", rs[0].Name, "只配 H 股时以 H 股名为主名")
	assert.Equal(t, "600036.SH", rs[0].Symbol)
	assert.Empty(t, rs[0].Aliases)
	assert.Equal(t, day("2026-06-30"), rs[0].Latest)
}

func TestCollectKeepsErrors(t *testing.T) {
	timeout := errors.New("timeout")
	src := &fakeSource{
		series: map[string]Series{
			"600036.SH": {Obs: slices.Clone(cmb)},
			"601658.SH": {Obs: slices.Clone(psbc), MissingFields: []string{"HXYJBCZL"}},
		},
		errs: map[string]error{"601658.SH": timeout},
	}
	rs := Collect([]BankCfg{
		{Market: "CN_A", Symbol: "601658.SH", Name: "邮储银行"},
		{Market: "CN_A", Symbol: "600036.SH", Name: "招商银行"},
	}, src)
	require.Len(t, rs, 2)
	assert.Same(t, timeout, rs[0].Err, "保留原错误")
	assert.True(t, rs[0].Latest.IsZero())
	for _, k := range Indicators {
		assertNaNChange(t, rs[0].Ind[k], "失败主体 "+k.Label())
	}
	assert.Empty(t, rs[0].MissingFields, "出错时返回的部分数据不采信")
	assert.NoError(t, rs[1].Err, "其他主体不受影响")
	assert.Equal(t, day("2026-06-30"), rs[1].Latest)
	assert.Equal(t, 0.94, rs[1].Ind[IndNPL].Value)
}
