# 需求 ↔ DoD 追溯矩阵

需求编号见 `docs/01-design/requirements-analysis.md`。本文件由脚本生成并机器检查。

## 需求 → DoD

| R | 覆盖的 DoD 条目 |
|---|---|
| R1 | TASK-001.functional[0], TASK-001.functional[1], TASK-001.boundary[0], TASK-001.boundary[1], TASK-001.error_handling[0] |
| R2 | TASK-002.functional[0], TASK-002.error_handling[0], TASK-002.non_functional[0], TASK-007.functional[0] |
| R3 | TASK-002.boundary[0], TASK-002.boundary[1], TASK-002.boundary[2] |
| R4 | TASK-001.functional[2], TASK-003.functional[0], TASK-003.functional[2], TASK-003.boundary[0], TASK-003.boundary[1] |
| R5 | TASK-001.functional[2], TASK-003.functional[1], TASK-003.functional[3], TASK-003.boundary[0], TASK-003.boundary[1], TASK-005.functional[2] |
| R6 | TASK-004.functional[0], TASK-005.functional[0] |
| R7 | TASK-004.functional[1], TASK-004.functional[3], TASK-004.boundary[0], TASK-004.boundary[1] |
| R8 | TASK-005.functional[0], TASK-005.functional[1], TASK-005.functional[2], TASK-005.boundary[0] |
| R9 | TASK-005.boundary[1], TASK-006.boundary[0] |
| R10 | TASK-006.functional[0], TASK-006.functional[2], TASK-006.boundary[0], TASK-006.non_functional[0] |
| R11 | TASK-001.error_handling[0], TASK-002.error_handling[0], TASK-004.error_handling[0], TASK-006.functional[0], TASK-006.functional[1], TASK-006.error_handling[1] |
| R12 | TASK-001.non_functional[0], TASK-002.non_functional[1], TASK-003.non_functional[0], TASK-004.non_functional[0], TASK-005.functional[1], TASK-005.non_functional[0], TASK-006.functional[2], TASK-006.non_functional[0], TASK-007.functional[0], TASK-007.boundary[0], TASK-007.error_handling[0] |
| R13 | TASK-001.functional[1], TASK-007.functional[1], TASK-007.non_functional[0], TASK-007.non_functional[1] |
| R14 | TASK-006.error_handling[0] |
| R15 | TASK-003.functional[1], TASK-003.functional[2], TASK-003.boundary[1], TASK-004.boundary[0], TASK-005.functional[1], TASK-007.functional[1] |
| R16 | TASK-004.functional[1], TASK-004.functional[2], TASK-005.functional[0] |
| R17 | TASK-005.functional[1], TASK-005.functional[2] |

## DoD → 需求

