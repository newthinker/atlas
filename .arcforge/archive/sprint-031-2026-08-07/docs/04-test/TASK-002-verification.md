# TASK-002 验证报告 —— Gate：节流 / 合并 / TTL / 超时

- 验证者: test-agent-17
- 被验对象: `73b724bb97cb652ceffdbe27183591f0683d7f82`（`gate.go` 300 行 + `gate_test.go` 655 行，955 行新增）
- 验证环境: 独立 worktree `../wt-verify-T002`（detached @ 73b724b），`.arcforge/` 读写在主仓库
- assignment_epoch: 1 / rework_count: null（首轮验证）
- 上游 context: `.arcforge/discoveries/TASK-001.json`（已读）

## 结论：**NEEDS WORK（rejected）** —— 一条 DoD 的一半形态无守护，修复一行

先把话说在前面：**dev-agent-35 这次的工作质量是本 Sprint 我见过最高的。** 36 个测试全 PASS
无 SKIP、`-race` 绿、覆盖率 97.3%、7 个自证变异**全部经我独立复现属实**，而且它在 discovery
里**自曝了自己最初的测试设计是假绿**（见 §三）。判 rejected 不是因为它做得不好。

判 rejected 的原因是单一且具体的：**DoD functional[0] 点名要防的「throttle 期间持有全局锁」
有两种代码形态，现有的三个跨域测试只能捕获其中一种，另一种全部测不出。**

- **G1b-var2**（全局锁加在 `throttle` 的 `if p.MinInterval <= 0 { return }` **之前**）→ 捕获 ✅
  ——这是 dev 做过的那个变异，它的自述属实。
- **G1b-var1**（全局锁加在该 early-return **之后**，只保护真正节流的路径）→ **三个跨域测试全绿 ❌**

根因：测试把 b 域配成 `"b.x": {}`（`MinInterval=0`，完全不节流），于是 b 域走 `throttle` 的
early-return，**根本不进入持锁路径**——任何位于该 return 之后的全局锁，对 b 域都不可见。

修复是一行，我已实测封堵两种形态且无假红（§四）。

---

## 一、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 对应测试 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0]a | 同域相邻请求间隔 >= MinInterval；同域不同主题共享闸门 | `TestGateThrottlesSameDomain` | G1 红 ✅ | **PASS** |
| functional[0]b | **跨域必须并发**——两域同时发起互不阻塞（反审 D：防「throttle 期间持有全局锁」） | `TestGateThrottleIsPerDomain`（作者已标注不构成守护）/ `…IsPerDomainConcurrent` / `…DoesNotHoldGlobalLock` | G1 红 ✅（定向）；**G1b-var2 红 ✅ / G1b-var1 绿 ❌ 存活** | **FAIL** |
| functional[1]a | Coalesce=true 时 20 并发同 key 只触发 1 次 fn 且全部拿到正确返回值 | `TestGateCoalescesConcurrentSameKey` | G1 定向复核通过 | **PASS** |
| functional[1]b | 不同 key 不合并 | `TestGateCoalesceIsPerKey` | — | **PASS** |
| functional[1]c | **缓存 key 与 singleflight key 均含 topic**（反审 A5） | `TestCacheKeyIncludesTopic` / `TestCoalesceKeyIncludesTopic` | G2 定向**双红** ✅ | **PASS** |
| functional[2]a | TTL 内只调 fn 一次；过期后重调 | `TestGateTTLHitSkipsFn` / `TestGateTTLExpires` | G11 红 ✅ | **PASS** |
| functional[2]b | **缓存命中不等待 MinInterval**（执行链 ① 在 ④ 前，反审 A2） | `TestCacheHitSkipsThrottle` | G3 定向红 ✅ | **PASS** |
| functional[3] | Do 强制 TTL=0 每次执行 fn；Wait 只节流不碰缓存/合并/配额 | `TestDoForcesNoCache` / `TestWaitThrottlesWithoutFn` / `TestWaitDoesNotCache` | G13 红 ✅、G14 红 ✅ | **PASS（但 DoD 措辞歧义，见 §五）** |
| boundary[0] | 未登记主题直通：不缓存不节流，3 次 Fetch 触发 3 次 fn 且 <50ms | `TestGateUnregisteredTopicPassesThrough` | **G8 绿 —— 判为等价变异体**（§六） | **PASS** |
| boundary[1] | nil `*Gate` 完全透明 | `TestNilGateIsTransparent` | G9 红 ✅ | **PASS** |
| boundary[2] | 超过 maxCacheEntries 淘汰最旧 | `TestGateEvictsOldestEntry` | G10 红 ✅ | **PASS** |
| error_handling[0]a | fn 出错不写缓存；超时返回 ErrTimeout 且不写缓存 | `TestGateDoesNotCacheErrors` / `TestGateTimeout` / `TestGateTimeoutDoesNotCache` | G12 红 ✅、G12b 红 ✅ | **PASS** |
| error_handling[0]b | **Coalesce 路径下 fn 失败时全部等待者共享同一错误**（反审 A6） | `TestCoalesceSharesErrorWithAllWaiters` | G4 定向红 ✅ | **PASS** |
| non_functional[0] | `-race` 全绿；runWithTimeout 不泄漏 goroutine；Timeout=0 语义为不限时 | `-race` 实测 / `TestRunWithTimeoutDoesNotLeakGoroutine` / `TestZeroTimeoutMeansNoLimit` | G6 红 ✅、G5 定向红 ✅ | **PASS** |

