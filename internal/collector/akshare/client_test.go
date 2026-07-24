package akshare

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Context Checkpoint: done_criteria → test mapping
//   error[0] 非 200 响应→error 含状态码与 body 片段 → TestGetHTTPError
func TestGetHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("akshare internal boom"))
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.FetchStockValuationSeries("600519.SH", "CN_A",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC))
	assert.ErrorContains(t, err, "500")
	assert.ErrorContains(t, err, "akshare internal boom")
}
