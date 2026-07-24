package prism

import (
	"errors"
	"math"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
//   functional[0] 增量锚点 LatestDate(无数据 "" → 两行后 2026-07-22) → TestLatestDateEmptyThenAdvances
//   functional[1] NaN↔NULL 往返(NaN 落 NULL,读回 IsNaN;非 NaN 一致)  → TestUpsertValuationsNaNRoundtrip
//   functional[2] Board 每标的最新行(含元数据,AsOf=MAX(d))           → TestBoardReturnsLatestPerInstrument
//   functional[3] Series 按 from 过滤(from="" 全部,升序)             → TestSeriesFrom
//   boundary[0]   UpsertInstrument (symbol,type) 幂等同 id+更新元数据   → TestUpsertInstrumentIdempotent
//   boundary[1]   UpsertValuations (instrument_id,d) 幂等 ON CONFLICT   → TestUpsertValuationsIdempotent
//   error[0]      Open 自动创建多级不存在的父目录(TempDir/a/b/prism.db) → TestOpenCreatesNestedParentDirs

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "prism.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestLatestDateEmptyThenAdvances(t *testing.T) {
	s := openTemp(t)
	id, err := s.UpsertInstrument(Instrument{Symbol: "000300.SH", Type: "index", Market: "CN_A", Name: "沪深300", Group: "A股指数", Source: "lixinger"})
	require.NoError(t, err)

	d, err := s.LatestDate(id)
	require.NoError(t, err)
	assert.Equal(t, "", d)

	require.NoError(t, s.UpsertValuations(id, []ValuationRow{
		{D: "2026-07-21", PETTM: 12.1, PB: 1.3, PSTTM: 1.1, Pctl5Y: 40, Pctl10Y: 35},
		{D: "2026-07-22", PETTM: 12.2, PB: 1.3, PSTTM: 1.1, Pctl5Y: 41, Pctl10Y: 36},
	}))
	d, err = s.LatestDate(id)
	require.NoError(t, err)
	assert.Equal(t, "2026-07-22", d)
}

func TestUpsertValuationsNaNRoundtrip(t *testing.T) {
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, s.UpsertValuations(id, []ValuationRow{
		{D: "2026-07-22", PETTM: 45.5, PB: math.NaN(), PSTTM: math.NaN(), Pctl5Y: 88, Pctl10Y: math.NaN()},
	}))
	got, err := s.Series("NVDA", "")
	require.NoError(t, err)
	require.Len(t, got.Dates, 1)
	assert.Equal(t, 45.5, got.PETTM[0])
	assert.True(t, math.IsNaN(got.Pctl10Y[0]))
	assert.Equal(t, 88.0, got.Pctl5Y[0])
}

func TestBoardReturnsLatestPerInstrument(t *testing.T) {
	s := openTemp(t)
	a, _ := s.UpsertInstrument(Instrument{Symbol: "000300.SH", Type: "index", Market: "CN_A", Name: "沪深300", Group: "A股指数", Source: "lixinger"})
	b, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, s.UpsertValuations(a, []ValuationRow{
		{D: "2026-07-21", PETTM: 12.1, Pctl5Y: 40, Pctl10Y: 35, PB: 1.3, PSTTM: 1.1},
		{D: "2026-07-22", PETTM: 12.2, Pctl5Y: 41, Pctl10Y: 36, PB: 1.3, PSTTM: 1.1},
	}))
	require.NoError(t, s.UpsertValuations(b, []ValuationRow{
		{D: "2026-07-22", PETTM: 45.5, Pctl5Y: 88, Pctl10Y: math.NaN(), PB: math.NaN(), PSTTM: math.NaN()},
	}))
	rows, err := s.Board()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	byID := map[string]BoardRow{}
	for _, r := range rows {
		byID[r.Symbol] = r
	}
	assert.Equal(t, "2026-07-22", byID["000300.SH"].AsOf)
	assert.Equal(t, 12.2, byID["000300.SH"].PETTM)
	assert.Equal(t, "NVIDIA", byID["NVDA"].Name)
}

func TestSeriesFrom(t *testing.T) {
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, s.UpsertValuations(id, []ValuationRow{
		{D: "2021-07-22", PETTM: 60},
		{D: "2026-07-22", PETTM: 45},
	}))
	got, err := s.Series("NVDA", "2025-01-01")
	require.NoError(t, err)
	require.Len(t, got.Dates, 1)
	assert.Equal(t, "2026-07-22", got.Dates[0])

	// from="" 返回全部且升序
	all, err := s.Series("NVDA", "")
	require.NoError(t, err)
	require.Len(t, all.Dates, 2)
	assert.Equal(t, []string{"2021-07-22", "2026-07-22"}, all.Dates)
}

