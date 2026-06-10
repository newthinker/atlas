# ATLAS 项目状态评估与优化建议

**评审日期**: 2026-06-03
**评审版本**: commit `de64351`（master）
**分析方法**: GitNexus 知识图谱（185 文件 / 4289 符号 / 10642 边 / 144 执行流程）+ 源码核查
**最近更新**: 2026-06-10，基于 master `76a54b5` 逐条复核进展

---

## 〇、进展更新（2026-06-10）

P0 四项建议已在 commit `4c7861f`（PR #24，2026-06-07）全部落地，API 认证（原 P1-6）也已完成：

| 建议 | 状态 | 证据 |
|------|------|------|
| P0-1 LLM Meta-Strategy 接线 | ✅ 完成 | `app.go:279` 调用 `a.arbitrate()`，`serve.go` 按 `cfg.Meta.Arbitrator` 接线 |
| P0-2 per-symbol 策略 | ✅ 完成 | `app.go:262` 改用 `AnalyzeWithStrategies(item.Strategies)` |
| P0-3 智能采集器选择 | ✅ 完成 | 抽取共享 `internal/collector/selector.go`，核心路径用 `orderedCollectors` |
| P0-4 Web Signals/dashboard 接 SignalStore | ✅ 完成 | web 包覆盖率 0% → 35% |
| P1-6 API 认证 | ✅ 完成 | `middleware/auth.go` `APIKeyAuth` 接入 `server.go:182`，常数时间比较 |
| P1-5 ExecutionManager / FutuBroker | ❌ 未动 | `serve.go:199` 仍注释，`broker.go:109` 仍 TODO |
| P2-7 采集并行化 | ❌ 未动 | `runAnalysisCycle` 仍串行 |
| P2-8 LLM 异步化 | ❌ 未动 | **优先级上调，见第五节修订** |
| P2-9 外部采集器覆盖率 | ❌ 未动 | 实测数字与原文一致 |
| backtest CLI 接引擎 | ❌ 未动 | `backtest.go:62` TODO 仍在 |

> ⚠️ 优先级修订：P0-1 接线后，LLM 仲裁在分析循环中**同步执行**（每个冲突标的阻塞 2-10s），与串行采集叠加，watchlist 越大延迟线性累积。原 P2-7 + P2-8 已合并上调为 P1「分析循环并行化」。

---

## 一、项目概况

| 指标 | 数值 |
|------|------|
| Go 源文件 | 826（含测试），核心约 185 |
| 代码行数 | ~96,000 行 |
| 测试文件 | 358 个，**全部通过** |
| 编译 / go vet | ✅ 干净，无警告 |
| 提交数 | 162 |
| 知识图谱 | 4289 符号 / 10642 边 / 144 执行流程 |

ATLAS 是一个 Go 编写的**全球资产监控与交易信号生成平台**，采用 **Clean Architecture + Plugin（Registry）模式**，分层清晰：

```
cmd → app（编排）→ collector / strategy / router / notifier / llm / broker → core（领域模型）
```

整体工程质量高，接口抽象与插件化设计良好。

---

## 二、里程碑完成情况

代码与 `docs/plans/` 设计文档对照，已完成 4 个 Phase + 4 个 Milestone：

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 1-4 | 采集/策略/路由/通知/回测基础 | ✅ 完成 |
| M1 Production-ready | 配置、日志、Web UI | ✅ 完成 |
| M2 API improvements | REST API、HTMX 前端 | ✅ 完成 |
| M3 Observability | metrics、alert、job | ✅ 完成（覆盖率 98.5%） |
| M4 Live Trading | execution/risk/position 层 | ⚠️ **部分完成（未接线）** |
| Crypto Collector | Binance/OKX/CoinGecko | ✅ 刚完成（当前分支） |

---

## 三、核心缺口：已构建但**未接线**的功能

最值得关注的问题——几个完整实现并测试过的模块，没有接入实际运行路径。

> 2026-06-10 更新：本节 3.1 / 3.3 / 3.4 / 3.5 均已在 `4c7861f` 修复，仅 3.2（M4 实盘链路）仍未接通。

### 3.1 LLM Meta-Strategy 完全未集成 🔴 → ✅ 已修复

`internal/meta/`（Arbitrator 仲裁器 + Synthesizer 合成器）有完整实现和测试，但全局检索显示**没有任何 app/api/cmd 代码引用它**。

```
$ grep -rln "internal/meta" --include='*.go' .
internal/meta/synthesizer.go
internal/meta/arbitrator.go
internal/meta/*_test.go        # 仅包内自引用，无外部接线
```

README 主打的"AI 信号仲裁"特性实际未生效。

### 3.2 M4 实盘交易链路断开 🔴（仍未修复）

