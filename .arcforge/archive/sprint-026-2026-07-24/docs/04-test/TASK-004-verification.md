# TASK-004 复验报告 (rework#2 / QA WARNING-2) — refreshEdgar 路径

- **验证者**: test-agent-4 (Reality Checker)
- **任务状态**: verifying(epoch=2, rework 2/3) → **VERIFIED**
- **commit**: 1335169（在 rework#1 b2d0dd6 基础上修 QA WARNING-2）
- **判定**: **PASS** — ttmPoints 季度连续性守卫正确;阈值 330 经独立实测验证;既有断言零改动
- 说明:rework#1（functional[2]/boundary[1]）与首验各项此前已 VERIFIED,本节聚焦 WARNING-2 修复。

## QA WARNING-2 修复核查

**修复**:ttmPoints 4 季滑窗新增季度连续性守卫——首尾 period_end 跨度 > maxTTMWindowDays(330) 则跳过(整季缺失时窗口跨缺口求和会使 TTM 失真)。

### 1. TestTTMPointsQuarterGap（数点数，非弱化）
- 12 季无缺口 → `assert.Len(ttmPoints,9)` ✓
- 整条删除中间季(idx6) → 11 季 8 候选窗口,3 个跨缺口窗口(365天>330)被剔 → `assert.Len(pts,5)` ✓
- 断言缺失季 period_end 不作任何 TTM 点日期 ✓
- 亲跑 PASS。与 rework#1 的 TestRefreshEdgarTTMGap(NaN 场景,窗口仍 273-275<330 靠 NaN 门跳过)并存双机制,均 PASS。

### 2. 阈值 330 裁量复核（dev 偏离 fix_item「370~400」，实测裁定）
独立构造 edgarQuarters(3 月间距)窗口实测跨度:
- **正常连续 4 季窗口:273~275 天**(9 个窗口全在此区间)。
- **单季缺口窗口:365 天**(删 idx6 后 3 个跨缺口窗口全为 365)。
- 53 周财年最坏(含一个 14 周季):3 季跨度 = 280 天。
裁定:330 落于「正常上界 280（含 53 周财年）」与「单季缺口 365」之间,上余 50 天、下余 35 天,两侧余量充分。**dev 偏离是必要修正**:fix_item 建议的 370~400 高于单季缺口实测 365,会漏判缺口窗口(365<370 不剔除、错误产点);330 正确区分。未发现使正常连续窗口 >330 的真实反例(53 周财年最多 280)。**PASS**。

### 3. WARNING-3 不修记录（QA 复核结论）
discovery 如实记录:「QA WARNING-3(派生值口径)不修——刻意设计」,rationale「PB/PS 派生值采用最新基准+原始 filing date 生效口径,smoke 已实证可接受,非缺陷」。记录在位、结论明确。

## 回归/覆盖/越界

- 既有断言**零改动**(diff 删除行无 assert/require,纯新增 const+守卫+新测试+注释)。
- 两包全量回归 `go test ./internal/prism/ ./internal/config/` 全 `ok`;此前各条 PASS 不受影响。
- ttmPoints 100% 覆盖;门禁口径覆盖率 **94.5%** ≥ 80。
- 越界(git show --name-only):仅 internal/prism/refresh.go、refresh_test.go,无越界。

## 结论

QA WARNING-2 修复正确:季度连续性守卫剔除跨缺口窗口;阈值 330 经独立实测(正常 273-275/缺口 365)验证两侧余量充分,dev 偏离 fix_item 的 370~400 是防漏判的必要修正;WARNING-3 不修记录在位。既有断言零改动、回归零漂移、覆盖率 94.5%、无越界。**VERIFIED**。
