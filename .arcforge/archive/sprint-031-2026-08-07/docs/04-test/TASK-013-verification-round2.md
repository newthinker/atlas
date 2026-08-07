# TASK-013 验证报告（第二轮 / review_fix 返工）—— 畸形夹具改为运行时生成

- 验证者: test-agent-17
- 被验对象: `b95d98d`（`nothrottle_test.go` +31/-4；删除 `testdata/parsefail/bad.go`）
- 验证环境: 独立 worktree `../wt-v013r2`（detached @ b95d98d）
- assignment_epoch: 1 / `rework_count` 未消耗（`reason_class=dod_defect`）
- 前序: `TASK-013-verification.md`（首轮 verified）

## 结论：**PASS（verified）**

29 测试 **0 SKIP**、17 包回归绿、全仓 **0 FAIL 包**、**生产代码零改动**。
我列的四项核查全部完成，其中**第一项证伪了我自己的预判** —— 而证伪的方式比预判有价值。

---

## 一、我的预判被证伪：不是「失去承重」，是「**承重方式转移**」

我预判：畸形夹具移出仓库后，`skipDirs` 的 `testdata` 项会退回「删掉不会有人发现」的状态。

**实测（核到断言行）**：

| | 返工前（`38a3d9c`） | 返工后（`b95d98d`） |
|---|---|---|
| **N2**（`skipDirs` 去掉 `testdata`） | 红 —— `:159 扫描 internal/collector 失败: 解析 testdata/parsefail/bad.go 失败` | 红 —— **`:167 发现私有节流状态字段（限流须走 policy 的 Gate）`** |

**同一个变异、同一条测试转红，红的原因换了** —— 从「撞上畸形文件而**崩溃中断**」变成
「`throttleback` 夹具**被真正检出为 offender**」。**后者是更语义化的守护**。

我的预判错在一个事实：**只有 `parsefail` 被移走了**，`decoys/` 与 `throttleback/` **刻意留在仓库**
（Dev 的注释写明理由：它们可编译、对 `go test` 无害、且**作为语料需要被人读**）。
`testdata` 跳过项因此仍然必要，且承重者从「解析失败夹具」变成了「阳性夹具」。

> **若只看红/绿会以为什么都没变。** 这是「判红要核到断言行」在**返工回归**场景的价值：
> 它不只用于分辨真红假红，还用于发现**「红还在但守护的东西已经不同了」**。
> —— 这条我认同并会带进后续工作。

---

## 二、其余三项核查

| 项 | 变异 | 结果 |
|---|---|---|
| **N6**（`TestScanReportsParseFailure`） | 解析失败静默跳过 | **红** `:337 语法错误的源码必须报错，却返回了 offenders=[] scanned=[]（静默跳过 = 没有守护）` |
| **V3**（真实 `policy` 包做语料） | 去掉 root 豁免 `path != root` | **红** `:272 直接以 policy 为 root 时必须检出它的 lastReq` ✅ **不受夹具迁移影响，如预期** |
| **`lineOfField` 行号现读** | V9 offender 行号恒记为 1 | **红** `:213 字段 lastReq 未被检出` ✅ **迁移中没丢** |

「行号现读不写死」这个设计在返工后**仍然成立** —— 它是原实现最好的一处，我特意确认了。

---

## 三、N10 我的变异没打中（据实记录）

**N10**（把 `fmt.Errorf("解析 %s 失败: %w", path, perr)` 的 `%s` 去掉）→ **预期红实际绿**。

诊断：探针打印错误全文 ——

```
解析失败: /var/folders/.../bad.go:3:14: expected ')', found 'EOF'
```

**文件名从 `perr` 里泄漏出来了**（Go parser 的错误信息本身就含完整路径），
所以 `strings.Contains(err.Error(), badFile)` 仍然满足。**我的变异形态选窄了。**

**N10b 重做**（错误完全丢掉原始信息，只留固定文案）→ **红**
`:340 错误信息必须指出是哪个文件，got: 解析失败(第 0 个文件)`

⇒ 那条断言**有效**，只是它守的是「**错误必须携带文件定位信息**」（无论来自 `path` 还是 `perr`），
而不是「`fmt.Errorf` 里必须写 `%s`」。**不构成缺陷。**

---

## 四、防回归抽查（改动动了测试文件本体，故重跑核心格）

