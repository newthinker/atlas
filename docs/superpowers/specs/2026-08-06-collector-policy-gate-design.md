# Collector 策略闸门与路由表 —— 设计文档

- **日期**：2026-08-06
- **来源**：`docs/reviews/2026-08-06-fincept-terminal-borrowable-design.md` 第 ① ② 项
- **范围**：`internal/collector` 域。第 ③（数据新鲜度元数据）与第 ④（meta 层 request_id）不在本次范围，各自后续单独走 brainstorm → spec → plan
- **基线**：master @ `ea5ac30`

## 1. 问题陈述

### 1.1 限流逻辑三处重复，且与缓存分居

`mu + lastReq + minInterval + sleep` 这段节流闸门在三处逐字重复：

- `internal/collector/tushare/client.go:48`（200ms 常量）
- `internal/collector/twelvedata/client.go:45`（8s，对应免费层 8 req/min）
- `internal/collector/yahoo/yahoo.go:66`（500ms）

三者**并非完全等价**：`yahoo.go:116-175` 在闸门之上还叠了重试预算、`Retry-After` 解析与指数退避。可抽取的是闸门本身，不是整套重试策略。

同时，缓存 TTL 在 `internal/collector/cache.go`（`CachedCollector`），限流在各 client 内部，两者互不知情。

### 1.2 已实测的配额未落地

`ea5ac30` 已实测记录 tushare `daily_basic` 为 **5 次/天**，但代码中只有 200ms 的 minInterval 兜底，日配额从未实现。

现状是**撞墙后降级**：`tushare/client.go:33` 定义 `ErrRateLimited`，`internal/prism/refresh.go:450` 据此走降级链。每次撞墙浪费一次注定失败的 API 调用。

### 1.3 装饰器模式已在付出代价

`cmd/atlas/serve.go:434-448` 的 `maybeCache` 显式跳过实现了扩展接口的 collector：`CachedCollector` 只嵌入 `collector.Collector`，包装后会遮蔽 `FundamentalCollector` 等扩展方法、打断 type assertion 路径。**结果是 lixinger 完全享受不到缓存。**

若继续以装饰器方式叠加 throttle / quota / coalesce，每加一层就多一次接口遮蔽，会让越来越多 collector 掉出策略层。

### 1.4 符号路由规则散落且双用途

`internal/collector/selector.go` 的五个谓词（`isAShareSymbol` / `isCryptoSymbol` / `isIndexSymbol` / `isCommoditySymbol` / `IsAShareIndex`）同时驱动 `SelectForSymbol`（选 collector）与 `MarketForSymbol`（推断市场）。`cryptoTickers` 是硬编码的 13 项白名单，加币种需改代码。

### 1.5 运行形态约束

atlas 有两种运行形态：

- `atlas serve` —— 长驻，`internal/app/app.go:302` 的 ticker 周期分析，`app.go:355-380` 以 `errgroup.SetLimit(workers)` **并发**处理符号
- `atlas prism refresh` —— `cmd/atlas/prism.go:31` 标注为 launchd 入口，**短命进程**，跑完即退

因此**内存中的配额计数在 launchd 形态下完全无效**（每次启动归零）。配额要生效必须跨进程持久化。

## 2. 方案选择

| 方案 | 结论 |
|---|---|
| A. 继续叠装饰器 | **否决**。直接放大 §1.3 的缺陷；且 `FetchQuote` 需限流不需缓存，装饰器无法对不同方法差异化施策 |
| B. Policy Gate 中介层 | **采纳** |
| C. 只抽取共用 throttle 工具函数 | 风险最低，但交付不了 quota 与 coalesce |

方案 B 同时解决三件事：消除重复、让 TTL 与限流同居、并修掉 lixinger 掉出缓存层的既有缺陷。

## 3. 架构

### 3.1 包结构

新包 `internal/collector/policy`，**不依赖** `internal/collector`（避免循环），仅依赖标准库与 `golang.org/x/sync`（`go.mod:18` 已有，`singleflight` 零新增依赖）。

```
internal/collector/policy/
  policy.go     # Policy / Quota 结构，主题策略表与查表
  gate.go       # Gate：准入控制 + TTL，唯一对外入口
  quota_file.go # QuotaStore 的 JSON 文件实现（跨进程）
  quota_mem.go  # QuotaStore 的内存实现（测试用）
```

### 3.2 调用位置

Gate **不包装 collector**，而是由各 collector 在**发 HTTP 请求处**调用。这是与装饰器方案的本质区别，也是接口不被遮蔽的原因。

