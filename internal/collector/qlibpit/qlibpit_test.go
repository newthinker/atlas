// Context Checkpoint: done_criteria -> test mapping
// functional[0]  "observe_date <= end 截断防前视"           -> TestFetchEPSHistoryExcludesFutureObserveDate
// functional[1]  "修订按 observe_date 升序保留"              -> TestFetchEPSHistoryKeepsRevisionsOrdered
// boundary[0]    "observe_date == end 精确等值边界"           -> TestFetchEPSHistoryIncludesObserveDateEqualEnd
// error_handling[0] "仓库无符号 -> 委托 fallback"             -> TestFallbackUsedWhenNoWarehouseFundamentals
// error_handling[1] "仓库无符号 & fallback nil -> 空切片"     -> TestNoFundamentalsNoFallbackReturnsEmpty
// error_handling[2] "仓库有符号 -> 不调 fallback"             -> TestWarehouseFundamentalsPreemptFallback
package qlibpit

import (
	"database/sql"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
	_ "modernc.org/sqlite"
)

func d(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE fundamentals_pit(
symbol TEXT,report_period TEXT,observe_date TEXT,eps_ttm REAL,
pe REAL,pb REAL,ps REAL,roe REAL,dividend_yield REAL,
PRIMARY KEY(symbol,report_period,observe_date));`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func ins(t *testing.T, db *sql.DB, sym, rp, od string, eps float64) {
	t.Helper()
	if _, err := db.Exec(
		"INSERT INTO fundamentals_pit(symbol,report_period,observe_date,eps_ttm) VALUES(?,?,?,?)",
		sym, rp, od, eps); err != nil {
		t.Fatal(err)
	}
}

// Task 4 tests

func TestFetchEPSHistoryExcludesFutureObserveDate(t *testing.T) {
	db := newTestDB(t)
	ins(t, db, "AAPL", "2023-12-31", "2024-03-01", 3.0)
	ins(t, db, "AAPL", "2024-03-31", "2024-05-15", 4.0) // future: end=2024-04-01 not visible
	src := New(db, nil)
	got, err := src.FetchEPSHistory("aapl", d("2020-01-01"), d("2024-04-01"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].EPS != 3.0 || !got[0].Date.Equal(d("2024-03-01")) {
		t.Fatalf("lookahead leak: %+v", got)
	}
}

func TestFetchEPSHistoryKeepsRevisionsOrdered(t *testing.T) {
	db := newTestDB(t)
	ins(t, db, "AAPL", "2024-03-31", "2024-05-15", 4.0)
	ins(t, db, "AAPL", "2024-03-31", "2024-08-01", 4.2) // revision
	src := New(db, nil)
	got, _ := src.FetchEPSHistory("AAPL", d("2020-01-01"), d("2024-12-31"))
	if len(got) != 2 || got[0].EPS != 4.0 || got[1].EPS != 4.2 {
		t.Fatalf("revisions must be kept ascending by observe_date: %+v", got)
	}
}

// TestFetchEPSHistoryIncludesObserveDateEqualEnd pins the <= boundary:
// a row whose observe_date exactly equals the query end must be included.
func TestFetchEPSHistoryIncludesObserveDateEqualEnd(t *testing.T) {
	db := newTestDB(t)
	ins(t, db, "AAPL", "2024-03-31", "2024-06-15", 5.5) // observe_date == end
	src := New(db, nil)
	got, err := src.FetchEPSHistory("AAPL", d("2020-01-01"), d("2024-06-15"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].EPS != 5.5 {
		t.Fatalf("observe_date equal to end must be included (<=, not <): %+v", got)
	}
}

// Task 5 tests

type fakeEPS struct {
	pts    []core.EPSPoint
	called string
}

func (f *fakeEPS) FetchEPSHistory(sym string, _, _ time.Time) ([]core.EPSPoint, error) {
	f.called = sym
	return f.pts, nil
}

func TestFallbackUsedWhenNoWarehouseFundamentals(t *testing.T) {
	db := newTestDB(t) // empty fundamentals_pit
	fb := &fakeEPS{pts: []core.EPSPoint{{Date: d("2024-01-01"), EPS: 9.9}}}
	src := New(db, fb)
	got, err := src.FetchEPSHistory("MSFT", d("2020-01-01"), d("2024-12-31"))
	if err != nil || len(got) != 1 || got[0].EPS != 9.9 {
		t.Fatalf("should delegate to fallback, got %+v err=%v", got, err)
	}
	if fb.called != "MSFT" {
		t.Errorf("fallback should be called with symbol, got %q", fb.called)
	}
}

func TestNoFundamentalsNoFallbackReturnsEmpty(t *testing.T) {
	src := New(newTestDB(t), nil)
	got, err := src.FetchEPSHistory("X", d("2020-01-01"), d("2024-12-31"))
	if err != nil || len(got) != 0 {
		t.Fatalf("want empty/nil, got %+v err=%v", got, err)
	}
}

func TestWarehouseFundamentalsPreemptFallback(t *testing.T) {
	db := newTestDB(t)
	ins(t, db, "AAPL", "2024-03-31", "2024-05-15", 4.0)
	fb := &fakeEPS{pts: []core.EPSPoint{{Date: d("2024-01-01"), EPS: 9.9}}}
	src := New(db, fb)
	got, _ := src.FetchEPSHistory("AAPL", d("2020-01-01"), d("2024-12-31"))
	if len(got) != 1 || got[0].EPS != 4.0 {
		t.Fatalf("warehouse should preempt fallback, got %+v", got)
	}
	if fb.called != "" {
		t.Error("fallback must not be called when warehouse has data")
	}
}
