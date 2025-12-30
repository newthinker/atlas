// internal/api/response/response.go
package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// Meta contains response metadata.
type Meta struct {
	Timestamp time.Time `json:"timestamp"`
}

// SuccessResponse is the standard success response format.
type SuccessResponse struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Cause   string `json:"cause,omitempty"`
}

// ErrorResponse is the standard error response format.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// JSON writes a success response with data.
func JSON(w http.ResponseWriter, status int, data any) {
	resp := SuccessResponse{
		Data: data,
		Meta: Meta{Timestamp: time.Now().UTC()},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// Error writes an error response.
func Error(w http.ResponseWriter, status int, err error) {
	detail := ErrorDetail{
		Code:    "INTERNAL_ERROR",
		Message: "an internal error occurred",
	}

	var coreErr *core.Error
	if errors.As(err, &coreErr) {
		detail.Code = coreErr.Code
		detail.Message = coreErr.Message
		if coreErr.Cause != nil {
			detail.Cause = coreErr.Cause.Error()
		}
	}

	resp := ErrorResponse{Error: detail}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
