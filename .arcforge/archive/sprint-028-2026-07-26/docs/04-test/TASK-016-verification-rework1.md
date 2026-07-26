# TASK-016 复验报告（rework-1，聚合防御）

- 验证者: test-agent-6 ／ 本轮 assignment_epoch: **2**（rework_count=1）
- 返工 commit: `a508843`（首轮 `1fd162d`）
- 首轮报告: `.arcforge/docs/04-test/TASK-016-verification.md`（判定 REJECTED）
- **判定: VERIFIED**

## 1. 首轮 REJECTED 的理由已解除

| 变异 | 首轮 | 本轮 |
|---|---|---|
| `len(qs) > 4` → `> 5` | ⚠ 存活 | **✅ 杀死** → `TestBuildPeriodsFiscalYearQuarterCountBoundaries/five_quarters_rejected` |

5 季档位已补，边界值 `4` 现在有锚点。

## 2. 首轮「强烈建议项」也已补齐 —— 且比我要求的更完整

| 变异 | 首轮 | 本轮 |
|---|---|---|
| `periodEndSlackDays` 15 → 14 | ⚠ 存活 | **✅ 杀死** → `/15_days_is_still_the_same_quarter`，信息「相差 15 天应视…」 |
| `periodEndSlackDays` 15 → 30 | ⚠ 存活 | **✅ 杀死** → `/16_days_is_a_different_quarter`，信息「相差 16 天是另一个季度，不得并入本期」 |
| 容差比较 `<=` → `<` | 未测 | **✅ 杀死** → `/15_days_is_still_the_same_quarter` |

**新增的 `aboutAYearApart` 也自带边界锚点**（本轮新写的防御，没有重蹈首轮覆辙）：

| 变异 | 结果 |
|---|---|
| `minYearGapDays` 340 → 300 | ✅ 杀死 → `TestBuildMatrixYoYRejectsWrongDistanceComparison/339_days_is_too_close` |
| `maxYearGapDays` 390 → 450 | ✅ 杀死 → `/391_days_is_too_far` |

## 3. 回归抽验（首轮已杀死的，仍杀死）

| 变异 | 结果 |
|---|---|
| 去掉 `> 4` 上界防御 | ✅ 杀死 → 4 个用例，信息「6 个季度不是一个财年，宁可不产出也不能相加成一个「完整财年」」 |
| 冲突列表恒空（去可观测性） | ✅ 杀死 → 含 `TestAnalyzeSurfacesPeriodConflicts`（AD-26 透传链仍有守护） |

全部按**两步核对**：被点名的测试在变红列表里，且失败信息指向该规则本身。

## 4. dev 主动报告的三件事：逐条核实

**① `daysBefore` 补齐没有削弱任何断言。**
`git show a508843 -- periods_test.go | grep -E "^-[^-]" | grep -E "assert|require"` → **空**。
`daysBefore` 是纯新增 helper（`periods_test.go:409`，3 处调用），无断言被删改。

**② 新字段盲区已钉住 —— 独立确认。**
变异「FY 的 `PeriodEnd` 取首季而非末季」→ **✅ 杀死** → `TestBuildPeriodsFYAggregation`，
信息「财年的 period_end 是它最后一季的，不是第一季的」。
dev 自报该变异最初存活（新增字段时没配断言），补断言后杀死 —— **属实，现已有守护**。

**③ 数字更正已落地**：`periods.go:71` 与 `periods_test.go:212` 均为 12.7% / 4 组，
全文无 27% 与 7/26 残留。

## 5. 实跑与回归

```
GOTOOLCHAIN=local go test ./internal/prism/sankey/ -count=1 -cover → 97.5%（>96.1% 基线）
gofmt -l → 空 ； go vet → ok ； go build ./... → ok
prism / sankey / cmd/atlas / collector/edgar 四包回归全 ok
```

## 6. 解决了一个跨三方的悬案：**覆盖率数字本身是非确定的**

首轮 dev 报 97.7 / Leader 98.0 / 我 97.7；本轮 dev 97.4 / Leader 97.5 / 我 97.5 与 97.8 都出现过。
**同一份代码、同一条命令，实测在 97.5% 与 97.8% 之间跳动**（连测 12 次：10 次 97.5%、2 次 97.8%）。

**根因已定位到具体语句**：`periods.go:206-208`，即 `sameQuarter` 里
「`time.Parse` 失败 → `return false`」这一分支，**时而被覆盖、时而不被覆盖**。

机制：`segmentsAt` 遍历 buckets **命中即返回**。`TestBuildPeriodsUnparsablePeriodEnd` 的 fixture 有两个桶
（`2026-03-31` 合法、`n/a` 不可解析）；若合法桶先被遍历到就直接返回，**`n/a` 桶根本不会被送进
`sameQuarter`**，该分支自然不被执行。而遍历起点是随机的（见首轮报告已坐实的
「Go 单桶小 map 在 8 个 cell 上取随机起点，P(插入序)=(9−n)/8」）。

**结论：三方的覆盖率分歧不是测量误差，也不是谁读错了 —— 是这个包的覆盖率确实在波动。**
今后比较应以多次采样为准，单次读数不足以支撑「谁的数对」。

## 7. 由此发现的一个加固点（非 DoD，不影响判定）

上述机制还有一个更重要的推论：**该分支的守护是概率性的**。

```
变异：sameQuarter 中「日期不可解析」由 return false 改为 return true（即「匹配一切」）
实测：25 次中变红 6 次 —— **存活率 76%**
```

也就是说，`TestBuildPeriodsUnparsablePeriodEnd` 大约只有四分之一的跑动能抓住这个退化。
后果是「不可解析的期末被并进本期」这个真实错误可能在 CI 里连续多次不被发现。

**这与我在 TASK-008 发现的 X16 是同一类问题**（断言依赖 map 遍历顺序），
而 dev 当时给出的解法可以直接复用。**修法（择一，均为小改动）**：
1. **加一个只含不可解析桶的子用例** —— 无合法桶可短路，该分支必然被执行，**确定性最强**；
2. 或沿用它在 TASK-008 用过的**用例内循环 N 轮**（存活率 `p^N`，不依赖对 p 的估计）。

**不作为 REJECTED 依据**：不可解析日期的处理属 DoD functional[1] 里「具体策略由实现者定」的范畴，
不是 DoD 写死的量值或行为。

## 8. 判定

**VERIFIED** —— 首轮 REJECTED 的唯一理由（上界 `4` 无锚点）已解除；
首轮提出的强烈建议项（±15 容差三档）已补齐，且本轮新增的 `aboutAYearApart` 自带 339/391 边界锚点；
9 个变异体全部按两步核对杀死；dev 自报的三件事逐条属实；覆盖率 97.5% 高于 96.1% 基线。

§7 的概率性守护属加固建议，移交 Leader 判断归属。
