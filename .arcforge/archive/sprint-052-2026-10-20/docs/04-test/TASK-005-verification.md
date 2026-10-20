# TASK-005 验证报告（第 3 轮返工复验，test-pw-c）

## 前几轮摘要（原文已被本报告覆盖）

| 轮次 | 验证者 / 被测树 | 结论 | 要点 |
|---|---|---|---|
| 首轮 | test-pw-b @ `49f4b71` | VERIFIED | 非 DoD 发现：Term 无上限导致 5250 码元、恰 4096 边界无测试、价格跨档外观 |
| QA 第 1 轮返工 | test-pw-b @ `ebc366e43a6deda8dc28ae8974b2e3fa77bb3c1b` | VERIFIED | AD-31 三步兜底；`TestFormatAlert_LimitBoundary` |
| QA iter2 返工（N1：FormatHealth 不限长） | test-pw-c @ `78b15ec1c69a2fed95a5a96aa93a044f06d26d97` | **REJECTED（task_defect）** | 实现正确，但 `capUTF16` 阈值边界无守卫：变异 N4（`<= max` → `<= max+1`）全包 + cmd/atlas 全绿，恰 4097 码元的健康消息会被原样发出。顺带评估：FormatHeartbeat 约 30 个资产即超 4096 |

## 本轮被测对象

- 被测树：`bc33f71827179242e06d2e74129b0f603bd9f699`（feature/pricewatch，等于 `verify_baseline.head`）；隔离 worktree `../wt-verify-TASK-005-c3`（detached）。
- 返工提交 `0cac1719a141d5d1a9c71235f59898c615005d5e`：message.go +2/−2（FormatHeartbeat 末尾 `return capUTF16(b.String())`），message_test.go +61/−5。在被测树上 `git diff 0cac171 HEAD` 对两个声明文件输出为空，改动在 `writes` 内。
- discovery sha256 `8ae0cceb8a7115e0c9ad9257f3880e50dff763b78b4176f10748b499c50ddd86` 与 baseline 相同。
- 工具链 `GOTOOLCHAIN=local`。

## 结论：VERIFIED

上一轮的退回点已经补上，并由变异证实（N4 现在被 `TestFormatHealth_Boundary` 杀死）。Leader 追加的心跳截断已落实，且有守卫。附 2 条非阻断说明（§6）。

## 1. 门禁对照

| 项 | 上一轮（78b15ec） | 本轮（bc33f71） |
|---|---|---|
| `go test -race -count=1 -cover ./internal/pricewatch/` | ok，94.3% | ok，94.3% |
| `go vet` / `gofmt -l internal/pricewatch/` | 无输出 | 无输出 |
| FormatHealth / FormatHeartbeat / capUTF16 覆盖 | 100% | 100% |
| `TestFormat*` | 19 PASS | **21 PASS**（新增 TestFormatHealth_Boundary、TestFormatHeartbeat_Limit），0 FAIL |
| `go test -count=1 ./cmd/atlas/ -run Pricewatch` | ok | ok |
| golden | plan 第 331–345 行代码块与 want 逐字相等（268 字符） | **重新抽取比对**，仍逐字相等（268 字符） |

## 2. done_criteria 覆盖矩阵（8 条，整份重验）

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | golden 逐字相等（Term "3 天"） | `TestFormatAlert_Golden` | PASS（与 plan 原文独立比对） |
| functional[1] | Advice nil：「AI 分析不可用」且无【AI 分析】 | `TestFormatAlert_NoAI` | PASS |
| functional[2] | 图标 🔺/🔀、rise 行文案；两条 Advice 分行 | `TestFormatAlert_Icons`、`TestFormatAlert_TwoSides` | PASS |
| functional[3] | ShortDays=100 追加、=0 不追加 | `TestFormatAlert_ShortWindow` | PASS |
| functional[4] | 心跳关键字；健康 down/up 文案 | `TestFormatHeartbeat`、`TestFormatHealth` | PASS（心跳加 cap 后正常长度用例不受影响） |
| boundary[0] | 价格/百分比格式、−0.0、档位写法 | `TestFormatPrice`、`TestFormatPct`、`TestFormatTier` | PASS |
| error_handling[0] | FormatAlert 1 万字 ≤4096、触发规则完整、截断标记、正常不截 | `TestFormatAlert_Truncate`、`NoTruncateNormal`、`Limit*` | PASS（AD-31 A1–A4 仍 KILLED） |
| non_functional[0] | race/vet/gofmt/覆盖率 ≥80% | §1 | PASS |

## 3. fix_items 核对矩阵

| fix_item | 落实 | 守卫（变异证据） |
|---|---|---|
| ① capUTF16 阈值边界：FormatHealth 恰 4096 不截 / 恰 4097 截到 ≤4096 | `TestFormatHealth_Boundary`（与我上一轮 §6 的探针同形） | **N4 `<= max+1` → KILLED**（只有 Boundary 变红）；N4b `< max` → KILLED（Boundary + LimitLongAdviceFields）；N3 常量 4097 → KILLED |
| ② Leader 追加：FormatHeartbeat 过 capUTF16、截尾保留首行；超长用例；overlay 去 cap 变红 | `message.go:212` `return capUTF16(b.String())`；`TestFormatHeartbeat_Limit`（60 资产 × 4 规则） | HB1 去 cap → KILLED；HB2 截头保尾丢首行 → KILLED；HB3 心跳自写阈值 4097 → KILLED；HB5 过度截断到 4000 → KILLED |
| ③ 失败信息按 rune 截（runeHead/runeTail） | 6 处 `got[:n]`/`got[len-n:]` 改为 runeHead/runeTail | 只影响失败信息，不影响判定；本轮 harness 解码未再出错 |

