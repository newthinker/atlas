# TASK-011 验证报告（交叉校验的「有声跳过」对看终端的人也要成立）

- **验证者**：test-agent-27　**被验树**：`master @ abaca101d54065ba1803b98d19a25558d8692a4f`（= 基线 = 当前 HEAD）
- **discovery sha256** `9cc7cd51…57036` 与基线逐字节一致　**epoch=1，rework_count=0**
- **改动**：2 个文件（`backfill_fetch.go` +33/-2、`backfill_fetch_test.go` +42/-0），与 `writes` 逐条一致、零越界
- **结论**：**VERIFIED（4/4 条 done_criteria 全部通过）**

---

## 1. ★ 补上 dev 主动标注「有理由没观察」的那个消融 —— **它成立**

dev 如实报告：`TestRunBackfillReportsSearchSkipped` 断言「标签 + reason 内容」两样，
但**「只打标签、丢掉 reason」这个消融它没跑**，只有推理。这是它整份交付里唯一一处如此。

**我跑了**：把 `Fprintf(w, "%s %s\n", label, cc.SearchSkippedReason)` 换成 `Fprintln(w, label)`（标签照打、reason 丢掉）：

```
--- FAIL: TestRunBackfillReportsSearchSkipped
    backfill_fetch_test.go:1166
    "…\n交叉校验未生效:\n…" does not contain "search side failed"
```

⇒ **红在第 1166 行那条 `assert.Contains(report, "search side failed")`**，其消息正是
「光有标签不够——reason 的内容才说得出为什么跳过」；而 1163 行的标签断言**照常通过**（标签确实还在）。

**⇒ 两条断言互补属实，且现在有观察撑着，不再只有注释。** dev 的推理正确，但它把「未观察」如实标出来这件事，
比推理正确更要紧——**这是本 sprint 第一次有人把「我这里只有理由、没有观察」写进交付**。

## 2. 它跑过的两个消融，我独立复现，结果一致

| 消融 | 红在哪 |
|---|---|
| T11-a 删掉整段输出 | **仅** `TestRunBackfillReportsSearchSkipped` |
| T11-b 去掉 `if`、无条件打印 | **仅** `TestRunBackfillOmitsSearchSkippedWhenSearchWorks` |

⇒ `functional[0]`（出现）与 `boundary[0]`（正常时不出现）**各自被唯一击杀**，DoD 要的「且只红它」成立。
三个消融合起来覆盖了这一对的三种坏法：不打印 / 无条件打印 / 打印但丢内容。

## 3. done_criteria 逐条

| # | 标准 | 证据 | 判定 |
|---|---|---|---|
| functional[0] | 搜索侧被跳过时 stdout 必须可见说明（含 reason） | T11-a 与 **T11-c** 各自击中该用例；输出实见 `交叉校验未生效: search side failed…` | ✅ |
| boundary[0] | 正常时**不得**出现该说明 | T11-b（无条件打印）⇒ 仅 `Omits…WhenSearchWorks` 红 | ✅ |
| error_handling[0] | 退出码语义不变、fail-open 不变 | 用例前置锚点即 `require.NoError`（搜索侧失效仍 fail-open）；`TestCrossCheckBackfillEmptySearchIsNotFailure` 等既有用例全绿；全套件 **879 RUN / 879 PASS / 0 红** | ✅ |
| non_functional[0] | gofmt/vet/全绿 + **覆盖率不低于 TASK-010 基线** | 见下 | ✅ |

## 4. 覆盖率：门禁守不了这条，我背对背测出来

`coverage_floor = null` ⇒ 门禁比的是全局绝对下限 80（实报 94.8% 通过），
**而 DoD 要的是相对判据**（不低于 TASK-010 交付基线）。**门禁通过不构成对这条的背书**——
这是本 sprint 第二次由 dev 主动划清，判断正确。

我的背对背单变量对照（两棵树各自在自己的源码树里渲染 profile，避开 D7 混渲）：

```
两包合并          pre 2391/2796 = 85.515%  →  post 2393/2798 = 85.525%   (+0.010pp)  下限 85.515 ✅
internal/hestia   pre 1368/1443 = 94.802%  →  post 1370/1445 = 94.810%   (+0.007pp)  下限 94.802 ✅
```

与 dev 报的两个数、Leader 独测的合并值**三方一致**。

⚠️ **顺带把那个解析坑第三次复现了**：同一份 post profile，**naive 按行求和不去重 = 2487/5596 = 44.442%**，
与 Leader 报的 44.442% 逐字相同、与我在 TASK-007 得到的 44.362% 同量级。
⇒ **同一个坑，本 sprint 三个人各踩一次、三次都靠与另一个口径对账才发现**（无一由报错暴露）。

## 5. dev 的 `self_reported_errors` 四条

前两条（harness 锚点二次转义、docstring 提前闭合）**都是响的失败**（有效性闸报 `锚点出现 0 次`、SyntaxError），
未污染结论。它自己点出的对照值得留下：

> 与「语义闸打印了 diff 却没读」正好成对：**同一类闸，执行者是人时失效，执行者是机器时生效。**

这句我认同，而且它正是我在 TASK-007 亲手撞过的那次（我打印了 diff 却没读，转义写错得到「8 条红」的假象）。
**把核对从「人读 diff」挪到「机器精确匹配锚点」是同方向的改进**，建议进 registry。

第 4 条是流程半条错（发请求后把自己留在 `in_progress`，真相源显示成「dev 正在开发」而实为「卡在 Leader 处」），
它补齐成：**发请求之前查自己手上的事，发请求之后把自己的状态改成实话。**

---

## 结论

**VERIFIED（4/4）。** 本轮唯一实质工作是把 dev 自陈「只有理由没有观察」的那条断言补上观察——**结果它成立**，
且红在预期的那一条。三个消融覆盖了这一对的三种坏法，`functional[0]` 与 `boundary[0]` 各被唯一击杀。
覆盖率两条下限均满足（由我背对背测出，门禁结构上守不了）。未发现遗留问题。
