# ATLAS 设计文档落地核查报告

> 日期：2026-07-02
> 范围：docs/ 下全部 54 份设计/实施/运维文档,对照 master 分支(f9b3d3a)代码逐项核实
> 方法：按主题分 6 组并行核查(每组逐文档提取交付物清单 → 代码逐项验证,给出 文件:行号 证据),外加对历史评审(2026-06-03 / 2026-06-11)追踪项与测试健康度的独立复核
> 关联文档：`2026-06-03-project-status-and-optimization.md`、`2026-06-11-qlib-integration-analysis.md`、`2026-06-11-asset-coverage-and-percentile-analysis.md`

---

## 一、总体结论

docs 下 54 份设计/实施文档核查完毕:**2026 年以来的文档(约 30 份)基本 100% 完整落地,且实现质量普遍优于计划**;缺口集中在 **2025-12 的早期架构愿景与 M2/M3/M4 的部分设计承诺**。

整体健康度好:

- `go build ./...` 干净;
- 全量 Go 测试仅 coingecko/okx 两个依赖真实网络的集成测试偶发失败(HTTP 429 限流);
- Python 侧 pytest 全绿(qlib_eval 122 + IC 相关 63)。

---

## 二、各主题落地情况

| 主题 | 状态 | 说明 |
|---|---|---|
| Phase 1-2(采集/策略/通知主链路) | ✅ 基本完整 | 仅缺 `rsi_extreme` 策略 |
| Phase 3(回测/S3/Web UI) | ⚠️ 部分 | WebSocket 实时推送未做,Web 为普通 HTTP 渲染 |
| Phase 4(LLM/meta/broker 抽象) | ⚠️ 部分 | Futu 不再实现(详见第三节);`atlas arbitrate/synthesize` CLI 缺;prompts 内联未外置 |
| M1 生产就绪 / review-fixes(19 项) | ✅ 完整 | 11 个修复任务全部可验证 |
| M2 API 改进 | ⚠️ 实施完整,设计部分 | `/api/htmx/*` 局部端点、`job/runner.go` 未做(整页渲染替代) |
| M3 可观测性 | ⚠️ 部分 | **告警评估器是死代码**(见第三节) |
| M4 实盘交易 | ⚠️ 部分 | paper 模式已通过 `wireExecution` 接线(`cmd/atlas/executor.go:141`);FutuBroker **决定不实现**(Futu 已禁止大陆用户使用,2026-07-02 决策),遗留 TODO 待清理(`cmd/atlas/broker.go:109`) |
| Watchlist market/type、Crypto collector、Notifier 接线 | ✅ 完整 | crypto 设计层 3 处细节未实现(`SupportsSymbol`、sentinel 错误、"仅可重试才 fallback") |
| Qlib 评估管线、数据 bundle、percentile 步进(8 份) | ✅ 完整 | 偏差均为向后兼容增强(lookback=0 全历史、`--expected-symbols` 防呆、多市场扩展) |
| US/HK 信号评估、lixinger 重写、benchmark 参数(8 份) | ✅ 完整 | lixinger 实现对齐 QA 修正版 spec,优于计划草案 |
| Qlib 数据仓库 Phase1/2、since-inception Phase3、runbook | ✅ 完整 | runbook 文档滞后(见第三节) |
| Telegram digest 表格 + PE% 列、IC/IR 评估(6 份) | ✅ 完整 | IC/IR 的 §4 文件清单、§5 接口签名、§6 done_criteria 逐条核对全部满足 |

### 历史技术债消化情况

早期评审(2026-06-03 / 2026-06-11)追踪的技术债已大部分消化:

| 追踪项 | 状态 | 证据 |
|---|---|---|
| P1-5 ExecutionManager 接线 | ✅(paper 模式) | `cmd/atlas/executor.go:141` `wireExecution`;FutuBroker 已决定不实现 |
| P1-7 分析循环并行化 | ✅ | `Analysis.Workers` + errgroup `SetLimit`,`internal/app/app.go:369` |
| P2-9 外部采集器测试覆盖 | ✅ | eastmoney 86.5% / lixinger 91.9% / yahoo 82.4% |
| P2-10 backtest CLI 接引擎 | ✅ | `cmd/atlas/backtest.go:80` 起用 `strategy.NewEngine()` |
| 策略 `AssetTypes` 声明与过滤 | ✅ | 全部策略已声明;`app.go:787-795` 交叉校验 |
| `core.AssetCrypto` 类型统一 | ✅ | `internal/core/types.go:25` |
| 历史百分位监控 | ✅ | pe_percentile / price_percentile 策略 + lixinger cvpos + qlib 数仓/PIT 全链路 |

