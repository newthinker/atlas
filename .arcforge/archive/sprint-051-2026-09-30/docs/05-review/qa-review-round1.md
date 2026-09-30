# QA Review Round 1 — Tiingo 美股备用数据源（qa-tg）

- 审查对象：`git diff 8d1c6cc7edf573c9879b53125f486eb6d515f01f b3232d6f3e2b07610bafe31e7381add623fdb442`（25 文件，+1799/−37），分支 feature/tiingo-source，审查时主仓库 HEAD = b3232d6f3e2b07610bafe31e7381add623fdb442
- 依据：spec docs/superpowers/specs/2026-09-30-tiingo-source-design.md、AD-1..23、9 份 discovery 与 docs/04-test/*-verification.md
- 第二轮 lens（只读子代理，结论落 scratchpad）：qa-tg-skeptic.md / qa-tg-architect.md / qa-tg-minimalist.md
- 跨模型：codex CLI 试一次，`perl alarm 40` 截断（rc=142），40 秒内仍在读仓库、未产出结论 ⇒ **降级为纯 Claude 跨视角**

## 最终判定：PASS（代码层无需退回 dev）＋ 1 条 Leader 侧流程 WARNING 待闭合

- CRITICAL：0
- WARNING：3 条。W1 是流程问题，需要 Leader 处理；W2、W3 是风险，需要人类确认是否接受。三条都**不需要 dev 返工**，fix_items 为空
- LOW：14 条，建议列入 final-report 或留给二期
- 对抗式 verdict：**PASS**。三个 lens 都没有 high-severity 发现；lens 之间也没有冲突，各自的 WARNING 在上面合并后分级

## 本体实测证据（HEAD b3232d6）

| 检查 | 结果 |
|---|---|
| `go test -count=1 -race ./internal/collector/tiingo/ ./internal/collector/ ./internal/collector/policy/ ./internal/prism/ ./internal/app/` | 全部 ok |
| `go test -count=1 ./cmd/atlas/` | ok（未加 -shuffle，理由见 AD-16） |
| `go test -tags integration -run Tiingo` | 两条都 SKIP，原因是环境里没有 token（AD-9） |
| `go vet` tiingo/collector/prism/cmd/atlas | 无输出 |
| `gofmt -l` | 只列出了 2 个文件（prism/sankey/template_test.go、cmd/atlas/backtest_test.go），都不在本次 diff 里，属于既有问题 |

## Leader 六个重点逐项结论

1. **安全，PASS。**
   - token 只放在 `Authorization` 请求头里（client.go:101）。URL 只含 startDate（client.go:95-96），所以 `*url.Error` 里不会有 token。
   - 所有出口都经过 `wrapErr`，用 %v 断开错误链（client.go:59）。statusErr 在截断之前先做了脱敏（client.go:135-137），TASK-004 的修复还在。
   - **JSON/URL 转义形式匹配不到 key，这一点不需要修。** 理由如下：
     - Tiingo token 是 hex 字符串，JSON 和 URL 转义都不会改动字母和数字，所以转义形态在现实中不会出现。
     - detail 分支走的是 `json.Unmarshal`，`\uXXXX` 形式会先被还原成明文，再参与替换。
   - 残余的低概率面列在 L4（大小写）和 L5（重定向）。
2. **价格口径、直连、配额，PASS。**
   - 拆股折算的累乘方向正确：先按当前 cum 输出，再乘上本行的因子，也就是「t 之后、不含 t」（normalize.go:52-74）。请求不带 endDate（client.go:94）。
   - `Proxy=nil` 设置在 Clone 出来的 Transport 上（client.go:47-48）。
   - `tiingo.daily` 已登记进内置表（policy.go:89-90），生产入口的装配顺序已查实：
     - serve：serve.go:85 `initPolicyGate` 在 serve.go:108 `buildCollectors` 之前。
     - prism：prism.go:185 `loadConfigOrDefaults`（其中 export_ohlcv.go:297 调 initPolicyGate）在 prism.go:212 `usPriceHops` 之前。
     - client 在构造时快照 `policy.Default()`（client.go:53），所以拿到的是带文件账本的 Gate。
3. **Registry 顺序，PASS（风险见 W3、L9、L10）。**
   - 注册顺序是 yahoo→eastmoney→crypto→tushare→baostock→tiingo→qlib。
   - 在当前生产配置下（yahoo 开启），buildArbitrator 的 US 市场取 yahoo，backtest `collectors[0]` 也是 yahoo，都比改动前的随机选择更确定。
   - api/server.go:183 按 GetAll 的顺序重新注册，顺序不变。
   - 只有当 yahoo 关闭时，tiingo 才会占到 US 位（W3）。
4. **P17 文案回归，PASS。**
   - 只配 twelvedata 一跳时，成功和失败两种文案都与改动前逐字一致：`"%s: yahoo price failed (%v)" + ", twelvedata fallback ok"`，以及 `"price history: %v; twelvedata fallback: %v"`。
   - hops 为空时仍然用 `%w`（refresh.go:323-325）。旧实现本来就是 %v 断链，所以改用 `errors.New` 不会丢失错误链。
5. **存活变异，都不构成 WARNING，记为 LOW（L11–L13）。** 理由是代码本身正确，只是缺测试。
   - `prev != 0` 守卫存在（collector.go:71-73）。它一旦被删，`ChangePercent` 会变成 ±Inf，snapshot（snapshot.go:124）会把它带进 JSON。有真实后果，但触发需要 Tiingo 返回 close=0。
   - `Symbol` 取的是入参（collector.go:66）。
   - M9/C5 装配壳在 diff 里已经确认传参正确。
6. **与 spec 的偏离：AD-19/20/22 覆盖了正则和 P17，AD-15/21 覆盖了 §3.6 日志订正没做这件事。** 另有三处没有登记（L6–L8），都是低风险，方向也更安全。

## WARNING

### W1 [WARNING·流程·owner=Leader] AD-23 约定的 code-simplifier 统一补跑，在 git 历史里找不到证据
- 证据：`git log --oneline 8d1c6cc..b3232d6 | grep -iE 'simplif|chore|refactor'` 只命中 `e11918a refactor(TASK-001)`，那是 TASK-001 本身的功能改动。AD-23 写的是「由 Leader 在 QA 前对整条分支 diff 统一补跑」，用户全局规范也要求「提交/PR 前必须跑 code-simplifier」。
- 处置：不需要 dev 返工。Leader 补跑一次：
  - 有改动：按 AD-23 单独提交，复跑受影响的包，并通知 QA 复核这部分 diff。
  - 零改动：在 final-report 里写明「已跑、零改动」，附上子代理结论文件路径。
- 在这一条闭合之前，本报告的 PASS 只针对代码层。

### W2 [WARNING·接受风险·需人类确认] 实际配额消耗是 spec 估算的数倍，而且失败的请求不缓存，会反复扣配额
- 位置：policy/gate.go:171-178（`takeQuota` 在 `fn` 之前执行，失败时 `return zero, err`，不写缓存）；client.go:82（缓存键为 `symbol|start|end`）；snapshot.go:116-121 与 :131-134（同一标的先调 FetchQuote，其 start 为 now−10d，再调 FetchHistory，其 start 为 snapshotHistoryStart）；app 分析循环再用第三个 start。
- 描述：yahoo 故障时，每个美股标的在每个 TTL 周期里最多占 3 个缓存键，也就是 3 次配额，默认 TTL 是全局 5m，6h 只是示例注释里的建议。Tiingo 返回 404 或 4xx 的标的每个周期都会重新扣配额。serve 和 prism 共用同一份账本（两个 plist 的 WorkingDirectory 相同，由 lens 核实），prism 那一跳可能被饿死，然后落到 twelvedata。
- 与 AD-21 的关系：AD-21 已经接受「容量不足（Yahoo 故障时 serve 一周期 ≈60 次）」，但**没有写出 ×3 的键乘数，也没有写出「永久 404 每周期都扣」**。
- 建议：请人类确认是否接受。二期可以做：① 对 404 做负缓存；② 运行时配置直接写上 `collector.topics."tiingo.daily".ttl: 6h`。这属于上线配置，由人类执行（AD-9）。

### W3 [WARNING·潜在·配置相关·需人类确认] yahoo 关闭、tiingo 开启、arbitrator 开启时，US 市场上下文由 tiingo 提供，每次仲裁都会消耗配额
- 位置：serve.go:331-337（每个市场取第一个支持它的 collector）；internal/context/market.go:74（US 取 SPY）；collectors.go:101-104（tiingo 排在 qlib 之前）。
- 描述：SPY 能通过 `Supported`，所以每次 Arbitrate 都会发一次请求（有 5m 缓存）。改动前，这个配置下 US 的位置属于 qlib 或者为空。所以这是**启用 tiingo 新带来的消耗路径**，而不只是「随机变成确定」。当前 runtime 配置里 yahoo 是开启的，不会触发。
- 建议：可以在 AD 里补一条「接受」，并在 config.example 里注明；也可以在后续让 buildArbitrator 跳过「只做兜底」的源。不作为本 sprint 的退回理由。

## LOW

| # | 文件:行 | 描述 | 建议 |
|---|---|---|---|
| L1 | normalize.go:36-43 | 坏日期的行被整行丢弃，它的 splitFactor 也跟着丢了。实测：坏日期行带 sf=10 时，前面的价格错了 10 倍，而 err=nil。这与同一段注释「缺价行保留因子」的原则不一致。spec §3.1 字面上允许跳过这种行 | 坏日期行带 sf≠1 时整段失败 |
| L2 | normalize.go:46-74 | 同一日期出现两行时，因子会被乘两次，输出里也会有重复的 bar（lens 实测） | 排序后去重或者报错 |
| L3 | normalize.go:48-49 | 时区：按调用方所在时区取日历日，三处口径一致。在上海早上调用时，窗口左端可能少一根 bar，与 yahoo 的行为相同 | 不改 |
| L4 | client.go:63-69 | 脱敏区分大小写。Tiingo 会不会回显大写形式的 token，未查实 | 可改为 `(?i)` 加 QuoteMeta |
| L5 | client.go:47-53 | 没有设置 CheckRedirect。同一主机、不同端口或 https→http 的重定向会带上 Authorization（lens 在 Go 1.24.4 上实测） | 拒绝重定向，或者只允许同主机 https |
| L6 | spec §3.1 / client.go:47 | Transport 用的是 `Clone()`，而 spec 写的是裸 Transport。AD 里没有登记，但 Clone 的方向更稳健 | 在 AD 里补登记 |
| L7 | symbols.go:18 | 小写代码会被拒，这与 spec §3.2「大小写原样保留」字面冲突。失败方向是安全的 | 在 AD-22 里补半句 |
| L8 | spec §3.6:117 | spec 原文「无代码依赖特定顺序」「调用方 4 处」是错的，TASK-001 已按 P12 更正为 5 处，但 spec 没改 | 订正 spec 或在 AD 里注明 |
| L9 | serve.go:170-177 | 只配置了 tiingo 和 qlib 时，backtester 固定用 tiingo，回测会消耗配额 | 在 AD 里记录 |
| L10 | cmd/atlas/prism.go:171-180 | usPriceHops 的两跳启用判据不一致：tiingo 要求 enabled 且有 key，twelvedata 只要有 key 就用。这是改动前就有的行为 | 在 config.example 里注明 |
| L11 | collector.go:71-73 | `prev != 0` 守卫没有测试（TASK-005 变异 V 存活）。删掉它会产生 ±Inf，并传到 snapshot 的 JSON 里 | 补一个 PrevClose=0 的用例 |
| L12 | collector.go:66 | FetchQuote 对 BRK.B 返回的 Symbol 没有断言（变异 W 存活）。当前实现是正确的 | 补断言 `Symbol=="BRK.B"` |
| L13 | cmd/atlas/prism.go:215、collectors.go:102 | 装配壳 M9（runPrismRefresh 传 nil）和 T6（传空 key）存活，DoD 定为 review 项，diff 核对通过 | 二期扩展 gate_wiring 的扫描范围时一起覆盖（AD-11） |
| L14 | refresh.go:71-80 | `PriceHop.Client` 的类型名是 `TwelvedataClient`，现在 tiingo 也放在里面，名字会误导 | 以后改名为 PriceHistoryClient，加别名兼容 |

另外，Minimalist 指出了几处测试冗余，都是 LOW，删不删不影响正确性，列出来供 simplifier 补跑时参考：
- policy_test.go:494/526 的两个 tiingo 用例重复测了 Gate/Table 的通用语义，其中 FortyFirst 还带跨整点重试。
- 40/1h 这个数值在 tiingo/gate_test.go:32-38 和 policy_test.go:469 各断言了一次。
- NVDA 折算在 normalize/client/collector 三层各测了一遍。
- collectors_test 对 YAML 注释的中文字面量做断言，这测的是文档，不是行为。

Skeptic 还有两条 LOW 没有单列：200 响应体为 `{"detail":…}` 时原因会被 decode 错误吞掉，Tiingo 是否会这样返回未查实；只有一根 bar 时 ChangePercent 为 0，snapshot 会丢掉 `Source=tiingo_eod`，但 spec §3.3 明确规定不足两根时取 0。

## fix_items（dev 返工）

无。三条 WARNING 的处置人分别是 Leader（W1）和人类（W2、W3），都不属于 dev 任务缺陷。reason_class 不适用。
