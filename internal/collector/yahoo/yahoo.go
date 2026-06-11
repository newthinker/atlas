package yahoo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

// defaultBaseURL is the production Yahoo Finance chart endpoint. It is the
// default for New; tests inject an httptest server URL via NewWithBaseURL.
const defaultBaseURL = "https://query1.finance.yahoo.com/v8/finance/chart"

// validSymbol matches stock symbols (AAPL, 600519.SH, 0700.HK),
// index symbols (^GSPC) and futures symbols (GC=F).
// Validation is purely syntactic and intentionally decoupled from the
// phase-1 coverage list (see design §2.1).
var validSymbol = regexp.MustCompile(`^(\^[A-Za-z0-9]{1,10}|[A-Za-z0-9]{1,10}(\.[A-Za-z]{1,4})?|[A-Za-z]{1,6}=F)$`)

// validateSymbol checks if a symbol has valid format
func validateSymbol(symbol string) error {
	if symbol == "" {
		return fmt.Errorf("symbol cannot be empty")
	}
	if len(symbol) > 20 {
		return fmt.Errorf("symbol too long: %s", symbol)
	}
	if !validSymbol.MatchString(symbol) {
		return fmt.Errorf("invalid symbol format: %s", symbol)
	}
	return nil
}

// Yahoo implements the Yahoo Finance collector
type Yahoo struct {
	client     *http.Client
	config     collector.Config
	baseURL    string
	epsBaseURL string
}

// New creates a new Yahoo collector pointing at the production endpoints.
func New() *Yahoo {
	return NewWithBaseURLs(defaultBaseURL, defaultEPSBaseURL)
}

// NewWithBaseURL creates a Yahoo collector with a custom chart endpoint, using
// the same URL for both chart and EPS endpoints. It keeps existing tests that
// only wire a single httptest server working unchanged.
func NewWithBaseURL(baseURL string) *Yahoo {
	return NewWithBaseURLs(baseURL, baseURL)
}

// NewWithBaseURLs creates a Yahoo collector with independent chart and EPS
// endpoints. It is intended for tests that point each at an httptest server.
func NewWithBaseURLs(chartURL, epsURL string) *Yahoo {
	return &Yahoo{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL:    chartURL,
		epsBaseURL: epsURL,
	}
}

// newRequest builds a GET request carrying browser-like headers. Yahoo's
// unofficial endpoints return 403 without a User-Agent, so every code path
// (quote, history, EPS) must share these headers.
func (y *Yahoo) newRequest(reqURL string) (*http.Request, error) {
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	return req, nil
}

func (y *Yahoo) Name() string {
	return "yahoo"
}

func (y *Yahoo) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketUS, core.MarketHK, core.MarketEU}
}

func (y *Yahoo) Init(cfg collector.Config) error {
	y.config = cfg
	return nil
}

func (y *Yahoo) Start(ctx context.Context) error {
	return nil
}

func (y *Yahoo) Stop() error {
	return nil
}

// toYahooSymbol converts internal symbol format to Yahoo format
func (y *Yahoo) toYahooSymbol(symbol string) string {
	// Shanghai stocks: 600519.SH -> 600519.SS
	if strings.HasSuffix(symbol, ".SH") {
		return strings.TrimSuffix(symbol, ".SH") + ".SS"
	}
	return symbol
}

// FetchQuote fetches real-time quote
func (y *Yahoo) FetchQuote(symbol string) (*core.Quote, error) {
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}
	yahooSymbol := y.toYahooSymbol(symbol)
	reqURL := fmt.Sprintf("%s/%s?interval=1d&range=1d", y.baseURL, url.PathEscape(yahooSymbol))

	req, err := y.newRequest(reqURL)
	if err != nil {
		return nil, err
	}

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching quote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s", result.Chart.Error.Description)
	}

	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data for symbol: %s", symbol)
	}

	r := result.Chart.Result[0]
	meta := r.Meta

	// Calculate change from previous close
	change := meta.RegularMarketPrice - meta.ChartPreviousClose

	return &core.Quote{
		Symbol:        symbol,
		Market:        y.detectMarket(symbol),
		Price:         meta.RegularMarketPrice,
		Open:          meta.RegularMarketOpen,
		High:          meta.RegularMarketDayHigh,
		Low:           meta.RegularMarketDayLow,
		PrevClose:     meta.ChartPreviousClose,
		Change:        change,
		ChangePercent: meta.RegularMarketChangePercent,
		Volume:        int64(meta.RegularMarketVolume),
		Time:          time.Unix(int64(meta.RegularMarketTime), 0),
		Source:        "yahoo",
	}, nil
}

// FetchHistory fetches historical OHLCV data
func (y *Yahoo) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if err := validateSymbol(symbol); err != nil {
		return nil, err
	}
	yahooSymbol := y.toYahooSymbol(symbol)
	yahooInterval := y.toYahooInterval(interval)

	reqURL := fmt.Sprintf("%s/%s?interval=%s&period1=%d&period2=%d",
		y.baseURL, url.PathEscape(yahooSymbol), yahooInterval, start.Unix(), end.Unix())

	req, err := y.newRequest(reqURL)
	if err != nil {
		return nil, err
	}

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error: %s", result.Chart.Error.Description)
	}

	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data for symbol: %s", symbol)
	}

	r := result.Chart.Result[0]
	timestamps := r.Timestamp
	quotes := r.Indicators.Quote[0]

	data := make([]core.OHLCV, 0, len(timestamps))
	for i, ts := range timestamps {
		if quotes.Open[i] == nil {
			continue // Skip missing data
		}
		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     *quotes.Open[i],
			High:     *quotes.High[i],
			Low:      *quotes.Low[i],
			Close:    *quotes.Close[i],
			Volume:   int64(*quotes.Volume[i]),
			Time:     time.Unix(int64(ts), 0),
		})
	}

	return data, nil
}

func (y *Yahoo) toYahooInterval(interval string) string {
	switch interval {
	case "1m":
		return "1m"
	case "5m":
		return "5m"
	case "1h":
		return "1h"
	case "1d":
		return "1d"
	default:
		return "1d"
	}
}

func (y *Yahoo) detectMarket(symbol string) core.Market {
	if strings.HasSuffix(symbol, ".HK") {
		return core.MarketHK
	}
	if strings.HasSuffix(symbol, ".SH") || strings.HasSuffix(symbol, ".SZ") {
		return core.MarketCNA
	}
	return core.MarketUS
}

// Yahoo API response types
type chartResponse struct {
	Chart struct {
		Result []chartResult `json:"result"`
		Error  *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

type chartResult struct {
	Meta       chartMeta  `json:"meta"`
	Timestamp  []int      `json:"timestamp"`
	Indicators indicators `json:"indicators"`
}

type chartMeta struct {
	Symbol                     string  `json:"symbol"`
	RegularMarketPrice         float64 `json:"regularMarketPrice"`
	RegularMarketVolume        int     `json:"regularMarketVolume"`
	RegularMarketTime          int     `json:"regularMarketTime"`
	RegularMarketOpen          float64 `json:"regularMarketOpen"`
	RegularMarketDayHigh       float64 `json:"regularMarketDayHigh"`
	RegularMarketDayLow        float64 `json:"regularMarketDayLow"`
	ChartPreviousClose         float64 `json:"chartPreviousClose"`
	RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
}

type indicators struct {
	Quote []quoteIndicator `json:"quote"`
}

type quoteIndicator struct {
	Open   []*float64 `json:"open"`
	High   []*float64 `json:"high"`
	Low    []*float64 `json:"low"`
	Close  []*float64 `json:"close"`
	Volume []*int     `json:"volume"`
}
