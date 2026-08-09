# 交接缺口追加：H8–H10（QA 阶段发现，Leader 裁定推迟）

> 与 `handoff-to-later-iterations.md` 的 H1–H7、`handoff-h11-cross-package.md` 的 H11 同级。
> **归档时应一并阅读。**
>
> 全部来自 qa-agent-10（Skeptic lens）的实测，非推断，且在返工后的终态（`823ca15`）**复测复现**。
> Leader 裁定：**推迟，不在本 Sprint 修**——三条都需要设计决策或属下游子迭代范围。

## H8 —— 【严重度高于 H3】同期两篇报告：每期静默丢一整份报告的字段

54 个字段**横跨至少两篇央行报告**：A.1 社融（`tsf_*`）与 A.2–A.4（`m2` / `deposit_*` / `loan_*`），**同期、通常同日发布、URL 不同** ⇒ `published_at` 相同而 `article_id` 不同。

`Meta` 每个 `Observation` 只有一个 `ArticleID`，说明 `Observation` 是「一篇文章」粒度。

**实测（终态复现）**：同 period / 同 published_at / 不同 article_id / 字段互补（`article-tsf` 只有 `tsf_stock`，`article-m2` 只有 `m2`）⇒ 第二篇 `err=<nil>`、`Verdict=Duplicate`、**只刷了 `article_id`，`m2` 从未写入**。

比 H3 已登记后果多两层：

1. **频率**：不是一次性迁移，而是**每期都发生**。
2. **与 H6-3 构成永不收敛的循环**：第一篇的 `article_id` 被刷掉 ⇒ M1b-4 的幂等检查永远 miss ⇒ 重抓 ⇒ 判 Duplicate 又刷回第一篇 ⇒ 第二篇再 miss。**两篇互相驱逐，全程 `Save` 返回 nil。**

**未言明的载荷假设**（本包没写下来）：`(period, period_type, published_at)` 唯一确定一篇文章、一个 `Observation` 携带该期全部字段。**Store 无从判断它成不成立。**

**落点要求**：这个数据模型问题**必须在有生产数据之前定案**——它需要 schema 变更，而本包明确不做自动迁移。建议先把契约写进 `Meta` / `Observation` 注释（同期多篇必须在解析层合并成一个 `Observation`），并显式写进 **M1b-2 的 DoD**。

## H9 —— OutOfOrder 幂等不对称

`Classify` 只比 `MAX`，故「这条 revision 已存在但不是最大值」识别不了，直接落 `insert` 撞主键。

**实测（终态复现）**：写 `07-10` → New；写 `08-20` → Revision；**重放 `07-10` → 硬失败**

```
constraint failed: UNIQUE constraint failed: ...(1555)
Verdict=New  Table=""   ← 未落 pending
```

⇒ **Duplicate 幂等、OutOfOrder 不幂等**，而「回填重跑」正是 H3 说必然发生的操作。

不是静默（错误带期次可定位），**但这处不对称既无测试也无契约登记**。

## H10 —— `period` × `period_type` 组合非法被放行

**关键：这不是 H1。** H1 讲的是 `2026-13` 这类**定宽但日历非法**的取值；本条**两值单独都合法、配对非法**——H1 的措辞覆盖不到。

**实测（终态复现）**：`2026-06/annual`、`2026-03/h1`、`2026-12/monthly` 三条 `err` 全 `<nil>`，各建一行。

而 `types.go:36-37` 自己写着「`period_type` 决定除数 1/6/12，写错让信号判定错一个量级」——**而 `2026-06/annual` 正是「用 12 去除半年报期末月」**。

同一份年报若一次写 `2026-12/annual`、一次写 `2026-06/annual` 会变成两个业务键，**修订链分叉、下游双计**。

**落点要求**：必须在 M1b-3 的 DoD 里**单独出现**，不能挂在 H1 名下——否则会顺着 H1 被读成「已登记」而消失。

## 观察级（记录不阻断）

- 全部 error 路径返回 `Outcome{}`，`Verdict` 即 `bitemporal.New(=0)`。store.go:185-187 花三行论证零值 Outcome 的危害并据此调整了 `!rep.Passed` 的位置，**同一风险在每条 error 返回上原样保留**。
- `refreshArticleID` 不看 `RowsAffected`，0 行更新静默返回 nil（仅并发 G9 或 `DB()` 旁路可达，两者已登记）。
- ~~`types.go:116` 的「Skipped 必填 Reason」无强制~~ —— **已在 C2 返工中关闭**：现报 `skipped check(s) c1 have no reason: a skip without a reason cannot be told apart from a gate that silently failed`。
- **空 `Values` 是合法输入**，会写一条全 NULL 业务列的权威行返回 New。**与 H3/H8 叠加不可逆**：一次解析全失败先占坑，之后任何正确重跑都是 Duplicate，字段永远补不回来。

## 附：C1 修复的溯源陷阱

`verifyCurrentView` 的实现随 **`572f2ce`** 一并落盘，而该 commit 的标题只写了「G10 白名单」，**未提视图校验**（成因：三个 review_fix 的 writes 完全相同 + 整文件 `git add`）。

⇒ **按 commit message 追溯 C1 会找错地方**，须按符号名：

```bash
git log --oneline -S 'func verifyCurrentView' -- internal/hestia/store.go
```
