# FinceptTerminal 可借鉴设计评估

- **日期**：2026-08-06
- **调研对象**：<https://github.com/Fincept-Corporation/FinceptTerminal>（v4.3.0，29.8k star）
- **对照 atlas 版本**：master @ `ea5ac30`

## 结论先行

FinceptTerminal 是 C++20 / Qt6 的**桌面金融终端**（43 个 service 模块），与 atlas 的 Go 后端 + HTMX Web 路线技术栈完全正交。**能借的是设计模型，不是代码**——法律上也只能借设计（见文末许可证警告）。

对照 atlas 现有的 `internal/collector`、`internal/meta`、`internal/storage` 实现后，筛出 4 个真正有落地价值的点，按优先级排列。

---

## 一、高价值：DataHub 的 TopicPolicy 模型

Fincept 把「一份数据该多久刷一次」抽成了一等公民的策略对象 `TopicPolicy`（`fincept-qt/src/datahub/TopicPolicy.h`）：

| 字段 | 语义 |
|---|---|
| `ttl_ms` | 缓存新鲜期 |
| `min_interval_ms` | 两次刷新的最小间隔（**与 TTL 正交**） |
| `refresh_timeout_ms` | 防止 producer 卡死 |
| `coalesce_within_ms` | 窗口内的重复刷新请求合并成一次 |
| `push_only` | 关闭调度器，由 producer 自己推 |
| `drop_on_idle` | 最后一个订阅者离开就丢弃整个 topic 状态 |

### 对照 atlas 现状

- `internal/collector/cache.go` 的 `CachedCollector` 只有单一 `ttl` + 256 条上限。
- 限频逻辑散在各 collector 内部，**三份几乎相同的 `minInterval + lastReq + mutex` throttle 重复实现**：
  - `internal/collector/tushare/client.go:48`（`minInterval` 常量）
  - `internal/collector/twelvedata/client.go:45`（`minInterval` 字段 + `throttle()`）
  - `internal/collector/yahoo/yahoo.go:66`（`minInterval` 字段 + `throttle()`）
- TTL（缓存层）与 minInterval（客户端层）分居两处、互不知情。

### 建议

把「TTL + 最小间隔 + 超时 + 合并窗口」收敛成一张**按数据主题（不是按 collector）配置**的策略表。tushare `daily_basic` 的 5 次/天限制、yahoo 的秒级 throttle、qlib 仓库的近乎无限制，本质是同一模型的不同参数，现在却是三种不同的代码形态。

其中 **`coalesce_within_ms`（请求合并）atlas 完全没有**——Web dashboard 多个面板同时触发同一 symbol 刷新时，会打满限频配额。这是当下就存在的真实问题。

## 二、高价值：pattern → producer 注册表，替代硬编码路由

Fincept 的 producer 按 **topic 通配符模式**注册，用最长前缀匹配索引查找。

### 对照 atlas 现状

`internal/collector/selector.go` 的 `SelectForSymbol` 是硬编码 if/else：

- `cryptoTickers` 是写死在源码里的 13 个币种白名单；
- `indexMarkets` 是写死的 5 个指数 map。

加一个币种要改代码、重编译、补测试。

### 建议

`Registry.Register(c)` 升级为 `Register(c, patterns...)`，路由规则外置成 `*.SH → eastmoney`、`*-USD → crypto`、`^* → yahoo` 这类配置。改动面小，直接消除「加数据源要改路由代码」的耦合。qlib 仓库的 `Covers()` 优先级可保留为显式的最高优先级 producer。

## 三、中高价值：stale-while-error + 新鲜度暴露

Fincept 把 `publish()` 和 `publish_error()` 分成两条通道：**刷新失败不污染缓存值**，订阅者拿到「旧值 + 错误标记」而非空值；同时 `age_ms()` 把数据年龄暴露给 UI 做新鲜度指示。

### 对照 atlas 现状

Prism M3.5a 的降级链解决的是「A 源失败换 B 源」，但没有「B 源也失败时，回退到上次成功值并标注 stale」这一层。`internal/storage/prism/sqlite.go` 存的是值，没有存「`fetched_at` + `source` + `stale`」三元组供上层判断。

### 建议

给持久化的行情/基本面数据加上「数据年龄 + 来源 + 是否降级得来」的元数据，Web 面板显示成「PE 12.3（tushare，2 天前，降级）」而不是无上下文的数字。对一个决策系统而言，这是可信度问题，不是 UI 装饰。

## 四、中价值：LLM meta 层的 request_id

`fincept-qt/src/services/agents/AgentTypes.h` 里两个细节：

1. **`request_id` 防串扰**：每次 agent 调用带 request_id，异步结果回来时校验，避免旧请求结果覆盖新请求。`internal/meta/arbitrator.go` 若被并发调用（多 symbol 同时仲裁），这是真实的坑。**建议采纳。**
2. **`RoutingResult` 带 `confidence` + `matched_keywords`**（路由决策自带可审计依据）：atlas 的 `ArbitrationResult` 已有 `Confidence` + `Reasoning` + `WeightedFrom`，**这块 atlas 不比它差，无需改动。**

另外 `ExecutionPlan` / `PlanStep`（`dependencies` + `status` 的 DAG）与 arcforge 任务图（`dependencies` + `wave` + status）是同一模式的独立收敛——说明该建模方向正确，可考虑下沉到 signal pipeline 编排。

## 五、可选：DataNormalizationService 独立成层

Fincept 有独立的 `services/data_normalization`（`DataNormalizationService` + `DataMappingTestClient`——映射的**可测试**客户端）。

atlas 目前每个 collector 自己把外部 JSON 转成 `core.OHLCV` / `core.Quote`，复权口径、时区、停牌、货币的处理散落在 9 个 collector 目录里。多源覆盖同一标的时（qlib / eastmoney / tushare 都能给 A 股日线），口径一致性没有集中的契约测试保障。

属于重构性质改动，收益取决于是否已踩过多源口径不一致的坑。**优先级低于前三项。**

---

## 明确不建议借鉴

| 项 | 原因 |
|---|---|
| Qt6/C++ 桌面 + 嵌入 Python 解释器 | 与 atlas 路线正交；已有 `scripts/qlib_eval` venv 桥接，成本低得多 |
| 37 个「投资大师人格」AI agent | 噱头性质；atlas 的 arbitrator/synthesizer 是更正经的建模 |
| 依赖其任何组件 | 该项目自 2026-06 起转为「月度更新」，核心团队已转向付费私有版与新项目 Quantcept |

## ⚠️ 许可证风险（必读）

FinceptTerminal 采用 **AGPL-3.0 + 商业许可双轨**，README 明确声明：**任何商业使用需付费许可，包括 fork 和「替换 API 的改写」，并会主动监控未授权使用、起诉索赔**。

因此：**只能借鉴架构思路，不能复制、改写或「参考着重写」其任何代码**。本文所有建议均刻意只描述设计模型（TTL / 最小间隔 / 合并窗口这类概念本身不受版权保护），未引用任何实现。若 atlas 有商业化打算，建议连其头文件都不要放进任何 AI 编码上下文。

## 建议的落地顺序

1. **统一限流 / 缓存策略层**（第一项）——同时消除三份重复 throttle 代码 + 补上缺失的请求合并。
2. pattern 化 collector 路由（第二项）——改动面小，解耦收益直接。
3. 数据新鲜度元数据（第三项）——需配合 storage schema 变更。
4. meta 层 request_id（第四项）——小补丁，可随手做。
