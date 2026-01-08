package eastmoney

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/collector/lixinger"
	"github.com/newthinker/atlas/internal/core"
)

const (
	quoteURL       = "https://push2.eastmoney.com/api/qt/stock/get"
	historyURL     = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
	fundURL        = "https://fundgz.1234567.com.cn/js"
	fundHistoryURL = "https://api.fund.eastmoney.com/f10/lsjz"
)

// Eastmoney implements the Eastmoney collector for A-shares
type Eastmoney struct {
	client           *http.Client
	config           collector.Config
	lixingerFallback *lixinger.Lixinger // Fallback collector for when Eastmoney fails
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

// SetLixingerFallback sets a Lixinger collector as fallback for when Eastmoney fails
func (e *Eastmoney) SetLixingerFallback(l *lixinger.Lixinger) {
	e.lixingerFallback = l
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

// isETF checks if the symbol is an ETF (exchange-traded fund)
// ETFs trade like stocks and have OHLCV data
func (e *Eastmoney) isETF(symbol string) bool {
	code, _ := e.parseSymbol(symbol)
	if len(code) != 6 {
		return false
	}
	// Shenzhen ETFs: 159xxx
	if strings.HasPrefix(code, "159") {
		return true
	}
	// Shanghai ETFs: 510xxx, 511xxx, 512xxx, 513xxx, 515xxx, 516xxx, 518xxx
	if strings.HasPrefix(code, "510") || strings.HasPrefix(code, "511") ||
		strings.HasPrefix(code, "512") || strings.HasPrefix(code, "513") ||
		strings.HasPrefix(code, "515") || strings.HasPrefix(code, "516") ||
		strings.HasPrefix(code, "518") {
		return true
	}
	return false
}

// isFund checks if the symbol is an open-end fund (开放式基金/场外基金)
// These funds only have daily NAV data, no intraday OHLCV
func (e *Eastmoney) isFund(symbol string) bool {
	code, _ := e.parseSymbol(symbol)
	if len(code) != 6 {
		return false
	}
	// ETFs are NOT open-end funds - they trade like stocks
	if e.isETF(symbol) {
		return false
	}
	// Open-end fund codes typically start with 0, 1, 2, 3, 5
	// (excluding ETF prefixes which are handled above)
	first := code[0]
	return first == '0' || first == '1' || first == '2' || first == '3' || first == '5'
}

// FetchQuote fetches real-time quote from Eastmoney
func (e *Eastmoney) FetchQuote(symbol string) (*core.Quote, error) {
	// Check if this is a fund
	if e.isFund(symbol) {
		return e.fetchFundQuote(symbol)
	}

	quote, err := e.fetchStockQuote(symbol)
	if err != nil {
		// Try Lixinger fallback if available
		if e.lixingerFallback != nil && e.lixingerFallback.HasAPIKey() {
			log.Printf("eastmoney: FetchQuote failed for %s, trying lixinger fallback: %v", symbol, err)
			return e.lixingerFallback.FetchQuote(symbol)
		}
		return nil, err
	}
	return quote, nil
}

// fetchStockQuote fetches stock quote from Eastmoney API
func (e *Eastmoney) fetchStockQuote(symbol string) (*core.Quote, error) {
	code, market := e.parseSymbol(symbol)
	secid := fmt.Sprintf("%s.%s", market, code)

	// f43=price, f44=bid, f45=ask, f46=open, f47=volume, f48=amount
	// f51=high, f52=low, f60=prev_close, f169=change, f170=change_percent
	url := fmt.Sprintf("%s?secid=%s&fields=f43,f44,f45,f46,f47,f48,f51,f52,f57,f58,f60,f169,f170",
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

	// ETF prices are returned in 厘 (1/1000 yuan), stocks in 分 (1/100 yuan)
	divisor := 100.0
	if e.isETF(symbol) {
		divisor = 1000.0
	}

	return &core.Quote{
		Symbol:        symbol,
		Market:        core.MarketCNA,
		Price:         d.F43 / divisor,
		Open:          d.F46 / divisor,
		High:          d.F51 / divisor,
		Low:           d.F52 / divisor,
		PrevClose:     d.F60 / divisor,
		Change:        d.F169 / divisor,
		ChangePercent: d.F170,
		Volume:        int64(d.F47),
		Bid:           d.F44 / divisor,
		Ask:           d.F45 / divisor,
		Time:          time.Now(),
		Source:        "eastmoney",
	}, nil
}

// fetchFundQuote fetches fund NAV data from Eastmoney
func (e *Eastmoney) fetchFundQuote(symbol string) (*core.Quote, error) {
	quote, err := e.fetchFundQuoteFromEastmoney(symbol)
	if err != nil {
		// Try Lixinger fallback if available
		if e.lixingerFallback != nil && e.lixingerFallback.HasAPIKey() {
			log.Printf("eastmoney: fetchFundQuote failed for %s, trying lixinger fallback: %v", symbol, err)
			return e.lixingerFallback.FetchFundQuote(symbol)
		}
		return nil, err
	}
	return quote, nil
}

// fetchFundQuoteFromEastmoney fetches fund NAV data from Eastmoney API
func (e *Eastmoney) fetchFundQuoteFromEastmoney(symbol string) (*core.Quote, error) {
	code, _ := e.parseSymbol(symbol)
	url := fmt.Sprintf("%s/%s.js", fundURL, code)

	resp, err := e.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching fund quote: %w", err)
	}
	defer resp.Body.Close()

	// Response is JSONP: jsonpgz({...});
	body := make([]byte, 4096)
	n, err := resp.Body.Read(body)
	if err != nil && n == 0 {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Extract JSON from JSONP
	content := string(body[:n])
	start := strings.Index(content, "(")
	end := strings.LastIndex(content, ")")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("invalid JSONP response")
	}
	jsonStr := content[start+1 : end]

	var fund fundData
	if err := json.Unmarshal([]byte(jsonStr), &fund); err != nil {
		return nil, fmt.Errorf("decoding fund response: %w", err)
	}

	// Parse NAV values
	price, _ := strconv.ParseFloat(fund.Dwjz, 64)
	gsz, _ := strconv.ParseFloat(fund.Gsz, 64)
	changePercent, _ := strconv.ParseFloat(fund.Gszzl, 64)

	// Calculate change from gsz (estimated) vs dwjz (previous NAV)
	change := gsz - price

	// Fetch fund details from Lixinger if available
	var fundInfo *core.FundInfo
	if e.lixingerFallback != nil && e.lixingerFallback.HasAPIKey() {
		fundInfo = e.lixingerFallback.FetchFundInfoPublic(code)
	}

	return &core.Quote{
		Symbol:        symbol,
		Market:        core.MarketCNA,
		Price:         gsz, // Use estimated NAV as current price
		PrevClose:     price,
		Change:        change,
		ChangePercent: changePercent,
		Time:          time.Now(),
		Source:        "eastmoney-fund",
		FundInfo:      fundInfo,
	}, nil
}

// fetchFundHistory fetches historical NAV data for funds
func (e *Eastmoney) fetchFundHistory(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	data, err := e.fetchFundHistoryFromEastmoney(symbol, start, end)
	if err != nil {
		// Try Lixinger fallback if available
		if e.lixingerFallback != nil && e.lixingerFallback.HasAPIKey() {
			log.Printf("eastmoney: fetchFundHistory failed for %s, trying lixinger fallback: %v", symbol, err)
			return e.lixingerFallback.FetchFundHistory(symbol, start, end)
		}
		return nil, err
	}
	return data, nil
}

// fetchFundHistoryFromEastmoney fetches historical NAV data for funds from Eastmoney API
func (e *Eastmoney) fetchFundHistoryFromEastmoney(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	code, _ := e.parseSymbol(symbol)

	// Calculate page size based on date range
	days := int(end.Sub(start).Hours()/24) + 30 // Add buffer for non-trading days
	if days < 30 {
		days = 30
	}
	if days > 365 {
		days = 365
	}

	url := fmt.Sprintf("%s?fundCode=%s&pageIndex=1&pageSize=%d", fundHistoryURL, code, days)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	// Required header for this API
	req.Header.Set("Referer", "https://fundf10.eastmoney.com/")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching fund history: %w", err)
	}
	defer resp.Body.Close()

	var result fundHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Data == nil || len(result.Data.LSJZList) == 0 {
		return nil, fmt.Errorf("no history for fund: %s", symbol)
	}

	data := make([]core.OHLCV, 0, len(result.Data.LSJZList))
	for _, item := range result.Data.LSJZList {
		t, err := time.Parse("2006-01-02", item.FSRQ)
		if err != nil {
			continue
		}

		// Skip dates outside range
		if t.Before(start) || t.After(end) {
			continue
		}

		nav, _ := strconv.ParseFloat(item.DWJZ, 64)

		// For funds, use NAV as all OHLC values (funds don't have intraday fluctuation)
		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: "1d",
			Open:     nav,
			High:     nav,
			Low:      nav,
			Close:    nav,
			Volume:   0,
			Time:     t,
		})
	}

	// Reverse to chronological order (API returns newest first)
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}

	return data, nil
}

