package okx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/core"
)

const (
	baseURL = "https://www.okx.com"
)

// OKX implements the crypto Provider interface for OKX exchange
type OKX struct {
	client  *http.Client
	baseURL string
}

// New creates a new OKX provider
func New() *OKX {
	return &OKX{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: baseURL,
	}
}

// NewWithBaseURL creates an OKX provider with custom base URL (for testing)
func NewWithBaseURL(url string) *OKX {
	o := New()
	o.baseURL = url
	return o
}

func (o *OKX) Name() string {
	return "okx"
}

// toInstID converts normalized symbol to OKX instrument ID
// BTCUSDT -> BTC-USDT
func (o *OKX) toInstID(symbol string) string {
	base, quote := crypto.ParseSymbol(symbol)
	return base + "-" + quote
}

// FetchQuote fetches real-time quote from OKX
func (o *OKX) FetchQuote(symbol string) (*core.Quote, error) {
	instID := o.toInstID(symbol)
	url := fmt.Sprintf("%s/api/v5/market/ticker?instId=%s", o.baseURL, instID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching quote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result okxTickerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Code != "0" || len(result.Data) == 0 {
		return nil, fmt.Errorf("okx error: %s", result.Msg)
	}

	data := result.Data[0]
	price, _ := strconv.ParseFloat(data.Last, 64)
	open, _ := strconv.ParseFloat(data.Open24h, 64)
	high, _ := strconv.ParseFloat(data.High24h, 64)
	low, _ := strconv.ParseFloat(data.Low24h, 64)
	volume, _ := strconv.ParseFloat(data.Vol24h, 64)
	bidPrice, _ := strconv.ParseFloat(data.BidPx, 64)
	askPrice, _ := strconv.ParseFloat(data.AskPx, 64)
	ts, _ := strconv.ParseInt(data.Ts, 10, 64)

	change := price - open
	changePercent := 0.0
	if open > 0 {
		changePercent = (change / open) * 100
	}

	return &core.Quote{
		Symbol:        symbol,
		Market:        core.MarketCrypto,
		Price:         price,
		Open:          open,
		High:          high,
		Low:           low,
		Change:        change,
		ChangePercent: changePercent,
		Volume:        int64(volume),
		Bid:           bidPrice,
		Ask:           askPrice,
		Time:          time.UnixMilli(ts),
		Source:        "okx",
	}, nil
}

// FetchHistory fetches historical OHLCV data from OKX
func (o *OKX) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	instID := o.toInstID(symbol)
	okxInterval := o.toInterval(interval)

	url := fmt.Sprintf("%s/api/v5/market/candles?instId=%s&bar=%s&before=%d&after=%d&limit=300",
		o.baseURL, instID, okxInterval, start.UnixMilli(), end.UnixMilli())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result okxCandleResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Code != "0" {
		return nil, fmt.Errorf("okx error: %s", result.Msg)
	}

	data := make([]core.OHLCV, 0, len(result.Data))
	// OKX returns newest first, reverse for chronological order
	for i := len(result.Data) - 1; i >= 0; i-- {
		candle := result.Data[i]
		if len(candle) < 6 {
			continue
		}

		ts, _ := strconv.ParseInt(candle[0], 10, 64)
		openPrice, _ := strconv.ParseFloat(candle[1], 64)
		high, _ := strconv.ParseFloat(candle[2], 64)
		low, _ := strconv.ParseFloat(candle[3], 64)
		closePrice, _ := strconv.ParseFloat(candle[4], 64)
		volume, _ := strconv.ParseFloat(candle[5], 64)

		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     openPrice,
			High:     high,
			Low:      low,
			Close:    closePrice,
			Volume:   int64(volume),
			Time:     time.UnixMilli(ts),
		})
	}

	return data, nil
}

func (o *OKX) toInterval(interval string) string {
	switch interval {
	case "1m", "5m", "15m", "30m":
		return interval
	case "1h":
		return "1H"
	case "2h":
		return "2H"
	case "4h":
		return "4H"
	case "1d":
		return "1D"
	case "1w":
		return "1W"
	default:
		return "1D"
	}
}

// OKX API response types
type okxTickerResponse struct {
	Code string      `json:"code"`
	Msg  string      `json:"msg"`
	Data []okxTicker `json:"data"`
}

type okxTicker struct {
	InstId  string `json:"instId"`
	Last    string `json:"last"`
	Open24h string `json:"open24h"`
	High24h string `json:"high24h"`
	Low24h  string `json:"low24h"`
	Vol24h  string `json:"vol24h"`
	BidPx   string `json:"bidPx"`
	AskPx   string `json:"askPx"`
	Ts      string `json:"ts"`
}

type okxCandleResponse struct {
	Code string     `json:"code"`
	Msg  string     `json:"msg"`
	Data [][]string `json:"data"`
}
