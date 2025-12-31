package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Registry holds all Prometheus metrics.
type Registry struct {
	*prometheus.Registry

	// HTTP metrics
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestDuration  *prometheus.HistogramVec
	httpRequestsInFlight prometheus.Gauge
}

// NewRegistry creates a new metrics registry with all metrics registered.
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()

	// Register Go runtime metrics
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	r := &Registry{
		Registry: reg,

		httpRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),

		httpRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),

		httpRequestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "http_requests_in_flight",
				Help: "Number of HTTP requests currently in flight",
			},
		),
	}

	reg.MustRegister(r.httpRequestsTotal)
	reg.MustRegister(r.httpRequestDuration)
	reg.MustRegister(r.httpRequestsInFlight)

	return r
}

// RecordRequest records metrics for an HTTP request.
func (r *Registry) RecordRequest(method, path string, status int, duration float64) {
	statusStr := statusToString(status)
	r.httpRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	r.httpRequestDuration.WithLabelValues(method, path).Observe(duration)
}

// InFlightInc increments in-flight requests.
func (r *Registry) InFlightInc() {
	r.httpRequestsInFlight.Inc()
}

// InFlightDec decrements in-flight requests.
func (r *Registry) InFlightDec() {
	r.httpRequestsInFlight.Dec()
}

func statusToString(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	case status >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}
