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

func TestLoggingMiddleware_LogsDuration(t *testing.T) {
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

	var logEntry map[string]any
	json.Unmarshal(buf.Bytes(), &logEntry)

	if _, ok := logEntry["duration_ms"]; !ok {
		t.Error("expected duration_ms in log entry")
	}
}

func TestLoggingMiddleware_LogsClientIP(t *testing.T) {
	var buf bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.InfoLevel)
	logger := zap.New(core)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := LoggingMiddleware(logger)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	var logEntry map[string]any
	json.Unmarshal(buf.Bytes(), &logEntry)

	if logEntry["client_ip"] != "10.0.0.1:54321" {
		t.Errorf("expected client_ip 10.0.0.1:54321, got %v", logEntry["client_ip"])
	}
}

func TestLoggingMiddleware_XForwardedFor(t *testing.T) {
	var buf bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.InfoLevel)
	logger := zap.New(core)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := LoggingMiddleware(logger)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	req.RemoteAddr = "10.0.0.1:54321"
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	var logEntry map[string]any
	json.Unmarshal(buf.Bytes(), &logEntry)

	if logEntry["client_ip"] != "203.0.113.50" {
		t.Errorf("expected client_ip 203.0.113.50, got %v", logEntry["client_ip"])
	}
}