### 心跳用例需不需要单独的阈值 ±1 守卫（Leader 要求判断）

**判断：不需要，fix_item ② 没有因此失去实质守卫。**

- 心跳与健康、提醒共用同一个 `capUTF16`。它的阈值比较由 `TestFormatHealth_Boundary` 守住（N4、N4b 都会变红）。心跳只要调用它，就继承了这条守卫。HB1 证明「不调用」会被抓到。
- 若有人在心跳里另写阈值：危险方向（>4096，HB3 = 4097）会被 `TestFormatHeartbeat_Limit` 的上界抓到。
- **唯一存活的是 HB4**：心跳自写阈值 4095，结果 4095 码元，落在用例的 `[4094, 4096]` 区间内。这是**保守方向**的偏移：消息仍能送达，只是少用了 1 个码元，不会触发 400。记为非 DoD 观察，不构成退回理由。

### 断言是否被夹具顺带满足

- `TestFormatHealth_Boundary`：夹具 detail 是纯「x」，标题与截断标记都由被测代码生成；「恰 4096 不截」用的是**逐字相等**，不是包含判断。
- `TestFormatHeartbeat_Limit`：前缀 `💓 监控仍在运行（2026-10-20 09:00）\n· 测试资产 SYM00 ` 由被测代码按夹具拼出；夹具里不含截断标记。HB1/HB2 都让它变红。

没有发现被夹具顺带满足的断言。

## 4. 独立变异汇总（隔离：worktree + `go test -overlay`，「全包」= `./internal/pricewatch/` + `./cmd/atlas/`）

harness：`/private/tmp/claude-501/-Users-zuowei-workspace-go-src-github-com-newthinker-atlas/b3c3a6ba-7de9-492d-9e24-953680e5a079/scratchpad/t005c3-mut/run.py`。每个变异体都断言锚点恰好出现 1 次、打印 diff，并先过 `go vet -overlay`。主仓库 message.go/message_test.go 的 sha256 与 `git status --porcelain` 指纹前后一致。

| 变异 | TestFormat* 命中 | 全包 |
|---|---|---|
| N4 capUTF16 `<= max` → `<= max+1`（上一轮存活） | Health_Boundary | KILLED |
| N4b capUTF16 `<= max` → `< max` | Health_Boundary、LimitLongAdviceFields | KILLED |
| N3 常量 4096→4097 | LimitBoundary、Truncate、Health_Boundary | KILLED |
| N8 capUTF16 预算不扣标记 | LimitWholeMessage、Health_Boundary、Health_Limit、Heartbeat_Limit | KILLED |
| H1 健康去 cap | Health_Boundary、Health_Limit | KILLED |
| H2 健康超长只留 detail（丢标题） | Health_Boundary、Health_Limit | KILLED |
| HB1 心跳去 cap | Heartbeat_Limit | KILLED |
| HB2 心跳截头保尾（丢首行） | Heartbeat_Limit | KILLED |
| HB3 心跳自写阈值 4097 | Heartbeat_Limit | KILLED |
| **HB4 心跳自写阈值 4095** | — | **SURVIVED**（保守方向，无害，见 §3） |
| HB5 心跳过度截断到 4000 | Heartbeat_Limit | KILLED |
| A1 FormatAlert 去末尾整条兜底 | LimitWholeMessage | KILLED |
| A2 只截 AI 分析 | LimitLongAdviceFields | KILLED |
| A3 无 AI 时不兜底 | LimitWholeMessage | KILLED |
| A4 `over <= 0` → `< 0` | LimitBoundary、Truncate | KILLED |

15 个变异：14 KILLED，1 个保守方向存活。

## 5. rework_count 上限说明

本任务 `rework_count=3`（等于 `max_rework`）。本轮判 **VERIFIED**，不涉及转 `blocked_human`。

## 6. 非阻断说明

1. **HB4（心跳阈值 4095）存活**：理由见 §3。如果想钉死，可以给心跳也加一条「恰 4096 不截」的用例，但收益只是防一个无害的偏移。
2. **心跳截尾会丢掉末行「过去 24 小时发出提醒 N 条」**：截尾保留首行是 Leader 指定的修法，资产行在前、计数行在最后，超长时计数行一定被截掉。只在约 30 个资产以上才会发生，影响的是信息完整而不是送达。如果在意，可以改为截资产行、保留末行；本轮不要求。

## 7. 复现命令（锚为全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-005-c3 bc33f71827179242e06d2e74129b0f603bd9f699
cd ../wt-verify-TASK-005-c3
GOTOOLCHAIN=local go test -race -count=1 -cover ./internal/pricewatch/
python3 <scratchpad>/t005c3-mut/run.py "$PWD" <scratchpad>/t005c3-mut <主仓库路径>
```
