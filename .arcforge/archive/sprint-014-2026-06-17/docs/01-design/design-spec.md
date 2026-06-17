# 设计规格 — Telegram 信号汇总表格

## 数据流（实现后）

```
runAnalysisCycle:
  并发 analyzeSymbolSafe(item) → router.Route(sig)
      ├─ batch_notify=false: 立即 NotifyAll(sig)  （原路径）
      └─ batch_notify=true : 缓冲 pending=append(pending, sig)  （路由决策/冷却/执行仍逐信号）
  g.Wait()
  router.FlushNotifications()           ← 新增：cycle 末一次性
      └─ NotifyAllBatch(pending) → telegram.SendBatch(signals)
            └─ formatBatch(signals) → 按动作分组的等宽表格
```

## 组件分解

### 1. `internal/notifier/telegram/width.go`（新建，纯函数）
- `displayWidth(s) int` — CJK 宽字符记 2，其余记 1
- `padRight(s, width) string` — 按显示宽度右补空格
- `isWide(r) bool` — 覆盖 atlas watchlist 名称/符号涉及的东亚宽字符区段（非全 Unicode 表，避免第三方依赖）

### 2. `internal/notifier/telegram/telegram.go`（修改）
- `formatBatch(signals) string` — 标题行（时间+条数）+ 每个非空动作组一张等宽表格
- `renderTable(rows) string` — 单组的 fenced 对齐表（列：SYMBOL/NAME/CONF/PRICE，末列不补尾空格）
- 重写 `SendBatch` → 调 `formatBatch`，空串直接返回 nil

### 3. `internal/router/router.go`（修改）
- `Config.BatchNotify bool`
- `Router.pending []core.Signal`（复用现有 `mu`）
- `Route`：batch 模式只缓冲、返回 true，不通知
- `FlushNotifications()`：取出 pending、清空、`NotifyAllBatch`，空缓冲/无 registry 为 no-op

### 4. 接线（修改）
- `internal/config/config.go`：`RouterConfig.BatchNotify` + `v.SetDefault("router.batch_notify", true)`
- `internal/app/app.go`：`routerCfg.BatchNotify = cfg.Router.BatchNotify`；`runAnalysisCycle` 末 `a.router.FlushNotifications()`
- `configs/config.example.yaml`：`router.batch_notify: true` 文档化

## 关键接口约定（供并行 Dev 对齐）

- `formatBatch([]core.Signal) string`：空输入返回 `""`；分组顺序「📈 买入 / 📉 卖出 / ⏸ 持有」；组内 Confidence 降序稳定排序；标题含「N 条」。
- `(*Router).FlushNotifications()`：无返回值，错误内部 log；幂等（连续调用第二次为 no-op）。
- `Config.BatchNotify`、`RouterConfig.BatchNotify`：`mapstructure:"batch_notify"`，默认 true。

## 分组规则细节

`core.Action`（`internal/core/types.go:115-121`）：`buy`/`sell`/`hold`/`strong_buy`/`strong_sell`。
- 买入组 = `ActionStrongBuy` + `ActionBuy`
- 卖出组 = `ActionStrongSell` + `ActionSell`
- 持有组 = `ActionHold`
