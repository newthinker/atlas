# TASK-005 验证报告 · 告警规则按规则冷却期（`alert.Rule.Cooldown`）

- 验证者：test-m15-a · 2026-09-04
- 判定对象：`verify_baseline.head = 2db85192f420a90ad5d79f912b66f11e2462836b`（master，merge of dev commit `d55b66ef64d291a7a0b9b5802c2089bfdddef0e4`）
- discovery：`.arcforge/discoveries/TASK-005.json` sha256 `107531f019af23cb3da77615c839f809bd6474305e9585fd58cb9d59001da112`（与基线一致）
- 验证环境：`git worktree add --detach ../wt-verify-TASK-005 2db85192f420a90ad5d79f912b66f11e2462836b`；变异与红阶段复现在 `mktemp -d` rsync 副本；主仓库与验证树三文件 sha256 前后一致（harness 自检 PASS）
- 基线对照锚：`037d1eb1e4f827c415319519e40f4e2208968920`（我在准备阶段自采：alert 92.3%）

## 结论：**VERIFIED**

7 条 done_criteria 全部有我自跑的输出作证据；范围核对无越界；`verify_baseline` 无漂移（当前 HEAD == baseline.head）。

## 范围核对

`git diff --numstat 037d1eb 2db8519`：`internal/alert/evaluator.go` 6/2、`evaluator_test.go` 38/0、`rules.go` 4/0——恰为声明 `writes` 三文件，无声明外改动。dev commit `d55b66e` numstat 同为这三文件（显式 pathspec）。`go.mod`/`go.sum` 与 `parse.go`/`extract.go`/`validate.go`/`fields.go` diff 为空。

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据（均在 2db8519 上自跑） | 判定 |
|---|---|---|---|
| functional[0] | `Rule.Cooldown time.Duration \`mapstructure:"cooldown"\`` 带注释（0=全局 5m、24h 例、`M1.5 的 TASK-005`）；`Evaluate` 改为 `cooldown := e.cooldown; if rule.Cooldown > 0 {…}; if hasFired && now.Sub(lastFired) < cooldown` | diff 逐行核对：`rules.go` 仅 +4 行（注释 3 行 + 字段）；`evaluator.go` 仅冷却块 +6/-2，pending/for、fire 循环、`anySucceeded` 块未动 | PASS |
| functional[1] | `TestEvaluator_PerRuleCooldown`：40 > 30，1h 规则 1 → +30m 仍 1 → +31m 2；`api_down` 未写 Cooldown 3 → +4m 仍 3 → +2m 4 | 测试体与 DoD 时序逐项一致（`evaluator_test.go:301-333`）；`go test -run … -v` ⇒ `--- PASS: TestEvaluator_PerRuleCooldown` | PASS |
| functional[2] | 既有 `TestEvaluator_Cooldown` 与全包不受影响；`SetCooldown` 语义不变；`Rule.Evaluate`/`FormatMessage` 不动 | `--- PASS: TestEvaluator_Cooldown`；`internal/alert` 全包 ok；diff 未触及 `SetCooldown`（:70）、`Rule.Evaluate`、`FormatMessage` | PASS |
| boundary[0] | 变异 M1（`if rule.Cooldown > 0` → `if false`）、M2（`e.cooldown` → `rule.Cooldown`）必红；负值视同未写只注释 | 我的 harness（隔离副本，diff 打印核对，gofmt+vet 有效性闸）：对照组 ok；**M1 KILLED**（`:313 30 分钟后仍在 1h 冷却内，应只发 1 条，got 2`）；**M2 KILLED**（`:326 … got 4`，且既有 `:93 got 3` 同红）；与 dev 自述一致。`rules.go` 注释含「负值按 `> 0` 判定视同未写，不加校验」 | PASS |
| error_handling[0] | 红阶段：加字段前 `unknown field Cooldown in struct literal` | 副本里把 `rules.go`/`evaluator.go` 换回 `037d1eb` 保留新测试 ⇒ `evaluator_test.go:308:84: unknown field Cooldown in struct literal of type Rule`，`[build failed]`；discovery `red_phase` 同文案 | PASS |
| non_functional[0] | 门禁：gofmt 只列三既有欠账；vet 零输出；五包全绿；alert ≥ 92.3%；hestia ≥ 96.3%；无新增依赖；四冻结文件不动 | `gofmt -l` 五包 = `snapshot_test.go`/`backtest_test.go`/`crisis_test.go` 恰三项；`go vet` rc=0；五包 `-count=1` 全 ok：hestia 96.5% / metrics 98.9% / **alert 92.6%**（基线 92.3%）/ config 83.3% / cmd/atlas 76.3%；`git diff --stat 037d1eb 2db8519 -- go.mod go.sum <四冻结文件>` 空 | PASS |
| non_functional[1] | AD-6 交付流程（worktree、pathspec、`feat(TASK-005):` 锚、code-simplifier、等 merge、master 重采、discovery、拆 worktree） | 提交信息 `feat(TASK-005): M1.5 …` 匹配门禁 grep；merge commit `2db8519` 在 master；`git worktree list` 无 `wt-TASK-005-m15`（已拆）；discovery 同时写 `my_commit`/`master_after_merge` 且各自证数字锚 2db8519、与我复采一致；code-simplifier 仅 dev 自述（review 级，不作断言） | PASS（review） |

## 备注

- 分支 `task/TASK-005-m15` 仍存在（已合入，可由 Leader 阶段边界清理；不在 DoD 内，不影响判定）。
- 复现命令（锚全 sha）：`git worktree add --detach <dir> 2db85192f420a90ad5d79f912b66f11e2462836b && cd <dir> && GOTOOLCHAIN=local go test ./internal/alert/ -run 'TestEvaluator_PerRuleCooldown|TestEvaluator_Cooldown' -count=1 -v`
