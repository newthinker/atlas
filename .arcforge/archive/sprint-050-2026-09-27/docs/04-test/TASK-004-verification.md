# TASK-004 验证报告（verifier: test-bk-a）

- 被验对象：`d4d1d051a376b0e8c36a834461c62949ea1b18c9`（= verify_baseline.head = 判定时的主仓库 HEAD）
- discovery sha256：`58e45170378fe967f13f85caabe9bc8d1ff462717f7c40c18b8fb17b498e32b0`（= baseline）
- 验证环境：隔离 worktree `../wt-verify-TASK-004`（detached，钉在上述全 sha），go1.24.4 darwin/arm64
- 结论：**VERIFIED**。有一个观察项需要 Leader 裁决，见文末：「Ahead 不进同期统计」没有测试守卫，而这条义务只写在 description 里，不在 DoD 中。

## 范围核对，以及 code-simplifier 改动的自证锚点（重点核查 ④）
- `git diff --stat 6d792474b9f78b3d76ec8807d2615947dff42e51 d4d1d051a376b0e8c36a834461c62949ea1b18c9` 只有 4 个文件（collect.go +27、collect_test.go +105、summary.go +123、summary_test.go +238），与 `writes` 一致。
- discovery 声称自证数字采自 `fb88b3e5e90e603738119320bb576321619d4cf8`。我核对了 4 个文件在 fb88b3e 与 d4d1d05 的 blob，逐一相同（collect.go b547ed41、collect_test.go b54bf885、summary.go a07483bf、summary_test.go 38875480）。fb88b3e 版本的 sha256 前缀分别为 a2540fab、2e14b31a、a6dc3830、087401a1，与 discovery 记录一致。也就是说，discovery 的数字测的正是被合入的这版，其中包含 simplifier 做的两处改写。
- 这两处改写我逐段读过：summary_test.go:82 的 `slices.Concat` 只用于拼接四组名单，summary.go:116-121 的中位数分支拆成了两行，都与语义等价。中位数的正确性另外由 Q20 和 Q21 两个变异证明。
- 下面的数字全部是我在 d4d1d05 上重采的，没有采用 discovery 的数字。

## 非功能重采
| 检查 | 结果 |
|---|---|
| `go vet ./internal/bank/` | 无输出，rc=0 |
| `go test -count=1 -v -coverprofile ./internal/bank/` | rc=0，`--- PASS` 102 行 / `--- FAIL` 0 行 |
| 计数互验 | 尺 1（按输出缩进计）：顶层 43 + 子测试 59 = 102 = `=== RUN` 102。尺 2（源码）：`^func Test` 共 43 个，其中本任务 collect 4 个、summary 7 个；子测试 = 上一版的 54 + TestSummarizeModePeriod 的 5 = 59。两尺一致。注意子测试名「并列取较晚/交错」自带斜杠，所以没有用斜杠数来判层级 |
| 覆盖率 | 包 99.5%，collect.go 与 summary.go 所有函数 100% |
| 执行顺序独立性 | `-shuffle=1` 到 `-shuffle=10` 共 10 个独立 seed 全部 rc=0 |
| 共享辅助 | `day`、`obs`、`nan` 各只定义一处（helpers_test.go:6、analyze_test.go:23/25），本任务没有重复定义 |

