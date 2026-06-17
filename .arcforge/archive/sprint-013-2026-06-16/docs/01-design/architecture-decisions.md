# 架构决策 — 第三期 分位全历史回看

## AD-1: 用大窗口哨兵而非分发哨兵实现 inception
`lookback_years:0` → 策略声明 `PriceHistory = SinceInceptionBars(100*252)`，复用既有 `historyWindowDays` 折算 + `FetchHistory` 数据裁剪。**理由**：零侵入——app/collector 无需新增「全史」分支判断，逐标的上市日由数据源天然裁剪。避免引入需在多处分发的特殊哨兵值。

## AD-2: PE lookback 常量→可配字段（默认 5 零回归）
`const valuationLookbackYears=5` → App 字段 `valuationLookback`（New 默认 5）+ `SetValuationLookback`。config 新增 `valuation.lookback_years`。**理由**：解耦硬编码（代码注释本就预告「later phases may push it down to a parameter」）；默认 5 保证未配置时行为与现状完全一致。

## AD-3: inception 下 EPS 取早 floor，PE 窗口由 ohlcv 决定
EPS fetch（app.go:809）inception 时 start=end.AddDate(-100,0,0)。`ReconstructPEPercentile(ohlcv, eps)` 的有效 PE 分位窗口 = ohlcv 窗口（已由策略 PriceHistory 在 inception 下覆盖全史）；EPS 多取仅用于阶梯对齐，无害。**理由**：单一窗口真相源（ohlcv），EPS 只需够早。

## AD-4: lixinger inception → y10（诚实上限）
lixinger cvpos 只有 y3/y5/y10 三档（valuation.go:51-59）。`lixingerLookback()`：0→10，否则透传。**理由**：外部 API 无全史档；inception 对 A 股个股/指数等价「最多 10 年」，文档与 Reason 诚实标注，不伪造。

## AD-5: Realistic Scope —— app 装配拆分
config（T4）/ app 字段（T5）/ serve 装配（T6）三 package 分拆。T5 的 App 字段+setter 不 import config（取 int），故 T5 独立于 T4；T6 汇聚两者。**理由**：单 owner 单 package，T4/T5 可与策略链并行。

## AD-6: 数据全史 dump 独立 best-effort
不改默认 `SIGNAL_FROM`（避免影响既有 signal-eval 数据包），新增 `WAREHOUSE_FROM=1970-01-01` 供仓库全史 dump。**理由**：数据生产依赖外部源属集成；主干（T1-T6）代码不依赖其完成即可交付验证。

## AD-7: 沿用前两期降级与环境
validator/arcforge-write 缺失→手工校验 + with-task-lock；GOTOOLCHAIN=local；venv python。
