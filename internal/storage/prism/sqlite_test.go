package prism

import (
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
