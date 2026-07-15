# TASK-003 验证报告 — notify_format.go 格式化原语与 sparkline

- **验证者**: test-agent-1
- **提交**: c73778f（notify_format.go +185 / notify_format_test.go +101，纯新增）
- **日期**: 2026-07-15，epoch=1，rework=0
- **判定**: REJECTED（NEEDS WORK）
- **一句话**: 字面值逐条对照设计 §6.1–6.3 全部正确、函数级覆盖 100%、包级 92.5%，但 functional[2] trendArrow 恰好 |Δ|==eps 的边界方向未被任何断言锁住（变异 `>=`→`>` 测试静默全过）——与设计「< eps → →，否则 ↗/↘」的判别边界不符，属 verify_by=test 判据的边界未覆盖。

## 亲跑证据
- `GOTOOLCHAIN=local go build ./...` → 通过（exit 0）
- `GOTOOLCHAIN=local go test ./internal/crisis/ -count=1` → ok，coverage 92.5%
- notify_format.go 全部 13 函数函数级覆盖 **100%**
- 边界变异：`delta >= eps`→`delta > eps` 且 `<= -eps`→`< -eps` 后 TestTrendArrow 仍 `ok`（未 FAIL）→ 边界方向无测试锁定

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | 层名映射/冰山层序/statusEmoji(🔴🟡🟢⚪)/nonColorNote/tagText 符 §6.1–6.3 | TestLayerEmojiAndTagText：layerName 7 指标全对（情绪/流动性/信用/领先/旁证）；icebergRank 信用<流动性<情绪<领先<旁证；emoji 四色；nonColorNote "数据断更(STALE)"/"无数据(NO_DATA)"/"季末抑制"/""；tagText "压力(STRESS)"/"自满(COMPLACENCY)"/"空头拥挤(CROWDED)"/"倒挂后复陡(STEEPENING)"/""。字面值逐字对照设计正确 | PASS |
| functional[1] | formatReading/formatDelta 各指标格式 §6.3；formatPct5y(0.98)="98%" | TestFormatReadingAndDelta：vix/move/usdjpy 一位小数（161.66→"161.7" 正确舍入）、hy_oas "612bp"、sofr/t10y2y 带符号 "+28bp"/"-10bp"/"+35bp"、nfci "%+.2f" "-0.52"；formatDelta 同规则；formatPct5y="98%"。全对 | PASS |
| functional[2] | trendArrow：\|Δ\| < 显示精度一单位(vix0.1/nfci0.01/bp1)→→，否则 ↗/↘ | 三 eps 值经 < 用例 exercise（vix0.05→→、sofr0.9→→、nfci-0.009→→）+ 远离值 ↗/↘。**但恰好 ==eps 的边界方向无断言**：无 trendArrow(vix,0.1)/(vix,-0.1) 等用例。变异 `>=`→`>` 测试静默全过，证明「== eps 归 ↗/↘」这一设计判别边界未锁 | **FAIL** |
| functional[3] | sparkline 21观测分7桶均值→min-max八阶归一，单调升序首▁末█ | TestSparkline：21 升序 0..20→len7、s[0]='▁'、s[6]='█'；另补降序用例 exercise v<lo 分支（s[0]='█'、s[6]='▁'） | PASS |
| boundary[0] | 全平→全▄；≤7逐点；空窗口→空串 | TestSparkline：全平 21 点→"▄▄▄▄▄▄▄"；win[:3]→len3；sparkline(nil)→"" | PASS |
| boundary[1] | showPct5y sofr=false/usdjpy=false/vix=true（决策4） | TestFormatReadingAndDelta：assert.False(sofr)/False(usdjpy)/True(vix) | PASS |
| non_functional[0] (review) | 全无状态纯函数（无IO无包级可变态） | 13 函数均纯（fmt.Sprintf 非 IO，无 Print/文件/网络）；唯一包级 var sparkGlyphs 是只读字形表从不写（Leader 已认可） | PASS |
| non_functional[1] (test) | build ./... + test 全绿 | 亲跑通过，coverage 92.5% | PASS |

## Leader 五点核查回复
1. functional[0][1] 字面值：**逐条对照设计 §6.1–6.3 全部正确**（emoji/STALE/STRESS/正负号/98% 无一抄错）。PASS。
2. sparkline 语义：我用自构造升/降序数据独立跑通（含 v<lo 降序分支）。PASS。
3. **trendArrow 边界：判 FAIL**——恰好 ==eps 时应为 ↗/↘（`>=`），但无用例锁定，变异 `>`→静默通过。这是本次唯一 FAIL。
4. 纯函数 review：确认无 IO/无可变包级态，sparkGlyphs 只读可接受。PASS。
5. modernize：提交为 2 个纯新增文件，无既有代码改动，不存在为消 lint 顺手改。PASS。

## detect_changes（Leader 代跑）
low、affected_processes 空（新符号预期）、diff 仅两新文件、无越界。作影响面证据。

## 拒绝原因（reject_reason）
functional[2] trendArrow 的判别边界（|Δ| 恰等于 eps 时归 ↗/↘ 而非 →）无测试锁定：变异 `delta >= eps`→`delta > eps` 后 TestTrendArrow 静默全过。设计明确「< eps → →，否则 ↗/↘」，边界属规格一部分。其余 7 条全 PASS。

## 建议修复方向（小改，仅测试，约 3–4 行）
在 TestTrendArrow 追加恰好边界用例（正负各一，覆盖 >= 与 <= -eps 两侧）：
```go
assert.Equal(t, "↗", trendArrow(IndVIX, 0.1))   // 恰等 eps → ↗（非 →）
assert.Equal(t, "↘", trendArrow(IndVIX, -0.1))  // 恰等 -eps → ↘
assert.Equal(t, "↗", trendArrow(IndT10Y2Y, 1))  // bp 类恰等 1bp → ↗
```

---

## 二次验证（增量，2026-07-15，epoch=2，rework=1，提交 83bf0d4）

**判定：VERIFIED（PASS）**

上轮 7 条 PASS 沿用（生产代码未动）；本轮复核 functional[2] 边界修复。

| 复核点 | 证据 | 判定 |
|---|---|---|
| functional[2] ==eps 边界方向 | 83bf0d4 加 3 断言：trendArrow(IndVIX,0.1)=↗、(IndVIX,-0.1)=↘、(IndT10Y2Y,1)=↗，覆盖 >= 与 <= -eps 两侧。亲跑 TestTrendArrow PASS。**独立变异确认**：`delta >= eps`→`> eps` 且 `<= -eps`→`< -eps` 后 TestTrendArrow FAIL（↗ 与 ↘ 两侧断言均报 Not equal），边界已锁 | PASS |
| diff 范围 | `git show --stat 83bf0d4`：仅 notify_format_test.go +6 行，无生产代码、无越界。变异后 notify_format.go 已还原干净 | PASS |

**结论**：functional[2] 边界修复到位，独立变异确认断言有效。8 条 DoD 全 PASS。TASK-003 verified（wave1 收尾）。
