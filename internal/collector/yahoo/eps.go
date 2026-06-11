package yahoo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// defaultEPSBaseURL is Yahoo's fundamentals-timeseries endpoint. Unofficial:
// subject to anti-bot and schema changes; failures degrade per design §5.
const defaultEPSBaseURL = "https://query2.finance.yahoo.com/ws/fundamentals-timeseries/v1/finance/timeseries"

// timeseriesResponse models the fundamentals-timeseries payload, narrowed to
// the trailingDilutedEPS series this collector consumes.
type timeseriesResponse struct {
	Timeseries struct {
		Result []struct {
			TrailingDilutedEPS []struct {
				AsOfDate      string `json:"asOfDate"`
				ReportedValue struct {
					Raw float64 `json:"raw"`
				} `json:"reportedValue"`
			} `json:"trailingDilutedEPS"`
		} `json:"result"`
	} `json:"timeseries"`
}

// FetchEPSHistory returns the trailing-diluted-EPS (TTM) quarterly series for
// a single equity, sorted ascending by date. Index symbols (^ prefix) carry no
// filings and are rejected without issuing a request. Points with a
// non-positive reported value are kept; pruning them is the valuation layer's
// responsibility (design §2.5). An empty or field-less result yields an empty
// slice with a nil error so callers can apply their own point-count threshold.
func (y *Yahoo) FetchEPSHistory(symbol string, start, end time.Time) ([]core.EPSPoint, error) {
	if strings.HasPrefix(symbol, "^") {
		return nil, fmt.Errorf("eps history unavailable for index symbol %s", symbol)
	}
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/%s?type=trailingDilutedEPS&period1=%d&period2=%d",
		y.epsBaseURL, url.PathEscape(y.toYahooSymbol(symbol)), start.Unix(), end.Unix())

	req, err := y.newRequest(reqURL)
	if err != nil {
		return nil, err
	}

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching eps history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result timeseriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Timeseries.Result) == 0 {
		return []core.EPSPoint{}, nil
	}

	raw := result.Timeseries.Result[0].TrailingDilutedEPS
	points := make([]core.EPSPoint, 0, len(raw))
	for _, p := range raw {
		date, err := time.Parse("2006-01-02", p.AsOfDate)
		if err != nil {
			continue // skip points with an unparseable date
		}
		points = append(points, core.EPSPoint{Date: date, EPS: p.ReportedValue.Raw})
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].Date.Before(points[j].Date)
	})

	return points, nil
}