**13 项中 12 项 PASS，1 项 FAIL（functional[0]b）。**

---

## 二、变异验证结果（22 次注入：20 捕获 / 2 存活）

每次注入后先取 `git diff --numstat` 校验改动量非空（为空即判「变异无效、锚定失败」并跳过，
绝不把「没改代码的绿」记成存活）；还原后 `md5` 与基线比对。

### 2.1 复核 dev 自证的 7 个变异 —— 全部属实

| ID | 变异 | 结果 | 捕获者与实测输出 |
|---|---|---|---|
| G1 | `domainState` 退化为全局共享一个 state | **红 ✅** | `IsPerDomainConcurrent` — `跨域请求被串行化，总耗时 603.1ms；按域隔离应 ~300ms，全局锁则 ~600ms` |
| G2 | 缓存/合并键去掉 topic | **红 ✅** | `CacheKeyIncludesTopic` — `a.x 的 fn 调用 2 次, want 1`；`CoalesceKeyIncludesTopic` — `xCalls=1, yCalls=0, want 1/1` |
| G3 | `throttle` 提到查缓存之前 | **红 ✅** | `CacheHitSkipsThrottle` — `缓存命中仍等了节流 500.9ms（查缓存必须先于节流）` |
| G4 | fn 失败时返回零值+nil error | **红 ✅** | `CoalesceSharesErrorWithAllWaiters` — `waiter 0 拿到 err = <nil>, want boom` |
| G5 | `Timeout=0` 实现成立即超时（`d<=0` 改 `d<0`） | **红 ✅** | `ZeroTimeoutMeansNoLimit` — `got (0, policy: fn timeout), want (42, nil)` |
| G6 | 结果 channel 去掉缓冲 | **红 ✅** | `RunWithTimeoutDoesNotLeakGoroutine` — `NumGoroutine = 5, base = 4` |
| G7 | 删掉 `New` 里的 opts 应用循环 | **红 ✅** | `WithWarnIsApplied` — `WithWarn 注入的回调未被调用` |

> **定向验证**：上表不是「套件转红」就算数——套件转红可能是别的测试抓到的，而 DoD 指定的
> 专属守护者仍可能空洞（这正是 TASK-001 的 M7→M21 教训）。故 G1/G2/G3/G4/G5 均**单独
> `-run` 了 DoD 指定的那个测试**再确认，上表输出即定向运行的结果。

### 2.2 我补的 DoD 覆盖变异

