# TASK-016 验证报告 —— crypto 接入 Gate（仅 TTL）

- 验证者: test-agent-16 / **assignment_epoch: 2**（任务曾被收回重派）
- 被验对象: `ee18812`（接入）+ **`0a1ed48`**（缓存键截断到分钟，Leader 裁定要求的修复）
- 验证环境: 独立 worktree `../wt-v016 @ 0a1ed48`（已在主仓库拆除）
- 判定线: **`coverage_floor = 70`**（Leader 裁定 (A)，单包口径）

## 结论：**NEEDS WORK（rejected）** —— reason_class = `task_defect`

**11 个变异中 10 个被捕获，质量很高。** 唯一缺口在 `error_handling[0]` 的复合句第二个分句：

> 「**校验必须留在被缓存的 `fn` 内部**（契约陷阱 14）」

**实现是对的**（`len(data) > 0` 确实在 fn 内，Dev 自己的注释也点明了这条风险），
**缺的只是测试**——把该校验挪到 `Fetch` 之后，**21 个测试全绿**。

返工量：**一个测试函数**，我已验证候选实现有效（§三）。

---

## 一、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 守护者 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 经 Gate 走 TTL 缓存，断言请求次数 | `TestFetchHistoryCachedByGate` | C1 绕过 Gate → 红 `:135` | **PASS** |
| functional[1] | 键覆盖 symbol/区间/interval，≥2 组不互相命中 | `TestCacheKeyCoversAllParams`（3 个子测试）| C3 键丢 symbol → 红 `:191`（**仅 /symbol 子测试**）| **PASS** |
| functional[2] | 构造函数取 `policy.Default()`，独立测试 | `TestConstructorsSnapshotDefaultGate` | C6 `gate: nil` → 红 `:265`/`:269`，**仅此一条**（陷阱 12 形态复现）| **PASS** |
| functional[3] | 主题**域段**与内置表一致 | `TestTopicMatchesBuiltinTable` | C9 域段写错 → 红 `:287` | **PASS** |
| **functional[4]** | **缓存键聚合度（双向）** | `TestCacheKeyAggregatesNearbyTimes`（2 个子测试）| **D1** 退回 `UnixNano` → 红 `:230`（仅 /相邻时间落进同一槽）；**D2** 截断放粗到小时 → 红 `:245`（仅 /分钟粒度不得放粗）| **PASS** |
| boundary[0] | 不被节流（否定断言 + 对照组）| `TestFetchHistoryNotThrottled` | C10 内置表加 `MinInterval` → 红 `:352` | **PASS** |
| boundary[1] | 缓存命中返回独立切片 | `TestFetchHistoryReturnsIndependentSlice` | C7 去 `slices.Clone` → 红 `:380` | **PASS** |
| boundary[2] | 既有测试互不串味 | 包级 `TestMain` 装零策略闸门 | 既有 8 用例一字未改且全绿 | **PASS** |
| **error_handling[0]** 分句 1 | 错误不写缓存（决定性断言=请求次数）| `TestErrorIsNotCached` | 覆盖 `err != nil` 路径 | **PASS** |
| **error_handling[0]** 分句 2 | **校验必须留在被缓存的 `fn` 内部** | **无** | **C11 把空结果校验挪到 `Fetch` 之后 → 21 个测试全绿 ❌** | **FAIL — 标准未覆盖** |
| error_handling[0] 分句 3 | policy 错误不外泄，临时性不映射成永久性 | `TestPolicyErrorDoesNotLeak` | C8 `%v`→`%w` → 红 `:433`，**仅此一条** | **PASS** |
| non_functional[0] | 既有测试一字不改全绿；`-race`；0 SKIP；覆盖率 | 实测 §四 | | **PASS** |

## 二、唯一的 FAIL：`error_handling[0]` 分句 2 无守护

### 2.1 本包**确实有**该风险面（不是「无对应形态」）

Dev 自己在 `crypto.go` 的注释里点明了：

> 「校验（空结果视为失败）留在这里面——**挪到闸门外会让失败结果被当成功写进缓存**。」

对应的形态是 **provider 返回 `(空切片, nil)`**，即上游 HTTP 200 但无数据——这正是本包版本的
「200-but-error」。DoD 的前置条件「若本包有该路径」**成立**。

