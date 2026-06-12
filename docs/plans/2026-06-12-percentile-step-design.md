# Router percentile_step（百分位步进提醒）— 设计文档

> 日期：2026-06-12
> 状态：设计已确认（用户批准）；rev4 — 按用户需求将步长下沉为策略级可配（params.percentile_step 经 Metadata 传递，全局值降为回退默认），方案 A 经用户确认
> 需求来源：百分位监控规则——低于 50 分位开始买入提醒，**此后每再跌 5 个历史百分位（45/40/35…）再次提醒**；高于 80 分位卖出提醒，对称地每涨 5 个百分位再提醒。现有 router 仅有按标的时间冷却，无法表达「按分位步进」语义。
> 关联配置：`configs/percentile-watchlist.yaml`（已预留 `percentile_step: 5` 注释行）

## 1. 已确认的设计决定

| 决定点 | 结论 |
|---|---|
| 步进语义 | 历史百分位点（非价格百分比）；对称规则 `|当前分位 − 上次通知分位| ≥ step` 即放行 |
| 状态持久化 | 内存态，重启清零（重启后首个越线信号重新提醒一次，视为状态确认；与 cooldown 同语义） |
| 与时间冷却交互 | **完全替代**：分位信号不受 cooldown 约束、也不更新按标的冷却戳（不压制同标的其它策略） |
| 逻辑落点 | Router 内置门控（方案一）；策略保持无状态，通知去重职责与 cooldown 同居 router |
| **步长粒度（rev4 变更，用户确认）** | **策略级可配**：`strategies.{name}.params.percentile_step` 经 `Signal.Metadata["percentile_step"]` 传递，router 门控优先用信号自带步长，缺失/无效时回退全局 `router.percentile_step`。watchlist 资产经绑定的策略获得不同步长（如价格分位 5、PE 分位 3） |
| 范围边界 | 不做状态持久化、不做最小冷却叠加、不做按标的独立 step（粒度到策略为止） |

## 2. 配置与状态

- **配置**：`router.percentile_step`（float64，默认 0 = 禁用，向后完全兼容）
  - `internal/config/config.go` 的 `RouterConfig` 增加 `PercentileStep float64 \`mapstructure:"percentile_step"\``
  - `internal/router/router.go` 的 `Config` 增加 `PercentileStep float64`
  - **装配点（spec 审查纠正）**：router.Config 由 `internal/app/app.go` 的 `app.New()` 构造，当前为**硬编码**（MinConfidence 0.5、CooldownDuration 1h）——`cfg.Router` 的 `cooldown_hours`/`min_confidence` 是从未生效的死配置（预存 bug）。本功能必须把 `app.New()` 改为从 `cfg.Router` 映射构造（CooldownHours→Duration、MinConfidence、PercentileStep），**顺带修复该预存 bug**（否则 §7 的「cooldown_hours: 24 约束 ma_crossover」同样落空，实际恒为 1h）。修复属于本设计范围，需有配置生效的回归测试。
  - **存量行为变更注记（发布说明用）**：config 默认值为 `cooldown_hours: 4`、`min_confidence: 0.6`，而硬编码生效值是 1h / 0.5——修复接线后，未显式配置的部署冷却 1h→4h、置信阈值 0.5→0.6，此为修复本意的可见变更
  - `cooldown_hours: 0` 约定为禁用冷却（恒放行），与 `percentile_step: 0 = 禁用` 风格对齐
  - `EnabledActions` 维持 `app.New()` 现有硬编码，config 无对应字段，不在本设计范围（YAGNI）
- **状态**：`map[string]float64`，key = `symbol|strategy|side`
  - side ∈ {buy, sell}：`buy`/`strong_buy` 归 buy 侧，`sell`/`strong_sell` 归 sell 侧（同侧不同 action 共享 key，按分位距离判定与档位无关）
  - 与 `cooldowns` 共用现有 `r.mu` 锁
- **分位提取**：依序尝试 `Signal.Metadata["percentile"]`（price_percentile）、`Metadata["pe_percentile"]`（pe_percentile），取第一个存在且断言为 float64 成功的值；均不存在/类型不符 → 该信号不适用步进门控
  - `.(float64)` 断言安全的依据：信号从 strategy → app → router 全程内存传递，signalStore 只写不回读再路由，不存在 JSON 反序列化边界；若未来引入「从存储重放信号」路径需重新评估此假设
- **策略级步长（rev4）**：
  - `price_percentile` / `pe_percentile` 的 params 增加可选 `percentile_step`（双形态 int/float64 读取，≤0 视为未配置）；策略持有该值并在 `Analyze` 产出信号时写入 `Metadata["percentile_step"]`（float64，仅在 >0 时写入）
  - router 的**有效步长**取值顺序：`Metadata["percentile_step"]`（存在且 float64 且 >0）→ 全局 `router.percentile_step` → 两者皆无效则该信号走冷却路径
  - 门控启用条件相应改为：信号带分位元数据 **且 有效步长 > 0**

