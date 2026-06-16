# 需求分析 — Qlib 数据仓库 第二期（Part B PIT 基本面）

> 来源: `docs/superpowers/plans/2026-06-15-qlib-data-warehouse-phase2.md`

## 目标
用本地 SQLite 仓库的 **PIT 双轴基本面**（报告期 report_period × 真实可知日 observe_date）为 atlas 提供**消除前视偏差**的 EPS(TTM) 历史，作为 PE 分位重建权威主源，Yahoo/lixinger 退兜底。

## 核心价值（为什么修了前视偏差）
现 Yahoo 路径用 `trailingDilutedEPS.asOfDate`（报告期末日）对齐，把某季收盘对齐到该季 EPS——但该 EPS 数周后才公布 = 前视偏差。Part B 的 `observe_date` 存真实可知日，Go 查询 `observe_date <= 窗口末` 截断，使「站在某日只见当日及之前已公布数据」。`ReconstructPEPercentile` 对每个收盘取「≤该日最近 EPS 点」阶梯对齐，升序喂入天然正确，修订（更晚 observe_date）自动接管。

## 功能模块（7 任务）
1. **基本面 CSV 摄取**（Python）：`fundamentals.py` 解析归一化契约 CSV → `FundRow`（数值空→None，symbol 大写）。
2. **writer 同次原子写**（Python）：`write(..., fundamentals=None)` 向后兼容，fundamentals 行同一临时库落 `fundamentals_pit`，修订原样保留不去重。
3. **build CLI**（Python）：`--fundamentals-dir`（可选）解析并随 OHLCV 同写。
4. **qlibpit EPS 源**（Go）：`FetchEPSHistory` 查 `observe_date<=end && eps_ttm IS NOT NULL` 升序 → `[]core.EPSPoint`，实现 `app.EPSSource`。
5. **兜底委托**（Go）：缺符号基本面→委托内层 EPSSource(yahoo)；fallback nil→空切片；仓库有数据优先（钉死测试）。
6. **serve 装配**（Go）：qlibpit 包装 yahoo EPS 源注入；需暴露第一期 wireQlibWarehouse 的 db 句柄复用。
7. **各市场适配器**（best-effort 文档 + Makefile）：ADAPTERS.md 契约 + US make 透传（`$(wildcard)` 守卫）。

## 归一化基本面 CSV 契约
`fundamentals_csv/<symbol>.csv` 表头固定：`symbol,report_period,observe_date,eps_ttm,pe,pb,ps,roe,dividend_yield`。必填 `symbol,report_period,observe_date,eps_ttm`；日期 `YYYY-MM-DD`（报告期用季末日）。

## 复杂度
整体**中等**。PIT 正确性（防前视 + 修订升序）是核心，已有计划测试钉死。风险点：T6 需适配第一期 wireQlibWarehouse 封装（db 句柄暴露），且保 `SetValuationSources` 在 `Start` 前的 QA S1 不变量。

## 范围边界（本期不做 / best-effort）
- 各市场 `fundamentals_csv/` 实际生产为 best-effort 适配器（T7 文档化），主干不依赖其精确性。
- 美股 observe_date 为披露滞后近似（优于现状但非精确备案日）。
- PB/PS/ROE 入库但 Go 侧本期仅消费 eps_ttm。
