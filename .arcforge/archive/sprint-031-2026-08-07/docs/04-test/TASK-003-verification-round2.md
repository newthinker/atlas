# TASK-003 验证报告（第二轮 / 返工复核）—— QuotaStore 配额记账

- 验证者: test-agent-17
- 被验对象: `05cf499206ce60d4029f42a737d197db22933ce4`（`quota_test.go` +19/-4）
- 验证环境: 独立 worktree `../wt-v003r2`（detached @ 05cf499）
- assignment_epoch: **2** / rework_count: 1
- 前序: `TASK-003-verification.md`（首轮 rejected：R1 缺 recover、R2 覆盖声明不准确）

## 结论：**PASS（verified）**

R1/R2 两条修复均经独立复现：**R1 的 2×2 闭环完全成立**（左下角 38 复现），
**R2 的 Q13 拿到真断言**（`:304`），G8 仍为等价变异体（结论不变）。
防回归抽查 12 条**全部断言红**，无一削弱。50 测试 0 SKIP、`-race` 绿、覆盖率 98.3%、
17 包回归绿、scope 仅 `quota_test.go`。

**`gate.go` / `quota.go` 与首轮 `837542f` 逐字节相同** —— 纯测试侧修复，实现零改动。

---

## 一、R1 闭环 2×2（同一 worktree、同一会话）

用「能跑到的 PASS 数」作量化指标 —— 这个指标选得好，它直接度量「一个测试倒下带倒多少」。

| | **无变异** | **注入 Q10**（删 `windowStart` 的 `loc == nil` 兜底） |
|---|---|---|
| **修复后（有 `recover`）** | PASS=**50**，全绿 | PASS=**49**，**断言** |
| **修复前（无 `recover`）** | PASS=**50**，全绿 | PASS=**38**，**PANIC 中断** ← 左下角 |

修复后的报错形状：

```
quota_test.go:117: Loc 为 nil 时不得 panic: time: missing Location in call to Time.In
```

**与 Dev 自报完全一致。** 修复使 **11 个**原本被 panic 中断、根本跑不到的测试恢复执行
（49 − 38 = 11），且报错从 panic 堆栈变成指向设计意图的断言。

四格跑在同一 worktree、同一次会话内，PASS 数差异只能归因于那 5 行 `recover`。

> Dev 在注释里把根因和实测数据一起写进了代码（「实测：删掉 Loc 兜底后 PASS 数从 50 掉到 38」
> 「红的方式决定了其他测试还能不能跑」）。下一位读者不需要重跑就能理解为什么这几行不能删。

---

## 二、R2 复核：覆盖声明已修正

| 变异 | 期望 | 实际 |
|---|---|---|
| **Q13**（给未登记主题配上带 `Quota` 的兜底策略） | 断言红 | **红(断言) ✅** `quota_test.go:304: 未登记主题不得触达配额记账（约束 C6）: Take 调用 3 次 [eastmoney.kline akshare.valuation crypto.ticker], want 0` |
| **G8**（删掉 `fetch` 里的直通短路），定向 | 绿（等价变异体） | **绿 ✅** |
| **G8**，全量 50 测试 | 绿 | **绿 ✅** |

测试注释现在明确写了**守什么 / 不守什么**，并把「两道各自独立充分的短路」这个因果结构
写了进去，末尾收在：

> 「删掉守卫 X 后行为不变」不等于「X 无用」，可能只是存在另一道守卫。

这正是我首轮那条前瞻出错的根因，写在测试旁边比写在报告里有用得多。

---

## 三、防回归抽查（12 条，全部**断言红**，无一削弱）

这 19 行只动 `quota_test.go` 的两处（加 `recover`、改注释），但按纪律实测而非推断。
四道门：① 改动量非空；② `go test -c` 编译通过；③ 核到**断言行**（正则区分 panic 堆栈）；
④ 还原后全部 6 个文件 md5 与基线断言相等。

| 变异 | 覆盖的 DoD | 断言位置 |
|---|---|---|
| Q1 `takeQuota` 挪到查缓存前 | boundary[2]① 缓存命中不消耗配额 | `:194` |
| Q2 `takeQuota` 挪到 singleflight 外侧 | boundary[2]② 合并只消耗 1 次 | `:277`（`Take 调用 20 次, want 1`） |
| Q3 被拒也计数 | boundary[0] | `:150` |
| Q6 丢自然日对齐 | functional[0] | `:83` |
| Q9 短窗口不 Truncate | functional[0] | `:103` |
| Q11 窗口翻篇不重置 | functional[1] | `:167` |
| Q12 账本不隔离 | functional[1] | `:150` |
| Q14 配额用尽仍调 fn | functional[2] | `:199` |
| Q15 忽略 Limit | functional[1] | `:147` |
| Q4 不 fail-open | error_handling 分句1 | `:320` |
| Q5 fail-open 不告警 | error_handling 分句2 | `:327` |
| Q7 `warn` 无兜底 | error_handling 分句3 | `:339` |

