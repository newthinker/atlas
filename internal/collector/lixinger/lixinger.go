package lixinger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// HasAPIKey returns true if the collector has a valid API key configured
func (l *Lixinger) HasAPIKey() bool {
	return l.apiKey != ""
}

// toLixingerSymbol converts internal symbol format to Lixinger format
// 600519.SH -> 600519, 000001.SZ -> 000001
func (l *Lixinger) toLixingerSymbol(symbol string) string {
	parts := strings.Split(symbol, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return symbol
}

// FetchQuote fetches real-time quote from Lixinger
func (l *Lixinger) FetchQuote(symbol string) (*core.Quote, error) {
	if l.apiKey == "" {
		return nil, fmt.Errorf("lixinger: api_key is required")
	}

	code := l.toLixingerSymbol(symbol)
	url := fmt.Sprintf("%s/cn/stock/real-time", baseURL)

	payload := map[string]any{
		"token":      l.apiKey,
		"stockCodes": []string{code},
	}

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
		return nil, fmt.Errorf("lixinger: fetch quote failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    []struct {
			StockCode string  `json:"stockCode"`
			Close     float64 `json:"close"`
			Open      float64 `json:"open"`
			High      float64 `json:"high"`
			Low       float64 `json:"low"`
			Volume    float64 `json:"volume"`
			Amount    float64 `json:"amount"`
			PreClose  float64 `json:"preClose"`
			Change    float64 `json:"change"`
			PctChange float64 `json:"pctChange"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("lixinger: decode response failed: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("lixinger: API error: %s", result.Message)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("lixinger: no quote data for %s", symbol)
	}

	d := result.Data[0]
	return &core.Quote{
		Symbol:        symbol,
		Market:        core.MarketCNA,
		Price:         d.Close,
		Open:          d.Open,
		High:          d.High,
		Low:           d.Low,
		PrevClose:     d.PreClose,
		Change:        d.Change,
		ChangePercent: d.PctChange,
		Volume:        int64(d.Volume),
		Time:          time.Now(),
		Source:        "lixinger",
	}, nil
}

// FetchHistory fetches historical OHLCV data from Lixinger
func (l *Lixinger) FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error) {
	if l.apiKey == "" {
		return nil, fmt.Errorf("lixinger: api_key is required")
	}

	code := l.toLixingerSymbol(symbol)
	url := fmt.Sprintf("%s/cn/stock/hq", baseURL)

	payload := map[string]any{
		"token":      l.apiKey,
		"stockCodes": []string{code},
		"startDate":  start.Format("2006-01-02"),
		"endDate":    end.Format("2006-01-02"),
		"metrics":    []string{"open", "high", "low", "close", "volume"},
	}

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
		return nil, fmt.Errorf("lixinger: fetch history failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    []struct {
			Date   string  `json:"date"`
			Open   float64 `json:"open"`
			High   float64 `json:"high"`
			Low    float64 `json:"low"`
			Close  float64 `json:"close"`
			Volume float64 `json:"volume"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("lixinger: decode response failed: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("lixinger: API error: %s", result.Message)
	}

	data := make([]core.OHLCV, 0, len(result.Data))
	for _, item := range result.Data {
		t, err := time.Parse("2006-01-02", item.Date)
		if err != nil {
			continue
		}
		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: interval,
			Open:     item.Open,
			High:     item.High,
			Low:      item.Low,
			Close:    item.Close,
			Volume:   int64(item.Volume),
			Time:     t,
		})
	}

	return data, nil
}

