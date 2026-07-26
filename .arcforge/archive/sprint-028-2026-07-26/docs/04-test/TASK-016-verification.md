# TASK-016 验证报告（聚合防御：拒绝标签冲突导致的错误财年与跨季相加）

- 验证者: test-agent-6 ／ assignment_epoch: 1 ／ 交付 commit: `1fd162d`
- **判定: REJECTED（7 条 DoD 中 6 条通过；functional[0] 的上界 `4` 无断言锚点）**
- **返工范围极窄**：补 1 个 5 季用例即可解除；另强烈建议同轮补 ±15 天容差的边界档位。

## 1. 实跑证据

```
GOTOOLCHAIN=local go test ./internal/prism/sankey/ -count=1 -cover
  ok  internal/prism/sankey  0.467s  coverage: 97.7%   （连测 3 次均为 97.7%）
go vet ./internal/prism/sankey/ → ok ； gofmt -l → 空
go build ./... ok ； prism / sankey / cmd/atlas 三包回归全 ok
```

**覆盖率分歧记录**：Leader 实测 98.0%、dev 报 97.7%。**我连测 3 次稳定 97.7%**，与 dev 一致。
两者都远超 96.1% 基线，不影响判定；但确实存在运行间差异，建议以多次采样为准。

## 2. REJECTED 的唯一理由：上界 `4` 没有锚点

**这与 TASK-008 的 ±3 容差完全同构。**

| 变异 | 结果 |
|---|---|
| `len(qs) > 4` → `len(qs) > 5` | **⚠ 存活** |
| 去掉 `len(qs) > 4` 上界防御（DoD 明写） | ✅ 杀死 → `TestBuildPeriodsRejectsOversizedFiscalYear`、`TestAnalyzeSurfacesPeriodConflicts`，信息 `[]string{"FY2025"} should not contain "FY2025" ｜ 6 个季度不是一个财年，宁可不产出也不能相加成一个「完整财年」` |

**根因**：现有档位只有 **4 季（接受）** 与 **6 季（拒绝）**——
- `> 4`：4 接受、6 拒绝；
- `> 5`：4 接受、6 拒绝 —— **两档表现完全一致**。

**只有 5 季能区分**（原实现拒绝、变异接受），而 `periods_test.go` 里没有 5 季用例。

DoD functional[0] 明文写的是「对 **`len(qs) > 4`** 的财年拒绝产出 FY」——**这个边界值写在 DoD 里却没有任何断言锚定它**，
按我在 TASK-008 提出、Leader 已采纳的口径，属该条 DoD **验证不完整**。

**修法（约 6 行）**：加一个 5 季同财年标签的用例，断言不产出该 FY 且 `conflicts` 含该 FY。
补后 `>4 → >5` 应被杀死。

> 说明：DoD 原文只要求「测试用 6 个同财年标签的季度构造」，dev **按字面完成了**。
> 这是 DoD 措辞的疏漏（Leader 已口头转达补正但未落进 DoD 文本），**不是 dev 的失职**。

## 3. 强烈建议同轮补：`periodEndSlackDays = 15` **完全没有锚点**

| 变异 | 结果 |
|---|---|
| `15 → 14`（收紧） | **⚠ 存活** |
| `15 → 30`（放宽一倍） | **⚠ 存活** |

**比 ±3 那次更彻底**——TASK-008 至少 `3→5` 被抓住，这里**两个方向都无人防守**。

该常量直接决定正确性：过小 → 真实数据里分部全部对不上、静默落进残差；
过大 → 相邻季度被误判为同一季，正是本任务要防的跨季混淆。目前它是个**无守护的魔数**。

**它不在 DoD 文本内**（DoD functional[1] 写的是「具体策略由实现者定……必须在 discovery 说明选择理由」），
**故不作为 REJECTED 的依据**，但强烈建议同轮补 14/15/16 三档（同 TASK-008 的 −3/+3/−4/+4 做法）。

## 4. 其余 6 条 DoD 逐条通过