Gate 暴露两个入口。Go **不支持泛型方法**，因此带返回值的那个必须是包级泛型函数而非 `Gate` 的方法：

```go
// 无返回值的准入控制（供不需要缓存结果的调用点）
func (g *Gate) Do(topic, key string, fn func() error) error

// 带 TTL 缓存的准入控制（包级泛型函数，非方法）
func Fetch[T any](g *Gate, topic, key string, fn func() (T, error)) (T, error)
```

两者共用同一套策略与内部状态，`Do` 等价于 `Fetch[struct{}]` 且强制 TTL=0。执行链：

```
policy.Fetch[T](g, topic, key, fn) →
    ① 查 TTL 缓存（命中即返回）
    ② singleflight 合并同 key 在途请求
    ③ quota 预判（超额 → ErrQuotaExceeded，不发请求）
    ④ throttle 等待到限流域的最小间隔
    ⑤ 执行 fn（yahoo 的重试/退避层留在 fn 内部，不动）
```

### 3.3 限流域与主题是两个维度

`yahoo.chart` 与 `yahoo.eps` 是两个主题（TTL 可不同），但共享同一个 500ms 闸门（同一服务端）。因此 Gate 内部维护两张表：

- `topics` —— TTL / quota / coalesce，按主题
- `domains` —— throttle，按限流域

`Policy.Domain` 缺省取主题名第一段。

### 3.4 组件职责

| 组件 | 职责 | 依赖 |
|---|---|---|
| `Policy` 表 | 纯数据：主题 → `{TTL, MinInterval, Coalesce, Quota, Timeout}`，内置默认 + config 追加 | 无 |
| `Gate` | 按策略施加准入控制与缓存，进程内单例，`Init` 时注入各 collector | `QuotaStore` |
| `QuotaStore` | 跨进程配额计数，`Take(topic, limit, window, now) (bool, error)` | 无 |

### 3.5 `CachedCollector` 删除

要真正修掉 §1.3，缓存必须下沉到 collector 内部（HTTP 调用处）。因此删除 `internal/collector/cache.go` 的 `CachedCollector` 与 `cmd/atlas/serve.go:434` 的 `maybeCache`。

**影响**：缓存粒度从「每次 `FetchHistory` 调用」变为「每次 HTTP 调用」。对 yahoo 二者等价（一次 `FetchHistory` = 一次 HTTP）；对 tushare 变细（多跳各自缓存），是改进。

## 4. 策略表

### 4.1 默认值原则

**未登记的主题 = 零策略**（不缓存、不限流、不计配额）。

`eastmoney` / `akshare` / `lixinger` / `crypto` / `fred` / `edgar` / `baostock` 当前均无任何节流，默认不进策略层，行为零变更。为它们加限流是后续的一次 config 改动，不属本次范围。

### 4.2 内置默认表

数值全部从现有常量平移，不做调整：

| 主题 | 限流域 | MinInterval | Quota | 来源 |
|---|---|---|---|---|
| `yahoo.chart` | `yahoo` | 500ms | — | `yahoo.go:49` |
| `yahoo.eps` | `yahoo` | 500ms | — | 同域共享闸门 |
| `tushare.daily_basic` | `tushare` | 200ms | **5 / 自然日** | `ea5ac30` 实测，**新增** |
| `tushare.daily` | `tushare` | 200ms | — | `client.go:48` |
| `tushare.index_daily` | `tushare` | 200ms | — | 同上 |
| `tushare.hk_daily` | `tushare` | 200ms | — | 同上 |
| `twelvedata.time_series` | `twelvedata` | 8s | — | `client.go:33` |

- TTL 沿用现有 `Collector.Cache.TTL` 配置值（`internal/config/config.go:303`，默认 5m），作为 OHLCV 类主题默认
- 现有的 `Collector.Cache.Enabled` 开关保留语义：为 `false` 时**所有主题的 TTL 强制归零**（等价于今天 `maybeCache` 直接返回原 collector），限流与配额不受影响
- `FetchQuote` 类主题 TTL = 0（保持「实时不缓存」的现有语义）
- `Coalesce` 对所有登记主题默认开启（只合并在途请求，不改变可观测结果）

### 4.3 结构定义

```go
type Policy struct {
    Domain      string        // 限流域，缺省 = 主题名第一段
    TTL         time.Duration // 0 = 不缓存
    MinInterval time.Duration
    Coalesce    bool
    Quota       *Quota        // nil = 不计配额
    Timeout     time.Duration
}

type Quota struct {
    Limit  int
    Window time.Duration   // 24h / 1m
    Loc    *time.Location  // 自然边界对齐时区，默认 Asia/Shanghai
}
```

