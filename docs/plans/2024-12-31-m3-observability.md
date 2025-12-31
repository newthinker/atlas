# M3: Observability Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Prometheus metrics, request logging, and alerting with integration to existing notifiers.

**Architecture:** Add metrics middleware that wraps HTTP handlers, a business metrics collector for app-level stats, and a lightweight alert evaluator that uses existing Telegram/Email notifiers.

**Tech Stack:** Go 1.24, prometheus/client_golang, google/uuid, zap logging.

---

## Task 1: Add Prometheus Dependency & Metrics Registry

**Files:**
- Modify: `go.mod`
- Create: `internal/metrics/metrics.go`
- Test: `internal/metrics/metrics_test.go`

**Step 1: Add Prometheus dependency**

Run:
```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
go get github.com/google/uuid
```

**Step 2: Write the failing test**

```go
// internal/metrics/metrics_test.go
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
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/metrics/... -v`
Expected: FAIL - package not found

**Step 4: Write minimal implementation**

```go
// internal/metrics/metrics.go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Registry holds all Prometheus metrics.
type Registry struct {
	*prometheus.Registry

	// HTTP metrics
	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
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
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/metrics/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add go.mod go.sum internal/metrics/
git commit -m "feat(metrics): add Prometheus registry and HTTP metrics"
```

---

## Task 2: HTTP Metrics Middleware

**Files:**
- Create: `internal/metrics/middleware.go`
- Test: `internal/metrics/middleware_test.go`

**Step 1: Write the failing test**

```go
// internal/metrics/middleware_test.go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/... -v -run Middleware`
Expected: FAIL - HTTPMiddleware not defined

**Step 3: Write minimal implementation**

```go
// internal/metrics/middleware.go
package metrics

import (
	"net/http"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// HTTPMiddleware returns middleware that records HTTP metrics.
func HTTPMiddleware(reg *Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reg.InFlightInc()
			defer reg.InFlightDec()

			start := time.Now()

			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rw, r)

			duration := time.Since(start).Seconds()
			reg.RecordRequest(r.Method, r.URL.Path, rw.statusCode, duration)
		})
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/metrics/middleware.go internal/metrics/middleware_test.go
git commit -m "feat(metrics): add HTTP metrics middleware"
```

---

## Task 3: Request Logging Middleware

**Files:**
- Create: `internal/metrics/logging.go`
- Test: `internal/metrics/logging_test.go`

**Step 1: Write the failing test**

```go
// internal/metrics/logging_test.go
package metrics

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestLoggingMiddleware(t *testing.T) {
	// Create a buffer to capture logs
	var buf bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.InfoLevel)
	logger := zap.New(core)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := LoggingMiddleware(logger)(handler)

	req := httptest.NewRequest("GET", "/api/v1/signals", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	// Parse the log line
	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log: %v, log: %s", err, buf.String())
	}

	if logEntry["method"] != "GET" {
		t.Errorf("expected method GET, got %v", logEntry["method"])
	}
	if logEntry["path"] != "/api/v1/signals" {
		t.Errorf("expected path /api/v1/signals, got %v", logEntry["path"])
	}
	if logEntry["status"].(float64) != 200 {
		t.Errorf("expected status 200, got %v", logEntry["status"])
	}
}

func TestLoggingMiddleware_AddsRequestID(t *testing.T) {
	var buf bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.InfoLevel)
	logger := zap.New(core)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := LoggingMiddleware(logger)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	// Check response header has request ID
	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("expected X-Request-ID header")
	}

	// Check log has request ID
	var logEntry map[string]any
	json.Unmarshal(buf.Bytes(), &logEntry)
	if logEntry["request_id"] != requestID {
		t.Errorf("expected request_id %s, got %v", requestID, logEntry["request_id"])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/... -v -run Logging`
Expected: FAIL - LoggingMiddleware not defined

**Step 3: Write minimal implementation**

```go
// internal/metrics/logging.go
package metrics

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// LoggingMiddleware returns middleware that logs each request.
func LoggingMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate request ID
			requestID := uuid.New().String()
			w.Header().Set("X-Request-ID", requestID)

			start := time.Now()

			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rw, r)

			duration := time.Since(start)

			logger.Info("request",
				zap.String("request_id", requestID),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rw.statusCode),
				zap.Int64("duration_ms", duration.Milliseconds()),
				zap.String("client_ip", getClientIP(r)),
			)
		})
	}
}

// getClientIP extracts client IP from request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	return r.RemoteAddr
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/metrics/logging.go internal/metrics/logging_test.go
git commit -m "feat(metrics): add request logging middleware with request ID"
```

