# TASK-001 验证报告 — IndicatorResult 新字段与规则层填充

- **验证者**: test-agent-1
- **提交**: d43d029（internal/crisis 6 文件 +103/-8）
- **日期**: 2026-07-15
- **判定**: REJECTED（NEEDS WORK）
- **一句话结论**: 生产代码正确、测试全绿、覆盖率 91.2%，但 functional[2] 明确要求的「负向断言」（全绿/零值行 omitempty 省略三键）在测试中完全缺失，只测了正向。

## 亲跑证据
- `GOTOOLCHAIN=local go build ./...` → 通过（exit 0）
- `GOTOOLCHAIN=local go test ./internal/crisis/ -count=1` → ok，coverage 91.2%（≥80% 达标）
- 新函数覆盖：consecutiveAbove 100%、evalVIX 91.7%、evalSOFREFFR 91.7%、evalUSDJPY 87.5%、buildEvaluations 84.6%、rawFromDetail 100%
- 两个新测试单独跑均 PASS：TestIndicatorResultPersistAndWow、TestBuildEvaluationsCarriesPersistAndWow

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | sofr 末尾连9>25bp→RED&PersistDays=9（不受 red_persist_days=5 限制）；末尾3在(10,25]→AMBER&PersistDays=3 | TestIndicatorResultPersistAndWow：断言 StatusRed+PersistDays=9、StatusAmber+PersistDays=3 | PASS |
| functional[1] | usdjpy 周环比恰-3%→RED,WowOK=true,Wow≈-0.03；vix 全平→WowOK=true,Wow≈0 | TestIndicatorResultPersistAndWow：usdjpy 段 assert StatusRed+WowOK+InDelta(-0.03)；vix 段 assert WowOK+InDelta(0) | PASS |
| functional[2] | detail JSON 携带三键（正向示例）；**全绿/零值行经 omitempty 省略三键（负向断言）** | TestBuildEvaluationsCarriesPersistAndWow：仅 3 条正向 Contains（sofr persist_days:9 / usdjpy wow:-0.031 / wow_ok:true）。**无任何 NotContains 断言验证 green 行省略三键**——负向半条未覆盖 | **FAIL** |
| boundary[0] | evalSOFREFFR lastN 还原后语义等价，rules/suppress/eval 全部测试无回归 | 全套 internal/crisis 测试通过（含既有 rules_test/eval_test/suppress 用例），无回归；diff 用 lastN(win, RedPersistDays/AmberPersistDays) 还原原判定窗口 | PASS |
| error_handling[0] | sr.Window 错误原样上抛，无新增吞错路径 | 既有 TestEvalDaySeriesReaderError（errReader.Window 恒报错→EvalDay 上抛不落半套）覆盖上抛路径；diff 显示 evalSOFREFFR 仍 `return err`，无新增 swallow | PASS |
| non_functional[0] (review) | gitnexus_impact 四符号执行且无 HIGH/CRITICAL + detect_changes + code-simplifier | discovery 记录：impact upstream 全 LOW；detect_changes 9 符号全落 internal/crisis；code-simplifier 无改动。审查式核对通过 | PASS |
| non_functional[1] (test) | build ./... + test ./internal/crisis/ 全绿 | 亲跑通过（见上） | PASS |

## 越界修改核查（Leader 要点1）
suppress.go diff 只改 indDetail 结构体（新增 3 个 omitempty 键），**未触及 line 18/25/110 的既有 for 循环**（modernize lint 提示未被"顺手改"）。其余文件 diff 均限于新字段填充与 marshal，无无关改动。**通过**。

## 拒绝原因（reject_reason）
functional[2] 后半句「全绿/零值指标行经 omitempty 省略这三个键（负向断言）」在测试中无对应断言。TestBuildEvaluationsCarriesPersistAndWow 仅对 sofr/usdjpy 行做正向 Contains，未对任一全绿行（如 vix/move/hy_oas/t10y2y/nfci）做 NotContains 验证。生产代码 omitempty 标签正确，缺的是断言。

## 建议修复方向（小改，仅测试）
在 TestBuildEvaluationsCarriesPersistAndWow 末尾补负向断言，例如对一个保持全绿零值的指标行：
```go
assert.NotContains(t, byInd[IndVIX].Detail, "persist_days")
assert.NotContains(t, byInd[IndVIX].Detail, "wow")   // 注意需确保该 green 行 WowOK=false、Wow=0
assert.NotContains(t, byInd[IndVIX].Detail, "wow_ok")
```
注：选负向断言目标行时避开 vix/usdjpy 这类会被填充 Wow/WowOK 的指标，宜选 hy_oas/t10y2y/nfci 等纯零值行，避免 omitempty 对 Wow=0&WowOK=true 情形的歧义。

## 附带观察（非阻塞，供参考）
`Wow float64 json:"wow,omitempty"`：当 WowOK=true 但 Wow 恰为 0.0（如 vix 基线全平）时，omitempty 会省略 "wow":0 却保留 "wow_ok":true，JSON 上 Wow=0 与"无 wow"不可区分。当前下游 rawFromDetail 只读 Raw，不回读 wow，暂无实际影响；若 Task 4 之后有从 detail JSON 回读 wow 的需求需留意。不计入本次判定。

---

## 二次验证（增量，2026-07-15，epoch=2，rework=1，提交 0e9ee0a）

**判定：VERIFIED（PASS）**

上轮 6 条 PASS 结论沿用（生产代码未动）；本轮只复核 functional[2] 修复点。

| 复核点 | 证据 | 判定 |
|---|---|---|
| functional[2] 负向断言 | 0e9ee0a 对 hy_oas 绿行新增 3 条 NotContains（`"persist_days":`/`"wow":`/`"wow_ok":`），冒号形式锁死独立性（`"wow":` 后为引号，不误匹配 `"wow_ok":` 的 `wow_`）。**变异测试确认有效**：临时去掉 suppress.go 三处 omitempty 后测试正确 FAIL（green 行含 `"wow_ok":`），已还原 suppress.go 无残留 | PASS |
| diff 范围 | `git show --stat 0e9ee0a`：仅 internal/crisis/eval_test.go +7 行，无生产代码、无越界 | PASS |
| 覆盖率 | `go test ./internal/crisis/` 全绿，coverage 91.2%（≥80%） | PASS |
| non_functional[0] review（detect_changes） | 本轮 MCP CLI(v40)/DB(v42) 版本不匹配，工具不可用——按 Leader 降级指示以 git diff 为证据（diff 仅 test 文件，无符号影响面变化），记录降级不 FAIL | PASS（降级） |

**结论**：functional[2] 修复到位且经变异测试验证断言非空洞，7 条 DoD 全部 PASS。TASK-001 verified。

### 补记（Leader 代跑 detect_changes）
non_functional[0] 的 detect_changes 门禁由 Leader 侧代跑（Leader 会话 MCP 为匹配 v42 索引的新构建，compare vs master）：变更符号全落 internal/crisis（types/rules/suppress/eval + 两测试文件），受影响执行流均为预期 eval* 流程，无越界。据此 non_functional[0] 由「工具不可用降级」**升级为「Leader 代跑通过」**，判定仍 PASS。后续任务的 detect_changes 门禁同样由 Leader 代跑（各 teammate 会话 MCP 旧构建与 v42 索引不匹配）。