config.yaml 以主题名为 key 追加或覆盖单个字段，内置表兜底。

### 4.4 QuotaStore 文件实现

```json
{
  "tushare.daily_basic": { "window_start": "2026-08-06T00:00:00+08:00", "count": 3 }
}
```

- 路径：`<data_dir>/collector-quota.json`，可配置
- `Take()` 全程持 `syscall.Flock` 排他锁，锁内完成「读 → 判窗口是否翻篇 → 计数 → 原子写（temp + rename）」
- 窗口翻篇判定：`now` 所属自然边界 ≠ `window_start` → 计数归零
- 文件损坏 / 不可读 / 加锁失败 → **fail-open**（放行 + 告警），不因账本损坏阻断降级链

**计数时机**：被 Gate 预判拦下的请求不计数（未发出）；实际发出的请求无论成败都计数（服务端已计）。

## 5. 错误处理与兼容性

**核心目标：`internal/prism/refresh.go` 一行不改。**

### 5.1 错误映射

`refresh.go:450` 靠 `errors.Is(err, tushare.ErrRateLimited)` 决定降级。配额预判产生的 `policy.ErrQuotaExceeded` 语义与之一致（临时性、窗口过后自愈），在 collector 内部完成映射，`policy` 包错误不外泄到 `prism` 层：

```go
if errors.Is(err, policy.ErrQuotaExceeded) {
    return nil, fmt.Errorf("%w: %s (本地配额预判)", ErrRateLimited, apiName)
}
```

降级链行为不变，仅从「撞墙后降级」提前为「撞墙前降级」。

### 5.2 各错误路径

| 情形 | 处理 |
|---|---|
| `fn` 返回错误 | 不写缓存、不延长 TTL；**计入配额** |
| 配额预判拦截 | 不计数，返回 `ErrQuotaExceeded` |
| `Timeout` 超时 | 作用于 `fn` 本身，返回 `ErrTimeout`；不写缓存 |
| QuotaStore 异常 | fail-open：放行 + `logger.Warn` |
| coalesce 后 `fn` 失败 | 所有等待者共享同一错误（`singleflight` 固有语义） |

### 5.3 唯一的可观测行为变化

今天 3 个 goroutine 同时请求同一 key 会各发一次请求，可能一成两败；改造后合并为一次，结果统一。错误不缓存，下次调用会重新发起。

判断：可接受。「同一时刻对同一数据的三次请求得到三种不同结果」本身就是应当消除的。

### 5.4 已知限制（登记为后续任务，不在本次范围）

`Collector` 接口方法签名不带 `context.Context`（`interface.go`），因此 Gate 在 throttle 等待期间**无法响应取消**——与 `yahoo.go:139` 现有的持锁 sleep 行为一致。

给接口加 `ctx` 会波及全部 9 个 collector 及 `app`、`prism` 的调用点，远超本次范围。

## 6. 路由表

### 6.1 公开 API 不变

`SelectForSymbol` / `SelectExternalForSymbol` / `MarketForSymbol` / `KnownIndexMarket` 全部保留原签名，仅内部改为查同一张表。外部 6 个调用点零改动：

- `internal/app/app.go:479`、`:558`、`:597`、`:781`、`:842`
- `internal/collector/tushare/collector.go:80`
- `internal/api/handler/api/symbol_detail.go:131`

### 6.2 结构与匹配规则

```go
type Route struct {
    Pattern   string       // glob: "*.SH" / "^HSI" / "*-USD" / "BTC*"
    Collector string
    Market    core.Market
}
```

**具体度优先，而非注册顺序**。具体度 = pattern 中非通配字符数，降序排列后取首个命中。config 追加规则时不存在顺序陷阱：`^HSI` 永远胜过 `^*`，与文件位置无关。

### 6.3 内置表

| Pattern | Collector | Market | 对应现状 |
|---|---|---|---|
| `*.SH` / `*.SZ` | eastmoney | CN_A | `isAShareSymbol` |
| `*.HK` | yahoo | HK | `MarketForSymbol` HK 分支 |
| `^GSPC` `^IXIC` `^DJI` | yahoo | US | `indexMarkets` |
| `^HSI` `^HSCE` | yahoo | HK | `indexMarkets` |
| `^*` | yahoo | US | `isIndexSymbol` 兜底 |
| `*=F` | yahoo | US | `isCommoditySymbol` |
| `*-USD` / `*USDT` | crypto | CRYPTO | `isCryptoSymbol` 后缀分支 |
| `BTC*` `ETH*` … ×13 | crypto | CRYPTO | `cryptoTickers` 前缀分支 |
| `*` | yahoo | US | 默认兜底 |

