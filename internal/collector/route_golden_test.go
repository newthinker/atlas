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

// TestHKSuffixBeatsCryptoPrefix 记录一处**刻意的行为统一**：旧实现里
// "BTC.HK" 的路由与市场自相矛盾（SelectExternalForSymbol → crypto，
// MarketForSymbol → HK）。统一到一张表后取 '.HK' 优先，两者一致。
// 该形态在 watchlist 中不存在，此测试只为把决定钉死，防止后来者反复横跳。
//
// 同时锁住具体度优先的两条语义（done_criteria functional[2]）：更具体的
// pattern 胜过更宽泛的，且排序结果与书写/注册顺序无关。
func TestHKSuffixBeatsCryptoPrefix(t *testing.T) {
	reg := newRegistryWith("yahoo", "eastmoney", "crypto")
	if got := SelectExternalForSymbol(reg, "BTC.HK"); got.Name() != "yahoo" {
		t.Errorf("SelectExternalForSymbol = %q, want yahoo", got.Name())
	}
	if m := MarketForSymbol("BTC.HK"); m != core.MarketHK {
		t.Errorf("MarketForSymbol = %q, want %q", m, core.MarketHK)
	}

	// 内置表已按具体度降序排好——兜底 '*' 必然排在最后，'^HSI' 必然在 '^*' 之前。
	for i := 1; i < len(routes); i++ {
		if specificity(routes[i-1].Pattern) < specificity(routes[i].Pattern) {
			t.Fatalf("routes 未按具体度降序：%q(%d) 排在 %q(%d) 之前",
				routes[i-1].Pattern, specificity(routes[i-1].Pattern),
				routes[i].Pattern, specificity(routes[i].Pattern))
		}
	}

	// 与注册顺序无关：把「宽泛在前、具体在后」的表反过来写，查表结果不变。
	specificFirst := sortRoutes([]Route{
		{"^HSI", "yahoo", core.MarketHK},
		{"^*", "yahoo", core.MarketUS},
	})
	broadFirst := sortRoutes([]Route{
		{"^*", "yahoo", core.MarketUS},
		{"^HSI", "yahoo", core.MarketHK},
	})
	if specificFirst[0].Pattern != "^HSI" || broadFirst[0].Pattern != "^HSI" {
		t.Errorf("具体度优先应与书写顺序无关，got %q / %q",
			specificFirst[0].Pattern, broadFirst[0].Pattern)
	}
}

// TestRouteMissTableFallsBackConservatively 覆盖「表被改坏」这条防御路径：
// 内置表末尾有 '*' 兜底，故 lookupRoute 实际总能命中；但它返回 ok 就是为了
// 有人误删兜底行时保守回退而非 panic。抽掉兜底行验证这层承诺仍然成立。
func TestRouteMissTableFallsBackConservatively(t *testing.T) {
	saved := routes
	defer func() { routes = saved }()
	routes = sortRoutes([]Route{{"*.SH", "eastmoney", core.MarketCNA}}) // 无 '*' 兜底

	if _, ok := lookupRoute("AAPL"); ok {
		t.Error("无兜底行时 lookupRoute 应返回 ok=false")
	}
	if m := MarketForSymbol("AAPL"); m != core.MarketUS {
		t.Errorf("查表落空应保守回退 US, got %q", m)
	}
	if n := routeCollector("AAPL"); n != "yahoo" {
		t.Errorf("查表落空应保守回退 yahoo, got %q", n)
	}
	if _, ok := KnownIndexMarket("AAPL"); ok {
		t.Error("查表落空的符号不得报告为已登记指数")
	}
}

// coveringQlib 是 Covers() 恒真的 qlib 桩。
type coveringQlib struct{ fakeCollector }

func (c *coveringQlib) Covers(symbol string) bool { return true }