---

## Task 4: Business Metrics Collector

**Files:**
- Modify: `internal/metrics/metrics.go`
- Test: `internal/metrics/metrics_test.go` (add tests)

**Step 1: Add failing tests**

Add to `internal/metrics/metrics_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/metrics/... -v -run Business`
Expected: FAIL - RecordSignal not defined

**Step 3: Add business metrics to implementation**

Update `internal/metrics/metrics.go` - add fields to Registry struct:

```go
// Add to Registry struct:
	// Business metrics
	signalsGenerated   *prometheus.CounterVec
	signalsRouted      *prometheus.CounterVec
	analysisCycles     prometheus.Counter
	analysisDuration   prometheus.Histogram
	backtestsTotal     *prometheus.CounterVec
	backtestDuration   prometheus.Histogram
	jobsActive         *prometheus.GaugeVec
	watchlistSymbols   prometheus.Gauge
```

Add to NewRegistry():

```go
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
```

Add methods:

```go
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/metrics/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/metrics/metrics.go internal/metrics/metrics_test.go
git commit -m "feat(metrics): add business metrics for signals, analysis, backtests"
```

---

## Task 5: Alert Evaluator

**Files:**
- Create: `internal/alert/evaluator.go`
- Create: `internal/alert/rules.go`
- Test: `internal/alert/evaluator_test.go`

**Step 1: Write the failing test**

```go
// internal/alert/evaluator_test.go
package alert

import (
	"testing"
	"time"
)

type mockNotifier struct {
	sent []string
}

func (m *mockNotifier) Name() string { return "mock" }
func (m *mockNotifier) Notify(msg string) error {
	m.sent = append(m.sent, msg)
	return nil
}

func TestEvaluator_EvaluateRule(t *testing.T) {
	notifier := &mockNotifier{}
	eval := NewEvaluator([]Notifier{notifier})

	rule := Rule{
		Name:     "test_alert",
		Expr:     "error_rate > 0.05",
		For:      time.Minute,
		Severity: "warning",
		Message:  "Error rate is high",
	}

	// Provide metrics that trigger the rule
	metrics := map[string]float64{
		"error_rate": 0.10, // 10% > 5%
	}

	eval.SetMetrics(metrics)
	eval.Evaluate(rule)

	// First evaluation starts the pending timer, doesn't fire
	if len(notifier.sent) != 0 {
		t.Errorf("expected no notification on first eval, got %d", len(notifier.sent))
	}

	// Simulate time passing and re-evaluate
	eval.advanceTime(2 * time.Minute)
	eval.Evaluate(rule)

	if len(notifier.sent) != 1 {
		t.Errorf("expected 1 notification after duration, got %d", len(notifier.sent))
	}
}

func TestEvaluator_Cooldown(t *testing.T) {
	notifier := &mockNotifier{}
	eval := NewEvaluator([]Notifier{notifier})
	eval.SetCooldown(5 * time.Minute)

	rule := Rule{
		Name:     "test_alert",
		Expr:     "up == 0",
		For:      0, // Immediate
		Severity: "critical",
		Message:  "Service is down",
	}

	metrics := map[string]float64{"up": 0}
	eval.SetMetrics(metrics)

	eval.Evaluate(rule)
	eval.Evaluate(rule)
	eval.Evaluate(rule)

	// Should only notify once due to cooldown
	if len(notifier.sent) != 1 {
		t.Errorf("expected 1 notification due to cooldown, got %d", len(notifier.sent))
	}
}

func TestEvaluator_RuleNotTriggered(t *testing.T) {
	notifier := &mockNotifier{}
	eval := NewEvaluator([]Notifier{notifier})

	rule := Rule{
		Name:     "test_alert",
		Expr:     "error_rate > 0.05",
		For:      0,
		Severity: "warning",
		Message:  "Error rate is high",
	}

	metrics := map[string]float64{"error_rate": 0.01} // 1% < 5%
	eval.SetMetrics(metrics)

	eval.Evaluate(rule)

	if len(notifier.sent) != 0 {
		t.Errorf("expected no notification, got %d", len(notifier.sent))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/alert/... -v`
