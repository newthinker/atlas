# TASK-003 验证报告 —— QuotaStore + MemStore + Gate 配额接线

- 验证者: test-agent-17
- 被验对象: `837542fde5084caf767157dadfff53db1bb157f3`（`quota.go` 88 行 + `quota_test.go` 348 行 + gate 接线，+462/-6）
- 验证环境: 独立 worktree `../wt-v003`（detached @ 837542f）
- assignment_epoch: 1 / rework_count: null（首轮）
- 上游 context: `.arcforge/discoveries/TASK-001.json`、`TASK-002.json`

## 结论：**NEEDS WORK（rejected）**

**先说清楚：这次的工作质量很高。** 50 个测试全 PASS 无 SKIP、`-race` 绿、覆盖率 98.3%
（`quota.go` 全部函数 100%）、17 包回归绿、scope 干净、三条否定断言都用了**可观测假账本**
（不是看计数值）、位置契约的两个方向（Q1/Q2）都有独立测试钉住、还主动补了两条
「不可达错误分支」测试。我另造的 8 个探索性变异也全部被捕获。

判 rejected 的原因集中在**一件事**：**Dev 把两次 panic 当成了断言捕获**，由此产生一个
真实的测试健壮性缺陷和两处不准确的覆盖声明。

| # | 问题 | 严重度 |
|---|---|---|
| **R1** | `TestWindowStartNilLocUsesUTC` 缺 `recover`，Q10 触发裸 panic **中断测试二进制，12 个测试未执行** | **MAJOR** |
| **R2** | 覆盖声明不准确：G8+ 与 Q10 的「红」都是 panic 而非断言；**G8+ 不能作为 boundary[2]③ 的证据** | MAJOR |

两条修复成本都很低（R1 加 4 行 recover，与同文件已写对的那条对齐；R2 换成我实测有效的
Q13 变异即可）。

---

## 一、先复现 Leader 点名的那件事：**我的 G8 前瞻确实被推翻**

我在 TASK-002 验证中推断「接入真实 QuotaStore 后 G8 不再等价，未登记主题会被计入配额」。
**独立复现结果：Dev 是对的，我错了。**

| 变异 | 结果 |
|---|---|
| **G8**（删掉 `fetch` 里未登记主题的直通短路）→ 定向 `-run TestUnregisteredTopicDoesNotTouchQuota` | **绿 ❌ 存活** |
| **G8**，跑全量 50 个测试 | **绿 ❌ 存活** |
| **对照（我补的）**：仅删 `takeQuota` 的 `p.Quota == nil` 短路，不删 G8 | **绿 ❌ 存活** |

断裂点确认在 `gate.go:308`：

```go
if p.Quota == nil || g.quota == nil { return nil }
```

未登记主题 → `Lookup` 返回 `(Policy{}, false)` → 零值 `Policy` 的 `Quota` 是 nil → 第二道短路。
「进入了 `takeQuota` 函数」与「触达了 `QuotaStore.Take`」确实是两件事。

**我补的那条对照更完整了因果图**：不是「第二道短路掩盖了 G8」，而是**两道短路各自独立充分**
——单删任何一道都不会触达配额记账。

Dev 的处置（保留测试但声明「它不是 G8 的守护者」）是对的。

---

## 二、但 **G8+ 组合不能作为那条 DoD 的证据**（R2）

Dev 报告 G8+（G8 + 删掉 `p.Quota == nil` 短路）→ `quota_test.go:289`，即认为
`TestUnregisteredTopicDoesNotTouchQuota` 捕获了它。**实测：那是 nil 指针解引用 panic。**

```
--- FAIL: TestUnregisteredTopicDoesNotTouchQuota (0.00s)
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x2 addr=0x0 pc=0x10486c944]
...
policy.(*Gate).takeQuota(...)
	.../gate.go:308 +0x64
```

