# TASK-008 验证报告（RefreshSegments 分部刷新编排）

- 验证者: test-agent-6 ／ 承接时 assignment_epoch: 1 ／ 交付 commit: `08f0eda`
- **判定: REJECTED（8 条 DoD 中 7 条通过；functional[2] 的 ±3 容差量值未被钉住）**
- **返工范围极窄**：只需补容差边界用例（+ 建议同轮补 3 处非 DoD 缺口），**其余一律不必动**。

## 1. 实跑证据

```
gofmt -l ./internal/prism/  → 空 ； go vet → ok
GOTOOLCHAIN=local go test ./internal/prism/ -count=1 -cover
  ok  github.com/newthinker/atlas/internal/prism  0.304s  coverage: 94.9%（基线 94.5%）
全仓：go build ./... ok；prism / sankey / cmd/atlas / storage/prism 四包回归全 ok
```

函数级：`RefreshSegments` **100%**、`lookupFiscalPeriod` 100%、`withinDays` 100%、
`quarterRows` 100%、`manualRows` 100%、`segmentSince` 100%；`deriveQ4` 90.9%、
`refreshSymbolSegments` 91.1%（未覆盖块均为 store 错误传播与坏日期分支）。

**签名偏离已主动披露**：实际为 `RefreshSegments(cfg, store, seg, templates, manualDir string, force bool)`
—— 相对 design-spec §4.2 增 `manualDir`、去 `now`。dev 在 discovery 给了理由
（`LoadManualSegments` 需要目录而 `config.PrismConfig` 无该字段；`now` 在实现中无任何读取点）
并注明「TASK-010 接线时请按此签名」。**理由成立，我认可**；建议 Leader 同步进 design-spec §4.2。

## 2. done_criteria 逐条覆盖矩阵

| # | 完成标准 | 对应测试 | 变异验证 | 判定 |
|---|---|---|---|---|
| F0 | since=锚点；已映射 member 以 source=edgar_segment 落库；未映射记 Degraded 且不失败 | `TestRefreshSegmentsIncrementalAndMapping` | X10 未映射不记 Degraded → 杀死；X7' source 改名 → 杀死；X13 FiscalPeriod 恒空 → 杀死 | **PASS** |
| F1 | AD-12 force=true 忽略锚点传零值 since | `TestRefreshSegmentsForceIgnoresAnchor` | X9 force 时仍用锚点 → 杀死 | **PASS** |
| F2 | AD-9 主键 period_end；**±3 天容差**反查；未命中仍落库+Degraded（负向）；≥2 条报错 | `TestRefreshSegmentsFiscalPeriodLookup` 3 子例 | X11 未命中不记 Degraded → 杀死；X12 取首个 → 杀死；**T1 容差 3→2 → ⚠ 存活**；**T3 `<=`→`<` → ⚠ 存活** | **FAIL**（见 §3） |
| F3 | Q4 逐 segment 推导；某 segment 凑不齐跳过该 segment；负值不落库+Degraded；**AD-17 FY 期不落库** | `TestRefreshSegmentsQ4Derivation` 3 子例 | X4 负值落库 → 杀死；X5' 去 per-segment 守卫 → 杀死；X15 Q4 source 改名 → 杀死 | **PASS** |
| F4 | manual 最后 upsert、source=manual、覆盖同键；**下一轮 auto 再次覆盖 manual** | `TestRefreshSegmentsManualOverride`（**连跑两轮**）、`ManualUnresolvableFiscalPeriod` | X8 manual source 改名 → 杀死；X14 凭空造 period_end → 杀死 | **PASS** |
| B0 | 无模板标的跳过（零请求）；templates 空 map → 零值 Report 且不报错 | `TestRefreshSegmentsSkipsUntemplated`（含 `assert.False(called)`） | — | **PASS** |
| E0 | 单标的失败只进 Failed，其余继续并落库 | `TestRefreshSegmentsPartialFailure`（断言 `Refreshed==1` 且其余标的确有行） | — | **PASS** |
| N0 | 全绿（含 TASK-006 既有测试） | 见 §1 | — | **PASS** |

### AD-17 的断言形态：**正是我在预案中预判的正确形态**
我预案里点名过一个陷阱——FY 期与 Q4 的 `period_end` 是同一天，故「FY 期不落库」**不能**断言
「不存在该 period_end 的行」（那行恰恰该存在），而必须断言**值**。dev 的写法：

```go
require.NotZero(t, q4ic.PeriodEnd, "Q4 行须落库")
assert.Equal(t, 17.0, q4ic.Revenue, "Q4=FY−Σ三季;若落成 50 说明 FY 期被直接落库")
assert.Len(t, store.segments["MSFT"], 8, "2 segment × 4 季,FY 期本身不额外落库")
```
断言的是**值 17 而非 FY 全额 50**，并用行数 8 兜底。**形态正确，陷阱已避开。**

### CIK 串号校验（超出 DoD，Leader 已批准）：**「零请求」成立**
实现把校验放在 `upsertMeta` 与 `FetchSegmentRevenue` **之前**（segments.go:72-74）。
测试有 `assert.Empty(t, seg.calls, "串号模板不得发起任何请求")`。
变异 X2（把校验挪到取数之后）→ **被杀死**。不是「先拉了再发现串号」。

