// internal/api/handler/api/symbol_detail.go
package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/collector"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/indicator"
)

// SymbolDetailHandler handles symbol detail API requests
type SymbolDetailHandler struct {
	collectors map[string]collector.Collector
}

// NewSymbolDetailHandler creates a new symbol detail handler
func NewSymbolDetailHandler(collectors map[string]collector.Collector) *SymbolDetailHandler {
	return &SymbolDetailHandler{
		collectors: collectors,
	}
}

// GetQuote handles GET /api/v1/symbols/{symbol}/quote
func (h *SymbolDetailHandler) GetQuote(w http.ResponseWriter, r *http.Request, symbol string) {
	col := h.selectCollector(symbol)
	if col == nil {
		response.Error(w, http.StatusServiceUnavailable, &core.Error{Code: "COLLECTOR_UNAVAILABLE", Message: "no collector available for this symbol"})
		return
	}

	quote, err := col.FetchQuote(symbol)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, core.WrapError(core.ErrCollectorFailed, err))
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"quote": quote,
	})
}

// GetHistory handles GET /api/v1/symbols/{symbol}/history
func (h *SymbolDetailHandler) GetHistory(w http.ResponseWriter, r *http.Request, symbol string) {
	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "3M"
	}

	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "1d"
	}

	// Calculate date range
	end := time.Now()
	var start time.Time
	switch rangeParam {
	case "1M":
		start = end.AddDate(0, -1, 0)
	case "3M":
		start = end.AddDate(0, -3, 0)
	case "6M":
		start = end.AddDate(0, -6, 0)
	case "1Y":
		start = end.AddDate(-1, 0, 0)
	default:
		start = end.AddDate(0, -3, 0)
	}

	col := h.selectCollector(symbol)
	if col == nil {
		response.Error(w, http.StatusServiceUnavailable, &core.Error{Code: "COLLECTOR_UNAVAILABLE", Message: "no collector available for this symbol"})
		return
	}

	history, err := col.FetchHistory(symbol, start, end, interval)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, core.WrapError(core.ErrCollectorFailed, err))
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"symbol":   symbol,
		"range":    rangeParam,
		"interval": interval,
		"data":     history,
	})
}

// GetIndicators handles GET /api/v1/symbols/{symbol}/indicators
func (h *SymbolDetailHandler) GetIndicators(w http.ResponseWriter, r *http.Request, symbol string) {
	strategiesParam := r.URL.Query().Get("strategies")
	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "3M"
	}

	// Parse strategies
	var strategies []string
	if strategiesParam != "" {
		strategies = strings.Split(strategiesParam, ",")
	}

	// Fetch historical data for calculations
	end := time.Now()
	var start time.Time
	switch rangeParam {
	case "1M":
		start = end.AddDate(0, -1, 0)
	case "3M":
		start = end.AddDate(0, -3, 0)
	case "6M":
		start = end.AddDate(0, -6, 0)
	case "1Y":
		start = end.AddDate(-1, 0, 0)
	default:
		start = end.AddDate(0, -3, 0)
	}

	col := h.selectCollector(symbol)
	if col == nil {
		response.Error(w, http.StatusServiceUnavailable, &core.Error{Code: "COLLECTOR_UNAVAILABLE", Message: "no collector available for this symbol"})
		return
	}

	history, err := col.FetchHistory(symbol, start, end, "1d")
	if err != nil {
		response.Error(w, http.StatusInternalServerError, core.WrapError(core.ErrCollectorFailed, err))
		return
	}

	// Calculate indicators for each strategy
	results := make(map[string]any)

	for _, strategy := range strategies {
		switch strategy {
		case "ma_crossover":
			results["ma_crossover"] = h.calculateMAIndicators(history)
		case "pe_band":
			results["pe_band"] = h.calculatePEBands(history)
		case "dividend_yield":
			results["dividend_yield"] = h.calculateDividendYield(history)
		}
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"symbol":     symbol,
		"strategies": strategies,
		"indicators": results,
	})
}

