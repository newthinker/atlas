package coingecko

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/collector/crypto"
	"github.com/newthinker/atlas/internal/core"
)

const (
	baseURL = "https://api.coingecko.com/api/v3"
)

// Symbol to CoinGecko ID mapping
var symbolToIDMap = map[string]string{
	"BTC":   "bitcoin",
	"ETH":   "ethereum",
	"BNB":   "binancecoin",
	"SOL":   "solana",
	"XRP":   "ripple",
	"DOGE":  "dogecoin",
	"ADA":   "cardano",
	"AVAX":  "avalanche-2",
	"DOT":   "polkadot",
	"MATIC": "matic-network",
	"LINK":  "chainlink",
	"UNI":   "uniswap",
	"ATOM":  "cosmos",
	"LTC":   "litecoin",
	"ETC":   "ethereum-classic",
	"XLM":   "stellar",
	"ALGO":  "algorand",
	"NEAR":  "near",
	"FTM":   "fantom",
	"SAND":  "the-sandbox",
	"MANA":  "decentraland",
	"AAVE":  "aave",
	"CRV":   "curve-dao-token",
	"APE":   "apecoin",
	"LDO":   "lido-dao",
	"ARB":   "arbitrum",
	"OP":    "optimism",
}

// CoinGecko implements the crypto Provider interface
type CoinGecko struct {
	client  *http.Client
	baseURL string
	apiKey  string
}

// New creates a new CoinGecko provider
func New(apiKey string) *CoinGecko {
	return &CoinGecko{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

// NewWithBaseURL creates a CoinGecko provider with custom base URL (for testing)
func NewWithBaseURL(apiKey, url string) *CoinGecko {
	c := New(apiKey)
	c.baseURL = url
	return c
}

func (c *CoinGecko) Name() string {
	return "coingecko"
}

// symbolToID converts trading pair to CoinGecko coin ID
func (c *CoinGecko) symbolToID(symbol string) string {
	base, _ := crypto.ParseSymbol(symbol)
	if id, ok := symbolToIDMap[base]; ok {
		return id
	}
	return strings.ToLower(base)
}

// symbolToVsCurrency extracts the quote currency for CoinGecko API
func (c *CoinGecko) symbolToVsCurrency(symbol string) string {
	_, quote := crypto.ParseSymbol(symbol)
	switch quote {
	case "USDT", "USDC", "BUSD", "USD":
		return "usd"
	case "BTC":
		return "btc"
	case "ETH":
		return "eth"
	default:
		return "usd"
	}
}

// FetchQuote fetches real-time quote from CoinGecko
func (c *CoinGecko) FetchQuote(symbol string) (*core.Quote, error) {
	coinID := c.symbolToID(symbol)
	vsCurrency := c.symbolToVsCurrency(symbol)

	url := fmt.Sprintf("%s/simple/price?ids=%s&vs_currencies=%s&include_24hr_vol=true&include_24hr_change=true&include_last_updated_at=true",
		c.baseURL, coinID, vsCurrency)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("x-cg-demo-api-key", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching quote: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	coinData, ok := result[coinID]
	if !ok {
		return nil, fmt.Errorf("no data for coin: %s", coinID)
	}

	price := coinData[vsCurrency]
	volume := coinData[vsCurrency+"_24h_vol"]
	changePercent := coinData[vsCurrency+"_24h_change"]
	lastUpdated := coinData["last_updated_at"]

	return &core.Quote{
		Symbol:        symbol,
		Market:        core.MarketCrypto,
		Price:         price,
		Volume:        int64(volume),
		ChangePercent: changePercent,
		Time:          time.Unix(int64(lastUpdated), 0),
		Source:        "coingecko",
	}, nil
}

// FetchHistory fetches historical OHLCV data from CoinGecko
func (c *CoinGecko) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	coinID := c.symbolToID(symbol)
	vsCurrency := c.symbolToVsCurrency(symbol)

	// CoinGecko uses days parameter
	days := int(end.Sub(start).Hours() / 24)
	if days < 1 {
		days = 1
	}
	if days > 365 {
		days = 365
	}

	url := fmt.Sprintf("%s/coins/%s/ohlc?vs_currency=%s&days=%d",
		c.baseURL, coinID, vsCurrency, days)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("x-cg-demo-api-key", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// CoinGecko returns [[timestamp, open, high, low, close], ...]
	var ohlcData [][]float64
	if err := json.NewDecoder(resp.Body).Decode(&ohlcData); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	data := make([]core.OHLCV, 0, len(ohlcData))
	for _, ohlc := range ohlcData {
		if len(ohlc) < 5 {
			continue
		}

		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     ohlc[1],
			High:     ohlc[2],
			Low:      ohlc[3],
			Close:    ohlc[4],
			Time:     time.UnixMilli(int64(ohlc[0])),
		})
	}

	return data, nil
}
