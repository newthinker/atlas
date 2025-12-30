// internal/api/response/response_test.go
package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/newthinker/atlas/internal/core"
)

func TestJSON_Success(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"hello": "world"}

	JSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json content type")
	}

	var resp SuccessResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data == nil {
		t.Error("expected data in response")
	}
	if resp.Meta.Timestamp.IsZero() {
		t.Error("expected timestamp in meta")
	}
}

func TestError_WithCoreError(t *testing.T) {
	w := httptest.NewRecorder()
	err := core.ErrConfigInvalid

	Error(w, http.StatusBadRequest, err)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp ErrorResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Code != "CONFIG_INVALID" {
		t.Errorf("expected CONFIG_INVALID, got %s", resp.Error.Code)
	}
}

func TestError_WithStandardError(t *testing.T) {
	w := httptest.NewRecorder()
	err := core.WrapError(core.ErrNoData, nil)

	Error(w, http.StatusNotFound, err)

	var resp ErrorResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Code != "NO_DATA" {
		t.Errorf("expected NO_DATA, got %s", resp.Error.Code)
	}
}
