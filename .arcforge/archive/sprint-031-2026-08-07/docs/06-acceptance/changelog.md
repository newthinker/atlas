# Changelog — collector policy gate

> 面向读代码的人。不含任务编号与返工历史；每条断言标注可核实的位置（文件:行 或 commit sha），
> 行号为落稿时现读值。

## 新增：`internal/collector/policy`

collector 的限流／缓存／请求合并／配额统一收口到一个中介层。调用方看得见的行为：

- **`policy.Fetch[T](gate, topic, key, fn)`** —— 带 TTL 缓存与 singleflight 合并地执行 `fn`。
  命中缓存时**不重新执行 `fn`**，也**不复制返回值**（多个调用方拿到同一个值，切片/映射类型需
  自行复制）。失败**不写缓存**（`gate.go:175`）。
- **`(*Gate).Wait(topic)`** —— 只施加限流域节流，供 `fn` 内部的重试循环复用同一闸门。
- **`(*Gate).Do(topic, key, fn)`** —— 强制不缓存地执行一次副作用；**仍受 singleflight 合并**
  （内置表所有主题 `Coalesce` 均为 true，并发的 `Do` 只会真正发生一次）。
- **跨进程配额账本**（`quota_file.go`）—— flock + 原子写 + 自然日窗口对齐；账本自身异常时
  **fail-open**（放行并告警），不阻断取数。
- 限流域按主题名第一段划分，同域共享一把闸门；**节流不跨域阻塞**。

溯源：`73b724b`（Gate 主体）、`837542f`（配额接口 + 内存实现）、`1ca7a7d`（跨进程账本）、
`3e75dc8`/`b197394`（进程内单例 + config 覆盖）。

## 行为变更（七家 collector）

**共同点：同参数的重复调用不再重复发 HTTP。** 缓存键统一为 `symbol|start|end|interval`，
其中时间**截断到分钟**——上层普遍以 `time.Now()` 作为 end，不截断则每次调用键都不同、
缓存永不命中（五处键：`baostock/collector.go:46`、`crypto/crypto.go:180`、
`eastmoney/eastmoney.go:436`、`yahoo/eps.go:49`、`yahoo/yahoo.go:312`）。

| collector | 变化 |
|---|---|
| **lixinger** | **此前完全没有缓存**，现获得 5 分钟 TTL（`b0bc78a`） |
| **eastmoney / crypto / baostock** | 恢复 TTL 缓存 —— 见下方「删除」一节（`b238221` / `ee18812` / `fb29d71`） |
| **yahoo** | 500ms 限流从 client 内部迁入策略表；chart/eps 获得缓存，**quote 保持实时不缓存**（`1c08953`） |
| **twelvedata** | 8s 闸门迁入策略表（`ec149a0`） |
| **tushare** | 200ms 闸门迁入策略表；`daily_basic` 的 5 次/自然日配额改为**撞墙前拦截**——配额耗尽时不再发出那次注定失败的请求（`5a57ff8`） |

**policy 错误不外泄给上层**，两种口径并存且各有依据：

- **本包已有语义相符的哨兵错误 → 映射成它**。`tushare` 把配额耗尽映射为既有的 `ErrRateLimited`
  （`tushare/client.go:99`），因为下游降级链靠 `errors.Is(err, tushare.ErrRateLimited)` 分叉。
- **本包没有哨兵错误 → 加本包前缀并断开错误链**（用 `%v` 而非 `%w`）。
  `crypto/crypto.go:159`、`baostock/collector.go:70`、`eastmoney`、`lixinger`、`yahoo`、
  `twelvedata` 走这条。
- 两种口径都保证 `errors.Is(err, policy.ErrQuotaExceeded)`／`policy.ErrTimeout` 在调用方侧为 false。
  ⚠ **用 `%w` 包一层前缀不满足这条**——消息变了但错误链还在。

## 删除：`CachedCollector` + `maybeCache`

OHLCV 缓存装饰器已删除，缓存改由 Gate 承接（`2a77803`）。

