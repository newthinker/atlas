# TASK-011 返工验证报告（review_fix / fix_items[S1]）—— serve 装配的 AST 守护

- 验证者: test-agent-16 / assignment_epoch: 1
- 被验对象: `519f99bc0bf5914eaf78800c602cca0eb0a46bc7` —— **仅新增 `cmd/atlas/wiring_test.go`（185 行），生产代码零改动**
- 验证环境: 独立 worktree `../wt-v011r2 @ 519f99b`（已在主仓库拆除）
- 判定依据: **`fix_items[S1]`**（非 done_criteria）

## 结论：**PASS（verified）**

`fix_items[S1]` 的**两个分句各自独立有证据**。我跑了 6 个变异，**R1–R4 全部按预期捕获**
（含 Leader 点名、QA 未做的 R4 边界反例）；**R5/R6 按预期存活**，它们刻画的是静态 AST
方法的**固有边界**而非缺陷（§四）。

---

## 一、`fix_items[S1]` 逐项对照

| S1 的要求 | 证据 | 判定 |
|---|---|---|
| 在 `cmd/atlas` 加 AST 测试，解析 serve.go、定位 `runServe` 函数体、断言其中**存在** `initPolicyGate` 调用 | `TestServeWiresPolicyGateBeforeCollectors` + `scanWiring`；**R1** 删该行 → 红 `:97` | **PASS** |
| 该调用出现在**任何 collector 构造之前** | 同上的顺序断言；**R2** 挪到 `buildCollectors` 之后 → 红 `:105`，且**精确报出两个行号**（init@108 > build@107）| **PASS** |
| **不需要动生产代码** | `git show --name-only` **只有 `wiring_test.go`** 一个文件 | **PASS** |
| 行号**现读不写死** | `fset.Position(call.Lparen).Line` 现算；无任何字面行号 | **PASS** |
| **阳性对照与阴性测试两格缺一不可** | 阴性：扫真实 serve.go；阳性：`DetectsMissingInitCall` / `DetectsWrongOrder` / `IgnoresOtherFunctions`；另有 2 条错误路径测试 | **PASS** |
| **自证**：注入变异确认新测试转红 + 2×2 闭环（旧测试集同变异下全绿）| Dev 自证 + 我独立复现（§三）；R1 下**全量 185 个测试里只有这一条 FAIL**，即旧测试集在同一变异下确实全绿 —— **闭环左下格成立** | **PASS** |

> **复合句已按两分句取证**：R1 只证明「存在性」，R2 只证明「顺序」。一个只检查存在性的
> 实现会在 R1 下红、在 R2 下绿 —— 两条变异把两个分句分开钉住了。

## 二、基线复跑

| 项 | 结果 |
|---|---|
| `cmd/atlas` 全量 | **185 PASS / 0 FAIL / 0 SKIP** ✅（陷阱 11：0 SKIP，无守卫被静默吞掉）|
| 新增 6 个测试单独 `-run` | 全 PASS ✅ |
| `-race` | ok（18.1s）✅ |
| 三包合并覆盖率 | **75.8%** ≥ floor **74** ✅（与门禁自述一致）|
| scope | 改动只有 `cmd/atlas/wiring_test.go`，与 `writes=["./cmd/atlas"]` 一致，**无越界** ✅ |

## 三、变异验证（6 个：4 捕获 / 2 按预期存活 / 0 无效）

**每个变异注入前先写下预期，实际与预期无一处不符。** 四道门全程强制。

| ID | 变异 | 预期 | 实际 | 断言行 / 说明 |
|---|---|---|---|---|
| R1 | 删掉 `serve.go:85` 的 `initPolicyGate`（换 `_ = log`）| 红 | **红** ✅ | `:97`「runServe 函数体内没有 initPolicyGate 调用」|
| R2 | 把它挪到 `buildCollectors` **之后** | 红 | **红** ✅ | `:105`「initPolicyGate 在第 **108** 行、buildCollectors 在第 **107** 行」|
| R3 | `runServe` 改名（扫描目标消失）| 红 | **红** ✅ | `:94`「找不到 runServe —— 扫描没扫到东西，下面的断言无意义」|
| **R4** | **【Leader 点名·QA 未做】删 runServe 里那处，同时在 serve.go 的另一个函数（`buildSignalStore`）里加一处** | 红 | **红** ✅ | `:97` —— **别处的调用没能蒙混过关** |
| R5 | 【我加】`initPolicyGate` 包进 `defer func(){...}()`，词法位置仍早于 buildCollectors | **绿** | **绿** ✅ | 见 §四 |
| R6 | 【我加】同上但用 `go func(){...}()` | **绿** | **绿** ✅ | 见 §四 |

