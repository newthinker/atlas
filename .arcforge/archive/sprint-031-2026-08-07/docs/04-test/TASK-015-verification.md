# TASK-015 验证报告 —— eastmoney 接入 Gate + 内置表补登记三家

- 验证者: test-agent-17
- 被验对象: `b238221de6d5d63a262ff1c391304c0e21d11acc`（4 文件 +342/-10）
- 验证环境: 独立 worktree `../wt-v015`（detached @ b238221），另开 `../wt-v015base` @ 父提交取覆盖率水位
- assignment_epoch: 1 / rework_count: null（首轮）

## 结论：**NEEDS WORK（rejected）**，`reason_class = task_defect`

**11 项 DoD 中 10 项 PASS，1 项 FAIL。** 交付质量很高：Dev 自报的变异我逐条独立复现全部属实，
两条既有守护的改动**不但没改弱、反而更强**（§三），三处 Leader 点名核查全部通过。

FAIL 的是 **`error_handling[1]`：policy 错误映射层完全缺失**，且**不是「今天测不出」而是「没有」**——
我配上 Quota/Timeout 后**立即复现外泄**：

```
配额耗尽时 FetchHistory 返回: policy: quota exceeded     ← 原样外泄
超时时     FetchHistory 返回: policy: fn timeout          ← 原样外泄
```

---

## 一、FAIL：`error_handling[1]` 映射层缺失（实测，非读码推断）

DoD 原文：

> **policy 错误不得外泄**：`ErrQuotaExceeded`/`ErrTimeout` 等 policy 包哨兵错误不得原样返回给上层，
> 须映射为本包错误。……本任务无 Quota/MinInterval 故今天不触发，**但映射层须就位**
> （否则将来给 `eastmoney.*` 加限流时，这个缺陷会静默生效）。

### 1.1 静态核实：映射层不存在

```go
data, err := policy.Fetch(e.gate, topicHistory, key, func() ([]core.OHLCV, error) { ... })
if err != nil {
    return nil, err        // ← 原样返回，无任何映射
}
```

```
grep -n 'ErrQuotaExceeded|ErrTimeout|errors.Is' eastmoney.go  →  无输出
```

对照 tushare（TASK-009 已验收）：`client.go:99-101` 有完整映射
（`errors.Is(err, policy.ErrQuotaExceeded)` → `ErrRateLimited`）。**eastmoney 没有对应物。**

### 1.2 动态复现：一旦配上策略立即外泄

我写探针给 `eastmoney.*` 配 `Quota{Limit:1}` 与 `Timeout: 1ns`（探针已删除，不在提交内）：

| 场景 | `FetchHistory` 返回 | `errors.Is(err, policy.ErrXxx)` |
|---|---|---|
| 配额耗尽 | `policy: quota exceeded` | **true** ❌ |
| fn 超时 | `policy: fn timeout` | **true** ❌ |

⇒ **这不是「不可达所以测不出」**，而是「映射层没写」。Leader 要求区分的两种情况里，
本任务属于**后者**：`ErrTimeout` 甚至不需要 Quota，任何主题配上 `Timeout` 就会触发，
而 config 的 `collector.topics.*.timeout` 是**已存在的可配置项**（TASK-005/006 的 `Override.Timeout`）——
**今天就能被配出来**。

### 1.3 修复方向

照 tushare 的形式在 `FetchHistory` 的错误分支加映射，并补一条测试
（形态：注入带 `Quota`/`Timeout` 的表 → 断言 `errors.Is(err, policy.ErrXxx) == false` 且满足本包错误）。
**注意 DoD 的另一半**：「临时性错误绝不可映射成永久性」——限流/超时不能映射成「无此标的」那类。

---

## 二、Leader 点名核查的三处

### 2.1 `boundary[3]` FetchQuote 不经 Gate —— **独立复核通过**

```
grep -n 'policy\.' eastmoney.go
:40  gate *policy.Gate            （字段）
:59  gate: policy.Default(),      （构造函数）
:436 policy.Fetch(...)            （唯一调用点）
```

