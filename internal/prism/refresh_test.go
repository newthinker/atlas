package prism

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	akshare "github.com/newthinker/atlas/internal/collector/akshare"
	"github.com/newthinker/atlas/internal/collector/edgar"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/collector/tushare"
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
	latest       map[string]string // symbol -> latest date
	upserts      map[string][]prismstore.ValuationRow
	fundamentals map[string][]prismstore.FundamentalRow // symbol → 落库季度事实
	ids          map[int64]string
	nextID       int64
	series       map[string]*prismstore.SeriesData // symbol → 预置历史(Series 返回)
	seriesErr    map[string]error                  // symbol → 注入 Series 读回错误
	prices       map[string][]prismstore.PriceRow  // symbol → 落库日收盘(M3)
	priceErr     map[string]error                  // symbol → 注入 UpsertPrices 失败
	priceCalls   int                               // UpsertPrices 调用次数(负向断言用)
	segments     map[string][]prismstore.SegmentRow
	segmentErr   map[string]error // symbol → 注入 UpsertSegments 失败
	fundErr      map[string]error // symbol → 注入 QuarterlyFundamentals 失败
	anchorErr    map[string]error // symbol → 注入 LatestSegmentPeriodEnd 失败
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		latest:       map[string]string{},
		upserts:      map[string][]prismstore.ValuationRow{},
		fundamentals: map[string][]prismstore.FundamentalRow{},
		ids:          map[int64]string{},
		series:       map[string]*prismstore.SeriesData{},
		prices:       map[string][]prismstore.PriceRow{},
		priceErr:     map[string]error{},
		segments:     map[string][]prismstore.SegmentRow{},
		segmentErr:   map[string]error{},
		fundErr:      map[string]error{},
		anchorErr:    map[string]error{},
	}
}

// M3 Store 扩展方法(TASK-002 产出)。refresh 目前只用 UpsertPrices,其余为
// TASK-007/008/009 的消费面,此处按接口如实实现以保证 fake 与真实 Store 同构。
func (f *fakeStore) UpsertPrices(id int64, rows []prismstore.PriceRow) error {
	f.priceCalls++
	sym := f.ids[id]
	if err := f.priceErr[sym]; err != nil {
		return err
	}
	f.prices[sym] = append(f.prices[sym], rows...)
	return nil
}

// UpsertSegments 复刻真实 Store 的 upsert 语义:按 (period_end, segment_key) 覆盖
// 而非追加。RefreshSegments 的 Q4 推导要读回自己刚写的季度行,若 fake 只追加就会
// 读到重复行、让测试在一个真实 Store 不可能出现的状态上通过。
func (f *fakeStore) UpsertSegments(id int64, rows []prismstore.SegmentRow) error {
	sym := f.ids[id]
	if err := f.segmentErr[sym]; err != nil {
		return err
	}
	cur := f.segments[sym]
	for _, r := range rows {
		replaced := false
		for i := range cur {
			if cur[i].PeriodEnd == r.PeriodEnd && cur[i].SegmentKey == r.SegmentKey {
				cur[i] = r
				replaced = true
				break
			}
		}
		if !replaced {
			cur = append(cur, r)
		}
	}
	f.segments[sym] = cur
	return nil
}

// SegmentRows 返回 (period_end, segment_key) 升序的副本,与真实 Store 的 ORDER BY 一致。
func (f *fakeStore) SegmentRows(id int64) ([]prismstore.SegmentRow, error) {
	rows := append([]prismstore.SegmentRow(nil), f.segments[f.ids[id]]...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].PeriodEnd != rows[j].PeriodEnd {
			return rows[i].PeriodEnd < rows[j].PeriodEnd
		}
		return rows[i].SegmentKey < rows[j].SegmentKey
	})
	return rows, nil
}
func (f *fakeStore) LatestSegmentPeriodEnd(id int64) (string, error) {
	sym := f.ids[id]
	if err := f.anchorErr[sym]; err != nil {
		return "", err
	}
	var latest string
	for _, r := range f.segments[sym] {
		if r.PeriodEnd > latest {
			latest = r.PeriodEnd
		}
	}
	return latest, nil
}
func (f *fakeStore) QuarterlyFundamentals(id int64) ([]prismstore.FundamentalRow, error) {
	sym := f.ids[id]
	if err := f.fundErr[sym]; err != nil {
		return nil, err
	}
	return f.fundamentals[sym], nil
}
func (f *fakeStore) UpsertFundamentals(id int64, rows []prismstore.FundamentalRow) error {
	f.fundamentals[f.ids[id]] = append(f.fundamentals[f.ids[id]], rows...)
	return nil
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
func (f *fakeStore) Series(symbol, from string) (*prismstore.SeriesData, error) {
	if err := f.seriesErr[symbol]; err != nil {
		return nil, err
	}
	if sd, ok := f.series[symbol]; ok {
		return sd, nil
	}
	return &prismstore.SeriesData{Symbol: symbol}, nil
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

type fakeAkshare struct {
	stockCalls map[string][2]time.Time // symbol → [start,end](参数捕获)
	indexCalls map[string][2]time.Time
	stockPts   []akshare.ValuationPoint
	indexPts   []akshare.ValuationPoint
	fail       map[string]error
}

func (f *fakeAkshare) FetchStockValuationSeries(symbol, market string, start, end time.Time) ([]akshare.ValuationPoint, error) {
	if f.stockCalls == nil {
		f.stockCalls = map[string][2]time.Time{}
	}
	f.stockCalls[symbol] = [2]time.Time{start, end}
	if err := f.fail[symbol]; err != nil {
		return nil, err
	}
	return f.stockPts, nil
}
func (f *fakeAkshare) FetchIndexValuationSeries(symbol string, start, end time.Time) ([]akshare.ValuationPoint, error) {
	if f.indexCalls == nil {
		f.indexCalls = map[string][2]time.Time{}
	}
	f.indexCalls[symbol] = [2]time.Time{start, end}
	if err := f.fail[symbol]; err != nil {
		return nil, err
	}
	return f.indexPts, nil
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
	rep := Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, nil, nil, now)
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
	Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, nil, nil, now)
	win := lix.calls["000300.SH"]
	assert.Equal(t, time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC), win[0], "增量从 latest+1 天开始")
	assert.Equal(t, now, win[1])
}

func TestRefreshUpToDateZeroRequest(t *testing.T) {
	store, lix := newFakeStore(), &fakeLix{}
	store.latest["000300.SH"] = "2026-07-22" // latest+1 == now → 已是最新
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	rep := Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, nil, nil, now)
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
	rep := Refresh(cfg, store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, nil, nil, time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC))
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
	rep := Refresh(cfg, store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, nil, nil, time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 1, rep.Refreshed, "已知 source 标的不受未知 source 影响")
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "XYZ")
	assert.Contains(t, rep.Failed[0], "bogus")
}

func TestRefreshBadLatestDate(t *testing.T) {
	store, lix := newFakeStore(), &fakeLix{}
	store.latest["000300.SH"] = "not-a-date"
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	rep := Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, nil, nil, now)
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
	rep := Refresh(cfg, store, &fakeLix{}, us, &fakeAkshare{}, fakeEdgar{}, nil, nil, now)
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

func engineCfg() config.PrismConfig {
	c := config.PrismConfig{Instruments: []config.PrismInstrument{engineInst("NVDA")}}
	c.ApplyDefaults()
	return c
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
			rep := Refresh(cfg, newFakeStore(), &fakeLix{}, tc.us, &fakeAkshare{}, fakeEdgar{}, nil, nil, now)
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

	rep := Refresh(cfg, store, &fakeLix{}, us, &fakeAkshare{}, fakeEdgar{}, nil, nil, now)
	assert.Equal(t, 1, rep.Refreshed, "OK 标的仍应成功")
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "BAD")
	assert.Contains(t, rep.Failed[0], "eps history:")
	require.Len(t, store.upserts["OK"], 1, "OK 标的应落库")
}