---

## 三、尚未落地的实质缺口

1. **M3 告警评估器未接入运行时(最值得关注)**。`internal/alert/evaluator.go` 实现和单测齐备,但 `cmd/atlas/serve.go` 从未实例化它——配置里的 `alerts.rules` 不会被消费,`up`/`error_rate` 等内置告警指标也没有喂数逻辑。这是典型的"已建成未接线",接通成本低。
2. **FutuBroker 已决定不实现**(Futu 已禁止大陆用户使用,2026-07-02 决策)。实盘链路定格为 paper 模式;遗留的 `cmd/atlas/broker.go:109` TODO、`FutuConfig` 配置占位与 M4 设计中的相关承诺待清理/标注(见第四节)。
3. **信号无持久化存储**。架构设计中的 TimescaleDB 热存储从未实现,信号仅存内存(`internal/storage/signal/memory.go`),重启即丢。
4. **早期设计愿景从未推进**:WebSocket 推送、HTMX 局部端点(`/api/htmx/*`)、sina 采集器、Parquet 冷存储、CircuitBreaker——多数可判定为 YAGNI,但处于"未兑现也未撤回"状态。
5. **文档漂移**:runbook(`docs/ops/qlib-warehouse-runbook.md`)写"安装 3 个 LaunchAgent",实际 `scripts/ops/install-services.sh` 装 4 个(多出每 30 分钟的 analysis 服务及 `trigger-analysis.sh`、`services.sh analysis-*` 子命令,手册未记载)。

### 设计层小缺口(不影响功能)

- crypto collector 设计的 `Provider.SupportsSymbol`、sentinel 错误(`ErrSymbolNotFound` 等)、"仅可重试错误触发 fallback"语义未实现(实施计划本身未纳入,计划已 100% 完成)。
- Phase4 设计的 prompts 外置(`internal/meta/prompts/*.txt`)未做,实际为代码内常量。
- notifier 接线时超计划新增了 telegram `WithProxy`(突破原计划 YAGNI 边界,属良性扩展)。

---

## 四、后续优化方向建议

### 高杠杆(建议优先)

1. **接线 alert.Evaluator**:在 serve 中实例化、起评估循环、把 metrics Registry 的数据喂给 `SetMetrics`、告警走已有 notifier——所有零件都在,只差装配,即可兑现 M3 的端到端告警闭环。
2. **隔离网络型集成测试**:coingecko/okx 测试打真实 API(429 偶发红、单包 20 秒拖慢全量测试),建议加 build tag(如 `//go:build integration`)或改 httptest,保证 `go test ./...` 确定性通过。

### 主线演进(路线图的下一步)

3. **方向② ML sidecar**:IC/IR 评估管线(2026-06-22)已完整落地并自证可信,`scores.csv` 契约已钉死,`make signal-ic` 就是验收抓手——设计文档明确写"IC 站得住再投 sidecar 工程",前置条件现已全部就绪。
4. **信号轻量持久化**:不必追 TimescaleDB 愿景,项目已引入 `modernc.org/sqlite`(数仓在用),给 SignalStore 加一个 sqlite 实现即可解决重启丢信号,顺带让 track_record context provider 有真实历史可用。

### 实盘方向(已决策:paper-only)

5. **清理 FutuBroker 遗留痕迹**:因 Futu 已禁止大陆用户使用,FutuBroker 不再实现(2026-07-02 决策),实盘链路明确为 paper-only。收尾工作:移除 `cmd/atlas/broker.go:109` 的 TODO、清理 `FutuConfig` 配置占位(`internal/config/config.go:205`)与 `config.example.yaml` 相关段落,并在 M4 设计文档(`2026-01-01-m4-live-trading-design.md`)标注该承诺已撤回。若未来需要真实券商,再按 Broker 接口另选可用券商评估。

### 文档治理(低成本)

6. runbook 补记 analysis 服务链;给 2025-12 架构设计中已被实践否决的部分(TimescaleDB/gin/WebSocket/Parquet)加"已被 superseded"标注;crypto 设计的 3 处未实现细节要么补上要么从设计中删除,避免后来者误读。
