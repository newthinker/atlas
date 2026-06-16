package strategy

// Context Checkpoint: done_criteria → test mapping
// functional[0] "SinceInceptionBars >= 100*252" → TestSinceInceptionBarsIsLarge

import "testing"

func TestSinceInceptionBarsIsLarge(t *testing.T) {
	const want = 100 * 252
	if SinceInceptionBars < want {
		t.Errorf("SinceInceptionBars = %d, want >= %d", SinceInceptionBars, want)
	}
}