> 对照：TASK-017（baostock）的 Dev 声明「本包无此形态」，我核实**属实**（业务失败只经状态码
> 表达），故那边不算缺口。**crypto 的情况相反：形态存在、实现处理了、但没有测试。**

### 2.2 变异证据

**C11**：把 `if err == nil && len(data) > 0` 的长度校验从被缓存的 fn 内移出，
改由 `Fetch` 返回后再判空。

四道门：① md5 已变 ✅ ② `go vet` 通过 ✅ ③ `=== RUN` = 59 ✅ ④ 无任何断言行

**结果：绿（存活）。21 个测试没有一条转红。**

### 2.3 不是等价变异体 —— 行为确实变了

| | provider 取数次数（两次调用） |
|---|---|
| 正确实现 | **2 次**（空结果在 fn 内被判为失败 ⇒ 返回 error ⇒ 不写缓存）|
| C11 变异 | **1 次**（空结果被当成功缓存 ⇒ 第二次命中缓存）|

**危害**：「无数据」这个结论被**冻结整个 TTL**（内置 5 分钟）。标的后来有数据了也看不到，
且期间不再向 provider 取数——正是陷阱 14 描述的「一次瞬时故障变成整个 TTL 的持续故障」。

### 2.4 现有测试为何抓不到

`TestErrorIsNotCached` 用的是 `countingProvider{err: errors.New("upstream down")}`——
**`err != nil` 路径**。C11 不改这条路径（fn 仍返回 error），故它照常绿。

**没有任何测试让 provider 返回 `(空切片, nil)`。**

## 三、候选修复（我已验证 2×2 闭环有效）

```go
// provider 返回「(空切片, nil)」——上游 HTTP 200 但无数据——是本包对应
// 「200-but-error」的形态。该校验必须留在被缓存的 fn 内部，否则空结果会被
// 当成功写进缓存，整个 TTL 内都不再向 provider 取数（标的后来有数据了也看不到）。
// 决定性断言是**取数次数**而非返回 error。
func TestEmptyResultIsNotCached(t *testing.T) {
	p := &countingProvider{name: "stub"} // history 为 nil、err 为 nil
	c := newGatedCollector(p, builtinGate())
	start, end := testRange()

	for i := 1; i <= 2; i++ {
		if _, err := c.FetchHistory("BTC", start, end, "1d"); err == nil {
			t.Fatalf("第 %d 次：空结果应报错", i)
		}
	}
	if got := p.count(); got != 2 {
		t.Errorf("空结果被写进了缓存：两次调用只向 provider 取了 %d 次, want 2。"+
			"校验一旦挪到闸门外，「无数据」这个结论会被冻结整个 TTL", got)
	}
}
```

**2×2 同环境闭环**（同 worktree、同会话）：

| | 正确实现 | C11 变异 |
|---|---|---|
| **现有测试集** | 绿 | **绿 ❌ 漏检** ← **这一格才是返工的理由** |
| **候选测试** | 绿 ✅ | **红 ✅**（`只向 provider 取了 1 次, want 2`）|

## 四、基线与约束

| 项 | 结果 |
|---|---|
| 测试规模 | **21 PASS / 0 FAIL / 0 SKIP** ✅ |
| `-race` | crypto + binance/coingecko/okx **四包全 ok** ✅ |
| 覆盖率（**单包口径**）| **73.9%** ≥ `coverage_floor` **70** ✅（既有水位 66.7%，上升 7.2pp）|
| 既有测试文件 | 改动只有 `crypto.go` + `gate_test.go`（新增），**既有 8 个用例一字未改** ✅ |
| scope | 全部落在 `./internal/collector/crypto/`，与 `writes` 一致 ✅ |
| 还原 | 每个变异 `cp` + `md5` 校验；收尾 `git status --porcelain` 空 ✅ |

## 五、变异验证结果表（11 个：10 捕获 / **1 存活** / 0 无效）

每个变异**注入前先写下预期**；四道门全程强制。

