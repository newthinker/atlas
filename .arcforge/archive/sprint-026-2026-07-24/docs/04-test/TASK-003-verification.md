# TASK-003 复验报告 (rework#1) — EDGAR companyfacts 客户端

- **验证者**: test-agent-4 (Reality Checker)
- **任务状态**: verifying(epoch=1, rework 1/3) → **VERIFIED**
- **commit**: 6de11e0（含架构级根因修复:period_end 建键 + Q4 区间匹配）
- **判定**: **PASS** — 5 条 fix_items + 原 8 条 DoD 在新架构下全部成立,3 新 fixture 真测分支

## 架构变更核查(按「新实现」严查)

新架构:quarters 改按实际 period_end 建键(EDGAR fy/fp 是报告上下文非实际期间);单季判定 isSingleQuarter(fp∈Q1/Q2/Q3 且 70~100 天)在 (fy,fp) 去重「之前」先过滤;Q4 按年度区间 (start,end] 匹配恰好 3 单季推导;股本只落单季不做 Q4 减法。全 9 测试重跑通过,原 DoD 数值断言一字未改仍成立。

## fix_items 逐条(5 条)

| fix_item | fixture/证据 | 判定 |
|---|---|---|
| BUG-1 累计期塌缩:duration 过滤先于去重 | mini.json 补 Q2 累计 6mo(181天,1.3)+单季(90天,0.7)、Q3 累计 9mo(2.1)+单季(0.8),累计条目排在前。isSingleQuarter 先剔累计→只留单季。TestFetchCompanyFactsQuarterization 断言 Q2=0.7(非1.3)、Q3=0.8(非2.1);若未修则 Q2/Q3 丢失 facts≠4 | PASS |
| BUG-2 Revenue tag 选择过弱:firstQuarterlyTag 选首个有可用单季的 tag | rev_fpfy.json:RevenueFromContract 两条均 fp=FY(99999+170000 无单季),Revenues 有 fp=Q1 单季。TestFetchCompanyFactsRevenueTagSkipsUnusable 断言 Revenue=40000(Revenues)非 99999 | PASS |
| BUG-3 fp=FY 90天条目语义 | isSingleQuarter 要求 fp∈Q1/Q2/Q3,fp=FY 90天(rev_fpfy 的 99999)天然被排除、不误当单季;与 BUG-2 交织处一并处理 | PASS |
| fixture 防回归 | 新增 q4guard/rev_fpfy/shares_noq4 三 fixture + mini.json 双条目改造,各有对应断言 | PASS |
| 既有测试回归 | 全 9 测试重跑全 PASS | PASS |

## 原 8 条 DoD 在新架构下复核

| DoD | 证据 | 判定 |
|---|---|---|
| functional[0] CIK 零填充+UA | TestFetchCompanyFactsRequest PASS | PASS |
| functional[1] 季度/Q4/filed/instant | TestFetchCompanyFactsQuarterization:len4、Q4 period_end=2026-01-25、FilingDate=10-K filed、Equity instant 对齐 95000 | PASS |
| functional[2] 修正重报去重 | mini.json Q1 双条目(0.5 早/0.6 晚)仍在,断言 EPS=0.6 | PASS |
| Q4=FY−ΣQ 数值 | InDelta(3.0−(0.6+0.7+0.8))、Revenue 170000−(40000+42000+44000)、NetIncome 74000−(15000+17000+19000) 全精确 | PASS |
| period_end 升序 | facts[0..3]=Q1<Q2<Q3<Q4,sort.Slice by PeriodEnd | PASS |
| boundary[0] 某季科目缺失 NaN | Q2 shares 缺失 → IsNaN(q2.DilutedShares) | PASS |
| boundary[1] Revenue tag 回退 | TestFetchCompanyFactsRevenueFallback(rev_fallback.json 缺首选 tag)=40000 | PASS |
| error IFRS/HTTP | TestFetchCompanyFactsIFRS(ErrNotUSGAAP)、HTTPError(403+CIK) | PASS |
| 加强项 PartialFYNoQ4 | qCount≠3 不产 Q4 | PASS |
| 新增 SharesNoQ4 | shares_noq4.json:Q4 EPS 正常推导、DilutedShares NaN(非 100−300=−200) | PASS |
| 新增 Q4Guard | q4guard.json:三单季 period_end 在 2022、年度区间外 → 不推 Q4,len3 | PASS |

## 3 新 fixture 真测分支复核(防假断言)

- rev_fpfy:RevenueFromContract 全 fp=FY(真无可用单季)→ 回退分支真被走到,断言 40000 非 99999。
- shares_noq4:shares 有完整 Q1/Q2/Q3 单季+FY,断言 Q4 shares NaN → 证明「股本不做 FY−ΣQ 减法」,若做减法会得 100−300=−200。
- q4guard:三单季实际 period_end 在 2022(与年度 2025~2026 区间不交),n=0≠3 → 拒推 Q4,证明按实际期间匹配而非 fy/fp。

## 覆盖率/越界

- 门禁口径覆盖率 **90.8%** ≥ 80(与 dev 报告一致)。
- 越界终核(git show --name-only):6 文件全在 internal/collector/edgar/ 内,无越界。

## 已知限制(不作 reject 依据,报告注明)

拆股重述会污染 EPS 派生 Q4(历史 EPS 按当前股本重述,而单季申报值为当时口径 → Q4=FY−ΣQ 近似在拆股年有偏差)。discovery 已记录,Leader 正请用户决策 follow-up。本任务 fixture 无拆股场景,不影响当前判定。

## 结论

架构级根因修复(period_end 建键 + duration 先过滤 + Q4 区间匹配)语义正确;5 条 fix_items 与原 8 条 DoD 全部成立;3 新 fixture 真测声称分支非空洞;覆盖率 90.8%;无越界。**VERIFIED**。
