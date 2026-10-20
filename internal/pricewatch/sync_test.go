package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-004)
// functional[0]     本地为空、okx/binance 两种语义 → 入库根数 == fake 总根数，段边界不缺根 → TestSync_Chunks
// functional[1]     老段 B 补、新段用 A；A 报错换 B；都不足 90% 取根数最多     → TestSync_FallbackPerChunk
// functional[2]     最新一根 high 100→120 被刷新（AD-12），两种语义              → TestSync_Incremental
// functional[3]     稳态只发 1 个请求且只调第一个 provider                        → TestSync_SteadyState
// functional[4]     中间缺一整段 → 只重取该段（AD-22）                            → TestSync_RepairsGap
// functional[5]     上线日写 kv；第二次不再请求早于上线日的段（AD-21）            → TestSync_Listing
// boundary[0]       坏 K 线丢弃、同批其余入库（AD-20b）                           → TestValidCandle、TestSync_DropsBadCandles
// non_functional[0] 5m 修剪不影响 1d；全源报错返回含 pair/bar 的 error 且已成功段仍在 → TestSync_Prune5m、TestSync_AllSourcesError
// QA 返工（review_fix 1）
// QA-8   上线日三道保护：部分源报错其余 0 根 / 某段全源报错 / 跳过的完整段 → TestSync_ListingNotRecordedOnError、TestSync_ListingNotRecordedAllError、TestSync_ListingStoppedBySkippedSegment
// AD-29  与最短窗口（1d→30d，5m→24h）相交的段要求 100%，更早段 90% → TestSync_RecentSegmentsNeedFullCoverage
// R1-5   best 不足且有源报错：best 照常入库并上抛错误；binance 全挂 → TestSync_BinanceDownReportsError

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// syncNow 不在日线边界上，最后一根日线（当天 00:00）尚未收盘。
var syncNow = time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)

type semantics int

const (
	okxSem     semantics = iota // 区间两端不含，返回区间内最新 300 根；只有最近 okxHorizon 根（reviewer P2）
	binanceSem                  // 区间两端含，从 start 起最多 1000 根
)

// okxHorizon：okx /market/candles 只能取到最近约 1440 根（reviewer P2 实测）。
const okxHorizon = 1440

type fakeCall struct{ start, end time.Time }

// fakeProvider 按真实 API 的区间语义返回 data 中的 K 线（reviewer P3/V12）。
type fakeProvider struct {
	name  string
	sem   semantics
	data  []core.OHLCV // 按时间升序
	extra []core.OHLCV // 每次原样附加在结果后（模拟 provider 吐出的坏数据）
	errIf func(start, end time.Time) bool

	mu    sync.Mutex
	calls []fakeCall
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) FetchQuote(string) (*core.Quote, error) { return nil, errors.New("unused") }

func (f *fakeProvider) FetchHistory(_ string, start, end time.Time, _ string) ([]core.OHLCV, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fakeCall{start, end})
	f.mu.Unlock()
	if f.errIf != nil && f.errIf(start, end) {
		return nil, errors.New(f.name + " boom")
	}
	data := f.data
	if f.sem == okxSem && len(data) > okxHorizon {
		data = data[len(data)-okxHorizon:]
	}
	var out []core.OHLCV
	for _, c := range data {
		switch f.sem {
		case okxSem:
			if c.Time.After(start) && c.Time.Before(end) {
				out = append(out, c)
			}
		case binanceSem:
			if !c.Time.Before(start) && !c.Time.After(end) && len(out) < 1000 {
				out = append(out, c)
			}
		}
	}
	if f.sem == okxSem && len(out) > 300 {
		out = out[len(out)-300:]
	}
	return append(out, f.extra...), nil
}

func (f *fakeProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeProvider) resetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

// series 生成 [from, to] 内每 step 一根、收盘价为 close 的 K 线。
func series(from, to time.Time, step time.Duration, close float64) []core.OHLCV {
	var out []core.OHLCV
	for ts := from; !ts.After(to); ts = ts.Add(step) {
		out = append(out, core.OHLCV{Time: ts, Open: close, High: close + 1, Low: close - 0.5, Close: close})
	}
	return out
}

const dayDur = 24 * time.Hour

var today = syncNow.Truncate(dayDur)

