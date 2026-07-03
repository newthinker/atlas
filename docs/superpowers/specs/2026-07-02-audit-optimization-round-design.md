# 落地核查优化轮 — 设计文档

> 日期:2026-07-02
> 状态:设计已确认(brainstorming 逐节评审通过)
> 上游分析:`docs/reviews/2026-07-02-docs-implementation-audit.md`(54 份设计文档落地核查)
> 定位:一轮补齐核查发现的"已建未接线/遗留/文档漂移"缺口;ML sidecar(整合方向②)**单独立项,不在本轮**

## 0. 一句话目标

用三个独立 PR(清理加固 → alert 接线 → 信号持久化)消化核查报告的全部 5 个非 sidecar 优化项,
每波可独立合并、独立回滚。

## 1. 已确认的范围决定

| 决定点 | 结论 | 理由 |
|---|---|---|
| 本轮范围 | 5 项全包:FutuBroker 清理、测试隔离、文档治理、alert 接线、sqlite 持久化 | 一轮补齐全部"已建未接线/遗留"缺口 |
| ML sidecar | 不在本轮,单独立项 | 独立子系统(Python 服务 + 新策略 + 部署),体量远超其余项 |
| 交付组织 | 单 spec、按依赖分 3 波次、每波一个 PR | 波次间可独立合并回滚;Wave 2/3 互不依赖可并行 |
| 告警去向 | 复用已注册 notifier(telegram/email/webhook) | M3 设计原意,零新增基础设施 |
| 指标喂数 | 通用 registry 桥接(Snapshot)+ 少量派生指标 | 一次投入,规则可引用任意已有指标 |
| 信号存储 | sqlite 新实现 + 配置可选 backend(默认 sqlite) | 接口不变,API/Web/router 零改动 |
| 测试隔离 | `//go:build integration` build tag | 保留活 API 探测能力,改动最小 |

## 2. Wave 1 — 清理加固(PR #1,零功能影响)

### 2a. FutuBroker 遗留清理

背景:Futu 已禁止大陆用户使用,FutuBroker 决定不实现(2026-07-02 决策),实盘链路定格 paper-only。

- `cmd/atlas/broker.go` `getBroker` 删除 `case "futu"` 分支及 TODO 注释;
  `futu` 落入 `default` 报 `unknown broker provider: futu`(语义从"尚未实现"转为"不支持")。
- `internal/config/config.go` 删除 `FutuConfig` 结构体与 `BrokerConfig.Futu` 字段;
  **同时把 `Broker.Provider` 的缺省值从 `"futu"` 改为 `"mock"`**(config.go:355,
  否则删除 futu case 后开启 broker 即报错)。
  viper 对 yaml 中多余的 `futu:` 段静默忽略,老配置文件不会崩。
- `configs/config.example.yaml` 删除 futu 相关段落。
- `docs/plans/2026-01-01-m4-live-trading-design.md` 头部加撤回标注。

### 2b. 网络型集成测试隔离

- `internal/collector/crypto/{okx,coingecko,binance}` 三包中打真实 API 的 `Test*_Integration`
  测试,各自拆到新文件 `*_integration_test.go`,文件头加 `//go:build integration`;
  httptest 类单测留在原文件不动。
- `Makefile` 新增 `test-integration` target:`go test -tags integration ./internal/collector/crypto/...`。
- 验收:`go test ./...` 不联网、确定性全绿;`make test-integration` 保留活 API 探测。

### 2c. 文档治理

- runbook(`docs/ops/qlib-warehouse-runbook.md`)补记第 4 个 LaunchAgent:
  analysis 服务(`com.newthinker.atlas.analysis.plist`)、`trigger-analysis.sh`、
  `services.sh analysis-now / analysis-logs` 子命令。
- `docs/plans/2025-12-28-atlas-architecture-design.md` 头部加"部分设计已被实践 superseded"
  注记,点名 TimescaleDB / gin / WebSocket / Parquet / CircuitBreaker / sina 六项及现实替代
  (net/http、内存+sqlite 存储、整页渲染等)。
- `docs/plans/2026-01-08-crypto-collector-design.md` 对 `SupportsSymbol` / sentinel 错误 /
  "仅可重试才 fallback" 三处加"未实现,实施时裁剪"标注(不补实现,尊重既成 YAGNI 判断)。

## 3. Wave 2 — alert.Evaluator 接线(PR #2)

背景:`internal/alert`(Evaluator + Rule 表达式解析 + for/冷却语义)实现与单测齐备,
但 serve 从未实例化——配置的 `alerts.rules` 是死配置。本波只做装配件,不动评估器本体。

### 3a. 指标快照(通用 registry 桥接)

- `internal/metrics` 新增 `snapshot.go`:给 Registry 加 `Snapshot() map[string]float64`。
  遍历 prometheus `Gather()` 结果:gauge/counter 取值(多 label 序列聚合求和),
  histogram 展开为 `_count` / `_sum` 两键。
- prometheus 依赖封闭在 metrics 包内;alert 包继续只见 `map[string]float64`,
  与 `Evaluator.SetMetrics` 的既有边界一致。
- 效果:规则表达式可引用任意已注册指标(如 `atlas_signals_generated_total > 100`)。

### 3b. 派生指标(runner 内计算,首批 2 个)

| 指标 | 计算 | 说明 |
|---|---|---|
| `http_error_rate` | runner 保留上次快照,用 5xx 与总请求的**增量**算区间错误率 | 累计比率无意义,必须 delta |
| `signals_24h` | `SignalStore.Count(from=now-24h)` | 直接复用信号存储 |

裁剪:M3 设计的 `up` 恒为 1(进程活着才会评估),无信息量,不做;
`analysis_failures_1h` 待 metrics 侧有对应 counter 后规则天然可用,不专门造。

