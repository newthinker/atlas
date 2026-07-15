# 设计规格 — 通知 v1.1（sprint-021）

> 完整规格见 `docs/plans/2026-07-15-crisis-notify-v1.1-design.md`（R1–R6）与
> `docs/plans/2026-07-15-crisis-notify-v1.1-impl.md`（逐任务代码）。本文件仅摘录跨任务契约。

## 跨任务契约

- **唯一新符号**：`staleDowngradeWarning(nc NotifyContext) string`（包内私有，Task 1 定义、仅 renderTransition 消费）。
- **零导出签名变更**：`Messages`/`FormatIntradayAlert`/`NotifyContext`/`Trend` 不动。
- **文案字面值即契约**：Task 3 的全家族测试断言 Task 1/2 的最终文案——按 Task 1→2→3 串行执行。
- **页脚归属判定基准变更**（Task 3）：由"非交易信号"子串改为 `strings.HasSuffix(m, notifyFooter)`；
  盘中速报的"成因未核实，非交易信号"是内联限定语、非页脚。

## 关键条件与判定

- ✅/🔽：`splitZones(res)` 的 abnormal 区空/非空（⚪ 不计入异常区）。
- 警示行触发（R1a）：降级路径 ∧ NewStale 非空 ∧ `severity(PrevDay[ind].Status) >= severity(StatusAmber)`；
  多指标 AllIndicators 序、颜色列表同序。
- P2 条件行（R1b）：同一 severity 判定；PrevDay 缺行不追加。
- R4：`!isColor(prev) && !isColor(cur)` 才用 nonColorNote 文案；混合迁移维持 colorWord。
- 非色彩常量：`StatusStale`/`StatusSuppressed`/`StatusNoData`（types.go:13-15，已核实）。