// FetchHistory fetches historical OHLCV data
func (e *Eastmoney) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	// Funds use different API for historical NAV
	if e.isFund(symbol) {
		return e.fetchFundHistory(symbol, start, end)
	}

	data, err := e.fetchStockHistory(symbol, start, end, interval)
	if err != nil {
		// Try Lixinger fallback if available
		if e.lixingerFallback != nil && e.lixingerFallback.HasAPIKey() {
			log.Printf("eastmoney: FetchHistory failed for %s, trying lixinger fallback: %v", symbol, err)
			return e.lixingerFallback.FetchHistory(symbol, start, end, interval)
		}
		return nil, err
	}
	return data, nil
}

// fetchStockHistory fetches stock history from Eastmoney API
func (e *Eastmoney) fetchStockHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
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
	F43  float64 `json:"f43"`  // Current price (cents)
	F44  float64 `json:"f44"`  // Bid
	F45  float64 `json:"f45"`  // Ask
	F46  float64 `json:"f46"`  // Open (cents)
	F47  float64 `json:"f47"`  // Volume
	F48  float64 `json:"f48"`  // Amount
	F51  float64 `json:"f51"`  // High (cents)
	F52  float64 `json:"f52"`  // Low (cents)
	F57  string  `json:"f57"`  // Code
	F58  string  `json:"f58"`  // Name
	F60  float64 `json:"f60"`  // Prev close (cents)
	F169 float64 `json:"f169"` // Change (cents)
	F170 float64 `json:"f170"` // Change percent
}

type historyResponse struct {
	Data *historyData `json:"data"`
}

type historyData struct {
	Code   string   `json:"code"`
	Name   string   `json:"name"`
	Klines []string `json:"klines"`
}

// Fund response data
type fundData struct {
	Fundcode string `json:"fundcode"` // Fund code
	Name     string `json:"name"`     // Fund name
	Jzrq     string `json:"jzrq"`     // NAV date (净值日期)
	Dwjz     string `json:"dwjz"`     // Unit NAV (单位净值)
	Gsz      string `json:"gsz"`      // Estimated NAV (估算净值)
	Gszzl    string `json:"gszzl"`    // Estimated change % (估算涨跌幅)
	Gztime   string `json:"gztime"`   // Estimate time (估算时间)
}

// Fund history response
type fundHistoryResponse struct {
	Data *struct {
		LSJZList []struct {
			FSRQ  string `json:"FSRQ"`  // Date (净值日期)
			DWJZ  string `json:"DWJZ"`  // Unit NAV (单位净值)
			LJJZ  string `json:"LJJZ"`  // Accumulated NAV (累计净值)
			JZZZL string `json:"JZZZL"` // Change percent (净值增长率)
		} `json:"LSJZList"`
	} `json:"Data"`
}

