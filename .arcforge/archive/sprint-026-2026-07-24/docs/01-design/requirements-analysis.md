# Prism M2「美股深度」需求分析

> 需求源:`docs/superpowers/plans/2026-07-24-prism-m2.md`(已由 superpowers writing-plans 精炼,含逐任务 TDD 步骤)。ECC 不可用,按降级路径直接基于该计划做结构化分析。

## 核心功能(R 编号供追溯矩阵引用)

| ID | 需求 | 来源 |
|---|---|---|
| R1 | EDGAR companyfacts 10Y 主源:新增 `internal/collector/edgar` REST 客户端,季度化解析(单季判定/Q4 推导/修正重报取 filed 最新/Revenue tag 优先级) | 计划 Task 3 |
| R2 | filing_date 防前视:`core.EPSPoint` 增 `FilingDate`,valuation 生效日逻辑升级,零值回退 `Date`(全兼容) | 计划 Task 1 |
| R3 | 美股 PB/PS 补齐:prism store 增 `fundamental_q` 表;refreshEdgar 产 BVPS/RPS_TTM 阶梯并入 valuation_daily | 计划 Task 2/4 |
| R4 | `/prism/compare` 多标的对比页:复用既有 `/api/prism/series`、`Board()`,零新 API,≤8 标的 PE 叠加 + 横截面表 | 计划 Task 6 |
| R5 | refreshEdgar 路径:source=="edgar" 分派,失败且 FallbackSource=="engine" 时降级并记 Report.Degraded | 计划 Task 4 |
| R6 | cmd 接线 + 首批 20 家 US-GAAP filer 配置(CIK 经官方 company_tickers.json 校验) | 计划 Task 5 |
| R7 | 文档同步:设计文档数据源矩阵、deployment.md EDGAR UA 说明;M2 验收 7 条手动核对 | 计划 Task 7 |

## 非功能性需求 / 全局约束

- N1 EDGAR 礼貌要求:UA 必须含可联系邮箱(SEC 强制,缺失 403);≤10 req/s(每公司每日 1 请求,天然安全);无增量 API → 每日全量重拉 + 幂等 upsert。
- N2 AKShare 链路(A/H 公司、A 股指数兜底)**零改动**,所有新代码不得破坏其测试。
- N3 FilingDate 零值兼容是硬验收:`pe_percentile`/engine/qlibpit 路径零漂移。
- N4 sqlite 固定 v1.38.2、NaN↔NULL、testify 风格、Context Checkpoint 注释、`feat(prism):` commit 规范。
- N5 首批限定 US-GAAP filer;IFRS(20-F)返回明确错误 `ErrNotUSGAAP`,M3+ 扩展。

## 范围排除

- ETF 成分聚合(D3)与 etfholdings 采集器 → 后置 M3(用户决策,计划头部已记录)。
- yahoo 回退「+45 天滞后」→ 不采纳(架构决策 AD-1,见 architecture-decisions.md)。

## 模糊/缺失点

无阻塞性模糊点。计划已含完整接口契约、fixture、测试代码与实现骨架。两处「以现实为准」的开放点已在计划内给出处置路径:
1. Task 5 的 20 家 CIK 清单——以官方映射命令输出为准(显式指令,非 TBD)。
2. Task 3 live 校验点——个别公司 tag 缺失属预期,按 NaN 落库并汇总在 refresh 日志。