| ID | 变异 | 预期 | 实际 | 断言行 |
|---|---|---|---|---|
| **D1** | 缓存键退回 `UnixNano`（去 Truncate）| 红 | **红** ✅ | `:230` —— **且 `CacheKeyCoversAllParams` / `CachedByGate` 保持绿** |
| **D2** | 截断放粗到 `time.Hour` | 红 | **红** ✅ | `:245` —— 与 D1 互补，两个方向各自独立 |
| C1 | 绕过 Gate 直调 `fetchHistoryFromProviders` | 红 | **红** ✅ | 6 处；构造函数那条按预期保持绿 |
| C3 | 键丢 symbol | 红 | **红** ✅ | `:191`，**仅 /symbol 子测试** |
| C6 | 构造函数 `gate: nil` | 红 | **红** ✅ | `:265`/`:269`，**仅此一条**（陷阱 12）|
| C7 | 去掉 `slices.Clone` | 红 | **红** ✅ | `:380` |
| C8 | policy 错误映射 `%v`→`%w` | 红 | **红** ✅ | `:433`，**仅此一条** |
| C9 | 主题域段写错（`cryptoX.history`）| 红 | **红** ✅ | `:287` |
| C10 | 内置表给 `crypto.*` 加 `MinInterval` | 红 | **红** ✅ | `:352` |
| **C11** | **空结果校验挪出被缓存的 fn** | ?（我注入前标为不确定）| **绿 ❌ 存活** | — |

### 5.1 D1/D2 的闭环价值 —— 印证了 Leader 补 functional[4] 的必要性

**D1 只让 `/相邻时间落进同一槽` 转红，而 `TestCacheKeyCoversAllParams` 与
`TestFetchHistoryCachedByGate` 保持全绿。** 这是「原 criteria 只写区分度、放过了
`UnixNano` 缺陷」的**直接实测证据**，与 Dev 自述一致。

**D2 只让 `/分钟粒度不得放粗` 转红。** 两个子测试各守一个方向，缺一不可——
把 `Truncate` 放粗到小时能通过 D1 那条，但会让相隔几分钟的不同查询串槽。

> 这一点我要更正自己：我在 TASK-017 的报告 §三 给 baostock 的候选补丁**只有单向**
> （只断言亚分钟聚合），`Truncate(time.Hour)` 能骗过它。crypto 这个双向形态才是正解，
> 我已就此向 Leader 发过更正。

## 六、备录

1. **Dev 未自批 `coverage_floor`** 而是转 `blocked_clarification` 请 Leader 裁定，理由是
   「该字段虽在我可写范围内，但它是给自己的工作放宽质量门禁，不应由被约束方自批」——
   我认同这个边界判断，与我在 `questions` 字段上的处置同源（**权限允许不等于该做**）。
2. Dev 自述的 8 条变异我全部独立复现，**结论逐条一致**，未发现夸大。其自述「首版 C2 因留着
   未使用的 `slices` 导入而编译失败（断裂点②），已废弃重做」是诚实记录。
3. `boundary[0]` 中 Leader 作废的「三子源独立 Domain」要求：我核实其推理成立——
   Gate 包在 Collector 层而非 per-provider，且 `crypto.*` 无 `MinInterval`（`throttle` 首行
   `if p.MinInterval <= 0 { return }`），去不去 Domain 行为完全一致 = 等价变异。**不作为缺口。**

## 七、返工项（仅一条）

### R1（阻塞项）为 `error_handling[0]` 分句 2 补一条测试

- **实现无需改动**（`len(data) > 0` 已正确留在 fn 内）
- 加 §三 的 `TestEmptyResultIsNotCached`（我已验证 2×2 闭环有效）
- 在 gate_test.go 头部的 DoD↔测试映射注释里补上该分句
- 自证时请复现左下格：**注入 C11 后，只有新测试转红**

## 八、复现命令

```bash
git worktree add --detach ../wt-v016 0a1ed48 && cd ../wt-v016
GOTOOLCHAIN=local go test ./internal/collector/crypto/... -count=1 -race   # 四包全 ok
GOTOOLCHAIN=local go test ./internal/collector/crypto/ -count=1 -cover     # 73.9% ≥ 70

# C11（决定性）：crypto.go 里
#   ① `if err == nil && len(data) > 0 {` → `if err == nil {`
#   ② FetchHistory 的 `return slices.Clone(data), nil` 前加 `if len(data)==0 { return nil, err }`
GOTOOLCHAIN=local go test ./internal/collector/crypto/ -count=1            # 仍然 ok ← 缺口
# 加入 §三 候选测试后重跑同一变异 → 该测试转红（取数 1 次 vs want 2）

cd <主仓库> && git worktree remove ../wt-v016
```
