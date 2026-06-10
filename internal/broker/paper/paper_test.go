package paper

// Context Checkpoint: done_criteria → test mapping
// functional[0] "编译期断言 + New 未连接 + Connect 后 IsConnected" → TestInterfaceAssertion / TestConnectLifecycle
// functional[1] "买单立即成交, cash/position 变化, 成交价=req.Price, 无价格报错" → TestBuyOrderFills / TestBuyOrderRequiresPrice
// functional[2] "卖单成交, cash/position 变化, 清零后无持仓"        → TestSellOrderFills / TestSellClearsPosition
// functional[3] "Subscribe handler 回调含成交订单"                 → TestSubscribeHandlerCalled
// boundary[0]   "卖出>持仓报错, 状态不变"                         → TestSellExceedsPosition
// boundary[1]   "买入>现金报错, 状态不变"                         → TestBuyExceedsCash
// error[0]      "未Connect报错; 不存在订单ID报错"                  → TestPlaceOrderNotConnected / TestCancelGetNotFound
// non_func[0]   "并发无竞争 (-race)"                              → TestConcurrentAccess

import (
	"context"
	"sync"
	"testing"

	"github.com/newthinker/atlas/internal/broker"
	"github.com/newthinker/atlas/internal/core"
)

// functional[0]: compile-time interface assertion.
var _ broker.Broker = (*PaperBroker)(nil)

func connectedBroker(t *testing.T, cash float64) *PaperBroker {
	t.Helper()
	pb := New(cash)
	if err := pb.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return pb
}

func buyReq(symbol string, qty int64, price float64) broker.OrderRequest {
	return broker.OrderRequest{
		Symbol:   symbol,
		Market:   core.MarketUS,
		Side:     broker.OrderSideBuy,
		Type:     broker.OrderTypeMarket,
		Quantity: qty,
		Price:    price,
	}
}

func sellReq(symbol string, qty int64, price float64) broker.OrderRequest {
	r := buyReq(symbol, qty, price)
	r.Side = broker.OrderSideSell
	return r
}

// functional[0]
func TestConnectLifecycle(t *testing.T) {
	pb := New(1000)
	if pb.IsConnected() {
		t.Fatal("new broker should be disconnected")
	}
	if err := pb.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !pb.IsConnected() {
		t.Fatal("expected connected after Connect")
	}
	if err := pb.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if pb.IsConnected() {
		t.Fatal("expected disconnected after Disconnect")
	}
}

// functional[1]
func TestBuyOrderFills(t *testing.T) {
	pb := connectedBroker(t, 10000)
	ctx := context.Background()
	order, err := pb.PlaceOrder(ctx, buyReq("AAPL", 10, 100))
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if order.Status != broker.OrderStatusFilled {
		t.Errorf("status = %s, want FILLED", order.Status)
	}
	if order.FilledQuantity != 10 || order.AverageFillPrice != 100 {
		t.Errorf("fill = %d@%v, want 10@100", order.FilledQuantity, order.AverageFillPrice)
	}
	bal, err := pb.GetBalance(ctx)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal.Cash != 9000 {
		t.Errorf("cash = %v, want 9000", bal.Cash)
	}
	pos, err := pb.GetPosition(ctx, "AAPL")
	if err != nil {
		t.Fatalf("GetPosition: %v", err)
	}
	if pos.Quantity != 10 {
		t.Errorf("position qty = %d, want 10", pos.Quantity)
	}
}

// functional[1]
func TestBuyOrderRequiresPrice(t *testing.T) {
	pb := connectedBroker(t, 10000)
	_, err := pb.PlaceOrder(context.Background(), buyReq("AAPL", 10, 0))
	if err == nil {
		t.Fatal("expected error when request carries no price")
	}
}

// functional[2]
func TestSellOrderFills(t *testing.T) {
	pb := connectedBroker(t, 10000)
	ctx := context.Background()
	if _, err := pb.PlaceOrder(ctx, buyReq("AAPL", 10, 100)); err != nil {
		t.Fatalf("buy: %v", err)
	}
	if _, err := pb.PlaceOrder(ctx, sellReq("AAPL", 4, 110)); err != nil {
		t.Fatalf("sell: %v", err)
	}
	bal, _ := pb.GetBalance(ctx)
	// 10000 - 10*100 + 4*110 = 9000 + 440 = 9440
	if bal.Cash != 9440 {
		t.Errorf("cash = %v, want 9440", bal.Cash)
	}
	pos, err := pb.GetPosition(ctx, "AAPL")
	if err != nil {
		t.Fatalf("GetPosition: %v", err)
	}
	if pos.Quantity != 6 {
		t.Errorf("position qty = %d, want 6", pos.Quantity)
	}
}

