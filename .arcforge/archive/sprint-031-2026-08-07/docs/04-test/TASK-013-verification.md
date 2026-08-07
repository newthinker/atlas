# TASK-013 验证报告 —— AST 防回潮（验收标准 5）

- 验证者: test-agent-17
- 被验对象: `38a3d9c`（含 `d2a6e37` 主体 + `38a3d9c` 重构；`nothrottle_test.go` 323 行 + 4 个 testdata 夹具）
- 验证环境: 独立 worktree `../wt-v013`（detached @ 38a3d9c）
- assignment_epoch: 1 / rework_count: null（首轮，不设搜索限制）

## 结论：**PASS（verified）**

**DoD functional[1] 要求的「变异验证内化为可重放用例」是本轮最该验的一条**，
它的核心论断是：**一个永远返回 false 的扫描函数同样能让本任务全绿**。
我用 V1/V1b 的对照**实证了这个论断**，并确认交付的 2×2 结构确实封住了它。

10 条变异全部符合注入前写下的预期。29 测试 **0 SKIP**、17 包回归绿、全仓 **0 FAIL 包**、
**生产代码零改动**（改动仅测试文件与 `testdata/`）。

---

## 一、DoD functional[1] 的核心：2×2 两格缺一不可（实证）

| 变异 | 定向到 | 预期 | 实际 |
|---|---|---|---|
| **V1** `scan` 恒返回空 offenders | `TestScanDetectsThrottleState`（**阳性**） | 红 | **红** `:204 阳性夹具未被检出 —— 扫描函数没有守护任何东西（它可能恒返回空）` ✓ |
| **V1b** 同一变异 | `TestNoPrivateThrottleState`（**阴性**） | 绿 | **绿** ✓ ← **单靠阴性抓不到** |
| **V2** `scan` 恒把每个字段记为 offender | `TestNoPrivateThrottleState`（阴性） | 红 | **红** `:163` ✓ |

**V1b 是这条 DoD 的直接证据**：恒返回空的扫描器让阴性测试**照常全绿**，只有阳性对照能抓到。
反方向（V2）也成立 —— 恒返非空只有阴性能抓。**两格确实缺一不可。**

代码注释里写的正是这个：

> 只有阴性那格时，一个恒返回空的扫描函数照样全绿；只有阳性那格时，一个恒返回非空的照样全绿。

---

## 二、其余变异（全部符合预期）

| ID | 变异 | 定向到 | 结果 |
|---|---|---|---|
| **V3** | 去掉 root 豁免 `path != root` | `TestScanRootExemptFromSkipRule` | **红** `:268 直接以 policy 为 root 时必须检出它的 lastReq —— 返回空说明跳过规则误伤了 root 自身` |
| **V4** | `skipDirs` 去掉 `testdata` | `TestNoPrivateThrottleState` | **红** `:159 扫描失败: 解析 testdata/parsefail/bad.go 失败` |
| **V5** | `skipDirs` 去掉 `policy` | `TestNoPrivateThrottleState` | **红** `:163 发现私有节流状态字段` |
| **V6** | 不跳过 `_test.go` | `TestScanIgnoresNonFieldOccurrences` | **红** `:287 误报了非结构体字段的出现` + `:295 测试文件不该进入扫描范围` |
| **V7b** | 解析失败静默跳过 | `TestScanReportsParseFailure` | **红** `:310 语法错误的源码必须报错，却返回了 offenders=[] scanned=[]（静默跳过 = 没有守护）` |
| **V8** | 扫描漏掉 tushare 子包 | `TestNoPrivateThrottleState` | **红** `:171 子包 tushare 一个生产源码文件都没被扫到 —— 「不存在 lastReq」是空真的（共扫 35 个文件）` |
| **V9** | offender 行号恒记为 1 | `TestScanDetectsThrottleState` | **红** `:209 字段 lastReq 未被检出` |

**V8 证明下界断言有效**（`scanned` 非空 + 四个子包各自被扫到）—— 「不存在 lastReq」不是空真的。
**V9 证明行号断言有效** —— 不是只比文件名，行号错位不会静默通过。

> V7 首版被门②拦下（删掉 `fmt.Errorf` 致 `fmt` import 未使用）。这是本 Sprint 第 N 次同一形态，
> 门②再次生效；用 `_ = fmt.Sprint(perr)` 保留引用后重做。

---

## 三、几处设计我认为值得记录

**1. 用真实 `policy` 包做阳性语料**（`TestScanRootExemptFromSkipRule`）。注释里的理由是我见过
最准确的教训 13 应用：

> 夹具是我按扫描器的行为造的，二者可能共享同一个盲区；**`policy/gate.go` 不是我写的，它是独立语料。**

它一次钉住两件事：root 豁免生效（V3 红）、扫描器在**真实生产代码**上确有检出能力。

**2. 行号现读不写死**（`lineOfField`）：

