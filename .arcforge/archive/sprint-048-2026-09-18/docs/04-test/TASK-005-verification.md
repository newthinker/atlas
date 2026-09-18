# TASK-005 验证报告 —— 五条 hestia 队列告警规则

# 第 2 轮（返工复验）

- 验证者：test-m4-b
- 判定：**VERIFIED**
- 判定对象：master @ `4cd6cccf2b060ccc718d841a6b17d5eb41f3b045`（= verify_baseline.head；判定时主仓库 HEAD 相同；discovery sha256 `8db78320…40b8` 与基线一致）
- 验证环境：隔离 worktree（detached @ 上述全 sha，已拆）
- assignment_epoch：2
- DoD 依据：Leader 订正后的 functional[0]（六项含 message 逐字 `assert.Equal`）与 functional[2]（done/ 放 1 份）
- 返工范围：merge 相对第一父 numstat 仅 `cmd/atlas/hestia_alert_rules_test.go 24/10`；`configs/config.example.yaml` 与第 1 轮基线逐字节相同

## R2-1. 亲跑全量（non_functional[1]，验收判据一）

`GOTOOLCHAIN=local go test -count=1 ./...` 输出落文件后分别取值：退出码 **0**；`^ok ` 65 行；`^(FAIL|--- FAIL)` 0 行；`^?` 0 行；总行数 65 = `go list ./...` 包数 65。尾部：

```
ok  	github.com/newthinker/atlas/internal/strategy/pe_band	6.836s
ok  	github.com/newthinker/atlas/internal/strategy/pe_percentile	6.725s
ok  	github.com/newthinker/atlas/internal/strategy/price_percentile	6.753s
ok  	github.com/newthinker/atlas/internal/text	6.621s
ok  	github.com/newthinker/atlas/internal/valuation	6.574s
```

目标用例 `-run 'TestHestiaAlertRules_|TestExampleConfigDeclaresHestiaRules' ./cmd/atlas/`：exit 0，PASS 22 / FAIL 0 / SKIP 0 / RUN 22。

## R2-2. Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | 六项含 message 逐字相等；规则名不重复 | `DeclaredInExampleConfig` 表加 message 列，`assert.Equal(t, tt.message, r.Message)`；5 条期望文案与 yaml 逐字一致（测试在未改动的 yaml 上为绿）；`loadExampleRules` 断言不重复。Z11、Z12、Z17（末尾加句号）、Z18（改一个字）全部被它杀死 | PASS |
| functional[1] | 五条 expr 可求值，含等号例 | `ExprEvaluable` 10 子测试未变；Z1/Z2（`or`）、Z4（label 选择器）、Z6/Z7 杀死 | PASS |
| functional[2] | E2E failed/ 与 done/ 各 1 份 | `E2EFailedItem` 新增 done/contract.json；**Z3（Y6 求和基键）现在被 E2EFailedItem 杀死**（第 1 轮它只被 Declared/ExprEvaluable 杀）；Z13（去 state 展开）仍唯一由它杀死 | PASS |
| boundary[0] | 空队列三条 false | `E2EEmptyQueueDoesNotFire` 未变 | PASS |
| boundary[1] | queue_blind 可恢复 | `E2EQueueBlindRecovers` 未变；Z14 杀死 | PASS |
| error_handling[0] | C2 双向隔离 | `E2EBlindRulesIsolated` 未变；Z14/Z15 杀死 | PASS |
| non_functional[0] | yaml 注释三条理由 | yaml 与第 1 轮逐字节相同，第 1 轮核实结论延续 | PASS |
| non_functional[1] | paste 原文 + 全量全绿 | `human_sync_required.paste`（2582 字节）与 merge 对 yaml 的 42 行新增逐字节相等，在基线 yaml 中恰出现 1 次；全量见 R2-1 | PASS |

## R2-3. 变异（在新测试上重跑第 1 轮同一套 + 2 个 message 细粒度变异）

作用于隔离 worktree，逐个 sha256 还原一致，收尾 `git status --porcelain` 与开始时相同，HEAD 未变。18 个全部 KILLED：