| 变异 | 定向到 | 结果 |
|---|---|---|
| **V1** `scan` 恒返空 | 阳性 `TestScanDetectsThrottleState` | **红** `:208` |
| **V1b** 同一变异 | 阴性 `TestNoPrivateThrottleState` | **绿** ← 2×2 两格仍缺一不可 |
| **V2** `scan` 恒记为 offender | 阴性 | **红** `:167` |
| **V8** 漏掉 tushare 子包 | 阴性（下界断言） | **红** `:175 子包 tushare 一个生产源码文件都没被扫到 —— 「不存在 lastReq」是空真的（共扫 35 个文件）` |
| **V6** 不跳过 `_test.go` | `TestScanIgnoresNonFieldOccurrences` | **红** `:291`+`:299` |

首轮验证的守护**无一被这次改动削弱**，DoD functional[1] 的 2×2 结构完好。

---

## 五、三步改动核实

| 改动 | 核实 |
|---|---|
| 畸形源码改为 `t.TempDir()` 运行时现写 | ✅ 注释写明了根因（`packages` 字段同时驱动漂移判据与 `go test`，两个约束在「畸形夹具存在于仓库」形态下不可兼得）；`badFile` 常量化，断言复用同一个值（不写死两处） |
| 删除 `testdata/parsefail/bad.go` | ✅ `decoys`/`throttleback` **刻意保留**，理由充分 |
| 头部映射注释修正（「四个子包」→「整棵树」） | ✅ 与 DoD 现文一致 —— 「**过时的声明会让下一个人去追一个不存在的偏差**」 |

畸形源码的构造也有讲究：`"package parsefail\n\nfunc broken(\n"` —— 注释说明「失败点在**包子句之后**，
确保走的是 `ParseFile` 的语法错误分支而不是『根本不是 Go 文件』」。**这是把变异形态选窄的风险
在写测试时就排除了**（与我 §三 犯的正是同一类问题的反面）。

---

## 六、Done Criteria 终态

| # | 完成标准（摘要） | 本轮证据 | 判定 |
|---|---|---|---|
| functional[0] | AST 扫全树、跳过 `_test.go`/`policy`/`testdata`、抽成 `scan(root)` | V2 `:167`、V6 `:291`、**V8 `:175`（下界）**、N2 `:167` | **PASS** |
| functional[1] | 变异验证内化为可重放用例 | **V1 红（阳性）/ V1b 绿（阴性）** 2×2 完好；V9 `:213` | **PASS** |
| boundary[0] | 只覆盖生产源码，不误报测试文件 | V6 `:291`+`:299` | **PASS** |
| error_handling[0] | 目录不存在/解析失败清晰失败，不 panic 不静默跳过 | N6 `:337`、**N10b `:340`**、`scanNoPanic` 转断言 | **PASS** |
| non_functional[0] | `go test ./internal/collector/` 全绿，**不引入生产代码改动** | 全绿；`git diff --name-only` 去掉 `_test.go`/`testdata/` 后**无输出** ✅ | **PASS** |

**5 项全部 PASS。**

---

## 七、复现命令

```bash
git worktree add --detach ../wt-v013r2 b95d98d
cd ../wt-v013r2

GOTOOLCHAIN=local go test ./internal/collector/ -count=1 -v      # 29 PASS 0 SKIP
git diff 38a3d9c b95d98d --name-only | grep -v '_test.go' | grep -v 'testdata/'   # 须无输出

# 承重方式转移（核心）：`var skipDirs = {"policy": true}`（去掉 testdata）
#   → TestNoPrivateThrottleState 红 :167「发现私有节流状态字段」
#   （返工前同一变异红在 :159「解析 testdata/parsefail/bad.go 失败」——**红的原因换了**）
# N6 解析失败 return nil（须 `_ = fmt.Sprint(perr)` 保留 import）→ :337
# N10 只去掉 %s **测不出**（文件名从 perr 泄漏）；N10b 丢掉全部原始信息 → :340
# V3 去掉 `path != root` → :272（用真实 policy 包，独立于夹具形态）
# V9 行号恒记为 1 → :213（行号现读设计仍成立）
# 2x2：V1 恒返空 → 阳性 :208 红 / 阴性**绿**；V2 恒记 offender → 阴性 :167 红
```

worktree 已于验证结束后清理；主工作区零污染。
