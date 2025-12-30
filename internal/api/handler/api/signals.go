// internal/api/handler/api/signals.go
package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/storage/signal"
)

// SignalsHandler handles signal-related API requests.
type SignalsHandler struct {
	store signal.Store
}

// NewSignalsHandler creates a new signals handler.
func NewSignalsHandler(store signal.Store) *SignalsHandler {
	return &SignalsHandler{store: store}
}

// List returns signals matching query parameters.
func (h *SignalsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := signal.ListFilter{
		Symbol:   q.Get("symbol"),
		Strategy: q.Get("strategy"),
	}

	if action := q.Get("action"); action != "" {
		filter.Action = core.Action(action)
	}

	if from := q.Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			filter.From = t
		} else if t, err := time.Parse("2006-01-02", from); err == nil {
			filter.From = t
		}
	}

	if to := q.Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			filter.To = t
		} else if t, err := time.Parse("2006-01-02", to); err == nil {
			filter.To = t
		}
	}

	if limit := q.Get("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			filter.Limit = n
		}
	} else {
		filter.Limit = 50 // Default limit
	}

	if offset := q.Get("offset"); offset != "" {
		if n, err := strconv.Atoi(offset); err == nil {
			filter.Offset = n
		}
	}

	signals, err := h.store.List(context.Background(), filter)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err)
		return
	}

	count, _ := h.store.Count(context.Background(), filter)

	response.JSON(w, http.StatusOK, map[string]any{
		"signals": signals,
		"total":   count,
		"limit":   filter.Limit,
		"offset":  filter.Offset,
	})
}

// GetByID returns a single signal by ID.
func (h *SignalsHandler) GetByID(w http.ResponseWriter, r *http.Request, id string) {
	sig, err := h.store.GetByID(context.Background(), id)
	if err != nil {
		response.Error(w, http.StatusNotFound, err)
		return
	}

	response.JSON(w, http.StatusOK, sig)
}
