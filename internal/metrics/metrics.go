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

	// Business metrics
	signalsGenerated *prometheus.CounterVec
	signalsRouted    *prometheus.CounterVec
	analysisCycles   prometheus.Counter
	analysisDuration prometheus.Histogram
	backtestsTotal   *prometheus.CounterVec
	backtestDuration prometheus.Histogram
	jobsActive       *prometheus.GaugeVec
	watchlistSymbols prometheus.Gauge
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

	// Business metrics
	r.signalsGenerated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "atlas_signals_generated_total",
			Help: "Total number of signals generated",
		},
		[]string{"strategy", "action"},
	)
	r.signalsRouted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "atlas_signals_routed_total",
			Help: "Total number of signals routed to notifiers",
		},
		[]string{"notifier", "status"},
	)
	r.analysisCycles = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "atlas_analysis_cycles_total",
			Help: "Total number of analysis cycles completed",
		},
	)
	r.analysisDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "atlas_analysis_duration_seconds",
			Help:    "Analysis cycle duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
	r.backtestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "atlas_backtests_total",
			Help: "Total number of backtests",
		},
		[]string{"status"},
	)
	r.backtestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "atlas_backtest_duration_seconds",
			Help:    "Backtest duration in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
		},
	)
	r.jobsActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "atlas_jobs_active",
			Help: "Number of active jobs",
		},
		[]string{"type"},
	)
	r.watchlistSymbols = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "atlas_watchlist_symbols",
			Help: "Number of symbols in watchlist",
		},
	)

	reg.MustRegister(r.signalsGenerated)
	reg.MustRegister(r.signalsRouted)
	reg.MustRegister(r.analysisCycles)
	reg.MustRegister(r.analysisDuration)
	reg.MustRegister(r.backtestsTotal)
	reg.MustRegister(r.backtestDuration)
	reg.MustRegister(r.jobsActive)
	reg.MustRegister(r.watchlistSymbols)

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

// RecordSignal records a generated signal.
func (r *Registry) RecordSignal(strategy, action string) {
	r.signalsGenerated.WithLabelValues(strategy, action).Inc()
}

// RecordSignalRouted records a routed signal.
func (r *Registry) RecordSignalRouted(notifier, status string) {
	r.signalsRouted.WithLabelValues(notifier, status).Inc()
}

// RecordAnalysisCycle records an analysis cycle completion.
func (r *Registry) RecordAnalysisCycle(duration float64) {
	r.analysisCycles.Inc()
	r.analysisDuration.Observe(duration)
}

// RecordBacktest records a backtest completion.
func (r *Registry) RecordBacktest(status string, duration float64) {
	r.backtestsTotal.WithLabelValues(status).Inc()
	r.backtestDuration.Observe(duration)
}

// SetJobsActive sets the number of active jobs of a type.
func (r *Registry) SetJobsActive(jobType string, count int) {
	r.jobsActive.WithLabelValues(jobType).Set(float64(count))
}

// SetWatchlistSize sets the watchlist size.
func (r *Registry) SetWatchlistSize(size int) {
	r.watchlistSymbols.Set(float64(size))
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
