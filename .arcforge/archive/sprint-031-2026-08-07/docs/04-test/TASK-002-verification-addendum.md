# TASK-002 验证报告 · 增补（派验单三份材料的补做）

- 验证者: test-agent-17
- 补做对象: `73b724bb97cb652ceffdbe27183591f0683d7f82`（首轮被验 sha；`gate.go` 在返工提交
  `487e2ba` 中**未改动**，故本文件结论对返工版同样适用）
- 本文件是 `TASK-002-verification.md` 的增补，**不替代**它；首轮 rejected 判定不变。

## 零、为什么有这份增补（时序如实说明）

我的首轮判定 `rejected` 落盘于 `04:00:52Z`，**早于** Leader 派验单送达。派验单里承诺的三份
材料（test-agent-16 的三个盲区、`Topics()` 缝对 Gate 的影响、dev 自报的 7 个变异）我在判定
时只覆盖了第三份和前两份的一部分。**两项明确要求未覆盖**，本文件是补做结论：

1. **test-agent-16 盲区② 的后半**：「coalesce 失败后有没有正确清理在途表」——首轮我只定向
   验了错误传播（G4），没为「在途表清理」造变异。
2. **材料②**：Gate 是否依赖 `Topics()` 的完整性、是否用 `Lookup` 的 `ok` 做了错误假设。

另外 Leader 明确要求我**评估 20ms sleep 概率性局限的可接受性，并在有更确定构造时提出**——
这条我给出了一个已实测的改进方案（§三）。

**补做未发现新缺陷**，首轮判定与 reject_reason 无需修改。

---

## 一、材料②：`Topics()` 那条缝对 Gate **无影响**

```
grep -n 'Topics()' internal/collector/policy/gate.go   → 无匹配
```

**Gate 从不调用 `Topics()`**，故 TASK-001 增补里的 M26c/M26d 逃逸路径对 Gate 不成立。

`Lookup` 的 `ok` 在 `gate.go` 里只有两处用法，均正确：

| 位置 | 用法 | 判定 |
|---|---|---|
| `Wait`（gate.go:131-134） | `if !ok { return }` —— 未登记不节流 | ✅ |
| `fetch`（gate.go:143-146） | `if !ok { return fn() }` —— 未登记直通 | ✅ |

Leader 提示的风险是「若 Gate 用 `_, ok := Lookup(topic)` 判断**是否已登记**，
`lixinger.任意后缀` 都会因通配段返回 true」。核对下来 **Gate 没有做这个错误假设**：
它两处要问的都是「这个 topic **有没有适用的策略**」，而通配兜底的语义恰恰就是
「`lixinger.*` 下的所有端点都适用该策略」——`ok=true` 正是想要的答案，不是误判。

> 换言之：`Lookup` 的 `ok` 宽于「显式登记」，但 Gate 消费的正是那个宽语义。
> 这条缝要造成危害，需要有人拿 `ok` 去回答「这个主题名是否在表里」——Gate 没有。

---

## 二、盲区② 后半：在途表清理**已有守护**（补造变异 G15 验证）

`TestCoalesceSharesErrorWithAllWaiters` 的最后 8 行即是这条断言：

```go
if _, err := Fetch(g, "a.x", "same", fn); !errors.Is(err, wantErr) { ... }
if calls != 2 {
    t.Errorf("失败后再次调用必须重新执行 fn: calls = %d, want 2（在途表漏清会复用已失败的条目）", calls)
}
```

首轮我只定向验了 G4（错误传播），没验这段。补造 **G15** 模拟「在途表漏清」——
`singleflight.Group` 自身保证清理，无法直接让它漏清，故用旁路 `errCache` 复现该**形态**
（失败结果被缓存并复用）：

| 变异 | 改动量 | 结果 | 捕获者与实测输出 |
|---|---|---|---|
| **G15** 失败结果被缓存复用（模拟在途表漏清） | 19+/5- | **红 ✅** | `TestCoalesceSharesErrorWithAllWaiters` — `失败后再次调用必须重新执行 fn: calls = 1, want 2（在途表漏清会复用已失败的条目）` |

