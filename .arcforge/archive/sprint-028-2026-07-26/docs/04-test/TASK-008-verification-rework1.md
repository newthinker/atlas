# TASK-008 复验报告（rework-1，RefreshSegments）

- 验证者: test-agent-6 ／ 本轮 assignment_epoch: **2**（rework_count=1）
- 返工 commit: `b3507c3`（首轮为 `08f0eda`）
- 首轮报告: `.arcforge/docs/04-test/TASK-008-verification.md`（判定 REJECTED）
- **判定: VERIFIED**

## 1. 返工范围核实（先确认没有夹带）

```
git show --stat b3507c3      → 仅 internal/prism/segments_test.go，+122/−30
git diff 08f0eda b3507c3 -- internal/prism/segments.go internal/prism/refresh.go
                             → **空输出，实现零改动**
顶层用例数 13 → 14；diff 中无 `-func Test` 删除行
（两处 `t.Run` 被删是被表驱动的五档版本取代，非用例减少）
```

**返工只动测试、不动实现** —— 与「首轮实现本就正确、缺的是断言锚点」这一判定完全吻合。

## 2. 判据：变异体死没死（不看用例存不存在、不看注释）

这是我首轮立下、Leader 认可的判据。八项全部实跑：

| # | 变异 | 首轮 | 本轮 |
|---|---|---|---|
| R1 | 容差常量 `3 → 2`（收紧） | ⚠ 存活 | **✅ 杀死** → `容差边界offset-3d` / `offset+3d` |
| R2 | 比较符 `<=` → `<`（边界排他） | ⚠ 存活 | **✅ 杀死** → 同上两个子例 |
| R3 | Q4 上界改闭区间（自吞噬） | ⚠ 存活 | **✅ 杀死** → `跨轮次:Q4 已存在时仍可重算并传播重述` |
| R4 | 容差 `3 → 5`（放宽，回归抽验） | 杀死 | ✅ 杀死 → `offset-4d` / `offset+4d` |
| R5 | Q4 负值改为落库（回归抽验） | 杀死 | ✅ 杀死 |
| R6 | CIK 校验挪到取数之后（回归抽验） | 杀死 | ✅ 杀死 |
| R7 | X6 去掉财年级 `len(ends)!=3` 守卫 | ⚠ 存活 | ✅ 杀死 → `财年内季度数不为 3 时整期跳过` |
| R8 | X16 `sortedKeys` 去排序 | ⚠ 存活 | ✅ 杀死 → `DeterministicOrdering`（检出率见 §4） |

**REJECTED 的唯一理由（R1/R2）已解除，且 R3 一并补上。** 基线未变异时包测试绿、覆盖率 95.5%（首轮 94.9%）。

## 3. 两处关键实现的独立确认

### R3 确为**强版** —— 且这一点由变异结果本身证明
我在首轮预热时已实测：**弱版（第二轮同数据、断言 Q4 仍为 17）根本杀不掉 X3**
——因为自吞噬会让第二轮整个财年跳过、`UpsertSegments(id, [])` 空写入，
第一轮的 17 原封不动留着，「仍为 17」恰好成立。

**因此「R3 被杀死」本身即证明该用例不是弱版**，无需依赖注释或用例名。
读代码复核也一致：

```go
// 第二轮:年报重述,FY 从 50 改为 60 → Q4 应随之更新为 60−33=27
assert.Equal(t, 27.0, got["2026-06-30|intelligent_cloud"].Revenue,
    "第二轮:Q4 须能重算并吸收重述(仍为 17 说明该财年被整个跳过,Q4 已被自己吞掉)")
assert.Equal(t, 27.0, got["2026-06-30|productivity"].Revenue, "未重述的 segment 保持 90−63")
assert.Len(t, store.segments["MSFT"], 8, "重算不得产生额外行")
```
断言 27 而非 17，另有「未重述 segment 保持不变」与「不产生额外行」两条兜底。

### 容差改为五档表驱动，取到了「正好等于阈值」
```go
{-4, false}, {-3, true}, {0, true}, {3, true}, {4, false}
```
命中档断言 `FiscalPeriod == "2026Q3"` 且 `rep.Degraded` 为空；
未命中档断言空标签 + `Degraded` 恰 1 条且含期末日期。
**两档都断言了「行仍落库」与「主键用分部期自身的 period_end」** —— AD-9 的负向要求未因改表驱动而丢失。

## 4. 一处精确观察（非 DoD，不影响判定）

`TestRefreshSegmentsDeterministicOrdering` 是**概率性检出**，不是确定性守护：

```
施加 X16（sortedKeys 去排序）后连跑 30 次：变红 26 次，**存活 4 次（13%）**
```

原因是该用例断言 3 条 Degraded 的顺序（Alpha/Mike/Zulu），而 Go map 的 3 元素随机迭代
恰好产出升序的概率是 1/6 ≈ 17% —— 与实测 13% 吻合。

**dev 自述的「连跑 5 次 5/5 全红」不足以说明该守护是确定的**：
在 1/6 存活率下，连续 5 次全红的概率约 (5/6)^5 ≈ 40%，属常见结果。

**加固建议（可选，非 DoD）**：把未映射 member 从 3 个增到 5 个，
偶然升序概率由 1/6 降到 1/120（<1%）；成本是 fixture 里加两行。

## 5. done_criteria 状态

| # | 完成标准 | 本轮 |
|---|---|---|
| F0 | since=锚点／member 映射／未映射记 Degraded | 首轮已通过，回归绿，**不重验** |
| F1 | AD-12 force 全量重拉 | 首轮已通过，回归绿，**不重验** |
| F2 | AD-9 主键与 ±3 容差反查 | **本轮转 PASS**（R1/R2 杀死；五档表驱动） |
| F3 | Q4 推导 + AD-17 + **跨轮次重算** | **本轮补强并 PASS**（R3/R5/R7 杀死） |
| F4 | manual 覆盖与跨轮次回压 | 首轮已通过，回归绿，**不重验** |
| B0 | 无模板跳过／空 templates 零值 Report | 首轮已通过，**不重验** |
| E0 | 单标的失败只进 Failed | 首轮已通过，**不重验** |
| N0 | 全包绿（含 TASK-006 既有） | ✅ 95.5%；全仓 prism/sankey/cmd/atlas/storage/edgar 五包回归全 ok；gofmt 空、vet ok |

函数级覆盖：`RefreshSegments` 100%、`lookupFiscalPeriod` 100%、`withinDays` 100%、
`quarterRows` 100%、`manualRows` 100%、`segmentSince` 100%、`sortedKeys` 100%；
`deriveQ4` 90.9% → **97.0%**、`refreshSymbolSegments` 91.1%。

## 6. 判定

**VERIFIED** —— 首轮 REJECTED 的唯一理由（±3 容差量值无断言锚定）已解除，
三个必须变红的变异体（R1/R2/R3）全部被杀死，六项回归抽验无退化，
两个非 DoD 项（X6/X16）也一并补上。实现零改动，既有用例零删除。

§4 的检出率观察属非 DoD 加固建议，移交 Leader 判断是否值得一行 fixture 改动。
