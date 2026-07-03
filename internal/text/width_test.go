package text

import "testing"

// Context Checkpoint: done_criteria → test mapping
// functional[0] "text.DisplayWidth 表驱动全过：\"\"=0, AAPL=4, 贵州茅台=8, 茅台A=5, 0700.HK=7" → TestDisplayWidth
// functional[0] "text.PadRight ASCII/CJK：PadRight(\"AAPL\",6), PadRight(\"茅台\",6)"          → TestPadRight
// functional[0] "text.PadRight 溢出：PadRight(\"AAPL\",3) 原样返回"                            → TestPadRight

func TestDisplayWidth(t *testing.T) {
	cases := map[string]int{
		"":        0,
		"AAPL":    4,
		"贵州茅台":    8,
		"茅台A":     5,
		"0700.HK": 7,
	}
	for s, want := range cases {
		if got := DisplayWidth(s); got != want {
			t.Errorf("DisplayWidth(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestPadRight(t *testing.T) {
	if got := PadRight("AAPL", 6); got != "AAPL  " {
		t.Errorf("PadRight(AAPL,6) = %q", got)
	}
	if got := PadRight("茅台", 6); got != "茅台  " {
		t.Errorf("PadRight(茅台,6) = %q", got)
	}
	if got := PadRight("AAPL", 3); got != "AAPL" {
		t.Errorf("PadRight overflow = %q", got)
	}
}
