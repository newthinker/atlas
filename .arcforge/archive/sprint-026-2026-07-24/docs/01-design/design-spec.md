# Prism M2 设计规格(摘要)

> 完整规格(含逐步 TDD 代码)见 `docs/superpowers/plans/2026-07-24-prism-m2.md`,本文件只记模块边界与接口契约,供任务并行开发时对齐。

## 模块与接口

```
internal/core            EPSPoint{Date, EPS, FilingDate}  ← Task 1 产出,Task 4 消费
internal/valuation       effectiveDate() helper;ReconstructPESeries 签名不变
internal/storage/prism   fundamental_q 表 + FundamentalRow + UpsertFundamentals/QuarterlyFundamentals ← Task 2 产出,Task 4 消费
internal/collector/edgar Client + QuarterlyFact + FetchCompanyFacts(cik) ← Task 3 产出,Task 4 消费
internal/prism           EdgarClient 接口 + refreshEdgar + Refresh 第 6 参数
internal/config          PrismInstrument.CIK;PrismConfig.EdgarUserAgent/EdgarLookbackYears(默认 10)
cmd/atlas                edgar client 构造注入
internal/api             /prism/compare 路由 + PrismCompare handler + prism_compare.html(零新 API)
```

## 关键数据流(refreshEdgar)

1. FetchCompanyFacts(CIK) → 全量落 fundamental_q(source="edgar")
2. EPS_TTM 4 季滑窗(4 季齐且非 NaN 才产点)→ EPSPoint{Date: PeriodEnd, FilingDate}
3. BVPS(Equity/DilutedShares,单季即点)与 RPS_TTM 同法 → 复用 ReconstructPESeries 对齐(对齐失败 → 整列 NaN,不影响 PE)
4. closes 走 yahoo(EdgarLookbackYears 回看)
5. PE + RollingPercentile(5Y/10Y) + PB/PS 按日期并入 → upsert valuation_daily

## 解析规则要点(Task 3)

- URL `{base}/api/xbrl/companyfacts/CIK{零填充10位}.json`;UA 含邮箱。
- 单季判定:duration 70~100 天;年度 350~380 天且 fp=="FY";instant(Equity)按 end。
- Q4 推导:FY − (Q1+Q2+Q3),EPS 同式为近似(误差 <1%,注释标注);Q4 filed = 10-K filed。
- Revenue tag 优先级:RevenueFromContractWithCustomerExcludingAssessedTax > Revenues > SalesRevenueNet。
- 同 (fy,fp) 多条 → filed 最新;非 us-gaap → ErrNotUSGAAP。
