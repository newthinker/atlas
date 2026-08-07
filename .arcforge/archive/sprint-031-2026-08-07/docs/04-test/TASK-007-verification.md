# TASK-007 验证报告 —— yahoo 接入 policy Gate

- 验证者: test-agent-17
- 被验对象: `1c08953`（父 `b0bc78a`；`yahoo.go` +92-37 / `eps.go` +16 / `gate_test.go` +306 / `throttle_test.go` +5-8）
- 验证环境: 独立 worktree `../wt-v007`（detached @ 1c08953）
- assignment_epoch: 1 / rework_count: null（首轮，不设搜索限制）

## 结论：**PASS（verified）**

8 项 DoD 全部有有效守护，**40 测试 0 SKIP**、`-race` 绿、覆盖率 **87.3%**（绝对下限 86.0 ✅）、
17 包回归绿、私有节流符号已从实现中移除、既有测试 10→10 条符合 Leader 裁定。

**一项报告给 Leader 决定的守护缺口**（不判 FAIL，DoD 未要求）：
**`FetchEPSHistory` 的 `slices.Clone(pts)` 无任何测试守护** —— 删掉后全量 40 测试全绿。
与 `FetchHistory` 的同类性质，但 DoD boundary[0] 只点名了 `FetchHistory`。**修复成本极低**（§四）。

---

## 一、变异验证（预期注入前写下）

| ID | 变异 | 预期 | 实际 | 断言位置 |
|---|---|---|---|---|
| **Y1** | `do()` 首发也 `Wait`（多等一倍） | 红 | **红** ✓ | `gate_test.go:303: 首次请求等了 198.3ms（约 100ms 的两倍）` |
| **Y2** | `FetchQuote` 误用 chart 主题 | 红 | **红** ✓ | `:232: chart+quote 命中 2 次, want 3` |
| **Y3b** | `FetchHistory` 不 `Clone` | 红 | **红** ✓ | `:129: 缓存命中必须返回独立副本，否则调用方能污染缓存` |
| **Y4b** | `FetchHistory` 不经 Gate | 红 | **红** ✓ | `:107: TTL 内应只发 1 次 HTTP 请求, got 3` |
| **Y5c** | `FetchQuote` 不经 Gate（失去节流） | 红 | **红** ✓ | `:176: quote 不缓存但仍应受 yahoo 闸门节流, 间隔 816µs` |
| **Y6b** | `FetchEPSHistory` 误用 chart 主题 | 红 | **红** ✓ | `:236: eps 命中 1 次, want 2；为 1 说明误用了带 TTL 的主题` |
| **Y8** | **`FetchEPSHistory` 不 `Clone`** | 红 | **绿 ❌** | 无 —— 见 §四 |

五道门 + 预期列全程执行。**我自己有 4 处不符/无效**，全部诊断到根因（§五）。

### 1.1 「多等一倍」型缺陷确实被上界断言钉住（Y1）

我预读时列的重点之一：这类缺陷若测试只断言下界（`>= iv`）就测不出，因为多等一倍仍满足。
实测确认 `TestFirstRequestDoesNotWaitTwice` 用的是**上界**（`elapsed > iv+50ms` 即红），
Y1 下报 `198.3ms`。**判据选对了。**

### 1.2 functional[1] 的两个分句各自有独立证据

| 分句 | 守护者 | 变异 |
|---|---|---|
| `quote` TTL=0 **不被缓存** | `TestFetchQuoteIsNotCached` | Y2 红 |
| ……**但仍受 500ms 节流** | `TestFetchQuoteStillThrottled`（方案未覆盖，Dev 自补） | **Y5c 红** |

与 TASK-004 那对复合句分句同构。Dev 在文件头注释里也点明了：
「只验前者时，把 quote 主题排除出限流域也照样绿」——**这个判断属实，Y5c 即其反证。**

### 1.3 `TestEachMethodUsesItsOwnTopic` 解决了一个真盲区

其余用例用 `gateWith` 给三个主题登记**同一份策略**，于是「把 `FetchQuote` 的主题写成
`topicChart`」也照样绿。Dev 反过来给 chart 单独配 TTL、另两个不配，用「是否被缓存」区分主题。
Y2/Y6b 双双转红证实有效。

**后果不是纸面的**（Dev 注释）：生产内置表里 chart 的 TTL 是 5 分钟，`FetchQuote` 若误用
chart 主题会被缓存 5 分钟，「实时报价」直接失效。

---

## 二、Dev 主动补的限定，正是我预读时担心的那点

我在预读时报告过：TASK-007 DoD boundary[0] 的「**必须自行 `slices.Clone`**」是**无条件**表述，
孤立阅读会误导（元素含 map/slice 时不适用）。

**Dev 在代码里自己补上了限定**（`yahoo.go:299-301`）：

> `core.OHLCV` 今天全是值类型，浅元素拷贝即深拷贝——**这个前提由 core/types.go 保证，
> 不是这里**；若将来给它加了 map/slice/指针字段，这里必须改成逐元素深拷贝。

`eps.go` 里也有同样的限定。**这比 DoD 的无条件措辞准确**，且「前提由 core/types.go 保证，
不是这里」这句把责任边界写清楚了 —— 与 TASK-009 tushare 那边的 map 情形正好构成对照。

---

## 三、既有测试改动符合 Leader 裁定

