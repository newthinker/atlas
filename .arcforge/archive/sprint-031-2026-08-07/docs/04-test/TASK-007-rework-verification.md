# TASK-007 返工验证报告 —— yahoo 两处缓存键的时间精度双向守护

- 验证者: test-agent-16 / `assignment_epoch: 1` / `rework_count: 1`
- 被验对象: **`415f900`**（`gate_test.go` +107，**生产代码零改动**）
- 判定依据: **`fix_items[F1]`**
- 验证环境: 独立 worktree `../wt-v007r @ 415f900`（已在主仓库拆除）

## 结论：**PASS（verified）** —— 但发现一处**范围之外**的既有缺口，见 §五

`fix_items[F1]` 完全满足：两处 key 各有双向守护，**四格判据交叉独立**，且避开了 F1 点名的精度坑
（我独立复现了那个坑，证明 `+3s/+15s` 是必需而非保守）。

---

## 一、`fix_items[F1]` 逐项对照

| F1 的要求 | 证据 | 判定 |
|---|---|---|
| 两处 key 各补一条**双向**守护（共两条）| `TestFetchHistoryCacheKeyAggregatesNearbyTimes` + `TestFetchEPSHistoryCacheKeyAggregatesNearbyTimes`，各含 (a)(b) 两个子测试 | **PASS** |
| (a) 相邻时间落进同一槽，去 `Truncate` 须转红 | **W1**（yahoo.go）红 `:420`；**W3**（eps.go）红 `:472` | **PASS** |
| (b) 粒度不得放粗，`Truncate→Hour` 须转红 | **W2** 红 `:436`；**W4** 红 `:487` | **PASS** |
| 两条判据**各自独立实证** | W1/W2 互不触发，W3/W4 互不触发（§二）| **PASS** |
| **偏移必须跨秒**（`.Unix()` 秒级精度坑）| 用的是 `+3s/+15s`；我独立复现了坑本身（§三）| **PASS** |
| 时间基准取当前分钟中点避开边界假红 | `time.Now().Truncate(time.Minute).Add(20*time.Second)`；连跑 10 次 0 失败 | **PASS** |
| **纯测试新增，生产代码零改动** | `git show --name-only` 只有 `gate_test.go` | **PASS** |
| 不碰 `yahoo_test.go` 的既有 gofmt 漂移 | 未触碰 | **PASS** |

## 二、四格判据交叉独立 —— 这是「两处 key 必须各写一条」的证据

每个变异**注入前先写下预期**，四道门全程强制。**四格全中，且每格只红对应的那一个子测试**：

| ID | 变异 | 预期 | 实际 | 红在 | 对照（应保持绿）|
|---|---|---|---|---|---|
| **W1** | `yahoo.go:312` 去 `Truncate` | 红 | **红** ✅ | `:420` History/(a) | **EPS 两格全绿** ✅ |
| **W2** | `yahoo.go:312` 放粗到 `Hour` | 红 | **红** ✅ | `:436` History/(b) | History/(a) 仍绿 ✅ |
| **W3** | `eps.go:49` 去 `Truncate` | 红 | **红** ✅ | `:472` EPS/(a) | **History 两格全绿** ✅ |
| **W4** | `eps.go:49` 放粗到 `Hour` | 红 | **红** ✅ | `:487` EPS/(b) | EPS/(a) 仍绿 ✅ |

**两个方向的证据分别是**：
- **W1 vs W3 的交叉全绿** ⇒ 两处 key 相互独立，**一处修好不代表另一处也修好**，所以必须各写一条
- **W1 vs W2 / W3 vs W4** ⇒ 只写 (a) 会被 `Truncate(time.Hour)` 骗过，两个方向缺一不可

## 三、独立复现 F1 点名的精度坑 —— 证明 `+3s/+15s` 是必需的

F1 的 `trap` 字段警告：yahoo 的 key 用 `.Unix()`（秒级），照搬别包的毫秒偏移会让变异**测不出来**。

我把新测试的偏移临时换成 crypto 的 `+50ms/+900ms`，再注入 **W1**（去 `Truncate`）：

```
--- PASS: TestFetchHistoryCacheKeyAggregatesNearbyTimes
    --- PASS: .../相邻时间落进同一槽
    --- PASS: .../分钟粒度不得放粗
⇒ 绿
```

**坑已复现**：`.Unix()` 把亚秒差异归为同一秒，去掉 `Truncate` 后键仍然相同 ⇒ 断言照样通过
⇒ 会得出一个**假的「变异无效」**结论。

⇒ **`+3s/+15s` 不是保守，是必需。** dev-agent-35 在 eastmoney 踩过这一次后，本次正确规避。

> 这条值得进契约：**「照搬另一个包的测试形态」时，必须检查偏移量/精度单位与本包 key 的
> 单位匹配**。同一个测试骨架在秒级 key 与纳秒级 key 下的有效性完全不同，而失效方向是
> **假的「变异无效」**——比假绿更隐蔽，因为它伪装成「这里本来就没问题」。

## 四、基线与稳定性

| 项 | 结果 |
|---|---|
| 测试规模 | **50 个顶层测试全 PASS / 0 FAIL / 0 SKIP**（`-run` 全包 83 个 `=== RUN`）✅ |
| `-race` | ok（22.2s）✅ |
| 覆盖率（**单包口径**，本任务无 `coverage_floor`）| **89.3%**（返工前 88.0%，+1.3pp）✅ |
| 新测试稳定性 | 连跑 **10 次 0 失败** ✅ |
| scope | 只改 `gate_test.go`，`writes` 内，**生产代码零改动** ✅ |
| 还原 | 每变异 `md5` 校验；收尾 `git status --porcelain` 空 ✅ |

