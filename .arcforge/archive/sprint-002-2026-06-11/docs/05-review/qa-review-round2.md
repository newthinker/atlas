# QA 终审 · 第二轮（跨视角对抗式验证，纯 Claude 三视角）— ATLAS sprint-002

- 变更规模 Large(+2284 行) → 启用全部三视角：数据正确性 / 运维 / 资金安全。
- 心智：对抗式找隐含假设、降级行为、极端输入与资金安全 latent。

---

## 视角 A · 数据正确性

### A1 cvpos×100 与重建分位口径一致性 — INFO（已知，代码已标注首日核对）
- 理杏仁路径：`cvpos∈[0,1] ×100`（valuation.go:109）。重建路径：`PercentileRank` strictly-less 0-100。
- 两者量纲一致(0-100)；语义微差：cvpos 可能为"≤当前"占比，PercentileRank 为"严格<"占比。极端序列下可差 1 个样本权重，对 20/80 阈值带不构成误判。
- `usHKIndexCodes`(SPX/COMP/DJI/HSI) 与成功码/metricsList 键名均在代码注释标注「实现首日用真实 API 核对固化」，**无 key 环境无法终验** → 提请交付前补核对。

### A2 PE 重建日期对齐 / 时区 / 边界日 — PASS（低风险）
- `latestEPSAtOrBefore`(reconstruct.go:73-79) `sort.Search(Date.After(t))`，当日相等被包含（"at or before"语义正确）。
- EPS `time.Parse("2006-01-02")` 落 UTC 零点，bar.Time 为采集器实例时刻，比较的是绝对时刻——同一自然日不同时分至多影响边界单日的对齐点选择，不改变分位统计量级。可接受。

### A3 极端序列信号语义 — INFO（by-design）
- 全同值序列 → `PercentileRank=0` → price_percentile classify `p<extremeLow` → **StrongBuy@~0.95**。当前价并非"低"而是"不变"，信号语义偏误。
- 缓释：`minSampleBars=252` 门槛 + 真实资产 252 根全同收盘极罕见；strictly-less 是 design 显式口径并有测试固化。判定 by-design，运维需知悉退化输入下的假阳。

---

## 视角 B · 运维（现场可观测性与降级）

### B1 理杏仁未配 key — PASS
- serve.go:105 `APIKey==""` 则 lixingerCollector 保持 nil → `valuationSourceOrNil` 返回 untyped-nil 接口 → buildFundamental `valuationSrc==nil` 分支 warnOnce("valuation percentile unavailable: lixinger not configured")，PEPercentile=-1，pe_percentile 静默不出信号。降级路径完备且有日志。

### B2 Yahoo 反爬 403 — PASS
- `FetchEPSHistory` StatusCode!=200 → error → buildFundamental `err!=nil`（非 ErrNonPositiveEPS）→ 理杏仁兜底；兜底也失败 → warnOnce("primary and fallback failed", zap.Error)。Source 编码 `yahoo_eps_error` 可观测。

### B3 表外指数绑定（^N225 等） — PASS
- effectiveStrategies:657-663 对 ^ 前缀且不在 phase-1 表的符号 warnOnce("index symbol outside phase-1 list, market defaults to US")；MarketForSymbol 默认 US。不崩溃、有告警、单次去重。
- `GC=F` 绑定 pe_percentile：assetTypeOf(期货)=commodity ∉ pe_percentile.AssetTypes[stock,index] → effectiveStrategies warnOnce("strategy skipped: asset type not supported") 并过滤；若过滤后为空则 analyzeSymbol 提前 return。符合验收清单第 5 条。

### B4 新配置缺省组合 — PASS
- numParam(两策略各自) 容忍 viper int/float64；阈值缺省回落 New() 默认；Init 校验 `extremeLow<low<high<extremeHigh` 与 `lookbackYears>0`，非法返回 error，serve 经 registerConfiguredStrategy 记 warn 不注册（不 panic）。

### S1（SUGGESTION）valuationSrc/epsSrc 无锁读取
- `app.go:155-157` SetValuationSources 写、`734+` 并行 worker 读，均未走 `a.mu`；而 executor/arbitrator 走 RLock 快照。当前 set-once@assembly(serve.go:138, Start 之前) 故 `-race` 全过、无实际竞态。
- 风险：若未来在运行中调用 SetValuationSources 即产生 data race，与既有 executor 防护模式不一致。**建议**：注释固化"必须 Start 前注入"不变量，或一并纳入 a.mu 快照。非阻塞。

---

## 视角 C · 资金安全

### C1 price_percentile/pe_percentile 信号 Price 非 0 — PASS
- price_percentile:80 `Price=cur`(末根收盘，>0)。
- pe_percentile:80-83 `Price=ctx.OHLCV[n-1].Close`（n>0 时）。analyzeSymbol 在 `len(ohlcv)==0` 时提前 return(:368-374)，且 pe_percentile 声明 PriceHistory→必拉 OHLCV，生产路径 n>0，Price 必非 0。无 sprint-001 W1 同类（信号无价）复发。

### C2（WARNING）meta_arbitrator 仲裁信号 Price=0 —— CARRYOVER I3 被本 sprint 放大为「可达」
- `app.go:504-511` 仲裁合成信号未设 Price（仍 0）。
- **放大链条（本 sprint 新引入可达性）**：sprint-001 serve 仅注册 ma_crossover 单策略，`arbitrate` 的 `len(signals)>=2`(:477) 永不满足→I3 不可达。本 sprint：(a) serve.go:163-168 注册 price_percentile + pe_percentile；(b) config.example.yaml:161 将 `^GSPC` 同时绑定二者。当两策略对同一 symbol 同时出信号→len==2→进入仲裁→合成信号 Price=0。
- **资金影响（条件触发）**：需 meta.arbitrator 启用 + executor 已接线。届时该 Price=0 信号经 router→executor 下单，复现 sprint-001 W1「fail-safe 不出错单 / 误导价」一类。ISSUE-3 曾修 ExecutionManager 市价单不带 Price 的 paper 拒单——意味 Price=0 现可能被当市价单"接受"，反而更需关注。
- **修复建议**：仲裁结果信号补价（参考 issues.md 所述 784ed71 模式，取冲突信号集的参考价或末根收盘）。或在交付前确认 serve 默认未接线 executor+arbitrator 组合以暂时规避。
- 级别 WARNING（条件可达、非本 sprint 新代码、但本 sprint 配置使其首次可达）。

---

## 三视角共识
- 无 CRITICAL；sprint-002 新增代码（采集/分位/策略/装配）正确性与降级路径稳健。
- 唯一高关注项 W1/C2 为 CARRYOVER I3 的可达性升级，三视角一致认为应由 Leader 裁决本 sprint 修复或随 I3 显式延期并在 final-report 标注；无 reviewer 分歧 → 不构成 CONTESTED。
