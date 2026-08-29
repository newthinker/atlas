# Changelog · M1c-3a · hestia 解析覆盖

**范围**：`7bccab44` → `0c9f4e87`（47 commits）｜**日期**：2026-08-25 → 2026-08-29

---

## Added

- **社融独立报告解析**：新增 `tsf-stock@v1`(18) / `tsf-flow@v1`(9) 两个 extractor，
  接上此前被 `calibrate.go` 硬过滤的 138 篇社融存量/增量报告。(TASK-002/003)
- **月报解析**：新增 `rule-monthly@v1`(25) / `rule-monthly@v2`(52)，接上 55 篇月报。(TASK-007)
- **`parseTitle` 加 `kind` 字段**，`Parse` 改三路分派（金融统计 / 社融存量 / 社融增量）。(TASK-007)
- **分部门口径守卫** `checkSectorCaliber`：拒绝把当月分部门值装进 `*_ytd`。(TASK-009)
- **`cumulativeRangeAlt`**：`periodAlt` 认「`N-M月`」范围前缀。(TASK-012)
- **诊断器**：分辨「期次前缀不被识别」与「本节没有累计句」两种失败成因 —— 两者
  **后续动作相反**，误判成后者会让真实可恢复的数据被永久写销。(TASK-011)
- **14 份 testdata fixture** + `testdata/README.md`。
- **`CONTRACTS.md` `## Sprint M1c-3a` 一节**（+528 行）：F 节 21 条机制发现、G 节 8 条结转项。

## Changed

- **`detectExtractor` 判据重构**：从「节数魔数」(`len(secs)==6/8`) 改为
  **板块集合 × `period_type`**。旧判据把 5 节季报（2020-09 / 2022-09）和全部四种月报布局
  挡在门外。**节数从来不是模板身份的代理**。(TASK-004)
- **`extractFields` 按 extractor 决定板块适用性**，不再对所有模板要求同一组板块。(TASK-006)
- **`calibrate` 放行社融两种 kind**，月报按口径四类分流。(TASK-010)
- **`extractTSFFlowSection` 改为一行委托 `extractTSFFlowArticle`** ——
  板块路径与整篇路径共用同一段代码。(TASK-011 / QA R1)
- **`checkSectorCaliber` 起点由抽取覆盖面派生**，不再依赖锚点存在。(TASK-011 / QA R2)

## Fixed

- 🔴 **`extractTSFFlowSection` 板块路径无作用域切分** ⇒ 静默产出**错位 18.8×** 的数据
  （`ytd=252100` 配 `rmb_loan=13400`）。板块路径正是 v2 月报的 going-forward 格式。(QA R1)
- 🔴 **`checkSectorCaliber` 锚点缺席即放行**，而抽取不依赖锚点 ⇒ 守卫可被绕过。(QA R2)
- 🔴 **`periodAlt` 不认 `N-M月`** ⇒ 4 期（2022-07/08/10/11）可恢复的数据被贴上
  「不存在」的假标签**并被指示写销**。(QA R3)
- **基线三条失败全部归零**：
  - `2019-12` `loan scope anchor 企（事）业单位贷款 not found`（TASK-005）
  - `2020-09` / `2022-09` `unrecognized layout: 5 sections`（TASK-004，两篇是前三季度报）
- **`calibrate.go:368` 硬编码 `backfillKindFinance`** 而 `Periods` 含 138 篇社融。(TASK-010)
- **`m2` 全角括号 / 企业贷款锚点**两处模板措辞变体。(TASK-005)

## Docs

- **`calibrate-AFTER-report.md` 数字订正**（2026-08-29）：原表采样于 `f74dc49d`，
  三条 HIGH 的修复在此之后合入 ⇒ 「195/34/23」订正为「**199/38/19**」。
  已背对背重跑并保留原文对照。

---

## ⚠️ 读数字前必读

**「解析失败 3 → 38 篇」是可见性提高，不是回退。**

```
34 篇 = 138 篇社融报告此前被硬过滤，既不贡献样本也不产生失败；TASK-010 把它们放进管线
 4 篇 = 2022-07/08/10/11 此前被误标为「没有累计数据」；TASK-012 的 R3 让它们显形
```

**从「静默写销」变成「响亮失败」是本 sprint 最重要的一类改进**，
而它在计数上表现为失败数上涨。**只看那个数会得出完全相反的结论。**

## Known issues（结转 M1c-3b）

见 `internal/hestia/CONTRACTS.md` 的 `## Sprint M1c-3a` → `### G.` 结转清单（8 条）。
**第 1 条 `2022-05` 是活缺陷** —— 已知、可修、本 sprint 明确不修。

⚠️ 凡引用「`Unsupported` 那 19 篇不是兜底工作量」，**先回结转清单核一遍例外**：
`2022-05` 就在那 19 篇里，而它的数据是可恢复的。