func newSyncer(t *testing.T, ps ...*fakeProvider) *Syncer {
	t.Helper()
	s, _ := newTestStore(t)
	sy := &Syncer{Store: s, Now: func() time.Time { return syncNow }}
	for _, p := range ps {
		sy.Providers = append(sy.Providers, p)
	}
	return sy
}

func mustSync(t *testing.T, sy *Syncer, pair, bar string) {
	t.Helper()
	if err := sy.Sync(context.Background(), pair, bar); err != nil {
		t.Fatalf("Sync %s/%s: %v", pair, bar, err)
	}
}

func countAll(t *testing.T, sy *Syncer, pair, bar string) int {
	t.Helper()
	return mustStats(t, sy.Store, pair, bar, time.Time{}, syncNow).Count
}

func TestSync_Chunks(t *testing.T) {
	cases := []struct {
		name string
		sem  semantics
		from time.Time
	}{
		{"okx（horizon 内）", okxSem, today.AddDate(0, 0, -1400)},
		{"binance", binanceSem, time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"okx 16:00 对齐", okxSem, today.AddDate(0, 0, -1400).Add(16 * time.Hour)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := series(c.from, syncNow, dayDur, 10)
			p := &fakeProvider{name: "p", sem: c.sem, data: data}
			sy := newSyncer(t, p)
			mustSync(t, sy, "BTCUSDT", "1d")
			if got := countAll(t, sy, "BTCUSDT", "1d"); got != len(data) {
				t.Fatalf("want %d candles, got %d", len(data), got)
			}
			// 每一根都在（段边界处不缺）
			for _, d := range data {
				if st := mustStats(t, sy.Store, "BTCUSDT", "1d", d.Time, d.Time); st.Count != 1 {
					t.Fatalf("missing candle at %s", d.Time)
				}
			}
		})
	}
}

func TestSync_FallbackPerChunk(t *testing.T) {
	t.Run("老段 B 新段 A", func(t *testing.T) {
		aFrom := today.AddDate(0, 0, -400)
		a := &fakeProvider{name: "A", sem: okxSem, data: series(aFrom, syncNow, dayDur, 1)}
		bData := series(today.AddDate(0, 0, -3000), syncNow, dayDur, 2)
		b := &fakeProvider{name: "B", sem: binanceSem, data: bData}
		sy := newSyncer(t, a, b)
		mustSync(t, sy, "BTCUSDT", "1d")

		if got := countAll(t, sy, "BTCUSDT", "1d"); got != len(bData) {
			t.Fatalf("want %d candles, got %d", len(bData), got)
		}
		if c, _, _ := sy.Store.FirstCloseSince("BTCUSDT", "1d", time.Time{}); c != 2 {
			t.Fatalf("oldest candle should come from B, close=%v", c)
		}
		if c, _, _ := sy.Store.FirstCloseSince("BTCUSDT", "1d", today.AddDate(0, 0, -10)); c != 1 {
			t.Fatalf("recent candle should come from A, close=%v", c)
		}
		// B 只用于 A 不够的老段：任何 B 请求都不应完全落在 A 的数据范围内
		for _, call := range b.calls {
			if !call.start.Before(aFrom) {
				t.Fatalf("B called for a segment A fully covers: %+v", call)
			}
		}
		if a.callCount() <= b.callCount() {
			t.Fatalf("A should be tried for every segment: A=%d B=%d", a.callCount(), b.callCount())
		}
	})

	t.Run("A 报错换 B", func(t *testing.T) {
		target := today.AddDate(0, 0, -100)
		a := &fakeProvider{name: "A", sem: okxSem, data: series(today.AddDate(0, 0, -1000), syncNow, dayDur, 1),
			errIf: func(s, e time.Time) bool { return s.Before(target) && e.After(target) }}
		b := &fakeProvider{name: "B", sem: binanceSem, data: series(today.AddDate(0, 0, -1000), syncNow, dayDur, 2)}
		sy := newSyncer(t, a, b)
		mustSync(t, sy, "BTCUSDT", "1d")
		if c, ok, _ := sy.Store.FirstCloseSince("BTCUSDT", "1d", target); !ok || c != 2 {
			t.Fatalf("segment where A errored should come from B, close=%v ok=%v", c, ok)
		}
		// today-600 所在段与 target 所在段相距 >300 天，且完全落在数据范围内
		if c, _, _ := sy.Store.FirstCloseSince("BTCUSDT", "1d", today.AddDate(0, 0, -600)); c != 1 {
			t.Fatalf("segments where A succeeded should come from A, close=%v", c)
		}
	})

	t.Run("都不足取最多", func(t *testing.T) {
		from := today.AddDate(0, 0, -600)
		a := &fakeProvider{name: "A", sem: okxSem, data: series(from, syncNow, 2*dayDur, 1)} // 1/2
		b := &fakeProvider{name: "B", sem: binanceSem, data: series(from, syncNow, 3*dayDur, 2)}
		sy := newSyncer(t, a, b)
		mustSync(t, sy, "BTCUSDT", "1d")
		if got := countAll(t, sy, "BTCUSDT", "1d"); got != len(a.data) {
			t.Fatalf("want A's %d candles, got %d", len(a.data), got)
		}
		st := mustStats(t, sy.Store, "BTCUSDT", "1d", time.Time{}, syncNow)
		if st.Low != 0.5 || st.High != 2 {
			t.Fatalf("B's candles (close=2, high=3) must not be stored: %+v", st)
		}
	})
}