## 五、⚠ 范围之外发现的既有缺口：yahoo 缓存键的**区分度**无守护

**这不是本次返工的问题**（不在 `fix_items[F1]` 内，也不在 TASK-007 原始 DoD 内），
但它是本 Sprint 一直在猎的那个形态，且出现在**主源** yahoo 上，故必须报。

### 5.1 变异证据

**W5**：把 `yahoo.go:312` 的键里的 `symbol` 换成固定串 —— **83 个测试全绿，无一转红。**

四道门全过（md5 已变 / vet 通过 / `=== RUN`=83 / 无断言行）。

### 5.2 根因：TASK-007 的原始 DoD 里**根本没有区分度这一条**

TASK-007 的 `done_criteria.functional` 只有四条：

```
0. FetchHistory 经 Gate 走 TTL 缓存（同参数只发一次）
1. FetchQuote 主题与 TTL=0、仍受 500ms 节流
2. yahoo.chart 与 yahoo.eps 共享限流域
3. 重试循环复用同一闸门
```

**没有「缓存 key 覆盖全部影响结果的参数」。** 那条（陷阱 16）与聚合度（functional[4]）一样，
都是本 Sprint **后期**才补出来的 criteria —— Leader 用 `fix_items[F1]` 回补了聚合度，
**区分度没有一并回补**。

包内唯一出现第二个 symbol 的地方是 `gate_test.go:297` 的
`y.FetchHistory("MSFT", ...)`，注释写着「换 key 绕开缓存」——它是
`TestFirstRequestDoesNotWaitTwice` 用来**避开缓存**的手段，且那个用例的闸门是
`gateWith(policy.Policy{MinInterval: iv})`（**没设 TTL** ⇒ 压根没缓存），
所以键塌掉了它也照样绿。**没有任何测试断言过「不同 symbol 必须分槽」。**

### 5.3 影响

键若丢掉 symbol，**AAPL 会拿到 MSFT 的行情**——「静默的错数据」，契约明写这比缓存没生效
危险得多。而 yahoo 是美股/A 股主源，影响面比 baostock/crypto 都大。

### 5.4 候选补丁（我已验证 2×2 闭环）

```go
// 缓存键的**区分度**——不同 symbol 不得共用缓存槽（契约陷阱 16）。
// a→b→a 重放同时排除两种缺陷：键不含 symbol（b 误命中 a ⇒ 总数 1）与
// 压根没缓存（总数 3）。只发 a、b 两次并断言 2 的写法对后者是假绿。
func TestFetchHistoryCacheKeyDistinguishesSymbol(t *testing.T) {
	srv, arrivals := countingServer(t)
	y := NewWithBaseURL(srv.URL)
	y.gate = gateWith(policy.Policy{TTL: time.Minute, Coalesce: true})

	start, end := time.Unix(1600000000, 0), time.Unix(1700086400, 0)
	for _, sym := range []string{"AAPL", "MSFT", "AAPL"} {
		if _, err := y.FetchHistory(sym, start, end, "1d"); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(arrivals()); n != 2 {
		t.Errorf("缓存键未区分 symbol: HTTP 请求 %d 次, want 2"+
			"（为 1 说明 MSFT 命中了 AAPL 的槽——生产上会静默返回别的标的的行情）", n)
	}
}
```

**2×2 同环境闭环**：

| | 正确实现 | W5 变异 |
|---|---|---|
| **现有测试集** | 绿 | **绿 ❌ 漏检** |
| **候选测试** | 绿 ✅ | **红 ✅**（`HTTP 请求 1 次, want 2`）|

### 5.5 建议

**这是 Leader 的决定，不是我的判定依据**：

- 本次返工按 `fix_items[F1]` 判 **PASS**（F1 完全满足，Dev 做的正是被要求的事）
- 建议**新开一条 `fix_items[F2]`** 回补区分度，形态同 F1（照抄上面的候选补丁）
- 若照 F1 的 `rework_accounting` 口径，**这条同样「规格后到不计」**熔断额度
- **另请同时核 `eps.go:49`**：它的键也含 symbol，很可能有同一缺口（我未验证 EPS 侧，
  因为已超出本次范围；但两处 key 同源同形，值得一并回补）
- 更值得做的是**横向排查**：区分度 criteria 是后期补的，凡在它之前定稿的任务
  （007 是最早那批）都可能同样漏掉。TASK-015/016/017 是在它之后定的，已含该条。

## 六、复现命令

```bash
git worktree add --detach ../wt-v007r 415f900 && cd ../wt-v007r
GOTOOLCHAIN=local go test ./internal/collector/yahoo/ -count=1 -race     # 50 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/yahoo/ -count=1 -cover    # 89.3%

# 四格判据（两处 key × 两个方向），每格只红对应子测试
#   W1 yahoo.go:312 去 Truncate      → :420   （EPS 两格保持绿）
#   W2 yahoo.go:312 Truncate→Hour    → :436
#   W3 eps.go:49    去 Truncate      → :472   （History 两格保持绿）
#   W4 eps.go:49    Truncate→Hour    → :487

# 精度坑复现：把新测试偏移换成 +50ms/+900ms 再注入 W1 → 转绿（假的「变异无效」）

# §五 的既有缺口：把 yahoo.go:312 键里的 symbol 换成固定串 → 83 测试全绿
GOTOOLCHAIN=local go test ./internal/collector/yahoo/ -count=1

cd <主仓库> && git worktree remove ../wt-v007r
```