| ID | 变异 | 结果 | 捕获者 |
|---|---|---|---|
| G9 | 去掉 `if g == nil { return fn() }` | **红 ✅** | `NilGateIsTransparent`（panic 转 FAIL） |
| G10 | `store` 去掉淘汰逻辑 | **红 ✅** | `EvictsOldestEntry` — `缓存条目 522 超过上限 512` |
| G11 | `load` 不检查 TTL 过期 | **红 ✅** | `GateTTLExpires` — `TTL 过期后应重新调用 fn, got 1` |
| G12 | fn 出错也写缓存 | **红 ✅** | `DoesNotCacheErrors` — `call 1: err = <nil>, want boom` |
| G12b | 超时也写缓存 | **红 ✅** | `TimeoutDoesNotCache` — `超时不得写缓存: got (0, <nil>), want (7, nil)` |
| G13 | `Do` 不再强制 TTL=0 | **红 ✅** | `DoForcesNoCache` — `Do 调用 1 次 fn, want 3` |
| G14 | `Wait` 不再节流 | **红 ✅** | `WaitThrottlesWithoutFn` — `Wait 应施加节流, 第二次耗时 83ns` |
| **G8** | 去掉未登记主题的直通短路 | **绿 ❌ 存活** | 无 —— **判为等价变异体**，见 §六 |

### 2.3 G1b 的两种形态（本轮核心发现）

| ID | 全局锁加在哪 | `IsPerDomain` | `IsPerDomainConcurrent` | `DoesNotHoldGlobalLock` |
|---|---|---|---|---|
| **G1b-var2** | `if p.MinInterval <= 0 { return }` **之前** | 绿 | 绿 | **红 ✅** |
| **G1b-var1** | 该 early-return **之后**（只锁真正节流的路径） | 绿 | 绿 | **绿 ❌** |

var2 的实测输出：`第 0 轮: a 域节流期间 b 域被阻塞 480.0ms —— throttle 持有了跨域的全局锁`。
var1 下三个测试连同全量套件都是 `ok`。

---

## 三、dev 的自查质量（据实记录，与判定无关但值得团队知道）

dev-agent-35 在 discovery 里**自曝了自己最初的测试设计是假绿**：

> 它原本只设计了双域对称版，并论证「无论调度顺序如何，全局锁必然串行 600ms」。
> **自己注入变异时发现这个论证是错的**：`throttle` 等的是**绝对时刻** `lastReq+MinInterval`，
> 不是「轮到我时再等一个 MinInterval」。两域几乎同时预热时，A 持全局锁等到 T+300ms 释放后，
> B 要等的绝对时刻也已经过了，于是立即返回——总耗时与按域隔离无异。

它据此补了非对称版并用两个变异实证分工，还在 `TestGateThrottleIsPerDomain` 上明确标注
「**它不构成守护**，保留仅作基础回归」。它另外诚实声明了 `DoesNotHoldGlobalLock` 里那个
20ms sleep 是**概率性**的、「猜错的后果是**假绿而非假红**」并用 `rounds=3` 降低漏检。

**本轮发现的 G1b-var1 正是这条论证链的延伸**：dev 找到了「等绝对时刻 vs 等相对间隔」这个
关键差异并据此补了测试，但补出来的测试为了制造「B 的等待时刻显著晚于 A 的完成时刻」而
把 b 域配成完全不节流——**这个选择恰好让 b 域绕开了 `throttle` 的持锁路径**。方向是对的，
落点差一步。

---

## 四、修复方案（一行，已实测）

```go
// TestGateThrottleDoesNotHoldGlobalLock 内：
- "b.x": {},                                  // 不节流：正确实现下必须立即返回
+ "b.x": {MinInterval: time.Millisecond},     // 极短间隔：仍进入持锁路径，正确实现下几乎立即返回
```

要点：b 域必须 **`MinInterval > 0`** 才会进入 `throttle` 的持锁临界区；间隔取极小值（1ms）
保证正确实现下几乎立即返回，不影响 150ms 的判定阈值。

实测（在 worktree 试装，**未提交**）：