func TestSync_Incremental(t *testing.T) {
	for _, sem := range []semantics{okxSem, binanceSem} {
		t.Run(strconv.Itoa(int(sem)), func(t *testing.T) {
			data := series(today.AddDate(0, 0, -50), syncNow, dayDur, 50)
			p := &fakeProvider{name: "p", sem: sem, data: data}
			sy := newSyncer(t, p)
			last := len(data) - 1
			data[last].High = 100
			mustUpsert(t, sy.Store, "BTCUSDT", "1d", data...)

			data[last].High = 120 // 未收盘的那根后来涨到 120
			mustSync(t, sy, "BTCUSDT", "1d")
			if st := mustStats(t, sy.Store, "BTCUSDT", "1d", today, today); st.High != 120 {
				t.Fatalf("latest candle not refreshed: %+v", st)
			}
		})
	}

	t.Run("停机超过一段后本地最新一根仍刷新", func(t *testing.T) {
		listing := time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC)
		data := series(listing, syncNow, dayDur, 50)
		p := &fakeProvider{name: "p", sem: okxSem, data: data}
		sy := newSyncer(t, p)
		if err := sy.Store.PutKV(ListingKey("BTCUSDT", "1d"), strconv.FormatInt(listing.UnixMilli(), 10)); err != nil {
			t.Fatal(err)
		}
		stop := len(data) - 400 // 本地只补到 400 天前，那根停在未收盘时的 high
		data[stop].High = 100
		mustUpsert(t, sy.Store, "BTCUSDT", "1d", data[:stop+1]...)

		data[stop].High = 120
		mustSync(t, sy, "BTCUSDT", "1d")
		if st := mustStats(t, sy.Store, "BTCUSDT", "1d", data[stop].Time, data[stop].Time); st.High != 120 {
			t.Fatalf("stale latest candle not refreshed: %+v", st)
		}
		if got := countAll(t, sy, "BTCUSDT", "1d"); got != len(data) {
			t.Fatalf("want %d, got %d", len(data), got)
		}
	})
}

func TestSync_SteadyState(t *testing.T) {
	cases := []struct {
		name string
		sem  semantics
		from time.Time
	}{
		{"okx（horizon 内）", okxSem, today.AddDate(0, 0, -1400)},
		{"binance", binanceSem, time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC)},
		{"okx 16:00 对齐", okxSem, today.AddDate(0, 0, -1400).Add(16 * time.Hour)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := &fakeProvider{name: "A", sem: c.sem, data: series(c.from, syncNow, dayDur, 1)}
			b := &fakeProvider{name: "B", sem: binanceSem, data: series(c.from, syncNow, dayDur, 2)}
			sy := newSyncer(t, a, b)
			mustSync(t, sy, "BTCUSDT", "1d")
			a.resetCalls()
			b.resetCalls()

			mustSync(t, sy, "BTCUSDT", "1d")
			if a.callCount() != 1 || b.callCount() != 0 {
				t.Fatalf("steady state: want A=1 B=0, got A=%d %+v B=%d", a.callCount(), a.calls, b.callCount())
			}
		})
	}
}