## 重点核查
- **① D15 众数统计期**：TestSummarizeModePeriod 覆盖了 10:1、2:2 并列的三种输入顺序，以及「3 家失败 + 3 家无数据 + 1 家当期」。失败和无数据的主体如果参与计数，统计期会变成 03-31 或零时间，所以这个用例有判别力。变异 Q1 到 Q6 全部 KILLED：取最晚、并列取早、并列保留先到、并列取后到、失败参与计数、无数据参与计数。「四组互斥且并集为全体」这一点，由 TestSummarizeGroupsAndRanks 把四组名单拼起来排序后与输入比较；Q11（失败主体同时进 Stale）因此 KILLED。
- **② D2 联动**：除了 TestSummarizeExcludesNaNAndFallback（回退值分别设成「会成为最优」和「会成为最差」），我还写了一个独立的端到端探针，只在隔离 worktree 里运行，用完即删。它通过 `Collect(fakeSource) → Summarize(IndCET1)` 跑一遍，用 600919 的实测序列作为 CET1 回退的当期主体，再加三家 2025-09-30 有完整数据的主体。结果：Period=2025-09-30；按 CET1 排名是「平安 9、兴业 8、工行 7、江苏」，江苏的回退值 8.49 排在最后，没有插到 8.0 与 7.0 之间；CET1 统计 N=3，最优平安、最差工行；NPL 统计 N=4；按 NPL 排名江苏第一（0.84 是当期值，照常参与）。变异 Q12、Q13、Q14 KILLED，分别是回退值同时在统计和排名、仅在统计、仅在排名中被当作当期值。
- **③ 失败主体 Ind 为 NaN 而不是 0**：TestCollectKeepsErrors 对三项都调用 assertNaNChange。fakeSource 出错时仍然会返回数据，所以 Q28（不丢弃出错时的部分数据）和 Q34（失败时 Ind 置 0）都能被杀掉。端到端探针里失败的「浦发」三项 Value 也是 NaN。
- **④** 见上一节。

## done_criteria 覆盖矩阵
| # | 条目 | 对应测试 | 断言是否测到 | 变异证据 | 判定 |
|---|---|---|---|---|---|
| functional[0] | A+H 去重、只 Fetch 一次、主名与 Aliases、首次出现顺序、MissingFields 透传、只配 H 股 | TestCollectDedupsAH、TestCollectOrderFirstAppearance、TestCollectHOnly | 用 calls 的 map 做等值断言，Name、Aliases、Symbol、MissingFields 也都是等值断言 | Q29 Q30 Q31 Q32 Q35 KILLED | PASS |
| functional[1] | 四组划分、排名方向、互斥且并集为全体 | TestSummarizeGroupsAndRanks、TestSummarizeRankDirection | 四组名单做等值断言，并集排序后比较，三个指标的排名方向都有断言 | Q7 Q8 Q9 Q10 Q11 Q17 Q18 KILLED | PASS |
| functional[2] | 众数统计期（10:1、2:2 取晚、失败不计） | TestSummarizeModePeriod（5 个子测试） | 见重点核查 ① | Q1 到 Q6 KILLED | PASS |
| functional[3] | Stat 的 N、均值、中位数（奇偶）、最优最差及方向 | TestSummarizeGroupsAndRanks、TestSummarizeMedianEven | 整个 Stat 做等值断言；偶数中位数的输入按 NPL 排名后是乱序 | Q20 Q21 Q22 Q24 KILLED | PASS |
| boundary[0] | NaN 与回退值不进统计、排名排最后；没有当期主体时各项为零值或 NaN | TestSummarizeExcludesNaNAndFallback、TestSummarizeNoCurrent，外加我的端到端探针 | N 只计合格值，均值、中位数、最优最差都有断言；排名断言为 C、B、A、N | Q12 Q13 Q14 Q16 Q23 Q25 Q26 KILLED | PASS |
| boundary[1] | 并列时排名稳定 | TestSummarizeTiesKeepConfigOrder（4 家 + 40 家） | 名单做等值断言；40 家的用例能避开小切片走插入排序的巧合 | Q15（SliceStable 改成 Slice）、Q19（并列视为更优）KILLED | PASS |
| error_handling[0] | 失败主体 Err 保留原错误、Latest 为零、Ind 为 NaN | TestCollectKeepsErrors | `assert.Same` 原错误，三项都做 NaN 断言 | Q28 Q34 KILLED | PASS |
| non_functional[0] | vet、测试全绿、覆盖率 ≥80% | 命令行重采 | 见上 | — | PASS |

## 变异表
脚本：scratchpad/tbka/mut_tbka_task004.py，变异集是我独立设计的，没有沿用 dev 的 harness。每个变异的锚点唯一，先 vet 通过再跑测试；每轮 `git checkout` 还原并比对主仓库指纹。收尾时 worktree 干净，主仓库指纹一致。

