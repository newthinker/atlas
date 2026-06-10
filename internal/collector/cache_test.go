package collector

// Context Checkpoint: done_criteria → test mapping
// functional[0] "TTL 内相同 key 第二次命中，底层调用计数=1"        → TestCachedCollector_HitWithinTTL
// functional[1] "不同 symbol/interval/时间范围互不命中"          → TestCachedCollector_DistinctKeys
// functional[2] "命中返回副本，调用方修改不影响后续命中"          → TestCachedCollector_ReturnsCopy
// functional[3] "Name/SupportedMarkets 等其余方法透传底层"        → TestCachedCollector_PassThrough
// boundary[0]   "TTL 过期重新穿透；超 256 淘汰最旧"               → TestCachedCollector_TTLExpiry / TestCachedCollector_CapacityEviction / TestCachedCollector_KeyTruncatedToMinute
// error[0]      "底层返回 error 时不入缓存，下次仍穿透"           → TestCachedCollector_ErrorNotCached
// nonfunc[0]    "并发 FetchHistory -race 通过"                   → TestCachedCollector_ConcurrentRace

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// countingCollector is a test double that counts FetchHistory calls and records
// the most recent call arguments. It is configurable to return an error.
type countingCollector struct {
	mu            sync.Mutex
	historyCalls  int
	quoteCalls    int
	lastSymbol    string
	lastStart     time.Time
	lastEnd       time.Time
	lastInterval  string
	returnErr     error
	dataPerSymbol map[string][]core.OHLCV
}

func newCountingCollector() *countingCollector {
	return &countingCollector{dataPerSymbol: map[string][]core.OHLCV{}}
}

func (f *countingCollector) Name() string                    { return "fake" }
func (f *countingCollector) SupportedMarkets() []core.Market { return []core.Market{core.MarketCrypto} }
func (f *countingCollector) Init(cfg Config) error           { return nil }
func (f *countingCollector) Start(ctx context.Context) error { return nil }
func (f *countingCollector) Stop() error                     { return nil }
func (f *countingCollector) FetchQuote(symbol string) (*core.Quote, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.quoteCalls++
	return &core.Quote{Symbol: symbol}, nil
}

func (f *countingCollector) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.historyCalls++
	f.lastSymbol, f.lastStart, f.lastEnd, f.lastInterval = symbol, start, end, interval
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	if data, ok := f.dataPerSymbol[symbol]; ok {
		out := make([]core.OHLCV, len(data))
		copy(out, data)
		return out, nil
	}
	return []core.OHLCV{{Symbol: symbol, Interval: interval, Close: 1.0, Time: start}}, nil
}

func (f *countingCollector) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.historyCalls
}

var (
	tStart = time.Date(2026, 1, 1, 9, 30, 0, 0, time.UTC)
	tEnd   = time.Date(2026, 1, 2, 9, 30, 0, 0, time.UTC)
)

// functional[0]
func TestCachedCollector_HitWithinTTL(t *testing.T) {
	f := newCountingCollector()
	c := NewCached(f, time.Minute)

	if _, err := c.FetchHistory("BTC", tStart, tEnd, "1d"); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if _, err := c.FetchHistory("BTC", tStart, tEnd, "1d"); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if got := f.calls(); got != 1 {
		t.Fatalf("backing FetchHistory called %d times, want 1", got)
	}
}

// functional[1]
func TestCachedCollector_DistinctKeys(t *testing.T) {
	f := newCountingCollector()
	c := NewCached(f, time.Minute)

	// Vary one dimension at a time; each distinct key must pass through.
	_, _ = c.FetchHistory("BTC", tStart, tEnd, "1d")
	_, _ = c.FetchHistory("ETH", tStart, tEnd, "1d")                // different symbol
	_, _ = c.FetchHistory("BTC", tStart, tEnd, "1h")                // different interval
	_, _ = c.FetchHistory("BTC", tStart.Add(time.Hour), tEnd, "1d") // different start
	_, _ = c.FetchHistory("BTC", tStart, tEnd.Add(time.Hour), "1d") // different end

	if got := f.calls(); got != 5 {
		t.Fatalf("backing FetchHistory called %d times, want 5 (no cross-hits)", got)
	}
}

// functional[2]
func TestCachedCollector_ReturnsCopy(t *testing.T) {
	f := newCountingCollector()
	f.dataPerSymbol["BTC"] = []core.OHLCV{{Symbol: "BTC", Close: 100}, {Symbol: "BTC", Close: 200}}
	c := NewCached(f, time.Minute)

	first, err := c.FetchHistory("BTC", tStart, tEnd, "1d")
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	// Mutate the returned slice's element.
	first[0].Close = -999

	second, err := c.FetchHistory("BTC", tStart, tEnd, "1d")
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if second[0].Close != 100 {
		t.Fatalf("cache corrupted by caller mutation: got Close=%v, want 100", second[0].Close)
	}
	if f.calls() != 1 {
		t.Fatalf("expected cache hit (1 backing call), got %d", f.calls())
	}
}