Expected: FAIL - package not found

**Step 3: Write rules implementation**

```go
// internal/alert/rules.go
package alert

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Rule defines an alert rule.
type Rule struct {
	Name     string        `mapstructure:"name"`
	Expr     string        `mapstructure:"expr"`
	For      time.Duration `mapstructure:"for"`
	Severity string        `mapstructure:"severity"`
	Message  string        `mapstructure:"message"`
}

// Evaluate evaluates the rule expression against metrics.
func (r *Rule) Evaluate(metrics map[string]float64) bool {
	// Simple expression parser: "metric op value"
	// Supports: >, <, >=, <=, ==, !=
	pattern := regexp.MustCompile(`^(\w+)\s*(>|<|>=|<=|==|!=)\s*([\d.]+)$`)
	matches := pattern.FindStringSubmatch(strings.TrimSpace(r.Expr))

	if len(matches) != 4 {
		return false
	}

	metricName := matches[1]
	op := matches[2]
	threshold, err := strconv.ParseFloat(matches[3], 64)
	if err != nil {
		return false
	}

	value, exists := metrics[metricName]
	if !exists {
		return false
	}

	switch op {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	case ">=":
		return value >= threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}

// FormatMessage formats the alert message with metric values.
func (r *Rule) FormatMessage(metrics map[string]float64) string {
	msg := fmt.Sprintf("[%s] %s: %s", strings.ToUpper(r.Severity), r.Name, r.Message)
	return msg
}
```

**Step 4: Write evaluator implementation**

```go
// internal/alert/evaluator.go
package alert

import (
	"sync"
	"time"
)

// Notifier interface for sending alerts.
type Notifier interface {
	Name() string
	Notify(msg string) error
}

// Evaluator evaluates alert rules and sends notifications.
type Evaluator struct {
	notifiers []Notifier
	metrics   map[string]float64
	cooldown  time.Duration

	// Track pending alerts (waiting for "for" duration)
	pending map[string]time.Time
	// Track last fired time for cooldown
	lastFired map[string]time.Time

	// For testing: allow time advancement
	now func() time.Time

	mu sync.RWMutex
}

// NewEvaluator creates a new alert evaluator.
func NewEvaluator(notifiers []Notifier) *Evaluator {
	return &Evaluator{
		notifiers: notifiers,
		metrics:   make(map[string]float64),
		cooldown:  5 * time.Minute,
		pending:   make(map[string]time.Time),
		lastFired: make(map[string]time.Time),
		now:       time.Now,
	}
}

// SetMetrics updates the current metrics.
func (e *Evaluator) SetMetrics(metrics map[string]float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.metrics = metrics
}

// SetCooldown sets the cooldown duration between alerts.
func (e *Evaluator) SetCooldown(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cooldown = d
}

// Evaluate evaluates a single rule and fires notification if triggered.
func (e *Evaluator) Evaluate(rule Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := e.now()

	// Check if rule condition is met
	if !rule.Evaluate(e.metrics) {
		// Rule not triggered, clear pending state
		delete(e.pending, rule.Name)
		return
	}

	// Rule is triggered
	if rule.For > 0 {
		// Check if we're already pending
		pendingSince, isPending := e.pending[rule.Name]
		if !isPending {
			// Start pending
			e.pending[rule.Name] = now
			return
		}

		// Check if pending duration exceeded
		if now.Sub(pendingSince) < rule.For {
			return // Still waiting
		}
	}

	// Check cooldown
	lastFired, hasFired := e.lastFired[rule.Name]
	if hasFired && now.Sub(lastFired) < e.cooldown {
		return // In cooldown
	}

	// Fire alert
	msg := rule.FormatMessage(e.metrics)
	for _, n := range e.notifiers {
		n.Notify(msg)
	}

	e.lastFired[rule.Name] = now
	delete(e.pending, rule.Name)
}

// EvaluateAll evaluates all rules.
func (e *Evaluator) EvaluateAll(rules []Rule) {
	for _, rule := range rules {
		e.Evaluate(rule)
	}
}

// advanceTime is for testing - advances the internal clock.
func (e *Evaluator) advanceTime(d time.Duration) {
	oldNow := e.now
	e.now = func() time.Time {
		return oldNow().Add(d)
	}
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/alert/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/alert/
git commit -m "feat(alert): add alert evaluator with rules and cooldown"
```

