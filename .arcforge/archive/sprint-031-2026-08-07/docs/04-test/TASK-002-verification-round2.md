# TASK-002 验证报告（第二轮 / 返工复核）—— Gate：节流 / 合并 / TTL / 超时

- 验证者: test-agent-17
- 被验对象: `487e2ba8e1911729776aee7acab18bd9b0992e89`（`test(collector): 跨域测试的对照域须 MinInterval>0 才进入持锁路径`）
- 验证环境: 独立 worktree `../wt-v002r2`（detached @ 487e2ba），`.arcforge/` 读写在主仓库
- assignment_epoch: **2** / rework_count: 1
- 前序：首轮报告 `TASK-002-verification.md`（rejected）、增补 `TASK-002-verification-addendum.md`

## 结论：**PASS（verified）**

首轮唯一的阻塞项（G1b 只覆盖一种形态）已修复并经**独立复现**：两种全局锁形态现在**双双转红**，
定向运行与全量套件结果一致。10 条防回归变异定向抽查**全部仍捕获**，无一削弱。
覆盖率 97.3% 保持、36 测试全 PASS **0 SKIP**、`-race` 连跑 3 次绿、collector 树 17 包回归绿、
C3/gofmt/vet 干净、scope 仅 `gate_test.go`（11+/2-）。

**`gate.go` 的 md5 与首轮基线逐字节相同**（`b350bcaa919e8b9a595e096c83a72216`）——
纯测试修复，实现零改动。

---

## 一、首轮阻塞项的修复复核（本轮核心）

### 1.1 修复内容

```go
- "b.x": {}, // 不节流：正确实现下必须立即返回
+ // b 域必须 MinInterval > 0：throttle 开头有 `if p.MinInterval <= 0 { return }`
+ // 的快速路径，配成 0 的话 b 域**根本不进入持锁临界区**，任何加在该
+ // early-return 之后的全局锁对它都不可见。取极小正值：既进入临界区，
+ // 正确实现下又几乎立即返回，不影响下面的 150ms 阈值。
+ "b.x": {MinInterval: time.Millisecond},
```

dev 同时把根因写进了函数头注释，并明确了「加在 early-return **之前** / **之后**」两种形态的
区别以及「先做快速路径检查再拿锁是很自然的写法」。**根因记录到位，不只是改了个数值。**

### 1.2 两种形态的独立复现

每次注入后先取 `git diff --numstat` 校验改动量非空；还原后 `md5` 与基线断言相等（不等即中止）。

| 变异 | 全局锁加在哪 | 定向 `-run DoesNotHoldGlobalLock` | 全量套件 |
|---|---|---|---|
| **G1b-var1** | `if p.MinInterval <= 0 { return }` **之后**（首轮存活的那个） | **红 ✅** `第 0 轮: a 域节流期间 b 域被阻塞 480.45ms —— throttle 持有了跨域的全局锁` | **红 ✅** `480.09ms` |
| **G1b-var2** | 该 early-return **之前**（dev 首轮做过的） | **红 ✅** `480.53ms` | **红 ✅** `480.81ms` |

**首轮的存活变异现在被捕获，且原本能捕获的那个没有丢失。修复有效。**

### 1.3 假红检查（改动引入了新的时序常量，须排除抖动）

`b.x` 从「完全不节流」改成 `MinInterval: 1ms` 后，b 域每次都要进入持锁临界区并可能等待
至多 1ms —— 相对 150ms 阈值有 150 倍余量，但仍实测确认：

| 检查 | 结果 |
|---|---|
| `DoesNotHoldGlobalLock` 连跑 **8 次** | **全绿 ✅** |
| 全量套件（含 `-race`）连跑 **3 次** | **全绿 ✅** |

**未引入假红。**

---

## 二、防回归抽查（10 条，全部定向到 DoD 专属守护者，全部仍捕获）

首轮教训：不以「套件转红」为准——套件红可能是别的测试抓到的，DoD 指定的专属守护者仍可能空洞。
故本轮全部用 `-run` 定向到该条 DoD 的守护者。

