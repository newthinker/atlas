package edgar

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context Checkpoint: done_criteria → test mapping
//   functional[0] CIK 零填充 + UA 头                 → TestFetchCompanyFactsRequest
//   functional[1] 季度判定/Q4 推导/filed/instant     → TestFetchCompanyFactsQuarterization
//   functional[2] 修正重报去重取 filed 最新           → TestFetchCompanyFactsQuarterization (Q1=0.6, 非旧 0.5)
//   boundary[0]   某季科目缺失 → NaN                  → TestFetchCompanyFactsQuarterization (Q2 shares)
//   boundary[1]   Revenue tag 回退分支                → TestFetchCompanyFactsRevenueFallback
//   error[0]      非 us-gaap → ErrNotUSGAAP           → TestFetchCompanyFactsIFRS
//   error[1]      HTTP 非 200 → 含状态码与 CIK 的错误  → TestFetchCompanyFactsHTTPError
//   加强项        季度不全的 FY 不产 Q4 (qCount==3)    → TestFetchCompanyFactsPartialFYNoQ4

// factsFileServer serves a fixture file, asserting the requested path.
func factsFileServer(t *testing.T, fixture, wantPath string) (*httptest.Server, *http.Request) {
	t.Helper()
	body, err := os.ReadFile(fixture)
	require.NoError(t, err)
	var captured http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = *r
		assert.Equal(t, wantPath, r.URL.Path)
		w.Write(body)
	}))
	return srv, &captured
}

func TestFetchCompanyFactsRequest(t *testing.T) {
	srv, captured := factsFileServer(t, "testdata/companyfacts_mini.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("atlas-prism/1.0 (test@example.com)", srv.URL)
	_, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	assert.Contains(t, captured.Header.Get("User-Agent"), "test@example.com")
}

func TestFetchCompanyFactsQuarterization(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_mini.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 4, "3 个 10-Q + 1 个推导 Q4")

	// functional[2] 修正重报去重:Q1 EPS 有两条 (filed 2025-05-01 val 0.5 / filed 2025-05-28 val 0.6),
	// 去重后取 filed 最新那条 0.6,而非旧条目 0.5。
	q1 := facts[0]
	assert.Equal(t, "2026Q1", q1.FiscalPeriod)
	assert.InDelta(t, 0.6, q1.EPSDiluted, 1e-9, "去重取 filed 最新 0.6,非旧条目 0.5")

	q4 := facts[3] // period_end 升序,最后为 Q4
	assert.Equal(t, "2026Q4", q4.FiscalPeriod)
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), q4.PeriodEnd)
	assert.Equal(t, time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC), q4.FilingDate, "Q4 生效日=10-K filed")
	assert.InDelta(t, 3.0-(0.6+0.7+0.8), q4.EPSDiluted, 1e-9, "Q4 EPS = FY − 3Q(近似口径)")
	assert.InDelta(t, 170000-(40000+42000+44000), q4.Revenue, 1e-9)
	assert.InDelta(t, 74000-(15000+17000+19000), q4.NetIncome, 1e-9)
	assert.InDelta(t, 95000.0, q4.Equity, 1e-9, "instant 型按 end 对齐")

	// boundary[0] Q2 shares 缺失 → NaN,不影响其他科目。
	q2 := facts[1]
	assert.Equal(t, "2026Q2", q2.FiscalPeriod)
	assert.True(t, math.IsNaN(q2.DilutedShares), "该季 shares 缺失 → NaN")
	assert.InDelta(t, 0.7, q2.EPSDiluted, 1e-9, "shares 缺失不影响 EPS")
}

// boundary[1] Revenue tag 回退分支:第一优先 tag 缺失 → 回退 Revenues 仍取到 Revenue。
func TestFetchCompanyFactsRevenueFallback(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_rev_fallback.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "2026Q1", facts[0].FiscalPeriod)
	assert.InDelta(t, 40000.0, facts[0].Revenue, 1e-9, "第一优先 tag 缺失 → 回退 Revenues 取到 40000")
}

// 加强项:季度不全的 FY(只有 Q1、Q2)→ qCount==2 → 不推导 Q4。
func TestFetchCompanyFactsPartialFYNoQ4(t *testing.T) {
	srv, _ := factsFileServer(t, "testdata/companyfacts_partial_fy.json", "/api/xbrl/companyfacts/CIK0001045810.json")
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	facts, err := c.FetchCompanyFacts("1045810")
	require.NoError(t, err)
	require.Len(t, facts, 2, "只有 Q1、Q2,qCount!=3 不产 Q4")
	for _, f := range facts {
		assert.NotEqual(t, "2026Q4", f.FiscalPeriod, "季度不全不应推导 Q4")
	}
}

func TestFetchCompanyFactsIFRS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"cik": 1, "facts": {"ifrs-full": {}}}`))
	}))
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	_, err := c.FetchCompanyFacts("1")
	assert.ErrorIs(t, err, ErrNotUSGAAP)
}

// error[1] HTTP 非 200 → 返回含状态码与 CIK 的错误。
func TestFetchCompanyFactsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewWithBaseURL("ua (t@e.com)", srv.URL)
	_, err := c.FetchCompanyFacts("1045810")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	assert.Contains(t, err.Error(), "1045810")
}
