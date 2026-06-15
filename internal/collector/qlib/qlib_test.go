// Context Checkpoint: done_criteria → test mapping
// T5 functional[0] "Covers reports warehouse membership (case-insensitive)" → TestCoversReportsWarehouseMembership
// T5 functional[1] "Name() returns qlib"                                    → TestNameIsQlib
// T6 functional[0] "FetchHistory reads warehouse range [start,end]"         → TestFetchHistoryReadsWarehouseRange
// T6 boundary[0]   "FetchHistory caps at last_date when end > last_date"    → TestFetchHistoryCapsAtLastDate
// T7 functional[0] "FetchHistory appends tail from external"                → TestFetchHistoryAppendsTail
// T7 error_handling[0] "tail-fill failure degrades gracefully"              → TestFetchHistoryTailFailureDegrades
// T8 functional[0] "FetchQuote delegates to external"                       → TestFetchQuoteDelegates
// T8 functional[1] "FetchHistory non-daily delegates to external"           → TestFetchHistoryNonDailyDelegates

package qlib

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// newTestDB builds an in-memory warehouse with the given ohlcv rows + meta.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE ohlcv(symbol TEXT,date TEXT,open REAL,high REAL,low REAL,close REAL,volume INTEGER,adj_close REAL,PRIMARY KEY(symbol,date));
CREATE TABLE warehouse_meta(symbol TEXT PRIMARY KEY,market TEXT,source TEXT,last_date TEXT,dumped_at TEXT);`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCoversReportsWarehouseMembership(t *testing.T) {
	db := newTestDB(t)
	_, _ = db.Exec("INSERT INTO warehouse_meta VALUES('AAPL','US','yahoo','2024-01-03','x')")
	c := New(db)
	if !c.Covers("aapl") {
		t.Error("expected Covers(aapl)=true (case-insensitive)")
	}
	if c.Covers("MSFT") {
		t.Error("expected Covers(MSFT)=false")
	}
}

func TestNameIsQlib(t *testing.T) {
	if New(newTestDB(t)).Name() != "qlib" {
		t.Error("Name should be qlib")
	}
}

func d(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func seedOHLCV(t *testing.T, db *sql.DB) {
	t.Helper()
	_, _ = db.Exec("INSERT INTO warehouse_meta VALUES('AAPL','US','yahoo','2024-01-04','x')")
	for _, r := range [][2]string{{"2024-01-02", "1.0"}, {"2024-01-03", "1.1"}, {"2024-01-04", "1.2"}} {
		_, _ = db.Exec("INSERT INTO ohlcv(symbol,date,open,high,low,close,volume,adj_close) VALUES('AAPL',?,?,?,?,?,?,?)",
			r[0], r[1], r[1], r[1], r[1], 100, r[1])
	}
}

func TestFetchHistoryReadsWarehouseRange(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db)
	c := New(db, WithClock(func() time.Time { return d("2024-01-05") }))
	got, err := c.FetchHistory("aapl", d("2024-01-02"), d("2024-01-04"), "1d")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 bars, got %d", len(got))
	}
	if got[0].Close != 1.0 || got[2].Close != 1.2 {
		t.Errorf("unexpected close sequence: %+v", got)
	}
	if got[0].Interval != "1d" || got[0].Symbol != "AAPL" {
		t.Errorf("bad metadata: %+v", got[0])
	}
	// F2: date parsed via 2006-01-02; volume scanned as int64.
	if !got[0].Time.Equal(d("2024-01-02")) {
		t.Errorf("bad time parse: got %v want %v", got[0].Time, d("2024-01-02"))
	}
	if got[0].Volume != int64(100) {
		t.Errorf("bad volume: got %d want 100", got[0].Volume)
	}
}

func TestFetchHistoryCapsAtLastDate(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db)
	// end beyond last_date, no external -> only warehouse段 (<=2024-01-04)
	c := New(db, WithClock(func() time.Time { return d("2024-01-10") }))
	got, _ := c.FetchHistory("AAPL", d("2024-01-02"), d("2024-01-09"), "1d")
	if len(got) != 3 {
		t.Fatalf("want 3 (capped at last_date), got %d", len(got))
	}
}

// fakeExt is a fake external collector for tail-fill tests.
type fakeExt struct {
	bars []core.OHLCV
	err  error
	got  [2]time.Time // captured start,end
}

func (f *fakeExt) Name() string                    { return "fake" }
func (f *fakeExt) SupportedMarkets() []core.Market { return nil }
func (f *fakeExt) Init(collector.Config) error     { return nil }
func (f *fakeExt) Start(_ context.Context) error   { return nil }
func (f *fakeExt) Stop() error                     { return nil }
func (f *fakeExt) FetchQuote(string) (*core.Quote, error) {
	return nil, errors.New("no")
}
func (f *fakeExt) FetchHistory(_ string, s, e time.Time, _ string) ([]core.OHLCV, error) {
	f.got = [2]time.Time{s, e}
	return f.bars, f.err
}

func TestFetchHistoryAppendsTail(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db) // last_date 2024-01-04
	ext := &fakeExt{bars: []core.OHLCV{{Symbol: "AAPL", Interval: "1d", Close: 1.3, Time: d("2024-01-05")}}}
	c := New(db, WithClock(func() time.Time { return d("2024-01-06") }),
		WithExternal(func(string) collector.Collector { return ext }))
	got, err := c.FetchHistory("AAPL", d("2024-01-02"), d("2024-01-05"), "1d")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[3].Close != 1.3 {
		t.Fatalf("want 4 bars incl tail, got %d: %+v", len(got), got)
	}
	if !ext.got[0].Equal(d("2024-01-05")) {
		t.Errorf("tail should start day after last_date, got %v", ext.got[0])
	}
}

func TestFetchHistoryTailFailureDegrades(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db)
	ext := &fakeExt{err: errors.New("api down")}
	c := New(db, WithClock(func() time.Time { return d("2024-01-06") }),
		WithExternal(func(string) collector.Collector { return ext }))
	got, err := c.FetchHistory("AAPL", d("2024-01-02"), d("2024-01-05"), "1d")
	if err != nil {
		t.Fatalf("tail failure must not error, got %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want warehouse段 only (3), got %d", len(got))
	}
}

// fakeExtQuote wraps fakeExt to also serve quotes.
type fakeExtQuote struct {
	fakeExt
	q *core.Quote
}

func (f *fakeExtQuote) FetchQuote(string) (*core.Quote, error) { return f.q, nil }

func TestFetchQuoteDelegates(t *testing.T) {
	db := newTestDB(t)
	q := &core.Quote{Symbol: "AAPL", Price: 9.9}
	ext := &fakeExtQuote{q: q}
	c := New(db, WithExternal(func(string) collector.Collector { return ext }))
	got, err := c.FetchQuote("AAPL")
	if err != nil || got.Price != 9.9 {
		t.Fatalf("expected delegated quote, got %+v err=%v", got, err)
	}
}

func TestFetchHistoryNonDailyDelegates(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db)
	ext := &fakeExt{bars: []core.OHLCV{{Symbol: "AAPL", Interval: "5m", Close: 7}}}
	c := New(db, WithExternal(func(string) collector.Collector { return ext }))
	got, _ := c.FetchHistory("AAPL", d("2024-01-02"), d("2024-01-04"), "5m")
	if len(got) != 1 || got[0].Interval != "5m" {
		t.Fatalf("non-daily should delegate fully, got %+v", got)
	}
}

func TestSupportedMarkets(t *testing.T) {
	got := New(newTestDB(t)).SupportedMarkets()
	want := map[core.Market]bool{core.MarketUS: false, core.MarketCNA: false, core.MarketHK: false}
	for _, m := range got {
		if _, ok := want[m]; ok {
			want[m] = true
		}
	}
	for m, seen := range want {
		if !seen {
			t.Errorf("SupportedMarkets missing %v: got %+v", m, got)
		}
	}
}

func TestLifecycleNoOps(t *testing.T) {
	c := New(newTestDB(t))
	if err := c.Init(collector.Config{}); err != nil {
		t.Errorf("Init should return nil, got %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Errorf("Start should return nil, got %v", err)
	}
	if err := c.Stop(); err != nil {
		t.Errorf("Stop should return nil, got %v", err)
	}
}

func TestWithMaxStalenessOption(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db)
	c := New(db, WithMaxStaleness(1*time.Hour),
		WithClock(func() time.Time { return d("2024-01-05") }))
	got, err := c.FetchHistory("AAPL", d("2024-01-02"), d("2024-01-04"), "1d")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 bars, got %d", len(got))
	}
}

func TestFetchQuoteNoExternalErrors(t *testing.T) {
	c := New(newTestDB(t))
	got, err := c.FetchQuote("AAPL")
	if err == nil {
		t.Fatalf("expected error with no external, got quote %+v", got)
	}
}

// T6 error_handling: daily request for a symbol absent from warehouse errors.
func TestFetchHistoryUnknownSymbolErrors(t *testing.T) {
	db := newTestDB(t) // no warehouse_meta rows seeded
	got, err := c(db).FetchHistory("ZZZZ", d("2024-01-02"), d("2024-01-04"), "1d")
	if err == nil {
		t.Fatalf("expected error for unknown symbol, got bars %+v", got)
	}
	if !strings.Contains(err.Error(), "not in warehouse") {
		t.Errorf("error should mention 'not in warehouse', got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty bars on error, got %d", len(got))
	}
}

// T8 error_handling: non-daily request without external source errors.
func TestFetchHistoryNonDailyNoExternalErrors(t *testing.T) {
	db := newTestDB(t)
	seedOHLCV(t, db)
	got, err := c(db).FetchHistory("AAPL", d("2024-01-02"), d("2024-01-04"), "5m")
	if err == nil {
		t.Fatalf("expected error for intraday without external, got bars %+v", got)
	}
	if !strings.Contains(err.Error(), "only stores daily bars") {
		t.Errorf("error should mention 'only stores daily bars', got %v", err)
	}
}

func c(db *sql.DB) *Collector { return New(db) }
