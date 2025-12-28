package strategy

import (
	"context"
	"sync"

	"github.com/newthinker/atlas/internal/core"
)

// Engine manages and runs strategies
type Engine struct {
	mu         sync.RWMutex
	strategies map[string]Strategy
}

// NewEngine creates a new strategy engine
func NewEngine() *Engine {
	return &Engine{
		strategies: make(map[string]Strategy),
	}
}

// Register adds a strategy to the engine
func (e *Engine) Register(s Strategy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.strategies[s.Name()] = s
}

// Get retrieves a strategy by name
func (e *Engine) Get(name string) (Strategy, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.strategies[name]
	return s, ok
}

// GetAll returns all registered strategies
func (e *Engine) GetAll() []Strategy {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Strategy, 0, len(e.strategies))
	for _, s := range e.strategies {
		result = append(result, s)
	}
	return result
}

// Analyze runs all strategies on the given context
func (e *Engine) Analyze(ctx context.Context, analysisCtx AnalysisContext) ([]core.Signal, error) {
	e.mu.RLock()
	strategies := make([]Strategy, 0, len(e.strategies))
	for _, s := range e.strategies {
		strategies = append(strategies, s)
	}
	e.mu.RUnlock()

	var allSignals []core.Signal

	for _, s := range strategies {
		select {
		case <-ctx.Done():
			return allSignals, ctx.Err()
		default:
		}

		signals, err := s.Analyze(analysisCtx)
		if err != nil {
			// Log error but continue with other strategies
			continue
		}

		// Add strategy name to signals
		for i := range signals {
			signals[i].Strategy = s.Name()
		}

		allSignals = append(allSignals, signals...)
	}

	return allSignals, nil
}

// AnalyzeWithStrategies runs specific strategies
func (e *Engine) AnalyzeWithStrategies(ctx context.Context, analysisCtx AnalysisContext, strategyNames []string) ([]core.Signal, error) {
	var allSignals []core.Signal

	for _, name := range strategyNames {
		select {
		case <-ctx.Done():
			return allSignals, ctx.Err()
		default:
		}

		s, ok := e.Get(name)
		if !ok {
			continue
		}

		signals, err := s.Analyze(analysisCtx)
		if err != nil {
			continue
		}

		for i := range signals {
			signals[i].Strategy = s.Name()
		}

		allSignals = append(allSignals, signals...)
	}

	return allSignals, nil
}