func TestSync_RepairsGap(t *testing.T) {
	data := series(today.AddDate(0, 0, -1400), syncNow, dayDur, 1)
	p := &fakeProvider{name: "p", sem: okxSem, data: data}
	sy := newSyncer(t, p)
	mustSync(t, sy, "BTCUSDT", "1d")

	// 挖掉中间一整段 300 天
	gapFrom := data[600].Time
	gapTo := gapFrom.AddDate(0, 0, 300)
	if _, err := sy.Store.db.Exec(`DELETE FROM candles WHERE ts >= ? AND ts < ?`, gapFrom.UnixMilli(), gapTo.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if got := countAll(t, sy, "BTCUSDT", "1d"); got != len(data)-300 {
		t.Fatalf("setup: want %d, got %d", len(data)-300, got)
	}
	p.resetCalls()

	mustSync(t, sy, "BTCUSDT", "1d")
	if got := countAll(t, sy, "BTCUSDT", "1d"); got != len(data) {
		t.Fatalf("gap not repaired: want %d, got %d", len(data), got)
	}
	// 缺口跨两段时最多 2 段 + 尾段；完整段不请求
	if n := p.callCount(); n > 3 {
		t.Fatalf("complete segments must not be fetched: %d calls %+v", n, p.calls)
	}
	for _, call := range p.calls {
		overlapsGap := call.start.Before(gapTo) && call.end.After(gapFrom)
		isTail := !call.start.Before(today.AddDate(0, 0, -2))
		if !overlapsGap && !isTail {
			t.Fatalf("request outside gap and tail: %+v", call)
		}
	}
}

// AD-13：okx 16:00 与 binance 00:00 两种日线混存时，重复的根数不能掩盖缺口。
func TestSync_RepairsGapUnderMixedAlignment(t *testing.T) {
	listing := time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC)
	p := &fakeProvider{name: "p", sem: binanceSem, data: series(listing, syncNow, dayDur, 1)}
	sy := newSyncer(t, p)
	mustSync(t, sy, "BTCUSDT", "1d")

	segFrom := listing.Add(3 * chunkBars * dayDur)
	gapTo := segFrom.AddDate(0, 0, 120)
	if _, err := sy.Store.db.Exec(`DELETE FROM candles WHERE ts >= ? AND ts < ?`, segFrom.UnixMilli(), gapTo.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	// 段内其余 180 天各多一根 16:00：根数 360 ≥ 90%×300，槽位只有 60%
	mustUpsert(t, sy.Store, "BTCUSDT", "1d", series(gapTo.Add(16*time.Hour), segFrom.AddDate(0, 0, 299).Add(16*time.Hour), dayDur, 1)...)

	mustSync(t, sy, "BTCUSDT", "1d")
	if st := mustStats(t, sy.Store, "BTCUSDT", "1d", segFrom, gapTo.Add(-time.Millisecond)); st.Count != 120 {
		t.Fatalf("gap hidden by duplicate alignment not repaired: %+v", st)
	}
}

func TestSync_Listing(t *testing.T) {
	listing := time.Date(2017, 8, 17, 0, 0, 0, 0, time.UTC)
	a := &fakeProvider{name: "A", sem: okxSem, data: series(listing, syncNow, dayDur, 1)}
	b := &fakeProvider{name: "B", sem: binanceSem, data: series(listing, syncNow, dayDur, 2)}
	sy := newSyncer(t, a, b)
	mustSync(t, sy, "BTCUSDT", "1d")

	v, err := sy.Store.GetKV("listing:BTCUSDT:1d")
	if err != nil || v != strconv.FormatInt(listing.UnixMilli(), 10) {
		t.Fatalf("listing kv: got %q err=%v, want %d", v, err, listing.UnixMilli())
	}

	a.resetCalls()
	b.resetCalls()
	mustSync(t, sy, "BTCUSDT", "1d")
	for _, call := range append(a.calls, b.calls...) {
		if !call.end.After(listing) {
			t.Fatalf("requested a segment entirely before listing: %+v", call)
		}
	}
	if got := countAll(t, sy, "BTCUSDT", "1d"); got != len(a.data) {
		t.Fatalf("want %d, got %d", len(a.data), got)
	}
}

func TestSync_NoListingWhenNoLeadingEmpty(t *testing.T) {
	// 第一段就有数据：没有「上线前全 0 段」，不记上线日
	p := &fakeProvider{name: "p", sem: binanceSem, data: series(time.Date(2009, 6, 1, 0, 0, 0, 0, time.UTC), syncNow, dayDur, 1)}
	sy := newSyncer(t, p)
	mustSync(t, sy, "BTCUSDT", "1d")
	if v, _ := sy.Store.GetKV("listing:BTCUSDT:1d"); v != "" {
		t.Fatalf("listing must not be recorded, got %q", v)
	}
}

func TestValidCandle(t *testing.T) {
	good := core.OHLCV{Time: syncNow, Open: 10, High: 12, Low: 9, Close: 11}
	cases := []struct {
		name string
		mod  func(*core.OHLCV)
		want bool
	}{
		{"正常", func(*core.OHLCV) {}, true},
		{"high==low", func(c *core.OHLCV) { c.High, c.Low, c.Open, c.Close = 5, 5, 5, 5 }, true},
		{"恰 2009-01-01", func(c *core.OHLCV) { c.Time = time.Date(2009, 1, 1, 0, 0, 0, 0, time.UTC) }, true},
		{"open NaN", func(c *core.OHLCV) { c.Open = math.NaN() }, false},
		{"high +Inf", func(c *core.OHLCV) { c.High = math.Inf(1) }, false},
		{"low -Inf", func(c *core.OHLCV) { c.Low = math.Inf(-1) }, false},
		{"close NaN", func(c *core.OHLCV) { c.Close = math.NaN() }, false},
		{"open 0", func(c *core.OHLCV) { c.Open = 0 }, false},
		{"high 0", func(c *core.OHLCV) { c.High = 0 }, false},
		{"low 0（ParseFloat 失败）", func(c *core.OHLCV) { c.Low = 0 }, false},
		{"close 负", func(c *core.OHLCV) { c.Close = -1 }, false},
		{"low>high", func(c *core.OHLCV) { c.Low, c.High = 13, 12 }, false},
		{"零值时间", func(c *core.OHLCV) { c.Time = time.Time{} }, false},
		{"1970（ts 解析失败）", func(c *core.OHLCV) { c.Time = time.UnixMilli(0) }, false},
		{"早于 2009-01-01", func(c *core.OHLCV) { c.Time = time.Date(2008, 12, 31, 23, 59, 59, 0, time.UTC) }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			k := good
			c.mod(&k)
			if got := validCandle(k); got != c.want {
				t.Fatalf("validCandle(%+v) = %v, want %v", k, got, c.want)
			}
		})
	}
}

