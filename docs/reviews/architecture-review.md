# ATLAS 系统架构评审报告

**评审日期**: 2024-12-30
**评审版本**: Phase 4 Complete
**评审人**: System Architect

---

## 1. 项目概览

| 指标 | 数值 |
|------|------|
| Go 源文件 | 89 个 |
| 测试文件 | 38 个 |
| 代码行数 | 7,683 行 |
| 测试包数 | 30 个 |
| 测试状态 | 全部通过 |

### 1.1 项目简介

**ATLAS** (Asset Tracking & Leadership Analysis System) 是一个基于 Go 语言的自动化交易信号生成平台，支持多市场资产监控。系统采用 Clean Architecture + Plugin Pattern 设计，具有良好的可扩展性。

### 1.2 技术栈

- **语言**: Go 1.24.4
- **CLI**: Cobra
- **配置**: Viper
- **日志**: Uber Zap
- **Web**: net/http + HTMX
- **LLM**: Anthropic SDK, OpenAI SDK, Ollama
- **存储**: LocalFS, AWS S3

---

## 2. 架构模式

### 2.1 整体架构：Clean Architecture + Plugin Pattern

```
┌─────────────────────────────────────────────────────────────────┐
│                         cmd/atlas (CLI)                         │
├─────────────────────────────────────────────────────────────────┤
│                      internal/app (Orchestrator)                │
├──────────┬──────────┬──────────┬──────────┬──────────┬─────────┤
│ collector│ strategy │  router  │ notifier │   llm    │ broker  │
│ (数据采集)│ (策略分析)│ (信号路由)│ (通知发送)│ (AI能力) │ (券商)  │
├──────────┴──────────┴──────────┴──────────┴──────────┴─────────┤
│                          internal/core (领域模型)               │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 项目结构

```
atlas/
├── cmd/atlas/              # CLI 入口 (Cobra)
│   ├── main.go
│   ├── serve.go            # HTTP 服务器启动
│   ├── backtest.go         # 回测 CLI
│   ├── broker.go           # 券商操作 CLI
│   └── version.go
├── internal/               # 核心应用逻辑
│   ├── api/                # HTTP 服务器 & Web UI (HTMX)
│   ├── app/                # 主应用协调器
│   ├── backtest/           # 策略回测框架
│   ├── broker/             # 券商抽象 (Futu, Mock)
│   ├── collector/          # 数据采集器 (Yahoo, Eastmoney, Lixinger)
│   ├── config/             # 配置管理 (Viper)
│   ├── context/            # LLM 市场上下文提供者
│   ├── core/               # 核心类型 (Quote, OHLCV, Signal)
│   ├── indicator/          # 技术指标 (SMA, EMA)
│   ├── llm/                # LLM 提供商 (Claude, OpenAI, Ollama)
│   ├── logger/             # 日志 (Uber Zap)
│   ├── meta/               # LLM 元策略 (Arbitrator, Synthesizer)
│   ├── notifier/           # 信号通知 (Telegram, Email, Webhook)
│   ├── router/             # 信号路由 & 过滤
│   ├── storage/            # 归档存储 (LocalFS, S3)
│   └── strategy/           # 交易策略 (MA Crossover, PE Band, Dividend Yield)
├── configs/                # 配置模板
├── docs/                   # 文档 & 设计文档
└── config.example.yaml     # 配置模板
```

---

## 3. 核心接口设计

### 3.1 接口清单

| 接口 | 职责 | 实现数量 |
|------|------|----------|
| `Collector` | 市场数据采集 | 3 (Yahoo, Eastmoney, Lixinger) |
| `Strategy` | 交易策略分析 | 3 (MA Crossover, PE Band, Dividend) |
| `Notifier` | 信号通知 | 3 (Telegram, Email, Webhook) |
| `Broker` | 券商集成 | 1 (Mock, Futu planned) |
| `llm.Provider` | LLM 提供商 | 3 (Claude, OpenAI, Ollama) |
| `archive.Storage` | 归档存储 | 2 (LocalFS, S3) |

### 3.2 Collector 接口

```go
type Collector interface {
    Name() string
    SupportedMarkets() []core.Market
    Init(cfg Config) error
    Start(ctx context.Context) error
    Stop() error
    FetchQuote(symbol string) (*core.Quote, error)
    FetchHistory(symbol string, start, end time.Time, interval string) ([]core.OHLCV, error)
}
```

**实现**:
- **Yahoo Finance**: US, HK, EU 市场
- **Eastmoney**: CN_A 市场 (上海/深圳 A 股)
- **Lixinger**: CN_A 基本面数据 (PE, PB, ROE)

### 3.3 Strategy 接口

```go
type Strategy interface {
    Name() string
    Description() string
    RequiredData() DataRequirements
    Init(cfg Config) error
    Analyze(ctx AnalysisContext) ([]core.Signal, error)
}
```

**实现**:
- **MA Crossover**: 技术分析 (金叉/死叉)
- **PE Band**: 基本面分析 (PE 历史百分位)
- **Dividend Yield**: 股息率分析

### 3.4 Notifier 接口

```go
type Notifier interface {
    Name() string
    Init(cfg Config) error
    Send(signal core.Signal) error
    SendBatch(signals []core.Signal) error
}
```

**实现**:
- **Telegram**: Markdown 格式, emoji 指示器
- **Email**: HTML + 纯文本, SMTP 支持
- **Webhook**: HTTP POST, 自定义 headers

### 3.5 LLM Provider 接口

```go
type Provider interface {
    Name() string
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
```

**实现**:
- **Claude**: Anthropic SDK
- **OpenAI**: go-openai SDK
- **Ollama**: 本地 HTTP API

### 3.6 Broker 接口

```go
type Broker interface {
    // 元数据
    Name() string
    SupportedMarkets() []core.Market

    // 连接
    Connect(ctx context.Context) error
    Disconnect() error
    IsConnected() bool

    // 读取操作
    GetPositions(ctx context.Context) ([]Position, error)
    GetOrders(ctx context.Context, filter OrderFilter) ([]Order, error)
    GetAccountInfo(ctx context.Context) (*AccountInfo, error)
    GetTradeHistory(ctx context.Context, start, end time.Time) ([]Trade, error)

    // 写入操作 (已定义, Phase 4 未实现)
    PlaceOrder(ctx context.Context, order OrderRequest) (*Order, error)
    CancelOrder(ctx context.Context, orderID string) error
    ModifyOrder(ctx context.Context, orderID string, changes OrderChanges) (*Order, error)
}
```

---

## 4. 数据流架构

### 4.1 实时信号生成流程

```
Ticker (可配置间隔)
    ↓
RunAnalysisCycle()
    ├─→ 获取 watchlist 标的
    │
    ├─→ 对每个标的:
    │   ├─→ FetchHistory (依次尝试各采集器)
    │   │
    │   ├─→ 准备 AnalysisContext
    │   │   ├─ Symbol, OHLCV 数据, 时间戳
    │   │   └─ 可选: Quote, Fundamental, Indicators
    │   │
    │   ├─→ Strategy.Analyze() (所有启用的策略)
    │   │   └─→ 生成 Signals:
    │   │       ├─ Action (BUY/SELL/HOLD)
    │   │       ├─ Confidence (0-1)
    │   │       ├─ Reason (可读原因)
    │   │       ├─ Metadata (策略特定)
    │   │       └─ GeneratedAt 时间戳
    │   │
    │   └─→ Router.Route(signal)
    │       ├─→ passesFilters() 检查:
    │       │   ├─ Confidence ≥ min_confidence
    │       │   ├─ Action in enabled_actions
    │       │   └─ Cooldown 时间已过
    │       │
    │       └─→ Registry.NotifyAll(signal)
    │           ├─→ Telegram: Markdown 消息
    │           ├─→ Email: SMTP 发送
    │           └─→ Webhook: HTTP POST
    │
    └─→ 日志: 生成的信号, 路由结果, 错误
```

### 4.2 LLM 增强信号流程 (元策略)

```
多策略信号生成
    ↓
Router 检测到信号冲突
    ↓
Signal Arbitrator 调用
    ├─→ 收集上下文:
    │   ├─ 市场状态/波动率
    │   ├─ 策略历史业绩
    │   ├─ 近期新闻情绪
    │   └─ 冲突信号详情
    │
    ├─→ 构建 LLM prompt
    │
    ├─→ LLM Provider (Claude/OpenAI/Ollama)
    │   └─→ JSON 响应: {decision, confidence, reasoning, weighted_from}
    │
    └─→ 仲裁后信号路由
        └─→ Notifiers
```

### 4.3 回测流程

```
用户运行回测命令
    ↓
Backtester.Run(strategy, symbol, start, end)
    ├─→ 获取历史 OHLCV
    │
    ├─→ 对每个 bar (滚动窗口):
    │   ├─→ 创建 AnalysisContext
    │   ├─→ Strategy.Analyze()
    │   └─→ 收集信号及收盘价
    │
    ├─→ 信号转换为交易:
    │   ├─ BUY → 开仓
    │   ├─ SELL → 平仓
    │   └─ 计算收益: (exit - entry) / entry
    │
    └─→ 计算统计:
        ├─ 胜率, 盈亏比
        ├─ 回撤, 夏普比率
        └─ 逐笔交易指标
```

---

## 5. 设计模式

### 5.1 已应用的模式

| 模式 | 应用位置 |
|------|----------|
| Factory | `llm/factory/factory.go` |
| Registry | `collector/registry.go`, `notifier/registry.go` |
| Strategy | `strategy/*` (策略模式) |
| Observer | `router/router.go` (信号分发) |
| Adapter | `collector/yahoo`, `collector/eastmoney` |
| Dependency Injection | 所有主要组件通过构造函数注入 |

### 5.2 Clean Architecture 原则

**依赖规则**: 内层 (core types) 不依赖外层 (API, notifiers, collectors)

**层级独立**:
- `core/`: 纯领域模型
- `strategy/`: 业务逻辑, 仅依赖 core
- `collector/`, `notifier/`: 可互换实现
- `api/`: 展示层 (可替换为 gRPC, GraphQL)

### 5.3 并发模式

**RWMutex 使用**:
- App: watchlist, running state
- Router: cooldowns map
- Broker (Mock): positions, orders, trades
- Registry (Collectors/Strategies/Notifiers): 全部使用 RWMutex

**Context 传播**:
- App.Start() 创建 context 用于优雅关闭
- Strategy 分析遵循 ctx.Done()
- Backtest 遵循 ctx.Done()

---

## 6. 依赖分析

### 6.1 核心依赖

| 依赖 | 版本 | 用途 |
|------|------|------|
| `spf13/cobra` | v1.10.2 | CLI 框架 |
| `spf13/viper` | v1.21.0 | 配置管理 |
| `go.uber.org/zap` | v1.27.1 | 结构化日志 |
| `anthropic-sdk-go` | v1.19.0 | Claude API |
| `go-openai` | v1.41.2 | OpenAI API |
| `aws-sdk-go-v2` | v1.41.0 | S3 存储 |

### 6.2 依赖风险评估

| 风险 | 依赖 | 缓解措施 |
|------|------|----------|
| 低 | 所有依赖都是成熟库 | 定期更新 |
| 中 | LLM SDK 变更 | 接口隔离已做好 |

---

## 7. 优点分析

### 7.1 架构优点

| 优点 | 说明 |
|------|------|
| 清晰的接口抽象 | 每个组件都有明确的接口定义 |
| 插件化设计 | 通过 Registry 模式支持动态注册 |
| 依赖注入 | App 通过构造函数注入依赖 |
| 领域模型独立 | `internal/core` 不依赖其他包 |
| 测试覆盖良好 | 38 个测试文件，所有包测试通过 |
| 类型安全 | 枚举和自定义类型代替字符串 |
| 多市场设计 | 抽象市场 (US, HK, CN_A) 贯穿所有层 |
| LLM 集成良好 | 设计良好的 provider 抽象支持多后端 |

### 7.2 代码示例

**接口抽象**:
```go
type Strategy interface {
    Name() string
    Analyze(ctx AnalysisContext) ([]core.Signal, error)
}
```

**Registry 模式**:
```go
registry := NewRegistry()
registry.Register("yahoo", yahooCollector)
```

**依赖注入**:
```go
func New(cfg *config.Config) *App {
    return &App{cfg: cfg, collectors: ..., strategies: ...}
}
```

---

## 8. 安全性与性能分析

### 8.1 安全性评估

| 领域 | 当前状态 | 风险等级 | 建议 |
|------|----------|----------|------|
| API 认证 | 无认证机制 | 高 | 添加 JWT/API Key 认证 |
| 敏感数据 | 环境变量存储 | 低 | 已采用最佳实践 ✓ |
| 输入验证 | 部分实现 | 中 | 添加统一验证层 |
| SQL 注入 | N/A (无 SQL) | - | 不适用 |
| 日志脱敏 | 未实现 | 中 | API Key 等敏感信息需脱敏 |
| HTTPS | 依赖反向代理 | 低 | 文档已说明 Nginx/Caddy 配置 ✓ |

### 8.2 性能特征

| 组件 | 并发模型 | 瓶颈分析 |
|------|----------|----------|
| 数据采集 | 串行 (按标的) | 大 watchlist 时延迟累积 |
| 策略分析 | 并行 (per strategy) | CPU 密集，受核心数限制 |
| 通知发送 | 并行 (per notifier) | 外部 API 延迟 |
| LLM 调用 | 同步阻塞 | API 延迟 2-10 秒 |

### 8.3 性能优化建议

| 优化项 | 说明 | 优先级 |
|--------|------|--------|
| 数据采集并行化 | 使用 worker pool 并行获取多个标的 | P1 |
| LLM 调用异步化 | 仲裁结果不阻塞信号路由 | P2 |
| 添加缓存层 | 对 OHLCV 历史数据添加 TTL 缓存 | P2 |
| 连接池复用 | HTTP client 连接池优化 | P3 |

---

## 9. 开发扩展指南

### 9.1 添加新策略

**步骤:**

1. 创建策略目录: `internal/strategy/<name>/`
2. 实现 `Strategy` 接口:

```go
// internal/strategy/rsi/strategy.go
package rsi

type Strategy struct {
    period     int
    overbought float64
    oversold   float64
}

func (s *Strategy) Name() string { return "rsi" }

func (s *Strategy) RequiredData() strategy.DataRequirements {
    return strategy.DataRequirements{
        PriceHistory: s.period + 1,
    }
}

func (s *Strategy) Analyze(ctx strategy.AnalysisContext) ([]core.Signal, error) {
    // 实现 RSI 计算和信号生成
}
```

3. 注册到引擎: `internal/strategy/engine.go`
4. 添加配置: `config.example.yaml`
5. 添加测试: `internal/strategy/rsi/strategy_test.go`

### 9.2 添加新采集器

**步骤:**

1. 创建采集器目录: `internal/collector/<name>/`
2. 实现 `Collector` 接口
3. 注册到 Registry: `internal/collector/registry.go`
4. 添加配置项

**关键方法:**

| 方法 | 说明 |
|------|------|
| `FetchQuote()` | 实时报价 |
| `FetchHistory()` | 历史 K 线 |
| `SupportedMarkets()` | 支持的市场列表 |

### 9.3 添加新通知器

**步骤:**

1. 创建目录: `internal/notifier/<name>/`
2. 实现 `Notifier` 接口
3. 注册到 Registry
4. 实现消息格式化 (参考 telegram 的 Markdown 格式)

### 9.4 添加新 LLM 提供商

**步骤:**

1. 创建目录: `internal/llm/<provider>/`
2. 实现 `llm.Provider` 接口
3. 在 `internal/llm/factory/factory.go` 添加 case
4. 添加配置结构到 `config.go`

---

## 10. 测试覆盖

### 10.1 测试文件分布 (38 个)

| 包 | 测试文件 |
|------|----------|
| `core` | `types_test.go` |
| `app` | `app_test.go` |
| `config` | `config_test.go` |
| `collector/*` | registry, yahoo, eastmoney, lixinger |
| `strategy/*` | engine, ma_crossover, pe_band, dividend_yield |
| `notifier/*` | registry, telegram, email, webhook |
| `router` | `router_test.go` |
| `broker/mock` | `mock_test.go`, `interface_test.go` |
| `backtest` | `backtester_test.go`, `types_test.go` |
| `meta` | `arbitrator_test.go`, `synthesizer_test.go` |
| `context/*` | market, track_record, news |
| `llm/*` | factory, claude, openai, ollama |
| `indicator` | `sma_test.go` |
| `storage/archive/*` | localfs, s3 |

### 10.2 测试模式

- Mock 实现用于测试 (broker/mock)
- 基于接口的测试 (所有主要组件)
- 测试文件与实现共置
- 主要为单元测试

---

## 11. 改进路线图

### 11.1 里程碑规划

| 里程碑 | 目标 | 包含任务 |
|--------|------|----------|
| M1: 生产就绪 | 系统可靠性达到生产标准 | 信号持久化, 错误处理标准化, 配置验证 |
| M2: API 完善 | Web Dashboard 可用 | API 端点实现, HTTP 测试, 前端集成 |
| M3: 可观测性 | 运维监控能力 | Metrics, Tracing, 告警集成 |
| M4: 实盘交易 | 券商集成上线 | Futu 实现, 风控模块, 订单管理 |

### 11.2 任务依赖关系

```text
                    ┌─────────────────┐
                    │ 统一错误类型    │
                    │ (core/errors)   │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              ▼              ▼              ▼
     ┌────────────┐  ┌────────────┐  ┌────────────┐
     │ 错误处理   │  │ 配置验证   │  │ API 错误   │
     │ 标准化     │  │            │  │ 响应格式   │
     └────────────┘  └────────────┘  └─────┬──────┘
                                           │
                                           ▼
                                    ┌────────────┐
                                    │ API 端点   │
                                    │ 实现       │
                                    └─────┬──────┘
                                          │
              ┌───────────────────────────┼───────────────────────────┐
              ▼                           ▼                           ▼
     ┌────────────┐              ┌────────────┐              ┌────────────┐
     │ 信号持久化 │              │ Metrics    │              │ Futu 券商  │
     │            │              │ 暴露       │              │ 集成       │
     └────────────┘              └────────────┘              └────────────┘
```

### 11.3 M1 详细任务清单

| 任务 | 依赖 | 产出 |
|------|------|------|
| 定义统一错误类型 | 无 | `core/errors.go` |
| 错误处理标准化 | 统一错误类型 | 更新 meta/, collector/, notifier/ |
| 信号持久化层 | 无 | `storage/signal/` 包 |
| Router 集成持久化 | 信号持久化层 | Router 写入信号到存储 |
| 配置验证方法 | 统一错误类型 | `config.Validate()` |
| 启动时配置校验 | 配置验证方法 | `serve.go` 调用验证 |

### 11.4 API 层当前状态

| 路由 | 状态 | 所属里程碑 |
|------|------|------------|
| `/api/health` | 已实现 | - |
| `/api/signals/recent` | 占位符 | M2 |
| `/api/backtest` | 占位符 | M2 |
| `/api/watchlist` | 占位符 | M2 |
| Web UI handlers | 模板引用 | M2 |

---

## 12. 总体评估

### 12.1 评分

| 维度 | 评分 | 说明 |
|------|------|------|
| 架构设计 | 95% | Clean Architecture, 接口清晰 |
| 代码质量 | 80% | 规范，可读性好 |
| 测试覆盖 | 80% | 38 个测试文件，全部通过 |
| 可扩展性 | 95% | 插件化设计，易于扩展 |
| 功能完整性 | 70% | API 层和部分功能待完善 |
| 运维友好性 | 60% | 缺少 metrics/tracing |
| 文档完整性 | 80% | README, 部署, 用户手册, API 文档 |

### 12.2 可视化评分

```
架构设计:     ████████████████████  95%
代码质量:     ████████████████░░░░  80%
测试覆盖:     ████████████████░░░░  80%
可扩展性:     ████████████████████  95%
功能完整性:   ██████████████░░░░░░  70%
运维友好性:   ████████████░░░░░░░░  60%
文档完整性:   ████████████████░░░░  80%
─────────────────────────────────────
综合评分:     B+ (Very Good) - 4.2/5
```

### 12.3 建议优先级

| 优先级 | 任务 | 原因 |
|--------|------|------|
| P0 | 信号持久化 | 避免信号丢失 |
| P0 | 错误处理标准化 | 提高可靠性 |
| P1 | API 实现完善 | Dashboard 依赖 |
| P1 | 配置验证 | 防止运行时错误 |
| P2 | 新闻提供商集成 | LLM 仲裁依赖 |
| P2 | Futu 券商实现 | 实盘交易 |
| P3 | Metrics/Tracing | 可观测性 |

---

## 11. 结论

ATLAS 项目采用了优秀的架构设计，Clean Architecture 和 Plugin Pattern 的结合使系统具有良好的可扩展性和可维护性。核心功能已完整实现，测试覆盖良好。

**主要优势**:
- 清晰的接口抽象和分层设计
- 插件化架构支持多数据源、多策略、多通知渠道
- LLM 集成设计良好，支持多个提供商

**需要改进**:
- 信号持久化机制
- 错误处理一致性
- API 层完善
- 可观测性能力

项目已完成 Phase 1-4 的开发，建议下一步优先补充信号持久化和错误处理标准化，以提高系统的可靠性和生产就绪度。

---

## 附录 A: 架构决策记录 (ADR)

### ADR-001: 选择 Clean Architecture

**状态:** 已采纳

**背景:**
需要一个支持多数据源、多策略、多通知渠道的可扩展架构。

**决策:**
采用 Clean Architecture，core 层定义领域模型，外层实现具体适配。

**理由:**

- 领域模型独立于框架和外部服务
- 便于测试 (可 mock 外层依赖)
- 支持替换任意外层实现

**后果:**

- 需要定义清晰的接口边界
- 代码量略增 (接口 + 实现分离)

---

### ADR-002: LLM Provider 抽象设计

**状态:** 已采纳

**背景:**
需要支持多个 LLM 提供商 (Claude, OpenAI, Ollama)，且可能随时切换。

**决策:**
定义 `llm.Provider` 接口，通过 Factory 模式根据配置创建实例。

**理由:**

- 提供商 API 差异大，需要统一抽象
- 配置驱动，无需改代码即可切换
- 支持本地模型 (Ollama) 降低成本

**后果:**

- 接口设计需兼顾各提供商能力
- 部分高级功能 (如 Claude 的 tool use) 暂未暴露

---

### ADR-003: 信号路由与过滤机制

**状态:** 已采纳

**背景:**
策略可能产生大量信号，需要过滤和去重。

**决策:**
Router 组件负责: Confidence 过滤 + Cooldown 去重 + 分发到所有 Notifier。

**理由:**

- 集中管理过滤逻辑
- Cooldown 防止同一标的短时间重复通知
- 解耦策略和通知

**后果:**

- 信号可能被过滤掉 (需要合理配置阈值)
- 当前未持久化被过滤的信号 (待改进，见 M1 路线图)

---

*报告生成时间: 2024-12-30*
*报告更新时间: 2024-12-30 (新增安全性分析、开发指南、路线图、ADR)*
