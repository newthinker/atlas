package pricewatch

// Context Checkpoint: done_criteria → test mapping (TASK-001)
// functional[0] Measure drop/rise 的 BTC 例                   → TestMeasure
// functional[1] Tier 边界与 1e-9 容差、NaN/±Inf 返回 0         → TestTier_Boundaries
// functional[3] Window 的 Span/Bar                            → TestWindow
// boundary[0]   step_pct 0 合法、Notify 默认/原样、Type/Window 常量 → TestParseRule_Valid
// error_handling[0] 非法取值返回含规则 id 与字段名的 error      → TestParseRule_Rejects
// functional[2]/boundary[1] 见 state_test.go

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestMeasure(t *testing.T) {
	if got := Measure(DropFromHigh, 50000, 100000, 20000); got != 50 {
		t.Errorf("drop: got %v, want 50", got)
	}
	if got := Measure(RiseFromLow, 40000, 100000, 20000); got != 100 {
		t.Errorf("rise: got %v, want 100", got)
	}
}

func TestTier_Boundaries(t *testing.T) {
	cases := []struct {
		x, th, step float64
		want        int
	}{
		{2.0, 2, 2, 1},
		{1.9999, 2, 2, 0},
		{1.9999999999, 2, 2, 1},
		{3.9999999999, 2, 2, 2},
		{4.0, 2, 2, 2},
		{5.5, 2, 2, 2},
		{5.9, 2, 2, 2},
		{6.1, 2, 2, 3},
		{55, 50, 0, 1},
		{100, 100, 50, 1},
		{150, 100, 50, 2},
		{math.NaN(), 2, 2, 0},
		{math.Inf(1), 2, 2, 0},
		{math.Inf(-1), 2, 2, 0},
	}
	for _, c := range cases {
		if got := Tier(c.x, c.th, c.step); got != c.want {
			t.Errorf("Tier(%v,%v,%v) = %d, want %d", c.x, c.th, c.step, got, c.want)
		}
	}
}

func TestWindow(t *testing.T) {
	cases := []struct {
		w    Window
		span time.Duration
		bar  string
	}{
		{Window24h, 24 * time.Hour, "5m"},
		{Window7d, 168 * time.Hour, "5m"},
		{Window30d, 720 * time.Hour, "1d"},
		{Window4y, 1461 * 24 * time.Hour, "1d"},
		{WindowAll, 0, "1d"},
	}
	for _, c := range cases {
		if got := c.w.Span(); got != c.span {
			t.Errorf("%s.Span() = %v, want %v", c.w, got, c.span)
		}
		if got := c.w.Bar(); got != c.bar {
			t.Errorf("%s.Bar() = %q, want %q", c.w, got, c.bar)
		}
	}
}

var testDefault = NotifyPolicy{RepeatInterval: time.Hour, MaxPerTier: 3}

func validCfg() RuleCfg {
	return RuleCfg{ID: "day_drop", Type: "drop_from_high", Window: "24h", ThresholdPct: 2, StepPct: 2}
}

func TestParseRule_Valid(t *testing.T) {
	// step_pct 0 合法；Notify==nil 取默认
	c := validCfg()
	c.StepPct = 0
	r, err := ParseRule(c, testDefault)
	if err != nil {
		t.Fatalf("step_pct 0: unexpected error %v", err)
	}
	want := Rule{ID: "day_drop", Type: DropFromHigh, Window: Window24h, ThresholdPct: 2, StepPct: 0, Notify: testDefault}
	if r != want {
		t.Errorf("got %+v, want %+v", r, want)
	}

	// Notify 非 nil 原样使用；Type/Window 为对应常量
	own := NotifyPolicy{RepeatInterval: 24 * time.Hour, MaxPerTier: 2}
	c = RuleCfg{ID: "cycle_rise", Type: "rise_from_low", Window: "4y", ThresholdPct: 100, StepPct: 50, Notify: &own}
	r, err = ParseRule(c, testDefault)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	want = Rule{ID: "cycle_rise", Type: RiseFromLow, Window: Window4y, ThresholdPct: 100, StepPct: 50, Notify: own}
	if r != want {
		t.Errorf("got %+v, want %+v", r, want)
	}

	for _, w := range []Window{Window24h, Window7d, Window30d, Window4y, WindowAll} {
		c := validCfg()
		c.Window = string(w)
		if r, err := ParseRule(c, testDefault); err != nil || r.Window != w {
			t.Errorf("window %s: got %v, %v", w, r.Window, err)
		}
	}
}

func TestParseRule_Rejects(t *testing.T) {
	cases := []struct {
		name  string
		field string
		mod   func(*RuleCfg)
	}{
		{"type foo", "type", func(c *RuleCfg) { c.Type = "foo" }},
		{"window 2d", "window", func(c *RuleCfg) { c.Window = "2d" }},
		{"threshold 0", "threshold_pct", func(c *RuleCfg) { c.ThresholdPct = 0 }},
		{"threshold -1", "threshold_pct", func(c *RuleCfg) { c.ThresholdPct = -1 }},
		{"step -1", "step_pct", func(c *RuleCfg) { c.StepPct = -1 }},
		{"max_per_tier 0", "max_per_tier", func(c *RuleCfg) {
			c.Notify = &NotifyPolicy{RepeatInterval: time.Hour, MaxPerTier: 0}
		}},
		{"repeat_interval 0", "repeat_interval", func(c *RuleCfg) {
			c.Notify = &NotifyPolicy{RepeatInterval: 0, MaxPerTier: 3}
		}},
		{"repeat_interval negative", "repeat_interval", func(c *RuleCfg) {
			c.Notify = &NotifyPolicy{RepeatInterval: -time.Minute, MaxPerTier: 3}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := validCfg()
			c.mod(&cfg)
			_, err := ParseRule(cfg, testDefault)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "day_drop") || !strings.Contains(err.Error(), c.field) {
				t.Errorf("error %q should contain rule id %q and field %q", err, "day_drop", c.field)
			}
		})
	}
}
