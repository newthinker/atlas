# TASK-005 验证报告 — cmd 接线与 20 家配置清单

- **验证者**: test-agent-4 (Reality Checker)
- **任务状态**: verified(epoch=1)
- **commit**: 9f704bc
- **判定**: **PASS** — 接线闭合编译窗口;20 家 CIK live 核验 0 不符;AD-8 文件级补偿达标;smoke 数据内部一致

## Done Criteria 覆盖矩阵

| # | 维度 | 完成标准 | 证据 | 判定 |
|---|---|---|---|---|
| 1 | functional | edgar.New(EdgarUserAgent) 传 Refresh 第 6 参;go build ./... 通过 | diff 接线 + 亲跑 **go build ./... BUILD-OK**(闭合 TASK-004 编译窗口) | PASS |
| 2 | functional | go test ./cmd/atlas/ 全 PASS | 亲跑 ok(9 通知用例补参 + TestHasEdgarInstrument) | PASS |
| 3 | boundary | UA 空且有 edgar 标的 → 报错含 prism.edgar_user_agent | hasEdgarInstrument 单测(true/false/nil);守卫 prism.go:105-106 审读正确 | PASS |
| 4 | non_functional (review) | 20 家 CIK 与官方一致;TSM IFRS 不入池 | live curl company_tickers.json(797926B) 逐个比对 **20/20 一致(0 不符)**;TSM 仅注释无条目 | PASS |
| 5 | non_functional (manual) | smoke NVDA → fundamental_q≥40、valuation 约 10Y | discovery 数据交叉核对(见下),真实 EDGAR 重打留验收 | PASS(数据一致) |

## AD-8 文件级覆盖补偿复核(Leader 要求)

整包 cmd/atlas 74.6%(CLI 入口天然 0% 拖累,历史基线),按 AD-8 做文件级补偿——亲跑 coverprofile:
- **hasEdgarInstrument 100.0%** ✓
- **runPrismRefreshWith 100.0%** ✓
- runPrismRefresh **0.0%**(集成入口,含 boundary[0] UA 守卫)——属 AD-8 认可的 CLI 入口惯例,不作 reject 依据;与 prism_test.go:20「shell 守卫→review」一致。

## smoke 数据完整性/内部一致性交叉核对(Leader 要求,不重打 EDGAR)

discovery 记录 NVDA live smoke:fundamental_q=71 行、Q1=17/Q2=18/Q3=19/Q4=17、eps 71/71、revenue 70/71、net_income 71/71、equity 68/71、diluted_shares 54/71。内部一致性:
- **Q1+Q2+Q3+Q4 = 17+18+19+17 = 71** = fundamental_q 总数 ✓
- **diluted_shares 54 = 71 − 17(Q4)** 精确吻合「Q4 股本按设计 NaN」(TASK-003 SharesNoQ4 设计),强一致性 ✓
- revenue 70/71 非整列 NULL ✓(BUG-2 已修);首跑(TASK-003 未修)仅 16 行全 Q1/revenue 全 NULL 正是 live 校验点暴露的 quarterization bug,经 6de11e0 修复后重跑 71 季。
- fundamental_q=71 ≥ 40 达标 ✓。
数据完整、内部自洽,与 TASK-003 修复链闭合。

## CIK live 核验(20/20)

curl 官方 company_tickers.json 逐个比对全一致。**XOM=2115436 特别核实**:初看疑似错误(印象中 ExxonMobil 旧 CIK 34088),官方源确为 2115436(ExxonMobil Holdings Corp)——dev discovery 决策正确,幸拉权威源核验未凭陈旧记忆误 reject。

## 越界/范围

commit 9f704bc 动 cmd/atlas/prism.go、prism_test.go、configs/config.example.yaml。config.example.yaml 虽在 ./cmd/atlas 之外,为 non_functional[0] DoD 明确要求产物,属任务内在范围,非漂移。无其他越界。

## 已知限制(不作 reject 依据)

拆股重述污染 EPS 派生 Q4(仅每股值;PE/EPS_TTM 跨拆股日不准,Revenue/NI/PB/PS 不受影响)。**TASK-008 已立项修复**,承 TASK-003 discovery。

## 结论

接线闭合编译窗口;cmd 测试全绿;20 家 CIK live 0 不符;AD-8 文件级补偿(hasEdgarInstrument/runPrismRefreshWith 均 100%)达标;smoke 数据完整且内部自洽(Q 分布求和=71、shares 54=71−17)。**VERIFIED**。
