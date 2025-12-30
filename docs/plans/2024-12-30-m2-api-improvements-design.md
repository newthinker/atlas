# M2: API Improvements Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:writing-plans to create implementation plan from this design.

**Goal:** Complete the API layer with JSON REST APIs for external clients and HTMX endpoints for the dashboard.

**Date:** 2024-12-30

---

## 1. Overview

**Scope:**
- JSON REST APIs at `/api/v1/*` for external clients
- HTMX endpoints at `/api/htmx/*` for dashboard
- Read operations + action triggers (backtest, analysis cycle)

**Key Decisions:**
- API Key authentication (header-based)
- Async with polling for long-running operations
- Structured error responses using M1 error types
- Full control over backtest parameters

---

## 2. Authentication

```
X-API-Key: <configured-key>
```

- API key configured in `config.yaml` under `server.api_key`
- Middleware validates on all `/api/v1/*` routes
- HTMX endpoints exempt (served from same origin)
- Missing/invalid key returns `401 Unauthorized`

---

## 3. Response Format

**Success:**
```json
{
  "data": {...},
  "meta": {"timestamp": "2024-12-30T10:00:00Z"}
}
```

**Error (using M1 error types):**
```json
{
  "error": {
    "code": "BACKTEST_FAILED",
    "message": "strategy analysis failed",
    "cause": "insufficient data for analysis"
  }
}
```

---

## 4. API Endpoints

### 4.1 Signals API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/signals` | List signals with filters |
| GET | `/api/v1/signals/{id}` | Get single signal by ID |

**Query Parameters for list:**
- `symbol` - filter by symbol (e.g., AAPL)
- `strategy` - filter by strategy name
- `action` - filter by action (BUY, SELL, HOLD)
- `from` - start date (ISO 8601)
- `to` - end date (ISO 8601)
- `limit` - max results (default 50)
- `offset` - pagination offset

### 4.2 Watchlist API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/watchlist` | List all watched symbols |
| POST | `/api/v1/watchlist` | Add symbol |
| DELETE | `/api/v1/watchlist/{symbol}` | Remove symbol |

**POST body:**
```json
{"symbol": "AAPL", "market": "US"}
```

### 4.3 Backtest API (Async)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/backtest` | Start backtest job |
| GET | `/api/v1/backtest/{job_id}` | Get job status/results |

**POST body:**
```json
{
  "symbol": "AAPL",
  "strategy": "ma_crossover",
  "start": "2023-01-01",
  "end": "2024-01-01",
  "params": {
    "short_period": 10,
    "long_period": 50
  }
}
```

**Response states:**
- `pending` - Job queued
- `running` - Job in progress (includes `progress: 0-100`)
- `complete` - Job finished (includes `result: {...}`)
- `failed` - Job failed (includes `error: {...}`)

### 4.4 Analysis Trigger API

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/analysis/run` | Trigger analysis cycle |

**Response:**
```json
{"data": {"triggered": true, "symbols_count": 15}}
```

---

## 5. HTMX Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/htmx/signals` | Recent signals table rows |
| GET | `/api/htmx/watchlist` | Watchlist table |
| POST | `/api/htmx/watchlist` | Add symbol, return updated table |
| DELETE | `/api/htmx/watchlist/{symbol}` | Remove, return updated table |
| GET | `/api/htmx/backtest/form` | Backtest form with strategy options |
| POST | `/api/htmx/backtest` | Submit backtest, return progress UI |
| GET | `/api/htmx/backtest/{id}` | Poll for result, return result card |

---

## 6. Async Job System

### 6.1 Job Structure

```go
type Job struct {
    ID        string
    Type      string      // "backtest"
    Status    string      // pending, running, complete, failed
    Progress  int         // 0-100
    Result    interface{} // BacktestResult when complete
    Error     *core.Error
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### 6.2 Job Store

- In-memory store (similar to signal store from M1)
- Jobs expire after 1 hour (configurable)
- Max 100 concurrent jobs (oldest evicted when full)
- Thread-safe with RWMutex

### 6.3 Execution Flow

1. POST creates job with `pending` status
2. Returns immediately with `job_id`
3. Background goroutine picks up job, sets `running`
4. Updates `progress` periodically during execution
5. Sets `complete` or `failed` when done
6. Client polls GET until terminal state

---

## 7. File Structure

```
internal/api/
├── server.go              # Modify: inject dependencies, add middleware
├── middleware/
│   └── auth.go            # NEW: API key validation middleware
├── handler/
│   ├── api/               # NEW: JSON API handlers
│   │   ├── signals.go
│   │   ├── watchlist.go
│   │   ├── backtest.go
│   │   └── analysis.go
│   └── web/               # Existing HTMX handlers (modify)
├── response/
│   └── response.go        # NEW: JSON/error response helpers
└── job/
    ├── store.go           # NEW: In-memory job store
    └── runner.go          # NEW: Background job executor
```

---

## 8. Dependencies

**Inject into Server:**
- `signal.Store` - for listing signals (from M1)
- `*app.App` - for watchlist access and triggering analysis
- `*backtest.Backtester` - for running backtests
- `job.Store` - for async job management

**Config additions:**
```yaml
server:
  api_key: "your-secret-key"  # NEW
  job_ttl_hours: 1            # NEW
  max_jobs: 100               # NEW
```

---

## 9. Implementation Order

1. Response helpers & middleware (no deps)
2. Job store (no deps)
3. Signals API (depends on signal.Store)
4. Watchlist API (depends on app.App)
5. Backtest API + job runner (depends on backtester, job store)
6. Analysis trigger API (depends on app.App)
7. HTMX endpoints (depends on all above)
8. Config additions + server wiring

---

## 10. Testing Approach

- Table-driven tests for each handler
- Mock dependencies (signal store, app, backtester)
- Test both success and error cases
- Test middleware auth separately
- HTTP test recorder for response validation

---

## 11. Out of Scope

- Rate limiting (M3)
- Request logging/tracing (M3)
- WebSocket for real-time updates (future)
- Persistent job storage (future - SQLite/Redis)
