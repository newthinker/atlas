# TASK-014 验证报告 —— 配额降级链集成回归（验收标准 4 的 prism 侧）

- 验证者: test-agent-17
- 被验对象: `8356662`（仅新增 `internal/prism/quota_degrade_test.go` **+225**）
- 验证环境: 独立 worktree `../wt-v014`（detached @ 8356662）
- assignment_epoch: 1 / rework_count: null（首轮，不设搜索限制）

## 结论：**PASS（verified）**

**验收标准 4 两端闭环** —— 与我验过的 tushare 那端（TASK-009）对上了。
**验收标准 1 双重确认**：`git diff --stat master -- internal/prism/refresh.go` **无输出**，
且 `git log master..8356662 -- internal/prism/refresh.go` **也无输出** ——
不是「改了又改回来」，是**从未被改**。

68 测试 **0 SKIP**、`-race` 绿、覆盖率 **94.0%**（水位 94% ✅）、全仓 **0 FAIL 包**、
scope 仅新增一个测试文件。

---

## 一、Dev 那个假发现：**诊断完全证实**

它报告 N2 注入后三条测试全绿，查出是正则锚在 `return rows, err` 上、
而 **TASK-009 的 (b) 返工已把它改成 `return cloneRows(rows), err`** ⇒ 静默失配、文件零改动。

我独立核实：

```
grep -c 'return rows, err' internal/collector/tushare/client.go   →  0   ← 该文本已不存在
grep -n  'return cloneRows(rows), err' ...                        →  102 ← 被 (b) 改成了这个
```

用那个过期锚跑我的 runner：

```
!! N2-stale **锚定失败 → 变异无效**（文本可能已被上游任务改过）
```

**门①（锚定检查 + 落盘校验）直接拦下。若无这道门，它就会表现为「变异存活」。**

> 这与我上一轮栽的「`-run` 目标过期」是同族：**都是验证工具的锚在被使用时已经不是写下时的那一份**。
> 我那次是测试名过期（守护迁移），它这次是源码文本过期（上游任务改动）。
> 共同点：**跨任务/跨轮次复用变异脚本时，锚点必须重新对齐当前代码**。

我自己的 N1/N2/N3 用的是当前锚点文本，三条全红，**且每条都记录了落盘量**（1+/3-、1+/1-、1+/1-）。

---

## 二、变异验证（预期注入前写下；跨包变异，还原用**仓库根作用域**）

| ID | 变异 | 落盘 | 预期 | 实际 | 关键输出 |
|---|---|---|---|---|---|
| **N1** | tushare 不映射（policy 错误外泄） | 1+/3- | 红 | **红** ✓ | `err = policy: quota exceeded，必须满足 errors.Is(err, tushare.ErrRateLimited)` |
| **N2** | 映射改 `ErrNoPermission` | 1+/1- | 红 | **红** ✓ | `err = tushare: no api permission: daily_basic (本地配额预判，未发出请求)` |
| **N3** | 配额拦截失效（`takeQuota` 恒放行） | 1+/1- | 红 | **红** ✓ | `配额已满时不得真的发出 HTTP 请求（撞墙前拦截）` |
| **N4** | `FileStore.read` 恒返空账本（预填失效） | 2+/1- | 红 | **红** ✓ | 同上 —— **证明「预填账本」这个前提本身有守护，不是摆设** |

> 还原作用域用**仓库根**（`git checkout -- .` 在根目录执行）—— 这三条都是跨包变异
> （改 `internal/collector/tushare` 与 `internal/collector/policy`，测 `internal/prism`），
> 上一轮我正是在子目录执行还原导致跨目录变异残留。

---

## 三、Leader 点名评估的自加设计：**并排比对确实能抓到「不一致」**

`TestLocalQuotaAndServerRateLimitDegradeIdentically` 并排比对两条路径产出的 `Report` 形状
（`Degraded`/`Failed` 条数、`Refreshed` 计数），再对两条路径**同时**断言文案分类。

Dev 的论证是：「DoD 说的是**关系**性质，分别断言两次只能证明各自符合我写下的期望，
**两组期望若都写窄了，不一致仍会溜过去**。」验证它是否成立：

| 变异（只偏离**本地**路径，服务端路径用桩不受影响） | 结果 |
|---|---|
| **N2** 映射改 `ErrNoPermission` | **红** — `"600519.SH: tushare 跳不可用(权限不足,配置性问题,不重试)"` |
| **N1** 不映射（policy 错误外泄） | **红** — `本地配额: 应有 Degraded` |

**N2 的输出尤其有说服力**：本地路径走进了「权限不足/配置性问题/不重试」分支，而服务端路径
仍走限频分支 —— **这正是「行为路径不一致」本身**，被并排比对直接捕获。

⇒ **Dev 的判断成立**，这条自加设计有效。

---

## 四、`Limit: 0` 地雷：处理正确，且两个测试的区分属实

我预读时点名的那条（方案行 4143 的 `Quota{Limit: 0}` 在 policy 语义下是「不设上限」）：

**`TestQuotaErrorSatisfiesRateLimitedAssertion`（有地雷的那个）**：
- 改为 `Limit: 1` + 先成功消耗一次 ✅
- handler 从 `t.Error` 改为**计数**，理由写在注释里：`Limit:1` 下首次请求是**合法**的，
  `t.Error` 会误报
- **首次调用同时充当正向对照**：`require.Equal(t, 1, hits(), "首次调用应真的发出请求")`
  —— 证明「第二次 0 次」是因为被拦下，不是整条配额链没接上（陷阱 8 的同源推论）
