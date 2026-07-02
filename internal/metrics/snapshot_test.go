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
