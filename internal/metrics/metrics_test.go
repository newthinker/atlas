package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestRegistry_HTTPMetrics(t *testing.T) {
	reg := NewRegistry()

	// Verify HTTP metrics are registered
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	// Should have go runtime metrics at minimum
	if len(mfs) == 0 {
		t.Error("expected some metrics to be registered")
	}
}

func TestRegistry_RecordRequest(t *testing.T) {
	reg := NewRegistry()

	reg.RecordRequest("GET", "/api/v1/signals", 200, 0.05)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "http_requests_total" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected http_requests_total metric")
	}
}

func TestRegistry_RecordRequest_StatusCodes(t *testing.T) {
	tests := []struct {
		status   int
		expected string
	}{
		{100, "1xx"},
		{200, "2xx"},
		{201, "2xx"},
		{301, "3xx"},
		{400, "4xx"},
		{404, "4xx"},
		{500, "5xx"},
		{503, "5xx"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			reg := NewRegistry()
			reg.RecordRequest("GET", "/test", tt.status, 0.01)

			mfs, err := reg.Gather()
			if err != nil {
				t.Fatalf("gather failed: %v", err)
			}

			found := false
			for _, mf := range mfs {
				if mf.GetName() == "http_requests_total" {
					for _, m := range mf.GetMetric() {
						for _, label := range m.GetLabel() {
							if label.GetName() == "status" && label.GetValue() == tt.expected {
								found = true
							}
						}
					}
				}
			}
			if !found {
				t.Errorf("expected status label %s for status code %d", tt.expected, tt.status)
			}
		})
	}
}

func TestRegistry_InFlight(t *testing.T) {
	reg := NewRegistry()

	reg.InFlightInc()
	reg.InFlightInc()
	reg.InFlightDec()

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "http_requests_in_flight" {
			found = true
			for _, m := range mf.GetMetric() {
				if m.GetGauge().GetValue() != 1 {
					t.Errorf("expected in-flight gauge to be 1, got %v", m.GetGauge().GetValue())
				}
			}
		}
	}
	if !found {
		t.Error("expected http_requests_in_flight metric")
	}
}

func TestRegistry_DurationHistogram(t *testing.T) {
	reg := NewRegistry()

	reg.RecordRequest("POST", "/api/v1/analysis", 200, 0.123)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "http_request_duration_seconds" {
			found = true
			for _, m := range mf.GetMetric() {
				hist := m.GetHistogram()
				if hist.GetSampleCount() != 1 {
					t.Errorf("expected sample count 1, got %d", hist.GetSampleCount())
				}
				if hist.GetSampleSum() < 0.12 || hist.GetSampleSum() > 0.13 {
					t.Errorf("expected sample sum ~0.123, got %v", hist.GetSampleSum())
				}
			}
		}
	}
	if !found {
		t.Error("expected http_request_duration_seconds metric")
	}
}

// Ensure the registry implements prometheus.Gatherer interface
func TestRegistry_ImplementsGatherer(t *testing.T) {
	reg := NewRegistry()
	var _ prometheus.Gatherer = reg
}

func TestRegistry_BusinessMetrics(t *testing.T) {
	reg := NewRegistry()

	reg.RecordSignal("ma_crossover", "buy")
	reg.RecordAnalysisCycle(0.5)
	reg.RecordBacktest("complete", 2.5)
	reg.SetWatchlistSize(10)

	mfs, _ := reg.Gather()

	expected := []string{
		"atlas_signals_generated_total",
		"atlas_analysis_cycles_total",
		"atlas_analysis_duration_seconds",
		"atlas_backtests_total",
		"atlas_backtest_duration_seconds",
		"atlas_watchlist_symbols",
	}

	found := make(map[string]bool)
	for _, mf := range mfs {
		found[mf.GetName()] = true
	}

	for _, name := range expected {
		if !found[name] {
			t.Errorf("expected metric %s not found", name)
		}
	}
}

func TestRegistry_SignalRouted(t *testing.T) {
	reg := NewRegistry()

	reg.RecordSignalRouted("telegram", "success")

	mfs, _ := reg.Gather()
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "atlas_signals_routed_total" {
			found = true
		}
	}
	if !found {
		t.Error("expected atlas_signals_routed_total metric")
	}
}

func TestRegistry_JobsActive(t *testing.T) {
	reg := NewRegistry()

	reg.SetJobsActive("backtest", 3)

	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "atlas_jobs_active" {
			for _, m := range mf.GetMetric() {
				if m.GetGauge().GetValue() != 3 {
					t.Errorf("expected jobs active to be 3, got %v", m.GetGauge().GetValue())
				}
			}
		}
	}
}
