package valuation

import "testing"

// Context Checkpoint: done_criteria → test mapping
// functional[0] "PercentileRank plan 用例全过" → TestPercentileRank

func TestPercentileRank(t *testing.T) {
	cases := []struct {
		name    string
		series  []float64
		current float64
		want    float64
	}{
		{"middle", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 5.5, 50},
		{"lowest", []float64{1, 2, 3, 4}, 0.5, 0},
		{"highest", []float64{1, 2, 3, 4}, 9, 100},
		{"all-equal", []float64{3, 3, 3, 3}, 3, 0}, // strictly-less 口径
		{"single", []float64{7}, 7, 0},
	}
	for _, c := range cases {
		if got := PercentileRank(c.series, c.current); got != c.want {
			t.Errorf("%s: PercentileRank = %v, want %v", c.name, got, c.want)
		}
	}
	if got := PercentileRank(nil, 1); got != -1 {
		t.Errorf("empty series should return -1, got %v", got)
	}
}
