// internal/api/handler/api/backtest.go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/newthinker/atlas/internal/api/job"
	"github.com/newthinker/atlas/internal/api/response"
	"github.com/newthinker/atlas/internal/backtest"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
)

const backtestTimeout = 5 * time.Minute

// BacktestRequest is the request body for starting a backtest.
type BacktestRequest struct {
	Symbol   string         `json:"symbol"`
	Strategy string         `json:"strategy"`
	Start    string         `json:"start"`
	End      string         `json:"end"`
	Params   map[string]any `json:"params,omitempty"`
}

// BacktestHandler handles backtest API requests.
type BacktestHandler struct {
	jobStore   *job.Store
	backtester *backtest.Backtester
	strategies *strategy.Engine
}

// NewBacktestHandler creates a new backtest handler.
func NewBacktestHandler(
	jobStore *job.Store,
	backtester *backtest.Backtester,
	strategies *strategy.Engine,
) *BacktestHandler {
	return &BacktestHandler{
		jobStore:   jobStore,
		backtester: backtester,
		strategies: strategies,
	}
}

// Create starts a new backtest job.
func (h *BacktestHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req BacktestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigInvalid, err))
		return
	}

	// Validate required fields
	if req.Symbol == "" || req.Strategy == "" {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigMissing, nil))
		return
	}

	// Parse dates
	start, err := time.Parse("2006-01-02", req.Start)
	if err != nil {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigInvalid, err))
		return
	}
	end, err := time.Parse("2006-01-02", req.End)
	if err != nil {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrConfigInvalid, err))
		return
	}

	// Find strategy
	strat, ok := h.strategies.Get(req.Strategy)
	if !ok {
		response.Error(w, http.StatusBadRequest,
			core.WrapError(core.ErrStrategyFailed, nil))
		return
	}

	// Create job
	j := h.jobStore.Create("backtest")

	// Copy values before starting goroutine to avoid race
	jobID := j.ID
	status := j.Status

	// Run backtest in background
	go h.runBacktest(jobID, strat, req.Symbol, start, end)

	response.JSON(w, http.StatusAccepted, map[string]any{
		"job_id": jobID,
		"status": status,
	})
}

// runBacktest executes the backtest and updates job status.
func (h *BacktestHandler) runBacktest(
	jobID string,
	strat strategy.Strategy,
	symbol string,
	start, end time.Time,
) {
	// Mark as running
	h.jobStore.Update(jobID, func(j *job.Job) {
		j.Status = job.StatusRunning
	})

	// Run backtest
	ctx, cancel := context.WithTimeout(context.Background(), backtestTimeout)
	defer cancel()
	result, err := h.backtester.Run(ctx, strat, symbol, start, end)

	if err != nil {
		h.jobStore.Update(jobID, func(j *job.Job) {
			j.Status = job.StatusFailed
			j.Error = core.WrapError(core.ErrStrategyFailed, err)
		})
		return
	}

	h.jobStore.Update(jobID, func(j *job.Job) {
		j.Status = job.StatusComplete
		j.Progress = 100
		j.Result = result
	})
}

// GetStatus returns the status of a backtest job.
func (h *BacktestHandler) GetStatus(w http.ResponseWriter, r *http.Request, jobID string) {
	j, err := h.jobStore.Get(jobID)
	if err != nil {
		response.Error(w, http.StatusNotFound, err)
		return
	}

	resp := map[string]any{
		"job_id":   j.ID,
		"status":   j.Status,
		"progress": j.Progress,
	}

	if j.Status == job.StatusComplete {
		resp["result"] = j.Result
	}
	if j.Status == job.StatusFailed && j.Error != nil {
		resp["error"] = map[string]string{
			"code":    j.Error.Code,
			"message": j.Error.Message,
		}
	}

	response.JSON(w, http.StatusOK, resp)
}
