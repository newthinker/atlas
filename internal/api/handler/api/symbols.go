// internal/api/handler/api/symbols.go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/app"
)

// SymbolSearchResult represents a single search result
type SymbolSearchResult struct {
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
	Market string `json:"market"`
	Type   string `json:"type"`
}

// SymbolsHandler handles symbol search API requests
type SymbolsHandler struct {
	httpClient *http.Client
}

// NewSymbolsHandler creates a new symbols handler
func NewSymbolsHandler() *SymbolsHandler {
	return &SymbolsHandler{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Search handles GET /api/v1/symbols/search?q=<query>
func (h *SymbolsHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" || len(query) < 2 {
		response.JSON(w, http.StatusOK, map[string]any{
			"results": []SymbolSearchResult{},
		})
		return
	}

	var results []SymbolSearchResult

	// Determine which API to search based on query pattern
	if startsWithDigit(query) {
		// Search Eastmoney for A-shares
		results = h.searchEastmoney(query)
	} else {
		// Search Yahoo for US/HK stocks
		results = h.searchYahoo(query)
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"results": results,
	})
}

// startsWithDigit checks if the query starts with a digit (A-share pattern)
func startsWithDigit(s string) bool {
	if len(s) == 0 {
		return false
	}
	return s[0] >= '0' && s[0] <= '9'
}

// searchYahoo searches Yahoo Finance for symbols
func (h *SymbolsHandler) searchYahoo(query string) []SymbolSearchResult {
	// Yahoo Finance autocomplete API
	apiURL := fmt.Sprintf("https://query1.finance.yahoo.com/v1/finance/search?q=%s&quotesCount=10&newsCount=0",
		url.QueryEscape(query))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var yahooResp struct {
		Quotes []struct {
			Symbol    string `json:"symbol"`
			ShortName string `json:"shortname"`
			LongName  string `json:"longname"`
			Exchange  string `json:"exchange"`
			QuoteType string `json:"quoteType"`
		} `json:"quotes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&yahooResp); err != nil {
		return nil
	}

	var results []SymbolSearchResult
	for _, q := range yahooResp.Quotes {
		name := q.LongName
		if name == "" {
			name = q.ShortName
		}

		// Detect market and type
		market := detectMarketFromExchange(q.Exchange, q.Symbol)
		assetType := detectTypeFromQuoteType(q.QuoteType)

		results = append(results, SymbolSearchResult{
			Symbol: q.Symbol,
			Name:   name,
			Market: market,
			Type:   assetType,
		})
	}

	return results
}

// searchEastmoney searches Eastmoney for A-share symbols
func (h *SymbolsHandler) searchEastmoney(query string) []SymbolSearchResult {
	// Eastmoney search API
	apiURL := fmt.Sprintf("https://searchapi.eastmoney.com/api/suggest/get?input=%s&type=14&token=D43BF722C8E33BDC906FB84D85E326E8&count=10",
		url.QueryEscape(query))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://www.eastmoney.com/")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var emResp struct {
		QuotationCodeTable struct {
			Data []struct {
				Code         string `json:"Code"`
				Name         string `json:"Name"`
				MktNum       string `json:"MktNum"`
				SecurityType string `json:"SecurityType"`
			} `json:"Data"`
		} `json:"QuotationCodeTable"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&emResp); err != nil {
		return nil
	}

	var results []SymbolSearchResult
	for _, item := range emResp.QuotationCodeTable.Data {
		// Convert Eastmoney market code to symbol suffix
		symbol := convertEastmoneySymbol(item.Code, item.MktNum)
		market := app.MarketAShare
		assetType := detectEastmoneyType(item.SecurityType)

		results = append(results, SymbolSearchResult{
			Symbol: symbol,
			Name:   item.Name,
			Market: market,
			Type:   assetType,
		})
	}

	return results
}

// detectMarketFromExchange detects market from Yahoo exchange code
func detectMarketFromExchange(exchange, symbol string) string {
	exchange = strings.ToUpper(exchange)
	switch {
	case exchange == "HKG" || strings.HasSuffix(symbol, ".HK"):
		return app.MarketHShare
	case exchange == "SHH" || exchange == "SHZ" || strings.HasSuffix(symbol, ".SS") || strings.HasSuffix(symbol, ".SZ"):
		return app.MarketAShare
	case strings.Contains(symbol, "-USD") || strings.Contains(symbol, "-USDT"):
		return app.MarketCrypto
	default:
		return app.MarketUS
	}
}

// detectTypeFromQuoteType detects asset type from Yahoo quote type
func detectTypeFromQuoteType(quoteType string) string {
	switch strings.ToUpper(quoteType) {
	case "EQUITY":
		return app.TypeStock
	case "ETF":
		return app.TypeETF
	case "MUTUALFUND":
		return app.TypeFund
	case "CRYPTOCURRENCY":
		return app.TypeCrypto
	case "FUTURE":
		return app.TypeFuture
	case "OPTION":
		return app.TypeOption
	default:
		return app.TypeStock
	}
}

// convertEastmoneySymbol converts Eastmoney code to standard symbol format
func convertEastmoneySymbol(code, mktNum string) string {
	switch mktNum {
	case "0": // Shenzhen
		return code + ".SZ"
	case "1": // Shanghai
		return code + ".SH"
	default:
		return code + ".SH"
	}
}

// detectEastmoneyType detects asset type from Eastmoney security type
func detectEastmoneyType(securityType string) string {
	switch securityType {
	case "1": // Stock
		return app.TypeStock
	case "2": // Index
		return app.TypeStock
	case "3": // Fund
		return app.TypeFund
	case "4": // Bond
		return app.TypeBond
	default:
		return app.TypeStock
	}
}
