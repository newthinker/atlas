# TASK-002 验证报告（QA 返工复验，test-pw-c）

## 首轮结论摘要（test-pw-a，原文已被本报告覆盖）

- 被测树 `f3d3223136084113ea104d57891d827d5f768419`，dev 提交 `eb206b6`；结论 **VERIFIED**。
- 附 1 条非阻断发现：`TestLoadConfig_Rejects` 中 10 行错误字样断言是空洞的——`t.TempDir()` 路径里含子测试名，字段名断言被路径顺带满足（即后来 QA 的 QA-4）。
- 之后 QA round2（`.arcforge/docs/05-review/qa-review-round2.md`）判 REJECT，转 review_fix，给出 4 条 fix_items：QA-14、R1-2、QA-4、R1-3。

## 本轮被测对象

- 被测树：`f8e37c2ce98451045361b8a7aa2acfa62299783d`（feature/pricewatch，等于 `verify_baseline.head`，HEAD 无漂移）；隔离 worktree `../wt-verify-TASK-002-c`（detached）。
- 返工提交 `769bdbd77eda0b92c7c86e6c56783e0f7b075eb4`（合入 `943dacc`）只改了 `config.go` / `config_test.go`，两者都在 `writes` 内；在被测树上 `git diff 769bdbd f8e37c2` 对这两个文件输出为空。`testdata/valid.yaml`、`configs/pricewatch.yaml` 自 `eb206b6` 起未改。之后 `rule.go` 被 TASK-001 返工改过（`5b69fe1`），不在本任务范围内，本任务测试在其上全绿。
- discovery sha256 `6417c8d02d3f14119946cfa28d1274a6d80a2d325212a8c6c5906617bb9e2f0e` 与 `verify_baseline.discovery_sha256` 相同，无漂移。
- 工具链 `GOTOOLCHAIN=local`。

## 结论：VERIFIED

另有 1 条非阻断观察（见 §5）：discovery 自称 QA-14 的报错「不回显原值」，但没有任何测试守卫这一点（M5 存活）。这条不是 DoD 项，也不是 fix_item，占位值本身也不是密钥。

## 1. 本轮与首轮门禁对照

| 项 | 首轮（f3d3223） | 本轮（f8e37c2） |
|---|---|---|
| `go test -race -count=1 -cover ./internal/pricewatch/` | ok，93.1% | ok，**94.1%** |
| config.go 各函数覆盖 | LoadConfig 91.7%，其余 100% | **7 个函数全部 100%**（Unmarshal 错误分支现在由 unknown key 行覆盖） |
| `go vet ./internal/pricewatch/` | 无输出 | 无输出（rc=0） |
| `gofmt -l internal/pricewatch/` | 无输出 | 无输出 |
| 本任务顶层测试 | 10 PASS | **11 PASS / 0 FAIL**（新增 `TestLoadConfig_PlaceholderRejected`） |
| `TestLoadConfig_Rejects` 行数 | 18 | **24**（新增 6 行），全部 PASS |
| `GOTOOLCHAIN=local go test -count=1 ./cmd/atlas/ -run Pricewatch` | — | ok，11 个顶层测试 PASS（`-v` 确认确实跑了，不是 no tests to run） |

## 2. done_criteria 覆盖矩阵（8 条，整份重验）

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | spec §3 示例逐项读入 + 非默认值 fixture | `TestLoadConfig_Valid`、`TestLoadConfig_NonDefaults` | PASS |
| functional[1] | 规则合并（原位替换、BTC 防别名、追加、notify 整条替换）、symbol 大写 | `TestLoadConfig_Merge`、`TestLoadConfig_SymbolNormalized` | PASS |
| functional[2] | `UnsupportedItems` 返回 `["stock:600519"]`，CryptoAssets 不受影响 | `TestUnsupportedItems` | PASS |
| functional[3] | `configs/pricewatch.yaml` 可加载，且与 testdata 结果相同 | `TestExampleConfigLoads` | PASS（**严格解析下**仍能加载，见 §4） |
| boundary[0] | ai.timeout 默认 60s、fail_alert_after 默认 30m、heartbeat 空合法、strategy 大小写 | `TestLoadConfig_Defaults` | PASS（R1-3 只拒绝显式 ≤0，SetDefault 路径不受影响） |
| error_handling[0] | Rejects：13 行 + sources/quote/资产级 check_interval，字样断言 | `TestLoadConfig_Rejects`（24 行） | PASS；字样断言**不再被路径满足**（§3 的 N 组） |
| error_handling[1] | `${ENV}` 严格匹配、明文不进错误文本、proxy 原样 | `TestLoadConfig_TelegramEnv` | PASS |
| non_functional[0] | race / vet / gofmt / 覆盖率 ≥80% | §1 | PASS |

## 3. fix_items 核对矩阵

