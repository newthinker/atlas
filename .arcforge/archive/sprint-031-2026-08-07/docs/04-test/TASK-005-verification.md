# TASK-005 验证报告 —— `Default`/`SetDefault` + `Table.Override`

- 验证者: test-agent-17
- 被验对象: `3e75dc8d7a023ad7578ffa7520f5b1d9ea89f622`（+324：`default.go` 40 / `default_test.go` 101 / `policy.go` 52 / `policy_test.go` 131）
- 验证环境: 独立 worktree `../wt-v005`（detached @ 3e75dc8）
- assignment_epoch: 1 / rework_count: null（首轮，不设搜索限制）

## 结论：**PASS（verified）**

8 条 DoD 全部有有效守护，**76 测试 0 SKIP**、`-race` 绿、覆盖率 94.4%、17 包回归绿、
scope 干净、gate/quota 系列未动。Leader 点名的五处重点全部独立复现。

**本轮我采用了 Dev 提出的「注入前写下预期」纪律，它立刻在我自己身上见效**：
14 次变异里有 **3 次与预期不符**，逐一诊断后**全部是我的变异有问题，没有一处是被测代码的缺口**（§三）。
若不先写预期，这 3 个「绿」我会记成「测试无守护」，从而报出三个不存在的缺陷。

---

## 一、Leader 点名的五处重点

### 1.1 `Override` 指针语义的两面 —— 两条测试确实互补

| 测试 | 验的 | O1（改成零值判定）下 |
|---|---|---|
| 方案原有 `TestOverrideAppliesOnlySetFields` | 未设置的字段保持不变 | **绿 —— 测不出** |
| Dev 补的 `TestOverrideExplicitZeroValues` | **显式零值必须生效** | **红(断言)** `policy_test.go:330: 显式 TTL=0 应关掉该主题的缓存, got 5m0s` |

与 TASK-004 那对复合句分句**同构**：两条断言方向相反（「没写的别动」vs「写了零值要生效」），
只验前者时「显式关缓存/关合并被静默忽略」完全不可见。
**全指针字段确实是必要性而非风格选择**，Dev 的判断成立。

### 1.2 `QuotaLimit` 覆盖须保留 `Window`/`Loc`

O4（只覆盖 Limit 时不保留原 Quota）→ **红** `policy_test.go:351: 只改 limit 时 Window/Loc 应保持:
&{Limit:20 Window:24h0m0s Loc:Asia/Shanghai...}`。与 TASK-001 的 M21 同类后果（配额在错误时刻重置），
已有守护。

### 1.3 并发单例的两个判据 —— **关系比「缺一不可」更精确**

我做了 2×2（变异 × 是否带 `-race`），并对 O6 跑了 10 次看稳定性：

| 变异 | 不带 `-race` | 带 `-race` |
|---|---|---|
| **O5b** 每次新建实例（去快速路径 + 去 nil 检查） | **红(断言)** `default_test.go:30: Default() 必须返回同一实例` | **红(断言)** `:98`，**无 DATA RACE** |
| **O6** 去掉锁 | **红(断言)** `:98`，**10 次跑 10 次红**（稳定，非概率性） | **DATA RACE**（3/3） |

**精确结论**：
- **单例断言对两个变异都稳定捕获**，是充分的；
- **`-race` 的独立价值在于「归因」而非「检出」**：同样是「拿到不同实例」，`-race` 能指出
  根因是**数据竞争**（O6）还是**逻辑错误**（O5b，有锁保护、无竞争但违反单例）。

这与转述的「两条缺一不可（检出层面）」略有出入，但**两条都该保留** —— 归因价值是真实的，
正是 TASK-002 那条「能转红不够，红得指向哪里也重要」的应用。**不构成缺陷**，仅精确化。

### 1.4 `SetDefault(nil)` 的判据不止「没崩」

O7（`SetDefault(nil)` 后 `Default()` 返回 nil）→ **红** `default_test.go:24: Default() 不得返回
nil —— 未接线的调用点也要能拿到内置策略`。测试用了 `defer recover`（「不该 panic」型的正确写法，
与 TASK-003 R1 确立的标准一致），且判据是「必须重新懒构造出**可用**的 Gate」而非「没崩」。**成立。**

### 1.5 `Override` 未登记主题会登记进表

O8（不登记未登记主题）→ **红** `policy_test.go:376: config 覆盖应能登记新主题`。
与约束 C6 的关系（**默认仍是零策略，只有显式 Override 才登记**）在注释里写明了。**成立。**

### 1.6 文档要求：`Override` 的「只应在构造阶段调用」**在**

`policy.go:169-171`：

> ⚠ 只应在**构造阶段**调用（config 装载时）。Table 是裸 map 无锁，设计意图是构造后只读；
> 拿它做运行期热更新会与并发读的 Lookup 竞争。**会违反这一点的人是写 config 热加载的人，故写在这里。**

符合「约束贴在会被违反的那一侧」。✅

---

