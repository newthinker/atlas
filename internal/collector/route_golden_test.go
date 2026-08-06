package collector

import (
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

// Context Checkpoint: done_criteria → test mapping (TASK-012)
// functional[0] "黄金值对新旧实现均全绿"                → TestRouteGoldenValues
// functional[2] "具体度优先，与注册顺序无关"            → TestHKSuffixBeatsCryptoPrefix
// functional[3] "符号大小写不敏感"                      → TestRouteCaseInsensitive
// boundary[0]   "qlib Covers() 为表前置规则"            → TestQlibCoversTakesPrecedenceOverTable
// boundary[1]   "IsAShareIndex() 为表前置规则"          → TestRouteGoldenValues 的 930713.CSI / 930604.CSI 条目
// boundary[2]   "命中但 collector 未注册时回退"         → TestRouteFallsBackWhenCollectorMissing
// error_handling[0] "BTC.HK 自相矛盾被统一"             → TestHKSuffixBeatsCryptoPrefix

// 路由黄金值回归（设计 §7.1）：穷举符号形态，逐一钉死四个公开函数的返回值。
// 本文件**先对旧实现跑绿**，再重写内部实现，必须仍然全绿。
//
// 用例覆盖 A 股、中证跨市场指数、港股、美股、已登记/未登记指数、商品、
// 两种加密后缀、加密前缀裸符号、空串与畸形符号。
func TestRouteGoldenValues(t *testing.T) {
	reg := newRegistryWith("yahoo", "eastmoney", "crypto")

	tests := []struct {
		symbol        string
		wantCollector string
		wantMarket    core.Market
		wantKnownIdx  bool
	}{
		{"600519.SH", "eastmoney", core.MarketCNA, false},
		{"000001.SZ", "eastmoney", core.MarketCNA, false},
		{"930713.CSI", "eastmoney", core.MarketCNA, false}, // 表成员判定，非后缀
		{"930604.CSI", "eastmoney", core.MarketCNA, false},
		{"0700.HK", "yahoo", core.MarketHK, false},
		{"AAPL", "yahoo", core.MarketUS, false},
		{"^GSPC", "yahoo", core.MarketUS, true},
		{"^IXIC", "yahoo", core.MarketUS, true},
		{"^DJI", "yahoo", core.MarketUS, true},
		{"^HSI", "yahoo", core.MarketHK, true},
		{"^HSCE", "yahoo", core.MarketHK, true},
		{"^N225", "yahoo", core.MarketUS, false}, // 未登记指数：兜底 US 且 unknown
		{"CL=F", "yahoo", core.MarketUS, false},
		{"GC=F", "yahoo", core.MarketUS, false},
		{"BTC-USD", "crypto", core.MarketCrypto, false},
		{"ETH-USD", "crypto", core.MarketCrypto, false},
		{"ETHUSDT", "crypto", core.MarketCrypto, false},
		{"BTCUSDT", "crypto", core.MarketCrypto, false},
		{"BTC", "crypto", core.MarketCrypto, false},
		{"SOL", "crypto", core.MarketCrypto, false},
		{"MATIC", "crypto", core.MarketCrypto, false},
		{"", "yahoo", core.MarketUS, false},
		{"!!!", "yahoo", core.MarketUS, false},
		{"...", "yahoo", core.MarketUS, false},
		{"600519", "yahoo", core.MarketUS, false}, // 无后缀：今天不认作 A 股
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			got := SelectExternalForSymbol(reg, tt.symbol)
			if got == nil {
				t.Fatalf("SelectExternalForSymbol = nil, want %q", tt.wantCollector)
			}
			if got.Name() != tt.wantCollector {
				t.Errorf("SelectExternalForSymbol = %q, want %q", got.Name(), tt.wantCollector)
			}
			if m := MarketForSymbol(tt.symbol); m != tt.wantMarket {
				t.Errorf("MarketForSymbol = %q, want %q", m, tt.wantMarket)
			}
			m, known := KnownIndexMarket(tt.symbol)
			if known != tt.wantKnownIdx {
				t.Errorf("KnownIndexMarket known = %v, want %v", known, tt.wantKnownIdx)
			}
			if known && m != tt.wantMarket {
				t.Errorf("KnownIndexMarket market = %q, want %q", m, tt.wantMarket)
			}
		})
	}
}

// 大小写不敏感：四个函数都先 ToUpper。
func TestRouteCaseInsensitive(t *testing.T) {
	reg := newRegistryWith("yahoo", "eastmoney", "crypto")
	for _, pair := range [][2]string{{"600519.sh", "600519.SH"}, {"btc-usd", "BTC-USD"}, {"^gspc", "^GSPC"}} {
		lower, upper := pair[0], pair[1]
		if MarketForSymbol(lower) != MarketForSymbol(upper) {
			t.Errorf("%s vs %s: MarketForSymbol 不一致", lower, upper)
		}
		if SelectExternalForSymbol(reg, lower).Name() != SelectExternalForSymbol(reg, upper).Name() {
			t.Errorf("%s vs %s: SelectExternalForSymbol 不一致", lower, upper)
		}
	}
}

// SelectForSymbol 的 qlib Covers() 前置规则（设计 §6.4 第 1 条）优先级最高。
func TestQlibCoversTakesPrecedenceOverTable(t *testing.T) {
	reg := newRegistryWith("yahoo", "eastmoney", "crypto")
	reg.Register(&coveringQlib{fakeCollector{name: "qlib"}})

	if got := SelectForSymbol(reg, "600519.SH"); got.Name() != "qlib" {
		t.Errorf("qlib 覆盖时应优先于路由表, got %q", got.Name())
	}
	// SelectExternalForSymbol 必须绕开 qlib
	if got := SelectExternalForSymbol(reg, "600519.SH"); got.Name() != "eastmoney" {
		t.Errorf("SelectExternalForSymbol 不得选中 qlib, got %q", got.Name())
	}
}

// 首选 collector 未注册时的回退链：yahoo → 任一非 qlib collector。
func TestRouteFallsBackWhenCollectorMissing(t *testing.T) {
	reg := newRegistryWith("yahoo") // 没有 eastmoney / crypto
	if got := SelectExternalForSymbol(reg, "600519.SH"); got.Name() != "yahoo" {
		t.Errorf("eastmoney 缺席应回退 yahoo, got %q", got.Name())
	}
	if got := SelectExternalForSymbol(reg, "BTC-USD"); got.Name() != "yahoo" {
		t.Errorf("crypto 缺席应回退 yahoo, got %q", got.Name())
	}

	only := newRegistryWith("eastmoney")
	if got := SelectExternalForSymbol(only, "AAPL"); got.Name() != "eastmoney" {
		t.Errorf("yahoo 也缺席时回退任一非 qlib collector, got %q", got.Name())
	}

	empty := NewRegistry()
	if got := SelectExternalForSymbol(empty, "AAPL"); got != nil {
		t.Errorf("空 registry 应返回 nil, got %q", got.Name())
	}
	if got := SelectExternalForSymbol(nil, "AAPL"); got != nil {
		t.Error("nil registry 应返回 nil")
	}
}

// coveringQlib 是 Covers() 恒真的 qlib 桩。
type coveringQlib struct{ fakeCollector }

func (c *coveringQlib) Covers(symbol string) bool { return true }
