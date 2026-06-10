# QA Code Review — 第一轮（常规）

- 范围：master `76a54b5..HEAD`（13 commits / 10 任务）
- 心智模型：Reality Checker（默认 NEEDS WORK）
- 日期：2026-06-10　审查人：qa-agent-1

## 0. 客观证据（实跑）
| 检查 | 结果 |
|---|---|
| `go build ./...` | rc=0 ✅ |
| `go vet ./...` | rc=0 ✅ |
| `go test ./...` | 全包 ok ✅ |
| `go test -race ./internal/app/... ./internal/broker/... ./internal/collector/` | rc=0 ✅（无数据竞争） |
| `gofmt -l` 全部改动文件 | 空 ✅ |

## 1. 正确性
- `internal/broker/paper/paper.go`：买卖现金/持仓/均价数学正确（BUY 加权均价 line 122-124，SELL 不改均价 line 129）；清仓删 position（135）。检查顺序 validate→price>0→lock→connected 合理。
- `internal/broker/execution.go`：ISSUE-3 修复确认——市价单 `OrderRequest.Price = price`（line 167），堵死 paper BUY 永拒缺陷。Execute 入口 `price<=0` 守卫（124）。
- `internal/collector/cache.go`：TTL + 容量 256 + 最旧淘汰（seq 单调）；命中/存储均 `cloneOHLCV` 返回副本，调用方无法污染共享底层数组（59,74,84）；错误不缓存（66）。逻辑正确。

## 2. 并发安全
- `app.runAnalysisCycle` 并行路径：`errgroup.SetLimit(workers)` 有界并发；Go 1.24 每轮独立 loop 变量，闭包捕获 `item` 安全；`analyzeSymbolSafe` panic 恢复隔离单 symbol 故障（287）。
- 共享组件均有锁：router(`sync.RWMutex`)、signal MemoryStore、strategy.Engine、PaperBroker、ExecutionManager。`a.executor`/`a.arbitrator` 读取均在 RLock 下快照（363,418）。
- `-race` 全绿佐证。✅

## 3. 错误处理 / 资源
- HTTP collector（ISSUE-1 全局复查）：eastmoney 4、lixinger 6、yahoo 2，共 **10 条 fetch 路径全部** `StatusCode != http.StatusOK → return error`，且 `defer resp.Body.Close()` 置于 Do/Get 错误检查之后、StatusCode 检查之前（无非 200 泄漏）。ISSUE-1 视为**已彻底解决**。✅
- arbitrate 超时：`context.WithTimeout` + `defer cancel()`（433-434），超时降级为原信号，无 goroutine 泄漏。

## 4. 接口一致性 / 设计偏差
- `maybeCache`（serve.go:278）正确避开包裹 `FundamentalCollector`（lixinger），防止 CachedCollector 仅嵌 `Collector` 隐藏扩展方法——设计注释充分。
- `SignalExecutor`/`orderExecutor` 定义在消费侧（ADR-2 依赖倒置），可注入 stub，符合 architecture-decisions。

## 5. 第一轮发现（详见 verdict）
- W1：生产链路 inert（Signal.Price 未填）。
- W2：执行不受 router cooldown 约束。
- W3：execution.mode="" 校验/运行不一致。
- I1/I2：见 verdict。