`:436` 位于 `FetchHistory`（`:433`）内；`FetchQuote`（`:186`）及其全部下游
（`fetchStockQuote :205`、`fetchFundQuote :261`、`fetchFundQuoteFromEastmoney :275`）
**无任何 `policy.Fetch`**。Quote 路径也不会间接走到 `FetchHistory`。**Dev 自述属实。**

### 2.2 `functional[3]` 已按 QA 观察改为验「域一致」—— 有效

E2（把 `topicHistory` 写成 `"eastmoneyx.kline"`）→ **红**
`gate_test.go:171: 主题常量 "eastmoneyx.kline" 在内置表中查不到——生产路径会退化为无策略直通`

**改后的 criteria 确实测得出东西**：写错**域名**才会真的落空，而写错域名正是通配登记下唯一
仍然致命的形态。原表述（「主题常量与内置表一致」）在通配形态下确实测不出 —— Leader 的修正成立。

### 2.3 `TestCacheKeyCoversAllParams` 的错误消息与断言一致

Dev 自我修正后的消息是「HTTP 请求 1 次, want 2（**为 1** 说明三次调用挤在同一个缓存槽里）」，
我三次变异实测输出的都是 `got 1` ⇒ **消息与实际现象一致** ✅

---

## 三、两条既有守护：**没有改弱，反而更强**（六方向变异实证）

Leader 的担心是「删掉清单里的三家单独看就是删证据」。实测结论：**双向断言补回来了且超出**。

| 变异 | 打的方向 | 结果 |
|---|---|---|
| **P1** 撤回三家登记（缓存丢失回归重现） | 反向：应命中 | **红** `:92 eastmoney.kline: 应命中 <域>.* 通配主题` ×3 |
| **P2** 三家凭空多出 `MinInterval` | 反向：只补缓存 | **红** `:99 只补缓存，不得新增限流/配额: MinInterval=1s` |
| **P3** 三家凭空多出 `Quota` | 反向：只补缓存 | **红** `:99 ... Quota=&{5 24h0m0s UTC}` |
| **P4** 三家 `TTL` 置 0 | 反向：必须有 TTL | **红** `:96 必须有 TTL（这正是要修复的缓存丢失回归）` |
| **P5** akshare 被凭空登记 | **正向**：未登记不应命中 | **红** `:84 akshare.valuation: 未登记主题不应命中策略表` |
| **P6** 撤回三家登记 | 集合等值（`TestDisableTTLKeepsThrottle` 9→12） | **红** `:233 Topics() = [...]` |

**原断言只验「6 家不命中」（单向）；现断言验「3 家不命中 + 3 家命中且 `TTL>0`/`MinInterval==0`/`Quota==nil`」
（双向 + 属性约束）。** 集合从 9 改 12 是集合等值断言的必然要求（P6 证明它有守护）。

---

## 四、其余 DoD 变异（全部符合注入前写下的预期）

| ID | 变异 | 定向到 | 结果 |
|---|---|---|---|
| **E9** | 短路 `fn` 内的空 klines 校验（twelvedata 形态） | `TestErrorResponseIsNotCached` | **红**，三条断言全触发：`:254`×2 识别为错误 + **`:258 错误不得写缓存，两次调用应各发一次 HTTP, got 1`** |
| E-k1 | 缓存键去掉 `symbol` | `TestCacheKeyCoversAllParams` | **红** `:137 缓存键未区分 symbol` |
| E-k2 | 缓存键去掉 `interval` | 同上 | **红** `:137 缓存键未区分 interval` |
| E-k3 | 缓存键去掉 `end` | 同上 | **红** `:137 缓存键未区分 区间` |
| E1 | 构造函数不取 `Default()` | `TestNewUsesDefaultGate` | **红** `:159 New 必须把 policy.Default() 存进 gate 字段` |
| E3 | `FetchHistory` 不 `Clone` | `TestFetchHistoryReturnsIndependentSlice` | **红** `:234` |
| E4 | `FetchHistory` 不经 Gate | `TestFetchHistoryCachesViaGate` | **红** `:97 TTL 内应只发 1 次 HTTP 请求, got 3` |
| E5 | 内置表给 eastmoney 加 `MinInterval` | `TestNotThrottled` | **红** `:213 3 次请求耗时 1.0016s——eastmoney 不应被节流` |

