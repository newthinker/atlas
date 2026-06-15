package lixinger

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// candlestickBody mirrors the real Lixinger cn/company/candlestick response
// captured live: success is HTTP 200 with code:1 (not 0), date is RFC3339,
// volume is an integer, and amount/change/to_r are extra fields we ignore.
const candlestickBody = `{"code":1,"message":"success","data":[` +
	`{"date":"2026-06-10T00:00:00+08:00","open":1252.08,"close":1275.88,"high":1282,"low":1250.21,"volume":3924400,"amount":4991686419,"change":0.0158,"stockCode":"600519","to_r":0.003139},` +
	`{"date":"2026-06-09T00:00:00+08:00","open":1262.99,"close":1256,"high":1263,"low":1252.55,"volume":2786000,"amount":3500590715,"change":-0.0055,"stockCode":"600519","to_r":0.002229}` +
	`]}`

func TestFetchHistory_RequestShapeAndParse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/cn/company/candlestick") {
			t.Errorf("wrong endpoint path: %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		if !strings.Contains(body, `"stockCode":"600519"`) {
			t.Errorf("payload must use singular stockCode, got: %s", body)
		}
		if strings.Contains(body, "stockCodes") {
			t.Errorf("payload must NOT use plural stockCodes (404s on real API): %s", body)
		}
		if !strings.Contains(body, `"type"`) {
			t.Errorf("payload must include required type, got: %s", body)
		}
		_, _ = w.Write([]byte(candlestickBody))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("test-key", srv.URL)
	rows, err := lx.FetchHistory("600519.SH",
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), "1d")
	if err != nil {
		t.Fatalf("FetchHistory must succeed despite code:1, got: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// FetchHistory must return CHRONOLOGICAL (oldest-first) order, matching the
	// eastmoney FetchHistory contract that the backtest replay assumes. The API
	// returns newest-first; the fixture is [06-10, 06-09], so after reversal
	// rows[0] is the OLDER 06-09 bar and rows[1] the newer 06-10 bar.
	r0 := rows[0]
	if r0.Symbol != "600519.SH" || r0.Interval != "1d" {
		t.Errorf("symbol/interval = %q/%q, want 600519.SH/1d", r0.Symbol, r0.Interval)
	}
	if !rows[0].Time.Before(rows[1].Time) {
		t.Errorf("rows must be chronological (oldest-first): rows[0]=%v rows[1]=%v", rows[0].Time, rows[1].Time)
	}
	if r0.Open != 1262.99 || r0.High != 1263 || r0.Low != 1252.55 || r0.Close != 1256 || r0.Volume != 2786000 {
		t.Errorf("oldest-first row[0] OHLCV mismatch (want 06-09 bar): %+v", r0)
	}
	wantOldest := time.Date(2026, 6, 9, 0, 0, 0, 0, time.FixedZone("", 8*3600))
	if !r0.Time.Equal(wantOldest) {
		t.Errorf("rows[0] must be oldest 06-09: got %v, want %v", r0.Time, wantOldest)
	}
	wantNewest := time.Date(2026, 6, 10, 0, 0, 0, 0, time.FixedZone("", 8*3600))
	if !rows[1].Time.Equal(wantNewest) {
		t.Errorf("rows[1] must be newest 06-10: got %v, want %v", rows[1].Time, wantNewest)
	}
}

func TestFetchHistory_HTTPErrorIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":0,"error":{"name":"ValidationError"}}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("test-key", srv.URL)
	if _, err := lx.FetchHistory("600519.SH", time.Now().AddDate(0, 0, -10), time.Now(), "1d"); err == nil {
		t.Error("non-200 HTTP must return an error")
	}
}

func TestFetchHistory_EmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"success","data":[]}`))
	}))
	defer srv.Close()

	lx := NewWithBaseURL("test-key", srv.URL)
	rows, err := lx.FetchHistory("600519.SH", time.Now().AddDate(0, 0, -10), time.Now(), "1d")
	if err != nil {
		t.Fatalf("empty data must not error, got: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}
