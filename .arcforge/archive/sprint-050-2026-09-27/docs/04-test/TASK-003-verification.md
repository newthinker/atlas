# TASK-003 验证报告（verifier: test-bk-a）

**最终结论：VERIFIED（第 2 轮，返工 r1 @ `6d792474b9f78b3d76ec8807d2615947dff42e51`）**

## 第 2 轮复验（返工 r1）

- 被验对象：`6d792474b9f78b3d76ec8807d2615947dff42e51`（= verify_baseline.head = 判定时的主仓库 HEAD）；discovery sha256 `6e6c173fee5308127ea4e385234fe26147bb699c74e4874af4ddf5bedbf97e9d`（= baseline）
- 隔离 worktree `../wt-verify-TASK-003`（detached，钉在上述全 sha）

### analyze.go 一字节未变（上轮结论可复用的依据）
- `git diff ea8e406789fac8c90cfc24a324bfff3967089748 6d792474b9f78b3d76ec8807d2615947dff42e51 -- internal/bank/analyze.go` 输出为 0 行。
- 两个版本的 blob 相同：`git rev-parse <sha>:internal/bank/analyze.go` 都是 `9702f46f33c3a374471f878d489b516012c3d580`。
- `git diff --stat ea8e406…6d79247` 只有 analyze_test.go 一个文件（+19 −7），在 `writes` 范围内。
- analyze_test.go 的改动：三个真实数据用例改为传 `slices.Clone(fixture)`（fixture 数值一个未改）；TestAnalyzeDoesNotMutateInput 改为用 `fresh()` 现场构造输入和期望，数据含 NaN 空期（5 期，其中 4 期带 NaN，末期全 NaN）。

### 重采
- `go vet` 无输出。`go test -count=1 -v` rc=0，PASS 86 / FAIL 0（顶层 32 与源码 `^func Test` 32 一致，子测试 54），覆盖率 99.3%。
- 未变异的对照组用 10 个独立 seed（`-shuffle=1` 到 `-shuffle=10`，每个 seed 单独运行一次）全部 rc=0。另外核对了不同 seed 的执行顺序确实不同：seed 1 先跑 LatestQoQYoY/ExplicitAktoolsURL/DayHelper，seed 2 先跑 EMSourceFetchErrors/AlertsLevel/RankBy。

### 上轮缺陷复验：P15 / P15b（另加 P15c 对照）
按 dev 的提醒，没有用 `-shuffle=on -count=N`（那样只有一个 seed），而是 10 个 seed 各跑一次。每格记录两项：TestAnalyzeDoesNotMutateInput 是否失败（1 = 失败），以及括号里全套件 `--- FAIL` 顶层行数。

| 变异 | 默认顺序 | 单跑 | seed 1–10 |
|---|---|---|---|
| P15：写 NaN（幂等） | rc=1，只有 DoesNotMutateInput 红 | rc=1 | 10/10 转红，每次都只红这一个测试 |
| P15b：原地前向填充（幂等） | rc=1，只有 DoesNotMutateInput 红 | rc=1 | 10/10 |
| P15c：`+= 1`（非幂等） | rc=1，只有 DoesNotMutateInput 红 | rc=1 | 10/10 |

上轮「默认顺序下存活、shuffle 下 1/3 转红」的顺序依赖已经消除。

### 全量变异回归
用同一脚本（scratchpad/tbka/mut_tbka_task003.py）在 6d79247 上重跑。27 个变异体全部 KILLED，其中 P15 从上轮的 SURVIVED 变为 KILLED；其余 26 个转红的测试条数与上轮逐条相同。P15b/P15c 见上表。合计 29 个变异体，29 个 KILLED。收尾时 worktree 干净，主仓库指纹一致。

### 第 2 轮覆盖矩阵
| # | 判定 | 依据 |
|---|---|---|
| functional[0]–[3] | PASS | analyze.go 未变、fixture 数值未变（diff 只加了 Clone），第 1 轮证据仍然有效；变异回归转红条数不变 |
| boundary[0] | PASS | 同上；P1、P2a/b/c、P3a/b/c、P4、P5b/c 重跑后仍 KILLED |
| boundary[1] | PASS | 同上；P8、P9 重跑后仍 KILLED |
| non_functional[0] | **PASS** | vet 与测试全绿，覆盖率 99.3%；入参修改守卫在默认顺序、单跑和 10 个 seed 下对三类写入都稳定转红 |

