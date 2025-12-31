package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPMiddleware(t *testing.T) {
	reg := NewRegistry()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	wrapped := HTTPMiddleware(reg)(handler)

	req := httptest.NewRequest("GET", "/api/v1/signals", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Verify metrics were recorded
	mfs, _ := reg.Gather()
	foundRequests := false
	for _, mf := range mfs {
		if mf.GetName() == "http_requests_total" {
			foundRequests = true
			break
		}
	}
	if !foundRequests {
		t.Error("expected http_requests_total to be recorded")
	}
}

func TestHTTPMiddleware_RecordsDuration(t *testing.T) {
	reg := NewRegistry()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := HTTPMiddleware(reg)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	mfs, _ := reg.Gather()
	foundDuration := false
	for _, mf := range mfs {
		if mf.GetName() == "http_request_duration_seconds" {
			foundDuration = true
			break
		}
	}
	if !foundDuration {
		t.Error("expected http_request_duration_seconds to be recorded")
	}
}

func TestHTTPMiddleware_TracksInFlight(t *testing.T) {
	reg := NewRegistry()

	inFlightDuringRequest := float64(-1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture in-flight value during request
		mfs, _ := reg.Gather()
		for _, mf := range mfs {
			if mf.GetName() == "http_requests_in_flight" {
				for _, m := range mf.GetMetric() {
					inFlightDuringRequest = m.GetGauge().GetValue()
				}
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	wrapped := HTTPMiddleware(reg)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if inFlightDuringRequest != 1 {
		t.Errorf("expected in-flight to be 1 during request, got %v", inFlightDuringRequest)
	}

	// After request completes, in-flight should be 0
	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "http_requests_in_flight" {
			for _, m := range mf.GetMetric() {
				if m.GetGauge().GetValue() != 0 {
					t.Errorf("expected in-flight to be 0 after request, got %v", m.GetGauge().GetValue())
				}
			}
		}
	}
}

func TestHTTPMiddleware_CapturesStatusCode(t *testing.T) {
	reg := NewRegistry()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	wrapped := HTTPMiddleware(reg)(handler)

	req := httptest.NewRequest("GET", "/not-found", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "http_requests_total" {
			for _, m := range mf.GetMetric() {
				for _, label := range m.GetLabel() {
					if label.GetName() == "status" && label.GetValue() != "4xx" {
						t.Errorf("expected status label 4xx, got %s", label.GetValue())
					}
				}
			}
		}
	}
}