func TestSync_DropsBadCandles(t *testing.T) {
	data := series(today.AddDate(0, 0, -20), syncNow, dayDur, 10)
	bad := []core.OHLCV{
		{Time: today.AddDate(0, 0, -5).Add(time.Hour), Open: 10, High: 1e9, Low: 0, Close: 10}, // low=0
		{Time: today.AddDate(0, 0, -4).Add(time.Hour), Open: 10, High: 5, Low: 6, Close: 10},   // low>high
		{Time: today.AddDate(0, 0, -3).Add(time.Hour), Open: 10, High: math.NaN(), Low: 1, Close: 10},
		{Time: time.UnixMilli(0), Open: 10, High: 11, Low: 9, Close: 10},
	}
	p := &fakeProvider{name: "p", sem: binanceSem, data: data, extra: bad}
	sy := newSyncer(t, p)
	mustSync(t, sy, "BTCUSDT", "1d")

	st := mustStats(t, sy.Store, "BTCUSDT", "1d", time.Time{}, syncNow)
	if st.Count != len(data) || st.High != 11 || st.Low != 9.5 || !st.First.Equal(data[0].Time) {
		t.Fatalf("bad candles leaked or good ones lost: %+v (want Count=%d High=11 Low=9.5)", st, len(data))
	}
}

