package lixinger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
)

const baseURL = "https://open.lixinger.com/api"

// Lixinger implements FundamentalCollector for Lixinger API
type Lixinger struct {
	apiKey string
	client *http.Client
}

// New creates a new Lixinger collector
func New(apiKey string) *Lixinger {
	return &Lixinger{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (l *Lixinger) Name() string { return "lixinger" }

func (l *Lixinger) SupportedMarkets() []core.Market {
	return []core.Market{core.MarketCNA}
}

func (l *Lixinger) Init(cfg collector.Config) error {
	if cfg.APIKey != "" {
		l.apiKey = cfg.APIKey
	}
	if l.apiKey == "" {
		return fmt.Errorf("lixinger: api_key is required")
	}
	return nil
}

func (l *Lixinger) Start(ctx context.Context) error { return nil }
func (l *Lixinger) Stop() error                     { return nil }

// FetchFundamental fetches latest fundamental data for a stock
func (l *Lixinger) FetchFundamental(symbol string) (*core.Fundamental, error) {
	url := fmt.Sprintf("%s/cn/company/fundamental/non_financial", baseURL)

	payload := map[string]any{
		"token":      l.apiKey,
		"stockCodes": []string{symbol},
		"metrics":    []string{"pe_ttm", "pb", "ps_ttm", "roe_ttm", "dividend_yield_ratio", "market_value"},
	}

	resp, err := l.postJSON(url, payload)
	if err != nil {
		return nil, fmt.Errorf("lixinger: fetch failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("lixinger: no data for symbol %s", symbol)
	}

	item := resp.Data[0]
	return &core.Fundamental{
		Symbol:        symbol,
		Market:        core.MarketCNA,
		Date:          time.Now(),
		PE:            item.PETTM,
		PB:            item.PB,
		PS:            item.PSTTM,
		ROE:           item.ROETTM,
		DividendYield: item.DividendYieldRatio,
		MarketCap:     item.MarketValue,
		Source:        "lixinger",
	}, nil
}

// FetchFundamentalHistory fetches historical fundamental data
func (l *Lixinger) FetchFundamentalHistory(symbol string, start, end time.Time) ([]core.Fundamental, error) {
	f, err := l.FetchFundamental(symbol)
	if err != nil {
		return nil, err
	}
	return []core.Fundamental{*f}, nil
}

type lixingerResponse struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    []lixingerMetric `json:"data"`
}

type lixingerMetric struct {
	StockCode          string  `json:"stockCode"`
	PETTM              float64 `json:"pe_ttm"`
	PB                 float64 `json:"pb"`
	PSTTM              float64 `json:"ps_ttm"`
	ROETTM             float64 `json:"roe_ttm"`
	DividendYieldRatio float64 `json:"dividend_yield_ratio"`
	MarketValue        float64 `json:"market_value"`
}

func (l *Lixinger) postJSON(url string, payload any) (*lixingerResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result lixingerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("API error: %s", result.Message)
	}

	return &result, nil
}
