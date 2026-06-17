package telegram

import "testing"

// Context Checkpoint: done_criteria → test mapping
// functional[0] "displayWidth 表驱动全过：\"\"=0, AAPL=4, 贵州茅台=8, 茅台A=5, 0700.HK=7" → TestDisplayWidth
// functional[1] "padRight ASCII：padRight(\"AAPL\",6)==\"AAPL  \"（补 2 空格）"                → TestPadRight
// functional[2] "padRight CJK：padRight(\"茅台\",6)==\"茅台  \"（茅台显示宽 4，补 2 空格）"    → TestPadRight
// boundary[0]   "padRight 溢出：padRight(\"AAPL\",3)==\"AAPL\"（已 >= 目标宽度，原样返回）"   → TestPadRight
// boundary[1]   "displayWidth(\"\")==0"                                                        → TestDisplayWidth

func TestDisplayWidth(t *testing.T) {
	cases := map[string]int{"": 0, "AAPL": 4, "贵州茅台": 8, "茅台A": 5, "0700.HK": 7}
	for s, want := range cases {
		if got := displayWidth(s); got != want {
			t.Errorf("displayWidth(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestPadRight(t *testing.T) {
	// ASCII: pad with spaces to target width
	if got := padRight("AAPL", 6); got != "AAPL  " {
		t.Errorf("padRight ascii = %q", got)
	}
	// CJK: 茅台 is width 4, pad to 6 -> 2 trailing spaces
	if got := padRight("茅台", 6); got != "茅台  " {
		t.Errorf("padRight cjk = %q", got)
	}
	// already >= width: unchanged
	if got := padRight("AAPL", 3); got != "AAPL" {
		t.Errorf("padRight overflow = %q", got)
	}
}
