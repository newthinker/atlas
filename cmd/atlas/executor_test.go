package main

// Context Checkpoint: done_criteria → test mapping (TASK-003)
// functional[0]   "paper 模式 ExecutionManager 注入 app.SetExecutor 与 deps"  → TestWireExecution_PaperMode_InjectsBoth
// functional[1]   "BUY/SELL 信号按 DefaultSizePct 与余额下单提交"             → TestSignalExecutor_EndToEnd_BuyChangesBalanceAndPosition
// functional[2]   "e2e BUY 改余额/持仓；风险拒绝场景余额/持仓不变"             → TestSignalExecutor_EndToEnd_BuyChangesBalanceAndPosition / TestSignalExecutor_EndToEnd_RiskRejectionNoChange
// functional[3]   "非 paper 模式保持 warning，进程正常启动"                    → TestBuildExecution_NonPaper_NilNoError
// boundary[0]     "Enabled=false 不构造组件，deps.ExecutionManager 为 nil"      → TestBuildExecution_Disabled_Nil / TestWireExecution_Disabled_NoInject
// boundary[1]     "非 BUY/SELL（HOLD/WATCH）跳过不生成订单"                    → TestSignalExecutor_HoldSignalSkipped
// boundary[2]     "余额 0 或下单数量为 0 时跳过下单，不报错"                    → TestSignalExecutor_ZeroQuantitySkipped
// error_handling[0] "Execute 返回错误时记日志并返回 nil，不向分析循环传播"      → TestSignalExecutor_ExecuteErrorReturnsNil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/app"
	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/broker/paper"
	"github.com/newthinker/atlas/internal/config"
	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/strategy"
	"github.com/newthinker/atlas/internal/strategy/ma_crossover"
	"go.uber.org/zap"
)

// goldenCrossSignal runs the real ma_crossover strategy over data engineered to
// produce a golden cross, returning the resulting BUY signal. This exercises the
// genuine strategy→signal path (rather than a hand-built signal) so the test
// fails if the strategy stops pricing its signals (QA W1 regression guard).
func goldenCrossSignal(t *testing.T, symbol string) core.Signal {
	t.Helper()
	prices := []float64{100, 95, 90, 85, 80, 120} // decline then sharp spike => golden cross
	ohlcv := make([]core.OHLCV, len(prices))
	for i, p := range prices {
		ohlcv[i] = core.OHLCV{Symbol: symbol, Close: p, Time: time.Now().Add(time.Duration(-len(prices)+i) * 24 * time.Hour)}
	}
	signals, err := ma_crossover.New(2, 4).Analyze(strategy.AnalysisContext{Symbol: symbol, OHLCV: ohlcv, Now: time.Now()})
	if err != nil || len(signals) == 0 {
		t.Fatalf("ma_crossover did not produce a signal: err=%v signals=%d", err, len(signals))
	}
	if signals[0].Action != core.ActionBuy {
		t.Fatalf("expected BUY golden-cross signal, got %s", signals[0].Action)
	}
	return signals[0]
}

// --- test doubles ---------------------------------------------------------

// stubExecutor records the last Execute call and returns a configured result.
type stubExecutor struct {
	called int
	result *broker.ExecuteResult
	err    error
}

func (s *stubExecutor) Execute(ctx context.Context, signal *core.Signal, price float64) (*broker.ExecuteResult, error) {
	s.called++
	return s.result, s.err
}

// recordingSetter captures the executor passed to SetExecutor.
type recordingSetter struct {
	executor  app.SignalExecutor
	setCalled int
}

func (r *recordingSetter) SetExecutor(e app.SignalExecutor) {
	r.setCalled++
	r.executor = e
}

func paperBrokerConfig(mode string, sizePct, maxPositionPct float64) *config.Config {
	cfg := config.Defaults()
	cfg.Broker.Enabled = true
	cfg.Broker.Provider = "paper"
	cfg.Broker.Mode = mode
	cfg.Broker.Execution.Mode = "auto"
	cfg.Broker.Execution.DefaultSizePct = sizePct
	cfg.Broker.Risk.MaxPositionPct = maxPositionPct
	cfg.Broker.Risk.MaxOpenPositions = 20
	cfg.Broker.Risk.MaxDailyLossPct = 100
	return cfg
}

// --- buildExecution -------------------------------------------------------

