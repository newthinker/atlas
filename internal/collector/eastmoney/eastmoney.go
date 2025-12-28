package eastmoney

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

const (
	quoteURL   = "https://push2.eastmoney.com/api/qt/stock/get"
	historyURL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
)

// Eastmoney implements the Eastmoney collector for A-shares
type Eastmoney struct {
	client *http.Client
	config collector.Config
}

// New creates a new Eastmoney collector
func New() *Eastmoney {
	return &Eastmoney{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (e *Eastmoney) Name() string {
	return "eastmoney"
}

func (e *Eastmoney) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketCNA}
}

func (e *Eastmoney) Init(cfg collector.Config) error {
	e.config = cfg
	return nil
}

func (e *Eastmoney) Start(ctx context.Context) error {
	return nil
}

func (e *Eastmoney) Stop() error {
	return nil
}

// parseSymbol converts 600519.SH to (600519, 1) for Eastmoney API
// Shanghai = 1, Shenzhen = 0
func (e *Eastmoney) parseSymbol(symbol string) (code, market string) {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 {
		return symbol, "1"
	}

	code = parts[0]
	switch parts[1] {
	case "SH":
		market = "1"
	case "SZ":
		market = "0"
	default:
		market = "1"
	}
	return
}

// FetchQuote fetches real-time quote from Eastmoney
func (e *Eastmoney) FetchQuote(symbol string) (*core.Quote, error) {
	code, market := e.parseSymbol(symbol)
	secid := fmt.Sprintf("%s.%s", market, code)

	url := fmt.Sprintf("%s?secid=%s&fields=f43,f44,f45,f46,f47,f48,f50,f57,f58,f60",
		quoteURL, secid)

	resp, err := e.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching quote: %w", err)
	}
	defer resp.Body.Close()

	var result quoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Data == nil {
		return nil, fmt.Errorf("no data for symbol: %s", symbol)
	}

	d := result.Data
	return &core.Quote{
		Symbol: symbol,
		Market: core.MarketCNA,
		Price:  float64(d.F43) / 100, // Price in cents
		Volume: int64(d.F47),
		Bid:    float64(d.F44) / 100,
		Ask:    float64(d.F45) / 100,
		Time:   time.Now(),
		Source: "eastmoney",
	}, nil
}

// FetchHistory fetches historical OHLCV data
func (e *Eastmoney) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	code, market := e.parseSymbol(symbol)
	secid := fmt.Sprintf("%s.%s", market, code)
	klt := e.toKlineType(interval)

	url := fmt.Sprintf("%s?secid=%s&klt=%s&fqt=1&beg=%s&end=%s&fields1=f1,f2,f3,f4,f5,f6&fields2=f51,f52,f53,f54,f55,f56",
		historyURL, secid, klt,
		start.Format("20060102"),
		end.Format("20060102"))

	resp, err := e.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching history: %w", err)
	}
	defer resp.Body.Close()

	var result historyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Data == nil || len(result.Data.Klines) == 0 {
		return nil, fmt.Errorf("no history for symbol: %s", symbol)
	}

	data := make([]core.OHLCV, 0, len(result.Data.Klines))
	re := regexp.MustCompile(`(\d{4}-\d{2}-\d{2}),([^,]+),([^,]+),([^,]+),([^,]+),([^,]+)`)

	for _, line := range result.Data.Klines {
		matches := re.FindStringSubmatch(line)
		if len(matches) < 7 {
			continue
		}

		t, _ := time.Parse("2006-01-02", matches[1])
		open, _ := strconv.ParseFloat(matches[2], 64)
		closePrice, _ := strconv.ParseFloat(matches[3], 64)
		high, _ := strconv.ParseFloat(matches[4], 64)
		low, _ := strconv.ParseFloat(matches[5], 64)
		volume, _ := strconv.ParseInt(matches[6], 10, 64)

		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     open,
			High:     high,
			Low:      low,
			Close:    closePrice,
			Volume:   volume,
			Time:     t,
		})
	}

	return data, nil
}

func (e *Eastmoney) toKlineType(interval string) string {
	switch interval {
	case "1m":
		return "1"
	case "5m":
		return "5"
	case "15m":
		return "15"
	case "30m":
		return "30"
	case "1h":
		return "60"
	case "1d":
		return "101"
	default:
		return "101"
	}
}

// Response types
type quoteResponse struct {
	Data *quoteData `json:"data"`
}

type quoteData struct {
	F43 int    `json:"f43"` // Current price (cents)
	F44 int    `json:"f44"` // Bid
	F45 int    `json:"f45"` // Ask
	F46 int    `json:"f46"` // Open
	F47 int64  `json:"f47"` // Volume
	F48 int64  `json:"f48"` // Amount
	F57 string `json:"f57"` // Code
	F58 string `json:"f58"` // Name
}

type historyResponse struct {
	Data *historyData `json:"data"`
}

type historyData struct {
	Code   string   `json:"code"`
	Name   string   `json:"name"`
	Klines []string `json:"klines"`
}
