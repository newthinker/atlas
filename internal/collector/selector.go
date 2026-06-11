package collector

import (
	"strings"

	"github.com/newthinker/atlas/internal/core"
)

// cryptoTickers lists common crypto base symbols used for routing decisions.
var cryptoTickers = []string{
	"BTC", "ETH", "SOL", "XRP", "DOGE", "ADA",
	"DOT", "AVAX", "MATIC", "LINK", "UNI", "ATOM", "LTC",
}

// indexMarkets pins the market for the phase-1 international index list.
// Symbols with a ^ prefix not present here default to MarketUS; the app
// assembly layer logs a warning for such bindings (design §2.3).
var indexMarkets = map[string]core.Market{
	"^GSPC": core.MarketUS,
	"^IXIC": core.MarketUS,
	"^DJI":  core.MarketUS,
	"^HSI":  core.MarketHK,
}

func isIndexSymbol(upper string) bool     { return strings.HasPrefix(upper, "^") }
func isCommoditySymbol(upper string) bool { return strings.HasSuffix(upper, "=F") }

// KnownIndexMarket reports whether a ^-prefixed symbol is in the phase-1
// index list and its market. The app assembly layer warns on unknown ones.
func KnownIndexMarket(symbol string) (core.Market, bool) {
	m, ok := indexMarkets[strings.ToUpper(symbol)]
	return m, ok
}

// SelectForSymbol picks the most appropriate registered collector for a symbol.
//
// Routing rules:
//   - A-share symbols (.SH/.SZ) -> "eastmoney"
//   - crypto symbols (BTC*, *-USD, *USDT, ...) -> "crypto"
//   - everything else (US/HK stocks) -> "yahoo"
//
// If the preferred collector is not registered it falls back to any available
// collector, returning nil only when the registry is empty.
func SelectForSymbol(reg *Registry, symbol string) Collector {
	if reg == nil {
		return nil
	}

	upper := strings.ToUpper(symbol)

	switch {
	case isAShareSymbol(upper):
		if c, ok := reg.Get("eastmoney"); ok {
			return c
		}
	case isIndexSymbol(upper), isCommoditySymbol(upper):
		if c, ok := reg.Get("yahoo"); ok {
			return c
		}
	case isCryptoSymbol(upper):
		if c, ok := reg.Get("crypto"); ok {
			return c
		}
	}

	// Default to Yahoo for US/HK stocks.
	if c, ok := reg.Get("yahoo"); ok {
		return c
	}

	// Fallback: return any available collector.
	if all := reg.GetAll(); len(all) > 0 {
		return all[0]
	}
	return nil
}

// MarketForSymbol infers the trading market from a symbol's pattern.
func MarketForSymbol(symbol string) core.Market {
	upper := strings.ToUpper(symbol)
	switch {
	case isAShareSymbol(upper):
		return core.MarketCNA
	case isIndexSymbol(upper):
		if m, ok := KnownIndexMarket(symbol); ok {
			return m
		}
		return core.MarketUS
	case isCommoditySymbol(upper):
		return core.MarketUS
	case strings.HasSuffix(upper, ".HK"):
		return core.MarketHK
	case isCryptoSymbol(upper):
		return core.MarketCrypto
	default:
		return core.MarketUS
	}
}

// isAShareSymbol reports whether an upper-cased symbol is a Shanghai or Shenzhen
// A-share listing.
func isAShareSymbol(upper string) bool {
	return strings.HasSuffix(upper, ".SH") || strings.HasSuffix(upper, ".SZ")
}

// isCryptoSymbol reports whether an upper-cased symbol looks like a crypto asset.
func isCryptoSymbol(upper string) bool {
	if strings.HasSuffix(upper, "-USD") || strings.HasSuffix(upper, "USDT") {
		return true
	}
	for _, t := range cryptoTickers {
		if upper == t || strings.HasPrefix(upper, t) {
			return true
		}
	}
	return false
}