// [TASK-006 fix MAJOR-1] 时区安全:latest 为宿主本地当日时,即便宿主处于西经
// 时区(now 的 UTC 日期已跨到次日),也应按日历日判定"已是最新"零请求,绝不
// 产生 startDate>endDate 的倒挂请求(理杏豆浪费)。
func TestRefreshLixingerTimezoneSafeUpToDate(t *testing.T) {
	west := time.FixedZone("PST", -8*3600)
	// 本地 2026-07-23 20:00(此刻 UTC 已是 2026-07-24 04:00)
	now := time.Date(2026, 7, 23, 20, 0, 0, 0, west)
	store, lix := newFakeStore(), &fakeLix{}
	store.latest["000300.SH"] = "2026-07-23" // 本地当日已有数据
	rep := Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, nil, nil, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed, "已是最新仍算成功")
	_, called := lix.calls["000300.SH"]
	assert.False(t, called, "本地当日已最新应零请求,绝不发倒挂请求")
	assert.Empty(t, store.upserts["000300.SH"])
}

// [TASK-006 fix MAJOR-1] 增量请求区间恒 start<=end(日历日),不受宿主时区影响。
func TestRefreshLixingerTimezoneSafeIncremental(t *testing.T) {
	west := time.FixedZone("PST", -8*3600)
	now := time.Date(2026, 7, 23, 20, 0, 0, 0, west) // 本地 07-23
	store, lix := newFakeStore(), &fakeLix{}
	store.latest["000300.SH"] = "2026-07-20"
	Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, nil, nil, now)
	win, called := lix.calls["000300.SH"]
	require.True(t, called, "有增量应发请求")
	assert.Equal(t, "2026-07-21", win[0].Format("2006-01-02"), "增量从 latest+1 天开始")
	assert.LessOrEqual(t, win[0].Format("2006-01-02"), win[1].Format("2006-01-02"),
		"请求区间日历日不得倒挂")
}

// [TASK-006 fix MAJOR-2] refreshEngine 当前 EPS(TTM)<=0 熔断:历史有 >=8 正 EPS
// 季但最新一季亏损的标的不入库,记入 Report.Failed(语义对齐 ErrNonPositiveEPS),
// 避免 board 展示截断到最后盈利日的过期序列。
func TestRefreshEngineNonPositiveCurrentEPS(t *testing.T) {
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	// 9 个季度点:前 8 正(满足 MinEPSPoints),最新(日期最晚)一季亏损
	eps := make([]core.EPSPoint, 9)
	for i := range eps {
		eps[i] = core.EPSPoint{Date: now.AddDate(0, -3*(9-i), 0), EPS: 2.0}
	}
	eps[8].EPS = -1.0
	us := &fakeUS2{closes: []core.OHLCV{{Time: now.AddDate(0, 0, -1), Close: 100}}, eps: eps}
	cfg := config.PrismConfig{Instruments: []config.PrismInstrument{engineInst("LOSS")}}
	cfg.ApplyDefaults()
	store := newFakeStore()

	rep := Refresh(cfg, store, &fakeLix{}, us, &fakeAkshare{}, fakeEdgar{}, nil, nil, now)
	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "LOSS")
	assert.Contains(t, rep.Failed[0], "non-positive", "错误语义应对齐 ErrNonPositiveEPS")
	assert.Empty(t, store.upserts["LOSS"], "当前亏损标的不得入库")
}

// [M3 TASK-008 补测:钉住 TASK-006 的落库时点]
// upsertPrices 位于「取得 closes 之后、EPS 熔断判断之前」,故熔断标的仍写 price_daily。
// 这是经裁决接受的行为(价格是原始事实、估值是派生计算,失败语义不该传染;亏损股恰恰
// 最需要基本面页的股价叠加),但此前无任何测试断言它 —— 后人把 upsertPrices 挪到熔断
// 之后会静默改变行为且无测试变红。本用例钉死四件事,任一被改动即变红。
func TestRefreshEngineNonPositiveEPSStillStoresPrices(t *testing.T) {
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	eps := make([]core.EPSPoint, 9)
	for i := range eps {
		eps[i] = core.EPSPoint{Date: now.AddDate(0, -3*(9-i), 0), EPS: 2.0}
	}
	eps[8].EPS = -1.0 // 最新一季亏损 → 熔断
	closes := []core.OHLCV{
		{Time: now.AddDate(0, 0, -2), Close: 12.5},
		{Time: now.AddDate(0, 0, -1), Close: 11.0},
	}
	cfg := config.PrismConfig{Instruments: []config.PrismInstrument{engineInst("LOSS")}}
	cfg.ApplyDefaults()
	store := newFakeStore()

	rep := Refresh(cfg, store, &fakeLix{}, &fakeUS2{closes: closes, eps: eps}, &fakeAkshare{}, fakeEdgar{}, nil, nil, now)

	// 1) 行情仍落库,且逐点与 closes 一致
	prices := store.prices["LOSS"]
	require.Len(t, prices, 2, "熔断不得连带丢弃行情(价格是原始事实)")
	assert.Equal(t, closes[0].Time.Format("2006-01-02"), prices[0].D)
	assert.Equal(t, 12.5, prices[0].Close)
	assert.Equal(t, 11.0, prices[1].Close)
	// 2) 该标的仍进 Failed
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "LOSS")
	assert.Contains(t, rep.Failed[0], "non-positive")
	// 3) 估值不落库
	assert.Empty(t, store.upserts["LOSS"], "熔断标的的 valuation_daily 仍不得写")
	// 4) 不计入 Refreshed
	assert.Equal(t, 0, rep.Refreshed)
}

// Context Checkpoint: TASK-004 done_criteria → test mapping
// functional[0]     "公司标的增量 latest+1 起拉取,新点 PE/PB/PS 落库"          → TestRefreshAkshareStockIncremental
// functional[1]     "300 天历史 PE=10 + 新点 PE=20 → pctl_5y=pctl_10y=100;只写新点" → TestRefreshAkshareLocalPercentile
// functional[2]     "index 走 FetchIndexValuationSeries、stock 走 FetchStockValuationSeries" → TestRefreshAkshareIndexDispatch
// boundary[0]       "空拉取→零写入仍计成功"                                      → TestRefreshAkshareEmptyFetch
// error_handling[0] "akshare 直连失败→Failed 含 symbol 与原因且 Refreshed=0"     → TestRefreshAkshareFetchFailure
// error_handling[0] "store.Series 读回失败→同样进 Failed"                        → TestRefreshAkshareSeriesReadbackFailure

func akCfg(inst config.PrismInstrument) config.PrismConfig {
	c := config.PrismConfig{Instruments: []config.PrismInstrument{inst}}
	c.ApplyDefaults()
	return c
}

func TestRefreshAkshareStockIncremental(t *testing.T) {
	store, ak := newFakeStore(), &fakeAkshare{stockPts: []akshare.ValuationPoint{
		{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), PETTM: 22.7, PB: 8.2, PSTTM: math.NaN()},
	}}
	store.latest["600519.SH"] = "2026-07-22"
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	inst := config.PrismInstrument{Symbol: "600519.SH", Name: "贵州茅台", Type: "stock", Market: "CN_A", Group: "A股公司", Source: "akshare"}

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, nil, nil, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)
	win := ak.stockCalls["600519.SH"]
	assert.Equal(t, "2026-07-23", win[0].Format("2006-01-02"), "增量从 latest+1")
	assert.Equal(t, now, win[1], "end=now")
	require.Len(t, store.upserts["600519.SH"], 1)
	assert.Equal(t, 22.7, store.upserts["600519.SH"][0].PETTM)
	assert.Equal(t, 8.2, store.upserts["600519.SH"][0].PB)
	assert.Equal(t, "2026-07-23", store.upserts["600519.SH"][0].D)
}

