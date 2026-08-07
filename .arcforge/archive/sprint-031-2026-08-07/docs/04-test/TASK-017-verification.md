# TASK-017 验证报告 —— baostock 接入 Gate（仅 TTL）

- 验证者: test-agent-16 / assignment_epoch: 1
- 被验对象: `fb29d71`（client.go +5 · collector.go +47 · gate_test.go +328）
- 验证环境: 独立 worktree `../wt-v017 @ fb29d71`（已在主仓库拆除）

## 结论：**NEEDS WORK（rejected）** —— reason_class = `task_defect`

**8 条 done_criteria 中 7 条通过，1 条无守护：`functional[4]`（缓存键的聚合度）。**

DoD 为这一条**明文规定了变异判据**：「**变异判据：去掉 Truncate 后该测试须转红**」。
我照做了——**去掉两个 `Truncate(time.Minute)` 后，28 个测试全绿**。

**实现是对的**（`Truncate` 确实写了，与 yahoo.go:292 / eastmoney.go:435 同款），
**缺的只是测试**。返工量：**一个测试函数**，我已验证候选实现有效（§三）。

---

## 一、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 守护者 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 经 Gate 走 TTL 缓存，同参数只发一次 HTTP（断言请求次数）| `TestFetchHistoryCachesViaGate` | T3 绕过 Gate → 红 `:88` | **PASS** |
| functional[1] | 键覆盖全部**影响结果**的参数，≥2 组参数互不命中 | `TestCacheKeyCoversParams`（a→b→a 重放）+ `TestIntervalDoesNotSplitSlot` | T2 键去掉 symbol → 红 `:123`；T3 → 红 `:123`/`:152` | **PASS**（interval 的处理见 §四） |
| functional[2] | 构造函数取 `policy.Default()`，**独立测试**断言 | `TestNewUsesDefaultGate` | T4 `gate: nil` → 红 `:170`，**且仅此一条**（陷阱 12 形态复现）| **PASS** |
| functional[3] | 主题**域段**与内置表一致 + `Policy.Domain` 断言 | `TestTopicDomainMatchesBuiltinTable` | T5 域段写错 → 红 `:188`；T6 内置表加 MinInterval → 红 `:197` | **PASS** |
| **functional[4]** | **缓存键聚合度：亚分钟差异的 end 须命中同一槽（`Truncate(time.Minute)`）** | **无** | **T1 去掉两个 `Truncate` → 28 个测试全绿 ❌** | **FAIL — 标准未覆盖** |
| boundary[0] | 不被节流（否定断言 + 对照组）| `TestNotThrottled` | T6 → 红 `:228`（对照组机制有效）| **PASS** |
| boundary[1] | 缓存命中返回独立切片 | `TestFetchHistoryReturnsIndependentSlice` | T7 去掉 `slices.Clone` → 红 `:249` | **PASS** |
| boundary[2] | 既有测试互不串味（陷阱 13）| 包级 `TestMain` 装零策略闸门 | 见 §五（Dev 的理由属实）| **PASS** |
| error_handling[0] | 错误不写缓存 + **policy 错误不外泄** + 临时性不得映射成永久性 | `TestErrorIsNotCached` + `TestPolicyErrorsDoNotLeak` | T8 `mapPolicyError` 改用 `%w` → 红 `:322`（链未断=泄漏被抓）| **PASS** |
| non_functional[0] | 既有测试一字不改全绿；`-race` 绿；0 SKIP；覆盖率不低于水位 | 实测 | §六 | **PASS** |

## 二、唯一的 FAIL：functional[4] 无守护

### 2.1 变异证据

**T1**：把缓存键里的两个 `Truncate(time.Minute)` 去掉

```go
// 被验实现（正确）
key := fmt.Sprintf("%s|%d|%d", symbol,
    start.Truncate(time.Minute).Unix(), end.Truncate(time.Minute).Unix())
// T1 变异
key := fmt.Sprintf("%s|%d|%d", symbol, start.Unix(), end.Unix())
```

四道门：① md5 已变 ✅ ② `go vet` 通过 ✅ ③ `=== RUN` = 28 ✅ ④ 无任何断言行

**结果：绿（存活）。28 个测试没有一条转红。**

### 2.2 为什么这不是「等价变异体」——生产影响是真实的

