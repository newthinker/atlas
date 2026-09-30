# TASK-006 验证报告（prism 美股价格链有序多跳 yahoo→tiingo→twelvedata）

- 验证者: test-tg-a
- 判定对象: feature/tiingo-source @ 3700b40c48ad28e5833366c5893a1dff547c825b（= verify_baseline.head；主仓库 HEAD 同值）
- 交付提交: 21cbc4d19154bfc76cd8ab968f6fd34c855458b2（merge f610117544c05de58508d9b4c4a8c3723c700626）；`--stat` 仅 writes 声明的四个文件；`git diff --stat f610117 3700b40 -- <四文件>` 为空，基线上四文件与交付逐字相同
- 运行环境: 隔离 worktree（detach @ 3700b40c48ad28e5833366c5893a1dff547c825b）+ 对照 worktree（detach @ 8d1c6cc，改动前），GOTOOLCHAIN=local，cmd/atlas 未用 -shuffle（AD-16）

## 覆盖矩阵

| # | done_criteria | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | P17：yahoo 败、tiingo 报错、twelvedata 成功，文案逐字 | TestFetchClosesSecondHopSucceeds（assert.Equal 全串） | PASS |
| functional[1] | tiingo 成功 ⇒ 含 `tiingo fallback ok` 且 twelvedata 调用记录空；yahoo 成功 ⇒ 无跳被调、文案空 | TestFetchClosesFirstHopSucceedsSkipsRest（全串 + `td.calls` 为空）/ TestFetchClosesYahooOKCallsNoHop | PASS |
| functional[2] | usPriceHops nil⇒空；两跳顺序；tiingo 未启用/缺 key ⇒ 仅 twelvedata；无 typed-nil | TestUSPriceHopsOrderAndSkips（含 reflect IsNil） | PASS |
| boundary[0] | tiingo 零行视为失败继续，文案逐字 | TestFetchClosesEmptyHopFallsThrough | PASS |
| boundary[1] | nil hops 错误逐字 + errors.Is；单 twelvedata 跳与改动前逐字一致；两个既有测试只改实参 | TestFetchClosesNoHopsKeepsPrefix / TestFetchClosesSingleHopMatchesLegacyFormat / 两既有测试 | PASS（另见「对照改动前」） |
| error_handling[0] | 全败错误按跳顺序逐字 | TestFetchClosesAllHopsFail | PASS |
| non_functional[0] | 两包 -race 全绿、vet 空；discovery 记 impact 结论 | `go test -count=1 -race ./internal/prism/ ./cmd/atlas/` 两包 ok；vet rc=0；gofmt -l 四文件空；discovery key_findings 记 UNKNOWN（gitnexus 失效）+ grep 补查、已报 Leader | PASS |
| non_functional[1] | runPrismRefresh 传 usPriceHops(cfg.Collectors)（review） | diff：`usHops := usPriceHops(cfg.Collectors)` 并作为 `prism.Refresh(..., ts, usHops, time.Now())` 实参；旧 `td := twelvedataClientOrNil(...)` 已删 | PASS（review） |

目标测试 `-run 'TestFetchCloses|TestRefreshUSPrice|TestUSPriceHops'`：10 PASS / 0 FAIL。

## 两个既有测试只改实参

`git show 21cbc4d -- internal/prism/refresh_test.go`：TestRefreshUSPriceFallsBackToTwelvedata 与 TestRefreshUSPriceTwelvedataEmptyIsNotSuccess 各只有一行 `-/+`，即 `td` → `[]PriceHop{{Name: "twelvedata", Client: td}}`，断言行无改动。

## Review Focus 5：与改动前逐字对照（不是按断言推理，是实跑）

在 8d1c6cc（旧 fetchCloses(us, td, …)）与基线树（新 fetchCloses(us, hops, …)）上跑同一个临时探针（跑完即删，两树 git status 均 0 行），同样的 fake：