// boundary[0]
func TestBuildExecution_Disabled_Nil(t *testing.T) {
	cfg := config.Defaults()
	cfg.Broker.Enabled = false

	em, err := buildExecution(context.Background(), cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if em != nil {
		t.Fatalf("expected nil ExecutionManager when broker disabled, got %v", em)
	}
}

// functional[3]
func TestBuildExecution_NonPaper_NilNoError(t *testing.T) {
	cfg := config.Defaults()
	cfg.Broker.Enabled = true
	cfg.Broker.Mode = "live"

	em, err := buildExecution(context.Background(), cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("non-paper mode must not error (keep warning + start), got: %v", err)
	}
	if em != nil {
		t.Fatalf("non-paper mode must not construct an ExecutionManager, got %v", em)
	}
}

// functional[0] partial: paper mode constructs a manager
func TestBuildExecution_PaperMode_Constructs(t *testing.T) {
	cfg := paperBrokerConfig("paper", 10, 50)

	em, err := buildExecution(context.Background(), cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("paper mode build: %v", err)
	}
	if em == nil {
		t.Fatal("expected non-nil ExecutionManager in paper mode")
	}
}

// --- wireExecution --------------------------------------------------------

// functional[0]
func TestWireExecution_PaperMode_InjectsBoth(t *testing.T) {
	cfg := paperBrokerConfig("paper", 10, 50)
	setter := &recordingSetter{}

	em, err := wireExecution(context.Background(), cfg, setter, zap.NewNop())
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	if em == nil {
		t.Fatal("ExecutionManager must be returned for deps injection")
	}
	if setter.setCalled != 1 {
		t.Fatalf("SetExecutor should be called once, got %d", setter.setCalled)
	}
	if setter.executor == nil {
		t.Fatal("an executor must be injected into the app")
	}
}

// boundary[0]
func TestWireExecution_Disabled_NoInject(t *testing.T) {
	cfg := config.Defaults()
	cfg.Broker.Enabled = false
	setter := &recordingSetter{}

	em, err := wireExecution(context.Background(), cfg, setter, zap.NewNop())
	if err != nil {
		t.Fatalf("wire: %v", err)
	}
	if em != nil {
		t.Fatalf("expected nil ExecutionManager when disabled, got %v", em)
	}
	if setter.setCalled != 0 {
		t.Fatalf("SetExecutor must not be called when broker disabled, got %d", setter.setCalled)
	}
}

// --- signalExecutor adapter: unit (stubbed) -------------------------------

// boundary[1]
func TestSignalExecutor_HoldSignalSkipped(t *testing.T) {
	stub := &stubExecutor{result: &broker.ExecuteResult{Success: true}}
	exec := newSignalExecutor(stub, zap.NewNop())

	err := exec.SubmitSignal(context.Background(), core.Signal{Symbol: "AAPL", Action: core.ActionHold, Price: 100})
	if err != nil {
		t.Fatalf("hold signal must not error: %v", err)
	}
	if stub.called != 0 {
		t.Fatalf("non-executable action must be skipped before Execute, called=%d", stub.called)
	}
}

// error_handling[0]
func TestSignalExecutor_ExecuteErrorReturnsNil(t *testing.T) {
	stub := &stubExecutor{err: errors.New("boom")}
	exec := newSignalExecutor(stub, zap.NewNop())

	err := exec.SubmitSignal(context.Background(), core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Price: 100})
	if err != nil {
		t.Fatalf("Execute error must not propagate to analysis loop, got: %v", err)
	}
	if stub.called != 1 {
		t.Fatalf("Execute should have been attempted once, got %d", stub.called)
	}
}

// boundary[2] (unit): Execute reports zero-quantity skip → adapter returns nil
func TestSignalExecutor_ZeroQuantitySkipped_Unit(t *testing.T) {
	stub := &stubExecutor{result: &broker.ExecuteResult{Success: false, Message: "calculated order quantity is zero or negative"}}
	exec := newSignalExecutor(stub, zap.NewNop())

	err := exec.SubmitSignal(context.Background(), core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Price: 100})
	if err != nil {
		t.Fatalf("zero-quantity skip must not error: %v", err)
	}
}

// --- signalExecutor adapter: end-to-end with real broker chain ------------

// buildChain wires a real paper-mode chain and the adapter for e2e assertions.
func buildChain(t *testing.T, sizePct, maxPositionPct float64) (*paper.PaperBroker, *signalExecutor) {
	t.Helper()
	cfg := paperBrokerConfig("paper", sizePct, maxPositionPct)

	pb := paper.New(100000)
	if err := pb.Connect(context.Background()); err != nil {
		t.Fatalf("connect paper broker: %v", err)
	}
	risk := broker.NewRiskChecker(broker.RiskConfig{
		MaxPositionPct:   cfg.Broker.Risk.MaxPositionPct,
		MaxDailyLossPct:  cfg.Broker.Risk.MaxDailyLossPct,
		MaxOpenPositions: cfg.Broker.Risk.MaxOpenPositions,
	}, pb)
	tracker := broker.NewPositionTracker(pb)
	em := broker.NewExecutionManager(broker.ExecutionConfig{
		Mode:           broker.ExecutionMode(cfg.Broker.Execution.Mode),
		DefaultSizePct: cfg.Broker.Execution.DefaultSizePct,
	}, pb, risk, tracker)
	return pb, newSignalExecutor(em, zap.NewNop())
}