## 二、变异记录表（**预期列在注入前填**）

| ID | 变异 | 预期 | 实际 | 断言位置 |
|---|---|---|---|---|
| O1a | 零值判定 → 方案原有测试 | 绿 | 绿 ✓ | — |
| O1b | 零值判定 → Dev 补的显式零值测试 | 红 | 红 ✓ | `policy_test.go:330` |
| O1c | 零值判定 → 全量 | 红 | 红 ✓ | `:330` |
| **O2** | 「Quota 共享而非复制」（**首版**） | 红 | **绿 ✗** | 诊断见 §三 |
| **O2b** | 真共享（就地改原指针，重做） | 红 | 红 ✓ | `policy_test.go:400: a.y 的 Limit = 99, want 5（Override 须复制 Quota，不得就地改共享实例）` |
| O3 | `Override` 不清空 `Domain` | 红 | 红 ✓ | `:419: Domain = "custom-domain", want "shared"（从通配条目继承 Domain 会让限流域串味）` |
| O4 | `QuotaLimit` 覆盖不保留 `Window`/`Loc` | 红 | 红 ✓ | `:351` |
| **O5** | 「每次新建实例」（**首版**） | 红 | **绿 ✗** | 诊断见 §三 |
| **O5b** | 每次新建（去快速路径 + 去 nil 检查，重做） | 红 | 红 ✓ | `default_test.go:30` |
| O5b-race | 同上带 `-race` | 红 | 红 ✓ | `:98`（无 DATA RACE） |
| O6 | 去掉锁（不带 `-race`） | 绿 | **红 ✗** | `:98` —— 见 §三，**这次是我的预期错了** |
| O6-race | 去掉锁（带 `-race`） | 红 | 红 ✓ | DATA RACE |
| O7 | `SetDefault(nil)` 后返回 nil | 红 | 红 ✓ | `default_test.go:24` |
| O8 | `Override` 不登记未登记主题 | 红 | 红 ✓ | `policy_test.go:376` |

五道门全程执行：① 改动量非空且改语义；② `go test -c` 编译通过；③ 核到**断言行**
（正则区分 panic 堆栈/DATA RACE）；④ `=== RUN` 数 > 0；⑤ 还原后 10 个文件 md5 与基线一致。

---

## 三、三处「与预期不符」的诊断 —— **全部是我的变异问题**

这三处正是 Dev 提出「先写预期」要解决的第③类断裂点。逐一诊断：

### O2：我又只删了注释

我的首版变异是：

```go
- q = *p.Quota // 复制而非共享：内置表的 *Quota 可能被多主题引用
+ q = *p.Quota
```

**只删了注释，语义完全没变**。门①（改动量非空）放行——这与我在 TASK-003 造 Q17 时犯的
是**同一个错误**，而且我当时就把它写进了报告。**重做为 O2b（`q := &Quota{...}` + `q = p.Quota`
就地改原指针）后立即转红** `:400`。

### O5：变异没打中 —— `Default()` 有快速路径

我把 `if defaultGate == nil { ... }` 改成无条件新建，但 `Default()` 开头有：

```go
defaultMu.RLock(); g := defaultGate; defaultMu.RUnlock()
if g != nil { return g }        // ← 第二次调用在这里就返回了，根本走不到我改的地方
```

**变异位于不可达路径上**。重做为 O5b（同时去掉快速路径与 nil 检查）后转红 `:30`。

> 这与 Dev 那次「`defer` 在 `p.Quota` 重新赋值之后执行」是同一类：**改对了地方，但那个地方在
> 该场景下不生效**。共同点是——**只看 diff 无法判断变异是否打中，必须看它在目标路径上是否可达。**

### O6：这次是**我的预期错了**（不是变异错）

我预期「去掉锁后不带 `-race` 应该绿，只有 `-race` 能抓」，实际**10 次跑 10 次红**。
去掉锁后并发下确实稳定产生多实例，单例断言直接抓到。

**这一处恰恰体现了预期列的另一重价值**：它不只暴露「变异没打中」，也暴露「**我对被测系统的
理解有偏差**」。若不写预期，我会把这个红当成理所当然，从而错过 §1.3 那个更精确的结论
（`-race` 的价值在归因而非检出）。

---