| 场景 | 8d1c6cc | 3700b40 |
|---|---|---|
| 单 twelvedata 跳失败 | `price history: yahoo 503; twelvedata fallback: td 429`，Unwrap==nil | 同左，Unwrap==nil |
| 单 twelvedata 跳成功 | `NVDA: yahoo price failed (yahoo 503), twelvedata fallback ok` | 同左 |
| 无备用跳 | `price history: yahoo 503`，errors.Is(err, yerr)=true | 同左，true |

逐字一致，连「有跳时不保链」这一点也一致（新实现 errors.New 与旧 `%v` 同效）。AD-20「首跳成功时与现状逐字一致」同样成立：tiingo 首跳成功文案为 `NVDA: yahoo price failed (yahoo 503), tiingo fallback ok`，与旧格式同构。

## 调用方独立核实（gitnexus 失效，grep 全仓非测试 .go）

- `prism.Refresh(` 生产调用：仅 cmd/atlas/prism.go:216（runPrismRefresh 内闭包）；全仓无其他名为 Refresh 的包级函数。
- `fetchCloses(`：仅 refresh.go:284（refreshEngine）与 :679（refreshEdgar）。
- `refreshEngine(`：refresh.go:137、:157（Refresh 内 engine 分支与 edgar→engine 兜底）；`refreshEdgar(`：:154。
- `usPriceHops(`：仅 prism.go:212；runPrismRefresh 仅被 prismRefreshCmd.RunE（prism.go:33）引用。
与 discovery 一致。

附带核实：tiingo.New 在构造时快照 policy.Default()；runPrismRefresh 先 `loadConfigOrDefaults()`（其内调用 initPolicyGate，export_ohlcv.go:297），后 `usPriceHops`，顺序正确，tiingo 跳拿到的是配置好的 Gate。

## 变异测试（独立设计，scratchpad/test-tg-a-TASK-006-mut.py；还原后两文件 sha256 一致、worktree git status 与开跑前相同）

| 变异 | 结果 | 致红 |
|---|---|---|
| P1 文案不附前序失败跳（计划草稿写法） | KILLED | SecondHopSucceeds, EmptyHopFallsThrough |
| P2 成功前先把所有跳调一遍 | KILLED | FirstHopSucceedsSkipsRest |
| P3 零行视为成功 | KILLED | EmptyHopFallsThrough, TwelvedataEmptyIsNotSuccess |
| P4 无跳时 %w→%v | KILLED | NoHopsKeepsPrefix |
| P5 错误串只留最后一跳 | KILLED | AllHopsFail |
| P6 成功文案跳名写死 twelvedata | KILLED | FirstHopSucceedsSkipsRest |
| P7 倒序遍历跳 | KILLED | 4 个 FetchCloses 测试 |
| P8 yahoo 成功前也调首跳 | KILLED | YahooOKCallsNoHop |
| P9 忽略 Enabled / P10 忽略 tiingo key / P11 twelvedata 在前 / P12 无 key 入 typed-nil | KILLED | USPriceHopsOrderAndSkips |
| P13 runPrismRefresh 改传 nil（= dev 的 M9） | SURVIVED（预期） | — |

P13 首版因 `usHops` 未使用导致编译失败（vet rc=1），属错误理由的 KILLED，已作废；重做为 `func() []prism.PriceHop { _ = usHops; return nil }()`（vet rc=0），`go test ./cmd/atlas/` 仍 ok ⇒ 存活，与 dev 自报 M9 一致。该装配壳历来 0% 覆盖，DoD 将其定为 review 项，按 diff 判定已正确传入（见矩阵 non_functional[1]），不作为退回理由。

12/12 行为变异 KILLED（全部 vet rc=0、断言失败而非崩溃）。

## 结论

VERIFIED。P17 文案、首跳成功短路、零行继续、全败错误顺序、usPriceHops 装配均有逐字断言且有牙；nil 跳与单跳两种旧形态经实跑对照与 8d1c6cc 逐字一致；生产调用方与 discovery 一致。
