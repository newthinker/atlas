# TASK-019 验证报告 —— crisis/backtest 的 Gate 接线缺口与装配守护

- 验证者: test-agent-16 / **assignment_epoch: 2**
- 被验对象: **`63c8245`**（backtest.go +4 · crisis.go +6 · policy.go +18 · gate_wiring_test.go +430 · wiring_test.go +11/-3）
- 判定线: **`coverage_floor = 74`**（单包口径）
- 验证环境: 独立 worktree `../wt-v019 @ 63c8245`（已在主仓库拆除）

## 结论：**PASS（verified）**

三条入口路径的接线与守护全部到位。**AST 复合句的两个分句各自独立有变异证据**，空真下界、
同文件他函数边界、运行期观测四类判据全部实证有效。

一处变异存活（X6），经取证判为**相对 DoD 的等价变异体**——两条路径都满足该条的三项断言（§四）。
另有一处**注释与实现的矛盾**，记入备录（§六）。

---

## 一、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 守护者 | 变异证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 两条 crisis 路径在 collector 构造前完成接线，**FRED_API_KEY 非空时也成立**；**直接观测接线结果**而非「没报错」 | `TestCrisisBackfillWiresGateWithFREDEnvSet` / `TestCrisisEvalWiresGateWithFREDEnvSet` / `TestBacktestWiresGate` | **X5** 把 `ensurePolicyGate` 变 no-op → **三条全红** `:354` `:368` `:382` | **PASS** |
| functional[1] 分句① | AST 断言接线调用**存在** | `TestEntrypointsWireGateBeforeCollectors` | **X1** 删掉 `crisis.go:120` → 红 **`:176`** | **PASS** |
| functional[1] 分句② | AST 断言接线在 collector 构造**之前** | 同上 | **X2b** 挪到 ctor 之后 → 红 **`:182`** | **PASS** |
| functional[2] | **空真自检**：扫不到目标须转红而非 vacuously 通过 | `found` 下界 + `ctorLine` 下界 | **X3b** 让 `runBacktest` 从 `backtest.go` 消失 → 红 **`:167`**「在没有函数体时空真地成立」 | **PASS** |
| boundary[0] | 判据限于**目标函数体内**，不得计入别处 | `TestScanGateOrderIgnoresOtherFunctions` + 真实文件反例 | **X4**（R4 式）删 `runCrisisBackfill` 那处、同时给同文件的 `openCrisisStore` 加一处 → **仍红 `:176`** | **PASS** |
| boundary[1] | 失败信息措辞须与实际保证对齐（词法位置，非执行顺序）| `wiring_test.go` 措辞已改 + 新增强度边界注释 | 见 §三 | **PASS** |
| boundary[2] | 不顺手重构；若提取 `initPolicyGate` 须逐个确认调用方 | **未提取**，改为新增 `ensurePolicyGate` 显式调用点 | 见 §五 | **PASS** |
| error_handling[0] | 配置不可读 + 环境变量有效 ⇒ crisis 仍能运行、Gate 退化而非 panic/退出 | `TestEnsurePolicyGateDegradesOnUnreadableConfig` | 三项断言齐备（不 panic `:409` / 不引入配置硬依赖 `:418` / 闸门非 nil 且可用 `:424` `:428`）；X6 见 §四 | **PASS** |
| non_functional[0] | 既有测试一字不改全绿；`-race`；0 SKIP；覆盖率 ≥74 | 实测 §二 | | **PASS** |

## 二、基线

| 项 | 结果 |
|---|---|
| 测试规模 | **196 个顶层测试全 PASS / 0 FAIL / 0 SKIP**（`-v` 全包 250 个 `=== RUN`）✅ |
| `-race` | ok（18.4s）✅ |
| 覆盖率（**单包口径**）| **75.2%** ≥ floor **74** ✅ |
| scope | 全部落在 `./cmd/atlas/`，与 `writes` 一致 ✅ |
| 既有测试 | `wiring_test.go` 仅改**失败信息措辞 + 新增注释**（boundary[1] 要求的那一项），断言逻辑与场景未变 ✅ |
| 还原 | 每变异 `md5` 校验；收尾 `git status --porcelain` 空 ✅ |