| 事实 | 证据 |
|---|---|
| 上层传的 end 就是 `time.Now()` | `internal/app/app.go:451` `end := time.Now()` → `:462` `c.FetchHistory(symbol, start, end, "1d")` |
| baostock 在这条路径上 | 它是 A 股行情第三跳，经同一 `FetchHistory` 接口消费 |
| 同款口径是既有约定 | `yahoo.go:292` 与 `eastmoney.go:435` **都是** `Truncate(time.Minute)`，注释均写「沿用被取代的口径」 |

⇒ 键若保留秒级精度，**每次调用的 end 都不同 ⇒ 缓存命中率恒为零**，而本任务的**全部目的**
就是把被删 `maybeCache` 的 TTL 缓存接回来。**缓存静默失效，且测试一片绿**——
这正是 DoD 描述的「crypto 的实际形态」。

### 2.3 测试集为何抓不到

现有 8 个测试全部使用**固定时刻**（`time.Unix(1600000000, 0)` / `time.Unix(1700086400, 0)`）。
固定时刻在 `Truncate` 前后都相等，键不变 ⇒ 有无 `Truncate` 行为完全一致。

`grep 'time.Now()' *_test.go` 的命中全部落在：
- `client_test.go`（走 `FetchDaily`，**不经 Gate**）
- `collector_test.go:79`（**单次**调用，无法验证两次是否同槽）
- `gate_test.go:211`（是 `t0 := time.Now()` 计时，与缓存键无关）

**没有任何测试用「两次亚分钟差异的 end」调用 `FetchHistory`。**

## 三、候选修复（我已验证有效，2×2 闭环）

```go
// 亚分钟差异的 end 必须落到同一缓存槽。
// 刻意不直接用两次 time.Now()——那样两次调用可能跨分钟边界而偶发假红；
// 用「同一分钟内的两个确定时刻」既无边界抖动，又精确表达被测语义。
func TestCacheKeyAggregatesSubMinuteEnd(t *testing.T) {
	srv, hits := countingBridge(t, http.StatusOK, dailyBody)
	c := New(srv.URL)
	c.gate = cachingGate()

	start := time.Unix(1600000000, 0)
	base := time.Now().Truncate(time.Minute).Add(20 * time.Second)
	for _, end := range []time.Time{base, base.Add(3 * time.Second)} {
		if _, err := c.FetchHistory("600519.SH", start, end, "1d"); err != nil {
			t.Fatal(err)
		}
	}
	if n := hits(); n != 1 {
		t.Errorf("同一分钟内的两个 end 必须命中同一缓存槽: HTTP 请求 %d 次, want 1"+
			"（上层 app.go:451 传的 end 就是 time.Now()，键保留秒级精度 ⇒ 生产命中率恒为零）", n)
	}
}
```

**2×2 同环境闭环**（同 worktree、同会话）：

| | 有 `Truncate`（正确实现） | 去掉 `Truncate`（T1 变异） |
|---|---|---|
| **现有测试集** | 绿 | **绿 ❌ 漏检** ← **这一格才是返工的理由** |
| **候选测试** | 绿 ✅ | **红 ✅ 捕获**（`HTTP 请求 2 次, want 1`）|

> 关于 DoD 措辞「以 `time.Now()` 为 end 的两次相邻调用」：**字面照做有边界抖动风险**——
> 两次 `time.Now()` 若跨越分钟边界，即便实现正确也会转红（假红）。
> 用「同一分钟内的两个确定时刻」表达的是同一语义且**确定性**。建议按此实现。

## 四、functional[1] 中 interval 的处理 —— 偏离字面但理由成立