func TestRefreshAkshareLocalPercentile(t *testing.T) {
	// 预置 300 天历史(PE 全为 10),新点 PE=20 → 新点在窗口内为最大值,pctl_5y=100;
	// 样本 300>=252 满足 minPoints。
	hist := &prismstore.SeriesData{Symbol: "600519.SH"}
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 300; i++ {
		hist.Dates = append(hist.Dates, base.AddDate(0, 0, i).Format("2006-01-02"))
		hist.PETTM = append(hist.PETTM, 10.0)
	}
	store := newFakeStore()
	store.series["600519.SH"] = hist
	store.latest["600519.SH"] = hist.Dates[len(hist.Dates)-1]

	newDay := base.AddDate(0, 0, 300)
	ak := &fakeAkshare{stockPts: []akshare.ValuationPoint{{Date: newDay, PETTM: 20.0, PB: math.NaN(), PSTTM: math.NaN()}}}
	now := newDay.AddDate(0, 0, 1)
	inst := config.PrismInstrument{Symbol: "600519.SH", Type: "stock", Market: "CN_A", Source: "akshare"}

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, nil, nil, now)
	require.Empty(t, rep.Failed)
	rows := store.upserts["600519.SH"]
	require.Len(t, rows, 1, "只写新点,历史行不回写")
	assert.InDelta(t, 100.0, rows[0].Pctl5Y, 1e-9, "新点为窗口最大值→100 分位")
	assert.InDelta(t, 100.0, rows[0].Pctl10Y, 1e-9)
}

func TestRefreshAkshareIndexDispatch(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{indexPts: []akshare.ValuationPoint{
		{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), PETTM: 14.6, PB: math.NaN(), PSTTM: math.NaN()},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	inst := config.PrismInstrument{Symbol: "000300.SH", Type: "index", Market: "CN_A", Source: "akshare"}

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, nil, nil, now)
	assert.Empty(t, rep.Failed)
	_, usedIndex := ak.indexCalls["000300.SH"]
	assert.True(t, usedIndex, "index 类型必须走 FetchIndexValuationSeries")
	assert.Empty(t, ak.stockCalls)
}

func TestRefreshAkshareEmptyFetch(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{} // 返回空切片
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	inst := config.PrismInstrument{Symbol: "600519.SH", Type: "stock", Market: "CN_A", Source: "akshare"}

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, nil, nil, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)
	assert.Empty(t, store.upserts["600519.SH"], "空拉取零写入")
}

// error_handling[0] ①:akshare 直连失败(fakeAkshare.fail 注入)→ Report.Failed
// 含 symbol 与原因且 Refreshed=0;并断言失败前确已发起拉取(参数捕获)。
func TestRefreshAkshareFetchFailure(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	inst := config.PrismInstrument{Symbol: "600519.SH", Type: "stock", Market: "CN_A", Source: "akshare"}

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, nil, nil, now)
	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "600519.SH")
	assert.Contains(t, rep.Failed[0], "aktools down")
	_, called := ak.stockCalls["600519.SH"]
	assert.True(t, called, "失败前必须已发起 stock 拉取")
	assert.Empty(t, store.upserts["600519.SH"], "拉取失败不得写库")
}

// error_handling[0] ②:store.Series 读回失败(seriesErr 注入)→ 同样进 Failed。
// 需 fetch 返回非空点触发读回路径。
func TestRefreshAkshareSeriesReadbackFailure(t *testing.T) {
	store := newFakeStore()
	store.seriesErr = map[string]error{"600519.SH": errors.New("db locked")}
	ak := &fakeAkshare{stockPts: []akshare.ValuationPoint{
		{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), PETTM: 22.7, PB: 8.2, PSTTM: math.NaN()},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	inst := config.PrismInstrument{Symbol: "600519.SH", Type: "stock", Market: "CN_A", Source: "akshare"}

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, nil, nil, now)
	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "600519.SH")
	assert.Contains(t, rep.Failed[0], "db locked")
	assert.Contains(t, rep.Failed[0], "read back series", "读回失败须带前缀")
	assert.Empty(t, store.upserts["600519.SH"], "读回失败不得写库")
}

// Context Checkpoint (TASK-005): done_criteria → test mapping
//   functional[0/1] 主源败+兜底成→Degraded+Refreshed+兜底行落库, 格式串双断言 → TestRefreshFallbackDegraded
//   error_handling  双源皆败→Failed 单条含两原因, Degraded 空                → TestRefreshFallbackBothFail
//   boundary        主源成→不触碰 akshare(indexCalls 空, 零多余请求)         → TestRefreshFallbackNotTriggered

// fbInst 是配置了 lixinger 主源 + akshare 兜底的指数标的。
func fbInst() config.PrismInstrument {
	return config.PrismInstrument{Symbol: "000300.SH", Name: "沪深300", Type: "index",
		Market: "CN_A", Group: "A股指数", Source: "lixinger", FallbackSource: "akshare"}
}

func TestRefreshFallbackDegraded(t *testing.T) {
	store := newFakeStore()
	lix := &fakeLix{fail: map[string]error{"000300.SH": errors.New("quota exhausted")}}
	ak := &fakeAkshare{indexPts: []akshare.ValuationPoint{
		{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), PETTM: 14.6, PB: math.NaN(), PSTTM: math.NaN()},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(fbInst()), store, lix, fakeUS{}, ak, fakeEdgar{}, nil, nil, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed, "兜底成功计入 Refreshed")
	require.Len(t, rep.Degraded, 1)
	assert.Contains(t, rep.Degraded[0], "000300.SH")
	assert.Contains(t, rep.Degraded[0], "lixinger failed", "格式串前段")
	assert.Contains(t, rep.Degraded[0], "quota exhausted", "含主源失败原因")
	assert.Contains(t, rep.Degraded[0], "akshare fallback ok", "格式串后段")
	require.Len(t, store.upserts["000300.SH"], 1, "兜底行已写入")
}

func TestRefreshFallbackBothFail(t *testing.T) {
	store := newFakeStore()
	lix := &fakeLix{fail: map[string]error{"000300.SH": errors.New("lix down")}}
	ak := &fakeAkshare{fail: map[string]error{"000300.SH": errors.New("aktools down")}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(fbInst()), store, lix, fakeUS{}, ak, fakeEdgar{}, nil, nil, now)
	assert.Equal(t, 0, rep.Refreshed)
	assert.Empty(t, rep.Degraded)
	require.Len(t, rep.Failed, 1, "双败合并为单条")
	assert.Contains(t, rep.Failed[0], "lix down")
	assert.Contains(t, rep.Failed[0], "aktools down")
}

func TestRefreshFallbackNotTriggered(t *testing.T) {
	store, lix, ak := newFakeStore(), &fakeLix{}, &fakeAkshare{}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(fbInst()), store, lix, fakeUS{}, ak, fakeEdgar{}, nil, nil, now)
	assert.Empty(t, rep.Failed)
	assert.Empty(t, rep.Degraded)
	assert.Empty(t, ak.indexCalls, "主源成功不得触碰兜底源(零多余请求)")
	assert.Empty(t, ak.stockCalls)
}

// Context Checkpoint (TASK-004): done_criteria → test mapping
//   functional[0]     edgar 主源:facts 落 fundamental_q + filing 生效 PE + PB/PS → TestRefreshEdgarPath
//   functional[1]     EPS_TTM 4 季滑窗产点;挖一季 NaN → 少一个 TTM 点(加强项)   → TestRefreshEdgarTTMGap
//   boundary[0]       inst.CIK 为空 → 提示配置 cik 的错误                        → TestRefreshEdgarMissingCIK
//   error_handling[0] edgar 失败 + FallbackSource==engine → engine 出数 + Degraded → TestRefreshEdgarFallback
//   error_handling[1] edgar 失败 + FallbackSource 空/非 engine → Failed、Degraded 空 → TestRefreshEdgarNoFallback

type fakeEdgar struct {
	facts []edgar.QuarterlyFact
	err   error
}

func (f fakeEdgar) FetchCompanyFacts(cik string) ([]edgar.QuarterlyFact, error) {
	return f.facts, f.err
}

func edgarQuarters(now time.Time) []edgar.QuarterlyFact {
	// 12 个季度,EPS=1.0/季 → TTM=4.0;equity=80、shares=10 → BVPS=8;revenue=20/季 → RPS_TTM=8
	out := make([]edgar.QuarterlyFact, 12)
	for i := range out {
		end := now.AddDate(0, -3*(12-i), 0)
		out[i] = edgar.QuarterlyFact{
			FiscalPeriod: fmt.Sprintf("FY%dQ%d", i/4, i%4+1), PeriodEnd: end,
			FilingDate: end.AddDate(0, 0, 40),
			EPSDiluted: 1.0, Revenue: 20, NetIncome: 15, Equity: 80, DilutedShares: 10,
		}
	}
	return out
}