| fix_item | 实现位置（f8e37c2） | 测试守卫 | 独立变异 |
|---|---|---|---|
| QA-14 拒绝 `REPLACE_WITH_` 占位值，报错指向 plist 注释 | `config.go:206,220-223`（`expandEnv`，bot_token/chat_id 共用） | `TestLoadConfig_PlaceholderRejected`（两个字段各一，断言字段名、env 名、`REPLACE_WITH_`、`~/Library/LaunchAgents/`） | M2 去掉检查 → KILLED；M6 删掉 LaunchAgents 指引 → KILLED。plist 里的变量名 `PRICEWATCH_TELEGRAM_BOT_TOKEN`/`CHAT_ID` 与占位值 `REPLACE_WITH_BOT_TOKEN`/`CHAT_ID` 已核对（`deploy/launchd/com.newthinker.atlas.pricewatch.plist:38-41`） |
| R1-2 `v.UnmarshalExact` + 未知键用例 | `config.go:72` | Rejects 的 `unknown key`（health.heartbeat_al）、`unknown rule key`（规则里的 typo） | M1 改回 `Unmarshal` → KILLED（这两行变红） |
| QA-4 断言剥路径前缀 | `config_test.go` `errBody`；Rejects 全部行改用它 | 自身就是守卫；见下面 N 组 | 12 个「删字段名」变异体：新测试**全部 KILLED**，旧测试（errBody 退化为 `err.Error()`）**全部 SURVIVED** ⇒ 修复有因果效力 |
| R1-3 fail_alert_after>0、ai.timeout>0 | `config.go:101-106` | Rejects 新增 `fail_alert_after 0s/-1m`、`ai.timeout 0s/-5s` | M3、M4（`<=0`→`<0`）→ KILLED（0s 行变红） |

## 4. 现有配置在严格解析下

- `configs/pricewatch.yaml`：`TestExampleConfigLoads` PASS（经 `LoadConfig`，现在用的是 UnmarshalExact）。
- `testdata/valid.yaml`：所有 `mustLoad(validYAML)` 类用例（Valid/Merge/Defaults/Unsupported 等）PASS。
- cmd/atlas：唯一外部调用方的测试 `-run Pricewatch` 11 个 PASS。

## 5. 独立变异（隔离：worktree + `go test -overlay`；变异体放在 scratchpad）

harness：`/private/tmp/claude-501/-Users-zuowei-workspace-go-src-github-com-newthinker-atlas/b3c3a6ba-7de9-492d-9e24-953680e5a079/scratchpad/t002c-mut/run.py`。每个变异体都断言「替换点恰好出现 1 次」、打印 diff，并先过 `go vet -overlay`（有效性闸）。主仓库里 config.go/config_test.go/rule.go 的 sha256 与 `git status --porcelain` 指纹在运行前后**一致**。

| 变异 | 内容 | 结果 |
|---|---|---|
| M1 | `UnmarshalExact` → `Unmarshal` | KILLED（unknown_key、unknown_rule_key） |
| M2 | 占位值检查改成 `if false && …` | KILLED（PlaceholderRejected 两个子测试） |
| M3 | `FailAlertAfter <= 0` → `< 0` | KILLED（fail_alert_after_0s） |
| M4 | `AI.Timeout <= 0` → `< 0` | KILLED（ai.timeout_0s） |
| M5 | 占位值报错末尾追加 `val=%s`（回显原值） | **SURVIVED**——见下方观察 |
| M6 | 删掉报错里的 `~/Library/LaunchAgents/` 指引 | KILLED |
| N×12 | 从错误文案里删掉字段名：type、window、max_per_tier、repeat_interval、check_interval、资产级 check_interval、heartbeat_at、sources、quote、symbol、fail_alert_after、ai.timeout（这 12 行的子测试名都包含 want 字样） | 新测试 **12/12 KILLED**；旧测试 **12/12 SURVIVED** |

计入对 dev 自报「4/4 KILLED」的核对：M1–M4 独立复现，结论一致。

**观察（非阻断）**：M5 存活。discovery `qa_fix[0].items[QA-14].fix` 写着「不回显原值」，当前实现确实不回显，但没有测试守卫这一点。QA-14 原文与任务 `fix_items` 只要求「报错指向 plist 注释」，DoD error_handling[1] 的「不回显明文」针对的是明文 token 行（已由 TelegramEnv 守卫）；占位值本身不是密钥。所以这条不阻断，作为后续可以补的断言记录在此（例如断言 msg 不含 `SOMETHING`）。

## 6. 复现命令（锚为全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-002-c f8e37c2ce98451045361b8a7aa2acfa62299783d
cd ../wt-verify-TASK-002-c
GOTOOLCHAIN=local go test -race -count=1 -cover ./internal/pricewatch/
GOTOOLCHAIN=local go test -count=1 -v ./cmd/atlas/ -run Pricewatch
python3 <scratchpad>/t002c-mut/run.py "$PWD" <scratchpad>/t002c-mut <主仓库路径>
```