| 变异 | 杀死它的测试 |
|---|---|
| Z1 queue_blind `a == 0 or b == 0` | Declared, ExprEvaluable, E2EQueueBlindRecovers, E2EBlindRulesIsolated |
| Z2 failed `a > 0 or b > 0` | Declared, ExprEvaluable, E2EFailedItem |
| Z3 failed 求和基键（Y6） | Declared, ExprEvaluable, **E2EFailedItem** |
| Z4 label 选择器 | Declared, ExprEvaluable, E2EFailedItem |
| Z5 blind 同名 | 全部 6 个 |
| Z6 stuck `>=` / Z7 processing 阈值 5 | Declared, ExprEvaluable |
| Z8 severity / Z9 for / Z10 cooldown | Declared |
| **Z11 queue_stuck message 改任意串** | **Declared**（第 1 轮存活） |
| **Z12 db_blind message 换成 queue_blind 文案** | **Declared**（第 1 轮存活） |
| Z13 snapshot 不做 state 展开 | E2EFailedItem |
| Z14 queue 失败 up 仍 1 | E2EQueueBlindRecovers, E2EBlindRulesIsolated |
| Z15 DB 失败 up 仍 1 | E2EBlindRulesIsolated |
| Z16 processing_stuck 改名 | Declared, ExprEvaluable, E2EEmptyQueueDoesNotFire |
| Z17 queue_blind message 末尾加「。」 | Declared |
| Z18 processing_stuck message 改一个字 | Declared |

附注：discovery `key_findings[4]` 把 Z11 描述为「db_blind 文案改为任意串」，我的 Z11 改的是 queue_stuck；标签不一致不影响结论（两种都被杀，Z17/Z18 另证逐条生效）。

## R2-4. 复现命令

```bash
B=4cd6cccf2b060ccc718d841a6b17d5eb41f3b045
git worktree add --detach ../wt-verify "${B}" && cd ../wt-verify
GOTOOLCHAIN=local go test -count=1 ./... > /tmp/all.txt 2>&1; echo "exit=$?"; grep -cE '^ok ' /tmp/all.txt; grep -cE '^(FAIL|--- FAIL)' /tmp/all.txt
```

---

# 第 1 轮（保留原文，含验证者订正）

> **验证者订正（2026-09-17，rejected 落盘之后）**：DoD 作者（Leader）事后说明，functional[0] 的「description」本意是**任务 description 里列出的值**，不含 message。据此：
> - 第 4 节「description 即 message」是我对歧义措辞的**解读**，不是 DoD 作者的原意；「dev 静默省略 description、未申报」这一指责**不成立**——dev 的映射（name/expr/for/cooldown/severity 逐项相等）与 DoD 本意一致。
> - 按事实，缺口根因是 DoD 措辞歧义，reason_class 更准确应为 `dod_defect`；任务文件里的 `task_defect` 已在 rejected 状态落盘，验证者与 Leader 均无权再改，此处如实记明。
> - 不变的部分：变异 Z11/Z12 存活（critical 规则 message 串位时全部测试仍绿）是观察事实；Leader 已采纳并把 message 逐字相等、E2E done/ 放 1 份写进订正后的 DoD。

- 验证者：test-m4-b
- 判定：**REJECTED**，reason_class=`task_defect`
- 判定对象：master @ `0af7c8d191e9dac81de823e2b6b07543dd98cc04`（= verify_baseline.head，判定时主仓库 HEAD 相同；discovery sha256 `dbf5f86c…cba8` 与基线一致）
- 验证环境：隔离 worktree `../wt-verify-TASK-005-b`（detached @ 上述全 sha，已拆）
- assignment_epoch：1
- 范围：merge 相对第一父 numstat = `cmd/atlas/hestia_alert_rules_test.go 181/0`、`configs/config.example.yaml 42/0`，与 `writes` 一致

### 1. 亲跑全量（验收判据一，non_functional[1]）

`GOTOOLCHAIN=local go test -count=1 ./...` 输出重定向到文件后**分别**取值（不经管道取退出码）：

| 量 | 值 |
|---|---|
| 退出码 | **0** |
| `^ok ` 行 | 65 |
| `^(FAIL\|--- FAIL)` 行 | 0 |
| `^?`（无测试文件）行 | 0 |
| 输出总行数 | 65 |
| `go list ./...` 包数 | 65 |

自洽：ok 65 + FAIL 0 + ? 0 = 总行 65 = 包数 65。尾部输出：

```
ok  	github.com/newthinker/atlas/internal/strategy/pe_band	6.814s
ok  	github.com/newthinker/atlas/internal/strategy/pe_percentile	6.687s
ok  	github.com/newthinker/atlas/internal/strategy/price_percentile	6.714s
ok  	github.com/newthinker/atlas/internal/text	6.732s
ok  	github.com/newthinker/atlas/internal/valuation	6.666s
```

目标用例：`go test -count=1 -v -run 'TestHestiaAlertRules_|TestExampleConfigDeclaresHestiaRules' ./cmd/atlas/` ⇒ exit 0，`--- PASS` 22 / `--- FAIL` 0 / `--- SKIP` 0 / `=== RUN` 22（本任务 6 顶层 + 15 子测试 = 21，另 1 条为既有 TestExampleConfigDeclaresHestiaRules）。

