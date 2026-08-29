# 转录原文存档（team-lead 亲笔，供验证者逐字比对）

**为什么有这份文件**：M1c-3a 的 TASK-011 / TASK-010 都已 `in_progress`
（`owner_table.in_progress = ["dev-*"]`）⇒ **leader 写不了它们的任务文件**，
两条裁决只能由 dev-m1c3a-a 转录进 `done_criteria`。

test-m1c3a-v1 指出的风险，我认为成立：

> **转录者正在转录一条关于它自己义务的裁决。**
> 不是说它会有意弱化——但**「弱化」不需要有意**：把「必须实现并被守住」写成
> 「已知缺口，留待后续」，**读起来仍然像忠实转录，而验收强度完全不同**。
> ⇒ 验证者若手里没有原文，只能核「读起来合理」，**那是判断不是观察**。

⚠️ 我此前只把原文放在 inbox —— **而 inbox 是 test-v1 排的最弱载体**（只有收信人、过期即失效）。
本文件落在 `docs/06-acceptance/*`（写者恒为 leader），归档时随 sprint 留存。

---

## 原文一 —— 转录进 **TASK-011 的 `done_criteria.error_handling`**

```
【以下由 dev-m1c3a-a 从 team-lead 的 inbox 消息逐字转录落盘，非 leader 亲笔。
 转录理由：TASK-011 已 in_progress，owner_table.in_progress = ["dev-*"]，leader 写不了本文件；
 而该义务若只存在于 inbox 与 TASK-012 的 discovery，下一个验证者不会读到（test-m1c3a-v1 指出）。
 若与 leader 本意有出入，以其原始 inbox 消息为准。】

🔴 **从 M1c-3a 的 TASK-012 转入（team-lead 裁决）**：期次前缀**不被 `periodAlt` 认识**
导致的失败，与**该节确实没有累计句**导致的失败，**错误信息必须可区分**——
不能都收敛成 `not found among N candidate sentence(s)`。

实测同形（dev-m1c3a-b 在 f74dc49 上复现，team-lead 与 test-m1c3a-v1 各自独立复跑确认）：
  2020-04（正文真的没有累计句）        → not found among 1 candidate(s) [4月份/人民币]
  2022-10（有「1-10月…累计增加18.7万亿元」，只是前缀不认识）
                                      → not found among 2 candidate(s) [10月份/人民币 10月份/外币]
结构完全相同，唯一差别是候选数，**而那个数不携带「为什么」**——认不出的前缀根本没进候选集。

两者的后续动作相反：前者是解析器缺口（M1c-4 该修）、后者是报告本身没数据（正确的是标注）。

⚠️ 交叉断言：两种错误串**互不出现**在对方的测试里——「两条分支可区分」是关系性属性，
两边都满足共有属性时它没有守卫。
⚠️ 消融：预先声明该只红哪一格再跑，看失败行号。

【闭环背景（供验证者判断严重度）】TASK-012 因这条被判 REJECTED，dev-b 走「显式申报」，
而 team-lead 批准那次申报的**唯一理由**是「义务转移到 TASK-011」。
若本任务也未覆盖它，**那次申报就退化成一次静默放弃**——两个任务各自申报「不在我 scope 内」，
缺口无人承接。
```

---

## 原文二 —— 转录进 **TASK-010 的 `done_criteria`**

```
【由 dev-m1c3a-a 从 team-lead 的 inbox 消息转录落盘，非 leader 亲笔。
 转录理由：本条原写在 TASK-012 的 interfaces_exposed 里，而 TASK-010.context_from
 不含 TASK-012 ⇒ 送不到（test-m1c3a-v1 指出，team-lead 核实）。】

🔴 **那 4 篇（2022-07/08/10/11）在 TASK-012 合入后已由 `Unsupported` 转入 `Failures`，
这是本任务 scope 内的活行为变化，不是待做项。**

实测（team-lead 在 master 真跑 calibrate；test-m1c3a-v1 独立从代码推导，两者逐字吻合）：

  Periods      195 → 199   (+4，改前那条分支同时执行 res.Periods--)
  Unsupported   23 → 19    (−4)
  Failures      34 → 38    (+4)
  Records / m2  161 / 50   不变   ← 存款侧仍无源，产不出 Observation

机制：`onlyCurrentMonthFlowSentences` **同时读 `cumulativePeriods` 与 `loanFlowRE`**，
而 TASK-012 恰好两个都改了（表里加 `cumulativeRangeAlt`、flowRE 加 `(?:累计)?`）
⇒ 这 4 篇的重分类是**构造上必然**，不是碰巧。

⚠️ **正确的一句话**：「它们从『不解析』变成了『解析失败』，但仍产不出样本。」
两格对下游意义相反 —— `Unsupported` 那格 CONTRACTS G 写「不是 M1c-4 的兜底工作量」，
`Failures` 那格写「M1c-4 要兜的就是这批」。**归到后者是对的。**

⚠️ 本任务的分类理由措辞要与此一致，**不要写成「那 4 篇仍在 Unsupported」**。
```

---

## 给验证者的使用说明

1. **逐字比对**落盘的 `done_criteria` 与上面对应的原文。
2. **不是核「读起来合理」** —— 那是判断；比对原文才是观察。
3. 若有出入，**报出来**，不要替它补齐。参照 test-m1c3a-v1 的那条立场：
   **替它检查等于把载体缺陷藏起来。**

   ⚠️ **但这条有分界，别走到另一个极端**（test-m1c3a-v1 补充）：

   > 「报出来不替它补齐」针对的是**义务性内容的缺失**；
   > 若只是措辞差异而**验收强度等价**，照常验并在报告里注明差异，
   > **不会因为一个逗号把任务退回**。
   > **判据是「这个差异会不会改变验收强度」，不是「是否逐字相同」。**

   ⇒ 逐字比对是**取证手段**，不是验收标准本身。
4. 请在验证报告里注明：**这两条 `done_criteria` 不是 leader 亲笔**，
   来源、转录人、原文出处（本文件）都要写上。
   理由是 test-v1 说的：**`done_criteria` 是最强载体，正因为强，它的来源更该可追溯。**