| ID | 变异 | 定向到 | 结果 | 实测输出 |
|---|---|---|---|---|
| G1 | `domainState` 全局共享一个 state | `IsPerDomainConcurrent` | **红 ✅** | `跨域请求被串行化，总耗时 602.03ms；按域隔离应 ~300ms，全局锁则 ~600ms` |
| G2a | `ck` 去掉 topic | `CacheKeyIncludesTopic` | **红 ✅** | `a.x 的 fn 调用 2 次, want 1` |
| G2b | `ck` 去掉 topic | `CoalesceKeyIncludesTopic` | **红 ✅** | `xCalls = 1, yCalls = 0, want 1/1（0 表示该组被跨主题合并掉了）` |
| G3 | `throttle` 提到查缓存之前 | `CacheHitSkipsThrottle` | **红 ✅** | `缓存命中仍等了节流 500.96ms（查缓存必须先于节流）` |
| G4 | fn 失败返零值+nil error | `CoalesceSharesErrorWithAllWaiters` | **红 ✅** | `waiter 0 拿到 err = <nil>, want boom` |
| G5 | `d<=0` 改 `d<0`（Timeout=0 变立即超时） | `ZeroTimeoutMeansNoLimit` | **红 ✅** | `got (0, policy: fn timeout), want (42, nil)` |
| G6 | 结果 channel 去掉缓冲 | `RunWithTimeoutDoesNotLeakGoroutine` | **红 ✅** | `超时后 fn 的 goroutine 未退出: NumGoroutine = 3, base = 2` |
| G7 | 删掉 `New` 的 opts 应用循环 | `WithWarnIsApplied` | **红 ✅** | `WithWarn 注入的回调未被调用` |
| G13 | `Do` 不再强制 TTL=0 | `DoForcesNoCache` | **红 ✅** | `Do 调用 1 次 fn, want 3` |
| G14 | `Wait` 不再节流 | `WaitThrottlesWithoutFn` | **红 ✅** | `Wait 应施加节流, 第二次耗时 416ns` |

**这一行测试改动没有削弱任何既有断言。**

---

## 三、Done Criteria 逐条覆盖矩阵（13 项全 PASS）

| # | 完成标准（摘要） | 对应测试 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0]a | 同域串行；同域不同主题共享闸门 | `ThrottlesSameDomain` | G1 红 ✅ | **PASS** |
| functional[0]b | **跨域必须并发**（防 throttle 持全局锁） | `IsPerDomainConcurrent` + `DoesNotHoldGlobalLock`（`IsPerDomain` 作者已标注不构成守护） | **G1b-var1 红 ✅（首轮存活，已修）**、G1b-var2 红 ✅、G1 红 ✅ | **PASS** |
| functional[1]a | 20 并发同 key 只 1 次 fn 且全部拿到正确返回值 | `CoalescesConcurrentSameKey` | 首轮定向复核通过 | **PASS** |
| functional[1]b | 不同 key 不合并 | `CoalesceIsPerKey` | — | **PASS** |
| functional[1]c | **两个 key 均含 topic** | `CacheKeyIncludesTopic` / `CoalesceKeyIncludesTopic` | G2a/G2b 定向双红 ✅ | **PASS** |
| functional[2]a | TTL 内只调一次；过期后重调 | `TTLHitSkipsFn` / `TTLExpires` | G11 红 ✅（首轮） | **PASS** |
| functional[2]b | **缓存命中不等 MinInterval** | `CacheHitSkipsThrottle` | G3 定向红 ✅ | **PASS** |
| functional[3] | **Do 强制 TTL=0：串行重复调用每次执行 fn；不排除 singleflight 合并；Wait 只节流** | `DoForcesNoCache` / `WaitThrottlesWithoutFn` / `WaitDoesNotCache` | G13 红 ✅、G14 红 ✅ | **PASS**（措辞已按 Leader 裁定修正，见 §四） |
| boundary[0] | 未登记主题直通 | `UnregisteredTopicPassesThrough` | G8 判等价变异体（首轮 §六） | **PASS** |
| boundary[1] | nil `*Gate` 透明 | `NilGateIsTransparent` | G9 红 ✅（首轮） | **PASS** |
| boundary[2] | 超上限淘汰最旧 | `EvictsOldestEntry` | G10 红 ✅（首轮） | **PASS** |
| error_handling[0]a | 出错/超时不写缓存 | `DoesNotCacheErrors` / `Timeout` / `TimeoutDoesNotCache` | G12 红 ✅、G12b 红 ✅（首轮） | **PASS** |
| error_handling[0]b | **waiter 共享同一错误**（+ 失败后在途表已清理） | `CoalesceSharesErrorWithAllWaiters` | G4 定向红 ✅、**G15 红 ✅**（增补补做的在途表清理） | **PASS** |
| non_functional[0] | `-race` 绿；不泄漏 goroutine；Timeout=0 不限时 | `-race` 实测 / `RunWithTimeoutDoesNotLeakGoroutine` / `ZeroTimeoutMeansNoLimit` | G6 红 ✅、G5 定向红 ✅ | **PASS** |

**13 项全部 PASS。**

---

## 四、DoD functional[3] 措辞修正已代为落盘

Leader 裁定了口径（「每次都执行 fn」指**串行**重复调用时不被**缓存**吞掉，**不**排除
singleflight 对并发同 key 的合并），并请我代为落盘——首轮判定时任务处于 `rejected`（owner
`test-*`）可写，但 Leader 重派后 owner 变为 `leader`、dev 返工后变为 `dev-*`，两次窗口都关着。
本轮转入 `verifying` 后 owner 回到 `test-*`，已执行：