删掉 `p.Quota == nil` 后，`*p.Quota` 对空指针解引用。**它证明的是「删掉 nil 检查会崩」，
不是「未登记主题会被计入配额」。** 而且这个形态会让**所有** `Quota` 为 nil 的主题崩溃，
不是「悄悄触达配额」的失效形态。

Dev 声称的 `:289` 是 panic 堆栈里出现的调用点行号，不是断言输出行。

### 我另造的 Q13 才是有效证据

让未登记主题拿到**带 Quota 的兜底策略**（不 panic 的真实失效形态）：

```go
p, ok := g.table.Lookup(topic)
if !ok {
    p = Policy{Quota: &Quota{Limit: 100, Window: 24 * time.Hour, Loc: time.UTC}}
}
```

结果：编译通过 ✅，**断言红**：

```
quota_test.go:289: 未登记主题不得触达配额记账（约束 C6）: Take 调用 3 次 [eastmoney.kline akshare.valuation crypto.ticker], want 0
```

**所以 `TestUnregisteredTopicDoesNotTouchQuota` 确实有守护价值** —— 它守的是「将来有人给
未登记主题配上默认配额策略」这类形态，而**不是** G8。覆盖声明应据此修正。

---

## 三、R1：`TestWindowStartNilLocUsesUTC` 缺 `recover`，一次失败会掩盖 12 个测试

`quota.go` 的注释写明了设计意图：

> `Loc` 为 nil 时按 UTC 自然日对齐：Quota 由调用方构造，**缺时区不该 panic**。

但测试只断言返回值，**没有断言「不 panic」**：

```go
func TestWindowStartNilLocUsesUTC(t *testing.T) {
	q := Quota{Limit: 1, Window: 24 * time.Hour} // Loc 为 nil
	...
	got := windowStart(now, q)          // ← 删掉兜底后这里直接 panic
	if !got.Equal(want) { ... }
}
```

注入 Q10（删掉 `loc == nil` 兜底）后实测：

| | 测试通过数 |
|---|---|
| 正常 | **50** |
| Q10 注入后 | **38** —— **12 个测试根本没跑到** |

裸 panic 会**中断整个测试二进制**，排在它之后的 12 个测试全部不执行，它们各自守护的 DoD
在那次运行中**完全失去检验**。这是「守卫在，但它倒下时会带倒一片」。

### 同一个文件里已经有写对的版本，直接对齐即可

`TestFailOpenWithoutWarnDoesNotPanic`（:321）的写法是正确的：

```go
defer func() {
	if r := recover(); r != nil {
		t.Fatalf("未注入 WithWarn 时 fail-open 不得 panic: %v", r)
	}
}()
```

实测 Q7（`New` 不给 `warn` 兜底）→ **断言红** `quota_test.go:324: 未注入 WithWarn 时
fail-open 不得 panic: runtime error: invalid memory address...` —— panic 被 recover 转成了
清晰的断言错误，**且不中断后续测试**。这与 TASK-001 的 `TestLoadLocFallsBackToUTCWithoutPanic`
是同一写法。

**修复**：给 `TestWindowStartNilLocUsesUTC` 加同样的 4 行 `defer recover()`。

> 备注：Q16（删掉 `g.quota == nil` 短路）也是 panic 型，但 `TestGateWithoutQuotaStoreAllows`
> 的 DoD 是「一律放行」而非「不 panic」，panic 只是附带后果，**不作为缺陷**。R1 的问题在于
> 测试的**设计意图**（注释明写「不该 panic」）与其断言不匹配。

---

## 四、Done Criteria 逐条覆盖矩阵

变异三道门：① `git diff --numstat` 非空；② `go test -c` 编译通过；
③ **核到断言错误行**（正则 `^\s+\w+_test\.go:\d+:\s+\S`，与 panic 堆栈里的裸文件行区分）。