**E9 的 `:258` 是决定性断言**（HTTP 请求数），Dev 自述的两条断言都触发属实。
**缓存键三组各自独立有效**（去掉哪个就报哪个），超出 DoD 要求的「至少两组」。

`error_handling[0]` 的形态判定：eastmoney 的 200-but-error 校验（`:492`
`result.Data == nil || len(result.Data.Klines) == 0`）位于被缓存的 `fn` 深处、返回已解析的
`[]core.OHLCV` ⇒ **twelvedata 形态（挪不出去）**，故变异取「短路 `fn` 内校验」而非「把校验上提」。

---

## 五、其余核查

| 项 | 结果 |
|---|---|
| `boundary[2]` 既有测试互不串味 | `TestMain` 装零策略闸门 ✅；`cachingGate` 用**内置表**而非手搓策略（「测试跑的是生产查表路径」）—— 这个选择比手搓更好 |
| `boundary[1]` 独立切片 | `slices.Clone` + 注释写明限定（`core.OHLCV` 是 flat value type，**前提由 core/types.go 保证，不是这里**）✅ |
| `non_functional[0]` 既有测试一字不改 | `eastmoney_test.go` **未出现在 diff 中** ✅ |
| 覆盖率不低于既有水位 | 父提交 **87.0%** → 本任务 **87.4%** ✅ |
| `-race` / SKIP | eastmoney `-race` 绿；两包 **0 SKIP 0 FAIL** ✅ |

> **覆盖率口径备录**：门禁报 `91.4%`，我实测 eastmoney 单包 **87.4%**、policy 单包 94.4%。
> 91.4% 落在两者之间，应是**两包合并**口径。三个数字都对，**量的不是同一个东西**——
> 与本 Sprint 的 `gofmt`（29/9/4）、覆盖率 floor `74` 是同一类。DoD 说「不低于既有水位」，
> 按**同口径**（eastmoney 单包，87.0→87.4）成立。建议 reject_reason 与后续报告统一注明口径。

---

## 六、复现命令

```bash
git worktree add --detach ../wt-v015 b238221de6d5d63a262ff1c391304c0e21d11acc
cd ../wt-v015      # 跨包变异，还原一律在**仓库根**执行 git checkout -- .

# FAIL 依据（error_handling[1]）：给 eastmoney.* 配 Quota{Limit:1} 或 Timeout:1ns，
#   调 FetchHistory 两次 → 第二次返回 "policy: quota exceeded" / "policy: fn timeout"
#   errors.Is(err, policy.ErrQuotaExceeded) == true ⇒ **原样外泄**
# grep -n 'ErrQuotaExceeded|ErrTimeout|errors.Is' internal/collector/eastmoney/eastmoney.go → 无输出

# E9：把 `result.Data == nil || len(result.Data.Klines) == 0` 改成只判 `result.Data == nil`
#   → :254 ×2 + :258（got 1，决定性）
# 缓存键：从 key 的 Sprintf 里分别去掉 symbol / interval / end → :137 各自报对应参数
# P1-P6：改 policy.go 里三家的 t.Set 登记（撤回/加 MinInterval/加 Quota/TTL 置 0/加 akshare）
#   → policy_test.go :84 / :92 / :96 / :99 / :233

# 五道门 + 预期列全程执行；每条变异先 `git diff --numstat` 确认落盘、`go test -c` 确认编译
```

worktree（`wt-v015`、`wt-v015base`）已于验证结束后清理；主工作区零污染。
