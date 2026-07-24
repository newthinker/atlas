# Sprint 026 (Prism M2) 最终 Code Review

- 审查者: qa-agent-4 (Reality Checker + Adversarial)
- 范围: master..feature/prism-m2 (原 10 commit + iteration-2 修复 2 commit: 1335169, fdb5664)
- 方法: 第一轮常规审查 + 第二轮 codex 跨模型复核 + iteration-2 定向修复复核

## 最终 Verdict: PASS(维持,可进入最终验收)

- CRITICAL(阻塞): 0
- WARNING 未解决: 0(W1/W2 已修复验证;W3 刻意设计维持不修,已记录)
- INFO: 2(XOM live 核 / 多 unit tag 非确定,均非阻塞)

---

## Iteration 2 定向修复复核(2026-07-24)

首轮 W1/W2 已由 Leader 修复、test-agent-4 复验 verified。qa-agent-4 定向复核(读 diff + 亲自重跑相关测试)结论:**两项修复真实有效,准予收 verdict**。

### W1 已修复 ✅ [commit fdb5664 client.go]
- **修复**: events 改为按生效日聚簇而非按 ratio 合并。`collectEffObs` 收集每比例的生效日观测(源改为**年度 EPS + 股本数**,去掉季度 EPS——消除季度低值舍入造出的同比例假观测,附带修正首轮 refineEffDates 扫季度 EPS 的精度隐患);`clusterEvents` 按 `splitClusterGap=365天` 间距把观测分簇为独立事件,同簇取最早生效日。
- **复核**: 手工 trace double_split fixture(年度 EPS 2020→2021→2024 各现一次 2:1 跳变,filed 相隔 >1 年)→ obsByRatio[2]=[2021-02,2024-02] → clusterEvents 分为两独立事件 → pre-both 季度 factor=4 → EPS 8.0÷4=2.0、股本 1000×4=4000。逻辑正确。
- **测试**: 亲自重跑 `go test ./internal/collector/edgar/ -count=1` → **12 PASS**,含新 `TestFetchCompanyFactsRepeatedSameRatioSplit`(断言 ÷4)及既有 `TestFetchCompanyFactsSplitNormalization`(NVDA 4:1+10:1 无回归)。
- clusterEvents 无 `dates[0]` 空切片 panic 风险(obsByRatio 键仅由 append 产生,恒非空)。

### W2 已修复 ✅ [commit 1335169 refresh.go]
- **修复**: ttmPoints 增 `maxTTMWindowDays=330` 守卫,首尾 period_end 跨度 >330 天(整季缺失→窗口跨缺口)则不产点。
- **阈值复核**: 正常连续 4 季跨度 ~273-287 天(含 53 周财年边际),单季缺口跨度 ~365 天;330 天两侧余量充分(上距正常 +43 天、下距缺口 -35 天),分隔正确。dev 实测把 Leader fix_item 初拟的 370-400 修正为 330(370 会漏掉 365 天的缺口窗口),test-4 独立复算确认——**修正合理,若用 370 反而失效**。
- **测试**: 亲自重跑 `go test ./internal/prism/ -count=1` → **28 PASS**,含新 `TestTTMPointsQuarterGap`(12 季删中间 1 季→8 候选窗口剔 3 跨缺口→5 点,窗口数学已复核)与 `TestRefreshEdgarTTMGap`。

### 全量回归
亲自重跑 edgar/prism/valuation/storage/cmd/api 全部包 `-count=1` → **全 PASS**,W1/W2 修复无回归。

### 新增已知限制(确认收录)
**两次真实同比例拆股相隔 <365 天会被 clusterEvents 误合并为单事件**(test-4 提出)。极罕见(同一公司一年内两次同比例拆股几乎无先例),交付批次不涉及;记入 final-report 已知限制。与既有三条限制(白名单原子比例/需≥2财年确认/Q4 EPS FY−ΣQ 近似)并列。

### W3 维持不修(确认)
派生值「最新基准值+原始 filing date」口径属拆股归一化刻意设计(smoke 实证跨拆股日 PE 连续、无 alpha 泄漏),discovery 已记录。可选加固(ttmPoints 生效日取 4 季 max)非必需。

---

## 首轮审查结论(存档)

第一轮常规审查:全量 `go build ./... && go test ./... -count=1` 全 PASS;EDGAR UA 合规、错误处理与 engine fallback、NaN 语义贯穿、单线程无并发隐患、防前视核心(FilingDate→effectiveDate)、compare 前端 JS 目检、仓库惯例一致性均通过。

第二轮 codex 跨模型复核(codex-cli 0.139.0, read-only):对 client.go/refresh.go/reconstruct.go 独立复核,原报 3 项 WARNING 中 W1/W2 现已修复,W3 为刻意设计。

INFO 保留:
- XOM cik=2115436 为 2024 重组后新实体,companyfacts 历史深度可能不足 10Y(不足会 fallback engine),建议 live 验收核序列起点。
- client.go unitsOf 取首个 unit,多 unit tag 时非确定(实际 EPS/Revenue/Shares 均单 unit,不触发)。

---

## 结论

iteration-2 两项修复(W1 同比例重复拆股聚簇、W2 TTM 季度缺口守卫)经定向复核真实有效、测试通过、无回归,并附带一处精度改进(eff 源去季度 EPS)。W3 刻意设计维持。新增一条已知限制(同比例拆股 <365 天误合并)收录。交付质量满足最终验收标准。

**Verdict: PASS**(可启动 accepted + final-report + 归档)
