# TASK-015 验证报告（第二轮 / 返工）—— eastmoney policy 错误断链映射

- 验证者: test-agent-17
- 被验对象: `11b7d56f35751ca54e08ea44abf66cd9ad1d012e`（**本次提交** `eastmoney.go` +24/-1、`gate_test.go` +89）
- 验证环境: 独立 worktree `../wt-v015r2`（detached @ 11b7d56）
- assignment_epoch: **2** / rework_count: 1
- 前序: `TASK-015-verification.md`（首轮 rejected，两项 FAIL）

## 结论：**NEEDS WORK（rejected）**，`reason_class = task_defect`

**首轮的两项 FAIL 全部修复**，且修得比我要求的更完整（§一、§二）。
判 rejected 的唯一原因是**派验时新落盘的 `functional[4]`（缓存键聚合度）无守护**：
实现是对的，但**改坏了不会红** —— 我实测变异有效而全量 9 条测试全绿（§三）。

29 测试 0 SKIP、`-race` 绿、eastmoney 单包覆盖率 87.4% → **87.6%**、17 包回归绿、
既有 `eastmoney_test.go` 一字未改。

---

## 一、FAIL①（policy 错误映射）：**已修复**，四个方向全红

`mapPolicyError`（`eastmoney.go:460`）用**断链**而非 `%w`，注释引用了 TASK-017 的变异 B8 实证。

| 变异 | 预期 | 实际 | 断言 |
|---|---|---|---|
| **R1** 撤回映射层（原样返回） | 红 | **红** ✓ | `:299 policy.ErrQuotaExceeded 泄漏到调用方错误链` + `:302 映射后的错误应带本包前缀` |
| **R2** 改用 `%w` 保留 policy 链 | 红 | **红** ✓ | `:299 ... got eastmoney: history 600519.SH: policy: q...` |
| **R3** 映射成永久性措辞（「无此标的」） | 红 | **红** ✓ | `:305 配额是临时性错误，消息须体现（否则调用方会停止重试）` + `:332 超时是临时性错误` |
| **R4** 非 policy 错误也被吞掉 | 红 | **红** ✓ | `:347 非 policy 错误应原样透传，保留原始错误链` |

**R2 是关键**：它证明测试断言的是 `errors.Is(err, policy.ErrXxx) == false`，而非只查本包前缀文本
——「消息带了本包前缀但 `errors.Is` 照样为真」正是断链与包装的分界。

**R4 是 Leader 点名要我补的增量（「换类型不是丢信息」）** —— 实测**已有守护**：
`mapPolicyError` 的 `return err` 分支保留 `fetchHistory` 的原始错误链（含 lixinger 回退的错误），
被 `:347` 钉住。

---

## 二、FAIL②（discovery）：**已修复**，且写全了我提的那一层

直读 `.arcforge/discoveries/TASK-015.json` 核实（不是数关键词，是读句子）：

> ②将来若要接入，**必须深拷贝 `core.Quote.FundInfo`**：它是指针字段（`core/types.go:44`），
> eastmoney 在 `:319-333` 从 `lixingerFallback.FetchFundInfoPublic` 取值设入；
> ③**照抄 yahoo 的 `out := *q` 挡不住** —— 那是浅拷贝，指针字段照样共享。
> **且风险不限于缓存**：即使按 yahoo 的 `yahoo.quote` 那样配 `TTL:0`，只要 `Coalesce: true`
> （`policy.go:73` 正是如此），合并的多个调用方就拿到同一个 `*FundInfo`。

三层全在：为什么不接 / 将来接时必须深拷贝 / **`Coalesce` 即使不缓存也共享**（我提的那层），
且三处事实基础（`types.go:44`、`policy.go:73`、`eastmoney.go:319-333`）都自行核实过并写明。
还关联到 tushare 的 `row.values` 是同一类问题、本 Sprint 已为此返工过（`199ed81`）。

Dev 自己的一句诊断准确：**「原 discovery 只写了『为什么不接』，漏了『接时要注意什么』——
而后者才是不会过时的那半（边界决定将来可能改，注意事项不会）。」**