func edgarCfg() config.PrismConfig {
	c := config.PrismConfig{Instruments: []config.PrismInstrument{
		{Symbol: "NVDA", Name: "NVIDIA", Type: "stock", Market: "US", Group: "美股公司",
			Source: "edgar", CIK: "1045810", FallbackSource: "engine"},
	}}
	c.ApplyDefaults()
	return c
}

// QA WARNING-2:ttmPoints 4 季滑窗须校验季度连续性——整季缺失(非 NaN,fundamental_q
// 根本没有该季行)时窗口会跨缺口求和,TTM 失真。删除 12 季中间一季 → 跨缺口窗口不产点。
// 实测(edgarQuarters 3 月间距):正常窗口跨度 273~275 天,单季缺口窗口 365 天。
func TestTTMPointsQuarterGap(t *testing.T) {
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	full := edgarQuarters(now)
	valFn := func(f edgar.QuarterlyFact) float64 { return f.EPSDiluted }
	require.Len(t, ttmPoints(full, valFn), 9, "12 季无缺口 → 9 个 TTM 点")

	// 整条删除中间一季(orig index 6),而非置 NaN
	gapped := make([]edgar.QuarterlyFact, 0, 11)
	gapped = append(gapped, full[:6]...)
	gapped = append(gapped, full[7:]...)
	// 11 季 → 8 个候选窗口,其中 3 个跨 6 月缺口(365 天)须被守卫剔除 → 5 个点
	pts := ttmPoints(gapped, valFn)
	assert.Len(t, pts, 5, "跨缺口窗口(365 天)不产点,仅保留连续 4 季窗口")
	// 产出的每个点其窗口跨度都应是正常的 ~9 个月,不含跨缺口的年跨度
	for _, p := range pts {
		assert.NotEqual(t, full[6].PeriodEnd, p.Date, "缺失季不应作为任何 TTM 点日期")
	}
}

func TestRefreshEdgarPath(t *testing.T) {
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	facts := edgarQuarters(now)
	lastFiled := facts[11].FilingDate
	closes := []core.OHLCV{
		{Time: lastFiled.AddDate(0, 0, -2), Close: 40}, // 最后一季 filing 前
		{Time: lastFiled.AddDate(0, 0, 2), Close: 40},  // filing 后(TTM 相同=4.0)
	}
	store := newFakeStore()
	rep := Refresh(edgarCfg(), store, &fakeLix{}, &fakeUS2{closes: closes}, &fakeAkshare{}, fakeEdgar{facts: facts}, nil, nil, now)
	require.Empty(t, rep.Failed)
	assert.Empty(t, rep.Degraded)
	assert.Len(t, store.fundamentals["NVDA"], 12, "季度事实落 fundamental_q")

	rows := store.upserts["NVDA"]
	require.Len(t, rows, 2)
	assert.InDelta(t, 10.0, rows[0].PETTM, 1e-9) // 40/4
	assert.InDelta(t, 5.0, rows[0].PB, 1e-9)     // 40/8
	assert.InDelta(t, 5.0, rows[0].PSTTM, 1e-9)  // 40/8
}

// functional[1] 加强项:挖去一季 EPS=NaN,该季所在的 4 个 TTM 窗口不产点。
func TestRefreshEdgarTTMGap(t *testing.T) {
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	full := edgarQuarters(now)
	facts := edgarQuarters(now)
	facts[6].EPSDiluted = math.NaN() // 挖去第 7 季 EPS

	closes := []core.OHLCV{{Time: full[11].FilingDate.AddDate(0, 0, 2), Close: 40}}
	storeFull, storeGap := newFakeStore(), newFakeStore()
	Refresh(edgarCfg(), storeFull, &fakeLix{}, &fakeUS2{closes: closes}, &fakeAkshare{}, fakeEdgar{facts: full}, nil, nil, now)
	Refresh(edgarCfg(), storeGap, &fakeLix{}, &fakeUS2{closes: closes}, &fakeAkshare{}, fakeEdgar{facts: facts}, nil, nil, now)
	// 缺口季落在 i=6..9 共 4 个 TTM 窗口 → 少 4 个 EPS 点。仍有足量点则 PE 列存在;
	// 断言 fundamental_q 仍全量落库(缺口不影响事实落库),PE 阶梯少一档跳变。
	assert.Len(t, storeGap.fundamentals["NVDA"], 12, "缺口季仍作为事实落库")
	// 直接验证 ttmPoints 缺口语义(纯函数,无副作用)
	assert.Len(t, ttmPoints(full, func(f edgar.QuarterlyFact) float64 { return f.EPSDiluted }), 9)
	assert.Len(t, ttmPoints(facts, func(f edgar.QuarterlyFact) float64 { return f.EPSDiluted }), 5,
		"挖一季 → 该季所在 4 个 TTM 窗口不产点,9-4=5")
}

func TestRefreshEdgarMissingCIK(t *testing.T) {
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	cfg := config.PrismConfig{Instruments: []config.PrismInstrument{
		{Symbol: "NVDA", Name: "NVIDIA", Type: "stock", Market: "US", Group: "美股公司", Source: "edgar"},
	}}
	cfg.ApplyDefaults()
	store := newFakeStore()
	rep := Refresh(cfg, store, &fakeLix{}, &fakeUS2{}, &fakeAkshare{}, fakeEdgar{facts: edgarQuarters(now)}, nil, nil, now)
	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "NVDA")
	assert.Contains(t, rep.Failed[0], "cik", "错误须提示配置 cik")
}

// boundary[1](edgar 路径):Equity 全缺失 → BVPS 无点、RPS 仍在 → PB 整列 NaN,
// 而 PE 列不受影响(仍为 10)。锁定「PB/PS 缺科目降级不污染 PE」。
func TestRefreshEdgarMissingEquityNaNPB(t *testing.T) {
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	facts := edgarQuarters(now)
	for i := range facts {
		facts[i].Equity = math.NaN() // 挖掉净资产 → BVPS 不可算
	}
	lastFiled := facts[11].FilingDate
	closes := []core.OHLCV{
		{Time: lastFiled.AddDate(0, 0, -2), Close: 40},
		{Time: lastFiled.AddDate(0, 0, 2), Close: 40},
	}
	store := newFakeStore()
	rep := Refresh(edgarCfg(), store, &fakeLix{}, &fakeUS2{closes: closes}, &fakeAkshare{}, fakeEdgar{facts: facts}, nil, nil, now)
	require.Empty(t, rep.Failed)
	rows := store.upserts["NVDA"]
	require.Len(t, rows, 2)
	assert.InDelta(t, 10.0, rows[0].PETTM, 1e-9, "PE 列不受 PB 缺科目影响")
	assert.True(t, math.IsNaN(rows[0].PB), "Equity 缺失 → PB 整列 NaN")
	assert.True(t, math.IsNaN(rows[1].PB))
}

func TestRefreshEdgarFallback(t *testing.T) {
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	// engine fallback 需要 yahoo EPS+closes(复用既有 fakeUS2 构造)
	eps := make([]core.EPSPoint, 8)
	for i := range eps {
		eps[i] = core.EPSPoint{Date: now.AddDate(0, -3*(8-i), 0), EPS: 2.0}
	}
	closes := []core.OHLCV{{Time: now.AddDate(0, 0, -1), Close: 100}}
	store := newFakeStore()
	rep := Refresh(edgarCfg(), store, &fakeLix{}, &fakeUS2{closes: closes, eps: eps},
		&fakeAkshare{}, fakeEdgar{err: errors.New("edgar down")}, nil, nil, now)
	require.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)
	require.Len(t, rep.Degraded, 1)
	assert.Contains(t, rep.Degraded[0], "NVDA")
	assert.InDelta(t, 50.0, store.upserts["NVDA"][0].PETTM, 1e-9, "fallback 走 yahoo 重建")
}

