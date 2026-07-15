# TASK-004 验证报告 — notify_render（一）NotifyContext/指标行/分区

- **验证者**: test-agent-1
- **提交**: 0beae94（notify_render.go +134 / notify_render_test.go +108，纯新增）
- **日期**: 2026-07-15，epoch=1，rework=0
- **判定**: REJECTED（NEEDS WORK）
- **一句话**: 高质量提交——跨包契约逐字段对齐、两处 impl 参考实现 bug 已正确修正、5 个关键变异（allGreen/两落界/iceberg/severity）全被拦截、函数级覆盖 100%。唯一 gap：functional[2] 明列的第三级排序判据「AllIndicators 序」无区分性用例，`SliceStable→Slice` 变异静默通过（正对应 Leader 概念#2 要求「三级判据各自有区分性用例」）。

## 亲跑证据
- `GOTOOLCHAIN=local go build ./...` → 通过；`go test ./internal/crisis/` → ok，coverage 93.0%
- notify_render.go 全部 6 函数函数级覆盖 **100%**
- 变异矩阵（每个应触发对应测试 FAIL）：
  | 变异 | 目标测试 | 结果 |
  |---|---|---|
  | allGreen(rest)→len(rest)==len(AllIndicators) | TestBodyZonesTitles | FAIL ✓ 拦截（补充决策 5 守住） |
  | sofr severity>=Amber→>Amber | TestIndicatorLineBoundaryTriggers | FAIL ✓ 拦截 |
  | usdjpy Wow<=amber→<amber | TestIndicatorLineBoundaryTriggers | FAIL ✓ 拦截 |
  | iceberg 层序 <→> | TestSplitZonesOrdering | FAIL ✓ 拦截 |
  | severity 降序 >→< | TestSplitZonesOrdering | FAIL ✓ 拦截 |
  | **SliceStable→Slice（第三级）** | TestSplitZonesOrdering | **ok ✗ 未拦截** |

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | Trend/NotifyContext 导出且符 §8+决策1 | 逐字段对 design-spec.md：Trend{Window []Observation, Delta float64}、NotifyContext 8 字段（Res/StateDays/SummaryDue/NewStale/StaleLastObs/PrevDay/ClearStreak/Trends）名与类型完全一致（T8 跨包消费编译即校验） | PASS |
| functional[1] | indicatorLine §5：hy_oas红/sofr持续(AMBER+&PersistDays>0)/usdjpy周跌(WowOK&&Wow<=amber)/tag | TestIndicatorLineRendering + BoundaryTriggers：hy_oas 红行、sofr 持续（RED+5、AMBER+3 落界）、usdjpy 周跌（-2.1%、恰-2.0% 落界）、tag 片段。字面值逐字对 §5 | PASS |
| functional[2] | splitZones 异常区严重度降序→冰山层序→**AllIndicators序**；其余🟢按序⚪殿后 | TestSplitZonesOrdering：severity 降序（M5 FAIL✓）、iceberg 层序（sofr<vix，M4 FAIL✓）、rest [t10y2y,nfci,usdjpy,move] ⚪殿后 ✓。**但第三级「同severity+同iceberg 按 AllIndicators 序」无区分性用例**——现有 abnormal 三键(severity,iceberg)互异，M6 `SliceStable→Slice` 静默通过。vix&move（同情绪层）或 t10y2y&nfci（同领先层）同色异常时第三级可达但未锁 | **FAIL** |
| functional[3] | bodyZones 全绿/有异常/含⚪三路径；monthDay("2026-07-14")="07-14"；stateRank NORMAL<WATCH<BREWING<CRISIS | TestBodyZonesTitles 三路径 + TestMonthDayAndStateRank。全 PASS | PASS |
| boundary[0] | Pct5y<0省略；NO_DATA无读数；STALE带读数+说明 | TestIndicatorLineRendering：nfci Pct5y=-1省略；"⚪ 领先 nfci 无数据(NO_DATA)"；"⚪ 情绪 move 88.1 · 数据断更(STALE)" | PASS |
| boundary[1] | CROWDED-only不带周跌(决策7)；含⚪无异常→"其余指标："(决策5) | usdjpy Wow=-0.005+CROWDED→无周跌片段；hy_oas STALE→HasPrefix"其余指标："（allGreen 修正验证） | PASS |
| boundary[2] | monthDay 非YYYY-MM-DD原样返回 | monthDay("2026-07")="2026-07"、monthDay("")="" | PASS |
| non_functional[0] (test) | 纯函数 + build+test 全绿 | 6 函数均纯（无 IO/无包级可变态）；亲跑通过 93.0% | PASS |