| ID | 变异 | 结果 |
|---|---|---|
| Q1 | 统计期取最晚 | KILLED |
| Q2 | 并列取较早 | KILLED |
| Q3 | 并列保留先到 | KILLED |
| Q4 | 并列取后到 | KILLED |
| Q5 | 失败主体参与计数 | KILLED |
| Q6 | 无数据主体参与计数 | KILLED |
| Q7 | 失败主体不进 Failed | KILLED |
| Q8 | Ahead 并入 Current | KILLED |
| Q9 | Ahead 并入 Stale | KILLED |
| Q10 | 无数据主体进 Current | KILLED |
| Q11 | 失败主体同时进 Stale | KILLED |
| Q12 | value 不看 Period | KILLED |
| Q13 | 仅统计中用原始 Value | KILLED |
| Q14 | 仅排名中用原始 Value | KILLED |
| Q15 | SliceStable 改成 Slice | KILLED |
| Q16 | NaN 视为最优 | KILLED |
| Q17 | 「越高越好」方向取反 | KILLED |
| Q18 | NPL 方向取反 | KILLED |
| Q19 | 并列视为更优 | KILLED |
| Q20 | 偶数个取上中位 | KILLED |
| Q21 | 中位数前不排序 | KILLED |
| Q22 | 均值分母用 N+1 | KILLED |
| Q23 | N 计入全部 Current | KILLED |
| Q24 | Worst 按 Best 的方向比较 | KILLED |
| Q25 | N==0 时 Mean 为 0 | KILLED |
| Q26 | N==0 时 BestVal 为 0 | KILLED |
| **Q27** | **统计把 Ahead 也算进去** | **SURVIVED**（见观察项） |
| Q28 | Collect 不丢弃出错时的部分数据 | KILLED |
| Q29 | Collect 不记 Aliases | KILLED |
| Q30 | Collect 主名取最后一个条目 | KILLED |
| Q31 | Collect 不去重 | KILLED |
| Q32 | Collect 不透传 MissingFields | KILLED |
| Q34 | Collect 失败时 Ind 置 0 | KILLED |
| Q35 | Collect 的 Symbol 取条目自身的 Symbol | KILLED |

合计 34 个，33 个 KILLED。Q33（包装 Err）需要在 collect.go 里新增 import，我的单锚点 harness 做不了，所以没跑；dev 的 C6 用 `assert.Same` 覆盖了这一点，我也读过 `assert.Same` 的断言。

## 观察项（需 Leader 裁决，不构成本任务的拒绝）
**Q27 存活，而且不是等价变异。** 本任务 description 里写着人类裁决 D15：「`Ahead` … **不进同期统计与排名**」。实现是满足的，stat 只遍历 Current。但测试套件守不住这一点。

- 为什么现有用例测不到：现有用例里 Ahead 主体的三项 Period 都等于它自己的 Latest，也就是说都晚于统计期，D2 的 Period 过滤（`value()`）已经把它们挡掉了，「stat 只遍历 Current」这道闸有没有都一样。
- 这道闸在什么情况下会独立起作用：Ahead 主体的某个指标恰好回退到了统计期。这正是 600919 的真实形态：季报不披露 CET1，Q3 已出而多数银行还停在 H1。
- 实证（只在隔离 worktree 中跑的探针，跑完即删）：甲、乙两家在 2025-06-30，江苏（600919 实测序列，Latest 2025-09-30，CET1 回退到 2025-06-30 的 8.49）进 Ahead。
  - 原实现：CET1 统计为 N=2，最差是甲（10）。
  - Q27 变异：N=3，最差是**江苏（8.49）**，均值从 10.5 被拉低到 9.83。
- 按协议，验收依据只有 done_criteria。DoD 的 functional[1] 与 [2] 只定义了 Ahead 的分组；boundary[0] 按字面只要求「Period ≠ 统计期的值不进统计」，而江苏这个回退值的 Period 恰好等于统计期。所以这是一条「义务写在 description 而不在 DoD」的缺口，我没有据此拒绝。
- 建议：由 Leader 决定是否补一个用例，即上面的场景，断言 CET1 的 N 与 Worst 不含 Ahead 主体。可以落在 QA 的 fix_items 里，或者作为 TASK-005 或后续任务的 done_criteria。补测时的自检方法：Q27 应当转红。