// error_handling[1](reviewer 增补):edgar 失败且 FallbackSource 为空(或非 engine)
// → Report.Failed 含该标的、Degraded 为空(不静默吞错,防配错 fallback_source 悄悄不出数)。
func TestRefreshEdgarNoFallback(t *testing.T) {
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	cfg := config.PrismConfig{Instruments: []config.PrismInstrument{
		{Symbol: "NVDA", Name: "NVIDIA", Type: "stock", Market: "US", Group: "美股公司",
			Source: "edgar", CIK: "1045810"}, // 无 FallbackSource
	}}
	cfg.ApplyDefaults()
	store := newFakeStore()
	rep := Refresh(cfg, store, &fakeLix{}, &fakeUS2{}, &fakeAkshare{}, fakeEdgar{err: errors.New("edgar down")}, nil, nil, now)
	assert.Equal(t, 0, rep.Refreshed)
	assert.Empty(t, rep.Degraded, "无兜底不得记 Degraded")
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "NVDA")
	assert.Contains(t, rep.Failed[0], "edgar down", "失败原因不得静默吞掉")
}

// ---------------------------------------------------------------------------
// M3 TASK-006 done_criteria → test mapping
//   functional[0] refreshEdgar 落库携带主干流 5 新字段(含 NaN 透传)
//                                            → TestRefreshEdgarStoresMainFlowFields
//   functional[1] refreshEdgar 取得 closes 后 UpsertPrices,日期与收盘价一致
//                                            → TestRefreshEdgarUpsertsPrices
//   functional[2] refreshEngine 同样 UpsertPrices(两条路径都覆盖)
//                                            → TestRefreshEngineUpsertsPrices
//   boundary[0]   UpsertPrices 失败仍完成估值:Refreshed 计数、错误进 Degraded 非 Failed
//                                            → TestRefreshPriceUpsertFailureDegradesOnly
//   error[0]      lixinger/akshare 路径不调用 UpsertPrices(A/H 链路零变更负向断言)
//                                            → TestRefreshCNPathsNeverUpsertPrices
// ---------------------------------------------------------------------------

// mainFlowQuarters 在 edgarQuarters 基础上填入主干流科目,第 6 季的 SGnA 置 NaN
// 用于验证 NaN 透传(不被 0 值污染)。
func mainFlowQuarters(now time.Time) []edgar.QuarterlyFact {
	facts := edgarQuarters(now)
	for i := range facts {
		facts[i].GrossProfit = 12
		facts[i].RnD = 3
		facts[i].SGnA = 2
		facts[i].OperatingIncome = 7
		facts[i].IncomeTax = 1.5
	}
	facts[6].SGnA = math.NaN()
	return facts
}

func TestRefreshEdgarStoresMainFlowFields(t *testing.T) {
	// functional[0]: QuarterlyFact 的 5 个 M3 字段必须原样出现在 FundamentalRow 上
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	facts := mainFlowQuarters(now)
	closes := []core.OHLCV{{Time: facts[11].FilingDate.AddDate(0, 0, 2), Close: 40}}
	store := newFakeStore()
	rep := Refresh(edgarCfg(), store, &fakeLix{}, &fakeUS2{closes: closes}, &fakeAkshare{},
		fakeEdgar{facts: facts}, nil, nil, now)
	require.Empty(t, rep.Failed)

	rows := store.fundamentals["NVDA"]
	require.Len(t, rows, 12)
	for i, r := range rows {
		assert.Equal(t, 12.0, r.GrossProfit, "第 %d 季 GrossProfit", i)
		assert.Equal(t, 3.0, r.RnD, "第 %d 季 RnD", i)
		assert.Equal(t, 7.0, r.OperatingIncome, "第 %d 季 OperatingIncome", i)
		assert.Equal(t, 1.5, r.IncomeTax, "第 %d 季 IncomeTax", i)
	}
	// NaN 透传:缺失科目不得被写成 0
	assert.True(t, math.IsNaN(rows[6].SGnA), "缺失的 SGnA 须以 NaN 落库,不得变成 0")
	assert.Equal(t, 2.0, rows[5].SGnA, "其余季 SGnA 不受影响")
	// 既有字段仍正确(接线不得破坏原映射)
	assert.Equal(t, 20.0, rows[0].Revenue)
	assert.Equal(t, "edgar", rows[0].Source)
}

func TestRefreshEdgarUpsertsPrices(t *testing.T) {
	// functional[1]: closes 落 price_daily,日期与收盘价逐点一致
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	facts := edgarQuarters(now)
	lastFiled := facts[11].FilingDate
	closes := []core.OHLCV{
		{Time: lastFiled.AddDate(0, 0, -2), Close: 40},
		{Time: lastFiled.AddDate(0, 0, 2), Close: 44.5},
	}
	store := newFakeStore()
	rep := Refresh(edgarCfg(), store, &fakeLix{}, &fakeUS2{closes: closes}, &fakeAkshare{},
		fakeEdgar{facts: facts}, nil, nil, now)
	require.Empty(t, rep.Failed)
	assert.Empty(t, rep.Degraded, "落价成功不得记 Degraded")

	prices := store.prices["NVDA"]
	require.Len(t, prices, 2)
	assert.Equal(t, closes[0].Time.Format("2006-01-02"), prices[0].D)
	assert.Equal(t, 40.0, prices[0].Close)
	assert.Equal(t, closes[1].Time.Format("2006-01-02"), prices[1].D)
	assert.Equal(t, 44.5, prices[1].Close)
}

func TestRefreshEngineUpsertsPrices(t *testing.T) {
	// functional[2]: engine 路径同样落 price_daily
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	closes := []core.OHLCV{
		{Time: now.AddDate(0, 0, -2), Close: 100},
		{Time: now.AddDate(0, 0, -1), Close: 110},
	}
	store := newFakeStore()
	rep := Refresh(engineCfg(), store, &fakeLix{}, &fakeUS2{closes: closes, eps: okEPSSeries(now)},
		&fakeAkshare{}, fakeEdgar{}, nil, nil, now)
	require.Empty(t, rep.Failed)
	assert.Empty(t, rep.Degraded)

	prices := store.prices["NVDA"]
	require.Len(t, prices, 2)
	assert.Equal(t, closes[0].Time.Format("2006-01-02"), prices[0].D)
	assert.Equal(t, 100.0, prices[0].Close)
	assert.Equal(t, 110.0, prices[1].Close)
}

func TestRefreshPriceUpsertFailureDegradesOnly(t *testing.T) {
	// boundary[0]: 落价失败只降级,估值主流程照常完成(Refreshed 计数、估值行仍落库)
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	facts := edgarQuarters(now)
	closes := []core.OHLCV{
		{Time: facts[11].FilingDate.AddDate(0, 0, -2), Close: 40},
		{Time: facts[11].FilingDate.AddDate(0, 0, 2), Close: 40},
	}
	for _, tc := range []struct {
		name string
		cfg  config.PrismConfig
		ed   fakeEdgar
	}{
		{"edgar 路径", edgarCfg(), fakeEdgar{facts: facts}},
		{"engine 路径", engineCfg(), fakeEdgar{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			store.priceErr["NVDA"] = errors.New("disk full")
			rep := Refresh(tc.cfg, store, &fakeLix{},
				&fakeUS2{closes: closes, eps: okEPSSeries(now)}, &fakeAkshare{}, tc.ed, nil, nil, now)

			assert.Empty(t, rep.Failed, "落价失败不得让标的进 Failed")
			assert.Equal(t, 1, rep.Refreshed, "估值主流程仍算刷新成功")
			require.Len(t, rep.Degraded, 1, "落价失败须可观测")
			assert.Contains(t, rep.Degraded[0], "NVDA")
			assert.Contains(t, rep.Degraded[0], "disk full", "降级说明须带原始错误")
			assert.NotEmpty(t, store.upserts["NVDA"], "估值行仍应落库")
			assert.Empty(t, store.prices["NVDA"], "失败的落价不得留下半截数据")
		})
	}
}