## Leader 四点核查回复
1. NotifyContext/Trend 契约：**逐字段对 design-spec 完全一致**，T8 跨包消费无字段名风险。PASS。
2. splitZones 三级判据：severity 降序 ✓、冰山层序 ✓（「同为红时流动性先于情绪」M4 拦截）——**但第三级 AllIndicators 序无区分性用例**（M6 静默通过）。此即本次唯一 FAIL。
3. bodyZones 三路径 + allGreen 变异：三路径全测，allGreen→len==7 变异被 TestBodyZonesTitles 拦截。PASS。
4. 两落界用例真实性：severity 恰 AMBER、Wow 恰 ==amber_wow_pct，M2/M3 变异确认真锁边界。PASS。

## impl 参考实现 2 处 bug 修正（交叉核对）
dev discovery decisions 记录与 Leader 说明一致：BUG1（NO_DATA 用纯空格接说明，非 " · "）、BUG2（allGreen 逐行校验替代 len==7，守补充决策 5）。均以 DoD+设计+文档自带测试为准，非拿参考实现逐行比对。测试断言体现正确行为，认可。

## detect_changes（Leader 代跑）
low、affected 空、仅两新文件、无越界。

## 拒绝原因（reject_reason）
functional[2] 第三级排序判据「同 severity+同 iceberg 时按 AllIndicators 序」无区分性用例：现有 abnormal 用例三键互异，`SliceStable→Slice` 变异静默通过。DoD 明列此级、Leader 概念#2 要求三级各有区分性用例。其余 7 条全 PASS（含契约、两落界、allGreen 变异均已锁）。

## 建议修复方向（小改，仅测试，约 4 行）
在 TestSplitZonesOrdering 追加同 severity+同 iceberg 对（vix 与 move 同为情绪层、AllIndicators 中 vix 在 move 前）：
```go
// 第三级：同严重度同冰山层 → AllIndicators 序（vix 先于 move，锁 SliceStable）
res2 := dayResult(StateWatch, StateWatch)
for _, ind := range []string{IndVIX, IndMOVE} {
    r := res2.Results[ind]; r.Status = StatusAmber; res2.Results[ind] = r
}
ab2, _ := splitZones(res2)
assert.Equal(t, []string{IndVIX, IndMOVE}, []string{ab2[0].Indicator, ab2[1].Indicator})
```

---

## 二次验证（增量，2026-07-15，epoch=2，rework=1，提交 ce52fa4）

**判定：VERIFIED（PASS）**

方案变更：原纯测试修复不可行（sort.Slice n≤7 插入排序恒稳定，SliceStable→Slice 黑盒不可区分）。改生产改动=显式三级全序比较器（severity→icebergRank→indicatorIndex），锁点转移到第三级比较方向。上轮 7 条 PASS 用例输出恒等不变。

| 复核点 | 证据 | 判定 |
|---|---|---|
| functional[2] 三级判据各有区分性用例 | 独立变异全序比较器三级：一级 `severity >`→`<` FAIL✓、二级 `icebergRank <`→`>` FAIL✓、**三级 `indicatorIndex <`→`>` FAIL✓（新锁点，同层对 [vix,move] 用例）**。三级各有独立区分用例 | PASS |
| 行为等价 | 既有 TestSplitZonesOrdering/TestBodyZonesTitles/TestIndicatorLine* 全部通过、输出不变；比较器重写不改任何排序结果 | PASS |
| SliceStable→Slice 无害确认 | 全序比较器下两者结果恒同，不再作判据（原因：n≤7 插入排序恒稳定） | N/A |
| diff 范围 | `git show --stat ce52fa4`：仅 notify_render.go(+25/-4)+notify_render_test.go(+14)。生产改动=splitZones 比较器 + 新增私有 indicatorIndex（含未知指标兜底测试）。无越界 | PASS |
| 覆盖率 | splitZones 100%、indicatorIndex 100%，包级 93.1% | PASS |
| detect_changes | Leader 重跑覆盖此提交（low、仅两文件） | PASS |

**附带观察（非阻塞）**：测试注释「锁 SliceStable 稳定性」措辞已略陈旧（现为显式全序比较器，非依赖稳定性），不影响正确性，供后续顺手更新。

**结论**：三级排序判据现各有独立区分性用例并经变异确认，行为等价。8 条 DoD 全 PASS。TASK-004 verified（并行窗口闸门）。