| 项 | 结果 |
|---|---|
| 测试函数数量 | **10 → 10**，一条不删 ✅ |
| 改动内容 | 加 `policy` import；删 7 处 `y.minInterval = 0`（裁定允许①）；`TestDoThrottlesConsecutiveRequests` 改为 `y.gate = gateWith(policy.Policy{MinInterval: 80ms})`（裁定允许②） |
| 断言与场景 | **未变** —— 仍是 80ms 闸门、仍断言相邻间隔 ≥60ms |
| 错误路径测试 | 一条未动 ✅ |

符合裁定的「允许①②③、不允许删除错误路径测试或改其断言与场景」。

`non_functional[0]`（`verify_by: review`）：`grep 'lastReq|minInterval|func.*throttle'` 在
**实现文件**中无匹配（唯一命中是 `gate_test.go` 的一句注释）。**符合 ✅**

---

## 四、报告项：`FetchEPSHistory` 的 `Clone` 无守护（请 Leader 决定）

**Y8**：把 `eps.go` 的 `return slices.Clone(pts), nil` 改成返回原切片（保留 `slices` 引用以通过
门②）→ **全量 40 测试全绿**。

`FetchEPSHistory` 与 `FetchHistory` 一样走 Gate 缓存（`policy.Fetch(y.gate, topicEPS, ...)`），
共享同一底层数组的问题**完全相同**，Dev 也确实写了 `slices.Clone(pts)` 并加了限定注释 ——
**实现是对的，只是没有测试守护它。**

**为什么不判 FAIL**：DoD boundary[0] 明文只点名 `FetchHistory`
（「缓存命中时 **FetchHistory** 返回独立切片……(TestFetchHistoryReturnsIndependentSlice)」），
未要求 EPS。按字面 PASS。

**但与 TASK-006 的 serve 那条不同，这次修复成本极低**：照抄
`TestFetchHistoryReturnsIndependentSlice` 换成 `FetchEPSHistory` 即可，**不需要任何重构**。
分类为守护缺口（实现正确、无守护），我倾向值得补 —— 但 DoD 是否扩围由 Leader 定。

---

## 五、我自己的 4 处不符/无效变异（方法论留痕）

| ID | 现象 | 根因 |
|---|---|---|
| Y3/Y4/Y5 | 门②拦下（编译失败） | 删掉唯一使用点导致 `slices` / `policy` **import 未使用**、`key` 变量未使用 —— **我在 TASK-001 栽过同一个坑，又踩了**。用 `if false { _ = ... }` 保留引用后重做成功 |
| Y6 | 预期红实际绿 | 等价变异体：EPS 从 `topicEPS` 改到 `topicQuote`，而该测试里两者**都不配 TTL**，行为不变。改用 `topicChart`（有 TTL 的那个）后转红 |
| Y7 | 预期红实际绿 | **变异打偏**：我改主题名想改变限流域，但 `gateWith` 对三个主题**显式设了 `Domain: "yahoo"`**，改名不影响域。生产表里的同域由 policy 包的 `TestLookupBuiltinTopics` 守护，本包测的是「两个方法都经闸门」 |
| Y8 | 预期红实际绿 | **真缺口**（§四） |

四处里三处是我的问题、一处是真缺口 —— **预期列再次把「我的变异有问题」与「代码有缺口」区分开**。
其中 Y3/Y4/Y5 是门②直接拦下的，若无门②会被记成「存活」，那将是三个不存在的缺陷。

---

## 六、覆盖率、回归、约束、scope

| 项 | 结果 |
|---|---|
| 测试 | **40 个全 PASS，0 SKIP，0 FAIL** |
| `-race` | 绿（23.2s） |
| 覆盖率 | **87.3%**（DoD 绝对下限 **86.0** ✅，按 Leader 统一口径） |
| collector 树回归 | 17 包全绿 ✅ |
| `go vet ./internal/collector/yahoo/` | 通过 ✅ |
| scope | 仅 `internal/collector/yahoo/` 四个文件 ✅ |

> **一处既有问题备录（非本次引入）**：`gofmt -l internal/collector/yahoo/` 报
> `yahoo_test.go` 未格式化。核实：该文件**不在本次改动内**（`git diff` 无输出），
> 且**父提交上就已不干净**（差异是 Go 1.19+ 注释缩进规则）。不计入本次判定，
> 建议 Sprint 末尾统一 `gofmt -w`。

---

## 七、复现命令

```bash
git worktree add --detach ../wt-v007 1c08953
cd ../wt-v007/internal/collector/yahoo

GOTOOLCHAIN=local go test . -count=1 -race            # 40 PASS 0 SKIP
GOTOOLCHAIN=local go test . -count=1 -cover           # 87.3% ≥ 86.0

# Y1 去掉 do() 的 `if attempt > 0` 守卫 → :303 红（上界断言，报「约两倍」）
# Y2 FetchQuote 用 topicChart          → :232 红
# Y3b/Y8 去掉 slices.Clone（须用 `if false { _ = slices.Clone(x) }` 保留 import，否则编译失败）
#   FetchHistory → :129 红 ；**FetchEPSHistory → 全量 40 测试全绿（无守护）**
# Y4b FetchHistory 不经 Gate（须 `_ = key` 保留变量）→ :107 红
# Y5c FetchQuote 不经 Gate             → :176 红
# Y6b FetchEPSHistory 用 topicChart    → :236 红
#   （用 topicQuote 是等价变异体：该测试里 eps 与 quote 都不配 TTL）

# 五道门 + 预期列：注入前写预期；不符必须诊断到根因
```

worktree 已于验证结束后清理；主工作区零污染。