---

## Task 6: Config Additions & Server Wiring

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/api/server.go`
- Modify: `configs/config.example.yaml`

**Step 1: Add config types**

Add to `internal/config/config.go`:

```go
// MetricsConfig holds metrics configuration.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

// AlertsConfig holds alerts configuration.
type AlertsConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	CheckInterval time.Duration `mapstructure:"check_interval"`
	Rules         []AlertRule   `mapstructure:"rules"`
}

// AlertRule defines a single alert rule.
type AlertRule struct {
	Name     string        `mapstructure:"name"`
	Expr     string        `mapstructure:"expr"`
	For      time.Duration `mapstructure:"for"`
	Severity string        `mapstructure:"severity"`
	Message  string        `mapstructure:"message"`
}
```

Add to Config struct:

```go
	Metrics MetricsConfig `mapstructure:"metrics"`
	Alerts  AlertsConfig  `mapstructure:"alerts"`
```

Add to Defaults():

```go
	Metrics: MetricsConfig{
		Enabled: true,
		Path:    "/metrics",
	},
	Alerts: AlertsConfig{
		Enabled:       false,
		CheckInterval: 60 * time.Second,
	},
```

**Step 2: Update server.go**

Add imports and update Dependencies:

```go
import (
	// ... existing imports
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/newthinker/atlas/internal/metrics"
)

// Add to Dependencies:
	Metrics *metrics.Registry
```

Update setupRoutes to add metrics endpoint and wrap handlers:

```go
// In setupRoutes, add metrics endpoint:
	if cfg.TemplatesDir != "" || true { // Always add metrics
		// Metrics endpoint
		s.mux.Handle("/metrics", promhttp.HandlerFor(deps.Metrics, promhttp.HandlerOpts{}))
	}

// Wrap API handlers with metrics middleware
	metricsMiddleware := metrics.HTTPMiddleware(deps.Metrics)
	loggingMiddleware := metrics.LoggingMiddleware(s.logger)

	// Update route registration to use middleware chain:
	// Example: s.mux.Handle("/api/v1/signals",
	//     loggingMiddleware(metricsMiddleware(authMiddleware(http.HandlerFunc(signalsHandler.List)))))
```

**Step 3: Update config.example.yaml**

Add to `configs/config.example.yaml`:

```yaml
# Metrics configuration
metrics:
  enabled: true
  path: "/metrics"

# Alerting configuration
alerts:
  enabled: true
  check_interval: 60s
  rules:
    - name: high_error_rate
      expr: "error_rate > 0.05"
      for: 5m
      severity: warning
      message: "API error rate above 5%"
    - name: api_down
      expr: "up == 0"
      for: 1m
      severity: critical
      message: "ATLAS API is not responding"
```

**Step 4: Update serve.go**

Add metrics registry creation:

```go
	// Create metrics registry
	metricsReg := metrics.NewRegistry()

	// Add to deps:
	deps := api.Dependencies{
		// ... existing
		Metrics: metricsReg,
	}
```

**Step 5: Run tests**

Run: `go test ./... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/config/config.go internal/api/server.go configs/config.example.yaml cmd/atlas/serve.go
git commit -m "feat(metrics): wire metrics and logging middleware into server"
```

---

## Final Steps

**Step 1: Run all tests**

```bash
go test ./... -v
```
Expected: All tests pass

**Step 2: Verify build**

```bash
go build ./...
```
Expected: No errors

**Step 3: Manual test**

```bash
./bin/atlas serve -c configs/config.example.yaml &
curl http://localhost:8080/metrics | head -20
curl http://localhost:8080/api/health
```
Expected: Prometheus metrics output, health OK

---

## Summary

| Task | Files | Description |
|------|-------|-------------|
| 1 | `metrics/metrics.go` | Prometheus registry & HTTP metrics |
| 2 | `metrics/middleware.go` | HTTP metrics middleware |
| 3 | `metrics/logging.go` | Request logging middleware |
| 4 | `metrics/metrics.go` | Business metrics (signals, backtests) |
| 5 | `alert/evaluator.go` | Alert evaluator with notifier integration |
| 6 | `config/`, `server.go` | Config additions & server wiring |

**Total: 6 tasks, ~12 commits**
