# TASK-002 验证报告「eps 接线 do 门 + 重试预算覆盖」

- 验证者: test-agent-5(Reality Checker)
- commit: fd0dedd(eps.go 一行 + throttle_test.go 追加 4 测试)
- assignment_epoch: 1
- 判定: **VERIFIED**
- 命令: `GOTOOLCHAIN=local go test -race ./internal/collector/yahoo/ -count=1`;`go build ./...`

## Done Criteria 覆盖矩阵

| # | 维度 | 完成标准 | 对应测试 | 证据 | 判定 |
|---|---|---|---|---|---|
| 1 | functional | eps 路径首响 429 → 重试成功解析 1 个 EPS 点(证明 eps 走 do 门) | TestFetchEPSHistoryRetries429 | PASS;len(pts)==1;FetchEPSHistory 已换 y.do | PASS |
| 2 | functional | retryBudget=1 序列 429/200/429 → 首次经重试成功、二次立即失败,服务端恰 3 请求 | TestDoRetryBudgetExhausted | PASS;len(arrivals)==3(真数到达),err 含 unexpected status: 429 | PASS |
| 3 | boundary | 既有 eps_test.go 零修改全 PASS | git diff + 全量回归 | git show fd0dedd --stat 仅 eps.go + throttle_test.go;eps_test.go diff 空;-race 全量 ok | PASS |
| 4 | boundary | 非法 Retry-After(garbage)→ 回退指数退避 | TestDoRetryAfterInvalidFallsBack | 重跑 3 次均 PASS;用例耗时 ~0.20s,断言 gap≥150ms(backoffBase=200ms);真测间隔非 err==nil 假锚定 | PASS |
| 5 | error_handling | 网络层错误不重试直接返回 err | TestDoNetworkErrorNoRetry | PASS;srv.Close() 后 fetch 触发连接层错误(非 HTTP 状态);断言 retryBudget==budgetBefore(读预算余量锚定未走重试路径) | PASS |
| 6 | non_functional (test) | 并发安全 -race 全 PASS + 跨包回归 | -race + build | -race 全量 ok 23.912s(慢速用例既定接受);go build ./... OK;collector/prism/valuation/cmd 显式包 build OK | PASS |
| 7 | non_functional (review) | 三调用点 FetchQuote/FetchHistory/FetchEPSHistory 均换 y.do,全包无残留 client.Do | grep | grep 全包非测试文件 client.Do 只剩 yahoo.go:120(do() 本体一处);eps.go:56 已接线 y.do;业务调用点零残留 | PASS |

## 增补用例锚定真实性核查(reviewer 重点)

1. TestDoRetryAfterInvalidFallsBack:断言两请求间隔 ≥150ms(backoffBase=200ms)。若把 garbage 当 0 则近乎瞬时会触发 fail —— 真锚定退避间隔,非假锚定 err==nil。计时类,重跑 3 次稳定。
2. TestDoNetworkErrorNoRetry:server 提前 Close 触发连接失败(网络层 err,非 HTTP 状态码);断言读 y.retryBudget 余量证明仅 1 次尝试未走重试。真锚定。
3. TestFetchEPSHistoryRetries429:eps 路径真走 do 门(429→重试→解析),断言 len(pts)==1。
4. TestDoRetryBudgetExhausted:arrivals 服务端 handler 内真数到达,恰 3 请求(预算耗尽绝不发第 4 个)。

## 越界核查

git show fd0dedd --stat 仅 eps.go + throttle_test.go 两文件,均在声明 package ./internal/collector/yahoo 内;既有 eps_test.go 零改动行。

## 结论

7 条 DoD 全部有压倒性证据(含 -race + 跨包 build),4 个增补用例锚定真实非假断言。判定 VERIFIED。Sprint 027 全部任务验证完毕,可启动 QA。
