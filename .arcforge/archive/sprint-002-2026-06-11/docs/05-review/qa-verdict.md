# QA 终审裁决（Verdict）— ATLAS sprint-002

## VERDICT: PASS（无 high-severity；三视角共识）
> 附 1 条 WARNING 提请 Leader 裁决（CARRYOVER I3 可达性升级），不阻塞 sprint-002 已交付功能的正确性与降级稳健性。

- 审查者: qa-agent-1（Reality Checker）
- 范围: 80c43df..HEAD / 12 commit / 34 files / +2284 行
- 门禁: `go vet` clean · `go test ./...` 全绿 · `-race`(app/collector/valuation/strategy) 无竞态
  （crypto 集成测试 FAIL = sandbox 无外网 EOF，非本 sprint）
- 工具限制: 本 QA 会话未提供 gitnexus MCP 工具，已用 vet+test+race+人工 diff 复核替代并记录。

## 分级问题清单

### CRITICAL — 无

### WARNING
- **W1 / C2** meta_arbitrator 仲裁合成信号 `Price=0`，CARRYOVER I3 由 sprint-001「不可达」升级为本 sprint「条件可达」。
  - 文件:行号 `internal/app/app.go:504-511`（合成处）；触发条件来自 `cmd/atlas/serve.go:163-168` 注册双策略 + `configs/config.example.yaml:161` `^GSPC` 同绑 price_percentile+pe_percentile + `app.go:472-511` arbitrate(len>=2)。
  - 影响: meta.arbitrator 启用且 executor 接线时，Price=0 信号可下单（资金安全 latent，沿用 sprint-001 W1 模式；ISSUE-3 修复后市价单 Price=0 反而可能被接受）。
  - 建议修复: 仲裁结果信号补价（取冲突信号参考价/末根收盘，参考 784ed71 模式）；或交付前确认默认未同时接线 executor+arbitrator 以暂避，并在 final-report 标注 I3 处置。

### SUGGESTION
- **S1** `valuationSrc/epsSrc` 在并行 worker(buildFundamental) 无锁读取，与 `executor`(a.mu 保护) 模式不一致；当前 set-once@assembly(serve.go:138, Start 前)故 `-race` 无竞态。建议注释固化"必须 Start 前注入"不变量或纳入 a.mu。
  - `internal/app/app.go:65-66,155-157,734+`

### INFO
- **I-a** cvpos×100 与重建 PercentileRank 口径(≤ vs <)需真实 lixinger API 核对；`usHKIndexCodes`(SPX/COMP/DJI/HSI) 待 LIXINGER_API_KEY 核对固化（代码已注明首日核对）。`lixinger/valuation.go:17-22,109`
- **I-b** 退化/全同值价格序列 → PercentileRank=0 → price_percentile StrongBuy@~0.95（strictly-less 设计口径 + 252 门槛，现实罕见，by-design）。`valuation/percentile.go:13`
- **I-c** `DetectType` 以原始大小写 symbol 调 `IsAShareIndex`，小写后缀(.sh)会漏判指数（符号惯例大写，trivial）。`app.go:610`
- **I-d** pe_percentile 在 OHLCV 为空时 Price=0，但 analyzeSymbol 生产路径保证 n>0（不可达，记录备查）。`pe_percentile/strategy.go:80-83`

## 验收对照（plan §1.2/§6）逐条
1. ^GSPC/GC=F 加 watchlist 出 price_percentile 信号 — 装配/路由/策略链齐备（PASS，手工 serve 终验建议补）。
2. 000300.SH 走 eastmoney 指数 secid + pe_percentile 走理杏仁 cn/index — PASS（parseSymbol 表命中 + endpointFor cn/index）。
3. 美股个股 pe 主路径/兜底/双失败三态 — PASS（buildFundamental + app_test TestBuildPEPercentile_Paths）。
4. 亏损股不出 PE 信号且不兜底（stub 未被调用）— PASS（ErrNonPositiveEPS 直接 return）。
5. GC=F 绑 pe_percentile 启动 warning+跳过不崩溃 — PASS（effectiveStrategies 过滤+warnOnce）。
6. 全量 go test 通过无回归 — PASS。

## 处置建议
PASS 放行 sprint-002 功能交付。请 Leader 就 **W1(I3 可达性)** 做二选一裁决：
(a) 本 sprint 补仲裁信号价（小改 app.go:504-511）后复验；或
(b) 显式延期至 I3，并在 final-report 注明"默认 serve 未同时接线 executor+arbitrator"的规避前提。
