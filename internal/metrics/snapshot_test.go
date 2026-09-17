package metrics

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// Context Checkpoint: done_criteria → test mapping (TASK-201)
// functional[0]    "gauge 取当前值 / counter 取累计值"                    → TestSnapshot_GaugeAndCounter
// functional[1]    "多 label 序列聚合求和为单键（3+5→8）"                → TestSnapshot_MultiLabelSum
// functional[2]    "histogram 展开 _count/_sum"                          → TestSnapshot_Histogram
// functional[3]    "3 位数字或 [1-5]xx 字符串 status → _<N>xx 求和；其他值不产额外键" → TestSnapshot_StatusClassKeys / TestSnapshot_RecordRequestProduces5xx
// boundary[0]      "空 registry → 空 map，非 nil，不 panic"             → TestSnapshot_EmptyRegistry
// error_handling[0] "Gather 出错不 panic，处理已收集部分"                → TestSnapshot_GatherError_NoPanic

// snapshotRegistry wraps a bare prometheus registry (the injected "fake") so a
// test can register only the metrics under test and call Snapshot.
func snapshotRegistry(t *testing.T, cs ...prometheus.Collector) *Registry {
	t.Helper()
	reg := prometheus.NewRegistry()
	for _, c := range cs {
		if err := reg.Register(c); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	return &Registry{Registry: reg}
}

func TestSnapshot_GaugeAndCounter(t *testing.T) {
	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_gauge"})
	g.Set(42)
	c := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_counter"})
	c.Add(7)

	snap := snapshotRegistry(t, g, c).Snapshot()

	if snap["test_gauge"] != 42 {
		t.Errorf("test_gauge = %v, want 42", snap["test_gauge"])
	}
	if snap["test_counter"] != 7 {
		t.Errorf("test_counter = %v, want 7", snap["test_counter"])
	}
}

func TestSnapshot_MultiLabelSum(t *testing.T) {
	cv := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_multi_total"},
		[]string{"a", "b"},
	)
	cv.WithLabelValues("x", "1").Add(3)
	cv.WithLabelValues("y", "2").Add(5)

	snap := snapshotRegistry(t, cv).Snapshot()

	if snap["test_multi_total"] != 8 {
		t.Errorf("test_multi_total = %v, want 8 (3+5)", snap["test_multi_total"])
	}
}

func TestSnapshot_Histogram(t *testing.T) {
	h := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "test_hist_seconds",
		Buckets: []float64{1, 5, 10},
	})
	h.Observe(2)
	h.Observe(4)

	snap := snapshotRegistry(t, h).Snapshot()

	if snap["test_hist_seconds_count"] != 2 {
		t.Errorf("_count = %v, want 2", snap["test_hist_seconds_count"])
	}
	if snap["test_hist_seconds_sum"] != 6 {
		t.Errorf("_sum = %v, want 6 (2+4)", snap["test_hist_seconds_sum"])
	}
}

func TestSnapshot_StatusClassKeys(t *testing.T) {
	cv := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_requests_total"},
		[]string{"status"},
	)
	cv.WithLabelValues("500").Add(2)  // 3-digit numeric → _5xx
	cv.WithLabelValues("502").Add(3)  // 3-digit numeric → _5xx
	cv.WithLabelValues("5xx").Add(1)  // class string (RecordRequest form) → _5xx
	cv.WithLabelValues("200").Add(4)  // → _2xx
	cv.WithLabelValues("ok").Add(9)   // other value → no extra key

	snap := snapshotRegistry(t, cv).Snapshot()

	// base name still aggregates every series.
	if snap["test_requests_total"] != 19 {
		t.Errorf("base = %v, want 19 (2+3+1+4+9)", snap["test_requests_total"])
	}
	// numeric 500/502 and the "5xx" class string all fold into _5xx (AD-13a).
	if snap["test_requests_total_5xx"] != 6 {
		t.Errorf("_5xx = %v, want 6 (500:2 + 502:3 + 5xx:1)", snap["test_requests_total_5xx"])
	}
	if snap["test_requests_total_2xx"] != 4 {
		t.Errorf("_2xx = %v, want 4", snap["test_requests_total_2xx"])
	}
	// values that are neither 3-digit nor [1-5]xx must not spawn a class key.
	if _, ok := snap["test_requests_total_okxx"]; ok {
		t.Error("non-status value must not produce a class key")
	}
}

// TestSnapshot_RecordRequestProduces5xx locks in the TASK-203 prerequisite: the
// real recording path (RecordRequest → statusToString stores "5xx") must surface
// an http_requests_total_5xx key through Snapshot (AD-13a). Note the metric is
// named http_requests_total (no atlas_ prefix — see metrics.go).
func TestSnapshot_RecordRequestProduces5xx(t *testing.T) {
	reg := NewRegistry()
	reg.RecordRequest("GET", "/x", 503, 0.01)
	reg.RecordRequest("GET", "/x", 200, 0.01)

	snap := reg.Snapshot()

	if snap["http_requests_total_5xx"] != 1 {
		t.Errorf("http_requests_total_5xx = %v, want 1", snap["http_requests_total_5xx"])
	}
	if snap["http_requests_total_2xx"] != 1 {
		t.Errorf("http_requests_total_2xx = %v, want 1", snap["http_requests_total_2xx"])
	}
}

func TestSnapshot_EmptyRegistry(t *testing.T) {
	snap := snapshotRegistry(t).Snapshot()

	if snap == nil {
		t.Fatal("Snapshot must not return nil")
	}
	if len(snap) != 0 {
		t.Errorf("empty registry snapshot = %v, want empty", snap)
	}
}

// erroringGatherer returns a partial family alongside an error, exercising the
// documented "process what was gathered, never panic" strategy.
type erroringGatherer struct{}