func TestRefreshCNPathsNeverUpsertPrices(t *testing.T) {
	// error_handling[0]: A/H 链路零行为变更 —— lixinger 与 akshare 都不得触碰 price_daily
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	cfg := config.PrismConfig{Instruments: []config.PrismInstrument{
		{Symbol: "000300.SH", Name: "沪深300", Type: "index", Market: "CN_A", Group: "A股指数", Source: "lixinger"},
		{Symbol: "600519.SH", Name: "贵州茅台", Type: "stock", Market: "CN_A", Group: "A股公司", Source: "akshare"},
	}}
	cfg.ApplyDefaults()
	store := newFakeStore()
	ak := &fakeAkshare{stockPts: []akshare.ValuationPoint{{Date: now, PETTM: 30, PB: 8, PSTTM: 12}}}
	rep := Refresh(cfg, store, &fakeLix{}, &fakeUS2{}, ak, fakeEdgar{}, nil, nil, now)

	require.Empty(t, rep.Failed)
	assert.Equal(t, 2, rep.Refreshed)
	assert.NotEmpty(t, store.upserts["000300.SH"], "lixinger 估值链路照常落库")
	assert.NotEmpty(t, store.upserts["600519.SH"], "akshare 估值链路照常落库")
	assert.Zero(t, store.priceCalls, "A/H 路径不得调用 UpsertPrices")
	assert.Empty(t, store.prices, "A/H 路径不得写入 price_daily")
}

// Context Checkpoint (TASK-005 M3.5a 降级链扩展): done_criteria → test mapping
//   functional[0]     akshare 失败 + ts!=nil → daily_basic 落库 + Degraded 双格式串
//                       → TestRefreshAkshareFallsBackToTushare / TestRefreshTushareFallbackLocalPercentile
//   functional[1]     yahoo 价格失败 + td!=nil → td 数据参与 PE 重建(EPS 链路不变)
//                       → TestRefreshUSPriceFallsBackToTwelvedata
//   functional[2]     港股 akshare 失败 → hk_daily 仅价格,不写估值行,Degraded 注明
//                       → TestRefreshHKPriceOnlyHopWiring(**仅验接线,非生产可用性证据**)
//                       + TestRefreshHKProductionSymbolHitsKnownGap(生产形态零行→判失败,已知缺口红线)
//                       缺口:配置是 4 位 0700.HK,tushare hk_daily 要 5 位;归一未做(Leader 裁决
//                       方案 2:无 5 位正向实证前不做),见 D4 后续任务,修复后两条用例都要同步改写
//   functional[3]     A 股指数链尾 tushare 跳 = 仅价格(TASK-001 探针:index_dailybasic 40203)
//                       → TestRefreshIndexChainTailTushareIsPriceOnly
//   boundary[0]       ts/td 均 nil → 行为与改动前完全一致(不发「未配置」Degraded,ADR#9)
//                       → TestRefreshNilClientsSkipHops(+ 本文件既有全部用例回归)
//   error_handling[0] ErrNoPermission → 不重试 + Degraded 含「权限不足,配置性问题」
//                       → TestRefreshTusharePermissionNotRetried
//   error_handling[0] 延伸 ErrRateLimited(TASK-001 拆出)→ Degraded 用临时性语义,
//                       不得写成配置问题 → TestRefreshTushareRateLimitedIsTemporary
//   [证据驱动补充] 探针实测 ts_code="0700.HK" 返回 code=0 且 items 为空,若把空结果
//   当兜底成功会产生「fallback ok 但零数据」的假成功 → TestRefreshTushareEmptyIsNotSuccess

type fakeTushare struct {
	valuation map[string][]tushare.ValuationPoint
	indexPx   map[string][]tushare.PricePoint
	hkPx      map[string][]tushare.PricePoint
	failVal   map[string]error
	failIndex map[string]error
	failHK    map[string]error
	calls     map[string]int // "api:symbol" → 调用次数(「不重试」断言用)
}

func (f *fakeTushare) note(api, symbol string) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[api+":"+symbol]++
}
func (f *fakeTushare) FetchDailyBasic(symbol string, start, end time.Time) ([]tushare.ValuationPoint, error) {
	f.note("daily_basic", symbol)
	if err := f.failVal[symbol]; err != nil {
		return nil, err
	}
	return f.valuation[symbol], nil
}
func (f *fakeTushare) FetchIndexDaily(symbol string, start, end time.Time) ([]tushare.PricePoint, error) {
	f.note("index_daily", symbol)
	if err := f.failIndex[symbol]; err != nil {
		return nil, err
	}
	return f.indexPx[symbol], nil
}
func (f *fakeTushare) FetchHKDaily(symbol string, start, end time.Time) ([]tushare.PricePoint, error) {
	f.note("hk_daily", symbol)
	if err := f.failHK[symbol]; err != nil {
		return nil, err
	}
	return f.hkPx[symbol], nil
}

type fakeTD struct {
	closes map[string][]core.OHLCV
	fail   map[string]error
	calls  map[string][2]time.Time
}

func (f *fakeTD) FetchHistory(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	if f.calls == nil {
		f.calls = map[string][2]time.Time{}
	}
	f.calls[symbol] = [2]time.Time{start, end}
	if err := f.fail[symbol]; err != nil {
		return nil, err
	}
	return f.closes[symbol], nil
}

func aStockInst() config.PrismInstrument {
	return config.PrismInstrument{Symbol: "600519.SH", Name: "贵州茅台", Type: "stock",
		Market: "CN_A", Group: "A股公司", Source: "akshare"}
}

func hkInst() config.PrismInstrument {
	return config.PrismInstrument{Symbol: "0700.HK", Name: "腾讯控股", Type: "stock",
		Market: "HK", Group: "港股公司", Source: "akshare"}
}

// functional[0]:A 股公司估值二跳。akshare 失败 → tushare daily_basic 官方口径落库,
// Degraded 记「主源失败原因 + 兜底成功」双格式串。
func TestRefreshAkshareFallsBackToTushare(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
	ts := &fakeTushare{valuation: map[string][]tushare.ValuationPoint{
		"600519.SH": {{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), PETTM: 21.5, PB: 7.7, PSTTM: 9.1}},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed, "兜底成功计入 Refreshed")
	require.Len(t, rep.Degraded, 1)
	assert.Contains(t, rep.Degraded[0], "600519.SH")
	assert.Contains(t, rep.Degraded[0], "akshare failed", "格式串前段")
	assert.Contains(t, rep.Degraded[0], "aktools down", "含主源失败原因")
	assert.Contains(t, rep.Degraded[0], "tushare fallback ok", "格式串后段")

	rows := store.upserts["600519.SH"]
	require.Len(t, rows, 1)
	assert.Equal(t, "2026-07-23", rows[0].D)
	assert.InDelta(t, 21.5, rows[0].PETTM, 1e-9)
	assert.InDelta(t, 7.7, rows[0].PB, 1e-9)
	assert.InDelta(t, 9.1, rows[0].PSTTM, 1e-9)
}

// functional[0]:兜底跳复用与 akshare 路径同一套本地滚动分位逻辑(共用 helper)。
// 300 天 PE=10 历史 + 新点 PE=20 → 新点为窗口最大值 → 5Y/10Y 分位均 100,且只写新点。
func TestRefreshTushareFallbackLocalPercentile(t *testing.T) {
	hist := &prismstore.SeriesData{Symbol: "600519.SH"}
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 300 {
		hist.Dates = append(hist.Dates, base.AddDate(0, 0, i).Format("2006-01-02"))
		hist.PETTM = append(hist.PETTM, 10.0)
	}
	store := newFakeStore()
	store.series["600519.SH"] = hist
	store.latest["600519.SH"] = hist.Dates[len(hist.Dates)-1]

	newDay := base.AddDate(0, 0, 300)
	ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
	ts := &fakeTushare{valuation: map[string][]tushare.ValuationPoint{
		"600519.SH": {{Date: newDay, PETTM: 20.0, PB: math.NaN(), PSTTM: math.NaN()}},
	}}

	rep := Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, newDay.AddDate(0, 0, 1))
	require.Empty(t, rep.Failed)
	rows := store.upserts["600519.SH"]
	require.Len(t, rows, 1, "只写新点,历史行不回写")
	assert.InDelta(t, 100.0, rows[0].Pctl5Y, 1e-9, "分位由本地滚动窗口算出")
	assert.InDelta(t, 100.0, rows[0].Pctl10Y, 1e-9)
}