## 3. 判定规则（对称式，防死锁）

```
首次（无记录）                  → 放行，记录当前分位
|当前分位 − 上次通知分位| ≥ step → 放行，更新记录
否则                            → 抑制（Route 返回 routed=false）
```

单条规则覆盖三种场景：

1. 买入侧继续下跌：50 通知 → 47 抑制（|47−50|<5）→ 44 通知（|44−50|=6≥5）
2. 卖出侧继续上涨：81 通知 → 83 抑制 → 86 通知
3. **行情恢复后的新一轮（防死锁）**：上次跌至 35 分位通知后反弹到 60（区间内无信号），再跌回 49：|49−35|=14 ≥ 5 → 放行并以 49 重新起算。无需单独的「重置」分支。

## 4. Route 分流

- `passesFilters` 重构拆分：confidence/action 过滤保留为通用前置；冷却检查拆为独立 `passesCooldown`
- `Route()` 分流：
  - 信号带分位元数据 且 `PercentileStep > 0` → 步进判定；通过则通知并更新步进状态，**不查、不更新**冷却戳
  - 否则 → 原有路径（冷却检查 → 通知 → 更新冷却戳），行为零变化
- `RouteBatch` 复用同一判定与状态更新函数，防旁路。注（spec 审查核实）：RouteBatch 当前**无生产调用方**（仅测试引用），分位信号实际只走 `Route()` 单发路径——改造按最小成本做：批内逐条顺序判定（同 key 第一条放行并更新状态后，第二条按更新后的状态判定，与连续调用 Route 等价）；RouteBatch 与 Route 现存的其它不对等（不写 signalStore、先更新冷却后通知）维持现状，不在本设计范围
- **判定与状态更新的原子性**：步进判定与状态写入在同一 `r.mu.Lock()` 临界区内完成（不照搬现有冷却的 RLock 读 + Lock 写两段式；app 侧 Route 虽为单 goroutine 串行调用，仍以单临界区写法为准）
- `GetStats` 增加 `percentile_gates_active`（状态条目数）与 `percentile_step` 回显
- `ClearAllCooldowns` 同步清空步进状态（手动重置 = 全部重置）；`ClearCooldown(symbol)` 按 `symbol+"|"` 前缀遍历删除该标的的全部步进 key（隐含假设：symbol 不含 `|`，当前所有 watchlist 符号形态均满足）

## 5. 边界与错误处理

| 场景 | 行为 |
|---|---|
| step = 0 / 未配置 | 所有信号走原冷却路径，与现状完全一致 |
| 分位元数据存在但类型非 float64 | 视为无元数据 → 冷却路径，debug 日志说明 |
| 同标的绑定两个分位策略 | 各自独立 key 互不干扰（price 49 分位的通知不影响 pe 52 分位的判定） |
| strong_buy 之后的 buy（同侧） | 共享 buy 侧 key，按分位距离判定 |
| 状态规模 | watchlist 量级（几十标的 × ≤2 策略 × 2 方向），无需清理例程；重启清零 |

## 6. 测试（第 1-8 条 `internal/router/router_test.go`；第 9 条 `internal/app/app_test.go`，表驱动、无外部依赖）

1. 买入侧步进序列：49 放行 → 47 抑制 → 44 放行 → 46 抑制
2. 恢复重算：last=44 时 49 放行（|49−44|≥5）并更新记录为 49
3. 卖出侧对称：81 放行 → 83 抑制 → 86 放行
4. buy/sell 侧独立；双分位策略（不同 strategy）key 独立
5. 无分位元数据 / step=0 → 冷却行为回归（既有用例不回归）
6. 分位信号不更新冷却戳：分位通知后，同标的无元数据信号仍按自身冷却判定放行
7. `ClearAllCooldowns` / `ClearCooldown(symbol)` 后首个分位信号重新放行
8. 元数据类型异常（如 string）→ 走冷却路径不 panic
9. **配置接线回归（修复预存死配置 bug）**：`app.New()` 用含自定义 `cooldown_hours`/`min_confidence`/`percentile_step` 的 cfg 构造后，router 实际行为反映配置值（而非硬编码 1h/0.5）
10. **策略级步长（rev4）**：信号自带 `percentile_step: 3` 时按 3 门控（覆盖全局 5）；信号无 step 元数据时回退全局值；两策略不同步长互不干扰；策略 Init 读取 params 并在信号中携带的端到端用例（策略包内测试）

## 7. 交付后的配置变更

`configs/percentile-watchlist.yaml`：两个分位策略的 params 增加 `percentile_step: 5`（可独立调整，如 PE 分位改 3）；router 节 `percentile_step: 5` 取消注释（作为未配置策略的全局默认）；`cooldown_hours` 仅约束无分位元数据的策略（如 ma_crossover）。`configs/config.example.yaml` 的 strategies 与 router 节同步补充参数及注释。