---

## 第 1 轮（ea8e406，REJECTED）——原文保留

- 被验对象：`ea8e406789fac8c90cfc24a324bfff3967089748`（= verify_baseline.head = 判定时的主仓库 HEAD）
- discovery sha256：`8ddc3a3ae53b19291be415b87fa19dd7e963b4376073b3a8fc178635ddc2d9a3`（= baseline）
- 验证环境：隔离 worktree `../wt-verify-TASK-003`（detached，钉在上述全 sha），go1.24.4 darwin/arm64
- 结论：**REJECTED（task_defect）**。只有 non_functional[0]「Analyze 不修改入参切片」这一条的测试守卫有缺口，实现本身和其余 6 条 done_criteria 都通过。

## 范围核对
`git diff --stat 7f51cd4c4e91c80021a958d6e11f9516181b2625 ea8e406789fac8c90cfc24a324bfff3967089748` 只有 analyze.go（+123）和 analyze_test.go（+247），与 `writes` 一致。`func day(` 只定义在 helpers_test.go:6。

## 非功能重采
| 检查 | 结果 |
|---|---|
| `go vet ./internal/bank/` | 无输出，rc=0 |
| `go test -count=1 -v -coverprofile ./internal/bank/` | rc=0，`--- PASS` 86 行 / `--- FAIL` 0 行 |
| 计数互验 | 尺 1（测试输出）：顶层 32 + 子测试 54 = 86 = `=== RUN` 86。尺 2（源码）：`^func Test` 全包 32 个（其中 analyze_test.go 12 个）；子测试 = TASK-001/002 的 44 + TestAlertsLevel 7 + BoundaryIsStrict 3 = 54。两尺一致 |
| 覆盖率 | 包 99.3%，analyze.go 各函数都是 100%。discovery 写的 98.9% 是在 00596c0 上采的，那棵树还没有 TASK-002 的 source.go，包的构成不同，所以两个数字不矛盾 |

## 重点核查 ①：真实数据与 aktools 直查对照
2026-09-27 本机直接 curl `http://127.0.0.1:8180/api/public/stock_financial_analysis_indicator_em?symbol=<代码>&indicator=按报告期`，三只都是 HTTP 200，接口分别返回 600036 102 期、600919 57 期、002142 83 期，没有重复期。
- 尺 1（Python 逐值比对）：cmb、jsbk、nbbk 三个 fixture 共 21 期 × 3 项 = 63 个值（null 与 NaN 视为相等），**不一致 0 个**。
- 尺 2（jq 单独取 DoD 点名的值）：600919 在 2025-09-30 的 CET1 为 null，2025-06-30 为 8.49，2025-03-31 为 8.36，2024-06-30 为 8.99；002142 在 2026-03-31 的 CET1 为 null，2025-12-31 为 9.34，2025-06-30 为 9.65，2026-06-30 为 9.53。都与测试一致。
- 另外看到的：600919 在 2025-12-31 到 2026-06-30 这三期的 CET1 都已披露（8.93 / 8.5 / 8.67）。DoD 截取的是到 2025-09-30 为止的序列，不受影响，与 discovery key_findings[0] 的说法一致。

## 浮点边界（两把尺）
| 差值 | Python repr | Go FormatFloat 'g' 17 |
|---|---|---|
| 0.67−0.57 | 0.10000000000000009 | 0.10000000000000009 |
| 236.04−256.04 | -20.00000000000003 | -20.000000000000028 |
| 15.51−16.01 | -0.5000000000000018 | -0.50000000000000178 |
| 1.02−0.92（计划原例） | 0.09999999999999998 | 0.099999999999999978 |

前三对的原始差都越过了阈值，第四对确实低于阈值，与 DoD 和测试注释一致。