| 场景 | 结果 |
|---|---|
| 修复版 + G1b-var1（现行测不出的形态） | **红 ✅** `a 域节流期间 b 域被阻塞 478.3ms` |
| 修复版 + G1b-var2（dev 原变异） | **红 ✅** `a 域节流期间 b 域被阻塞 479.5ms` |
| 修复版 + 正确实现，连跑 **5 次** | **全绿 ✅**（无假红，1.69~1.89s 稳定） |

建议同时更新该函数的注释：现有注释说「b 域完全不节流：正确实现下必须立即返回」，
应改为说明「b 域必须**也进入持锁路径**（故 MinInterval 取极小正值而非 0），否则
early-return 会让它绕开任何在 `MinInterval` 检查之后加的锁」。

---

## 五、DoD 侧问题：functional[3] 措辞歧义（需 Leader 定口径，不计入 dev 返工）

DoD functional[3] 写的是「**Do 强制 TTL=0 每次都执行 fn**」，而实现与 `Do` 的文档注释写的是
「Do 仍走同一个 fetch，**因此照样受 singleflight 合并**；内置表所有主题 Coalesce 均为 true，
所以 20 个并发 `Do` 只会真正发生 1 次副作用」。**并发场景下这两句互相矛盾。**

我采用的口径与依据：functional[1] 已统一规定了合并行为适用于所有走 `fetch` 的路径，若
functional[3] 要求 `Do` **不被合并**，就与 functional[1] 直接冲突；故「每次都执行」应读作
「不被**缓存**吞掉」，而非「不被**合并**」。据此判 **PASS**——G13（`Do` 不再强制 TTL=0）
在顺序调用下转红（`Do 调用 1 次 fn, want 3`），该口径下有有效守护。

**这是措辞歧义，不是实现缺陷，也不是 dev 的责任。** 若 Leader 口径不同（要求 `Do` 并发下
也每次执行），则实现需改、本条改判 FAIL，且应记为 `dod_defect` 而非 `task_defect`。
建议无论如何把 DoD 措辞改为「Do 强制 TTL=0，不被缓存吞掉（仍受 singleflight 合并，
需要每次都发生时用不同 key 或关闭该主题 Coalesce）」以消除歧义。

---

## 六、G8 判为等价变异体（不计入 FAIL），但有一条**前瞻风险**

G8 删掉 `fetch` 里的 `if !ok { return fn() }`（未登记主题直通），全量 36 测试含 `-race`
**全绿**。这不是测试缺陷——未登记主题 `Lookup` 返回 `(Policy{}, false)`，零值 `Policy` 的
每个字段都让后续步骤短路：`ttl=0` 不查也不写缓存、`Coalesce=false` 不走 singleflight、
`MinInterval=0` 使 `throttle` early-return、`Timeout=0` 使 `runWithTimeout` 直接调 fn、
`takeQuota` 是放行一切的桩。**行为完全等价**，该 `if` 是快速路径与意图表达，不是行为分支。

⚠ **但它只在 `takeQuota` 还是桩的时候等价。** 已实测验证：

| 场景 | 未登记主题是否触达 `takeQuota` |
|---|---|
| 注入 G8 + 让 `takeQuota` panic | **触达**（测试 panic 转 FAIL） |
| 不注入 G8 + 让 `takeQuota` panic | **不触达**（`ok`） |

即：**TASK-003 接入真实 QuotaStore 后，一旦有人删掉或绕过这个直通短路，未登记主题就会
被计入配额**——违反约束 C6「未登记 collector 行为零变更」——而
`TestGateUnregisteredTopicPassesThrough` 届时**仍然测不出**（它只断言 fn 调用次数与总耗时）。

给 TASK-003 的建议（不是本任务的返工项）：接入 QuotaStore 时补一条断言「未登记主题不触达
配额记账」，例如用注入的 QuotaStore 桩记录调用次数并断言为 0。

---

## 七、覆盖率、race、回归、约束、scope

### 7.1 覆盖率

```
ok  github.com/newthinker/atlas/internal/collector/policy  3.163s  coverage: 97.3% of statements
```