// functional[2]
func TestSellClearsPosition(t *testing.T) {
	pb := connectedBroker(t, 10000)
	ctx := context.Background()
	if _, err := pb.PlaceOrder(ctx, buyReq("AAPL", 10, 100)); err != nil {
		t.Fatalf("buy: %v", err)
	}
	if _, err := pb.PlaceOrder(ctx, sellReq("AAPL", 10, 100)); err != nil {
		t.Fatalf("sell: %v", err)
	}
	if _, err := pb.GetPosition(ctx, "AAPL"); err == nil {
		t.Fatal("expected error / no position after clearing")
	}
	positions, err := pb.GetPositions(ctx)
	if err != nil {
		t.Fatalf("GetPositions: %v", err)
	}
	if len(positions) != 0 {
		t.Errorf("positions = %d, want 0", len(positions))
	}
}

// functional[3]
func TestSubscribeHandlerCalled(t *testing.T) {
	pb := connectedBroker(t, 10000)
	var got broker.OrderUpdate
	var called bool
	if err := pb.Subscribe(func(u broker.OrderUpdate) {
		got = u
		called = true
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	order, err := pb.PlaceOrder(context.Background(), buyReq("AAPL", 1, 100))
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if !called {
		t.Fatal("handler not called")
	}
	if got.Order.OrderID != order.OrderID {
		t.Errorf("handler order id = %s, want %s", got.Order.OrderID, order.OrderID)
	}
	if got.Order.Status != broker.OrderStatusFilled {
		t.Errorf("handler order status = %s, want FILLED", got.Order.Status)
	}
}

// boundary[0]
func TestSellExceedsPosition(t *testing.T) {
	pb := connectedBroker(t, 10000)
	ctx := context.Background()
	if _, err := pb.PlaceOrder(ctx, buyReq("AAPL", 5, 100)); err != nil {
		t.Fatalf("buy: %v", err)
	}
	balBefore, _ := pb.GetBalance(ctx)
	if _, err := pb.PlaceOrder(ctx, sellReq("AAPL", 10, 100)); err == nil {
		t.Fatal("expected error selling more than held")
	}
	balAfter, _ := pb.GetBalance(ctx)
	if balBefore.Cash != balAfter.Cash {
		t.Errorf("cash changed after rejected sell: %v -> %v", balBefore.Cash, balAfter.Cash)
	}
	pos, _ := pb.GetPosition(ctx, "AAPL")
	if pos.Quantity != 5 {
		t.Errorf("position qty = %d, want 5 (unchanged)", pos.Quantity)
	}
}

// boundary[1]
func TestBuyExceedsCash(t *testing.T) {
	pb := connectedBroker(t, 500)
	ctx := context.Background()
	if _, err := pb.PlaceOrder(ctx, buyReq("AAPL", 10, 100)); err == nil {
		t.Fatal("expected error buying beyond cash")
	}
	bal, _ := pb.GetBalance(ctx)
	if bal.Cash != 500 {
		t.Errorf("cash = %v, want 500 (unchanged)", bal.Cash)
	}
	if _, err := pb.GetPosition(ctx, "AAPL"); err == nil {
		t.Fatal("expected no position after rejected buy")
	}
}

// error[0]
func TestPlaceOrderNotConnected(t *testing.T) {
	pb := New(10000)
	if _, err := pb.PlaceOrder(context.Background(), buyReq("AAPL", 1, 100)); err == nil {
		t.Fatal("expected error when not connected")
	}
}

// error[0]
func TestCancelGetNotFound(t *testing.T) {
	pb := connectedBroker(t, 10000)
	ctx := context.Background()
	if err := pb.CancelOrder(ctx, "nope"); err == nil {
		t.Fatal("expected error cancelling unknown order")
	}
	if _, err := pb.GetOrder(ctx, "nope"); err == nil {
		t.Fatal("expected error getting unknown order")
	}
}

// non_func[0]
func TestConcurrentAccess(t *testing.T) {
	pb := connectedBroker(t, 1_000_000)
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = pb.PlaceOrder(ctx, buyReq("AAPL", 1, 100))
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = pb.GetPositions(ctx)
			_, _ = pb.GetBalance(ctx)
		}()
	}
	wg.Wait()
}