## done_criteria 覆盖矩阵
| # | 条目 | 对应测试 | 断言是否测到 | 变异证据 | 判定 |
|---|---|---|---|---|---|
| functional[0] | cmb 最新期与三项 {Value, QoQ, YoY, Period} | TestAnalyzeLatestQoQYoY | 整个 Change 结构体做 assert.Equal 精确比较 | P1、P2b、P2c KILLED | PASS |
| functional[1] | 600919 按指标回退、CET1 Period=2025-06-30、QoQ +0.13、预警带回退期 | TestAnalyzeFallbackPerIndicator600919 | Latest、三项 Change 与 Alerts 整个切片做等值断言 | P6、P10、P11、P20 KILLED；数据已经 aktools 核对 | PASS |
| functional[2] | 002142 跨空期 QoQ +0.19、YoY −0.12 | TestAnalyzeAcrossGap002142 | CET1 Change 做等值断言 | P7（环比不跨空期）KILLED | PASS |
| functional[3] | 阈值与恶化预警的 Kind/Value/Limit/Period；改善方向不预警 | TestAlertsLevel、TestAlertsDeterioration | 三项指标用不同的 Period，Alert 切片做等值断言 | P11、P12、P18、P19 KILLED | PASS |
| boundary[0] | 严格不等号；三组浮点边界 QoQ 精确等于阈值且不预警；删 round4 或任一指标 `>` 改 `>=` 都须转红 | TestAlertsLevel（三项等于阈值）、TestAlertsDeteriorationBoundaryIsStrict/{NPL,Coverage,CET1} | QoQ 精确等值，Alerts 为空 | **P1 删 round4、P2a/b/c 逐指标去 round4、P3 以及 P3a/b/c 逐指标恶化 `>=` 全部 KILLED，而且逐指标变异只让对应指标的子测试转红**；P4、P5、P5b、P5c 阈值等号变异 KILLED | PASS |
| boundary[1] | 全 NaN 尾期、指标全 NaN、单值、空序列、上年同日期为 NaN ⇒ YoY NaN | SkipsAllNaNTail、IndicatorAllNaN、SingleValidValue、Empty、YoYPeriodNaN | 逐字段检查 IsNaN 与 IsZero；YoYPeriodNaN 在目标期前后都放了非 NaN 的诱饵 | **P8（目标期为 NaN 时回退到更早的有效期）与 P9（改取其后首个有效期）都 KILLED**；P13、P14、P16、P17 KILLED | PASS |
| non_functional[0] | Analyze 不修改入参；vet；全绿；覆盖率 ≥80% | TestAnalyzeDoesNotMutateInput + 命令行 | **在全套件默认顺序下，这个守卫看不到幂等写入（见缺陷）** | **P15、P15b SURVIVED**；P15c KILLED | **FAIL** |

## 缺陷：TestAnalyzeDoesNotMutateInput 的判定依赖测试执行顺序
- 位置：analyze_test.go:172-177（`in := append([]Observation(nil), nbbk...)`），以及 :101 `Analyze(nbbk)` 把全局 fixture 直接交给了被测函数。
- 机制：TestAnalyzeAcrossGap002142 按源码顺序先运行，它把**全局 `nbbk` 本体**传给 Analyze。如果 Analyze 会写入参，全局 fixture 在守卫运行之前就已经被改写。守卫随后复制这份**已经被污染的** nbbk 作为「修改前」快照，只要写入是幂等的（重复写结果不变），「修改前」和「修改后」就逐字相同，守卫保持沉默。也就是说，守卫恰好在它该起作用的时候失效。
- 证据（每个变异都作用在隔离 worktree 上，vet 通过，用完即 `git checkout` 还原）：