| # | 完成标准 | 变异验证（两步核对：被点名测试在列 + 失败信息指向该规则） | 判定 |
|---|---|---|---|
| F0 | >4 拒绝产出 FY **且可观测** | K2 去防御 → 杀死（信息见 §2）；**K7 冲突不上报 → 杀死**，信息 `"2025Q4: 标签对应 3 个不同季度 (2023-06-30, 2024-06-30, 2025-06-30)…" does not contain "FY2025"` —— 可观测性有守护 | **FAIL**（仅因 §2 的边界锚点） |
| F1 | `buildQuarters` 不得静默 `+=` | **K3 恢复跨季相加 → 杀死**，信息 `Max difference between 3.4681e+10 and 6.1432e+10 …` —— **DoD 指定的 61.432 与 34.681 被逐值锚定** | **PASS** |
| F2 | 真实形态回归 | `TestBuildPeriodsSegmentLabelSpanningThreeYears`（2025Q4 ×3 → 2023/2024/2025-06-30）、`TestBuildPeriodsRejectsCrossQuarterSegmentSum`、`TestAnalyzeSurfacesPeriodConflicts`（FY2026 收 6 季） | **PASS** |
| B0 | 既有 18 个测试零断言修改 | diff 中删除行仅 7 处：5 处 `got :=` → `got, _ :=` 的**元数适配**，2 处 `assert.Empty(BuildPeriods(...))` 因返回值变两个无法内联，改写为 `q, qConflicts := ...` 后**断言不减反增**（多断言了 conflicts 为空）。**无任何断言被弱化或删除** | **PASS** |
| B1 | 恰为 4 时仍产出 FY | 既有 FY 聚合用例（4 季）全绿，未被上界防御误伤 | **PASS** |
| N0 | 变异 (a)(b) 必须变红 | (a) K2 ✅ ／ (b) K3 ✅，均按两步核对 | **PASS** |
| N1 | 全绿且覆盖率 ≥96.1% | 97.7%（连测 3 次） | **PASS** |

**AD-26 签名变更与透传链**：`BuildPeriods` 返回 `([]PeriodMetrics, []PeriodConflict)`；
`service.go:108` 有 `Conflicts: conflicts` 落进 `Analysis`，`TestAnalyzeSurfacesPeriodConflicts` 守护该透传。
**没有重蹈 AD-22 Notes 的坑**（信号产出后在装配层被丢掉）。

**F1 策略的独立评估**：dev 弃用 Leader 给的三个候选（取最新／拒绝标签／标记冲突），
改用「分部桶按 `(label, period_end)` 二级键 + 主表行按自身 `period_end` 匹配」。
**我认可这个思路**：三个候选都在**猜哪一季才是对的**，而主表行自带 AD-9 认定的权威主键，
用它匹配就不需要猜。K6 变异（对不上时返回首个桶）被 `TestBuildPeriodsNoSegmentBucketMatchesQuarter` 杀死，
信息 `Should be empty, but was map[cloud:2.6e+10] ｜ 对不上就没有分部数据，不能拿别的季度的值顶上`
—— 「宁可没有也不要错值」的语义有守护，与描述一致。

## 5. dev 报告的三件过程事实：均核实

**① 「编译错误不是有效变异」**——**独立印证**：我第一版 K3 变异也因未使用变量编译失败。
编译失败只证明代码引用了该符号，不证明测试有区分力。我改用可编译版本（合并全部桶）后 K3 被正常杀死。
dev 的这个判断正确，且它最终用的确实是可编译版本。

**② code-simplifier 自报 Unchanged 但实际有改动**（引入 `segmentBuckets` 命名类型、抽出 `sortConflicts`）。
**核实通过**：我的全部变异都跑在**交付 commit `1fd162d`（已含简化后代码）**上，
K2/K3/K6/K7 四个变异均被正常杀死 —— **简化后的代码仍能守住那些规则**。

**③ 覆盖率分歧**：见 §1，我三次均得 97.7%。