## 四、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 对应测试 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | `Default()` 懒初始化返回内置表 Gate；重复调用同一实例；`SetDefault` 生效 | `DefaultIsLazyAndUsesBuiltinTable` / `SetDefaultReplaces` | O5b 红 `:30`、O7 红 `:24` | **PASS** |
| functional[1] | `Override` 只应用非 nil 字段，未设置的保持内置值 | `OverrideAppliesOnlySetFields` + **`OverrideExplicitZeroValues`** | O1b 红 `:330`（前者在 O1 下绿，两条互补） | **PASS** |
| functional[2] | 只覆盖 `QuotaLimit` 时保留 `Window`/`Loc` | `OverrideQuotaLimitKeepsWindowAndLoc` | O4 红 `:351` | **PASS** |
| boundary[0] | 对无 `Quota` 的主题可新增配额 | `OverrideCanAddQuotaToTopicWithout` | O4 覆盖同一代码块 | **PASS** |
| boundary[1] | `Override` 未登记主题会登记进表（默认仍零策略） | `OverrideRegistersUnknownTopic` | O8 红 `:376` | **PASS** |
| error_handling[0] 前半 | `SetDefault(nil)` 不 panic 且能重新懒构造 | `SetDefaultNilDoesNotPanic`（`defer recover`） | O7 红 `:24` | **PASS** |
| error_handling[0] 后半 | 并发 `Default()` 不 panic、初始化并发安全 | `DefaultIsConcurrencySafe` | O6 红 `:98` + DATA RACE；O5b 红 `:98` | **PASS** |
| non_functional[0] | `-race` 全包全绿，TASK-001~004 用例保持通过 | 全量 76 测试 | 实测绿 | **PASS** |

**另有一条 DoD 未明写但被守护的性质**：`Override` 清空 `Domain` 让 `Set` 重新推导
（O3 红 `:419`）—— 命中通配条目时原样带回 `Domain` 会让新主题继承错误限流域。
这是 Dev 主动补的，**属实且有价值**。

---

## 五、覆盖率、回归、约束、scope

| 项 | 结果 |
|---|---|
| 测试 | **76 个全 PASS，0 SKIP，0 FAIL** |
| `-race` | 绿 |
| 覆盖率 | **94.4%**；`Default` **100%**、`SetDefault` **100%**、`Override` **95.0%** |
| C3 不循环导入 | 仅 policy 自身 ✅ |
| gofmt / vet | 无输出 / exit 0 ✅ |
| 全量回归 | **17 包全部 ok ✅** |
| scope | 仅 `default.go` / `default_test.go` / `policy.go` / `policy_test.go`，落在声明内 ✅ |
| 上游产物 | `git diff 6183aba -- gate.go quota.go quota_file.go` **无输出** → TASK-002/003/004 实现未动 ✅ |

`Override` 未覆盖的 5% 是 `o.Timeout != nil` 分支（DoD 未点名 Timeout 覆盖场景），不计入判定。

---

## 六、方法论：「注入前写下预期」的实效（建议进契约）

本轮 14 次变异，**3 次与预期不符**，诊断后全部是我这边的问题：

| 不符类型 | 实例 | 若无预期列会怎样 |
|---|---|---|
| 变异只改了注释（第③类） | O2 | 记成「Quota 共享无守护」→ **报出不存在的缺陷** |
| 变异位于不可达路径 | O5 | 记成「单例性无守护」→ **报出不存在的缺陷** |
| **我对系统的理解有偏差** | O6 | 把红当理所当然 → **错过 §1.3 的精确结论** |

前两类是 Dev 说的「变异绿有两种解释」，第三类是我发现的**反向价值**：预期列也能暴露
**「红得比我以为的更容易」**，从而修正我对系统的理解。

⇒ 建议契约里把这条写成双向的：**预期与实际不符时，两个方向都要诊断** ——
「预期红实际绿」查变异是否打中，「**预期绿实际红**」查自己对系统的理解是否有偏差。

关于 Dev 那句「判断语义是否按我意图改变需要我的意图，而那只存在于我脑子里」——
本轮也印证了它的**边界**：预期列能把意图外化成可比对的东西，但**写下的预期本身也可能是错的**
（O6）。它不是自动化，是**把一次判断变成两次判断**，两次都错的概率显著低于一次。

---

## 七、复现命令

```bash
git worktree add --detach ../wt-v005 3e75dc8d7a023ad7578ffa7520f5b1d9ea89f622
cd ../wt-v005

GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -race        # 76 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/collector/policy/ -count=1 -v | grep -c '^--- SKIP'   # 0

# 重点 1（指针语义两面）：把 Override 的 `if o.X != nil` 改成 `if o.X != nil && *o.X != 0`
#   → TestOverrideAppliesOnlySetFields 仍绿；TestOverrideExplicitZeroValues 红 :330
# 重点 3（并发单例判据）：
#   O5b 去掉 Default 的快速路径与 nil 检查 → 单例断言红 :30，-race **不报**
#   O6  去掉 Default 的全部锁            → 单例断言红 :98（10/10 稳定），-race 报 DATA RACE
# O2b（Quota 必须复制）：把 `q := Quota{...}; q = *p.Quota` 改成 `q := &Quota{...}; q = p.Quota`
#   → 红 :400（注意：只删那行注释是**无效变异**，语义不变）

# 五道门：① 改动量非空且改语义 ② go test -c 编译通过 ③ 核到断言行（区分 panic/DATA RACE）
#        ④ === RUN 数 > 0 ⑤ 还原后 10 文件 md5 一致
# **外加：每个变异注入前先写下预期，跑完比对；不符必须诊断到根因再下结论。**
```

worktree 已于验证结束后清理；主工作区 `internal/` 零污染。