DoD 写「键覆盖全部影响结果的参数（symbol/区间/**interval**）」，而实现**刻意不把 interval 入键**。

我核实了 Dev 的理由：`FetchHistory` 开头 `if interval != "" && interval != "1d" { return error }`
——**只有 `""` 与 `"1d"` 能到达 Gate，二者都调用同一个 `c.FetchDaily(symbol, start, end)`，
结果完全相同**。故 interval **确实不影响结果**，DoD 的限定语是「全部**影响结果**的参数」，
interval 不在其列。

且该决定有测试守护（`TestIntervalDoesNotSplitSlot`：两者须命中同一槽 + 其余粒度须在进 Gate
前被拒），并在注释里写明「若将来桥支持别的粒度，必须把 interval 加进键并改写本测试」。

⇒ **判 PASS。** 这是有据的设计决定，不是遗漏。

## 五、boundary[2] 既有测试串味 —— Dev 的理由属实

Dev 用包级 `TestMain` 装零策略闸门，理由是 `collector_test.go` 的成功用例与「桥不可达应报错」
用例共用 `600519.SH`，两个 `Client` 实例不同但 **gate 同为 `policy.Default()`**，缓存共享。

我核实：两个用例确实都走 `FetchHistory` 且共用该 symbol；`New()` 取的是同一个进程单例。
**理由成立，挡法与 TASK-008/010 一致。**

## 六、基线与约束

| 项 | 结果 |
|---|---|
| 测试规模 | **21 个测试全 PASS / 0 FAIL / 0 SKIP** ✅ |
| `-race` | ok（2.15s）✅ |
| 覆盖率（**单包口径**，本包无 `coverage_floor`）| **96.4%** ✅ |
| 既有测试文件 | `git show --name-only` 只有 **gate_test.go（新增）**，`client_test.go`/`collector_test.go` **一字未改** ✅ |
| scope | 全部落在 `./internal/collector/baostock/`，与 `writes` 一致 ✅ |
| 还原 | 每个变异 `cp` + `md5` 校验；收尾 `git status --porcelain` 空 ✅ |

### 6.1 陷阱 14 的处置 —— Dev 主动声明「本包无此风险面」，我核实属实

Dev 在测试文件头写明：本包**没有「HTTP 200 但响应体表示失败」的形态**（`FetchDaily` 对空数组
返回空切片 + nil error，业务失败只经 HTTP 状态码表达），故不硬造测试，理由是
「**一条永远绿的测试比一条注释更糟，它占着『已守护』的名分**」。

我核实：`FetchDaily` 的错误路径确由状态码驱动，`TestErrorIsNotCached` 用 5xx 覆盖。
**该声明属实，处置正确**——这与 TASK-011 那次废弃恒真断言是同一判断。

## 七、变异验证结果表（8 个：7 捕获 / **1 存活** / 0 无效）

每个变异**注入前先写下预期**；四道门全程强制。

| ID | 变异 | 预期 | 实际 | 断言行 |
|---|---|---|---|---|
| **T1** | **去掉键里两个 `Truncate(time.Minute)`** | **绿**（我预判无守护）| **绿 ❌ 存活** | — |
| T2 | 键去掉 symbol | 红 | **红** ✅ | `:123` |
| T3 | 绕过 Gate 直调 `FetchDaily` | 红 | **红** ✅ | `:88` `:123` `:152` `:224` `:295` `:319` |
| T4 | `New` 不取 `Default()`（`gate: nil`）| 红 | **红** ✅ | `:170`，**仅此一条**（陷阱 12）|
| T5 | 主题域段写错（`baostockX.daily`）| 红 | **红** ✅ | `:88` `:123` `:152` `:188` |
| T6 | 内置表给 `baostock.*` 加 `MinInterval` | 红 | **红** ✅ | `:197` `:228` |
| T7 | 去掉 `slices.Clone` | 红 | **红** ✅ | `:249` |
| T8 | `mapPolicyError` 改用 `%w`（链不断）| 红 | **红** ✅ | `:322` |

> **我自己的一次操作失误**：首轮 T2–T8 全部报「**无效·门③** 跑了 0 个测试」——我复用
> TASK-011 的 runner 时 `-run 'TestServeWires|TestScanWiring'` 过滤器没改，在本包匹配不到
> 任何测试。**若无门③，这 7 个会被全部误记为「绿·存活」，我会报出 7 个不存在的缺口。**
> 这正是契约「变异无效第④类：测试名打错 → 跑了 0 个测试 → 被判绿存活」的实例。

## 八、返工项（仅一条）

### R1（阻塞项）为 `functional[4]` 补一条测试

- **实现无需改动**（`Truncate` 已正确写入，与 yahoo/eastmoney 同款）
- 加 §三 给出的 `TestCacheKeyAggregatesSubMinuteEnd`（我已验证 2×2 闭环有效）
- 在 gate_test.go 头部的 DoD↔测试映射注释里补上 `functional[4]` 一行（现缺）
- 自证时请复现左下格：**去掉 `Truncate` 后，只有新测试转红**

## 九、复现命令

```bash
git worktree add --detach ../wt-v017 fb29d71 && cd ../wt-v017
GOTOOLCHAIN=local go test ./internal/collector/baostock/ -count=1 -race    # 21 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/baostock/ -count=1 -cover   # 96.4%

# T1（决定性）：把 collector.go:45-46 的两个 Truncate(time.Minute) 去掉
GOTOOLCHAIN=local go test ./internal/collector/baostock/ -count=1          # 仍然 ok ← 缺口
# 加入 §三 的候选测试后重跑同一变异 → 该测试转红

cd <主仓库> && git worktree remove ../wt-v017
```