> R3 首版无效（**门②**：我只改了函数定义没改 `RunE: runServe` 引用 → `go vet` 不过，
> 被 runner 判为「变异无效」而非「捕获」）。改成全引用一并改后才拿到干净结果。
> **记此以自证四道门不是形式** —— 若只看退出码，首版会被误记成「捕获」。

### 3.1 R4：Leader 点名的边界反例，**边界真的在守**

`TestScanWiringIgnoresOtherFunctions` 用内联夹具证明「别的函数里的调用不计入」。
我把它搬到**真实文件**上做了强化版：**删掉 `runServe` 里那处、同时给 `buildSignalStore`
加一处**（两者同在 serve.go，会被同一次 `ParseFile` 解析到）。

结果 **`:97` 转红** —— 扫描器确实把判据限定在 `runServe` 的函数体内，不会被同文件其它
函数的调用蒙混。

> 补充一点事实订正：Leader 提到「`loadConfigOrDefaults` 里也有一处 `initPolicyGate`」。
> 实测那处在 **`cmd/atlas/export_ohlcv.go:297`**，与 serve.go **不同文件**，
> 而 `scanWiring("serve.go", ...)` 只解析 serve.go，那处根本不在扫描范围内。
> 所以真正需要守的边界是「**同文件内的其它函数**」—— R4 测的正是这个，比原设想更贴切。

## 四、方法边界：它量的是**词法位置**，不是**执行顺序**（R5/R6）

这是我主动补的两格，用来回答 Leader 的第 1 问「还有没有别的让扫描静默落空的路径」。

把 `initPolicyGate(cfg, log)` 换成 `defer func(){ initPolicyGate(cfg, log) }()`（R5）
或 `go func(){ ... }()`（R6）后：调用的**词法行号仍早于** `buildCollectors`，
`ast.Inspect` 会走进函数字面量把它找到，于是 `initGateLine < buildLine` → **测试通过**。

但**实际执行**：`defer` 要到 `runServe` 返回时才跑（远晚于 collector 构造）；
`go` 则是并发、顺序不确定。**两者都是真实的接线错误，而测试是绿的。**

**这不构成缺陷，理由三条**：

1. **静态 AST 分析在原理上无法验证执行顺序** —— 要验执行顺序只能做运行期插桩，
   而那正需要重构 `runServe` 单体函数，即 QA 已论证「不必要」的那条路。
2. **失效路径不是「会不小心踩到」的形态**：没人会无意间把一行同步调用包进
   `defer`/`go` 闭包。真实的回归形态是**删掉**或**挪位置** —— 那两种 R1/R2/R4 全部捕获。
3. `fix_items[S1]` 的措辞是「该调用**出现在**任何 collector 构造之前」，
   「出现」本就是词法语义。

**建议（非返工项）**：在 `scanWiring` 的注释里补一句边界声明，例如
「本扫描判定的是**词法位置**；`defer`/`go` 闭包内的调用词法在前但执行在后，本方法不区分」。
理由是那条失败信息现在写的是「闸门必须**早于** collector 构造」——读起来像执行顺序的承诺，
而实现给的是词法位置的保证。**这与教训 5『注释描述的是当前巧合还是被守护的契约』同源：
把保证的边界写清楚，比让后人以为它保证得更多要好。**

## 五、Leader 三处重点核查的答复

### 5.1 空真防护是否兜住了全部形态（第 1 问）

我把「让扫描静默落空」的路径逐条走了一遍：