---

## 三、FAIL：`functional[4]` 缓存键**聚合度**无守护

DoD（派验时新落盘）：

> 以 `time.Now()` 为 end 的两次相邻调用**必须命中同一缓存槽**……
> **变异判据：去掉 `Truncate` 后该测试须转红**。

### 3.1 实测：变异有效，但全量 9 条测试全绿

把 `key` 的 `start/end.Truncate(time.Minute).Unix()` 改成 `UnixNano()`：

| | 全量 9 条测试 |
|---|---|
| **A1（UnixNano）** | **全绿 ❌ 无守护** |

### 3.2 先证明变异确实改变了行为，再下结论

「全绿」有两种解释（测试无守护 / 变异没打中），故写探针直接观测**生产路径形态**
（以 `time.Now()` 为 end，即 `app.go:452` 的调用形态）：

| 实现 | 两次相邻调用的 HTTP 请求数 |
|---|---|
| **变异（`UnixNano`）** | **2 次** —— 每次都新键，**生产路径命中率恒为零** |
| 交付态（`Truncate` 到分钟） | **1 次** —— 命中缓存 |

⇒ **变异有效**（行为确实改变），而无任何测试转红 ⇒ **`functional[4]` 确实无守护。**

### 3.3 定性

**实现是对的**（`eastmoney.go:435` 的 `Truncate` 在），缺的是守护 —— 与首轮 FAIL② 同类：
「结果对，但保证结果的东西没留下」。

本次交付只新增了一条测试（`TestPolicyErrorsDoNotLeak`），**没有聚合度测试**。

> **据实说明**：`functional[4]` 是 Leader **在派验这一刻**才落盘的 criteria，Dev 交付时大概率
> 不知道它的存在。按「DoD 是唯一依据」判为 FAIL，但这不是 Dev 的执行失误。
> Leader 已说明「三家都在补这条」，本条应与 016/017 一并处理。

**修复**：补一条测试 —— 以 `time.Now()` 为 end 连调两次，断言 HTTP 请求 **1 次**
（我 §3.2 的探针即为现成形态）。注意与既有的 `TestCacheKeyCoversAllParams`（区分度）
是两个方向，**都需要**。

---

## 四、其余核查（全部通过）

| 项 | 结果 |
|---|---|
| 既有测试一字未改 | 本次提交仅 `eastmoney.go` + `gate_test.go`，`eastmoney_test.go` **不在其中** ✅ |
| `-race` | 绿 ✅ |
| 0 SKIP | 29 测试 0 SKIP 0 FAIL ✅ |
| 覆盖率（同口径） | eastmoney 单包 87.4% → **87.6%** ✅ |
| collector 树回归 | 17 包全绿 ✅ |

> **scope 口径提醒**：`git diff b238221 11b7d56 --stat` 会列出 baostock/crypto/`cmd/atlas` 的改动
> ——那是 TASK-016/017 的提交夹在中间。**本任务的 scope 须看单提交** `git show --stat 11b7d56`
> （仅 eastmoney 两文件）。这与我上一轮自我修正的「基线 + 在途」是同一件事。

---

## 五、复现命令

```bash
git worktree add --detach ../wt-v015r2 11b7d56
cd ../wt-v015r2

git show --stat 11b7d56              # 本任务 scope（勿用 b238221..11b7d56，含 016/017）
GOTOOLCHAIN=local go test ./internal/collector/eastmoney/ -count=1 -race -cover

# FAIL 依据（functional[4]）：把 key 的 Truncate(time.Minute).Unix() 换成 UnixNano()
#   → 全量 9 条测试**全绿**
#   变异有效性探针：以 time.Now() 为 end 连调两次
#   → 变异下 HTTP 2 次（生产命中率恒为零）；交付态 1 次
# FAIL① 已修复：R1 撤回映射 / R2 改 %w / R3 永久性措辞 / R4 吞掉非 policy 错误 → 四条全红
# FAIL② 已修复：直读 discovery，三层俱全（含 Coalesce 那一层）
```

worktree 已于验证结束后清理；主工作区零污染。
