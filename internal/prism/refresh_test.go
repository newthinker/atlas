package prism

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	prismstore "github.com/newthinker/atlas/internal/storage/prism"
)

// Context Checkpoint: done_criteria → test mapping
// functional[0]     "首次(LatestDate=\"\")从 now-LookbackYears 回填 upsert 落库" → TestRefreshLixingerBackfill
// functional[1]     "已有数据从 latest+1 天增量拉取,end=now"                  → TestRefreshLixingerIncremental
// functional[2]     "Report.Refreshed 计数成功,Failed 收集 \"SYMBOL: error\""  → TestRefreshLixingerBackfill / TestRefreshPartialFailure
// boundary[0]       "latest+1 >= now → 零请求直接成功返回(不调用 Fetch)"       → TestRefreshUpToDateZeroRequest
// boundary[1]       "单标的失败不中断其余标的(部分失败语义)"                   → TestRefreshPartialFailure
// error_handling[0] "未知 source 或 latest 解析失败 → 记入 Failed 并继续"       → TestRefreshUnknownSource / TestRefreshBadLatestDate

type fakeStore struct {
	latest  map[string]string // symbol -> latest date
	upserts map[string][]prismstore.ValuationRow
	ids     map[int64]string
	nextID  int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{latest: map[string]string{}, upserts: map[string][]prismstore.ValuationRow{}, ids: map[int64]string{}}
}
func (f *fakeStore) UpsertInstrument(inst prismstore.Instrument) (int64, error) {
	f.nextID++
	f.ids[f.nextID] = inst.Symbol
	return f.nextID, nil
}
func (f *fakeStore) LatestDate(id int64) (string, error) { return f.latest[f.ids[id]], nil }
func (f *fakeStore) UpsertValuations(id int64, rows []prismstore.ValuationRow) error {
	f.upserts[f.ids[id]] = append(f.upserts[f.ids[id]], rows...)
	return nil
}

type fakeLix struct {
	calls map[string][2]time.Time
	fail  map[string]error
}

func (f *fakeLix) FetchValuationSeries(symbol string, start, end time.Time) ([]lixinger.ValuationPoint, error) {
	if f.calls == nil {
		f.calls = map[string][2]time.Time{}
	}
	f.calls[symbol] = [2]time.Time{start, end}
	if err := f.fail[symbol]; err != nil {
		return nil, err
	}
	return []lixinger.ValuationPoint{{Date: end, PETTM: 12.0, Pctl5Y: 40, Pctl10Y: 35}}, nil
}

type fakeUS struct{}

func (fakeUS) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	return nil, errors.New("not used in this test")
}
func (fakeUS) FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error) {
	return nil, errors.New("not used in this test")
}

func lixCfg() config.PrismConfig {
	c := config.PrismConfig{Instruments: []config.PrismInstrument{
		{Symbol: "000300.SH", Name: "沪深300", Type: "index", Market: "CN_A", Group: "A股指数", Source: "lixinger"},
	}}
	c.ApplyDefaults()
	return c
}

func TestRefreshLixingerBackfill(t *testing.T) {
	store, lix := newFakeStore(), &fakeLix{}
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	rep := Refresh(lixCfg(), store, lix, fakeUS{}, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)

	win := lix.calls["000300.SH"]
	assert.Equal(t, now.AddDate(-10, 0, 0), win[0], "首次回填 lookback_years=10")
	assert.Equal(t, now, win[1], "end=now")
	require.Len(t, store.upserts["000300.SH"], 1)
	assert.Equal(t, 12.0, store.upserts["000300.SH"][0].PETTM)
}

func TestRefreshLixingerIncremental(t *testing.T) {
	store, lix := newFakeStore(), &fakeLix{}
	store.latest["000300.SH"] = "2026-07-20" // 预置 latest → id 映射在 UpsertInstrument 时建立
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	Refresh(lixCfg(), store, lix, fakeUS{}, now)
	win := lix.calls["000300.SH"]
	assert.Equal(t, time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC), win[0], "增量从 latest+1 天开始")
	assert.Equal(t, now, win[1])
}

func TestRefreshUpToDateZeroRequest(t *testing.T) {
	store, lix := newFakeStore(), &fakeLix{}
	store.latest["000300.SH"] = "2026-07-22" // latest+1 == now → 已是最新
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	rep := Refresh(lixCfg(), store, lix, fakeUS{}, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed, "零请求仍算成功")
	_, called := lix.calls["000300.SH"]
	assert.False(t, called, "latest+1 >= now 时不应调用 FetchValuationSeries")
	assert.Empty(t, store.upserts["000300.SH"], "零请求不写库")
}

func TestRefreshPartialFailure(t *testing.T) {
	cfg := lixCfg()
	cfg.Instruments = append(cfg.Instruments, config.PrismInstrument{
		Symbol: "^GSPC", Name: "标普500", Type: "index", Market: "US", Group: "美股指数", Source: "lixinger"})
	store := newFakeStore()
	lix := &fakeLix{fail: map[string]error{"000300.SH": errors.New("boom")}}
	rep := Refresh(cfg, store, lix, fakeUS{}, time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 1, rep.Refreshed, "^GSPC 仍应成功")
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "000300.SH")
	assert.Contains(t, rep.Failed[0], "boom")
}

func TestRefreshUnknownSource(t *testing.T) {
	cfg := config.PrismConfig{Instruments: []config.PrismInstrument{
		{Symbol: "000300.SH", Name: "沪深300", Type: "index", Market: "CN_A", Group: "A股指数", Source: "lixinger"},
		{Symbol: "XYZ", Name: "未知", Type: "index", Market: "CN_A", Group: "杂项", Source: "bogus"},
	}}
	cfg.ApplyDefaults()
	store, lix := newFakeStore(), &fakeLix{}
	rep := Refresh(cfg, store, lix, fakeUS{}, time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 1, rep.Refreshed, "已知 source 标的不受未知 source 影响")
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "XYZ")
	assert.Contains(t, rep.Failed[0], "bogus")
}

func TestRefreshBadLatestDate(t *testing.T) {
	store, lix := newFakeStore(), &fakeLix{}
	store.latest["000300.SH"] = "not-a-date"
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	rep := Refresh(lixCfg(), store, lix, fakeUS{}, now)
	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "000300.SH")
	_, called := lix.calls["000300.SH"]
	assert.False(t, called, "latest 解析失败应在拉取前中止该标的")
}