| # | 完成标准（摘要） | 对应测试 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `windowStart`：>=24h 按 Loc 自然日对齐；<24h 按 UTC 截断 | `WindowStartAlignsNaturalDay` / `WindowStartTruncatesShortWindow` | Q6 红 `:83`、Q9 红 `:103` | **PASS** |
| functional[1] | `MemStore.Take` 放行到 Limit；窗口翻篇归零；主题账本隔离 | `MemStoreTakeUpToLimit` / 窗口/隔离用例 | Q15 红 `:137`、Q11 红 `:157`、Q12 红 `:140` | **PASS** |
| functional[2] | 配额用尽返回 `ErrQuotaExceeded` 且 **fn 一次都不被调用** | `GateQuotaExceededDoesNotCallFn` | Q14 红 `:189`、Q8 红 `:189` | **PASS** |
| functional[3] | `New` 签名变更，TASK-002 全部用例保持全绿 | 全量 50 测试 | 回归全绿 | **PASS** |
| boundary[0] | 被拒不计数；`QuotaStore` 为 nil 一律放行 | `MemStoreTakeUpToLimit`（Count 仍 3）/ `GateWithoutQuotaStoreAllows` | Q3 红 `:140`；Q16 panic（见 §三备注，可接受） | **PASS** |
| boundary[1] | fn 已执行但返回错误的请求**必须计数** | `GateCountsFailedRequests` | **Q17b 红 `:208`**（我造：takeQuota 挪到 fn 成功之后 → `Count = 0, want 1`） | **PASS** |
| boundary[2]① | 缓存命中不消耗配额 | `CacheHitDoesNotConsumeQuota` | Q1 红 `:184` | **PASS** |
| boundary[2]② | Coalesce 合并的 N 个请求只消耗 1 次配额 | `CoalescedRequestsConsumeOneQuota` | Q2 红 `:267`（`Take 调用 20 次, want 1`） | **PASS** |
| boundary[2]③ | **未登记主题不得触达配额记账** | `UnregisteredTopicDoesNotTouchQuota` | **Q13 红 `:289`**（Dev 声称的 G8+ 是 panic，不是证据——见 §二） | **PASS（但覆盖声明需修正）** |
| error_handling[0] 分句1 | 账本异常时放行全部请求 | `GateFailsOpenOnStoreError` | Q4 红 `:305` | **PASS** |
| error_handling[0] 分句2 | 经 `WithWarn` 上报该 err | 同上 | Q5 红 `:312` | **PASS** |
| error_handling[0] 分句3 | 未注入 `WithWarn` 时不 panic | `FailOpenWithoutWarnDoesNotPanic` | Q7 红 `:324`（**recover 写法正确**） | **PASS** |
| non_functional[0] | `-race` 全包全绿 | — | 实测绿 | **PASS** |

**13 项 DoD 全部有有效守护**。R1/R2 不是覆盖缺失，是**测试健壮性**与**覆盖声明准确性**问题。

---

## 五、位置契约（本任务核心价值）独立复现

`takeQuota` 必须在「查缓存后、singleflight 内侧、节流前」。三个方向都已钉死：

| 变异 | 方向 | 结果 |
|---|---|---|
| Q1 挪到**查缓存之前** | 缓存命中会吃掉配额 | **红** `:184` — `第 2 次: policy: quota exceeded` |
| Q2 挪到 **singleflight 外侧** | N 个合并请求各消耗一次 | **红** `:267` — `20 个合并请求只应消耗 1 次配额: Take 调用 20 次, want 1` |
| Q8 挪到 **fn 之后** | 配额用尽仍会发请求 | **红** `:189` — `err = <nil>, want ErrQuotaExceeded` |

这个位置此前是「碰巧正确、挪动不会有任何测试变红」，现在三个方向都有独立守护。

---

## 六、我自己的一次无效变异（据实记录）

我最初的 Q17 想验 boundary[1]，写成了：

```go
- return zero, err // 失败不写缓存、不延长 TTL
+ return zero, err
```

**只删了注释，语义完全没变**，结果自然是绿——我一度记为「存活」。门①（`git diff --numstat`
非空）放行了它，因为**注释也算改动**。