func TestSync_Prune5m(t *testing.T) {
	const fiveMin = 5 * time.Minute
	// 开盘时间恰为 now 的那根还不存在
	// 8 天 5m 线 2304 根超过 okx 的 1440 根上限，回填由 binance 承担（AD-20）
	p := &fakeProvider{name: "p", sem: binanceSem, data: series(syncNow.Add(-8*dayDur), syncNow.Add(-fiveMin), fiveMin, 1)}
	sy := newSyncer(t, p)
	old := []core.OHLCV{candle(syncNow.Add(-10*dayDur), 2, 1, 1), candle(syncNow.Add(-9*dayDur), 2, 1, 1)}
	mustUpsert(t, sy.Store, "BTCUSDT", "5m", old...)
	mustUpsert(t, sy.Store, "BTCUSDT", "1d", old...)

	mustSync(t, sy, "BTCUSDT", "5m")
	st := mustStats(t, sy.Store, "BTCUSDT", "5m", time.Time{}, syncNow)
	if st.First.Before(syncNow.Add(-8 * dayDur)) {
		t.Fatalf("5m older than 8 days not pruned: first=%s", st.First)
	}
	if st.Count != len(p.data) {
		t.Fatalf("5m: want %d, got %d", len(p.data), st.Count)
	}
	if got := countAll(t, sy, "BTCUSDT", "1d"); got != 2 {
		t.Fatalf("1d affected by 5m prune: %d", got)
	}
}

func TestSync_AllSourcesError(t *testing.T) {
	target := today.AddDate(0, 0, -400)
	hit := func(s, e time.Time) bool { return s.Before(target) && e.After(target) }
	data := series(today.AddDate(0, 0, -1000), syncNow, dayDur, 1)
	a := &fakeProvider{name: "A", sem: okxSem, data: data, errIf: hit}
	b := &fakeProvider{name: "B", sem: binanceSem, data: data, errIf: hit}
	sy := newSyncer(t, a, b)

	err := sy.Sync(context.Background(), "BTCUSDT", "1d")
	if err == nil || !strings.Contains(err.Error(), "BTCUSDT") || !strings.Contains(err.Error(), "1d") {
		t.Fatalf("want error mentioning pair and bar, got %v", err)
	}
	if st := mustStats(t, sy.Store, "BTCUSDT", "1d", time.Time{}, target.AddDate(0, 0, -300)); st.Count == 0 {
		t.Fatal("segments fetched before the failure must stay in the store")
	}
	if st := mustStats(t, sy.Store, "BTCUSDT", "1d", today, today); st.Count != 1 {
		t.Fatal("later segments should still be synced after one segment fails")
	}
}

func TestSync_Errors(t *testing.T) {
	p := &fakeProvider{name: "p", sem: okxSem}
	sy := newSyncer(t, p)
	if err := sy.Sync(context.Background(), "BTCUSDT", "3h"); err == nil {
		t.Fatal("unknown bar must be rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sy.Sync(ctx, "BTCUSDT", "1d"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx: want context.Canceled, got %v", err)
	}
	if p.callCount() != 0 {
		t.Fatalf("no request after cancel, got %d", p.callCount())
	}
}

// deleteCandle 从库里删掉恰在 ts 的那根（模拟补数失败留下的单根缺口）。
func deleteCandle(t *testing.T, st *Store, bar string, ts time.Time) {
	t.Helper()
	if _, err := st.db.Exec(`DELETE FROM candles WHERE bar = ? AND ts = ?`, bar, ts.UnixMilli()); err != nil {
		t.Fatal(err)
	}
}

func hasCandle(t *testing.T, st *Store, bar string, ts time.Time) bool {
	t.Helper()
	return mustStats(t, st, "BTCUSDT", bar, ts, ts).Count == 1
}

// QA-8（探针源码 .arcforge/docs/02-plan/qa-8-probe.md）：A 只有最近 400 天；B 有 3000 天但对老段报错。
// 老段「A 成功 0 根 + B 报错」不能当成「未上线」。
func TestSync_ListingNotRecordedOnError(t *testing.T) {
	aFrom := today.AddDate(0, 0, -400)
	a := &fakeProvider{name: "A", sem: okxSem, data: series(aFrom, syncNow, dayDur, 1)}
	b := &fakeProvider{name: "B", sem: binanceSem, data: series(today.AddDate(0, 0, -3000), syncNow, dayDur, 2),
		errIf: func(s, e time.Time) bool { return s.Before(aFrom.AddDate(0, 0, -300)) }}
	sy := newSyncer(t, a, b)
	_ = sy.Sync(context.Background(), "BTCUSDT", "1d")
	if v, _ := sy.Store.GetKV(ListingKey("BTCUSDT", "1d")); v != "" {
		t.Fatalf("listing recorded despite errors: %q", v)
	}
}