| DoD | R | 摘要 |
|---|---|---|
| TASK-001.functional[0] | R1 | 合法配置加载：缺省 `source.aktools_url` = `http://127.0.0.1:8180`；缺省 … |
| TASK-001.functional[1] | R1,R13 | 仓库自带 `configs/bank-monitor.yaml` 可被 `LoadConfig("../../confi… |
| TASK-001.functional[2] | R4,R5 | types：`Indicators` 顺序为 NPL、Coverage、CET1；`Label()` 分别为 不良率/拨… |
| TASK-001.boundary[0] | R1 | 拒绝表逐条断言错误文案：空列表「banks 不能为空」、market=US「仅支持 CN_A / HK」、A 股 `'6… |
| TASK-001.boundary[1] | R1 | 六个阈值字段（npl_max、coverage_min、cet1_min、deterioration.npl_up/co… |
| TASK-001.error_handling[0] | R1,R11 | 配置文件不存在 ⇒ 错误含 `reading bank config`；YAML 语法错误 ⇒ 返回错误；校验失败的错误… |
| TASK-001.non_functional[0] | R12 | `go vet ./internal/bank/` 无输出；`go test -count=1 ./internal/b… |
| TASK-002.functional[0] | R2 | httptest 回放样本：请求路径 `/api/public/stock_financial_analysis_ind… |
| TASK-002.boundary[0] | R3 | parseEMRows 容错：字符串数值 "1.23" ⇒ 1.23；null ⇒ NaN；缺键 ⇒ NaN；REPOR… |
| TASK-002.boundary[1] | R3 | **全 null 列不算缺失**：某键在每一行都存在且值全为 null ⇒ 该键**不在** `MissingField… |
| TASK-002.boundary[2] | R3 | 非数值统统 ⇒ NaN、绝不为 0 或 ±Inf：`"-"`、`""`、`"Inf"`、`"-Infinity"`、`"… |
| TASK-002.error_handling[0] | R2,R11 | 错误表：HTTP 500 ⇒ 含 `HTTP 500`；`[]` 与 `null` 响应体 ⇒ `无数据`；全部行日期非… |
| TASK-002.non_functional[0] | R2 | `NewEMSource(..).hc.Timeout == 60*time.Second` 有测试断言。三个取数字段名… |
| TASK-002.non_functional[1] | R12 | `go vet ./internal/bank/` 无输出；`go test -count=1 ./internal/b… |
| TASK-003.functional[0] | R4 | cmb 实测序列（2025-03-31…2026-06-30，每期三项都有）：最新期 2026-06-30；NPL {0… |
| TASK-003.functional[1] | R15,R5 | **按指标回退（真实数据 600919.SH）**：取 2024-06-30…2025-09-30 的真实序列（2025… |
| TASK-003.functional[2] | R15,R4 | **跨空期的环比/同比（真实数据 002142.SZ 截至 2026-06-30）**：2026-03-31 CET1 … |
| TASK-003.functional[3] | R5 | 阈值预警（NPL > max、Coverage < min、CET1 < min）与恶化预警（NPL 环比 +0.11、… |
| TASK-003.boundary[0] | R4,R5 | 严格不等号：NPL=1.5、Coverage=150、CET1=8.5 均不预警。**浮点边界三例**：由 Analyz… |
| TASK-003.boundary[1] | R4,R5,R15 | NaN：全 NaN 的尾期不算最新期；某指标全序列 NaN ⇒ Value/QoQ/YoY 为 NaN、Period 为… |
| TASK-003.non_functional[0] | R12 | Analyze 不修改入参切片；`go vet ./internal/bank/` 无输出；`go test -coun… |
| TASK-004.functional[0] | R6 | A+H 去重：招商银行 + 招商银行H(→600036.SH) + 邮储银行 ⇒ 2 个结果，600036.SH 只 F… |
| TASK-004.functional[1] | R7,R16 | Summarize 分组：Current（Latest=统计期）按 rank_by 由优到劣（NPL 升序；Covera… |
| TASK-004.functional[2] | R16 | **统计期取众数（D15）**：10 家 Latest=2026-06-30、1 家 Latest=2026-12-31… |
| TASK-004.functional[3] | R7 | Stat：N、均值、中位数（奇数取中位、偶数取中间两数均值）、最优/最差名称与值，方向随指标（拨备覆盖率越高越好）。… |
| TASK-004.boundary[0] | R7,R15 | 统计只用当期值：NaN 与 `Ind[k].Period ≠ 统计期` 的回退值都不进统计（N 只计合格值）且在按该指标… |
| TASK-004.boundary[1] | R7 | 并列值排名稳定（保持配置顺序）。… |
| TASK-004.error_handling[0] | R11 | 单主体失败不影响其他：失败主体 `Err` 保留原错误、`Latest` 为零、`Ind` 三项 Value **均为 … |
| TASK-004.non_functional[0] | R12 | `go vet ./internal/bank/` 无输出；`go test -count=1 ./internal/b… |
| TASK-005.functional[0] | R8,R6,R16 | 段落与行：标题日期、覆盖行、预警段、同期统计段（n=当期主体数）、排名段（首行含环比/同比）、别名行「（A银行H 同 6… |
| TASK-005.functional[1] | R8,R12,R15,R17 | **golden 比对（spec §7）**：`renderSample()` 完整输出与 golden 逐字节相等。样… |
| TASK-005.functional[2] | R8,R5,R17 | 预警行：阈值型按方向用 `>`（NPL）或 `<`，恶化型 `· X银行 CET1 环比 -0.62pp（超 0.50p… |
| TASK-005.boundary[0] | R8 | 无预警 ⇒ `⚠️ 预警 (0)\n无`；无当期主体 ⇒ `统计期 —` 且无 📊 与排名段；NaN ⇒ 取值 `N/A… |
| TASK-005.boundary[1] | R9 | Split：每段 rune 数 ≤ limit；**恰好等于 limit 的段不被切、limit+1 必被切**（两个精… |
| TASK-005.non_functional[0] | R12 | `go vet ./internal/bank/` 无输出；`go test -count=1 ./internal/b… |
| TASK-006.functional[0] | R10,R11 | executeBankReport 全成功：退出码 0；600036.SH 只 Fetch 1 次；推送 1 条，含「🏦… |
| TASK-006.functional[1] | R11 | 全部失败：返回 error（⇒ 退出码 1），仍推送 1 条错误摘要，含「全部 2 家拉取失败」且**每家**的名称与错… |
| TASK-006.functional[2] | R10,R12 | **--dry-run（spec §7）**：经 `runBankReport` 以 `--dry-run` 运行 ⇒ … |
| TASK-006.boundary[0] | R9,R10 | 超长报告（如 150 家当期银行）⇒ Sender 按序收到多段，每段 rune 数 ≤ 4000，段数与 `bank.… |
| TASK-006.error_handling[0] | R14 | **D1**：非 dry-run 且 sender 构造返回 nil ⇒ stdout 有完整报告、stderr 含原因… |
| TASK-006.error_handling[1] | R11 | 推送失败 ⇒ error 含「telegram 推送第 1/1 段失败」；`runBankReport` 遇非法 ban… |
| TASK-006.non_functional[0] | R10,R12 | 命令注册：`rootCmd.Find(bank report)` 成功，含 `--dry-run`，继承 `--bank… |
| TASK-007.functional[0] | R12,R2 | `go test -tags integration -count=1 ./internal/bank/ -run In… |
| TASK-007.functional[1] | R13,R15 | 真实数据 dry-run：`go run ./cmd/atlas bank report --dry-run --ban… |
| TASK-007.boundary[0] | R12 | 不带 tag 时集成测试不参与编译：`go list -f '{{.TestGoFiles}}' ./internal/… |
| TASK-007.error_handling[0] | R12 | `ATLAS_AKTOOLS_URL=http://127.0.0.1:1` 时集成测试**失败**（非 skip）且错… |
| TASK-007.non_functional[0] | R13 | plist：`plutil -lint` OK；Label `com.newthinker.atlas.bank-mon… |
| TASK-007.non_functional[1] | R13 | `go test -count=1 ./internal/bank/` 全绿；包覆盖率 ≥ 80%；验证过程中未向 Te… |

## 机器检查

- 孤儿需求：无
- 凭空 DoD：无
- 映射表中不存在的条目：无
- DoD 总条数：48