// functional[1] + functional[2] — real strategy→signal→execution path.
// Drives a genuine ma_crossover BUY signal (priced at the latest close) through
// the chain, rather than a hand-built Price:100 signal, so the production wiring
// (strategy must price its signals) is actually exercised (QA W1 regression).
func TestSignalExecutor_EndToEnd_BuyChangesBalanceAndPosition(t *testing.T) {
	pb, exec := buildChain(t, 10, 50)
	ctx := context.Background()

	sig := goldenCrossSignal(t, "AAPL")
	if sig.Price <= 0 {
		t.Fatalf("strategy signal must be priced for execution, got Price=%v", sig.Price)
	}

	before, _ := pb.GetBalance(ctx)

	if err := exec.SubmitSignal(ctx, sig); err != nil {
		t.Fatalf("buy submit: %v", err)
	}

	after, _ := pb.GetBalance(ctx)
	if after.Cash >= before.Cash {
		t.Fatalf("expected cash to decrease after BUY: before=%v after=%v", before.Cash, after.Cash)
	}
	pos, err := pb.GetPosition(ctx, "AAPL")
	if err != nil {
		t.Fatalf("expected an AAPL position after BUY, got err: %v", err)
	}
	if pos.Quantity <= 0 {
		t.Fatalf("expected positive AAPL position after BUY, got %d", pos.Quantity)
	}
}

// QA W1 guard: an unpriced signal (Price=0, as ma_crossover produced before the
// fix) must place no order — execution rejects "price must be positive", the
// adapter swallows it and the analysis loop continues. Asserts the failure mode
// explicitly so a regression that drops Signal.Price is caught here rather than
// silently going inert in production.
func TestSignalExecutor_UnpricedSignalNotTraded(t *testing.T) {
	pb, exec := buildChain(t, 10, 50)
	ctx := context.Background()

	before, _ := pb.GetBalance(ctx)

	if err := exec.SubmitSignal(ctx, core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Price: 0}); err != nil {
		t.Fatalf("unpriced signal must not error (fail-safe): %v", err)
	}

	after, _ := pb.GetBalance(ctx)
	if after.Cash != before.Cash {
		t.Fatalf("unpriced signal must not place an order: cash before=%v after=%v", before.Cash, after.Cash)
	}
	if _, err := pb.GetPosition(ctx, "AAPL"); !errors.Is(err, broker.ErrPositionNotFound) {
		t.Fatalf("unpriced signal must not create a position, GetPosition err=%v", err)
	}
}

// functional[2] — risk rejection leaves balance and position untouched
func TestSignalExecutor_EndToEnd_RiskRejectionNoChange(t *testing.T) {
	// DefaultSizePct=10 of 100k = 10k order, positionPct=10% > MaxPositionPct=1% → rejected.
	pb, exec := buildChain(t, 10, 1)
	ctx := context.Background()

	before, _ := pb.GetBalance(ctx)

	err := exec.SubmitSignal(ctx, core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Price: 100})
	if err != nil {
		t.Fatalf("risk-rejected submit must not error: %v", err)
	}

	after, _ := pb.GetBalance(ctx)
	if after.Cash != before.Cash {
		t.Fatalf("risk rejection must not change cash: before=%v after=%v", before.Cash, after.Cash)
	}
	if _, err := pb.GetPosition(ctx, "AAPL"); !errors.Is(err, broker.ErrPositionNotFound) {
		t.Fatalf("risk rejection must not create a position, GetPosition err=%v", err)
	}
}

// boundary[2] — end-to-end zero quantity (tiny size) places no order, no error
func TestSignalExecutor_ZeroQuantitySkipped(t *testing.T) {
	// price 100 with sizePct producing <1 share: 0.00001% of 100k = 0.01 → /100 = 0 qty.
	pb, exec := buildChain(t, 0.00001, 50)
	ctx := context.Background()

	before, _ := pb.GetBalance(ctx)
	if err := exec.SubmitSignal(ctx, core.Signal{Symbol: "AAPL", Action: core.ActionBuy, Price: 100}); err != nil {
		t.Fatalf("zero-quantity submit must not error: %v", err)
	}
	after, _ := pb.GetBalance(ctx)
	if after.Cash != before.Cash {
		t.Fatalf("zero quantity must place no order: before=%v after=%v", before.Cash, after.Cash)
	}
}