// QA-8：老段所有源都报错，之后的段有数据：报错段不能被当成「未上线」。
func TestSync_ListingNotRecordedAllError(t *testing.T) {
	cut := today.AddDate(0, 0, -400)
	bad := func(s, e time.Time) bool { return s.Before(cut) }
	a := &fakeProvider{name: "A", sem: okxSem, data: series(cut, syncNow, dayDur, 1), errIf: bad}
	b := &fakeProvider{name: "B", sem: binanceSem, data: series(cut, syncNow, dayDur, 2), errIf: bad}
	sy := newSyncer(t, a, b)
	_ = sy.Sync(context.Background(), "BTCUSDT", "1d")
	if v, _ := sy.Store.GetKV(ListingKey("BTCUSDT", "1d")); v != "" {
		t.Fatalf("listing recorded despite all-source errors: %q", v)
	}
}

// QA-8（S29 的非空形态）：前面各段都成功返回 0 根，接着一段 A 只给出部分数据、B 报错。
// B 在该段可能有更早的数据 ⇒ 不能把 A 的最早一根记成上线日；A 的数据照常入库（R1-5）。
func TestSync_ListingNotRecordedOnPartialWithError(t *testing.T) {
	// 取 2010 起的第 10 段 [seg, seg+300d)：A 从段内第 150 天起有数据，B 从第 50 天起有、但对该段报错
	seg := dailyBackfillStart.AddDate(0, 0, 10*chunkBars)
	d := seg.AddDate(0, 0, 150)
	hit := func(s, e time.Time) bool { return !s.After(d) && e.After(d) }
	a := &fakeProvider{name: "A", sem: binanceSem, data: series(d, syncNow, dayDur, 1)}
	b := &fakeProvider{name: "B", sem: binanceSem, data: series(seg.AddDate(0, 0, 50), syncNow, dayDur, 2), errIf: hit}
	sy := newSyncer(t, a, b)
	if err := sy.Sync(context.Background(), "BTCUSDT", "1d"); err == nil {
		t.Fatal("want error from B")
	}
	if v, _ := sy.Store.GetKV(ListingKey("BTCUSDT", "1d")); v != "" {
		t.Fatalf("listing recorded from a segment where a source errored: %q", v)
	}
	if !hasCandle(t, sy.Store, "1d", d) {
		t.Fatal("A's partial data for the erroring segment must still be stored")
	}
}

// QA-8 第三道：本地已有数据的段（完整而被跳过，或含本地最新一根而被强制重取却返回 0 根）
// ⇒ 此前不可能「未上线」，上线日判定必须停下。
func TestSync_ListingStoppedBySkippedSegment(t *testing.T) {
	firstSeg := series(dailyBackfillStart, dailyBackfillStart.AddDate(0, 0, chunkBars-1), dayDur, 1)
	cases := []struct {
		name  string
		local []core.OHLCV
	}{
		// 本地另有今天的数据 ⇒ 第一段不含本地最新一根 ⇒ 走「覆盖率 ≥90% 跳过」
		{"完整段被跳过", append(append([]core.OHLCV(nil), firstSeg...), candle(today, 2, 1, 1))},
		// 本地只有第一段 ⇒ 它含本地最新一根 ⇒ 被强制重取，数据源对它返回 0 根
		{"含最新一根的段重取得 0 根", firstSeg},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 数据源只有最近 1000 天，2010 起的各段都返回 0 根
			p := &fakeProvider{name: "p", sem: binanceSem, data: series(today.AddDate(0, 0, -1000), syncNow, dayDur, 1)}
			sy := newSyncer(t, p)
			mustUpsert(t, sy.Store, "BTCUSDT", "1d", c.local...)
			mustSync(t, sy, "BTCUSDT", "1d")
			if v, _ := sy.Store.GetKV(ListingKey("BTCUSDT", "1d")); v != "" {
				t.Fatalf("listing recorded although local data exists in an earlier segment: %q", v)
			}
		})
	}
	// 对照：同样的数据源、本地为空时会记上线日（排除「本来就不会记」）
	p := &fakeProvider{name: "p", sem: binanceSem, data: series(today.AddDate(0, 0, -1000), syncNow, dayDur, 1)}
	sy := newSyncer(t, p)
	mustSync(t, sy, "BTCUSDT", "1d")
	if v, _ := sy.Store.GetKV(ListingKey("BTCUSDT", "1d")); v == "" {
		t.Fatal("control: listing should be recorded when every leading segment was fetched and empty")
	}
}