### 2. Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 核对 | 判定 |
|---|---|---|---|
| **functional[0]** | 经 config.Load 路径读出五条，**name/expr/for/cooldown/severity 与 description 逐项相等** | `TestHestiaAlertRules_DeclaredInExampleConfig`：name/expr/for/cooldown/severity 用 `assert.Equal` 逐项比；**message 只 `assert.NotEmpty`** | **FAIL：description（= message）未覆盖**，见第 4 节 |
| functional[1] | 五条 expr 经 Evaluate 应触发 true / 不应触发 false，等号各一例 | `TestHestiaAlertRules_ExprEvaluable` 10 子测试（24 / 0 / 0.5 / db_up 1 / queue_up 1 等号或反例齐全）；变异 Z1（`a == 0 or b == 0`）、Z2（`a > 0 or b > 0`）、Z4（label 选择器）均使其转红 | PASS |
| functional[2] | 端到端 failed 1 份 ⇒ true，删掉 ⇒ false | `TestHestiaAlertRules_E2EFailedItem`（真实 HestiaCollector + QueueHealthOf + Registry.Snapshot，pending/ 空）；变异 Z13（Snapshot 不做 state 展开）**只**被它杀死 ⇒ 它确实在证明 `hestia_queue_items_failed` 键在真实 Snapshot 中存在 | PASS（关于 Y6 见第 5 节） |
| boundary[0] | 四目录空 ⇒ 三条 false | `TestHestiaAlertRules_E2EEmptyQueueDoesNotFire`，先 `require hestia_queue_up == 1` 防空真 | PASS |
| boundary[1] | 删 pending/ ⇒ queue_blind true，建回 ⇒ false | `TestHestiaAlertRules_E2EQueueBlindRecovers`；Z14（失败时 queue_up 仍报 1）杀死 | PASS |
| error_handling[0] | 规则层 C2 双向隔离 | `TestHestiaAlertRules_E2EBlindRulesIsolated`；Z14、Z15（DB 失败 db_up 仍报 1）均杀死 | PASS |
| non_functional[0] | 注释三条理由 | review：yaml 注释写明 ④⑤ 拆两条不同名（evaluator 的 pending/lastFired 按名存——我读 `internal/alert/evaluator.go:86/93/96/111/136` 均以 `rule.Name` 为键，属实）、up 而非 `_errors_total`（`queueErrors` 为 `prometheus.Counter`，属实）、`hestia_queue_items_failed` 来自 Snapshot state 展开 TASK-002（`internal/metrics/snapshot.go` 的 `addState`，属实）；「年龄指标在目录为空时不输出」与 `hestia_collector.go` `collectQueue` 的 `IsZero` 判断一致 | PASS |
| non_functional[1] | discovery 列出生产侧粘贴原文；亲跑全量全绿 | `human_sync_required.paste`（2582 字节）与 merge 对 `configs/config.example.yaml` 的 42 行新增**逐字节相等**，且在基线树 yaml 中作为连续块恰好出现 1 次；全量见第 1 节 | PASS |

### 3. 变异测试（独立设计，作用于隔离 worktree；逐个 sha256 还原一致，收尾 `git status --porcelain` 与开始时相同，worktree HEAD 未变）

测试命令：`go test -count=1 -run 'TestHestiaAlertRules_|TestExampleConfigDeclaresHestiaRules' ./cmd/atlas/`

| 变异 | 结果 | 杀死它的测试 |
|---|---|---|
| Z1 queue_blind 改 `hestia_queue_up == 0 or hestia_db_up == 0` | KILLED | Declared, ExprEvaluable, E2EQueueBlindRecovers, E2EBlindRulesIsolated |
| Z2 failed 改 `hestia_queue_items_failed > 0 or hestia_queue_items_pending > 0` | KILLED | Declared, ExprEvaluable, E2EFailedItem |
| Z3 failed 改求和基键 `hestia_queue_items > 0`（dev 的 Y6） | KILLED | Declared, ExprEvaluable（E2E 未红，与 discovery 一致） |
| Z4 failed 改 label 选择器 | KILLED | Declared, ExprEvaluable, E2EFailedItem |
| Z5 db_blind 改名为 queue_blind（同名） | KILLED | 全部 6 个 |
| Z6 stuck `>` → `>=` | KILLED | Declared, ExprEvaluable |
| Z7 processing 阈值 0.5 → 5 | KILLED | Declared, ExprEvaluable |
| Z8 queue_blind severity critical → warning | KILLED | Declared |
| Z9 stuck for 10m → 5m | KILLED | Declared |
| Z10 stuck cooldown 24h → 6h | KILLED | Declared |
| **Z11 queue_stuck message 改成任意文案** | **SURVIVED** | — |
| **Z12 db_blind message 换成 queue_blind 的文案（串位）** | **SURVIVED** | — |
| Z13 `snapshot.go` 不做 state 展开 | KILLED | E2EFailedItem（唯一） |
| Z14 `collectQueue` 失败时 queue_up 仍报 1 | KILLED | E2EQueueBlindRecovers, E2EBlindRulesIsolated |
| Z15 DB 失败时 db_up 仍报 1 | KILLED | E2EBlindRulesIsolated |
| Z16 processing_stuck 改名 | KILLED | Declared, ExprEvaluable, E2EEmptyQueueDoesNotFire |

