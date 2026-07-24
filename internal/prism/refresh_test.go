package prism

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	akshare "github.com/newthinker/atlas/internal/collector/akshare"
	"github.com/newthinker/atlas/internal/collector/edgar"
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
	latest       map[string]string // symbol -> latest date
	upserts      map[string][]prismstore.ValuationRow
	fundamentals map[string][]prismstore.FundamentalRow // symbol → 落库季度事实
	ids          map[int64]string
	nextID       int64
	series       map[string]*prismstore.SeriesData // symbol → 预置历史(Series 返回)
	seriesErr    map[string]error                  // symbol → 注入 Series 读回错误
}

func newFakeStore() *fakeStore {
	return &fakeStore{latest: map[string]string{}, upserts: map[string][]prismstore.ValuationRow{}, fundamentals: map[string][]prismstore.FundamentalRow{}, ids: map[int64]string{}, series: map[string]*prismstore.SeriesData{}}
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
	rep := Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, now)
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
	Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, now)
	win := lix.calls["000300.SH"]
	assert.Equal(t, time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC), win[0], "增量从 latest+1 天开始")
	assert.Equal(t, now, win[1])
}

func TestRefreshUpToDateZeroRequest(t *testing.T) {
	store, lix := newFakeStore(), &fakeLix{}
	store.latest["000300.SH"] = "2026-07-22" // latest+1 == now → 已是最新
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	rep := Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, now)
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
	rep := Refresh(cfg, store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC))
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
	rep := Refresh(cfg, store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 1, rep.Refreshed, "已知 source 标的不受未知 source 影响")
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "XYZ")
	assert.Contains(t, rep.Failed[0], "bogus")
}

func TestRefreshBadLatestDate(t *testing.T) {
	store, lix := newFakeStore(), &fakeLix{}
	store.latest["000300.SH"] = "not-a-date"
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	rep := Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, now)
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
	rep := Refresh(cfg, store, &fakeLix{}, us, &fakeAkshare{}, fakeEdgar{}, now)
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
			rep := Refresh(cfg, newFakeStore(), &fakeLix{}, tc.us, &fakeAkshare{}, fakeEdgar{}, now)
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

	rep := Refresh(cfg, store, &fakeLix{}, us, &fakeAkshare{}, fakeEdgar{}, now)
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
	rep := Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, now)
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
	Refresh(lixCfg(), store, lix, fakeUS{}, &fakeAkshare{}, fakeEdgar{}, now)
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

	rep := Refresh(cfg, store, &fakeLix{}, us, &fakeAkshare{}, fakeEdgar{}, now)
	assert.Equal(t, 0, rep.Refreshed)
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "LOSS")
	assert.Contains(t, rep.Failed[0], "non-positive", "错误语义应对齐 ErrNonPositiveEPS")
	assert.Empty(t, store.upserts["LOSS"], "当前亏损标的不得入库")
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

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, now)
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

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, now)
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

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, now)
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

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, now)
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

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, now)
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

	rep := Refresh(akCfg(inst), store, &fakeLix{}, fakeUS{}, ak, fakeEdgar{}, now)
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

	rep := Refresh(akCfg(fbInst()), store, lix, fakeUS{}, ak, fakeEdgar{}, now)
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

	rep := Refresh(akCfg(fbInst()), store, lix, fakeUS{}, ak, fakeEdgar{}, now)
	assert.Equal(t, 0, rep.Refreshed)
	assert.Empty(t, rep.Degraded)
	require.Len(t, rep.Failed, 1, "双败合并为单条")
	assert.Contains(t, rep.Failed[0], "lix down")
	assert.Contains(t, rep.Failed[0], "aktools down")
}

func TestRefreshFallbackNotTriggered(t *testing.T) {
	store, lix, ak := newFakeStore(), &fakeLix{}, &fakeAkshare{}
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	rep := Refresh(akCfg(fbInst()), store, lix, fakeUS{}, ak, fakeEdgar{}, now)
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

func TestRefreshEdgarPath(t *testing.T) {
	now := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	facts := edgarQuarters(now)
	lastFiled := facts[11].FilingDate
	closes := []core.OHLCV{
		{Time: lastFiled.AddDate(0, 0, -2), Close: 40}, // 最后一季 filing 前
		{Time: lastFiled.AddDate(0, 0, 2), Close: 40},  // filing 后(TTM 相同=4.0)
	}
	store := newFakeStore()
	rep := Refresh(edgarCfg(), store, &fakeLix{}, &fakeUS2{closes: closes}, &fakeAkshare{}, fakeEdgar{facts: facts}, now)
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
	Refresh(edgarCfg(), storeFull, &fakeLix{}, &fakeUS2{closes: closes}, &fakeAkshare{}, fakeEdgar{facts: full}, now)
	Refresh(edgarCfg(), storeGap, &fakeLix{}, &fakeUS2{closes: closes}, &fakeAkshare{}, fakeEdgar{facts: facts}, now)
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
	rep := Refresh(cfg, store, &fakeLix{}, &fakeUS2{}, &fakeAkshare{}, fakeEdgar{facts: edgarQuarters(now)}, now)
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
	rep := Refresh(edgarCfg(), store, &fakeLix{}, &fakeUS2{closes: closes}, &fakeAkshare{}, fakeEdgar{facts: facts}, now)
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
		&fakeAkshare{}, fakeEdgar{err: errors.New("edgar down")}, now)
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
	rep := Refresh(cfg, store, &fakeLix{}, &fakeUS2{}, &fakeAkshare{}, fakeEdgar{err: errors.New("edgar down")}, now)
	assert.Equal(t, 0, rep.Refreshed)
	assert.Empty(t, rep.Degraded, "无兜底不得记 Degraded")
	require.Len(t, rep.Failed, 1)
	assert.Contains(t, rep.Failed[0], "NVDA")
	assert.Contains(t, rep.Failed[0], "edgar down", "失败原因不得静默吞掉")
}