| 路径 | 结果 | 依据 |
|---|---|---|
| `runServe` **改名** | ✅ 兜住 → `found=false` → `Fatal` | **R3 实测红** |
| `runServe` **移到别的文件** | ✅ 兜住 → serve.go 里找不到 → `found=false` | 同一机制（`found` 下界）|
| serve.go **被改名/移走** | ✅ 兜住 → `ParseFile` 返回 err → `Fatal` | `TestScanWiringReportsMissingFile` 证明 err 不被吞 |
| `ParseFile` **出错被吞** | ✅ 不会 → err 原样返回且带文件名 | `TestScanWiringReportsParseFailure` 断言 err 文本含文件名 |
| 调用改成**限定名**（`pkg.initPolicyGate`）或经 helper 间接调用 | ✅ 安全方向 → `initGateLine=0` → `Fatal`（**假红**而非假绿）| `call.Fun.(*ast.Ident)` 只匹配非限定调用 |
| **`defer` / `go` 闭包包裹** | ❌ **不兜住 → 假绿** | **R5/R6 实测绿**，见 §四 |

⇒ **除 `defer`/`go` 这一类外，其余静默落空路径都有下界守护**，且未兜住的那类失效方向
不是「不小心踩到」。`found` 字段这个下界设计是有效的。

### 5.2 阳性对照与真实文件是否走同一个扫描函数（第 3 问）—— **是**

`scanWiring` 内部只有**一次** `parser.ParseFile(fset, filename, src, 0)` 调用，
**不对 `src` 是否为 nil 做任何分支**；其后的 `f.Decls` 遍历与 `ast.Inspect` walk 完全共用。
阳性格（内联 `src`）与阴性格（`src=nil` 读真实文件）**走的是同一条代码路径**，
差别仅在 `parser.ParseFile` 内部「从 src 取字节还是从磁盘读字节」。

⇒ **四格对照没有断**。Dev 用内联源码替代 `testdata/` 目录以规避 scope 陷阱
（`writes` 只有 `./cmd/atlas`，Go 语义下不含子目录，新建 `cmd/atlas/testdata/` 会形成
未声明的包路径 ⇒ 门禁报漂移）——**这个规避是对的，且没有付出「阳性格走了另一条路径」的代价**。

### 5.3 扫描器本身是不是自证循环（第 3 问延伸）—— **不是**

判据：阳性格的**输入由测试构造、期望值由人写死**，与被扫描的真实文件无关。

- `DetectsMissingInitCall`：夹具**故意不含** `initPolicyGate` → 断言 `initGateLine == 0`
  **且** `buildLine != 0`（后半条尤其关键 —— 它排除了「扫描器什么都找不到」这种
  两条都为 0 的退化，否则这一格自己就是空转的）
- `DetectsWrongOrder`：夹具**故意颠倒** → 断言 `initGateLine > buildLine`
- `IgnoresOtherFunctions`：夹具在别的函数里放调用 → 断言不计入

三格的期望值都不是「扫真实文件得到的结果」，而是**从夹具构造方式直接推出的**，
因此不构成自证循环。

## 六、备录（不作为 FAIL 依据）

1. §四 的边界声明建议（失败信息说「早于」，实现保证的是词法位置）。
2. Leader 派验单里「`loadConfigOrDefaults` 里也有一处」的事实订正：实际在
   `export_ohlcv.go:297`，不同文件、不在扫描范围内（§3.1）。
3. R3 首版因门②被判无效的记录（§三脚注）—— 若只看退出码会误记成「捕获」。

## 七、复现命令

```bash
git worktree add --detach ../wt-v011r2 519f99bc0bf5914eaf78800c602cca0eb0a46bc7
cd ../wt-v011r2
GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -race
GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -v | grep -c '^--- SKIP'      # 0
GOTOOLCHAIN=local go test ./internal/collector/ ./cmd/atlas/ ./internal/core/ -count=1 \
  -coverpkg=./internal/collector,./cmd/atlas,./internal/core -cover            # 75.8%

# R1 存在性 / R2 顺序 / R3 空真 / R4 同文件他函数边界（改 serve.go 后跑）
GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -run 'TestServeWires|TestScanWiring' -v
#   R3 必须把 `RunE: runServe` 引用一并改名，否则 go vet 不过 = 变异无效（门②）
# R5/R6 边界：把 initPolicyGate 包进 defer / go 闭包 → 按预期保持绿

cd <主仓库> && git worktree remove ../wt-v011r2
```
