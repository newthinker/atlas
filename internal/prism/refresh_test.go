package prism

import (
	"errors"
	"math"
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
// [TASK-006] engine functional[0] "拉取窗口:价格[now-USLookbackYears,now]/1d,EPS 多回看1年 + 重算落库"
//                                                                        → TestRefreshEnginePath
// [TASK-006] engine error_handling[0] "eps/price history/reconstruct 三处失败带前缀进 Failed"
//                                                                        → TestRefreshEngineFailurePrefixes
// [TASK-006] engine boundary[1] "engine 单标的失败不中断其余标的"           → TestRefreshEnginePartialFailure

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

type priceCall struct {
	start, end time.Time
	interval   string
}

type fakeUS2 struct {
	closes     []core.OHLCV
	eps        []core.EPSPoint
	epsCalls   map[string][2]time.Time // symbol -> {start,end}
	priceCalls map[string]priceCall
	failEPS    map[string]error
	failPrice  map[string]error
}

func (f *fakeUS2) FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error) {
	if f.epsCalls == nil {
		f.epsCalls = map[string][2]time.Time{}
	}
	f.epsCalls[symbol] = [2]time.Time{start, end}
	if err := f.failEPS[symbol]; err != nil {
		return nil, err
	}
	return f.eps, nil
}
func (f *fakeUS2) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if f.priceCalls == nil {
		f.priceCalls = map[string]priceCall{}
	}
	f.priceCalls[symbol] = priceCall{start, end, interval}
	if err := f.failPrice[symbol]; err != nil {
		return nil, err
	}
	return f.closes, nil
}

func TestRefreshEnginePath(t *testing.T) {
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	eps := make([]core.EPSPoint, 8)
	for i := range eps {
		eps[i] = core.EPSPoint{Date: now.AddDate(0, -3*(8-i), 0), EPS: 2.0}
	}
	closes := []core.OHLCV{
		{Time: now.AddDate(0, 0, -2), Close: 100},
		{Time: now.AddDate(0, 0, -1), Close: 110},
	}
	cfg := config.PrismConfig{Instruments: []config.PrismInstrument{
		{Symbol: "NVDA", Name: "NVIDIA", Type: "stock", Market: "US", Group: "美股公司", Source: "engine"},
	}}
	cfg.ApplyDefaults()

	store := newFakeStore()
	us := &fakeUS2{closes: closes, eps: eps}
	rep := Refresh(cfg, store, &fakeLix{}, us, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)
	rows := store.upserts["NVDA"]
	require.Len(t, rows, 2)
	assert.InDelta(t, 50.0, rows[0].PETTM, 1e-9) // 100/2
	assert.InDelta(t, 55.0, rows[1].PETTM, 1e-9) // 110/2
	// 序列只有 2 点,不足 rollingMinPoints → 滚动分位为 NaN(落 NULL)
	assert.True(t, math.IsNaN(rows[0].Pctl5Y))
	// M1 口径:yahoo 路径无 PB/PS,10Y 分位无意义,恒 NaN
	assert.True(t, math.IsNaN(rows[0].PB))
	assert.True(t, math.IsNaN(rows[0].PSTTM))
	assert.True(t, math.IsNaN(rows[0].Pctl10Y))

	// functional[0] 拉取窗口:价格 [now-USLookbackYears, now]/interval=1d,EPS 多回看 1 年
	priceStart := now.AddDate(-cfg.USLookbackYears, 0, 0)
	assert.Equal(t, priceStart, us.priceCalls["NVDA"].start, "价格从 now-USLookbackYears 开始")
	assert.Equal(t, now, us.priceCalls["NVDA"].end, "价格 end=now")
	assert.Equal(t, "1d", us.priceCalls["NVDA"].interval)
	assert.Equal(t, priceStart.AddDate(-1, 0, 0), us.epsCalls["NVDA"][0], "EPS 比价格多回看 1 年")
	assert.Equal(t, now, us.epsCalls["NVDA"][1], "EPS end=now")
}

func engineInst(symbol string) config.PrismInstrument {
	return config.PrismInstrument{Symbol: symbol, Name: symbol, Type: "stock", Market: "US", Group: "美股公司", Source: "engine"}
}

func okEPSSeries(now time.Time) []core.EPSPoint {
	eps := make([]core.EPSPoint, 8)
	for i := range eps {
		eps[i] = core.EPSPoint{Date: now.AddDate(0, -3*(8-i), 0), EPS: 2.0}
	}
	return eps
}

// error_handling[0]:engine 路径三处失败(eps history / price history / reconstruct)
// 各自进 Report.Failed 且带正确前缀。fakeUS2 对每个 symbol 返回同一份数据,故
// 每个失败例单独构造(单标的),前缀验证不受多标的干扰。
func TestRefreshEngineFailurePrefixes(t *testing.T) {
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	okCloses := []core.OHLCV{{Time: now.AddDate(0, 0, -1), Close: 100}}

	tests := []struct {
		name    string
		us      *fakeUS2
		wantSub string
	}{
		{"eps history 失败", &fakeUS2{closes: okCloses, eps: okEPSSeries(now), failEPS: map[string]error{"NVDA": errors.New("boom")}}, "eps history:"},
		{"price history 失败", &fakeUS2{closes: okCloses, eps: okEPSSeries(now), failPrice: map[string]error{"NVDA": errors.New("boom")}}, "price history:"},
		{"reconstruct 失败(EPS 点数 < MinEPSPoints)", &fakeUS2{closes: okCloses, eps: okEPSSeries(now)[:3]}, "reconstruct:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.PrismConfig{Instruments: []config.PrismInstrument{engineInst("NVDA")}}
			cfg.ApplyDefaults()
			rep := Refresh(cfg, newFakeStore(), &fakeLix{}, tc.us, now)
			assert.Equal(t, 0, rep.Refreshed)
			require.Len(t, rep.Failed, 1)
			assert.Contains(t, rep.Failed[0], "NVDA")
			assert.Contains(t, rep.Failed[0], tc.wantSub, "错误须带对应包装前缀")
		})
	}
}

// error_handling[0] / boundary[1]:engine 路径单标的失败不中断其余标的。
// 用 per-symbol failEPS 注入,保证 OK 标的仍能成功。
func TestRefreshEnginePartialFailure(t *testing.T) {
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	us := &fakeUS2{
		closes:  []core.OHLCV{{Time: now.AddDate(0, 0, -1), Close: 100}},
		eps:     okEPSSeries(now),
		failEPS: map[string]error{"BAD": errors.New("boom")},
	}
	cfg := config.PrismConfig{Instruments: []config.PrismInstrument{engineInst("BAD"), engineInst("OK")}}
	cfg.ApplyDefaults()
	store := newFakeStore()

	rep := Refresh(cfg, store, &fakeLix{}, us, now)
	assert.Equal(t, 1, rep.Refreshed, "OK 标的仍应成功")
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "BAD")
	assert.Contains(t, rep.Failed[0], "eps history:")
	require.Len(t, store.upserts["OK"], 1, "OK 标的应落库")
}