16 个，KILLED 14，SURVIVED 2——两个存活均落在同一条 DoD 缺口上。

### 4. 缺陷：functional[0] 的 description 逐项相等未被测试覆盖（task_defect）

- DoD 原文：「name/expr/for/cooldown/severity **与 description 逐项相等**（table-driven 断言）」。
- 配置结构 `internal/config.AlertRule` 与 `alert.Rule` 的字段恰为 Name / Expr / For / Severity / Message / Cooldown——DoD 列举的五项之外唯一剩下的描述性字段就是 `Message`，任务描述也写「message 沿用计划原意」，计划 `2026-09-17-hestia-queue-observability-and-trigger.md` 第 505/512/521 行给出了前三条的 message 原文（与 yaml 逐字相同）。⇒ description 指 message，期望值是可得的。
- 测试表只有 `name, expr, severity, forDur, cooldown` 五列，message 仅 `assert.NotEmpty`。**Z12 实证**：把 critical 规则 `hestia_db_blind` 的 message 换成 `hestia_queue_blind` 的文案（值班人会被指去查队列目录而不是库），全部测试仍绿。
- discovery `done_criteria_map.functional[0]` 写作「name/expr/for/cooldown/severity 逐项相等」，**静默省略了 description**，未经 `blocked_clarification` 提问，也未在 key_findings / decisions 中申报。
- 判 `task_defect` 而非 `dod_defect`：DoD 用词「description」与字段名「message」不同，但在只有一个候选字段、且任务描述明写 message 的情况下，指代不构成矛盾或不可测；若 dev 认为有歧义，正确路径是提问而不是降级为 NotEmpty。

**修复方向**（仅改 `cmd/atlas/hestia_alert_rules_test.go`，yaml 不需改）：表加 `message` 列，五条各填期望文案，`assert.NotEmpty` 换成 `assert.Equal(t, tt.message, r.Message)`；修后 Z11/Z12 应转 KILLED。

### 5. 对 Leader 所问「Y6 是否构成 functional[2] 缺口」的判断：**不构成**

- functional[2] 的职责是证明**样例配置里的 failed 规则**在真实 Snapshot 上可被点亮/熄灭。它读的是从 yaml 加载的规则，而该规则的 expr 已由 Declared 用 `assert.Equal` 钉死为 `hestia_queue_items_failed > 0`（Z3 被 Declared 与 ExprEvaluable 杀死）。在 expr 被钉死的前提下，E2E 的红绿就是「`hestia_queue_items_failed` 键在真实 Snapshot 中存在且值正确」的证据——Z13（去掉 state 展开）**只**被 E2EFailedItem 杀死，证明它守的正是这件事，且是唯一守它的测试。
- DoD 规定的夹具就是「failed/ 放 1 份、pending/ 空」，dev 照做。
- **不阻断的建议**：`snapshot.go` 注释写明生产上求和键「因历史 done/ 项恒为真」；在 E2E 夹具的 done/ 里放 1 件，E2EFailedItem 就能独立区分求和键与 failed 键，更贴近生产形态。

### 6. 复现命令（锚为全 sha）

```bash
B=0af7c8d191e9dac81de823e2b6b07543dd98cc04
git worktree add --detach ../wt-verify "${B}" && cd ../wt-verify
GOTOOLCHAIN=local go test -count=1 ./... > /tmp/all.txt 2>&1; echo "exit=$?"; grep -cE '^ok ' /tmp/all.txt; grep -cE '^(FAIL|--- FAIL)' /tmp/all.txt
## Z12：db_blind 的 message 换成 queue_blind 的文案后，目标用例仍全绿
```
（zsh 下锚写 `"${B}:…"` / `"${B}"`，避免 `$B:x` 被当作历史修饰符。）