> 夹具一改行，写死的期望值就会变成假红（锚点文本要现读现取）。

这正是本 Sprint 反复踩的锚点过期（我的 `-run` 目标、Dev 的正则文本、我的多处替换），
它在写测试时就把这个失效模式排除了。

**3. 强度边界如实声明**：

> 只能挡住「结构体里有个叫 lastReq / minInterval / throttle 的字段」这一种形态。挡不住：
> 改名字段（lastCall、last）、包级 var、局部变量、裸 `time.Sleep`、以及「有 gate 字段但压根没调 Fetch」。

**这个声明是准确的**，我用 V1-V9 未能证伪它。后两类的真实守护确实在各 collector 自己包里
（我验过的构造函数取 `Default()` 的测试、主题常量对内置表的断言）。

**4. `scanNoPanic` 把 panic 转断言**（教训 10），**`testdata` 跳过规则自身由解析失败夹具承重**
（V4 红即其证明）—— 一条原本「删掉不会有人发现」的规则现在有了守护。

---

## 四、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 变异证据 | 判定 |
|---|---|---|---|
| functional[0] | AST 扫描**整棵 collector 树**生产源码，断言无 `lastReq`（跳过 `_test.go`/`policy`/`testdata`）；扫描抽成 `scan(root)` | V2 红 `:163`、V5 红、V6 红、**V8 红（下界）**；`scanPrivateThrottleState(root)` 已参数化并被四个测试复用 | **PASS** |
| functional[1] | **变异验证内化为可重放用例**：夹具须被检出、生产树须不检出 | **V1 红（阳性）/ V1b 绿（阴性）** 实证两格缺一不可；V9 红（行号非只比文件名） | **PASS** |
| boundary[0] | 只覆盖生产源码，不误报测试文件中的同名标识符 | V6 红 `:287`+`:295` | **PASS** |
| error_handling[0] | 目录不存在/解析失败时清晰失败，**不 panic 不静默跳过** | V7b 红 `:310`；`TestScanReportsMissingRoot` 覆盖不存在分支；`scanNoPanic` 把 panic 转断言 | **PASS** |
| non_functional[0] | `go test ./internal/collector/` 全绿，**不引入生产代码改动** | 全绿；`git diff --name-only` 排除 `_test.go` 与 `testdata/` 后**无输出** ✅ | **PASS** |

**5 项全部 PASS。**

---

## 五、重构提交 38a3d9c 已复验

`d2a6e37`（主体）之后有 `38a3d9c`（抽出 `forEachStructFieldName`，-33/+34）。
按 TASK-009 确立的纪律「重构错误路径后必须复验对应守护」—— **我的 V1-V9 全部跑在
`38a3d9c` 上**，十条变异均有效，重构未削弱任何守护。

该重构本身也有价值：`forEachStructFieldName` 同时被生产扫描与 `lineOfField` 使用，
**保证二者永远看的是同一批 AST 节点**（否则「检出的字段」与「现读行号的字段」可能用不同判据）。

---

## 六、覆盖率、回归、约束、scope

| 项 | 结果 |
|---|---|
| `internal/collector` 测试 | **29 个全 PASS，0 SKIP，0 FAIL** |
| collector 树回归 | 17 包全绿 ✅ |
| 全仓回归 | **0 个 FAIL 包** ✅ |
| 生产代码改动 | **无**（`git diff --name-only` 去掉 `_test.go` 与 `testdata/` 后为空）✅ |
| gofmt | `nothrottle_test.go` 与 `testdata/` 均无输出 ✅ |
| scope | 仅 `internal/collector/`（1 测试文件 + 4 夹具）✅ |

---

## 七、复现命令

```bash
git worktree add --detach ../wt-v013 38a3d9c
cd ../wt-v013

GOTOOLCHAIN=local go test ./internal/collector/ -count=1 -v      # 29 PASS 0 SKIP
git diff 04bf776 38a3d9c --name-only | grep -v '_test.go' | grep -v 'testdata/'   # 须无输出

# 核心 2x2（DoD functional[1]）：改 nothrottle_test.go 的 scanPrivateThrottleState 返回
#   V1  恒返回 scanResult{scanned: res.scanned}（丢掉 offenders）
#       → TestScanDetectsThrottleState 红 :204 ；TestNoPrivateThrottleState **绿**（关键对照）
#   V2  把 `if forbiddenFields[name.Name]` 改成 `if true`
#       → TestNoPrivateThrottleState 红 :163
# 其余：V3 去 `path != root`；V4/V5 删 skipDirs 项；V6 不跳 _test.go；
#      V7b 解析失败 return nil（须 `_ = fmt.Sprint(perr)` 保留 import）；
#      V8 额外 SkipDir 掉 tushare；V9 行号恒记为 1
```

worktree 已于验证结束后清理；主工作区零污染。