func (erroringGatherer) Gather() ([]*dto.MetricFamily, error) {
	name := "partial_counter"
	typ := dto.MetricType_COUNTER
	val := 3.0
	return []*dto.MetricFamily{{
		Name: &name,
		Type: &typ,
		Metric: []*dto.Metric{{
			Counter: &dto.Counter{Value: &val},
		}},
	}}, errors.New("gather boom")
}

func TestSnapshot_GatherError_NoPanic(t *testing.T) {
	snap := snapshot(erroringGatherer{}) // must not panic

	if snap == nil {
		t.Fatal("snapshot must not return nil on gather error")
	}
	// partial family that was gathered is still surfaced.
	if snap["partial_counter"] != 3 {
		t.Errorf("partial_counter = %v, want 3 (partial data kept)", snap["partial_counter"])
	}
}

// Context Checkpoint: done_criteria → test mapping (TASK-002)
// functional[0]     "gauge state=failed/done 展开为 x_failed/x_done，原求和不变；status 与 state 同在两键都产生" → TestSnapshot_StateGaugeExpands / TestSnapshot_StateAndStatusBothExpand
// functional[1]     "counter 同样展开 y_failed"                                   → TestSnapshot_StateCounterExpands
// functional[2]     "既有 status 分类行为不变（既有测试函数体零改动）"              → review（本块之上的既有测试未改）
// boundary[0]       "无 state label ⇒ 键集合精确等于 {z}"                         → TestSnapshot_NoStateLabel_NoExtraKeys
// boundary[1]       "同 state 多序列（shard 区分）⇒ <name>_<state> 求和"           → TestSnapshot_StateSameValueSums
// boundary[2]       "state 为空串或不匹配 ^\w+$ ⇒ 不产生 <name>_ 键"               → TestSnapshot_StateInvalidValue_NoKey
// error_handling[0] "histogram 带 state ⇒ 只有 _count/_sum"                        → TestSnapshot_HistogramWithState_NoStateKey

// assertKeys fails unless snap's key set is exactly want.
func assertKeys(t *testing.T, snap map[string]float64, want ...string) {
	t.Helper()
	if len(snap) != len(want) {
		t.Errorf("keys = %v, want exactly %v", snap, want)
		return
	}
	for _, k := range want {
		if _, ok := snap[k]; !ok {
			t.Errorf("missing key %q in %v", k, snap)
		}
	}
}

func TestSnapshot_StateGaugeExpands(t *testing.T) {
	gv := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "x"}, []string{"state"})
	gv.WithLabelValues("failed").Set(2)
	gv.WithLabelValues("done").Set(5)

	snap := snapshotRegistry(t, gv).Snapshot()

	if snap["x"] != 7 {
		t.Errorf("x = %v, want 7 (base sum unchanged)", snap["x"])
	}
	if snap["x_failed"] != 2 {
		t.Errorf("x_failed = %v, want 2", snap["x_failed"])
	}
	if snap["x_done"] != 5 {
		t.Errorf("x_done = %v, want 5", snap["x_done"])
	}
	assertKeys(t, snap, "x", "x_failed", "x_done")
}

// addStatusClass returns as soon as it sees a status label, so the state
// expansion must not live inside that loop — both keys have to appear.
func TestSnapshot_StateAndStatusBothExpand(t *testing.T) {
	gv := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "x"}, []string{"status", "state"})
	gv.WithLabelValues("500", "failed").Set(4)

	snap := snapshotRegistry(t, gv).Snapshot()

	if snap["x_5xx"] != 4 {
		t.Errorf("x_5xx = %v, want 4", snap["x_5xx"])
	}
	if snap["x_failed"] != 4 {
		t.Errorf("x_failed = %v, want 4", snap["x_failed"])
	}
}

func TestSnapshot_StateCounterExpands(t *testing.T) {
	cv := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "y"}, []string{"state"})
	cv.WithLabelValues("failed").Add(3)

	snap := snapshotRegistry(t, cv).Snapshot()

	if snap["y"] != 3 {
		t.Errorf("y = %v, want 3", snap["y"])
	}
	if snap["y_failed"] != 3 {
		t.Errorf("y_failed = %v, want 3", snap["y_failed"])
	}
}

func TestSnapshot_NoStateLabel_NoExtraKeys(t *testing.T) {
	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: "z"})
	g.Set(1)

	snap := snapshotRegistry(t, g).Snapshot()

	assertKeys(t, snap, "z")
}

func TestSnapshot_StateSameValueSums(t *testing.T) {
	gv := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "x"}, []string{"state", "shard"})
	gv.WithLabelValues("failed", "a").Set(2)
	gv.WithLabelValues("failed", "b").Set(6)

	snap := snapshotRegistry(t, gv).Snapshot()

	if snap["x_failed"] != 8 {
		t.Errorf("x_failed = %v, want 8 (2+6)", snap["x_failed"])
	}
}

func TestSnapshot_StateInvalidValue_NoKey(t *testing.T) {
	tests := []struct {
		name  string
		state string
	}{
		{"empty", ""},
		{"hyphen", "in-flight"},
		{"space", "in flight"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gv := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "x"}, []string{"state"})
			gv.WithLabelValues(tt.state).Set(1)

			snap := snapshotRegistry(t, gv).Snapshot()

			assertKeys(t, snap, "x")
		})
	}
}

func TestSnapshot_HistogramWithState_NoStateKey(t *testing.T) {
	hv := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "h",
		Buckets: []float64{1},
	}, []string{"state"})
	hv.WithLabelValues("failed").Observe(0.5)

	snap := snapshotRegistry(t, hv).Snapshot()

	assertKeys(t, snap, "h_count", "h_sum")
}