Q7 尤其值得看：它转红时是 `未注入 WithWarn 时 fail-open 不得 panic: runtime error: invalid
memory address...` —— **panic 被 recover 转成了断言**，与 R1 修复后的形态一致。
`TestFailOpenWithoutWarnDoesNotPanic` 首轮就写对了，现在两条同型测试终于一致。

---

## 四、覆盖率、回归、约束、scope

| 项 | 结果 |
|---|---|
| 测试 | **50 个全 PASS，0 SKIP，0 FAIL** |
| `-race` | 绿（4.064s） |
| 覆盖率 | **98.3%**（与首轮持平——本轮是断言健壮性提升，不产生新语句） |
| C3 不循环导入 | **仅 policy 自身 ✅** |
| gofmt / vet | 无输出 / exit 0 ✅ |
| 全量回归 | **17 包全部 ok ✅** |
| scope | 仅 `internal/collector/policy/quota_test.go`（19+/4-）✅ |
| 实现零改动 | `git diff 837542f -- gate.go quota.go` **无输出** ✅ |
| 上游产物 | `git diff 6a2a8df -- policy.go policy_test.go` **无输出** → TASK-001/002 产物未动 ✅ |

---

## 五、13 项 DoD 终态

首轮已逐条验证并全部 PASS，本轮抽查确认无一削弱。R1/R2 修复的是**测试健壮性**与
**覆盖声明准确性**，不改变任何 DoD 的覆盖结论。

| # | 完成标准（摘要） | 本轮证据 | 判定 |
|---|---|---|---|
| functional[0] | `windowStart` 自然日对齐 / 短窗口 UTC 截断 | Q6 `:83`、Q9 `:103`；**Q10 现为断言 `:117`** | **PASS** |
| functional[1] | `MemStore.Take` 到 Limit / 翻篇归零 / 账本隔离 | Q15 `:147`、Q11 `:167`、Q12 `:150` | **PASS** |
| functional[2] | 配额用尽返回 `ErrQuotaExceeded` 且 fn 不被调用 | Q14 `:199` | **PASS** |
| functional[3] | `New` 签名变更，TASK-002 用例保持全绿 | 50 测试全绿 | **PASS** |
| boundary[0] | 被拒不计数 / `QuotaStore` 为 nil 放行 | Q3 `:150` | **PASS** |
| boundary[1] | fn 出错的请求必须计数 | 首轮 Q17b `:208` | **PASS** |
| boundary[2]① | 缓存命中不消耗配额 | Q1 `:194` | **PASS** |
| boundary[2]② | 合并的 N 个请求只消耗 1 次 | Q2 `:277` | **PASS** |
| boundary[2]③ | 未登记主题不触达配额记账 | **Q13 `:304`**（G8 为等价变异体，注释已写明） | **PASS** |
| error_handling[0] 三分句 | 放行 / 上报 / 无 warn 时不 panic | Q4 `:320`、Q5 `:327`、Q7 `:339` | **PASS** |
| non_functional[0] | `-race` 全包全绿 | 实测绿 | **PASS** |

---

## 六、复现命令

```bash
git worktree add --detach ../wt-v003r2 05cf499206ce60d4029f42a737d197db22933ce4
cd ../wt-v003r2/internal/collector/policy

# R1 闭环 2x2：组合「有/无 recover 块」×「有/无 Q10 变异」，各跑 go test -v 数 PASS 行
#   Q10 = 删掉 windowStart 里的 `if loc == nil { loc = time.UTC }`
#   期望：50 / 49(断言) / 50 / 38(PANIC 中断)
# R2：Q13 = 把 `if !ok { return fn() }` 换成
#     `p = Policy{Quota: &Quota{Limit: 100, Window: 24 * time.Hour, Loc: time.UTC}}`
#     → 断言红 :304；G8（删直通短路）→ 仍绿（等价变异体）

# 变异四道门：① diff 非空且改的是语义 ② go test -c 编译通过
#            ③ 核到断言行 `^\s+\w+_test\.go:\d+:\s+\S`（区分 panic 堆栈）
#            ④ 还原后 6 个文件 md5 与基线一致
```

worktree 已于验证结束后清理；主工作区 `internal/` 零污染。
