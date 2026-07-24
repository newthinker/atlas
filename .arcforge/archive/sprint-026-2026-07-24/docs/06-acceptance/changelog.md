# Changelog — Sprint 026 (Prism M2)

feature/prism-m2 @ master(12 commits):

- e95f6bf feat(prism): EPSPoint.FilingDate with effective-date alignment (zero-value compatible)
- ce7d4b1 feat(prism): EDGAR companyfacts client with quarterization and Q4 derivation
- 3393887 feat(prism): fundamental_q table for EDGAR quarterly facts
- 7c133d2 feat(prism): compare page with multi-symbol PE overlay and cross-section table
- a35b837 feat(prism): edgar refresh path with filing-date PE, PB/PS series and engine fallback
- b2d0dd6 fix(prism): lock EdgarLookbackYears default and cover edgar PB/PS NaN boundary
- 9f704bc feat(prism): wire edgar client and first-batch 20 US companies config
- 6de11e0 fix(prism): correct EDGAR quarterization on real data (dedup order, revenue tag, period-based Q4)
- 91a6dbb docs(prism): sync design data-source matrix and record M2 acceptance
- a59d2af feat(prism): EDGAR per-share stock-split normalization
- 1335169 fix(prism): guard ttmPoints against quarter gaps
- fdb5664 fix(prism): key split events by effective date to support repeated same-ratio splits

范围变更:ETF 成分聚合(D3)+etfholdings 移至 M3(用户决策);拆股归一化(TASK-008)Sprint 中新增(用户批准)。
