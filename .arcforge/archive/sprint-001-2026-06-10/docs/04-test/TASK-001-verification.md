# TASK-001 验证报告 — PaperBroker（内存模拟券商）

- 验证者: test-agent-2 (Reality Checker)
- 包: ./internal/broker/paper
- 验证命令（亲自复跑）: `go test -race -cover -count=1 ./internal/broker/paper/`
- 结果: **PASS**，21/21 用例通过，覆盖率 **97.6%**（与 dev 自报一致）
- gofmt/vet: discovery 声明 clean（覆盖率与测试结果实测复核通过）

## 判定: VERIFIED ✅

压倒性证据：所有 done_criteria 逐条有对应的、有真实断言的测试；-race 实跑无竞争；覆盖率 97.6%。

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 断言核验 | 判定 |
|---|---------|---------|---------|------|
| functional[0] | 编译期接口断言 + New 未连接 + Connect 后 IsConnected()==true | `var _ broker.Broker = (*PaperBroker)(nil)` (paper_test.go:23) + TestConnectLifecycle | 编译期断言随包编译通过；TestConnectLifecycle 断言 new→未连接、Connect→已连接、Disconnect→断开 | PASS |
| functional[1] | 买单立即成交、cash 减 qty*价、持仓增 qty、成交价=请求价、无价报错 | TestBuyOrderFills + TestBuyOrderRequiresPrice | cash 10000→9000、pos.Quantity=10、AverageFillPrice=100、Status=FILLED；price=0→error | PASS |
| functional[2] | 卖单成交 cash 增/持仓减；清零后无持仓 | TestSellOrderFills + TestSellClearsPosition | cash=9440(=9000+4*110)、pos=6；清零后 GetPosition 报错且 GetPositions 长度 0 | PASS |
| functional[3] | Subscribe 后成交触发 handler 且参数含成交订单 | TestSubscribeHandlerCalled | called==true、got.Order.OrderID==order.OrderID、Status==FILLED | PASS |
| boundary[0] | 卖出>持仓报错，状态不变 | TestSellExceedsPosition | sell 10>held 5→error；cash 不变；pos 仍为 5 | PASS |
| boundary[1] | 买入>现金报错，状态不变 | TestBuyExceedsCash | buy 1000>cash 500→error；cash 仍 500；无持仓 | PASS |
| error_handling[0] | 未 Connect 报错；不存在订单 ID 的 Cancel/Get 报错 | TestPlaceOrderNotConnected + TestCancelGetNotFound | 未连接 PlaceOrder→error；未知 ID Cancel/Get→error | PASS |
| non_functional[0] (verify_by:test) | 并发 PlaceOrder+GetPositions/GetBalance 在 -race 下无竞争 | TestConcurrentAccess | 50×2 goroutine 并发，-race 实跑通过无 DATA RACE | PASS |

## 附加观察（非阻塞）
- 额外覆盖：未连接读操作、二次 Connect、nil handler、Unsubscribe 后不回调、清算单 Cancel 返回 ErrOrderNotCancellable、默认现金回退等，测试充分非空洞。
- discovery 声明唯一未覆盖行为 CancelOrder 末尾不可达 `return nil`（接口契约完整性），合理，不影响 DoD。