> **我派发前的覆盖率风险预警未兑现，这是好事。** 我算过 `cmd/atlas` 余量仅 **6 条语句**
> （1299 总 / 965 覆盖 = 74.29%），且新增接线正落在 `crisis.go:144-156` 的未覆盖区，
> 担心「AST 守护不产生覆盖率 ⇒ 正确实现被门禁 DENY」。实测 **75.2%（+0.9pp）**——
> 因为 Dev 把 `error_handling[0]` 写成了**真正的运行期测试**（三条 `...WiresGate` 实际驱动
> `runCrisisBackfill` / `runCrisisEval` / `runBacktest`），既满足该条的诉求，又顺带覆盖了新增行。
> **这正是 Leader 采纳的那条处置起了作用。**

## 三、boundary[1]：TASK-011 遗留的措辞项已修

我在 TASK-011 返工报告 §四 指出：失败信息写「闸门必须**早于** collector 构造」读起来像执行顺序
承诺，而 AST 保证的是词法位置。本任务一并修掉了：

```diff
-  "闸门必须早于 collector 构造（各 collector 在构造函数里取 policy.Default()，"
+  "闸门的**词法位置**须在 collector 构造之前（各 collector 在构造函数里取 "
+// ⚠ 强度边界：本测试断言的是**源码里的词法位置**，不是执行顺序的承诺。
+// 把 initPolicyGate 包进 defer/go 闭包、恒假 if、或写在提前 return 之后的
+// 死代码里，词法位置照样在前而运行期根本不执行——那几类不在本测试的射程内。
+// 运行期证据见 ... gate_wiring_test.go 里三条入口的运行期观测。
```

**它比我要求的更进一步**：不仅收窄了措辞，还**指明了运行期证据在哪**——把「静态守词法、
运行期守行为」这个分工写清楚了。新文件的同类断言（`:182`）从一开始就用「词法位置」措辞。

## 四、X6 存活的取证：相对 DoD 是**等价变异体**（我差点归因错误，记录在此）

**X6**：把 `ensurePolicyGate` 失败分支里的 `initPolicyGate(config.Defaults(), nil)` 去掉
（配置坏时什么都不做）→ **250 个测试全绿**。

### 4.1 先确认它不是「代码等价」

两条路径构出的 Gate **确实不同**：

| 路径 | Gate |
|---|---|
| 懒构造 `policy.Default()` | `New(NewTable(), nil)` —— 内置表，**quota store = nil** |
| `initPolicyGate(config.Defaults(), nil)` | 内置表 + `ApplyTTL/DisableTTL` + **`NewFileStore("data/collector-quota.json")`** + warn 钩子 |

⇒ 行为有实质差异（配额账本有无）。**不是代码层面的等价变异体。**

### 4.2 但相对 DoD 的断言面，它是等价的

`error_handling[0]` 要求断言的是：**crisis 仍能运行 / 不 panic / Gate 退化而非阻断**。
测试对应三项断言：

```
:409  不得 panic（defer recover）
:418  err 不含 "loading config"（不引入配置硬依赖）
:424  policy.Default() 不为 nil —— 应退化为内置策略表
:428  退化后的闸门可用（一次 Fetch 成功）
```

**X6 之下这四条全部仍然成立**——因为 `policy.Default()` 会懒构造出一个内置表 Gate，
同样非 nil、同样可用。⇒ **就 DoD 写下的要求而言，两条路径都满足，X6 是等价变异体。**

### 4.3 我的归因修正

我最初看到 X6 存活时，倾向判「error_handling[0] 的第三分句无守护」并准备 reject。
**直读断言行之后发现判错了**：那一条是有断言的（`:424`/`:428`），只是它区分不了两条
都合格的退化路径。

> 这是契约「第三类错误：归因错了——红/绿的原因不是被测行为」的一次实例，发生在我身上。
> 解药也正是契约写的：**构造更精确的对照形态**（这里是直读断言行、比对两条路径的实际产物），
> 而不是重读一遍结论。**若我停在「变异存活 ⇒ 无守护」，会报出一个不存在的缺陷并触发一轮返工。**

## 五、boundary[2]：未提取 `initPolicyGate`，判为合规

DoD 说「若选择把 `initPolicyGate` 从 `loadConfigOrDefaults` 里提出来（**推荐**）」——是推荐非强制。
Dev 选择**不提取**，改为新增 `ensurePolicyGate()` 作为显式调用点。

其实现依赖 `loadConfigOrDefaults` 内部的副作用（成功时它自己会调 `initPolicyGate`）：

```go
func ensurePolicyGate() {
	if _, err := loadConfigOrDefaults(); err != nil {
		initPolicyGate(config.Defaults(), nil)
	}
}
```