- 三条错误断言齐全：is `ErrRateLimited` / not `ErrNoPermission` / **not `policy.ErrQuotaExceeded`**（约束 C2）

**`TestRefreshTushareLocalQuotaBlocksBeforeHTTP`（方案行 4096，没有地雷）**：
- 用 `builtinDailyBasicTable(t)`，且 `require.Equal(t, 5, p.Quota.Limit, "生产配额为 5 次/自然日（ea5ac30 实测）")`
  **显式断言了这个前提**
- 用 `exhaustedLedger(t, "tushare.daily_basic", 5)` **预填账本**至上限，而非靠 `Limit` 取值
- ⇒ **确实不受 `Limit<=0` 语义影响**，Leader 说的区分属实

Dev 在注释里还点明了误诊风险 —— 与我预读时的判断一致：

> 照抄 `Limit: 0` 会让配额永不触发、请求真的发出去 —— 而方案原文那个 handler 写的是
> `t.Error("配额已满时不应发出请求")`，于是失败会**表现成「实现有问题」而不是「测试写错了」**，极易误诊。

---

## 五、「纯回归测试任务无 RED→GREEN」的结构性事实

Dev 指出：被测行为在 TASK-009 已实现，三条测试**一写就全绿**，而「一写就绿的空洞测试」
与「有效测试」表现完全一样 ⇒ **有效性完全依赖变异自证**。

这解释了为什么那次假发现特别危险：**它发生在唯一的证据来源上**。
我的独立复现（§一、§二）正是补上这一环 —— 四条变异全部**先确认落盘再解释红绿**。

---

## 六、Done Criteria 逐条覆盖矩阵

| # | 完成标准（摘要） | 变异证据 | 判定 |
|---|---|---|---|
| functional[0] | 本地配额耗尽时 refresh 流程中 tushare 一次 HTTP 都不发（含 `Limit:0` 地雷规避） | N3 红、N4 红（前提有守护）、`Limit:1` + 正向对照 ✅ | **PASS** |
| functional[1] | 配额错误满足 `errors.Is(err, tushare.ErrRateLimited)`（验收标准 4 prism 侧） | N1 红、N2 红 | **PASS** |
| functional[2] | 降级链继续走向下一源，与限频撞墙路径一致 | **N1/N2 → 并排比对双双红**（§三） | **PASS** |
| boundary[0] | 配额未耗尽时行为与改造前一致，既有 prism 测试全通过 | 68 测试 0 SKIP 全绿；正向对照 `hits==1` | **PASS** |
| error_handling[0] | `policy.ErrQuotaExceeded` 不出现在 prism 可见错误链（约束 C2） | N1 红（外泄即捕获） | **PASS** |
| non_functional[0] | **`refresh.go` 零改动**（验收标准 1） | `git diff` **与** `git log` 双双无输出 ✅ | **PASS** |
| non_functional[1] | `-race` 全绿；覆盖率 ≥ 94% | 绿；**94.0%** | **PASS** |

**7 项全部 PASS。**

---

## 七、覆盖率、回归、约束、scope

| 项 | 结果 |
|---|---|
| 测试 | **68 个全 PASS，0 SKIP，0 FAIL** |
| `-race` | 绿 |
| 覆盖率 | **94.0%**（水位 94% ✅） |
| 全仓回归 | **0 个 FAIL 包** ✅ |
| scope | 仅新增 `internal/prism/quota_degrade_test.go`（+225）✅ |
| 新增文件格式化 | `gofmt -l internal/prism/quota_degrade_test.go` 无输出 ✅ |

> **既有问题备录（非本次引入）**：`gofmt -l internal/prism/` 列出
> `internal/prism/sankey/template_test.go`。核实：**不在本次改动内**
> （`git diff --stat 8356662^ 8356662 -- internal/prism/sankey/` 无输出），
> 且**父提交上就已不干净**。与 TASK-007 的 `yahoo_test.go` 同类，建议 Sprint 末尾统一 `gofmt -w`。

---

## 八、复现命令

```bash
git worktree add --detach ../wt-v014 8356662
cd ../wt-v014      # 变异跨包，还原一律在**仓库根**执行 git checkout -- .

# 验收标准 1（双重）：
git diff --stat master -- internal/prism/refresh.go     # 须无输出
git log --oneline master..8356662 -- internal/prism/refresh.go   # 也须无输出（防「改了又改回来」）

GOTOOLCHAIN=local go test ./internal/prism/ -count=1 -race     # 68 PASS 0 SKIP
GOTOOLCHAIN=local go test ./internal/prism/ -count=1 -cover    # 94.0%

# 变异（每条先 `git diff --numstat` 确认落盘，再 go build，最后才解释红绿）：
#   N1 删掉 tushare/client.go 的 ErrQuotaExceeded→ErrRateLimited 映射块
#   N2 该映射的 ErrRateLimited 改成 ErrNoPermission
#   N3 policy/gate.go 的 takeQuota 恒短路
#   N4 policy/quota_file.go 的 read 恒返空账本（验「预填账本」前提本身有守护）
#   N1/N2 另外定向到 TestLocalQuotaAndServerRateLimitDegradeIdentically → 双双红
#
# ⚠ 锚点必须对齐**当前**代码：`return rows, err` 已被 TASK-009 的 (b) 改成
#   `return cloneRows(rows), err`，用旧锚会静默失配并伪装成「变异存活」。
```

worktree 已于验证结束后清理；主工作区零污染
（其中 `D internal/collector/cache.go` 等属 TASK-011 在途改动，非本次验证所致）。
