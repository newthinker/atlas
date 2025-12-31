# M3: Observability Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:writing-plans to create implementation plan from this design.

**Goal:** Add production monitoring capabilities with Prometheus metrics, request logging, and alerting.

**Date:** 2024-12-31

---

## 1. Overview

**Scope:**
- Prometheus metrics endpoint (`/metrics`)
- Request logging middleware for debugging
- Alert evaluator with integration to existing notifiers (Telegram/Email)

**Not Included:**
- Distributed tracing (future)
- Rate limiting (future)

---

## 2. Metrics Categories

| Category | Metrics |
|----------|---------|
| **API** | `http_requests_total`, `http_request_duration_seconds`, `http_requests_in_flight` |
| **Business** | `atlas_signals_generated_total`, `atlas_backtests_total`, `atlas_analysis_cycles_total` |
| **System** | Go runtime metrics (goroutines, memory, GC) via default collector |

---

## 3. API Metrics

Collected automatically via middleware for every HTTP request:

```
http_requests_total{method="GET", path="/api/v1/signals", status="200"}
http_request_duration_seconds{method="GET", path="/api/v1/signals"}  # histogram
http_requests_in_flight{handler="/api/v1/signals"}  # gauge
```

---

## 4. Business Metrics

Instrumented in handlers and app:

```
# Signals
atlas_signals_generated_total{strategy="ma_crossover", action="buy"}
atlas_signals_routed_total{notifier="telegram", status="success"}

# Analysis
atlas_analysis_cycles_total
atlas_analysis_duration_seconds  # histogram

# Backtests
atlas_backtests_total{status="complete|failed"}
atlas_backtest_duration_seconds  # histogram

# Jobs
atlas_jobs_active{type="backtest"}  # gauge
atlas_jobs_completed_total{type="backtest", status="complete|failed"}

# Watchlist
atlas_watchlist_symbols  # gauge - current count
```

---

## 5. Request Logging

Every request logs:
- Request ID (UUID, added to response header `X-Request-ID`)
- Method, path, status code
- Duration in milliseconds
- Client IP
- API key ID (if authenticated, not the key itself)

**Log Format:**
```json
{
  "level": "info",
  "ts": 1704067200,
  "msg": "request",
  "request_id": "abc-123",
  "method": "GET",
  "path": "/api/v1/signals",
  "status": 200,
  "duration_ms": 45,
  "client_ip": "192.168.1.1"
}
```

---

## 6. Alerting

**Approach:** Lightweight alert evaluator that:
1. Periodically checks metrics against thresholds
2. Uses existing notifiers (Telegram/Email) for delivery
3. Supports cooldown to prevent alert storms

**Built-in Metrics for Alerting:**

| Metric | Description |
|--------|-------------|
| `up` | 1 if healthy, 0 if not |
| `error_rate` | HTTP 5xx / total requests (last 5min) |
| `signals_24h` | Signals generated in last 24h |
| `analysis_failures_1h` | Failed analysis cycles in last hour |

**Alert Rule Configuration:**
```yaml
alerts:
  enabled: true
  check_interval: 60s
  rules:
    - name: high_error_rate
      expr: "error_rate > 0.05"
      for: 5m
      severity: warning
      message: "API error rate is {{ .Value | printf \"%.1f\" }}%"

    - name: api_down
      expr: "up == 0"
      for: 1m
      severity: critical
      message: "ATLAS API is not responding"

    - name: no_signals
      expr: "signals_24h == 0"
      for: 24h
      severity: info
      message: "No signals generated in 24 hours"
```

**Notification Flow:**
```
Alert Rule Triggered --> Cooldown Check --> Existing Notifier (Telegram/Email)
```

---

## 7. Architecture

```
+-------------+     +--------------+     +-------------+
|  API Server |---->| Metrics Mid. |---->| /metrics    |
+-------------+     +--------------+     +------+------+
                                                |
                    +--------------+            |
                    | Prometheus   |<-----------+
                    +------+-------+
                           |
                    +------v-------+     +-------------+
                    | Alert Eval.  |---->| Telegram/   |
                    +--------------+     | Email       |
                                         +-------------+
```

---

## 8. File Structure

```
internal/
├── metrics/
│   ├── metrics.go           # Prometheus registry, metric definitions
│   ├── collector.go         # Business metrics collector
│   └── middleware.go        # HTTP metrics + request logging middleware
├── alert/
│   ├── evaluator.go         # Alert rule evaluation engine
│   ├── rules.go             # Rule parsing and built-in metrics
│   └── evaluator_test.go
└── api/
    └── server.go            # Modify: add /metrics endpoint, wrap handlers

configs/
└── config.example.yaml      # Add metrics and alerts sections
```

---

## 9. Config Additions

```yaml
metrics:
  enabled: true
  path: "/metrics"

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

---

## 10. Implementation Order

| Task | Description | Dependencies |
|------|-------------|--------------|
| 1 | Metrics registry & definitions | None |
| 2 | HTTP metrics middleware | Task 1 |
| 3 | Request logging middleware | None |
| 4 | Business metrics collector | Task 1 |
| 5 | Alert evaluator with notifier integration | Task 1 |
| 6 | Config additions & server wiring | All above |

---

## 11. Dependencies

**New Go dependencies:**
- `github.com/prometheus/client_golang` - Prometheus client library
- `github.com/google/uuid` - Request ID generation

---

## 12. Testing

- Unit tests for metrics middleware
- Unit tests for alert evaluator
- Integration test: trigger alert -> verify notification sent
- Manual verification: Prometheus scrape endpoint

---

## 13. Out of Scope

- Distributed tracing (OpenTelemetry) - future milestone
- Rate limiting - future milestone
- Grafana dashboards - external configuration
- Prometheus/Alertmanager deployment - external infrastructure