### 3c. notifier 适配 + serve 装配

- 适配器(alert.Notifier ← notifier.Notifier):
  - 若底层 notifier 实现可选接口 `interface{ SendText(string) error }` → 直发文本;
    telegram 加一个公开 `SendText`(内部走已有 `sendRaw`,约 3 行);
  - 否则回退:包装为系统信号 `core.Signal{Symbol:"SYSTEM", Strategy:"alert", Reason:告警文本}`
    走 `Send`。email/webhook 先用回退路径,零改动。
- `cmd/atlas/serve.go`:`cfg.Alerts.Enabled` 时,把已注册 notifiers 包上适配器构造
  `Evaluator`,起 goroutine 按 `check_interval` 循环:
  `Snapshot()` → 补派生指标 → `SetMetrics` → `EvaluateAll(rules)`;随 app ctx 优雅退出。
- `config.AlertRule` 与 `alert.Rule` 字段一一对应,直接映射。
- `config.example.yaml` 补可用示例规则(如 `http_error_rate > 0.1` + `for: 5m`)。

### 3d. 测试

- snapshot 单测(注入假 registry:gauge/counter/histogram/多 label);
- delta 派生指标单测(两次快照差);
- 适配器双路径单测(SendText 直发 / 系统信号回退);
- serve 装配测试用 `observer.ObservedLogs` 验证(沿用 notifier 接线 PR 的测试风格)。

## 4. Wave 3 — 信号 sqlite 持久化(PR #3)

背景:信号仅存内存(`internal/storage/signal/memory.go`),重启即丢。
TimescaleDB 愿景不追,用已有 `modernc.org/sqlite` 落地轻量持久化。

### 4a. 新实现 `internal/storage/signal/sqlite.go`

- `NewSQLiteStore(path string)`:复用 `modernc.org/sqlite`(**保持锁定 v1.38.2,不升版**);
  打开时设 `journal_mode=WAL` + `busy_timeout`;单写多读由 `database/sql` 连接池覆盖。
- 单表 schema(`CREATE TABLE IF NOT EXISTS`,不引入迁移框架):

  ```sql
  CREATE TABLE IF NOT EXISTS signals (
    id           TEXT PRIMARY KEY,
    symbol       TEXT NOT NULL,
    action       TEXT NOT NULL,
    confidence   REAL,
    price        REAL,
    reason       TEXT,
    strategy     TEXT,
    metadata     TEXT,            -- JSON
    generated_at TEXT NOT NULL    -- RFC3339
  );
  CREATE INDEX IF NOT EXISTS idx_signals_symbol_time ON signals(symbol, generated_at);
  CREATE INDEX IF NOT EXISTS idx_signals_time ON signals(generated_at);
  ```

- ID 生成沿用内存实现的 `sig_<unixnano>_<counter>` 方案,行为一致。
- `List/Count` 把 `ListFilter`(symbol/strategy/action/from/to/limit/offset)直译 WHERE;
  排序与分页语义以内存实现为基准(由契约测试钉死)。

### 4b. 配置与装配

- config 新增 `storage.signals` 节:`backend: memory|sqlite`(默认 **sqlite**)、
  `path`(默认 `data/signals.db`)。
- `cmd/atlas/serve.go` 按配置选择构造(替换现 `serve.go:86` 固定 memory)。
- sqlite 打开失败**快速失败**(启动即退,不降级内存):用户显式要求持久化时
  静默降级等于悄悄丢数据,与采集器"降级仍正确"的场景性质不同。
- `config.example.yaml` 补注释说明两种 backend 与默认值。

### 4c. 契约测试

新增 `store_contract_test.go`:同一套表驱动用例依次跑 memory 与 sqlite 两实现——
Save 赋 ID、GetByID、各 filter 组合、排序、分页、Count、metadata JSON 往返。
sqlite 测试用 `t.TempDir()` 建库。保证换后端对 API/Web/router 完全透明。

## 5. 明确不做(YAGNI,本轮边界)

- 不做 retention 清理:分钟级周期 × 十几个标的,一年不过几十 MB;留 backlog。
- 不做内存 store 历史迁移:重启前数据本在内存,无可迁。
- 不动 `track_record` context provider:自然受益于持久化,接入是另一件事。
- 不补 crypto 设计的 `SupportsSymbol` / sentinel 错误 / 可重试 fallback:仅文档标注。
- 不做 ML sidecar:单独立项(前置条件 IC/IR 验收抓手已就绪)。

## 6. 整体验收

- 三个 PR 各自 `go test ./...` 全绿;Wave 1 合并后该命令不再联网、确定性通过。
- `make test-integration` 手动跑通(允许外部 API 限流性失败,不进默认门禁)。
- Wave 2 合并后:配置一条规则 + 触发条件,可在 telegram 收到 `[SEVERITY] name: message` 告警。
- Wave 3 合并后:重启进程,`/api/v1/signals` 仍能查到重启前信号。
- 每波提交前 `gitnexus_detect_changes()` 确认改动范围符合预期。

## 7. 风险与约束

- **alert 误报噪声**:首批规则保守(仅示例 1 条),冷却 5 分钟默认值沿用评估器现值;
  规则调优交给运行观察,不在本轮预设。
- **sqlite 写入热路径**:信号产生频率极低(路由过滤后每周期至多几条),同步写无性能风险。
- **配置兼容**:删除 `FutuConfig` 后老配置文件的 `futu:` 段被 viper 静默忽略;
  `storage.signals` 缺省时默认 sqlite——**行为变化**(原为内存),在 PR 描述与
  config.example.yaml 中显著说明。
- **GOTOOLCHAIN=local 约束**:sqlite 依赖保持 v1.38.2,构建环境不升 toolchain。