// functional[1]:美股价格二跳。yahoo 价格失败 → twelvedata 补该段并参与 PE 重建;
// EPS 仍走 yahoo(EPS 链路不变),td 不承担 EPS。
func TestRefreshUSPriceFallsBackToTwelvedata(t *testing.T) {
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	closes := []core.OHLCV{{Time: now.AddDate(0, 0, -1), Close: 100}}
	store := newFakeStore()
	us := &fakeUS2{closes: closes, eps: okEPSSeries(now),
		failPrice: map[string]error{"NVDA": errors.New("yahoo 503")}}
	td := &fakeTD{closes: map[string][]core.OHLCV{"NVDA": closes}}

	rep := Refresh(engineCfg(), store, &fakeLix{}, us, &fakeAkshare{}, fakeEdgar{}, nil, td, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)
	require.Len(t, rep.Degraded, 1)
	assert.Contains(t, rep.Degraded[0], "NVDA")
	assert.Contains(t, rep.Degraded[0], "yahoo price failed")
	assert.Contains(t, rep.Degraded[0], "yahoo 503", "含主源失败原因")
	assert.Contains(t, rep.Degraded[0], "twelvedata fallback")

	_, tdCalled := td.calls["NVDA"]
	assert.True(t, tdCalled, "td 必须被调用补价格")
	_, epsCalled := us.epsCalls["NVDA"]
	assert.True(t, epsCalled, "EPS 链路不变:仍走 yahoo")
	assert.NotEmpty(t, store.upserts["NVDA"], "td 价格参与 PE 重建并落库")
	assert.NotEmpty(t, store.prices["NVDA"], "兜底价格同样落 price_daily")
}

// functional[2] 的**接线**部分:港股 akshare 失败 → 分派到 hk_daily、仅价格落库、
// 不写估值行、Degraded 文案正确。
//
// ⚠ 本用例**不构成生产可用性证据**:fake 以配置形态 "0700.HK" 为 key,而真实 tushare
// hk_daily 要 5 位 "00700.HK"(4 位实测返回 code=0 且 items 为空)。也就是说 fake 会命中
// 只因为它按配置形态建键,真实上游不会命中。生产侧的已知缺口由
// TestRefreshHKProductionSymbolHitsKnownGap 锁定;归一修复见 D4 后续任务。
// 保留本用例的价值在于:分派逻辑、仅价格语义、水位保护这三件事仍需回归。
//
// 不写估值行是刻意设计:LatestDate 取 MAX(d) FROM valuation_daily,写 NaN 估值行会把
// 增量水位推到今天,主源恢复后 incrementalStart 直接 skip,这些天的真实估值将永久不回填。
func TestRefreshHKPriceOnlyHopWiring(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"0700.HK": errors.New("aktools down")}}
	ts := &fakeTushare{hkPx: map[string][]tushare.PricePoint{
		"0700.HK": {{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), Close: 512.5}},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(hkInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)
	require.Len(t, rep.Degraded, 1)
	assert.Contains(t, rep.Degraded[0], "0700.HK")
	assert.Contains(t, rep.Degraded[0], "仅价格")
	assert.Contains(t, rep.Degraded[0], "估值缺失")

	require.Len(t, store.prices["0700.HK"], 1, "价格落 price_daily")
	assert.InDelta(t, 512.5, store.prices["0700.HK"][0].Close, 1e-9)
	assert.Empty(t, store.upserts["0700.HK"], "仅价格跳不得写估值行(否则污染增量水位)")
	assert.Equal(t, 1, ts.calls["hk_daily:0700.HK"])
}

// functional[3]:A 股指数链尾。lixinger→akshare 双败且 ts!=nil → tushare index_daily
// 仅价格(TASK-001 live 探针:index_dailybasic 40203 无权限,链尾定为仅价格)。
func TestRefreshIndexChainTailTushareIsPriceOnly(t *testing.T) {
	store := newFakeStore()
	lix := &fakeLix{fail: map[string]error{"000300.SH": errors.New("quota exhausted")}}
	ak := &fakeAkshare{fail: map[string]error{"000300.SH": errors.New("aktools down")}}
	ts := &fakeTushare{indexPx: map[string][]tushare.PricePoint{
		"000300.SH": {{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), Close: 4588.19}},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(fbInst()), store, lix, fakeUS{}, ak, fakeEdgar{}, ts, nil, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)
	require.NotEmpty(t, rep.Degraded)
	last := rep.Degraded[len(rep.Degraded)-1]
	assert.Contains(t, last, "000300.SH")
	assert.Contains(t, last, "仅价格")

	require.Len(t, store.prices["000300.SH"], 1)
	assert.InDelta(t, 4588.19, store.prices["000300.SH"][0].Close, 1e-9)
	assert.Empty(t, store.upserts["000300.SH"], "链尾仅价格,不写估值行")
	assert.Equal(t, 1, ts.calls["index_daily:000300.SH"])
	assert.Zero(t, ts.calls["daily_basic:000300.SH"], "指数不得走 daily_basic")
}

// error_handling[0]:ErrNoPermission 是永久性错误 —— 该跳只调用一次(不重试),
// 且 Degraded 文案点明是配置性问题,避免运维把它当临时故障反复观察。
func TestRefreshTusharePermissionNotRetried(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
	ts := &fakeTushare{failVal: map[string]error{
		"600519.SH": fmt.Errorf("%w: daily_basic (抱歉，您没有接口访问权限)", tushare.ErrNoPermission),
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, now)
	require.NotEmpty(t, rep.Degraded)
	assert.Contains(t, rep.Degraded[0], "权限不足")
	assert.Contains(t, rep.Degraded[0], "配置性问题")
	assert.Equal(t, 1, ts.calls["daily_basic:600519.SH"], "永久性错误不得重试")

	require.Len(t, rep.Failed, 1, "兜底未成功,标的仍判失败")
	assert.Contains(t, rep.Failed[0], "600519.SH")
	assert.Equal(t, 0, rep.Refreshed)
	assert.Empty(t, store.upserts["600519.SH"])
}

// boundary[0](ADR#9):ts/td 均 nil → 与改动前完全一致:主源失败即 Failed,
// 不发「备源未配置」提示,错误文本不含兜底痕迹。
func TestRefreshNilClientsSkipHops(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, nil, nil, now)
	assert.Equal(t, 0, rep.Refreshed)
	assert.Empty(t, rep.Degraded, "未配置备源不得产生 Degraded(ADR#9)")
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "aktools down")
	assert.NotContains(t, rep.Failed[0], "tushare", "nil 客户端不得留下兜底痕迹")
}

// 证据驱动:live 探针实测 hk_daily 对 ts_code="0700.HK" 返回 code=0 且 items 为空
// (非报错)。空结果若判成功会产出「fallback ok 但零数据」的假成功并吞掉真实缺口,
// 故空结果必须判失败。
func TestRefreshTushareEmptyIsNotSuccess(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"0700.HK": errors.New("aktools down")}}
	ts := &fakeTushare{} // hkPx 未预置 → 返回空切片,err=nil
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(hkInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, now)
	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "0700.HK")
	assert.Empty(t, store.prices["0700.HK"], "零数据不得落库")
	for _, d := range rep.Degraded {
		assert.NotContains(t, d, "fallback ok", "零数据不得上报为兜底成功")
	}
}

// Context Checkpoint (TASK-005 review_fix 轮次 1): QA fix_items → test mapping
//   C2 "daily_basic 三值全 NaN 的行不得落库(会推进 LatestDate 水位致该日永不重访)"
//        → TestRefreshTushareValuationAllNaNIsNotSuccess
//   C2 边界 "只有 PE 为 NaN(亏损标的真实形态)时仍须落库,守卫不得误伤"
//        → TestRefreshTushareValuationKeepsRowsWhenOnlyPEIsNaN
//   M1 "仅价格跳的 upsertPrices 写失败须上抛为 error(价格是该跳全部交付物)"
//        → TestRefreshTusharePricesUpsertFailureIsError
//   M2 "零行守卫锁定:A 股估值零行 / 美股 TD 零行"
//        → TestRefreshTushareValuationEmptyIsNotSuccess / TestRefreshUSPriceTwelvedataEmptyIsNotSuccess