| 变异（都加在 Analyze 的 `return` 之前，不影响返回值） | 全套件 `go test -count=1` | 单跑 `-run '^TestAnalyzeDoesNotMutateInput$'` | `-shuffle=1/2/3` |
|---|---|---|---|
| P15：`obs[0].Values[IndNPL] = NaN` | rc=0（存活） | rc=1 | 1 / 0 / 0 |
| P15b：对入参原地前向填充 NaN（「按指标回退」功能很自然会写出的越界实现） | rc=0（存活） | rc=1 | 1 / 0 / 0 |
| P15c：`obs[0].Values[IndNPL] += 1`（非幂等） | rc=1（KILLED） | rc=1 | 1 / 1 / 1 |

  同一份代码的判定随执行顺序翻转，而 dev_done 门禁跑的正是存活的那种默认顺序。
- 为什么按 task_defect 处理：DoD 要求这条性质 `verify_by: test`。守卫的断言本身没问题，但它的输入被其他测试污染了，因此在全套件里不能可靠地测到这条性质。Leader 派验时把 ④ 列为重点核查项。
- 建议修复方向（改动很小）：
  1. TestAnalyzeDoesNotMutateInput 的输入改为在测试内用字面量构造，且要含 NaN 空期（空期正是回退逻辑最可能写回的地方）。「修改前」的期望值也在测试内独立构造一份，不要从可能被污染的全局变量复制。
  2. 或者（最好同时做）让其他测试传给 Analyze 的都是副本，比如 `Analyze(slices.Clone(nbbk))`，避免被测函数的缺陷跨测试传播。
  3. 修好后自检：P15、P15b 在全套件默认顺序下应该转红，`-shuffle` 下也应该稳定转红。

## 变异总表
脚本：scratchpad/tbka/mut_tbka_task003.py。每个变异的锚点只命中一处，先 vet 通过再跑测试；每轮 `git checkout` 还原并比对主仓库指纹。收尾时 worktree 干净，主仓库指纹一致。P15b、P15c 以及 P15 的单跑与 shuffle 是用单独的命令补跑的，同样作用在隔离 worktree 上，跑完已还原。

| ID | 变异 | 结果 |
|---|---|---|
| P1 | round4 改为恒等 | KILLED（9 条红，含 BoundaryIsStrict 三个子测试） |
| P2a/b/c | 仅 NPL / Coverage / CET1 的环比不做 round4 | 各自 KILLED，并且都点名对应的 BoundaryIsStrict 子测试 |
| P3 | 恶化比较 `>` 改 `>=`（全部指标） | KILLED |
| P3a/b/c | 仅 NPL / Coverage / CET1 的恶化比较改 `>=` | 各自 KILLED，只红对应子测试 |
| P4 | 阈值 NPL `>` 改 `>=` | KILLED（不良率等于阈值） |
| P5 / P5b / P5c | 阈值 `<` 改 `<=`（两项一起 / 仅 Coverage / 仅 CET1） | KILLED，只红对应子测试 |
| P6 | 取消按指标回退 | KILLED |
| P7 | 环比不跨空期 | KILLED |
| P8 | 同比目标期为 NaN 时回退到更早的有效期 | KILLED |
| P9 | 同比取目标期之后的首个有效期 | KILLED |
| P10 | 同比基于最后一期而不是该指标的 Period | KILLED |
| P11 / P12 | 阈值 / 恶化 Alert.Period 改取 r.Latest | KILLED |
| P13 | Latest 取首个指标的 Period | KILLED |
| P14 | Latest 取最后一期（包括全 NaN 的尾期） | KILLED |
| P15 | 幂等写入参 | **SURVIVED** |
| P15b | 原地前向填充入参 | **SURVIVED** |
| P15c | 非幂等写入参 | KILLED |
| P16 / P17 | NaN 参与阈值 / 恶化比较时判为真 | KILLED |
| P18 | 恶化方向符号取反 | KILLED |
| P19 | 恶化 Alert 的 Value 取 Value 而非 QoQ | KILLED |
| P20 | Value 取末期原值但 Period 回退 | KILLED |

合计 29 个变异体，27 个 KILLED，2 个 SURVIVED（都属于 non_functional[0]）。

## 返工范围
只需修改 analyze_test.go 的测试隔离，analyze.go 无需改动。其余 6 条的判定依据在返工后仍然有效，只要 analyze.go 不变、fixture 数值不变就不必重验，复验时重点看修复后 P15 和 P15b 能否转红。