```
task TASK-002 update --expect-epoch 2 --json-field done_criteria=<修正后>
```

`jq` 直读核实：`functional[3]` 已是新表述；`functional`/`boundary`/`error_handling`/
`non_functional` 条数仍为 4/3/1/1；`functional[0]` 首句未变；`status`/`assignment_epoch`/
`rework_count`/`verifier` 均未受影响。**歧义已消除，后续读者（尤其 QA 阶段）不会再撞。**

本轮按修正后的表述验证，判 PASS：`TestDoForcesNoCache` 是**串行**调用 3 次断言 fn 被调 3 次，
G13 转红证明有有效守护；并发被合并不判 FAIL（设计如此，已在 `Do` 的文档注释中警告并给出出路）。

---

## 五、首轮增补的两项补做结论（在返工版上仍成立）

`gate.go` 本轮零改动，故增补报告的结论直接适用：

- **`Topics()` 那条缝对 Gate 无影响**：`grep 'Topics()' gate.go` 无匹配，Gate 从不调用它。
  `Lookup` 的 `ok` 两处用法（`Wait` 不节流、`fetch` 直通）问的都是「有无适用策略」，
  通配兜底返回 `true` 正是想要的答案，**未做错误假设**。
- **在途表清理有守护**：G15（用旁路 `errCache` 模拟「失败结果被复用」）→
  `CoalesceSharesErrorWithAllWaiters` 转红 `calls = 1, want 2`。

---

## 六、覆盖率、race、回归、约束、scope

| 项 | 结果 |
|---|---|
| 覆盖率 | **97.3%**（与首轮持平；本轮为断言强度提升，不产生新语句） |
| 测试 | **36 个全 PASS，0 SKIP，0 FAIL** |
| `-race` | 连跑 3 次全绿 |
| C3 不循环导入 | `go list -deps \| grep newthinker/atlas` → **仅 policy 自身 ✅** |
| gofmt | 无输出 ✅ |
| vet（`./internal/collector/...`） | exit 0 ✅ |
| 全量回归 | **17 包全部 ok ✅** |
| scope | 仅 `internal/collector/policy/gate_test.go`（11+/2-），落在 `packages`/`writes` 声明内 ✅ |
| 实现零改动 | `gate.go` md5 = `b350bcaa919e8b9a595e096c83a72216`，与首轮基线**逐字节相同** ✅ |

---

## 七、一条**未采纳的增强建议**（据实记录，不影响判定）

首轮增补 §三 我提过一个改进：用 `aDone` channel 把 `DoesNotHoldGlobalLock` 的
「静默假绿」转成可观测的「无效轮次」，并统计有效轮次、全部无效则 fail。**dev 本轮未采纳**
（它只改了 `b.x` 那一行）。

**这不构成 FAIL**：现有写法在 `rounds=3` 下漏检概率已很低（20ms 相对 500ms 等待窗口，
goroutine 启动 + 一次 map 查找 + 加锁通常是微秒级），且我本轮实测 8 次均有效捕获。
是否采纳由 Leader 决定，建议登记为 Sprint 末尾的可选增强。

若采纳，**务必注意断言顺序**：`elapsed` 阈值判定必须放在有效性判定**之前**。我第一版写反了，
结果两种 G1b 形态虽都转红但错误信息是错的——全局锁下 B 恰恰是等到 A 完成才返回的，
有效性条件必然不满足，每轮 `continue` 跳过 `elapsed` 断言，最后靠兜底断言才 fail，
读者会误以为是测试构造失败而非被测行为有问题。

---

## 八、复现命令

```bash
git worktree add --detach ../wt-v002r2 487e2ba8e1911729776aee7acab18bd9b0992e89
cd ../wt-v002r2

GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -race        # 36 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -coverprofile=/tmp/t2r.out
GOTOOLCHAIN=local go tool cover -func=/tmp/t2r.out                           # 97.3%
GOTOOLCHAIN=local go test ./internal/collector/... -count=1                  # 17 包全绿

# G1b 两种形态（在 Gate 加 globalMu，分别加锁于 throttle 的 MinInterval 检查之后 / 之前）
#   → 两者都应让 TestGateThrottleDoesNotHoldGlobalLock 转红，报「b 域被阻塞 ~480ms」
# 每次注入后先 `git diff --numstat -- gate.go` 确认改动量非空（为空=表达式静默失配，绿是假绿）

# 基线 md5（还原后须复原到）:
#   gate.go      = b350bcaa919e8b9a595e096c83a72216   （与首轮基线相同 → 实现零改动）
#   gate_test.go = eaed18ee9e8ef7fb2c5e0f5c0fd3f90b
```

worktree 已于验证结束后 `git worktree remove ../wt-v002r2` 清理；主工作区 `internal/` 零污染。