// C2(CRITICAL):亏损标的的 daily_basic 会返回 pe_ttm:null → 客户端置 NaN。三值全 NaN 的行
// 一旦落库,sqlite 侧 NaN→NULL,而 LatestDate=MAX(d) **不过滤 NULL** ⇒ 水位被推进、
// 次日 start=latest+1、该日永不重访,且当次还报 fallback ok。必须在写库前拦下。
func TestRefreshTushareValuationAllNaNIsNotSuccess(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
	nan := math.NaN()
	ts := &fakeTushare{valuation: map[string][]tushare.ValuationPoint{
		"600519.SH": {
			{Date: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), PETTM: nan, PB: nan, PSTTM: nan},
			{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), PETTM: nan, PB: nan, PSTTM: nan},
		},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, now)
	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "600519.SH")
	assert.Empty(t, store.upserts["600519.SH"], "全 NaN 行不得落库,否则推进 LatestDate 水位")
	for _, d := range rep.Degraded {
		assert.NotContains(t, d, "fallback ok", "全 NaN 不得上报为兜底成功")
	}
}

// C2 边界:守卫判据是「三值全 NaN」而非「PE 为 NaN」。亏损标的的真实形态是
// pe_ttm 缺失但 pb/ps_ttm 有值 —— 那是有效估值数据,必须照常落库,守卫不得误伤。
func TestRefreshTushareValuationKeepsRowsWhenOnlyPEIsNaN(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
	ts := &fakeTushare{valuation: map[string][]tushare.ValuationPoint{
		"600519.SH": {{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC),
			PETTM: math.NaN(), PB: 6.2, PSTTM: 9.1}},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, now)
	assert.Empty(t, rep.Failed)
	assert.Equal(t, 1, rep.Refreshed)
	rows := store.upserts["600519.SH"]
	require.Len(t, rows, 1, "PB/PS 有值即为有效估值,必须落库")
	assert.True(t, math.IsNaN(rows[0].PETTM))
	assert.InDelta(t, 6.2, rows[0].PB, 1e-9)
}

// M2 第 1 处零行守卫:A 股估值兜底拉到 0 行不得算成功。
func TestRefreshTushareValuationEmptyIsNotSuccess(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
	ts := &fakeTushare{} // valuation 未预置 → 返回空切片,err=nil
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, now)
	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Empty(t, store.upserts["600519.SH"])
	for _, d := range rep.Degraded {
		assert.NotContains(t, d, "fallback ok")
	}
}

// M2 第 2 处零行守卫:yahoo 失败后 TD 返回 0 行,同样不得算兜底成功
// (否则 PE 会用一段空价格重建,或直接产出空序列却报 ok)。
func TestRefreshUSPriceTwelvedataEmptyIsNotSuccess(t *testing.T) {
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	store := newFakeStore()
	us := &fakeUS2{closes: []core.OHLCV{{Time: now.AddDate(0, 0, -1), Close: 100}},
		eps:       okEPSSeries(now),
		failPrice: map[string]error{"NVDA": errors.New("yahoo 503")}}
	td := &fakeTD{} // closes 未预置 → 返回空切片,err=nil

	rep := Refresh(engineCfg(), store, &fakeLix{}, us, &fakeAkshare{}, fakeEdgar{}, nil, td, now)
	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "NVDA")
	assert.Contains(t, rep.Failed[0], "price history:", "须保持既有错误前缀")
	assert.Empty(t, store.upserts["NVDA"])
	for _, d := range rep.Degraded {
		assert.NotContains(t, d, "twelvedata fallback ok")
	}
}

// M1:仅价格跳的价格就是它的全部交付物 —— 写库失败必须让该标的判失败。
// 修复前的行为(QA 实证):Refreshed=1、Failed 为空、落库 0 行,还发两条自相矛盾的
// Degraded(一条说 fallback ok,一条说 price_daily upsert failed)。
func TestRefreshTusharePricesUpsertFailureIsError(t *testing.T) {
	store := newFakeStore()
	store.priceErr["0700.HK"] = errors.New("disk full")
	ak := &fakeAkshare{fail: map[string]error{"0700.HK": errors.New("aktools down")}}
	ts := &fakeTushare{hkPx: map[string][]tushare.PricePoint{
		"0700.HK": {{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), Close: 512.5}},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(hkInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, now)
	assert.Equal(t, 0, rep.Refreshed, "价格写失败 = 该跳无交付物,不得计成功")
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "0700.HK")
	assert.Contains(t, rep.Failed[0], "disk full")
	for _, d := range rep.Degraded {
		assert.NotContains(t, d, "fallback ok", "写失败不得同时上报兜底成功")
	}
}

// M3b(Leader 裁决方案 2:如实记录红线,不做归一)——**已知缺口的锁定测试**。
//
// 缺口本体:配置(configs/config.yaml)里的港股形态是 4 位 "0700.HK",而 tushare
// hk_daily 要 5 位 "00700.HK";4 位实测返回 code=0 且 items 为空(静默空,不是报错)。
// 客户端**未做** %05s 归一(ADR#8 认为形态天然一致,在港股上不成立),因此该跳在生产里
// 恒零行、恒判失败——这正是本用例锁定的事实。
//
// 本用例的 fake 按**真实上游契约**建键(只认 5 位),故 refresh 传 4 位时查不到 → 零行。
// 它与 TestRefreshHKPriceOnlyHopWiring 的分工:那条验接线,这条验生产现实。
//
// ⚠ 后续任务(D4)做完归一后,本用例必须同步改写为「归一后能命中 5 位并成功」,
// 否则它会反过来把修复判成失败。改写前置条件:先拿到 5 位形态的正向实证
// ——截至 2026-08-02 两次探针均撞 hk_daily 限频(窗口已自升级到 1 次/小时),尚无证据。
func TestRefreshHKProductionSymbolHitsKnownGap(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"0700.HK": errors.New("aktools down")}}
	// 真实 tushare 只认 5 位:fake 按上游契约建键,不迁就被测代码。
	ts := &fakeTushare{hkPx: map[string][]tushare.PricePoint{
		"00700.HK": {{Date: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC), Close: 512.5}},
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(hkInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, now)

	assert.Equal(t, 0, rep.Refreshed, "已知缺口:4 位配置形态取不到数据,该跳必然失败")
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "0700.HK")
	assert.Empty(t, store.prices["0700.HK"], "零行不得落库")
	for _, d := range rep.Degraded {
		assert.NotContains(t, d, "fallback ok", "已知缺口不得被上报为兜底成功")
	}
	assert.Equal(t, 1, ts.calls["hk_daily:0700.HK"], "确实是以 4 位形态发起的调用")
}

// M2 延伸(Leader 指定):消费 TASK-001 拆出的 ErrRateLimited。
// 限频是**临时**错误——窗口过后自愈,运维什么都不用改。若沿用 ErrNoPermission 那套
// 「权限不足,配置性问题」文案,运维会去查积分档(TASK-006 演练已实锤该误导)。
func TestRefreshTushareRateLimitedIsTemporary(t *testing.T) {
	store := newFakeStore()
	ak := &fakeAkshare{fail: map[string]error{"600519.SH": errors.New("aktools down")}}
	ts := &fakeTushare{failVal: map[string]error{
		"600519.SH": fmt.Errorf("%w: daily_basic (抱歉，您访问接口(daily_basic)频率超限(1次/分钟))",
			tushare.ErrRateLimited),
	}}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(aStockInst()), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, ts, nil, now)
	require.NotEmpty(t, rep.Degraded)
	assert.Contains(t, rep.Degraded[0], "限频")
	assert.Contains(t, rep.Degraded[0], "下次自动重试", "须传达临时性语义")
	assert.NotContains(t, rep.Degraded[0], "配置性问题", "限频不是配置问题,不得误导运维去改配置")
	assert.NotContains(t, rep.Degraded[0], "权限不足")

	require.Len(t, rep.Failed, 1, "本次兜底确实没成功,标的仍判失败")
	assert.Equal(t, 0, rep.Refreshed)
}
