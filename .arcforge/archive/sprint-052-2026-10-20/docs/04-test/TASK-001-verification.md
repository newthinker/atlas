# TASK-001 验证报告（QA 返工复验，test-pw-c）

## 首轮结论摘要（test-pw-a，原文已被本报告覆盖）

- 被测树 `f2fe680c52b896ef86a25df0ef3622623d3a337a`，dev 提交 `f646800`；结论 **VERIFIED**，附 3 条非阻断发现。独立变异 65 个，KILLED 61 / SURVIVED 4，存活体都做了辨析（等价或已记录）。
- 之后 QA round2（`.arcforge/docs/05-review/qa-review-round2.md`）转 review_fix，给出 2 条 fix_items：QA-1（ParseRule 放行 NaN/+Inf）、QA-2（缺 prev{2,2,T0} 用例，`SentInTier` 写死为 2 的变异存活）。

## 本轮被测对象

- 被测树：`ebc366e43a6deda8dc28ae8974b2e3fa77bb3c1b`（feature/pricewatch，等于 `verify_baseline.head`）；隔离 worktree `../wt-verify-TASK-001-c`（detached）。
- 返工提交 `5b69fe101ce4c218572a9c0225fac16c085cfb84`（合入 `56c7c6f`）改了 rule.go / rule_test.go / state_test.go，都在 `writes` 内；在被测树上 `git diff 5b69fe1 ebc366e` 对 4 个声明文件输出为空。
- discovery sha256 `4be0411300e71c1b2ae615e45a5bf8d321d4d63c27bbd5cd74e9e8540a898a47` 与 baseline 相同。
- 工具链 `GOTOOLCHAIN=local`。

## 结论：VERIFIED

## 1. 门禁对照

| 项 | 首轮 | 本轮（ebc366e） |
|---|---|---|
| `go test -race -count=1 -cover ./internal/pricewatch/` | ok | ok，包 94.1% |
| rule.go / state.go 各函数覆盖 | 全 100% | 全 100%（Span/Bar/ParseRule/Measure/Tier/Decide） |
| `go vet` / `gofmt -l internal/pricewatch/` | 无输出 | 无输出 |
| 本任务顶层测试 | 7 PASS | 7 PASS / 0 FAIL |
| `TestParseRule_Rejects` 子测试 | 8 | 14（新增 NaN/±Inf × 2 字段） |
| `TestDecide` 行数 | 12 | 13（新增 QA-2 行） |
| `go test -count=1 ./cmd/atlas/ -run Pricewatch` | — | ok |

## 2. done_criteria 覆盖矩阵（8 条，整份重验）

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | Measure drop 50 / rise 100 | `TestMeasure` | PASS |
| functional[1] | Tier 11 个边界点 + NaN/±Inf→0 | `TestTier_Boundaries` | PASS（本轮 Tier 未改动） |
| functional[2] | Decide 计划 10 例 + 恰满间隔 + 用尽后跌深一档，每行断言 next 三字段与 kind | `TestDecide`（13 行，每行比较 Tier/SentInTier/LastSentAt.Equal/kind） | PASS |
| functional[3] | Window Span/Bar | `TestWindow` | PASS |
| boundary[0] | step_pct 0 合法、Notify 默认/原样、Type/Window 常量 | `TestParseRule_Valid` | PASS（R5 变异 `>=0`→`>0` 被它杀死，说明 0 合法这条仍有守卫） |
| boundary[1] | Decide 不改入参；零值时间 + t=1 立即 SendNew | `TestDecide_DoesNotMutatePrev`、`TestDecide/3` | PASS |
| error_handling[0] | ParseRule 8 种非法输入报错，且含 id 与字段名 | `TestParseRule_Rejects`（断言含 `day_drop` 与字段名） | PASS |
| non_functional[0] | race / vet / gofmt / 覆盖率 ≥80% | §1 | PASS |

## 3. fix_items 核对矩阵

| fix_item | 实现位置 | 测试守卫 | 独立变异 |
|---|---|---|---|
| QA-1 threshold_pct/step_pct 拒绝 NaN/+Inf | `rule.go` ParseRule：`!(x > 0) \|\| IsInf(x,1)`、`!(x >= 0) \|\| IsInf(x,1)` | `TestParseRule_Rejects` 新增 6 行（-Inf 两行在修复前已拦，作边界钉住） | R1 threshold 改回 `<= 0` → KILLED（threshold_NaN）；R2 去 IsInf → KILLED（threshold_+Inf）；R3 step 改回 `< 0` → KILLED（step_NaN）；R4 去 IsInf → KILLED（step_+Inf） |
| QA-2 补 prev{2,2,T0}、t=2、now=T0+1h → SendRepeat、next{2,3,now} | 只补用例（`state_test.go`） | `TestDecide/QA-2_档内第_3_次提醒` | S1 `SentInTier: prev.SentInTier+1` → `2`：KILLED；**消融**：S1 + 删掉 QA-2 那一行 → SURVIVED ⇒ 这一行是该变异的唯一守卫。对照：只删这一行、不改源码 → 绿 |

dev 自报 4/4 KILLED 加 QA-2 消融：我独立复现，结论一致。

## 4. 独立变异（隔离：worktree + `go test -overlay`）

harness：`/private/tmp/claude-501/-Users-zuowei-workspace-go-src-github-com-newthinker-atlas/b3c3a6ba-7de9-492d-9e24-953680e5a079/scratchpad/t001c-mut/run.py`。每个变异体都断言替换点恰好出现 1 次、打印 diff，并先过 `go vet -overlay`。主仓库 4 个声明文件的 sha256 与 `git status --porcelain` 指纹在运行前后一致。

| 变异 | 结果 |
|---|---|
| R1 `!(ThresholdPct > 0)` → `ThresholdPct <= 0` | KILLED |
| R2 threshold 去掉 `IsInf(…,1)` | KILLED |
| R3 `!(StepPct >= 0)` → `StepPct < 0` | KILLED |
| R4 step 去掉 `IsInf(…,1)` | KILLED |
| R5 `!(StepPct >= 0)` → `!(StepPct > 0)` | KILLED（`TestParseRule_Valid`，step 0 合法） |
| S1 `SentInTier` 写死为 2 | KILLED |
| S1 + 删 QA-2 行（消融） | SURVIVED（预期内，证明唯一守卫） |
| 只删 QA-2 行（对照） | 绿（预期内） |

## 5. 复现命令（锚为全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-001-c ebc366e43a6deda8dc28ae8974b2e3fa77bb3c1b
cd ../wt-verify-TASK-001-c
GOTOOLCHAIN=local go test -race -count=1 -cover ./internal/pricewatch/
python3 <scratchpad>/t001c-mut/run.py "$PWD" <scratchpad>/t001c-mut <主仓库路径>
```
