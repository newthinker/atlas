package collector

import "testing"

// Context Checkpoint: done_criteria → test mapping
// functional[0] "IsAShareIndex 000300.SH/000001.SH→true, 000001.SZ/600519.SH→false" → TestIsAShareIndex

func TestIsAShareIndex(t *testing.T) {
	cases := []struct {
		symbol string
		want   bool
	}{
		{"000300.SH", true},  // 沪深300
		{"000001.SH", true},  // 上证指数（指数表命中）
		{"000905.SH", true},  // 中证500
		{"399001.SZ", true},  // 深证成指
		{"000001.SZ", false}, // 平安银行（个股）
		{"600519.SH", false}, // 贵州茅台（个股）
	}
	for _, c := range cases {
		if got := IsAShareIndex(c.symbol); got != c.want {
			t.Errorf("IsAShareIndex(%q) = %v, want %v", c.symbol, got, c.want)
		}
	}
}
