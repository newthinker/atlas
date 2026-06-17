# Telegram 信号汇总表格（按动作分组）设计

> 日期：2026-06-17
> 目标：把一轮分析产生的多条信号，从「逐条多消息」改为**一条按动作分组的等宽表格 digest**

## 一、动机

当前 `router.Route` 对每条放行信号立即 `registry.NotifyAll` → telegram 逐条发，一轮分析
（50 标的）可一次性刷出几十条消息。希望改为**每轮一条汇总表格**：

- Telegram 不渲染真表格（markdown 管道表 / HTML `<table>` 均不识别），唯一可行形态是
  **等宽代码块 + 空格对齐列**。
- 按动作分组（买入 / 卖出 /（如有）持有），组内按置信度降序。

## 二、关键约束（来自现有架构）

- `router.Route(sig)` 返回 `routed`，驱动**执行下单**（`executor.SubmitSignal`）+ 冷却 +
  信号存储 → **路由决策必须保持逐信号**，只把**通知**聚合。
- `runAnalysisCycle`（`internal/app/app.go:327`）用 errgroup 并行跑 `analyzeSymbol`，
  `g.Wait()` 是干净的「一轮结束」边界。`RunOnce → runAnalysisCycle`（app.go:619-621），
  故 flush 落在 `runAnalysisCycle` 末尾即覆盖 ticker 循环与 `/analysis/run` 触发两条路径。
- `registry.NotifyAllBatch(signals)` 已存在（对每个 notifier 调 `SendBatch`）；
  telegram/email/webhook 均已实现 `SendBatch`。
- 冷却（`cooldown_hours` 默认 4h）使每轮 digest 只含**新放行**信号，不会每轮重复同样的行。

## 三、架构（路由层「延迟通知缓冲 + cycle 末尾 flush」）

```
analyzeSymbol(并行)
  └ router.Route(sig)   # 过滤/步进门/冷却/存储/执行 —— 逐信号不变
       └ batchNotify 时: 已放行信号 append 到 router.pending（加锁）  ← 不再即时 NotifyAll
runAnalysisCycle: g.Wait() 之后
  └ router.FlushNotifications()
       └ 取出 pending → registry.NotifyAllBatch(batch) 一次
            └ telegram.SendBatch → 按动作分组等宽表格
```

**为什么这样切**：执行/冷却/存储语义零变化（仍逐信号），只有「通知时机」从逐条改为
每轮一次；与 cycle 边界精确对齐。被否方案：App 层收集（要拆 Route 通知职责，改契约更大）、
通知器定时缓冲（与 cycle 对不齐，易重复/漏发）。

### 组件改动

- `internal/router/router.go`
  - 新增字段：`pending []core.Signal`、`pendingMu sync.Mutex`、`batchNotify bool`。
  - `Route`：通过过滤/门/冷却后，原 `NotifyAll` 改为——`batchNotify` 为真则加锁 append 到
    `pending`（仍 `return true`）；为假维持立即 `NotifyAll`（保留旧行为，可回退）。
    信号存储、`routed` 返回、冷却记录均不变。
  - 新增 `FlushNotifications()`：加锁把 `pending` 换出（置空）→ 为空则直接返回（不发）→
    否则 `registry.NotifyAllBatch(batch)`，记 `digest sent count=N errors=...`。
- `internal/app/app.go`
  - `runAnalysisCycle` 在 `_ = g.Wait()` 之后调用 `a.router.FlushNotifications()`。
- `internal/notifier/telegram/telegram.go`
  - 重写 `SendBatch`：按动作分组渲染等宽表格（见 §四）。`Send`（单条）保持不变。
- `internal/notifier/telegram/width.go`（新增）
  - 显示宽度工具：`displayWidth(string) int`、`padRight(string, width) string`，CJK 等宽
    字符记 2 列。**不引第三方依赖**，内置最小 `isWide(rune)` 覆盖常见 CJK 区间
    （CJK 统一表意 `0x4E00-0x9FFF`、扩展、全角 `0xFF00-0xFF60`、CJK 符号 `0x3000-0x303F` 等）。
- `internal/config/config.go`
  - `RouterConfig` 新增 `BatchNotify bool mapstructure:"batch_notify"`，默认 `true`（digest）。
    serve 装配 router 时传入；`false` 回退逐条即时发。

## 四、表格格式

`parse_mode: "Markdown"`。标题行在代码块外（emoji 正常渲染），每个动作组一个 ``` 代码块：

```
📊 Atlas 信号汇总 · 2026-06-17 14:43 · 12 条

📈 买入
SYMBOL     NAME      CONF   PRICE
600519.SH  贵州茅台   94.7%  1240.92
000858.SZ  五粮液     95.0%  168.30

📉 卖出
SYMBOL     NAME      CONF   PRICE
AAPL       苹果       93.4%  299.24
^GSPC      标普500    93.9%  7511.35
```

- 分组：买入 = `strong_buy`+`buy`；卖出 = `strong_sell`+`sell`；持有 = `hold`（仅在出现时）。
- 组内按 `Confidence` 降序。
- 列：`SYMBOL`(原始 atlas 符号) / `NAME`(`Metadata["name"]`，空留白) / `CONF`(`%.1f%%`) /
  `PRICE`(`%.2f`，`Price<=0` 留空)。列宽 = 该列各行显示宽度的最大值 + 2 空格，按显示宽度补齐。
- 列名用英文（避免表头中文宽度复杂化；数据行中文按显示宽度对齐）。
- `SendBatch(nil/空)` → 返回 nil（不发）。

## 五、边界与降级

- 空轮（pending 为空）→ `FlushNotifications` 不发消息。
- 单条信号 → 1 行表（可接受）。
- telegram 投递失败 → 仍按现有 `notifier failed` 错误日志降级，不影响执行/其他 notifier。
- 其他 notifier（email/webhook，当前均 `enabled:false`）→ 经 `NotifyAllBatch` 走各自既有
  `SendBatch`，本设计不改其格式。
- `batch_notify:false` → 完全回到当前逐条即时发行为（回退路径）。

## 六、测试

- **router**：`batchNotify` 时 `Route` 不立即通知而入 `pending`；`FlushNotifications` 批量发且
  清空；空轮不发；`batchNotify:false` 时维持逐条 `NotifyAll`（零回归）。用 fake notifier 断言。
- **telegram `SendBatch`**：按动作分组、组内降序、含中文名的列对齐（快照断言含 NAME 列对齐）、
  空输入不发、`Price<=0` 留空。
- **width 工具**：`displayWidth`/`padRight` 对 ASCII、CJK(记 2)、混排的正确性。
- **app**：`runAnalysisCycle` 末尾调用 `FlushNotifications`（编译 + 行为：一轮多信号 → 一次批量）。
- **config**：`batch_notify` 解析，缺省 = true。

## 七、不做（YAGNI）

- 不引入 runewidth 第三方库（内置最小宽度表足够覆盖 watchlist 的中文名）。
- 不做 HTML/MarkdownV2 表格尝试（Telegram 不渲染）。
- 不改 email/webhook 的批量格式。
- 不做跨轮聚合 / 自定义列配置（按动作分组固定列）。
