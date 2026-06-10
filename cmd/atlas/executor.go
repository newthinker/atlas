package main

import (
	"context"
	"fmt"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/broker/paper"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	"go.uber.org/zap"
)

// orderExecutor is the minimal slice of *broker.ExecutionManager that the
// signal adapter depends on. Defined here (consumer side) so the adapter can be
// unit-tested with a stub, while production wiring injects the real manager.
type orderExecutor interface {
	Execute(ctx context.Context, signal *core.Signal, price float64) (*broker.ExecuteResult, error)
}

// executorSetter is satisfied by *app.App; it lets wireExecution be tested
// without constructing a full App.
type executorSetter interface {
	SetExecutor(app.SignalExecutor)
}

// signalExecutor adapts core.Signal (from the analysis loop) to the broker
// ExecutionManager. It implements app.SignalExecutor.
//
// The ExecutionManager already maps the signal action to an order side, sizes
// the order from ExecutionConfig.DefaultSizePct and the account balance, runs
// risk checks and places the order. This adapter therefore only bridges the two
// layers: it skips non-actionable signals early and ensures execution problems
// never propagate back into the analysis loop.
type signalExecutor struct {
	exec orderExecutor
	log  *zap.Logger
}

// newSignalExecutor builds the adapter around an order executor.
func newSignalExecutor(exec orderExecutor, log *zap.Logger) *signalExecutor {
	return &signalExecutor{exec: exec, log: log}
}

// SubmitSignal submits a routed signal for execution. It always returns nil:
// execution failures and rejections are logged but must not interrupt the
// analysis cycle (done_criteria error_handling[0]).
func (s *signalExecutor) SubmitSignal(ctx context.Context, sig core.Signal) error {
	if !isExecutableAction(sig.Action) {
		s.log.Debug("skipping non-executable signal",
			zap.String("symbol", sig.Symbol),
			zap.String("action", string(sig.Action)),
		)
		return nil
	}

	result, err := s.exec.Execute(ctx, &sig, sig.Price)
	if err != nil {
		s.log.Warn("signal execution failed",
			zap.String("symbol", sig.Symbol),
			zap.String("action", string(sig.Action)),
			zap.Error(err),
		)
		return nil
	}
	if !result.Success {
		s.log.Info("signal not executed",
			zap.String("symbol", sig.Symbol),
			zap.String("reason", result.Message),
		)
		return nil
	}

	s.log.Info("signal executed",
		zap.String("symbol", sig.Symbol),
		zap.String("message", result.Message),
	)
	return nil
}

// isExecutableAction reports whether a signal action warrants an order.
// Non-directional actions (hold/watch/etc.) are skipped (boundary[1]).
func isExecutableAction(a core.Action) bool {
	switch a {
	case core.ActionBuy, core.ActionStrongBuy, core.ActionSell, core.ActionStrongSell:
		return true
	default:
		return false
	}
}

// buildExecution constructs the paper-mode execution chain
// (PaperBroker → RiskChecker → PositionTracker → ExecutionManager).
//
// It returns (nil, nil) when the broker is disabled or configured for a mode
// other than paper — only paper trading is wired in this sprint; other
// providers/modes keep the existing warning behaviour and let the process start
// normally (boundary[0], functional[3]).
func buildExecution(ctx context.Context, cfg *config.Config, log *zap.Logger) (*broker.ExecutionManager, error) {
	if !cfg.Broker.Enabled {
		return nil, nil
	}
	if cfg.Broker.Mode != "paper" {
		log.Warn("broker enabled but only paper mode is wired; execution disabled",
			zap.String("provider", cfg.Broker.Provider),
			zap.String("mode", cfg.Broker.Mode),
			zap.String("execution_mode", cfg.Broker.Execution.Mode),
		)
		return nil, nil
	}

	pb := paper.New(0) // 0 → broker's default initial cash
	if err := pb.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connecting paper broker: %w", err)
	}

	risk := broker.NewRiskChecker(broker.RiskConfig{
		MaxPositionPct:   cfg.Broker.Risk.MaxPositionPct,
		MaxDailyLossPct:  cfg.Broker.Risk.MaxDailyLossPct,
		MaxOpenPositions: cfg.Broker.Risk.MaxOpenPositions,
	}, pb)
	tracker := broker.NewPositionTracker(pb)
	execManager := broker.NewExecutionManager(broker.ExecutionConfig{
		Mode:           broker.ExecutionMode(cfg.Broker.Execution.Mode),
		BatchTime:      cfg.Broker.Execution.BatchTime,
		DefaultSizePct: cfg.Broker.Execution.DefaultSizePct,
	}, pb, risk, tracker)

	log.Info("paper-mode execution chain wired",
		zap.String("execution_mode", cfg.Broker.Execution.Mode),
		zap.Float64("default_size_pct", cfg.Broker.Execution.DefaultSizePct),
	)
	return execManager, nil
}

// wireExecution builds the execution chain and, when present, injects the
// signal adapter into the app via SetExecutor. It returns the ExecutionManager
// (nil when disabled/non-paper) so the caller can also expose it through the API
// dependencies (functional[0]).
func wireExecution(ctx context.Context, cfg *config.Config, setter executorSetter, log *zap.Logger) (*broker.ExecutionManager, error) {
	execManager, err := buildExecution(ctx, cfg, log)
	if err != nil {
		return nil, err
	}
	if execManager == nil {
		return nil, nil
	}
	setter.SetExecutor(newSignalExecutor(execManager, log))
	return execManager, nil
}
