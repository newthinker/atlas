# 需求分析 — 港股 qlib 数据包扩展（atlas_hk）

## 来源
- 计划 `docs/superpowers/plans/2026-06-13-hk-qlib-bundle.md`
- 设计 `docs/superpowers/specs/2026-06-13-hk-qlib-bundle-design.md`
- 数据可用性已用 yahoo live 实测验证。

## 目标
把港股股票/ETF/指数纳入 config watchlist，经 yahoo 采集 OHLCV，构建独立于 atlas_cn 的
qlib 数据包 atlas_hk，并产出港股 watchlist 分析报告。

## 复杂度
中等。跨 cmd/atlas（Go）、scripts/qlib_eval（Python）、configs、Makefile，但每处改动小。
核心是 export-ohlcv 的 market 参数化 + Go/Python 双侧 HK 命名契约对称。

## 关键技术要点
1. **独立 atlas_hk 包**：港股交易日历与 A股不同，混入 atlas_cn 会污染日历 → 单独 dump。
2. **export-ohlcv market 参数化**：`--market cn|hk`，按 market 取基准（cn=000300.SH/hk=^HSI）
   与 watchlist 子集；A股路径零回归（默认 cn）。
3. **HK 命名契约**（Go+Python 对称）：.HK→HK#####（5 位补零）、^HSI→HSI、^HSCE→HSCEI。
4. **行情走 yahoo**：.HK + ^HSI/^HSCE 路由已就绪（SelectForSymbol），无需改 selector。
5. **^HSTECH 不纳入**：yahoo 404，由 3033.HK 恒生科技ETF 代理。

## watchlist 新增
- 基金（场内 ETF，type 基金）：2800.HK 盈富、2828.HK 国企、3033.HK 恒生科技ETF、3181.HK AI。
- 指数（type 指数）：^HSI 恒生、^HSCE 国企（HSCEI）。
- 均仅 price_percentile（ETF/指数无基本面 PE）。

## 依赖关系
Go 关键路径串行（同文件）：TASK-001→003→007。Python/config/Makefile 独立。

## 边界
港股无场外基金 NAV（基金=场内 ETF/REIT）；港股 PE 离线回放不在本期；不引入 atlas 内调度。
