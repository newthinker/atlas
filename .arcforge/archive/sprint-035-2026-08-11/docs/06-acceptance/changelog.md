# Changelog — Sprint 035 · Hestia M1b-3 validate

**基线** `125ad896fb096f7766cbb3c958ba2635a311c6ba` → **交付** `722aa2728723b573537160b602bb06a03b3169b4`
**包** `internal/hestia` · **规模** 10 文件 +2400/-7 · **覆盖率** 89.4% → **92.1%**

---

## 新增能力

### `Validate` — 七道校验闸门，产出 `Store.Save` 要求的 `ValidationReport`

```go
func Validate(ctx context.Context, obs Observation, h History, cfg Thresholds) (ValidationReport, error)
```

表驱动的 `gates` 集合，每道闸是 `func(gateInput) Check` 纯函数。**顺序即报告里 `Checks` 的顺序**，
与 M0 契约样本一致，让不同期次的报告可逐行对照：

| # | 闸门 ID | 判据 | 交付时的实际防护力 |
|---|---|---|---|
| 1 | `monetary_hierarchy` | M2 > M1 > M0 | ✅ 有信号 |
| 2 | `deposit_sum` | 绝对残差 ≤12% **且** 漂移 ≤3pct | ⚠️ 弱信号（余量仅 3pct；漂移待 M1c） |
| 3 | `corp_loan_reconcile` | 短期+中长期+票据 vs 企业合计 ≤5% | ✅ 有信号 |
| 4 | `stock_continuity` | 社融存量环比 ≤2% | ⏸ 恒 skipped 至 M1c |
| 5 | `yoy_sanity` | 同比绝对值 ≤50 | ✅ 有信号 |
| 6 | `completeness` | 必填集齐全（从模板表派生） | ⏸ 恒 passed 至 M1c |
| 7 | `magnitude_sanity` | 字段落在合理区间 | ⏸ 恒 skipped 至 M1c（区间表**有意为空**） |

**闸门判定失败不是 Go error** —— 它进 `report.Checks`。`error` 只用于基础设施故障
（配置非法、`History` 查库失败）。两者混用会让调用方分不清「这期数据没过闸」（正常路径）
与「数据库连不上」（该重试）。

### `Thresholds` — 七道闸门的全部可调参数 + 口径豁免

```go
func DefaultThresholds() Thresholds        // 经 M0 三期真实数据校准
func (t Thresholds) validate() error       // 配置自校验，Validate 最先调用
```

阈值取值均有实测依据（如 `DepositSumTolerance = 0.12` 而非方案初稿的 0.02 ——
M0 三期实测残差 7.65% / 8.57% / 9.06%，±2% 会让**每一期**都被拦下）。

**口径豁免**按 `(期次, 检查 ID)` **精确匹配**，命中记 `skipped{caliber_exemption:<version>}`
**而非 passed** —— 豁免与通过在数据上必须可分，把「这次没查」记成「查了没问题」等于伪造检查记录。

### `History` 窄接口 + `Store.Preceding`（只读）

```go
type History interface {
    Preceding(ctx context.Context, period, periodType string, n int) ([]Observation, error)
}
var NoHistory History   // 让「确实没有历史」可显式表达，Validate 据此拒绝 nil
```

定义在**消费方**而非 Store 侧：闸门只要「前 n 期」这一个能力，收成单方法接口后
单测可注入假历史，不必为测一个纯函数去建真库。一个方法支撑两道闸
（`stock_continuity` 用 n=1、`deposit_sum` 漂移用 n=6），`Validate` **一次取满 6 期**共用。

### `completeness` 必填集派生

```go
func requiredFields(extractor string) []string   // rule@v2=54, rule@v1=27, 其余 nil
```

从模板表（`tsfStockItems` / `tsfFlowItems`）派生，**不手写第五份字段清单**，
也**不用 `tsf_` 前缀判断** —— 前缀与板块归属当前恰好一致，但那是巧合，模板表才是事实。

---

## 行为变更

### `checkEnum` 解开硬编码的 `meta.` 前缀

前缀从函数体挪到两处调用点，**两处既有调用的错误信息输出逐字不变**
（背对背探针穷举 `Meta.validate()` 的 17 条错误路径，两棵树输出 diff 为空、sha256 相同，
并有阴性对照确认「diff 为空」非平凡为真）。

### 导出面扩大三项（均已在守卫中登记）

`DefaultThresholds` / `Store.Preceding` / `Validate`。
`store_test.go` 的两条导出面守卫（AST 版 + reflect 版）保持**精确集合相等**断言，
逐项登记并各补一行说明为何扩大 —— **不是放松断言**。

---

## 已知限制（详见 `internal/hestia/CONTRACTS.md` 的 Sprint 035 一节）

1. **交付时七道闸只有三道半真正拦得住东西** —— 后三道都依赖 M1c 的回填数据，
   会在同一时刻一起从沉默变成有声
2. **未过闸的期次落 pending ⇒ 不在 `v_hestia_current` ⇒ `Preceding` 看不见
   ⇒ 依赖历史的闸门无法自愈**（QA 发现的结构性根因）：
   `deposit_sum` 漂移在口径变更后永不恢复；`stock_continuity` 任意一期落 pending 后**级联失败**
3. **`StockContinuityMax` 单标量服务不了 monthly/h1/annual 三条序列** —— 重新标定解决不了
4. **加总闸对「同组内分项互换」零鉴别力**（加总是置换不变量）——
   而分项错位是抽取层点名的头号风险
5. **ULP 守卫不观察生产算术** —— 它钉的是「浮点乘法会产生误差」这个语言事实

---

## 交付后事项

1. 合并 master、更新 vault 文档状态
2. 把计划的三个实现决策回填上游 spec（`minDriftHistory=3` / `llm-fallback@v1` 返回 nil /
   `yoy_sanity` 用后缀筛）
3. 下一子迭代 **M1b-4**（discover + CLI），M1b 至此收尾
4. **M1c 填表时必须一并补**（提醒已挂在 `thresholds_test.go` 的 `assert.Empty` 绊线上）：
   遍历顺序守卫、`Min`/`Max` 两个边界方向、`Range.Unit` 单位、
   优先覆盖互为同组的分项字段、置换表补阴性对照行
