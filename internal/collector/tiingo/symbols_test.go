package tiingo

import "testing"

// Context Checkpoint: done_criteria → test mapping
// functional[0] AAPL/SPY/QQQ/GOOGL/BRK-B/BRK.B → true          → TestSupported（"美股/ETF"）
// functional[1] toTicker BRK.B→BRK-B、AAPL 不变                 → TestToTicker
// boundary[0]   A 股/中证指数/港股 → false                       → TestSupported（"A 股"/"港股"）
// boundary[1]   ^GSPC/^HSI/GC=F/BTC-USD/BTCUSDT/ETH → false      → TestSupported（"指数"/"期货"/"加密"）
// boundary[2]   空串/小写/6 字母/超长/双字母后缀 → false          → TestSupported（"形态"）
//               + 澄清答复补的字母集合边界 BF.B → true、BRK.D → false
// boundary[3]   P11 外国交易所与未登记指数 → false                → TestSupported（"P11"）
//               + 返工 R1 补的基底数字守卫 7203/7203.A/9988 → false
// non_functional[0] go test/gofmt/vet/覆盖率                       → 包级命令，非单测

func TestSupported(t *testing.T) {
	cases := []struct {
		group string
		sym   string
		want  bool
	}{
		{"美股/ETF", "AAPL", true},
		{"美股/ETF", "SPY", true},
		{"美股/ETF", "QQQ", true},
		{"美股/ETF", "GOOGL", true}, // 5 字母上界
		{"美股/ETF", "BRK-B", true},
		{"美股/ETF", "BRK.B", true},
		{"美股/ETF", "BF.B", true},

		{"A 股", "600036.SH", false},
		{"A 股", "000001.SZ", false},
		{"A 股", "930713.CSI", false}, // 中证指数，IsAShareIndex 前置规则
		{"港股", "0700.HK", false},
		{"港股", "03968.HK", false},

		{"指数", "^GSPC", false},
		{"指数", "^HSI", false},
		{"期货", "GC=F", false},
		{"加密", "BTC-USD", false},
		{"加密", "BTCUSDT", false},
		// ETH 形态上是合法美股代码（3 位大写字母），只能靠路由表 'ETH*' → MarketCrypto
		// 拒绝——这一条守卫的是 Supported 中 MarketForSymbol 那半个合取。
		{"加密", "ETH", false},

		{"形态", "", false},
		{"形态", "aapl", false},
		{"形态", "ABCDEF", false}, // 6 字母
		{"形态", "TOOLONGTICKER1", false},
		{"形态", "BRK.BB", false}, // 后缀只允许单字母
		{"形态", "BRK.D", false},  // 后缀字母白名单 A–C 的上边界（TASK-003 questions[0] 裁决）

		// P11（AD-19）：外国交易所后缀与未登记指数。MarketForSymbol 对它们兜底 US，
		// 故只能由形态正则拒绝。7203.T 在 AD-22 后同时被后缀规则拒（T ∉ A–C），
		// 已不能区分基底规则。
		{"P11", "SAP.DE", false},
		{"P11", "HSBA.L", false},
		{"P11", "RY.TO", false},
		{"P11", "7203.T", false},
		{"P11", "000300.SS", false},
		{"P11", "931151.CSI", false},
		// 基底排除数字的独立守卫（返工 R1）：无后缀或合法后缀，只能由基底 [A-Z] 拒。
		{"P11", "7203", false},
		{"P11", "7203.A", false},
		{"P11", "9988", false},
	}
	for _, c := range cases {
		if got := Supported(c.sym); got != c.want {
			t.Errorf("[%s] Supported(%q) = %v, want %v", c.group, c.sym, got, c.want)
		}
	}
}

func TestToTicker(t *testing.T) {
	cases := map[string]string{
		"BRK.B": "BRK-B",
		"AAPL":  "AAPL",
	}
	for in, want := range cases {
		if got := toTicker(in); got != want {
			t.Errorf("toTicker(%q) = %q, want %q", in, got, want)
		}
	}
}