### 6.4 两处保留为表前置规则

1. **qlib `Covers()`** —— 运行时判定仓库覆盖，优先级最高，保持在查表之前（`selector.go:56-61` 原样）
2. **`IsAShareIndex()`** —— 对 `AShareIndexSecIDs` 的成员判定（`indexes.go:29`），覆盖 `930713.CSI` 这类不带 `.SH/.SZ` 后缀的中证跨市场指数。键集离散，无法通配

### 6.5 `KnownIndexMarket` 语义映射

`app.go:781` 用其 `known` 返回值对未登记的 `^` 符号告警。改造后 `known = 命中的 Route 不含通配符`——命中 `^HSI` 为 known，落到 `^*` 为 unknown。语义等价。

## 7. 测试策略

按项目 TDD 约定测试先行。

### 7.1 路由黄金值回归（最先写）

穷举符号形态，逐一断言四个公开函数的返回值：`600519.SH`、`930713.CSI`、`0700.HK`、`AAPL`、`^GSPC`、`^HSI`、`^N225`（未登记）、`CL=F`、`BTC-USD`、`ETHUSDT`、`SOL`、空串、畸形符号。

**先对旧实现跑绿，再重写内部，必须仍然全绿。**

### 7.2 Gate 单元测试

- throttle：同域连续两次 `Fetch`，第二次耗时 ≥ MinInterval；跨域互不阻塞
- coalesce：N 个 goroutine 并发同 key，断言 `fn` 只被调用 1 次且 N 个调用方都拿到结果
- TTL：命中不调 `fn`；过期后重新调用；`fn` 报错时不写缓存
- Timeout：`fn` 阻塞超时返回 `ErrTimeout`

### 7.3 QuotaStore 跨进程语义

配额设计的立身之本，必须真正验证：

- **两个独立 `Gate` 实例指向同一 JSON 文件**（模拟两次 launchd 启动），第一个用掉 5 次，第二个首次 `Take` 即被拒 —— 直接验证「短命进程下配额仍生效」
- 窗口翻篇：`window_start` 属于昨天 → 计数归零
- fail-open：文件内容为非法 JSON → `Take` 放行且不 panic

### 7.4 防回潮断言

一条 `collector` 包级测试，用 `go/ast` 扫描各 collector 源码，断言不再出现 `lastReq` 字段，防止后来者在新 collector 里重写私有 throttle。

Go 无法机制化强制「必须走 Gate」，这是最接近的替代，不宜高估其强度。

### 7.5 降级链兼容性

配额耗尽后 `tushare` 的返回值必须满足 `errors.Is(err, tushare.ErrRateLimited)`。这是「`refresh.go` 一行不改」承诺的锚点。

### 7.6 prism refresh 集成回归

用 httptest 把 `tushare.daily_basic` 打到配额上限，断言降级链正常走到下一跳，且**没有真的发出第 6 次 HTTP 请求**（撞墙前拦截生效）。

### 7.7 需改写的既有测试

- `internal/collector/cache_test.go` —— 随 `CachedCollector` 删除而重写为 Gate 测试
- `internal/collector/yahoo/throttle_test.go` —— 迁移到 Gate
- `internal/collector/twelvedata/client_test.go`、`internal/collector/tushare/client_test.go` 的节流用例 —— 改为注入短间隔 Gate

## 8. 验收标准

1. `internal/prism/refresh.go` 零改动，全部既有测试通过
2. 路由黄金值测试对新旧实现均全绿
3. 两个 Gate 实例共享文件时配额跨「进程」生效
4. tushare 配额耗尽返回的错误满足 `errors.Is(err, ErrRateLimited)`
5. 各 collector 源码中不再存在 `lastReq` 字段
6. lixinger（`FundamentalCollector`）能进入缓存路径 —— 即 §1.3 的缺陷被修复

## 9. 不在本次范围

- 数据新鲜度元数据（评估报告第 ③ 项）：需 `valuation_daily` / `price_daily` 加 `source` / `fetched_at` 列，涉及 schema 迁移，单独立项
- meta 层 `request_id`（第 ④ 项）：独立小补丁，单独立项
- 给 `Collector` 接口加 `context.Context`（§5.4）
- 为 eastmoney / akshare / lixinger 等当前无节流的 collector 配置限流策略（§4.1）