// FetchFundQuote fetches fund NAV and info from Lixinger
func (l *Lixinger) FetchFundQuote(symbol string) (*core.Quote, error) {
	if l.apiKey == "" {
		return nil, fmt.Errorf("lixinger: api_key is required")
	}

	code := l.toLixingerSymbol(symbol)
	url := fmt.Sprintf("%s/cn/fund/nav", baseURL)

	payload := map[string]any{
		"token":     l.apiKey,
		"fundCodes": []string{code},
	}

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
		return nil, fmt.Errorf("lixinger: fetch fund failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    []struct {
			FundCode  string  `json:"fundCode"`
			Nav       float64 `json:"nav"`       // 单位净值
			AccNav    float64 `json:"accNav"`    // 累计净值
			NavChange float64 `json:"navChange"` // 净值涨跌
			NavPct    float64 `json:"navPct"`    // 净值涨跌幅
			Date      string  `json:"date"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("lixinger: decode response failed: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("lixinger: API error: %s", result.Message)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("lixinger: no fund data for %s", symbol)
	}

	d := result.Data[0]

	// Also fetch fund info
	fundInfo := l.fetchFundInfo(code)

	return &core.Quote{
		Symbol:        symbol,
		Market:        core.MarketCNA,
		Price:         d.Nav,
		PrevClose:     d.Nav - d.NavChange,
		Change:        d.NavChange,
		ChangePercent: d.NavPct,
		Time:          time.Now(),
		Source:        "lixinger",
		FundInfo:      fundInfo,
	}, nil
}

// FetchFundInfoPublic fetches detailed fund information from Lixinger (public method)
func (l *Lixinger) FetchFundInfoPublic(code string) *core.FundInfo {
	return l.fetchFundInfo(code)
}

// fetchFundInfo fetches detailed fund information from Lixinger
func (l *Lixinger) fetchFundInfo(code string) *core.FundInfo {
	url := fmt.Sprintf("%s/cn/fund/fundamental", baseURL)

	payload := map[string]any{
		"token":     l.apiKey,
		"fundCodes": []string{code},
		"metrics": []string{
			"name", "manager", "management_company", "inception_date",
			"fund_size", "fund_type", "annualized_return", "max_drawdown",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    []struct {
			FundCode          string  `json:"fundCode"`
			Name              string  `json:"name"`
			Manager           string  `json:"manager"`
			ManagementCompany string  `json:"management_company"`
			InceptionDate     string  `json:"inception_date"`
			FundSize          float64 `json:"fund_size"`
			FundType          string  `json:"fund_type"`
			AnnualizedReturn  float64 `json:"annualized_return"`
			MaxDrawdown       float64 `json:"max_drawdown"`
			LatestNav         float64 `json:"latest_nav"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	if result.Code != 0 || len(result.Data) == 0 {
		return nil
	}

	d := result.Data[0]
	var inceptionDate time.Time
	if d.InceptionDate != "" {
		inceptionDate, _ = time.Parse("2006-01-02", d.InceptionDate)
	}

	return &core.FundInfo{
		Name:              d.Name,
		Manager:           d.Manager,
		ManagementCompany: d.ManagementCompany,
		InceptionDate:     inceptionDate,
		FundSize:          d.FundSize,
		FundType:          d.FundType,
		AnnualizedReturn:  d.AnnualizedReturn,
		MaxDrawdown:       d.MaxDrawdown,
		LatestNAV:         d.LatestNav,
	}
}

// FetchFundHistory fetches historical NAV data for funds from Lixinger
func (l *Lixinger) FetchFundHistory(symbol string, start, end time.Time) ([]core.OHLCV, error) {
	if l.apiKey == "" {
		return nil, fmt.Errorf("lixinger: api_key is required")
	}

	code := l.toLixingerSymbol(symbol)
	url := fmt.Sprintf("%s/cn/fund/nav/history", baseURL)

	payload := map[string]any{
		"token":     l.apiKey,
		"fundCodes": []string{code},
		"startDate": start.Format("2006-01-02"),
		"endDate":   end.Format("2006-01-02"),
	}

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
		return nil, fmt.Errorf("lixinger: fetch fund history failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    []struct {
			Date string  `json:"date"`
			Nav  float64 `json:"nav"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("lixinger: decode response failed: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("lixinger: API error: %s", result.Message)
	}

	data := make([]core.OHLCV, 0, len(result.Data))
	for _, item := range result.Data {
		t, err := time.Parse("2006-01-02", item.Date)
		if err != nil {
			continue
		}
		// For funds, OHLC are all the same (NAV)
		data = append(data, core.OHLCV{
			Symbol:   symbol,
			Interval: "1d",
			Open:     item.Nav,
			High:     item.Nav,
			Low:      item.Nav,
			Close:    item.Nav,
			Volume:   0,
			Time:     t,
		})
	}

	return data, nil
}

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