func TestUpsertInstrumentIdempotent(t *testing.T) {
	s := openTemp(t)
	id1, err := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, err)
	// 同 (symbol,type) 重复 upsert:返回同一 id 且更新元数据
	id2, err := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "英伟达", Group: "美股龙头", Source: "engine"})
	require.NoError(t, err)
	assert.Equal(t, id1, id2)

	require.NoError(t, s.UpsertValuations(id2, []ValuationRow{{D: "2026-07-22", PETTM: 45.5}}))
	rows, err := s.Board()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "英伟达", rows[0].Name)
	assert.Equal(t, "美股龙头", rows[0].Group)
}

func TestUpsertValuationsIdempotent(t *testing.T) {
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, s.UpsertValuations(id, []ValuationRow{{D: "2026-07-22", PETTM: 45.5}}))
	// 同 (instrument_id,d) 重复写:幂等更新,不产生重复行
	require.NoError(t, s.UpsertValuations(id, []ValuationRow{{D: "2026-07-22", PETTM: 50.0}}))
	got, err := s.Series("NVDA", "")
	require.NoError(t, err)
	require.Len(t, got.Dates, 1)
	assert.Equal(t, 50.0, got.PETTM[0])
}

func TestOpenCreatesNestedParentDirs(t *testing.T) {
	// error_handling[0]: 多级不存在的父目录应被自动创建
	path := filepath.Join(t.TempDir(), "a", "b", "prism.db")
	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()
	_, err = s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "engine"})
	require.NoError(t, err)
}

func TestSeriesUnknownSymbolReturnsErrNotFound(t *testing.T) {
	// error_handling: 未知 symbol 返回包装的 ErrNotFound(errors.Is 可判)
	s := openTemp(t)
	_, err := s.Series("NOPE", "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound), "未知 symbol 应返回可判定的 ErrNotFound")
	assert.Contains(t, err.Error(), "NOPE")
}

// TASK-002 done_criteria → test mapping:
//   functional[0]/boundary[1] 幂等 upsert + period_end 升序读回 → TestUpsertFundamentalsRoundtrip
//   functional[1]             FundamentalRow 字段契约(逐字段相等) → TestUpsertFundamentalsRoundtrip
//   boundary[0]               Equity NaN 落 NULL 读回 IsNaN        → TestUpsertFundamentalsRoundtrip
//   error_handling[0]         空 rows 不报错且不产生行             → TestUpsertFundamentalsEmpty

func TestUpsertFundamentalsRoundtrip(t *testing.T) {
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "edgar"})
	rows := []FundamentalRow{
		{FiscalPeriod: "2026Q1", PeriodEnd: "2026-04-27", FilingDate: "2026-05-28",
			Revenue: 44e9, NetIncome: 18e9, EPSDiluted: 0.76, Equity: 80e9, DilutedShares: 24.6e9, Source: "edgar"},
		{FiscalPeriod: "2025Q4", PeriodEnd: "2026-01-26", FilingDate: "2026-02-26",
			Revenue: 39e9, NetIncome: 22e9, EPSDiluted: 0.89, Equity: math.NaN(), DilutedShares: 24.7e9, Source: "edgar"},
	}
	require.NoError(t, s.UpsertFundamentals(id, rows))
	require.NoError(t, s.UpsertFundamentals(id, rows)) // 幂等

	got, err := s.QuarterlyFundamentals(id)
	require.NoError(t, err)
	require.Len(t, got, 2)
	// period_end 升序:2026-01-26 在前,2026-04-27 在后
	assert.Equal(t, "2025Q4", got[0].FiscalPeriod, "period_end 升序")
	assert.True(t, math.IsNaN(got[0].Equity), "Equity NaN 落 NULL 读回 IsNaN")
	assert.Equal(t, "2026-02-26", got[0].FilingDate)
	// 第二行(2026Q1)字段逐一相等
	want := rows[0]
	assert.Equal(t, want.FiscalPeriod, got[1].FiscalPeriod)
	assert.Equal(t, want.PeriodEnd, got[1].PeriodEnd)
	assert.Equal(t, want.FilingDate, got[1].FilingDate)
	assert.Equal(t, want.Revenue, got[1].Revenue)
	assert.Equal(t, want.NetIncome, got[1].NetIncome)
	assert.Equal(t, want.EPSDiluted, got[1].EPSDiluted)
	assert.Equal(t, want.Equity, got[1].Equity)
	assert.Equal(t, want.DilutedShares, got[1].DilutedShares)
	assert.Equal(t, want.Source, got[1].Source)
}

func TestUpsertFundamentalsEmpty(t *testing.T) {
	// error_handling[0]: 空 rows 切片 Upsert 不报错且不产生行
	s := openTemp(t)
	id, _ := s.UpsertInstrument(Instrument{Symbol: "NVDA", Type: "stock", Market: "US", Name: "NVIDIA", Group: "美股公司", Source: "edgar"})
	require.NoError(t, s.UpsertFundamentals(id, nil))
	require.NoError(t, s.UpsertFundamentals(id, []FundamentalRow{}))
	got, err := s.QuarterlyFundamentals(id)
	require.NoError(t, err)
	assert.Empty(t, got)
}
