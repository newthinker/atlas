# TASK-005 验证报告（tiingo collector.Collector 实现）

- 验证者: test-tg-a
- 判定对象: feature/tiingo-source @ f43495731ba45e05451542f97e59d52b27664021（= verify_baseline.head；主仓库 HEAD 同值，无漂移）
- 交付提交: 290dbbc7ea66a4d809561d13341dab31997839ab，`--stat` 仅 collector.go / collector_test.go（writes 内，无越界）
- 运行环境: 隔离 worktree `git worktree add --detach <scratchpad>/test-tg-a-wt5 f43495731ba45e05451542f97e59d52b27664021`，GOTOOLCHAIN=local

## 覆盖矩阵

| # | done_criteria | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | 编译期断言；Name/SupportedMarkets/Start/Stop；Init(k2) 后请求头 `Token k2` | collector_test.go `var _ collector.Collector = (*Collector)(nil)`；TestCollectorMetadataAndInit（经 httptest 读回 Authorization==`Token k2`、请求数 1） | PASS |
| functional[1] | 末两根拼装 Quote 12 个字段 | TestCollectorFetchQuoteFromLastTwoBars（逐字段断言，含 ChangePercent≈5、Time=2026-09-29、Source tiingo_eod） | PASS |
| functional[2] | startDate = now−10 天 = 2026-09-20 | TestCollectorFetchQuoteStartDate（读服务端收到的 query） | PASS |
| functional[3] | FetchHistory 1d 透传并返回折算后 bars | TestCollectorFetchHistoryPassthrough（NVDA 拆股夹具，bars[0].Close=122.44 为折算后值、path 与 startDate 核对） | PASS |
| boundary[0] | 单根 PrevClose/Change/ChangePercent=0；零根错误含 `no recent bars` | TestCollectorFetchQuoteSingleBarAndEmpty | PASS |
| error_handling[0] | 5 个不支持代码 FH/FQ 均报错（FH 含 `unsupported symbol`）且请求计数 0；1h 报错含 `interval` 且计数 0；Init(Config{}) 报错 | TestCollectorRejectsUnsupportedWithoutHTTP / TestCollectorRejectsNonDaily / TestCollectorMetadataAndInit | PASS |
| non_functional[0] | -race 全绿、覆盖率 ≥80%、vet 空 | `go test -count=1 -race -v ./internal/collector/tiingo/`：33 PASS / 0 FAIL；覆盖率 95.6%（collector.go 除 FetchQuote 94.1% 外均 100%）；vet rc=0；gofmt -l 空 | PASS |

## Review Focus 4：两条路径的零请求是否各自被守卫

`TestCollectorRejectsUnsupportedWithoutHTTP` 的计数断言是两条路径的合计，但变异分别删掉两处白名单检查：
- C（FetchHistory 不查白名单）→ KILLED
- D（FetchQuote 不查白名单）→ KILLED。注意 D 下 FetchQuote 仍然报错（空回放触发 `no recent bars`），`require.Error` 抓不到，**真正杀死它的是计数断言**——正是 DoD 要的判据。
- E（不查 interval）→ KILLED（计数与错误文本）。

## 有据偏离：Init 用 NewWithBaseURL(key, c.client.baseURL)

读基线树代码核实：
1. **生产端点不变**：`NewCollector` → `New(apiKey)` → `NewWithBaseURL(apiKey, defaultBaseURL)`，`defaultBaseURL="https://api.tiingo.com"`（client.go:25/40）。Init 取的 `c.client.baseURL` 即它（已 TrimRight，无尾斜杠，再 TrimRight 幂等）⇒ 生产下与计划的 `New(key)` 指向同一端点。
2. **Gate 重新快照**：是的，Init 经 `NewWithBaseURL` 重新执行 `gate: policy.Default()`（client.go:53）；**计划的 `New(key)` 同样会走这一行**，偏离不引入差异。生产中 `policy.SetDefault` 只在 `cmd/atlas/policy.go:66`（`initPolicyGate`）调用，serve.go:85 早于 `app.New`（:101）与一切 collector 构造；`Default()` 是进程单例（policy/default.go，非 nil 直接返回同一指针），故重新快照拿到的是同一个 Gate，无副作用。唯一的差别是 Init 顺带新建了 `http.Client`（同样的 Proxy=nil、30s 超时），无状态影响。
3. 附带：Init 非原子替换 `c.client`，若与 Fetch 并发会有竞争；collector 约定 Init 先于 Start/Fetch 调用，DoD 未要求，不计缺陷。

结论：偏离有据，生产行为不变。

## 变异测试（独立设计，scratchpad/test-tg-a-TASK-005-mut.py；每个 vet rc=0；还原后 sha256 一致、worktree git status 与开跑前逐字相同）

23 个变异，21 KILLED（全部为断言失败，**0 个崩溃型**），2 SURVIVED：

| 变异 | 结果 | 致红测试 |
|---|---|---|
| A Init 不换 key / B 不校验空 key | KILLED | MetadataAndInit |
| C FH 不查白名单 / D FQ 不查白名单 | KILLED | RejectsUnsupportedWithoutHTTP |
| E 不查 interval | KILLED | RejectsNonDaily |
| F 回看 7 天 | KILLED | FetchQuoteStartDate |
| G ChangePercent 小数 / H Time 取 now / I Source 改名 / J 取首根 / L PrevClose 需 ≥3 根 / N Market 非 US / O Low 取 High / Q Volume 置 0 / R Open 取 Close / S Change 反号 | KILLED | FetchQuoteFromLastTwoBars |
| K 零根返回空 Quote（非崩溃写法） | KILLED | FetchQuoteSingleBarAndEmpty |
| M Name 改名 / P Start 报错 / U SupportedMarkets 为空 | KILLED | MetadataAndInit |
| T FH start/end 互换 | KILLED | FetchHistoryPassthrough |
| V 去掉 `prev != 0` 守卫 | SURVIVED | — |
| W Quote.Symbol 改为 toTicker(symbol) | SURVIVED | — |

存活二者**不在 DoD 范围内**，不作为退回理由，记给 QA/Leader：
- V：非等价（前收盘为 0 时 ChangePercent 变 ±Inf/NaN），但 DoD 未列 prev=0 场景，美股实际收盘价为 0 基本不可能。
- W：对 DoD 用例 `AAPL` 等价（无 `.`）；对 `BRK.B` 会让 Quote.Symbol 变成 `BRK-B`。DoD 只要求 Symbol==AAPL。若希望锁住「Quote.Symbol 用 atlas 代码而非 Tiingo ticker」，可在 QA 阶段补一条 BRK.B 的 FetchQuote 用例。

与 dev 自报（16/16，含 1 崩溃型）对照：我的 K、L 采用非崩溃写法，均由断言杀死，说明零根/单根边界不依赖 panic 才被抓住。

## 结论

VERIFIED。7 条 DoD 全部有对应且有牙的测试，Review Focus 4 的两条零请求路径各自被计数断言独立守卫，Init 偏离经代码核实生产行为不变、Gate 重新快照无副作用。
