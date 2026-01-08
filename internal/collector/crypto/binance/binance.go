package binance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

const (
	baseURL = "https://api.binance.com"
)

// Binance implements the crypto Provider interface for Binance exchange
type Binance struct {
	client  *http.Client
	baseURL string
}

// New creates a new Binance provider
func New() *Binance {
	return &Binance{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: baseURL,
	}
}

// NewWithBaseURL creates a Binance provider with custom base URL (for testing)
func NewWithBaseURL(url string) *Binance {
	b := New()
	b.baseURL = url
	return b
}

func (b *Binance) Name() string {
	return "binance"
}

// FetchQuote fetches real-time quote from Binance
func (b *Binance) FetchQuote(symbol string) (*core.Quote, error) {
	url := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", b.baseURL, symbol)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching quote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result ticker24hr
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	price, _ := strconv.ParseFloat(result.LastPrice, 64)
	open, _ := strconv.ParseFloat(result.OpenPrice, 64)
	high, _ := strconv.ParseFloat(result.HighPrice, 64)
	low, _ := strconv.ParseFloat(result.LowPrice, 64)
	prevClose, _ := strconv.ParseFloat(result.PrevClosePrice, 64)
	change, _ := strconv.ParseFloat(result.PriceChange, 64)
	changePercent, _ := strconv.ParseFloat(result.PriceChangePercent, 64)
	volume, _ := strconv.ParseFloat(result.Volume, 64)
	bidPrice, _ := strconv.ParseFloat(result.BidPrice, 64)
	askPrice, _ := strconv.ParseFloat(result.AskPrice, 64)

	return &core.Quote{
		Symbol:        symbol,
		Market:        core.MarketCrypto,
		Price:         price,
		Open:          open,
		High:          high,
		Low:           low,
		PrevClose:     prevClose,
		Change:        change,
		ChangePercent: changePercent,
		Volume:        int64(volume),
		Bid:           bidPrice,
		Ask:           askPrice,
		Time:          time.UnixMilli(result.CloseTime),
		Source:        "binance",
	}, nil
}

// FetchHistory fetches historical OHLCV data from Binance
func (b *Binance) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	binanceInterval := b.toInterval(interval)
	url := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&startTime=%d&endTime=%d&limit=1000",
		b.baseURL, symbol, binanceInterval, start.UnixMilli(), end.UnixMilli())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var klines [][]any
	if err := json.NewDecoder(resp.Body).Decode(&klines); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	data := make([]core.OHLCV, 0, len(klines))
	for _, k := range klines {
		if len(k) < 6 {
			continue
		}

		openTime, _ := k[0].(float64)
		openStr, _ := k[1].(string)
		highStr, _ := k[2].(string)
		lowStr, _ := k[3].(string)
		closeStr, _ := k[4].(string)
		volumeStr, _ := k[5].(string)

		open, _ := strconv.ParseFloat(openStr, 64)
		high, _ := strconv.ParseFloat(highStr, 64)
		low, _ := strconv.ParseFloat(lowStr, 64)
		close, _ := strconv.ParseFloat(closeStr, 64)
		volume, _ := strconv.ParseFloat(volumeStr, 64)

		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     open,
			High:     high,
			Low:      low,
			Close:    close,
			Volume:   int64(volume),
			Time:     time.UnixMilli(int64(openTime)),
		})
	}

	return data, nil
}

func (b *Binance) toInterval(interval string) string {
	switch interval {
	case "1m", "5m", "15m", "30m":
		return interval
	case "1h", "2h", "4h":
		return interval
	case "1d":
		return "1d"
	case "1w":
		return "1w"
	default:
		return "1d"
	}
}

// Binance API response types
type ticker24hr struct {
	Symbol             string `json:"symbol"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	LastPrice          string `json:"lastPrice"`
	OpenPrice          string `json:"openPrice"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
	Volume             string `json:"volume"`
	PrevClosePrice     string `json:"prevClosePrice"`
	BidPrice           string `json:"bidPrice"`
	AskPrice           string `json:"askPrice"`
	CloseTime          int64  `json:"closeTime"`
}