**判为合规**：DoD 未强制提取，且不提取就**不触及那 6 个 `loadConfigOrDefaults` 调用方的行为**
（`crisis.go:109/420`、`prism.go:171`、`watchlist.go:58`、`export_ohlcv.go:325`、
`export_signals.go:109`——我逐个查过，全在 `cmd/atlas` 内），避免了 DoD 警告的那类连带风险。

代价是**隐式耦合仍在**：接线依旧藏在 `loadConfigOrDefaults` 的函数体里，只是多了一个显式入口。
将来若再出现第四条「跳过配置加载」的路径，**同一族缺口会第四次发生**。建议记入末尾清单
（提取是独立重构，不该塞进本任务）。

## 六、备录：注释与实现矛盾（不作为 FAIL 依据）

`policy.go` 的注释写：

> 退化目标是内置策略表，即「限流/缓存按内置值、无 config 覆盖、**无跨进程配额账本**」。

但 `initPolicyGate` 无条件执行 `policy.NewFileStore(path)`，`path` 为空时取
`"data/collector-quota.json"`（`config.go:69` 的 `defaultQuotaPath`，:332 兜底）。
⇒ **退化路径实际上是有跨进程配额账本的**，与注释相反。

危害低（多一层配额保护而非少），但属教训 5 那类「注释描述的是当前巧合还是被守护的契约」。
**建议订正注释**（或改实现为传 nil store，若确实想要「无账本」）。这也解释了 X6 为什么
在行为上不等价却在断言面上等价——**那个差异恰好落在没有人声称要守护的地方**。

## 七、变异验证结果表（7 个：6 捕获 / 1 等价 / 0 无效计入判定）

每个变异**注入前先写下预期**；四道门全程强制。

| ID | 变异 | 预期 | 实际 | 断言行 |
|---|---|---|---|---|
| X1 | 删 `crisis.go:120` 的 `ensurePolicyGate()` | 红 | **红** ✅ | `:176`（存在性）+ 运行期 `:354` |
| X2b | 同上并挪到 ctor（`:151`）之后 | 红 | **红** ✅ | **`:182`（顺序）** + `:354` |
| X3b | `runBacktest` 从 `backtest.go` 消失（改名 + 同名薄封装另置） | 红 | **红** ✅ | `:167`（`found` 下界） |
| X4 | 【R4 边界】删目标函数那处 + 给同文件 `openCrisisStore` 加一处 | 红 | **红** ✅ | `:176` —— **未被蒙混** |
| X5 | `ensurePolicyGate` 变 no-op | 红 | **红** ✅ | `:354` `:368` `:382` **三条运行期全红** |
| **X6** | 退化分支去掉 `initPolicyGate` | ?（标为未知）| **绿** | **等价变异体（§四）** |

### 7.1 三次被门拦下的无效变异（自证四道门不是形式）

| 尝试 | 被哪道门拦下 | 若无该门会怎样 |
|---|---|---|
| X1 首版（多行 perl 表达式失配）| **门①** md5 未变 | 判成「绿·存活」⇒ 报出不存在的缺口 |
| X3 首版（`runBacktest` 改名）| **门②** `go vet` 不过（测试文件直接调用它）| 判成「捕获」⇒ 高估守护强度 |
| X3 次版（函数移出文件）| **门②** `"os" imported and not used` | 同上（这正是我自己 M15 的形态，第二次撞）|
| X2 首版（`sed 150a`）| **人工核对** | **最危险的一次**：门①②③全过、代码确实改了，但 `sed` 用**原始行号**、插入点落在 ctor **之前** ⇒ 语义没变。我因「预期红、实际绿」回头核对插入位置才发现 —— 这是第③类断裂点（改动量非空但语义未变），**四道门里没有一道能拦**，只能靠「先写预期再比对」 |

## 八、复现命令

```bash
git worktree add --detach ../wt-v019 63c8245 && cd ../wt-v019
GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -race        # 196 PASS 0 SKIP
GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -cover        # 75.2% ≥ 74

# AST 复合句两分句（注意 sed 用原始行号，插入点要在 ctor 之后）
sed -i '' '120d' cmd/atlas/crisis.go                          # X1 → :176
sed -i '' '120d;151a\'$'\n''\tensurePolicyGate()' cmd/atlas/crisis.go   # X2b → :182
# 空真下界：backtest.go 内 `func runBacktest(` 改名 + 另起文件加同名薄封装 → :167
# R4 边界：删 crisis.go:120 + 在 openCrisisStore(:90) 后加一处 → 仍红 :176
# 运行期：ensurePolicyGate 改空函数 → :354 :368 :382

cd <主仓库> && git worktree remove ../wt-v019
```