`gate.go` 逐函数：`WithWarn` / `New` / `Fetch` / `Do` / `Wait` / `throttle` / `domainState` /
`load` / `store` / `evictOldest` / `entryCount` / `runWithTimeout` / `takeQuota` **均 100%**；
`fetch` **91.2%**。未覆盖的三处均非 DoD 要求，**不计入判定**：

| 位置 | 内容 | 说明 |
|---|---|---|
| `gate.go:78` | `warn: func(string, error) {}` 默认 no-op 的空函数体 | 无语句可覆盖 |
| `gate.go:164-167` | `call` 内**二次检查缓存**（合并窗口内已有人写好） | 竞态窗口内路径，难确定性触发；纯优化，不改可观测行为 |
| `gate.go:169-171` | `takeQuota` 返回错误的分支 | 桩恒返 nil，TASK-003 才可达 |

### 7.2 测试与 race

- **36 个测试全 PASS，0 SKIP，0 FAIL**
- `go test -race` 绿（3.998s）

### 7.3 约束与回归

| 检查 | 结果 |
|---|---|
| C3 不循环导入（`go list -deps \| grep newthinker/atlas`） | **仅 policy 自身 ✅**（新增依赖仅 `golang.org/x/sync/singleflight`） |
| gofmt | 无输出 ✅ |
| vet（`./internal/collector/...`） | exit 0 ✅ |
| 全量回归（`go test ./internal/collector/... -count=1`） | **17 包全部 ok ✅** |

### 7.4 scope

```
73b724b  internal/collector/policy/gate.go      | 300 ++++
         internal/collector/policy/gate_test.go | 655 ++++
         2 files changed, 955 insertions(+)
```

严格落在 `packages`/`writes` 声明的 `./internal/collector/policy` 内。**无越界申报 ✅**
另核实 `policy.go` / `policy_test.go` 的 md5 与 TASK-001 验收基线**逐字节一致**
（`89da8d21…` / `67d68a6c…`）——本任务未动上游产物，TASK-001 的结论不受影响。

---

## 八、复现命令

```bash
git worktree add --detach ../wt-verify-T002 73b724bb97cb652ceffdbe27183591f0683d7f82
cd ../wt-verify-T002

GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -race        # 36 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -coverprofile=/tmp/t2.out
GOTOOLCHAIN=local go tool cover -func=/tmp/t2.out                            # 97.3%
GOTOOLCHAIN=local go test ./internal/collector/... -count=1                  # 17 包全绿

# G1b-var1 复现（存活变异，本轮 FAIL 依据）——在 Gate 加 globalMu，并在 throttle 里
# 【MinInterval 检查之后】加锁：
#   func (g *Gate) throttle(p Policy) {
#       if p.MinInterval <= 0 { return }
#       g.globalMu.Lock(); defer g.globalMu.Unlock()      // ← 变异
#       d := g.domainState(p.Domain)
#       ...
# 然后：go test . -run '^TestGateThrottleDoesNotHoldGlobalLock$'   → 仍然绿（即缺口）
#
# G1b-var2 对照（dev 做过的那个）——把 globalMu.Lock() 移到 MinInterval 检查【之前】
#   → 同一条测试转红（证明 dev 自述属实，只是形态覆盖不全）
#
# 修复验证：把测试里 "b.x": {} 改成 "b.x": {MinInterval: time.Millisecond}
#   → var1 红、var2 红、正确实现连跑 5 次全绿

# 基线 md5（还原后须复原到）:
#   gate.go        = b350bcaa919e8b9a595e096c83a72216
#   gate_test.go   = f400ec5ad2940f70bff9bc845aab08df
#   policy.go      = 89da8d213e62570dc8c90dee89d1ddd4   （与 TASK-001 基线一致）
#   policy_test.go = 67d68a6c946d18f39ee2557c5503cb03   （与 TASK-001 基线一致）
```

worktree 已于验证结束后 `git worktree remove ../wt-verify-T002` 清理；
主工作区 `internal/` 全程零污染。