func TestNameAndMarkets(t *testing.T) {
	pb := New(1)
	if pb.Name() == "" {
		t.Error("Name should be non-empty")
	}
	if len(pb.SupportedMarkets()) == 0 {
		t.Error("SupportedMarkets should be non-empty")
	}
}

func TestNewDefaultCash(t *testing.T) {
	pb := connectedBroker(t, 0)
	bal, _ := pb.GetBalance(context.Background())
	if bal.Cash != defaultInitialCash {
		t.Errorf("cash = %v, want default %v", bal.Cash, defaultInitialCash)
	}
}

func TestConnectTwice(t *testing.T) {
	pb := connectedBroker(t, 1000)
	if err := pb.Connect(context.Background()); err != broker.ErrAlreadyConnected {
		t.Errorf("second Connect err = %v, want ErrAlreadyConnected", err)
	}
}

func TestSubscribeNilHandler(t *testing.T) {
	pb := connectedBroker(t, 1000)
	if err := pb.Subscribe(nil); err == nil {
		t.Fatal("expected error for nil handler")
	}
}

func TestUnsubscribe(t *testing.T) {
	pb := connectedBroker(t, 10000)
	var called bool
	_ = pb.Subscribe(func(broker.OrderUpdate) { called = true })
	if err := pb.Unsubscribe(); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if _, err := pb.PlaceOrder(context.Background(), buyReq("AAPL", 1, 100)); err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if called {
		t.Error("handler should not be called after Unsubscribe")
	}
}

func TestGetOpenOrdersEmpty(t *testing.T) {
	pb := connectedBroker(t, 10000)
	ctx := context.Background()
	if _, err := pb.PlaceOrder(ctx, buyReq("AAPL", 1, 100)); err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	open, err := pb.GetOpenOrders(ctx)
	if err != nil {
		t.Fatalf("GetOpenOrders: %v", err)
	}
	if len(open) != 0 {
		t.Errorf("open orders = %d, want 0 (paper fills immediately)", len(open))
	}
}

func TestGetOrderAfterFill(t *testing.T) {
	pb := connectedBroker(t, 10000)
	ctx := context.Background()
	placed, _ := pb.PlaceOrder(ctx, buyReq("AAPL", 1, 100))
	got, err := pb.GetOrder(ctx, placed.OrderID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if got.OrderID != placed.OrderID {
		t.Errorf("got %s, want %s", got.OrderID, placed.OrderID)
	}
}

func TestCancelFilledOrder(t *testing.T) {
	pb := connectedBroker(t, 10000)
	ctx := context.Background()
	placed, _ := pb.PlaceOrder(ctx, buyReq("AAPL", 1, 100))
	if err := pb.CancelOrder(ctx, placed.OrderID); err != broker.ErrOrderNotCancellable {
		t.Errorf("CancelOrder filled = %v, want ErrOrderNotCancellable", err)
	}
}

func TestInvalidRequest(t *testing.T) {
	pb := connectedBroker(t, 10000)
	if _, err := pb.PlaceOrder(context.Background(), buyReq("", 1, 100)); err == nil {
		t.Error("expected error for empty symbol")
	}
}

func TestReadOpsNotConnected(t *testing.T) {
	pb := New(10000)
	ctx := context.Background()
	if _, err := pb.GetPositions(ctx); err != broker.ErrNotConnected {
		t.Errorf("GetPositions err = %v", err)
	}
	if _, err := pb.GetPosition(ctx, "AAPL"); err != broker.ErrNotConnected {
		t.Errorf("GetPosition err = %v", err)
	}
	if _, err := pb.GetBalance(ctx); err != broker.ErrNotConnected {
		t.Errorf("GetBalance err = %v", err)
	}
	if _, err := pb.GetOpenOrders(ctx); err != broker.ErrNotConnected {
		t.Errorf("GetOpenOrders err = %v", err)
	}
	if _, err := pb.GetOrder(ctx, "x"); err != broker.ErrNotConnected {
		t.Errorf("GetOrder err = %v", err)
	}
	if err := pb.CancelOrder(ctx, "x"); err != broker.ErrNotConnected {
		t.Errorf("CancelOrder err = %v", err)
	}
}
