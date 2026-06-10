# TASK-004 验证报告 — 新增 analysis/arbitrator-timeout/collector-cache 配置

- 验证者: test-agent-2 (Reality Checker)
- 包: ./internal/config
- 验证命令（亲自复跑）:
  - `go test -race -cover -count=1 ./internal/config/` → ok, coverage **93.5%**（与 dev 自报一致）
  - 全量回归 `go test -race -count=1 ./internal/config/` → ok（无既有用例破坏）

## 判定: VERIFIED ✅

所有 5 条 done_criteria 逐条有对应的、真实断言的测试；-race 通过；覆盖率 93.5%；无回归。

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 断言核验 | 判定 |
|---|---------|---------|---------|------|
| functional[0] | yaml 设 analysis.workers / meta.arbitrator.timeout / collector.cache.{enabled,ttl} 后 Load 返回对应值 | TestLoad_NewConfig_FromYAML | Workers=8、Timeout=30s、Cache.Enabled=false、Cache.TTL=2m 全部断言 | PASS |
| functional[1] | 三组配置全缺省时取默认 Workers=4/Timeout=15s/Enabled=true/TTL=5m | TestLoad_NewConfig_Defaults | 四个默认值逐一断言 | PASS |
| boundary[0] | workers=0/负数 Load 不报错（串行语义，零值保留） | TestLoad_Workers_ZeroOrNegative | 表驱动 w∈{0,-1}：Load 无 error 且 Workers==w（验证 SetDefault 不覆盖显式零值） | PASS |
| boundary[1] | timeout/ttl=0 时取各自默认值 | TestLoad_TimeoutTTL_ZeroUsesDefault | timeout:0→15s、ttl:0→5m 断言（验证 <=0 后处理回退） | PASS |
| error_handling[0] | timeout/ttl 非法 duration 字符串（如 "abc"）时 Load 返回错误 | TestLoad_InvalidDuration | timeout:"abc"→Load 返回非 nil error | PASS（见下注） |

## 注记（非阻塞）
- error_handling[0]: 测试仅覆盖 `timeout:"abc"`，未单独覆盖 `ttl:"abc"`。两者经同一 viper
  `StringToTimeDurationHookFunc` 解码路径，timeout 用例已证明 Unmarshal 阶段对非法 duration
  返回 error，机制对 ttl 同样成立，故判 PASS。建议（可选，不阻塞）后续补一条 ttl 非法字符串用例使矩阵更对称。
- 实现核查：SetDefault("analysis.workers",4)/("collector.cache.enabled",true) 仅在 key 缺省时生效，
  保留显式零值（boundary[0] 成立）；Timeout/TTL 在 Unmarshal 后 `<=0` 回退默认（boundary[1] 成立）。
  Defaults() 同步含 Analysis/Arbitrator.Timeout/Collector 默认，与 Load 一致。
- 额外覆盖：Validate 分支表驱动、WarnHardcodedSecrets 等，测试充分。

---

## 复验 (review_fix W3, rework_count=1, commit cc2f0ff) — 2026-06-10 19:10

- 验证者: test-agent-2
- fix_item: W3[中] Validate 接受 execution.mode 空串但 Execute 运行期拒绝 → broker 启用漏配 mode 时静默不下单。

### 实跑证据
- `go test ./internal/config/ -race -cover` → **ok, 93.6%**（原 5 条 DoD 回归全过，无破坏）
- W3 专项：`-run TestLoad_ExecutionMode` → 2/2 PASS

### fix_item 核对
| fix_item | 修复 | 对应测试 | 断言核验 | 判定 |
|---|---|---|---|---|
| W3: Load 为 execution.mode 补默认 confirm，测试覆盖「yaml 缺省 → confirm 且 Validate 通过」 | config.go: `v.SetDefault("broker.execution.mode","confirm")` | TestLoad_ExecutionMode_DefaultsToConfirm | yaml 缺省 execution.mode（broker.enabled=true,mode=paper）→ Load 后 `Execution.Mode=="confirm"` **且** `Validate()==nil` | PASS |
| （附加保障）显式值不被默认覆盖 | 同上 SetDefault 仅缺省生效 | TestLoad_ExecutionMode_ExplicitPreserved | 显式 mode=batch → 保留 "batch" | PASS |

### 根因有效性核查（Reality Checker）
- 确认 W3 根因真实：execution.go:180-187 switch 中空串 mode 落入 `default → ErrInvalidExecutionMode`（line 186），
  而 `ExecutionConfirm` 是合法分支（line 183 → queueForConfirmation）。
- 故「默认 confirm」使漏配场景由「运行期静默拒单」变为「合法 confirm 模式」，修复对症且有效。
- 测试非空洞：断言同时校验默认值落地（Mode=="confirm"）与 Validate 通过，覆盖 fix_item 全部要求。

### 复验判定: VERIFIED ✅