**调用方需要知道**：该装饰器原本覆盖 yahoo/eastmoney/crypto/tushare/baostock **五家**，
而 Gate 内置表首版只登记了 yahoo/tushare ⇒ **eastmoney/crypto/baostock 一度丢失
`FetchHistory` 的 TTL 缓存**（eastmoney 影响最大，它在 HTTP 接口路径上、每次请求直打上游）。
现已由内置表补登记恢复（`policy/policy.go:99-101`）。

## 配置面

```yaml
collector:
  cache:
    enabled: true          # false ⇒ 只清 TTL，限流与配额不受影响
    ttl: 5m                # 施加到**本来就缓存**的主题；TTL 已为 0 的（如 yahoo.quote）不被提升
  quota:
    path: data/collector-quota.json   # 跨进程配额账本
  topics:
    "yahoo.chart":         # 主题名含点，注意加引号
      ttl: 0               # 0 是合法值（显式关掉该主题缓存），不是「未设置」
      min_interval: 500ms
      timeout: 0
      coalesce: true
      quota_limit: 5
      quota_window: 24h
```

内置策略（`policy/policy.go:66-`）：yahoo.chart/eps 5min+500ms、yahoo.quote 不缓存+500ms、
tushare 四个接口 5min+200ms（`daily_basic` 另有 5 次/自然日配额）、twelvedata.time_series
5min+8s、lixinger/eastmoney/crypto/baostock 通配 5min 仅缓存无限流。

**配错会怎样**：

- **给 `yahoo.quote` 设 TTL 会触发一个真实缺陷**：`core.Quote` 含 `FundInfo *FundInfo` 指针字段，
  而 `yahoo/yahoo.go:246` 的 `out := *q` 是结构体浅拷贝、复制的是指针 ⇒ 多个调用方共享同一个
  `*FundInfo`。**今天不触发的唯一原因就是该主题内置 `TTL: 0`。**
- **`quota_limit: 0` 表示「不设上限」，不是「禁止调用」**（`policy/quota.go:57` 的判定是
  `q.Limit > 0 && …`）。
- 主题名的**域段**（第一段）写错会让查表落空 ⇒ 闸门直通、缓存彻底失效**且不报任何错**；
  接口段在通配登记下写成什么都能命中。

## 已知限制

1. **回退路径的 TTL 叠加**：eastmoney 的 `FetchHistory` 在被缓存的取数函数内部回退到 lixinger
   （`eastmoney/eastmoney.go:479-481`），而 lixinger 自身也经 Gate ⇒ 两层各 5 分钟，
   **该路径最坏 ~10 分钟陈旧**。不死锁（缓存键含主题段，singleflight 不自我阻塞）。
   只此一家——crypto 的三个 provider 是裸 HTTP、单层。
2. **`Gate.Do` 目前零生产调用方**（仅 policy 包内有定义 `gate.go:120`）。它的 singleflight
   合并语义对「必须每次都发生」的副作用不适用，接入前请确认。
3. **`policy.Fetch` 命中缓存时不复制返回值**。切片类型需调用方 `slices.Clone`；
   **元素含 map/slice/指针字段时 `slices.Clone` 不够**，须逐元素深拷贝
   （`core.OHLCV` 是纯值类型故足够，定义处 `core/types.go:68` 有约束注释）。

## 一条提交信息与实际改动不符（存档备注）

`ce73488` 标 `test(collector): policy 补齐 error_handling…`，但**改了生产文件**
`internal/collector/policy/policy.go`：把写死的 `shanghai()` 拆成参数化的 `loadLoc(name)`，
`shanghai()` 改为 `return loadLoc("Asia/Shanghai")`。

**逐行核过 diff：返回值与「加载失败退回 UTC」的行为完全未变**，改动纯粹是为了让失败分支可达
（写死名字时那个分支不可达，改成 `panic(err)` 都没有测试转红）。

⇒ **行为零变更，故不在上述任何一节里**。记在这里是因为 `test(` 前缀盖住了「触碰生产代码」
这件事——将来做 `git log --grep` 类审计会漏掉它。
