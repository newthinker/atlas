# TASK-001 验证报告「do() 节流+退避核心」

- 验证者: test-agent-5(Reality Checker)
- commit: cf147dd
- assignment_epoch: 1
- 判定: **VERIFIED**
- 测试命令: `GOTOOLCHAIN=local go test ./internal/collector/yahoo/ -count=1`

## Done Criteria 覆盖矩阵

| # | 维度 | 完成标准 | 对应测试 | 证据 | 判定 |
|---|---|---|---|---|---|
| 1 | functional | 首响 429 → 退避重试成功,服务端恰收 2 请求 | TestDoRetries429ThenSucceeds | PASS;len(arrivals)==2,arrivals 为服务端 handler 内 time.Now() 真实到达计数 | PASS |
| 2 | functional | 首响 500 → 重试成功,恰 2 请求 | TestDoRetries5xxThenSucceeds | PASS;len(arrivals)==2 | PASS |
| 3 | functional | 429+Retry-After:0 且 backoffBase=2s 时总耗时 <1s(头优先于退避) | TestDoHonorsRetryAfterHeader | 重跑 3 次均 PASS,elapsed 0.00s « 1s(退避基数拉到 2s,若忽略头将 ≥2s) | PASS |
| 4 | functional | minInterval=80ms 时两请求到达间隔 ≥60ms | TestDoThrottlesConsecutiveRequests | 重跑 3 次均 PASS,用例耗时 0.08s,gap≥60ms | PASS |
| 5 | boundary | 既有 yahoo_test.go/eps_test.go 零修改全 PASS | 全量包回归 | git show cf147dd -- yahoo_test.go eps_test.go diff 为空;全量 ok(15.387s) | PASS |
| 6 | error_handling | 连续 429 → 恰 4 请求(1+3)后返回含 unexpected status: 429 错误,网络层错误不重试 | TestDoGivesUpAfterMaxRetries | PASS;len(arrivals)==4 且 strings.Contains(err, "unexpected status: 429");源码 client.Do err 直接 return 不重试 | PASS |
| 7 | non_functional (review) | 四常量为包内常量不进 config;仅 GET 生效;重试前排干关闭 body;与计划一致 | 代码审查 | 四常量均 const(minRequestInterval/retryBackoffBase/maxRetryAttempts/retryBudgetPerRun),不进 config;newRequest 仅构造 GET,do 注释声明;重试前 io.Copy(io.Discard, body) + Close() | PASS |

## 附加核对项(reviewer 增补)

1. 调用点替换(判据:除 do() 本体与 eps.go 外零残留): yahoo.go:120 的 client.Do 位于 do() 的 throttle+重试循环内,是包裹器实现本体(非业务调用点);FetchQuote(:216)/FetchHistory(:277)均已换 y.do;eps.go:56 仍 client.Do(TASK-002 范围,不作 reject 依据)。业务调用点零残留。PASS
2. 计时断言重跑 3 次全稳定,无 flaky。PASS
3. 请求计数真实性: newRetryServer 的 arrivals 在 handler 内 append(time.Now()),真数服务端到达数,非弱化为 err==nil。PASS
4. 既有测试零修改: git show cf147dd --stat 仅 yahoo.go + throttle_test.go 两文件,既有测试文件 diff 空。PASS

## 已知偏差(Leader 已裁定接受,仅记录)

既有 TestFetch_NonOKStatus 因 500 变为可重试,全量回归耗时 ~15.4s(其中该用例 ~14.5s);断言不变。治理留 QA 建议(可为该用例在测试内注入 maxRetries=0 或 minInterval=0 以缩短耗时)。

## 结论

7 条 DoD 全部有压倒性证据支撑,4 项附加核对全过。判定 VERIFIED。

---

## Rework 1 附录:QA WARNING「Retry-After cap」修复复验

- commit: 6288a48(修复 dev-13 完成;收尾由记录员 dev-agent-14 落盘——dev-13 连续 API 中断被 Leader 收回,epoch 2 机制防止了迟到双写)
- assignment_epoch: 2
- 复验判定: **VERIFIED**
- 命令: `GOTOOLCHAIN=local go test -race ./internal/collector/yahoo/ -count=1`

### fix_items 逐条核对

| # | fix_item | 证据 | 判定 |
|---|---|---|---|
| 1 | retryAfterWait 取 min(retryAfter, cap);cap=常量 60s | 源码新增常量 maxRetryAfterWait = 60*time.Second;retryAfterWait 签名加 maxWait 参数,返回 min(time.Duration(secs)*time.Second, maxWait);do 传入 y.maxRetryAfter | PASS |
| 2 | maxRetryAfter 仿 minInterval 模式(字段+NewWithBaseURLs 初始化+测试可覆盖) | struct 新增 maxRetryAfter 字段;NewWithBaseURLs 初始化 maxRetryAfter: maxRetryAfterWait;测试内 y.maxRetryAfter=100ms 覆盖 | PASS |
| 3 | TestDoRetryAfterCapped 真锚定(maxRetryAfter=100ms + Retry-After:1 → 总耗时 <500ms) | PASS;用例耗时 ~0.10s « 500ms;断言 elapsed>500ms 才 fail,若无 cap 会等满 1s;真测总耗时非 err==nil 假锚定;计时类重跑 3 次稳定(0.34s) | PASS |

### 回归核对

- 既有测试零修改: git show 6288a48 --stat 仅 yahoo.go + throttle_test.go 两文件;throttle_test.go 为纯追加(TestDoRetryAfterCapped + 注释),既有测试无改动行;retryAfterWait 签名变更未波及测试(测试不直接调用该包内函数)。
- TestDoHonorsRetryAfterHeader(Retry-After:0)不受 cap 影响仍 PASS(0 < 100ms,min 取 0)。
- -race 全量 ok 24.303s(慢速用例既定接受),既有 + 新增全 PASS(全包 48 个 RUN 含子测试)。
- grep 判据: 全包非测试文件 client.Do 只剩 yahoo.go:123(do() 本体一处,行号因新增常量 120→123);业务调用点零残留。

### Rework 结论

3 条 fix_items 全部满足,cap 逻辑真锚定生效,既有测试零回归,-race 通过。复验 **VERIFIED**。