// AD-29：与最短窗口相交的段只要缺一根就重取；完全早于最短窗口的段仍按 90%（缺一根不重取）。
func TestSync_RecentSegmentsNeedFullCoverage(t *testing.T) {
	t.Run("1d：30d 内缺一根补齐，30d 外不补", func(t *testing.T) {
		// 上线日取在使某段边界恰落在 now−15d：该段 [now−315d, now−15d) 与 30d 窗口相交、与 7d 窗口不相交
		listing := today.AddDate(0, 0, -15-3*chunkBars)
		p := &fakeProvider{name: "p", sem: binanceSem, data: series(listing, syncNow, dayDur, 1)}
		sy := newSyncer(t, p)
		mustSync(t, sy, "BTCUSDT", "1d") // 首次同步记下上线日 = listing，之后按 listing 切段
		if v, _ := sy.Store.GetKV(ListingKey("BTCUSDT", "1d")); v != strconv.FormatInt(listing.UnixMilli(), 10) {
			t.Fatalf("setup: listing = %q, want %d", v, listing.UnixMilli())
		}
		recent, old := today.AddDate(0, 0, -20), today.AddDate(0, 0, -400)
		deleteCandle(t, sy.Store, "1d", recent)
		deleteCandle(t, sy.Store, "1d", old)

		mustSync(t, sy, "BTCUSDT", "1d")
		if !hasCandle(t, sy.Store, "1d", recent) {
			t.Fatal("gap inside the 30d window must be refetched (AD-29)")
		}
		if hasCandle(t, sy.Store, "1d", old) {
			t.Fatal("segment entirely before the 30d window should still be skipped at ≥90%")
		}
	})

	t.Run("5m：24h 内缺一根补齐，24h 外不补", func(t *testing.T) {
		const fiveMin = 5 * time.Minute
		p := &fakeProvider{name: "p", sem: binanceSem, data: series(syncNow.Add(-8*dayDur), syncNow.Add(-fiveMin), fiveMin, 1)}
		sy := newSyncer(t, p)
		mustSync(t, sy, "BTCUSDT", "5m")
		recent, old := syncNow.Add(-2*time.Hour), syncNow.Add(-3*dayDur)
		deleteCandle(t, sy.Store, "5m", recent)
		deleteCandle(t, sy.Store, "5m", old)

		mustSync(t, sy, "BTCUSDT", "5m")
		if !hasCandle(t, sy.Store, "5m", recent) {
			t.Fatal("gap inside the 24h window must be refetched (AD-29)")
		}
		if hasCandle(t, sy.Store, "5m", old) {
			t.Fatal("segment entirely before the 24h window should still be skipped at ≥90%")
		}
	})
}

// R1-5：binance（主回填源）全挂、okx 只有最近 1440 根。okx 能给的照常入库（含不足 90% 的边界段），
// 同时 Sync 上抛含 pair/bar 的错误，使 sync: 健康键可报；不得记上线日。
func TestSync_BinanceDownReportsError(t *testing.T) {
	data := series(today.AddDate(0, 0, -3000), syncNow, dayDur, 1)
	okx := &fakeProvider{name: "okx", sem: okxSem, data: data}
	bin := &fakeProvider{name: "binance", sem: binanceSem, data: data,
		errIf: func(time.Time, time.Time) bool { return true }}
	sy := newSyncer(t, okx, bin)

	err := sy.Sync(context.Background(), "BTCUSDT", "1d")
	if err == nil || !strings.Contains(err.Error(), "BTCUSDT") || !strings.Contains(err.Error(), "1d") || !strings.Contains(err.Error(), "binance") {
		t.Fatalf("want error naming pair, bar and the failing source, got %v", err)
	}
	if got := countAll(t, sy, "BTCUSDT", "1d"); got != okxHorizon {
		t.Fatalf("everything okx can serve must be stored, want %d got %d", okxHorizon, got)
	}
	if v, _ := sy.Store.GetKV(ListingKey("BTCUSDT", "1d")); v != "" {
		t.Fatalf("listing must not be recorded while binance is down: %q", v)
	}
}