### TASK-006 行为锁定补测：**四件事齐全**
1. price_daily 仍写入（且逐点比对 close 值 12.5 / 11.0）；2. 进 `Report.Failed` 且含 `non-positive`；
3. `assert.Empty(store.upserts["LOSS"])` 估值不落库；4. `Refreshed == 0`。
第 1 条（熔断仍写行情）正是本次补测的全部意义，已钉住。

## 3. REJECTED 的唯一理由：±3 容差量值只被「上界」钉住

**独立复现 dev 的推演（未采信，实跑变异）**：

| 变异 | 结果 |
|---|---|
| 容差常量 `3 → 5` | **杀死** → `TestRefreshSegmentsFiscalPeriodLookup` |
| 容差常量 `3 → 2` | **⚠ 存活** |
| 比较符 `<=` → `<`（边界排他） | **⚠ 存活** |

**dev 的自查结论完全属实。** 根因是用例取值为 `+2`（命中）／`+4`（不命中）／`−1,+1`（歧义），
**没有任何用例取正好 ±3 天**：
- 容差收窄到 2：+2 仍命中、+4 仍不命中、±1 仍都命中 → 三个用例全绿；
- `<=` 改 `<`：同理全绿。

即：实现用 3 天且闭区间是**对的**，但**任何把容差改小或把边界改成排他的改动都不会有测试变红**。
DoD functional[2] 明文写的是「**±3 天**容差」，这个具体数值目前没有锚点。

**为什么这条判 FAIL，而 TASK-007/009 的同类发现我判 PASS**——我用的是同一把尺子，区别在于：
- TASK-007 的 M11、TASK-009 的 W2~W10：涉及的性质**不在 DoD 文本内**（或如 M11 那样仍被
  `NetMargin` 一行锁住），故列加固建议、判定照常；
- 本条：**「±3」是 DoD 里写死的具体数值**，且**两个方向的变异有一个完全不被捕获**。
  按我此前提出、Leader 已采纳的原则——「测试断言的期望值必须写死字面量才有锚定作用」——
  一个 DoD 指定的常量却没有任何断言锚定它，属于该条 DoD **验证不完整**，而非单纯的加固机会。

**修法（约 4 行）**：在 `TestRefreshSegmentsFiscalPeriodLookup` 增两个子例——
`−3d` 与 `+3d` 须**命中**（钉住闭区间与量值下界），`−4d` 须**不命中**（已有 +4d，补对称侧）。
补后 T1 与 T3 都应被杀死。

## 4. 我另外发现的 3 个存活变异体（**均非 DoD 违反**，建议同轮补掉）

| # | 变异 | 后果 | 优先级 |
|---|---|---|---|
| **X3** | Q4 上界 `d.Before(a.PeriodEnd)` 改为闭区间 | **第二轮起 Q4 永不重算**：Q4 的 period_end 恰等于 FY 期的 PeriodEnd，闭上界会把 Q4 自身数进来 → `len(ends)==4` → 整个财年跳过 → **重述数据永远更新不进来**（值不变、无报错、无 Degraded） | **高** |
| X6 | 去掉 `len(ends) != 3` 财年级守卫 | 4 季齐但某 segment 缺 1 季时，会用 3 个值除以 4 季跨度推导（per-segment 守卫挡不住这种组合） | 中 |
| X16 | `sortedKeys` 去掉排序 | 落库行序与 Degraded 文本序变随机；当前无断言依赖顺序，但确定性是**刻意加的**却无守护 | 低 |

**X3 值得展开** —— 这正是 Leader 提示的「Q4 自吞噬」防护。**实现是对的**
（`!d.Before(a.PeriodEnd)` 严格上界排除 Q4 自身），但**没有任何测试守护它**。
根因：现有 Q4 用例都是**单轮**的，而该 bug 只在**第二轮**显形（首轮 Q4 尚不存在，
`len(ends)` 正好是 3，行为正确）。
仓库里确实存在跨轮次测试（`TestRefreshSegmentsManualOverride` 连跑两轮），
但那个 fixture **没有年度期**，走不到 Q4 推导。

**修法**：在 Q4 用例里连跑两次 `RefreshSegments`，断言第二轮后 Q4 值仍为 17（而非消失或过期）；
更强的版本是第二轮把 FY 值改为 60，断言 Q4 随之更新为 27 —— 后者能同时钉住「重述可传播」。

## 5. 判定与返工边界

**REJECTED**，理由**仅限** functional[2] 的 ±3 容差量值未被锚定（T1/T3 两个变异体存活）。

**明确不必返工的部分**（已逐条实测通过，请勿重做）：
F0/F1/F3/F4/B0/E0/N0 七条 DoD、AD-17 断言形态、CIK 零请求、TASK-006 补测四件事、
manual 跨轮次回压语义、17 个变异体中已被杀死的 13 个。

**建议一轮补完**：§3 的容差边界用例（必须，解除 REJECTED）+ §4 的 X3（强烈建议）
+ X6/X16（可选）。
