# TASK-007 验证报告（test-tg-b）

- 任务：serve 装配注册 tiingo + 配置示例 + gate_wiring 登记
- 判定对象：`3700b40c48ad28e5833366c5893a1dff547c825b`（= verify_baseline.head，= 验证时主仓库 HEAD），即 dev 提交 `fa345831f31e4bfd0a109a3fb4b38862181779a2` 的 merge
- `git diff --stat fa34583 3700b40 -- <4 个 writes 文件>` 为空；dev 提交只改 4 个文件，都在 `writes` 内，无越界
- discovery sha256 = `0adcd50e…d6a5`，与 verify_baseline 一致
- 环境：隔离 detached worktree（测试一个、变异/探针另一个），`GOTOOLCHAIN=local`；cmd/atlas 未用 `-shuffle`（AD-16）

## 结论：VERIFIED

## done_criteria 覆盖矩阵

| # | 完成标准 | verify_by | 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | GetCollectors 名字序列逐元素 = [yahoo, eastmoney, tushare, baostock, tiingo] | test | TestBuildCollectors_RegistersTiingoLast 用 `slices.Equal` 比较 `GetCollectors()` 的注册顺序序列（不是排序后的 collectorNames）；T4 挪到 tushare 之前、T5 挪到 baostock 之前、T7 配置键名错 均被杀 | PASS |
| functional[1] | tiingo 位于 qlib 之前 | review（dev 另加了测试） | TestBuildCollectors_TiingoBeforeQlib 用临时 sqlite 真正启用了 warehouse，断言里 `qi < 0` 直接判失败，所以不存在空真。基线上该测试通过 ⇒ qlib 确实进了 registry。T1 把 tiingo 挪到 wireQlibWarehouse 之后，被这条测试独家杀掉。行号核对（合并树）：collectors.go:101–104 为 tiingo 块，collectors.go:109 为 `wireQlibWarehouse(` | PASS |
| functional[2] | 示例配置有 tiingo 块（enabled false / api_key "" / markets [US]），注释含 tiingo.daily TTL 6h 与共用 40 次/时，能被 config.Load 加载 | test | TestExampleConfigDeclaresTiingo；Y1–Y6（改 ttl、删「40 次/时」、enabled true、markets HK、改主题名、改块名）全部被杀。**另做了探针**：把注释里建议的 `collector.topics."tiingo.daily".ttl: 6h` 取消注释后交给 config.Load，得到 `Collector.Topics["tiingo.daily"].TTL = 6h0m0s`，说明注释给的配置路径是真能生效的（探针跑完已删除） | PASS |
| boundary[0] | Enabled:false+key、或 Enabled:true 缺 key：都不登记 tiingo，其余序列与不配置 tiingo 时完全相同 | test | TestBuildCollectors_SkipsTiingoWhenUnconfigured：基准序列取实际运行结果，并断言基准非空；T2 去掉 key 判空、T3 去掉 Enabled 判断 被杀 | PASS |
| non_functional[0] | collectorCtors 含 `"tiingo.New": true`，相邻注释写明不守护 usPriceHops 与 tiingo.NewCollector | review | gate_wiring_test.go 的登记与注释已读。注释写的是「这条登记**不守护** tiingo 的真实调用点……prism 的构造在 usPriceHops 里、serve 调用的是 tiingo.NewCollector」。核对扫描范围：gate_wiring_test.go:162–164 只扫 runCrisisBackfill、runCrisisEval、runBacktest 三个函数，与注释和 AD-11 一致 | PASS |
| non_functional[1] | discovery 给出 prism / serve「先装配闸门、后构造」的行号 | review | 在合并树上逐行 `sed` 核对：serve.go:85 为 `initPolicyGate(cfg, log)`，serve.go:108 为 `buildCollectors(...)`，collectors.go:102 为 `tiingo.NewCollector(collectorCfg.APIKey)`；prism.go:185 为 `loadConfigOrDefaults()`，其内 export_ohlcv.go:297 为 `initPolicyGate(cfg, nil)`，prism.go:212 为 `usPriceHops(cfg.Collectors)`，prism.go:174 为 `tiingo.New(tc.APIKey)`，prism.go:171 为 `func usPriceHops`。**全部与 discovery 一致**，先后顺序成立 | PASS |
| non_functional[2] | go test 全绿、go vet 无输出、cmd/atlas 覆盖率 ≥78（两把尺） | test | 见下方证据 | PASS |
| non_functional[3] | 注册时传入的是 collectorCfg.APIKey | review | diff 与合并树 collectors.go:102 均为 `tiingo.NewCollector(collectorCfg.APIKey)`。T6（改传空 key）在名字断言下存活，与 DoD 的预判一致（Collector 的 key 未导出，测试无法观察），所以本条按 review 判定 | PASS |

## 证据
```
@ 3700b40
go test -count=1 ./cmd/atlas/ ./internal/...  → rc=0，ok 67 个包，非 ok 行 0 条
go vet ./cmd/atlas/ ./internal/collector/... ./internal/prism/  → rc=0 无输出
gofmt -l cmd/atlas configs → 只有 cmd/atlas/backtest_test.go；基线 f434957 上的同一文件经 stdin gofmt -l 同样报出 ⇒ 基线既有问题，不在本任务 writes 内
cmd/atlas 覆盖率：
  go test 报告      79.6%
  go tool cover -func total 79.8%
  profile 按块去重求和 1325/1664 = 79.63%（块数 1070）
  ⇒ 两把尺都 ≥ 78，数字与 dev 自报一致
-v：TestBuildCollectors_* 7 条与 TestExampleConfigDeclaresTiingo 全部 PASS
```

### 独立变异（隔离 worktree；脚本 scratchpad/test-tg-b-TASK-007-mut.py；每个变异先 vet，收尾源文件还原、porcelain 为空）
| 变异 | 结果 | 杀死它的测试 |
|---|---|---|
| T1 tiingo 挪到 qlib 之后 | KILLED | TiingoBeforeQlib |
| T2 去掉 key 判空 | KILLED | SkipsTiingoWhenUnconfigured |
| T3 去掉 Enabled 判断 | KILLED | SkipsTiingoWhenUnconfigured |
| T4 挪到 tushare 之前 | KILLED | RegistersTiingoLast |
| T5 挪到 baostock 之前 | KILLED | RegistersTiingoLast |
| T6 传空 key | SURVIVED | 预期存活，nf[3] 按 review 判 |
| T7 配置键名写错 | KILLED | RegistersTiingoLast、TiingoBeforeQlib |
| T8 删除整块 | 无效变异（import 未使用） | T7 已覆盖同一行为 |
| Y1–Y6 示例配置的 6 个变异 | 全部 KILLED | ExampleConfigDeclaresTiingo |

有效变异 13 个，被杀 12 个，唯一存活的 T6 是 DoD 预期存活。

## 观察（不影响判定）
- discovery 记录 impact(buildCollectors) = UNKNOWN，已用 grep 补查调用方：serve.go:108 与 watchlist.go:64，符合 AD-10。watchlist 同样获得 tiingo 兜底，AD-13 已接受。