> **门① 的固有局限：`diff` 非空只证明字节变了，不证明语义变了。**
> 设计变异时必须确认改的是**行为**，注释/格式改动要排除。

重做为 Q17b（takeQuota 挪到 fn 成功之后）后拿到断言红 `:208`。这是本 Sprint 第三类
「变异无效」形态：① 表达式静默失配（改动量为空）；② 改坏代码致编译失败（红是构建的红）；
③ **改动量非空但语义未变**（本次）。

---

## 七、覆盖率、回归、约束、scope

| 项 | 结果 |
|---|---|
| 覆盖率 | **98.3%**；`quota.go` 的 `windowStart`/`take`/`NewMemStore`/`Take`/`Count` **全部 100%**；`gate.go` 的 `takeQuota` **100%** |
| 测试 | **50 个全 PASS，0 SKIP，0 FAIL** |
| `-race` | 绿（4.063s） |
| C3 不循环导入 | **仅 policy 自身 ✅**（未新增外部依赖） |
| gofmt / vet | 无输出 / exit 0 ✅ |
| 全量回归 | **17 包全部 ok ✅** |
| scope | `gate.go` +28/`gate_test.go` +4/`quota.go` +88/`quota_test.go` +348，全部落在 `./internal/collector/policy` 声明内 ✅ |
| 上游产物 | `git diff 6a2a8df -- policy.go policy_test.go` **无输出** → TASK-001 产物未被触碰 ✅ |

---

## 八、修复清单

**R1（MAJOR）** —— 给 `TestWindowStartNilLocUsesUTC`（quota_test.go:110）加 `recover`，
与同文件 `TestFailOpenWithoutWarnDoesNotPanic`（:321）对齐：

```go
func TestWindowStartNilLocUsesUTC(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Loc 为 nil 不得 panic: %v", r)
		}
	}()
	...
}
```

理由：`quota.go` 注释明写「缺时区不该 panic」，测试须断言这个意图；且裸 panic 会中断测试
二进制，实测掩盖 12 个后续测试。

**R2（MAJOR）** —— 修正 boundary[2]③ 的覆盖声明：G8+ 是 panic 不是断言捕获，**不能作为证据**。
改用 Q13（给未登记主题配上带 `Quota` 的兜底策略），已实测得到断言红 `:289`。
同时建议在测试注释里写明「本测试**不**守护 G8——G8 在两道短路下是等价变异体」，
避免下一位读者重走我踩过的推理链。

---

## 九、复现命令

```bash
git worktree add --detach ../wt-v003 837542fde5084caf767157dadfff53db1bb157f3
cd ../wt-v003/internal/collector/policy

GOTOOLCHAIN=local go test . -count=1 -race                      # 50 PASS 0 SKIP
GOTOOLCHAIN=local go test . -count=1 -coverprofile=/tmp/q.out   # 98.3%

# 变异三道门（缺一不可）：
#   ① git diff --numstat 非空，**且改的是语义不是注释**
#   ② go test -c -o /dev/null .  编译通过（否则红是构建的红）
#   ③ 输出须含断言行 `^\s+\w+_test\.go:\d+:\s+\S`（与 panic 堆栈的裸文件行区分）

# G8（我的前瞻，实为等价变异体）：删掉 fetch 里 `if !ok { return fn() }` → 全量 50 测试仍绿
# G8+（Dev 声称的证据，实为 panic）：G8 + 删掉 takeQuota 的 `p.Quota == nil` → nil 解引用 panic
# Q13（有效证据）：把 `if !ok { return fn() }` 换成
#     `p = Policy{Quota: &Quota{Limit: 100, Window: 24 * time.Hour, Loc: time.UTC}}`
#     → 断言红 quota_test.go:289 `Take 调用 3 次, want 0`
# Q10（R1 依据）：删掉 windowStart 的 `if loc == nil { loc = time.UTC }`
#     → 裸 panic，`go test -v` 的 PASS 数从 50 降到 38
```

worktree 已于验证结束后清理；主工作区 `internal/` 零污染。