// selectCollector chooses the appropriate collector based on symbol
func (h *SymbolDetailHandler) selectCollector(symbol string) collector.Collector {
	upperSymbol := strings.ToUpper(symbol)

	// A-share symbols use Eastmoney
	if strings.HasSuffix(upperSymbol, ".SH") || strings.HasSuffix(upperSymbol, ".SZ") {
		if col, ok := h.collectors["eastmoney"]; ok {
			return col
		}
	}

	// Crypto symbols use crypto collector
	// Common crypto symbols: BTC, ETH, SOL, or pairs like BTCUSDT, ETH-USD
	cryptoSymbols := []string{"BTC", "ETH", "SOL", "XRP", "DOGE", "ADA", "DOT", "AVAX", "MATIC", "LINK", "UNI", "ATOM", "LTC"}
	for _, cs := range cryptoSymbols {
		if strings.HasPrefix(upperSymbol, cs) || upperSymbol == cs {
			if col, ok := h.collectors["crypto"]; ok {
				return col
			}
		}
	}
	// Also check for -USD suffix (like ETH-USD)
	if strings.HasSuffix(upperSymbol, "-USD") || strings.HasSuffix(upperSymbol, "USDT") {
		if col, ok := h.collectors["crypto"]; ok {
			return col
		}
	}

	// Default to Yahoo for US/HK stocks
	if col, ok := h.collectors["yahoo"]; ok {
		return col
	}

	// Return any available collector
	for _, col := range h.collectors {
		return col
	}

	return nil
}

// MAIndicatorData holds MA crossover indicator data
type MAIndicatorData struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

// MASignal represents a MA crossover signal
type MASignal struct {
	Time   string `json:"time"`
	Action string `json:"action"`
	Price  float64 `json:"price"`
}

// calculateMAIndicators calculates MA crossover indicators
func (h *SymbolDetailHandler) calculateMAIndicators(history []core.OHLCV) map[string]any {
	if len(history) < 20 {
		return nil
	}

	// Extract closing prices
	prices := make([]float64, len(history))
	for i, bar := range history {
		prices[i] = bar.Close
	}

	// Calculate MAs (fast=10, slow=20)
	fastMA := indicator.SMA(prices, 10)
	slowMA := indicator.SMA(prices, 20)

	// Build response data
	fastData := make([]MAIndicatorData, 0)
	slowData := make([]MAIndicatorData, 0)
	signals := make([]MASignal, 0)

	// Align data - slow MA starts later
	slowOffset := len(fastMA) - len(slowMA)

	for i, val := range fastMA {
		if i >= len(history)-len(fastMA) {
			continue
		}
		idx := len(history) - len(fastMA) + i
		fastData = append(fastData, MAIndicatorData{
			Time:  history[idx].Time.Format("2006-01-02"),
			Value: val,
		})
	}

	for i, val := range slowMA {
		idx := len(history) - len(slowMA) + i
		slowData = append(slowData, MAIndicatorData{
			Time:  history[idx].Time.Format("2006-01-02"),
			Value: val,
		})

		// Detect crossovers
		if i > 0 && slowOffset+i < len(fastMA) && slowOffset+i > 0 {
			currFast := fastMA[slowOffset+i]
			prevFast := fastMA[slowOffset+i-1]
			currSlow := val
			prevSlow := slowMA[i-1]

			// Golden cross
			if prevFast <= prevSlow && currFast > currSlow {
				signals = append(signals, MASignal{
					Time:   history[idx].Time.Format("2006-01-02"),
					Action: "buy",
					Price:  history[idx].Close,
				})
			}
			// Death cross
			if prevFast >= prevSlow && currFast < currSlow {
				signals = append(signals, MASignal{
					Time:   history[idx].Time.Format("2006-01-02"),
					Action: "sell",
					Price:  history[idx].Close,
				})
			}
		}
	}

	return map[string]any{
		"fast_ma": fastData,
		"slow_ma": slowData,
		"signals": signals,
		"params": map[string]int{
			"fast_period": 10,
			"slow_period": 20,
		},
	}
}

// calculatePEBands calculates PE band thresholds
func (h *SymbolDetailHandler) calculatePEBands(history []core.OHLCV) map[string]any {
	if len(history) == 0 {
		return nil
	}

	// Calculate price statistics for bands
	var sum, min, max float64
	min = history[0].Close
	max = history[0].Close

	for _, bar := range history {
		sum += bar.Close
		if bar.Close < min {
			min = bar.Close
		}
		if bar.Close > max {
			max = bar.Close
		}
	}

	avg := sum / float64(len(history))

	// Simple PE-based bands (using price as proxy)
	return map[string]any{
		"upper":  max,
		"middle": avg,
		"lower":  min,
	}
}

// calculateDividendYield calculates dividend yield threshold
func (h *SymbolDetailHandler) calculateDividendYield(history []core.OHLCV) map[string]any {
	if len(history) == 0 {
		return nil
	}

	// Use recent price as reference
	currentPrice := history[len(history)-1].Close

	// Target yield thresholds (example: 3% and 5%)
	return map[string]any{
		"current_price":    currentPrice,
		"yield_3_percent":  currentPrice * 0.97, // Price where yield would be ~3%
		"yield_5_percent":  currentPrice * 0.95, // Price where yield would be ~5%
	}
}