## 6. 判定与返工边界

**REJECTED**，理由**仅限** functional[0] 的上界值 `4` 无断言锚定（`>4 → >5` 变异存活）。

**不必返工的部分**（已逐条实测通过）：F1 / F2 / B0 / B1 / N0 / N1 六条、
可观测性守护（K7）、AD-26 透传链、`(label, period_end)` 二级键策略、对不上时返回 nil 的语义。

**建议一轮补完**：
1. **必须**：5 季用例（解除 REJECTED，约 6 行）；
2. **强烈建议**：`periodEndSlackDays` 的 14/15/16 三档（当前两个方向都无守护）。

---

## 7. 更正记录：任务描述里的「27% 标签冲突」应为 **12.7%**（Leader 事后更正）

**本报告正文未引用该数字**（已 grep 确认），故结论与判定不受影响。此节仅为阻断错误数字继续传播。

**更正内容**（Leader 消息，2026-07-25）：
- 描述里的「27% 的 FiscalPeriod 标签有冲突」「7/26 个标签重复」「2018Q4 有 4 个不同季度」
  是 Leader 复刻过滤管道时**漏了 `fp` 检查**导致的高估；
- 真实为 **4 组 / 32 个标签，影响 71 行中的 9 行（12.7%）**：
  `2025Q4 ×3 → [2023-06-30, 2024-06-30, 2025-06-30]`、`2026Q1 ×2`、`2026Q2 ×2`、`2026Q3 ×2`；
- **不存在任何 ×4 组**，描述里的 2018Q4/2019Q4/2020Q4 ×4 是幻影。

### 我能独立核实的部分：**根因机制成立**

无 `data/prism.db`（仓库内不存在），**故 12.7% 这个数字我无法独立复核**，此处按 Leader 的更正记录。
但其根因是代码级主张，我直接读源码验证：

```go
// internal/collector/edgar/client.go
func isSingleQuarter(f rawFact) bool {
	if f.FP != "Q1" && f.FP != "Q2" && f.FP != "Q3" {
		return false          // ← Leader 复刻时漏掉的正是这一行
	}
	d, ok := durationDays(f)
	return ok && d >= 70 && d <= 100
}
```

**确认**：`fp=FY` 但 duration 90 天的条目（年报里披露的季度数据）会被此处拒绝，
**根本进不了 `fundamental_q`**，因而不产生任何标签冲突。Leader 的高估来源属实。

**dev-agent-15 的推翻理由同样在代码层成立**（「这个形态在代码里产生不出来」）：
- 单季条目的标签来自 `client.go:529` `label(end, f.FY, f.FP)`，而 `f.FP` 经 `isSingleQuarter`
  过滤后只可能是 Q1/Q2/Q3 —— **单季标签不可能是 Q4**；
- Q4 标签唯一来源是 `client.go:566` 的派生年度条目 `label(a.end, a.fy, "Q4")`。
  故「同一 Q4 标签对应 4 个不同季度」需要 4 个同 fy 的年度条目，与 end 相隔一年的事实矛盾。

### 对本次验证的影响：无

- **缺陷本身与严重度不变**：FY2025 productivity 报 392.914B（真值 120.810B，3.25 倍）
  等数字来自真实 DB 实跑，不受比例更正影响，发布阻断级定性成立；
- **F1 的断言值不受影响**：26.751 / 34.681 → 不得为 61.432 来自 `2026Q3 ×2`，
  是更正后仍成立的四组之一；
- **REJECTED 的理由（上界 `4` 无锚点）与本更正无关**。

**给后续读者的提示**：`.arcforge/tasks/TASK-016.json` 的 `description` 字段仍含 27% 与 ×4 形态
（`verifying`/`rejected` 状态下 leader 无 update 权改不了）。**以本节为准。**
若 dev 已照描述里的 ×4 形态建了 fixture，那部分描述的是**不可能出现的形态**——
不构成缺陷（防御对任何冲突形态都应生效），但不宜作为"真实数据回归"的依据。