- `broker/execution.go`、`risk.go`、`position.go` 已实现且测试覆盖良好
- 但 `cmd/atlas/serve.go:179` 中 `execManager = broker.NewExecutionManager(...)` 被**注释掉**
- `FutuBroker` 从未实现，只有 Mock（`cmd/atlas/broker.go:109` TODO）
- **结果**：信号生成后不会触发任何下单，execution 层目前是死代码

### 3.3 分析周期忽略「每标的策略」配置 🟡 → ✅ 已修复

`internal/app/app.go:238` 调用 `a.strategies.Analyze()`（对所有标的跑全部策略），而引擎里已有 `AnalyzeWithStrategies(strategyNames)`（`engine.go:110`）却未被使用。`WatchlistItem.Strategies` 字段（config 里配置的 per-symbol 策略）实际**被丢弃**。

### 3.4 采集器选择过于简化 🟡 → ✅ 已修复

`internal/app/app.go:202` 注释明说 `simplified: just use first available`，循环试每个 collector。而 Web 层 `api/handler/api/symbol_detail.go:158` 已有智能 `selectCollector()`，核心分析路径却没用——加密标的可能被错误路由到股票采集器。

### 3.5 Web Signals 页面是空壳 🟡 → ✅ 已修复

`internal/api/handler/web/signals.go:23` `// TODO: Fetch actual signals from storage` 返回空列表，而 API 层 `api/handler/api/signals.go` **已经**接好了 `SignalStore`。Web 页面没复用已有的查询能力。

---

## 四、其他技术债

| 项 | 位置 | 风险 | 状态（2026-06-10） |
|----|------|------|------|
| API 无认证 | 架构 review 标注「高」 | 🔴 暴露即可被任意调用 | ✅ 已加 `APIKeyAuth`；注意空 key 时静默关闭、Web UI 路由无认证 |
| 串行采集 | 大 watchlist 延迟累积（P1） | 🟡 性能 | ❌ 未动 |
| LLM 同步阻塞 | 阻塞信号路由（2-10s） | 🟡 性能 | ❌ 未动，且 Arbitrator 接线后已进入热路径，**风险上升** |
| 外部采集器低覆盖 | eastmoney 8.6% / lixinger 4.5% / yahoo 25% | 🟡 质量 | ❌ 未动 |
| backtest CLI 未接引擎 | `cmd/atlas/backtest.go:62` TODO | 🟡 功能 | ❌ 未动 |
| dashboard 数据未接 | `web/dashboard.go:27` SignalsToday 硬编码 0 | 🟡 功能 | ✅ 已接 SignalStore |

---

## 五、下一步优化建议（按优先级）

> 2026-06-10 修订：原 P0 全部完成、原 P1-6 完成。原 P2-7/P2-8 因 Arbitrator 进入热路径而合并上调为 P1。

### ~~P0 — 连通已有功能（投入小、价值大）~~ ✅ 已全部完成（`4c7861f`）

功能代码都写好了，只差几行接入代码就能让 README 宣传的特性真正生效。

1. ~~接线 LLM Meta-Strategy：把 `meta.Arbitrator` 插入 `analyzeSymbol` 的信号路由前~~ ✅
2. ~~修复 per-symbol 策略：`strategies.Analyze` → `AnalyzeWithStrategies(item.Strategies)`~~ ✅
3. ~~核心路径改用智能 `selectCollector`，复用 Web 层逻辑~~ ✅
4. ~~Web Signals / dashboard 页接 `SignalStore`（API 已有，照搬即可）~~ ✅

### P1 — 完成 M4 闭环 + 分析循环并行化

5. 实现 `FutuBroker` 或先启用 paper-trading 模式接线 `ExecutionManager`（解开 `serve.go:199` 注释即可先验证链路）
6. ~~给 API 加 API Key/JWT 认证中间件~~ ✅ 已完成（建议补充：空 key 时打 warning 日志）
7. **分析循环并行化**（原 P2-7 + P2-8 合并上调）：worker pool 并行处理标的，LLM 仲裁随之并行；每个冲突标的的同步 LLM 调用（2-10s）叠加串行采集，watchlist 越大延迟线性累积

### P2 — 性能与健壮性

8. OHLCV 历史数据 TTL 缓存
9. 补外部采集器（eastmoney/lixinger/yahoo）的测试覆盖
10. backtest CLI 接入回测引擎（`backtest.go:62`）

---

## 六、结论

ATLAS 工程成熟度高、质量好（编译干净、358 测试全过、架构清晰）。当前最大的机会**不是写新功能，而是把已建好却未接线的模块连通**——P0 接线类修复投入产出比最高。M4 实盘闭环与 API 认证是进入生产前必须补齐的两块。

**2026-06-10 更新**：「先连通再新建」的判断已被实践验证——P0 四项在单个 PR（`4c7861f`，+718/-103 行）内全部完成，API 认证也已落地。剩余两大重点：① M4 实盘闭环（paper-trading 先行）；② 分析循环并行化（Arbitrator 接线后 LLM 同步阻塞已进入热路径，与串行采集叠加放大）。