// functional[3]
func TestCachedCollector_PassThrough(t *testing.T) {
	f := newCountingCollector()
	c := NewCached(f, time.Minute)

	if c.Name() != "fake" {
		t.Errorf("Name() = %q, want fake", c.Name())
	}
	mk := c.SupportedMarkets()
	if len(mk) != 1 || mk[0] != core.MarketCrypto {
		t.Errorf("SupportedMarkets() = %v, want [crypto]", mk)
	}
	if _, err := c.FetchQuote("BTC"); err != nil {
		t.Errorf("FetchQuote passthrough err: %v", err)
	}
	if f.quoteCalls != 1 {
		t.Errorf("FetchQuote not passed through, quoteCalls=%d", f.quoteCalls)
	}
	if err := c.Init(Config{}); err != nil {
		t.Errorf("Init passthrough err: %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Errorf("Start passthrough err: %v", err)
	}
	if err := c.Stop(); err != nil {
		t.Errorf("Stop passthrough err: %v", err)
	}
}

// boundary[0] — TTL expiry
func TestCachedCollector_TTLExpiry(t *testing.T) {
	f := newCountingCollector()
	c := NewCached(f, 20*time.Millisecond)

	_, _ = c.FetchHistory("BTC", tStart, tEnd, "1d")
	_, _ = c.FetchHistory("BTC", tStart, tEnd, "1d") // hit
	if f.calls() != 1 {
		t.Fatalf("within TTL expected 1 backing call, got %d", f.calls())
	}
	time.Sleep(40 * time.Millisecond)
	_, _ = c.FetchHistory("BTC", tStart, tEnd, "1d") // expired → pass through
	if f.calls() != 2 {
		t.Fatalf("after TTL expiry expected 2 backing calls, got %d", f.calls())
	}
}

// boundary[0] — capacity eviction (oldest evicted, total <= 256)
func TestCachedCollector_CapacityEviction(t *testing.T) {
	f := newCountingCollector()
	c := NewCached(f, time.Hour)

	// Insert 257 distinct keys (vary start minute).
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 257; i++ {
		s := base.Add(time.Duration(i) * time.Minute)
		_, _ = c.FetchHistory("BTC", s, tEnd, "1d")
	}
	if got := c.entryCount(); got > 256 {
		t.Fatalf("entry count %d exceeds capacity 256", got)
	}
	callsBefore := f.calls()
	// The oldest (i=0) should have been evicted → re-fetch passes through.
	_, _ = c.FetchHistory("BTC", base, tEnd, "1d")
	if f.calls() != callsBefore+1 {
		t.Fatalf("oldest entry should have been evicted (expected pass-through), calls %d→%d", callsBefore, f.calls())
	}
	// A recent entry (i=256) should still be cached → hit.
	recent := base.Add(256 * time.Minute)
	callsBefore = f.calls()
	_, _ = c.FetchHistory("BTC", recent, tEnd, "1d")
	if f.calls() != callsBefore {
		t.Fatalf("recent entry should still be cached (hit), but got a pass-through")
	}
}

// boundary[0] — key time truncated to minute
func TestCachedCollector_KeyTruncatedToMinute(t *testing.T) {
	f := newCountingCollector()
	c := NewCached(f, time.Hour)

	s1 := time.Date(2026, 1, 1, 9, 30, 15, 0, time.UTC)
	s2 := time.Date(2026, 1, 1, 9, 30, 59, 0, time.UTC) // same minute, different seconds
	_, _ = c.FetchHistory("BTC", s1, tEnd, "1d")
	_, _ = c.FetchHistory("BTC", s2, tEnd, "1d")
	if f.calls() != 1 {
		t.Fatalf("times within same minute should share a cache key, got %d backing calls", f.calls())
	}
}

// error[0]
func TestCachedCollector_ErrorNotCached(t *testing.T) {
	f := newCountingCollector()
	f.returnErr = errors.New("boom")
	c := NewCached(f, time.Hour)

	if _, err := c.FetchHistory("BTC", tStart, tEnd, "1d"); err == nil {
		t.Fatal("expected error from backing collector")
	}
	if _, err := c.FetchHistory("BTC", tStart, tEnd, "1d"); err == nil {
		t.Fatal("expected error on second call too")
	}
	if f.calls() != 2 {
		t.Fatalf("error result must not be cached: expected 2 backing calls, got %d", f.calls())
	}
}

// nonfunc[0]
func TestCachedCollector_ConcurrentRace(t *testing.T) {
	f := newCountingCollector()
	c := NewCached(f, time.Hour)

	symbols := []string{"BTC", "ETH", "SOL", "ADA"}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sym := symbols[i%len(symbols)]
			s := tStart.Add(time.Duration(i%8) * time.Minute)
			if _, err := c.FetchHistory(sym, s, tEnd, "1d"); err != nil {
				t.Errorf("concurrent fetch: %v", err)
			}
		}(i)
	}
	wg.Wait()
}
