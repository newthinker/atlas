package collector

import (
	"strings"

	"github.com/newthinker/atlas/internal/core"
)

// KnownIndexMarket reports whether a ^-prefixed symbol is in the phase-1
// index list and its market. The app assembly layer warns on unknown ones.
//
// 「已登记」的口径 = 命中的路由不含通配符：'^HSI' 有精确条目故 known，
// '^N225' 落到 '^*' 兜底故 unknown（设计 §6.5，与旧的 indexMarkets 表等价）。
func KnownIndexMarket(symbol string) (core.Market, bool) {
	var zero core.Market
	r, ok := lookupRoute(strings.ToUpper(symbol))
	if !ok || strings.Contains(r.Pattern, "*") {
		return zero, false
	}
	return r.Market, true
}

// warehouseCoverer is implemented by the qlib warehouse collector.
// Using an interface here avoids a direct import of the qlib package.
type warehouseCoverer interface{ Covers(symbol string) bool }

// SelectForSymbol picks the most appropriate registered collector for a symbol.
//
// Routing rules (in priority order):
//  1. qlib warehouse collector covers the symbol → return qlib
//  2. 其余一律查路由表（route.go），具体度优先
//
// If the preferred collector is not registered it falls back to any available
// collector, returning nil only when the registry is empty.
func SelectForSymbol(reg *Registry, symbol string) Collector {
	if reg == nil {
		return nil
	}
	if c, ok := reg.Get("qlib"); ok {
		if cov, ok2 := c.(warehouseCoverer); ok2 && cov.Covers(symbol) {
			return c
		}
	}
	return SelectExternalForSymbol(reg, symbol)
}

// SelectExternalForSymbol routes to an external (non-qlib) collector.
// It applies the same market-based routing as SelectForSymbol but explicitly
// skips the qlib collector to prevent tail-fill delegation loops.
func SelectExternalForSymbol(reg *Registry, symbol string) Collector {
	if reg == nil {
		return nil
	}

	if c, ok := reg.Get(routeCollector(symbol)); ok {
		return c
	}

	// Default to Yahoo for US/HK stocks.
	if c, ok := reg.Get("yahoo"); ok {
		return c
	}

	// Fallback: return any available external collector, skipping qlib to
	// prevent infinite delegation when qlib is the only registered collector.
	for _, c := range reg.GetAll() {
		if c.Name() == "qlib" {
			continue
		}
		return c
	}
	return nil
}

// MarketForSymbol infers the trading market from a symbol's pattern.
func MarketForSymbol(symbol string) core.Market {
	// 表前置规则（设计 §6.4 第 2 条）：AShareIndexSecIDs 覆盖 930713.CSI 这类
	// 不带 .SH/.SZ 后缀的中证跨市场指数。键集离散，无法通配。
	if IsAShareIndex(symbol) {
		return core.MarketCNA
	}
	if r, ok := lookupRoute(strings.ToUpper(symbol)); ok {
		return r.Market
	}
	return core.MarketUS
}

// routeCollector 返回符号应走的 collector 名（同样先过 IsAShareIndex 前置规则）。
func routeCollector(symbol string) string {
	if IsAShareIndex(symbol) {
		return "eastmoney"
	}
	if r, ok := lookupRoute(strings.ToUpper(symbol)); ok {
		return r.Collector
	}
	return "yahoo"
}