**盲区② 的两半（错误传播 + 在途表清理）均有有效守护。**

至于盲区①（并发验证是否靠真实并发取证）与盲区③（key 含 topic 是否只断言字符串），
首轮已覆盖：前者见 `IsPerDomainConcurrent` 用 barrier 而非 sleep 猜时序（首轮报告 §一）；
后者见 `TestCacheKeyIncludesTopic` / `TestCoalesceKeyIncludesTopic` 断言的是
**fn 调用次数与跨主题不串味**（`xCalls=1, yCalls=0, want 1/1`），不是 key 的字符串形状——
**不是空洞写法**，G2 定向双红已证。

---

## 三、20ms sleep 概率性局限的评估（Leader 点名要求）

### 3.1 结论：局限**可接受**，但可以改进到「假绿可观测」

dev 的判断是对的：包外确实没有观测 `throttle` 持锁状态的信号，`fn` 在执行链 ⑤ 才被调用、
那时锁已释放，所以拿 `fn` 当信号来得太晚。**在不给 `gate.go` 加测试钩子的前提下，没有完全
确定的构造。** 它的处理（如实声明 + `rounds=3` + 猜错是假绿而非假红）是该约束下的合理选择。

但「假绿」目前是**静默**的：B 抢先拿锁时这一轮什么也没检验，测试照样报 PASS，读者无从得知。
可以把它变成**可观测**的。

### 3.2 已实测的改进：把「静默假绿」转成「无效轮次」

用 `aDone` channel 判定「B 返回时 A 是否仍在等待」——仍在等待才算这一轮真正构成检验；
统计有效轮次，全部无效则 fail：

```go
const rounds = 3
effective := 0
for round := 0; round < rounds; round++ {
    ...
    aDone := make(chan struct{})
    go func() { defer close(aDone); _, _ = Fetch(g, "a.x", "k", noop) }()
    time.Sleep(20 * time.Millisecond)

    start := time.Now()
    if _, err := Fetch(g, "b.x", "k", noop); err != nil { t.Fatal(err) }
    elapsed := time.Since(start)

    // 先判阻塞：B 耗时过长本身就是问题，无论 A 此刻是否已完成。
    if elapsed > 150*time.Millisecond {
        t.Fatalf("第 %d 轮: a 域节流期间 b 域被阻塞 %v —— throttle 持有了跨域的全局锁", round, elapsed)
    }

    // 再判有效性：B 返回时 A 仍在等待，这一轮才真正构成检验。
    valid := false
    select {
    case <-aDone:
    default:
        valid = true
    }
    <-aDone
    if !valid {
        t.Logf("第 %d 轮无效：B 返回时 A 已完成，本轮未构成检验", round)
        continue
    }
    effective++
}
if effective == 0 {
    t.Fatalf("全部 %d 轮均无效（B 每次都晚于 A 完成），本测试未构成任何检验", rounds)
}
```

### 3.3 ⚠ 两条断言的**顺序**是关键（我第一版写反了，实测才发现）

第一版我把有效性判定放在 `elapsed` 断言**之前**，结果两种 G1b 形态虽然都转红，
**但错误信息是错的**：报的是「第 0 轮无效：B 返回时 A 已完成，本轮未构成检验」，
而不是「b 域被阻塞 480ms」。

原因：**全局锁实现下，B 恰恰是等到 A 完成才返回的**，所以「B 返回时 A 仍在等待」这个
有效性条件必然不满足 → 每轮都被判无效 → `continue` 跳过了 `elapsed` 断言 → 最后靠
`effective == 0` 的兜底断言才 fail。捕获是捕获了，但读者会以为是测试构造失败，而非被测行为有问题。

**先判 `elapsed`、再判有效性**，修正后错误信息准确：

| 场景 | 结果 | 实测输出 |
|---|---|---|
| G1b-var1（锁在 early-return 之后） | **红 ✅** | `第 0 轮: a 域节流期间 b 域被阻塞 480.04ms —— throttle 持有了跨域的全局锁` |
| G1b-var2（锁在 early-return 之前） | **红 ✅** | `第 0 轮: a 域节流期间 b 域被阻塞 480.72ms —— throttle 持有了跨域的全局锁` |
| 正确实现，连跑 **6 次** | **全绿 ✅** | 无假红 |

> 这条顺序陷阱本身值得记：**当有效性判定的条件与被测缺陷的表现耦合时，判定顺序会决定
> 错误信息指向哪儿。** 通用规则是「先判被测行为，再判本轮是否有效」——反过来会让缺陷
> 伪装成「测试没测到」。`go vet` 还在这个过程中抓到我把 `t.Fatal` 写成带 `%d` 的格式串，
> 顺带证明本仓库的 vet 是有效的。

### 3.4 建议

这是**增强项不是缺陷**：现有写法在 `rounds=3` 下漏检概率已经很低（20ms 相对 500ms 等待窗口，
goroutine 启动 + 一次 map 查找 + 加锁通常是微秒级）。是否采纳由 Leader 决定。
若采纳，价值在于**把不可消除的概率性变成可观测的**，而不是消除它——后者做不到。

---

## 四、返工提交 `487e2ba` 的初步核对（非正式验证）

dev 已采纳首轮的修复方案并转 `dev_done`（epoch=2、rework_count=1）：

```go
- "b.x": {}, // 不节流：正确实现下必须立即返回
+ "b.x": {MinInterval: time.Millisecond},
```

并把根因写进了注释（「throttle 开头有 `if p.MinInterval <= 0 { return }` 的快速路径，
配成 0 则 b 域根本不进入持锁临界区，于是只能捕获『全局锁加在该 early-return 之前』
这一种形态」）。`gate.go` **一行未动**。

**这只是收到派验单前的初步核对，不构成验证结论**——正式复核待 Leader 派验后进行，
届时须独立复现 G1b-var1/var2 双双转红。

---

## 五、无法执行 DoD 措辞修正（权限窗口已过）

Leader 请求我代为 `update` TASK-002 的 `done_criteria`（修正 functional[3] 的措辞歧义），
依据是「任务处于 `verifying`，合法写者只有 `test-*`」。**该前提在 Leader 自己重派后已失效**：

| 时刻 | status | owner | 我能否 update |
|---|---|---|---|
| 派验单撰写时 | `verifying` | `test-*` | 能 |
| 我判定后 | `rejected` | `test-*` | 能 |
| Leader 重派后（04:02:18Z） | `assigned` | **`leader`** | **不能** |
| dev 返工后（现在） | `dev_done` | **`dev-*`** | **不能** |

且 Leader 给的命令带 `--expect-epoch 1`，而重派已使 epoch=2，即便状态允许也会被 DENY。

**可行窗口**：Leader 下次把任务转 `verifying` 之后（owner 回到 `test-*`），我可以立即执行；
或者由当前 owner 自己改（`dev_done` 阶段是 dev，`accepted` 阶段是 leader）。
口径本身 Leader 已裁定，我在首轮报告 §五 采用的解读与该裁定一致，**首轮判定无需改动**。

---

## 六、复现命令

```bash
git worktree add --detach ../wt-v002b 73b724bb97cb652ceffdbe27183591f0683d7f82
cd ../wt-v002b/internal/collector/policy

grep -n 'Topics()' gate.go                    # 无匹配 → Gate 不依赖 Topics()
go test . -count=1 -run '^TestCoalesceSharesErrorWithAllWaiters$'

# G15（在途表漏清）：给 Gate 加 errCache map，fetch 开头查、call 的错误路径写，
#   → 期望 TestCoalesceSharesErrorWithAllWaiters 转红 `calls = 1, want 2`

# 基线 md5（还原后须复原到）:
#   gate.go      = b350bcaa919e8b9a595e096c83a72216
#   gate_test.go = f400ec5ad2940f70bff9bc845aab08df
```

worktree 已拆除；主工作区 `internal/` 全程零污染。
