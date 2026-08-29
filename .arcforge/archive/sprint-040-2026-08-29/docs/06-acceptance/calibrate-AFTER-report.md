# M1c-3a 真跑验收：calibrate 输出全文

> **provenance**：原文正文（下方 `calibrate` 输出）由 **dev-m1c3a-a** 真跑产出；
> **落盘由 team-lead 代执行**——`docs/06-acceptance/*` 的写者在 `write-matrix.json` 的
> `rules` 里恒为 `leader`，`dev-*` 无权写该目录，且 `check_path_writer` 走 `rules` 不走 `writes`
> ⇒ 加进 dev 的声明也没用。这是 DoD 设计失误（把一条 leader 的动作写进了 dev 的 DoD），
> 非 dev 执行问题。详见 contracts-checklist 第 92 条。
>
> **2026-08-29 订正与重采：由 team-lead 执行**（见下方「🔴 订正」一节）。

---

## 🔴 订正（2026-08-29，team-lead）：本报告原表测在三条 HIGH 修复**之前**的树上

**原报告采样于 `f74dc49d`（08-26 21:26）。此后合入了 QA 判 REJECT 的三条修复：**

```
f74dc49d  08-26 21:26  ← 原报告在这里采样
cc8baf6   08-27 09:09  feat(TASK-012): R3 —— periodAlt 认「N-M月」
e600b39   08-28 16:07  fix(TASK-011): R1 板块路径作用域切分 + R2 口径守卫
f280d3c   08-29 06:46  fix(TASK-011): error_handling[0] 分辨两种失败成因
0c9f4e8   08-29 16:29  ← 当前 master
```

⇒ **原报告的「195 / 34 / 23」是修复前的值，当前值是「199 / 38 / 19」。**

⚠️ **这不是质量下降，恰恰相反**——详见下面「订正后的四项判据」与「背对背对照」两节。
**但如果只看原表就去划 M1c-4 的范围，会漏掉 4 个期次。**

### 这个缺陷本身值得记

**本报告是本 sprint 唯一面向人类的验收产物**，而它在**修复合入后没有被重采**——
因为 TASK-008 后两轮返工都是**文档改动**，验证者据「docs-only ⇒ 数字与前两轮逐字一致」
放行，**这个推断对 `go test` 成立、对本报告不成立**：本报告的数字不来自被改动的文档，
来自**另一批文件（`extract.go` / `profiles.go` / `sections.go`）的改动**。

⇒ **「本轮改动是 docs-only」不蕴含「一切自证数字仍然有效」**——要问的是
**「这些数字依赖哪些文件？那些文件从采样点到现在动过吗？」**
判据是 `git diff --numstat <采样锚> <当前 HEAD>`，不是「我这轮改了什么」。

⚠️ **它躲过了三道检查**：dev 自查、验证者三轮、QA 三轮。共同原因是三方都在看
**「本轮改动」**，而危害来自**「采样锚之后的全部改动」**——两者在 TASK-008 的
最后两轮里恰好不相交。

---

## 订正后的四项判据（当前 master `0c9f4e8`）

| 观察项 | 基线 @`4a12794` | 原报告 @`f74dc49d` | **当前 @`0c9f4e8`** | 变了吗 |
|---|---|---|---|---|
| 尝试解析 | 25 期 | 195 期 | **199 期** | ⬆ +4 |
| 非社融 `n`（`m2`） | 22 | 50 | **50** | 不变 |
| 社融 `n`（`tsf_stock`） | 4 | 79 | **79** | 不变 |
| 环比一节 | 整节 `—` | annual 6 / monthly 68 | **annual 6 / monthly 68** | 不变 |
| 解析失败 | 3 篇 | 34 篇 | **38 篇** | ⬆ +4 |
| 本迭代不解析 | 193 篇 | 23 篇 | **19 篇** | ⬇ −4 |
| 基线那 3 条失败 | 3 篇 | 全部归零 | **仍全部归零**（已重核） | 不变 |

**四格恒等式（当前）**：待解析 199 + 本迭代不解析 19 + 标题解析不出 0 = **218** ✓

⚠️ **两个 `4` 是同一批期次**：`2022-07 / 2022-08 / 2022-10 / 2022-11` 的**金融统计数据报告**
从「本迭代不解析」移进了「解析失败」。

### 计数用两把独立的尺各算一遍

汇总行说 38 / 19；**数清单条目**也得 38 / 19（不是读同一行两次）。分组：

```
20  社融增量总量        18  社融增量分项(8) + 分部门/住户(5) + 期内合计(4) + M1缺(1)
合计 38 ✓
```

---

## 🔴 为什么「失败从 34 涨到 38」是修复在起作用

R3（`periodAlt` 认 `N-M月`）让这 4 期**进入了管线**。它们此前被归进
「本迭代不解析：该期报告**只有当月数、正文无任何期内累计口径的合计句**」——
**那句话是假的**：它们有累计句，只是前缀（`今年前N个月`）不被识别。

⇒ 修复前：**真实可恢复的数据被贴上「不存在」的标签静默排除**。
⇒ 修复后：它们**响亮失败**，进入 M1c-4 的兜底工作量清单。

**从「静默写销」变成「响亮失败」是本 sprint 最重要的一类改进**，
而它在计数上表现为「失败数上涨」。**只看那个数会得出完全相反的结论。**

同一修复还改写了 `2022-05` 的分类理由（它仍在 19 篇里，但理由整句换掉了）：

```
修复前：该期报告只有当月数、正文无任何期内累计口径的合计句 …（假）
修复后：失败的成因是**期次前缀不被识别**，不是本节没有累计句：正文里有 1 句
        人民币合计句的句尾完全正确，但它们的前缀 今年前5个月 不在 periodAlt 里…
        误判成后者会让真实可恢复的数据被永久写销（M1c-3a 的 TASK-012 的 R3 就是这么发生的）
```

⚠️ **`2022-05` 是那 19 篇里的已知例外**（CONTRACTS.md `### G.` 结转清单第 1 条）。
凡引用「`Unsupported` 那 19 篇不是兜底工作量」，**先回结转清单核一遍例外**。

---

## 🔴 R1 / R2 在这份语料上不改变任何抽取值——这是观察，不是对修复的怀疑

PRE 与 POST 的**字段分布段除首行（`尝试 N 期`）外逐字节一致**：
所有 `n` / `min` / `p5` / `median` / `p95` / `max` 全等。

⇒ R1（板块路径作用域切分）与 R2（口径守卫改按抽取覆盖面判）在
**这份 2026-08-14 语料上不触发**。QA 判它们 HIGH 的理由是
「板块路径正是 v2 月报的 **going-forward** 格式」——**going-forward 的意思就是这份语料还没有**。

⚠️ **不要把这读成「修复没必要」**：它们有单测且 9/9 变异 KILLED。
正确的读法是——**这两条修复对本语料是预防性的，它们的价值要到语料里出现该格式那天才兑现**，
而那天没有任何东西会提醒你。**这正是它们必须现在修的理由。**

⚠️ 同样**不要**把它读成「已确认这份语料不含该格式」：
我观察到的是「**输出没变**」，不是「**该路径没被走到**」。两者不等价。

---

## 两条必须与数字一起读，否则会被误解成质量下降（原文保留，数字已订正）

### 1. `m2` 实测 50 vs 推算「约 80」——差 30 有精确解释

```
80 篇金融统计报告 − 23 篇（只有当月数，无累计） − 7 篇（解析失败） = 50
```

**推算没错，是它的隐含前提不成立**——「约 80」假定了 80 篇全部可解析，
而真实语料里有 30 篇本身就没有累计数据或口径混装。

⚠️ **订正**：上式里的 `23` 与 `7` 是修复前的分桶。**`m2` 的 `n` 仍是 50**（未变），
因为新进入管线的 4 期全部失败、不贡献样本。**等式的两端都对，中间的分桶已变。**

社融侧相反：推算约 73、实测 **79** = 69 篇社融存量 + 年报 7 + h1 1 + q1 1 + q1_q3 1。
**79 > 73 不是超额完成**，是口径包含了非社融存量的报告。

### 2. 失败清单 3 → 38 是**可见性提高**，不是回退

DoD 判据表写「3 条 → **0 条**，或只剩新暴露的」——实测 **38 篇**，
而**基线那 3 条确实全部归零** ⇒ 满足的是「或」的后半句。

**38 篇全部是「新暴露」而非「新引入」**：

```
34 篇 = 那 138 篇社融报告此前被 calibrate.go 硬过滤掉，既不贡献样本也不产生失败；
        是 TASK-010 把它们放进管线，失败才显形。
 4 篇 = 2022-07/08/10/11，此前被误标为「没有累计数据」，是 TASK-012 的 R3 让它们显形。
```

原 34 篇的成因拆分（仍然有效）：

```
27 篇 = 社融增量报告（19 × 总量句是单月口径、8 × 分项句缺）
 4 篇 = 分部门段口径混装（2023-07/08/10/11，金融统计报告）
 3 篇 = 2020-01 M1 缺 / 2020-02 住户存款孪生句 / 2026-01 社融增量总量
```

⚠️ `2026-01` 的错误信息在 `f280d3c` 后**变得精确得多**（从「pattern 不匹配」变成
「候选句口径是单月、而字段名带 `_ytd`，拒绝回落」），条目本身仍在。

## 基线三条失败的归零核实（已在当前 master 重核）

| 期次 | 基线错误 | 修它的任务 | 现状 @`0c9f4e8` |
|---|---|---|---|
| 2019-12 | `loan scope anchor 企（事）业单位贷款 not found` | M1c-3a 的 TASK-005 | 归零（失败清单出现 0 次）|
| 2020-09 | `unrecognized layout: 5 sections` | M1c-3a 的 TASK-004 | 归零（失败清单出现 0 次）|
| 2022-09 | `unrecognized layout: 5 sections` | M1c-3a 的 TASK-004 | 归零（失败清单出现 0 次）|

⚠️ 后两篇是**前三季度报**，不是月报。

---

## 背对背对照的方法（可复现）

两份输出**同一时刻、同一语料、同样的 flag** 产生，不是跨时间点取的历史值：

```bash
git worktree add --detach <tmp> f74dc49d4678a7599681ef2dae29f41ee3f2908e
(cd <tmp> && GOTOOLCHAIN=local go build -o /tmp/atlas-PRE ./cmd/atlas)
GOTOOLCHAIN=local go build -o /tmp/atlas-POST ./cmd/atlas      # 于 0c9f4e8

D=data/hestia-backfill-2026-08-14
/tmp/atlas-PRE  hestia backfill calibrate --dir "$D" --allow-incomplete > /tmp/cal-PRE.txt
/tmp/atlas-POST hestia backfill calibrate --dir "$D" --allow-incomplete > /tmp/cal-POST.txt
diff /tmp/cal-PRE.txt /tmp/cal-POST.txt
```

**自证**：`/tmp/cal-PRE.txt` 与本报告**原先嵌入的 185 行逐字节一致**
⇒ PRE/POST 的差异**全部归因于代码改动**，不含环境或语料漂移。

```
cal-PRE.txt   sha256 b3551fcfc26a5a989212d3a706987980bcafb1115dbac77aaad813977f8dbbe8  （185 行）
cal-POST.txt  sha256 b71d88f65a40d1ec6dac529e98c9820a74829bea3129fcc61f2c11738e91272e  （186 行）
```

### PRE → POST 完整差异（39 行，逐字原样）

```
2,3c2,3
<   待解析（金融统计数据报告，受支持期次）: 195 篇
<   本迭代不解析: 23 篇
---
>   待解析（受支持期次）: 199 篇
>   本迭代不解析: 19 篇
9c9
<     - 3 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-11, 2021-11, 2022-11
---
>     - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-11, 2021-11
13,16c13
<     - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [5月份/人民币 5月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-05
<     - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [7月份/人民币 7月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-07
<     - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [8月份/人民币 8月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-08
<     - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [10月份/人民币 10月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-10
---
>     - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 失败的成因是**期次前缀不被识别**，不是本节没有累计句: 正文里有 1 句人民币合计句的句尾完全正确，但它们的前缀 今年前5个月 不在 periodAlt 里，于是整条模板不命中、根本没进候选集。这是**解析器缺口**——该往 periodAlt 与 cumulativePeriods 同步加一项；与「本节确实没有累计句」的后续动作相反：后者修不了，正确的做法是标注。误判成后者会让真实可恢复的数据被永久写销（M1c-3a 的 TASK-012 的 R3 就是这么发生的）: 2022-05
18c15
<   解析失败（M1c-4 的兜底工作量）: 34 篇
---
>   解析失败（M1c-4 的兜底工作量）: 38 篇
36a34
>     - 2022-07  articles/2025092212552757258.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [7月份/人民币 7月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
37a36
>     - 2022-08  articles/2025092212552776983.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [8月份/人民币 8月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
38a38
>     - 2022-10  articles/2025092212552824305.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [10月份/人民币 10月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
39a40
>     - 2022-11  articles/2025092212552893388.html  hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
52c53
<     - 2026-01  articles/2026021314205610794.html  hestia: 社融增量总量 not found (pattern 社会融资规模增量累计为([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
---
>     - 2026-01  articles/2026021314205610794.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [无期次前缀/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
58c59
< 标定报告：尝试解析 195 期
---
> 标定报告：尝试解析 199 期
63c64
< 字段分布（尝试 195 期；n = 该字段实际取到的样本数）
---
> 字段分布（尝试 199 期；n = 该字段实际取到的样本数）
125c126
< 解析失败（该支持却失败了，M1c-3 入库前要清零）（34 篇）
---
> 解析失败（该支持却失败了，M1c-3 入库前要清零）（38 篇）
143a145
>   2022-07  金融统计数据报告  articles/2025092212552757258.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [7月份/人民币 7月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
144a147
>   2022-08  金融统计数据报告  articles/2025092212552776983.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [8月份/人民币 8月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
145a149
>   2022-10  金融统计数据报告  articles/2025092212552824305.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [10月份/人民币 10月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
146a151
>   2022-11  金融统计数据报告  articles/2025092212552893388.html  hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
159c164
<   2026-01  金融统计数据报告  articles/2026021314205610794.html  hestia: 社融增量总量 not found (pattern 社会融资规模增量累计为([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
---
>   2026-01  金融统计数据报告  articles/2026021314205610794.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [无期次前缀/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
161c166
< 本迭代不解析（不是失败）（23 篇）
---
> 本迭代不解析（不是失败）（19 篇）
177,181c182
<   2022-05  金融统计数据报告  articles/2025092212552655029.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [5月份/人民币 5月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
<   2022-07  金融统计数据报告  articles/2025092212552757258.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [7月份/人民币 7月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
<   2022-08  金融统计数据报告  articles/2025092212552776983.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [8月份/人民币 8月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
<   2022-10  金融统计数据报告  articles/2025092212552824305.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [10月份/人民币 10月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
<   2022-11  金融统计数据报告  articles/2025092212552893388.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
---
>   2022-05  金融统计数据报告  articles/2025092212552655029.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 失败的成因是**期次前缀不被识别**，不是本节没有累计句: 正文里有 1 句人民币合计句的句尾完全正确，但它们的前缀 今年前5个月 不在 periodAlt 里，于是整条模板不命中、根本没进候选集。这是**解析器缺口**——该往 periodAlt 与 cumulativePeriods 同步加一项；与「本节确实没有累计句」的后续动作相反：后者修不了，正确的做法是标注。误判成后者会让真实可恢复的数据被永久写销（M1c-3a 的 TASK-012 的 R3 就是这么发生的）
```

---

# calibrate 输出全文 @`0c9f4e8`（185 行，逐字原样）

```
标定输入: data/hestia-backfill-2026-08-14
  待解析（受支持期次）: 199 篇
  本迭代不解析: 19 篇
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [4月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-04, 2021-04
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [5月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-05, 2021-05
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [7月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-07, 2021-07
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [8月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-08, 2021-08
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [10月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-10, 2021-10
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-11, 2021-11
    - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [2月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2021-02
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [2月份/人民币 2月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-02, 2023-02
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [4月份/人民币 4月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-04, 2023-04
    - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 失败的成因是**期次前缀不被识别**，不是本节没有累计句: 正文里有 1 句人民币合计句的句尾完全正确，但它们的前缀 今年前5个月 不在 periodAlt 里，于是整条模板不命中、根本没进候选集。这是**解析器缺口**——该往 periodAlt 与 cumulativePeriods 同步加一项；与「本节确实没有累计句」的后续动作相反：后者修不了，正确的做法是标注。误判成后者会让真实可恢复的数据被永久写销（M1c-3a 的 TASK-012 的 R3 就是这么发生的）: 2022-05
    - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: section ordinals are not consecutive from 一: got 4 sections, section[0] is "二、" but should start with "一、". A section title that fails to anchor at line start is dropped silently, and a rule@v2 report missing exactly its two 社融 sections becomes indistinguishable from a valid rule@v1 one — so this is refused rather than guessed. Common cause: leading whitespace before the ordinal (stripHTML folds runs of spaces but does not remove them at line start): 2023-05
  解析失败（M1c-4 的兜底工作量）: 38 篇
    - 2020-01  articles/2025092212550543854.html  hestia: 货币 M1 not found (pattern 狭义货币[(（]M1[)）]余额([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元)，同比(净融资|融资|增加|减少|增长|下降)([0-9]+(?:\.[0-9]+)?)%)
    - 2020-02  articles/2025092212550537670.html  hestia: 存款分部门 住户存款 matched 2 sentences (pattern 住户存款(净融资|融资|增加|减少|增长|下降)([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元)): refusing to pick one — a template is expected to hit exactly once; more than one means the section carries twin sentences (e.g. a month-to-date and a current-month figure) and leftmost-first would choose silently, with both values looking plausible
    - 2020-04  articles/2025092212550698119.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2020-05  articles/2025092212550754182.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2020-07  articles/2025092212550867086.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年7月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2020-08  articles/2025092212550983034.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年8月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2020-10  articles/2025092212551089565.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年10月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2020-11  articles/2025092212551158195.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年11月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-02  articles/2025092212551348672.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-04  articles/2025092212551557589.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-05  articles/2025092212551650395.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-07  articles/2025092212551764950.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年7月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-08  articles/2025092212551890602.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年8月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-10  articles/2025092212552026494.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年10月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-11  articles/2025092212552056909.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年11月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2022-02  articles/2025092212552466986.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2022-04  articles/2025092212552570253.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2022-05  articles/2025092212552660938.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2022-07  articles/2025092212552757258.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [7月份/人民币 7月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
    - 2022-07  articles/2025092212552780603.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2022-08  articles/2025092212552776983.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [8月份/人民币 8月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
    - 2022-08  articles/2025092212552713374.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2022-10  articles/2025092212552824305.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [10月份/人民币 10月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
    - 2022-10  articles/2025092212552885604.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2022-11  articles/2025092212552893388.html  hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
    - 2022-11  articles/2025092212552812893.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2023-02  articles/2025092212553041814.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2023-04  articles/2025092212553273042.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2023-05  articles/2025092212553232699.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2023-07  articles/2025092212553361129.html  hestia: 人民币存款分部门段的期次前缀 "7月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
    - 2023-07  articles/2025092212553390836.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2023-08  articles/2025092212553479819.html  hestia: 人民币存款分部门段的期次前缀 "8月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
    - 2023-08  articles/2025092212553432047.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2023-10  articles/2025092212553553490.html  hestia: 人民币存款分部门段的期次前缀 "10月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
    - 2023-10  articles/2025092212553537621.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2023-11  articles/2025092212553629758.html  hestia: 人民币存款分部门段的期次前缀 "11月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
    - 2023-11  articles/2025092212553691180.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2026-01  articles/2026021314205610794.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [无期次前缀/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  标题解析不出期次: 0 条
  fetch 阶段未抓到: 0 篇
  ⚠ manifest 没有 reconcile 对账摘要（出自 TASK-010 之前）：看不出这份分布是不是算在一条有洞的序列上
⚠ 该 manifest 无完成标记（completed_at），已按 --allow-incomplete 放行；若它出自 TASK-010 之前的 fetch，属预期。**这不代表已确认完整** —— 夭折的产物与正常完成的在结构上无法区分。

标定报告：尝试解析 199 期

存疑（1 条，不阻断，说的是语料的性质）
  - ⚠ manifest 没有 reconcile 对账摘要（出自 TASK-010 之前）：看不出这份分布是不是算在一条有洞的序列上

字段分布（尝试 199 期；n = 该字段实际取到的样本数）
字段                              n 期次类型                            min           p5       median          p95          max  建议区间
tsf_stock                      79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1       251.31       262.24       359.02       456.89       463.27  [39.35000000000002,675.23]
tsf_stock_yoy                  79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1          7.4          7.8          9.6         13.3         13.7  [1.1000000000000014,20]
tsf_flow_ytd                   52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6        50700        59200       188700       348600       356000  [-254600,661300]
tsf_stock_rmb_loan             79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1       151.57       158.82       223.96        277.3       279.16  [23.97999999999996,406.75000000000006]
tsf_stock_rmb_loan_yoy         79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1          5.2          5.6         10.9         13.3         13.5  [-3.1000000000000005,21.8]
tsf_stock_fx_loan              79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         1.05         1.13         1.86         2.41         2.49  [-0.3900000000000001,3.9300000000000006]
tsf_stock_fx_loan_yoy          79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        -34.5        -30.9           -7          9.6         12.6  [-81.6,59.7]
tsf_stock_entrust              79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        10.87        10.91        11.22        11.36        11.45  [10.29,12.03]
tsf_stock_entrust_yoy          79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         -7.6         -6.6         -0.7          3.6          4.1  [-19.299999999999997,15.799999999999999]
tsf_stock_trust                79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         3.74         3.75         4.36         7.43         7.49  [-0.009999999999999787,11.24]
tsf_stock_trust_yoy            79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        -32.1        -30.5         -5.1         11.1           13  [-77.2,58.1]
tsf_stock_bankaccept           79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         1.92         2.08         2.82         3.83         4.06  [-0.21999999999999975,6.199999999999999]
tsf_stock_bankaccept_yoy       79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        -24.7        -21.8         -8.2         15.2           32  [-81.4,88.7]
tsf_stock_corp_bond            79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        23.47        25.21        31.42        35.52        36.47  [10.469999999999999,49.47]
tsf_stock_corp_bond_yoy        79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         -0.7         -0.4          6.7         20.6         21.5  [-22.9,43.7]
tsf_stock_govt_bond            79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        37.73        39.31        62.02        99.37       102.68  [-27.22000000000002,167.63000000000002]
tsf_stock_govt_bond_yoy        79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         10.1         12.2           16         21.3         22.1  [-1.9000000000000021,34.1]
tsf_stock_equity               79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         7.36         7.48        10.85         12.4         12.6  [2.120000000000001,17.84]
tsf_stock_equity_yoy           79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1          2.5          2.6          8.4           15         15.3  [-10.3,28.1]
tsf_flow_rmb_loan_ytd          52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6        34900        42000       123100       200300       222200  [-152400,409500]
tsf_flow_govt_bond_ytd         52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6         2437         4140        44500       119500       138400  [-133526,274363]
tsf_flow_corp_bond_ytd         52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6         1486         3865        15000        33300        44500  [-41528,87514]
tsf_flow_fx_loan_ytd           52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6        -5254        -3241          -80         2531         3482  [-13990,12218]
tsf_flow_entrust_ytd           52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6        -9396        -3190         -513         1203         3579  [-22371,16554]
tsf_flow_trust_ytd             52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6 -20099.999999999996       -11000          305         3734         3976  [-44175.99999999999,28051.999999999996]
tsf_flow_bankaccept_ytd        52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6        -4916        -3440          260         5635         5797  [-15629,16510]
tsf_flow_equity_ytd            52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6          422          536         2149         8923        12400  [-11556,24378]
m2                             50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6       198.65       213.49       303.31       353.86       356.71  [40.59000000000003,514.77]
m2_yoy                         50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6          6.2          6.3          8.6         12.1         12.7  [-0.29999999999999893,19.2]
m1                             50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        57.51        60.23        67.17       115.93       119.32  [-4.299999999999997,181.13]
m1_yoy                         50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         -7.4         -6.6            4          8.1         14.7  [-29.5,36.8]
m0                             50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         7.72         8.24        11.88        14.75        15.14  [0.29999999999999893,22.560000000000002]
m0_yoy                         50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         -3.9          5.4         11.5         15.3         18.5  [-26.299999999999997,40.9]
deposit_balance                50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6       192.88       207.48       294.92       344.45       346.47  [39.289999999999964,500.06000000000006]
deposit_balance_yoy            50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6          5.8          6.3          8.6         11.3         12.7  [-1.0999999999999996,19.599999999999998]
deposit_flow_ytd               50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        35700        43200       153600 257399.99999999997       264100  [-192700,492500]
deposit_household_ytd          50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        14800        52400        89400       146400       178400  [-148800,342000]
deposit_corp_ytd               50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6       -32300       -28400 11399.999999999998        54900        65700  [-130300,163700]
deposit_fiscal_ytd             50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        -3143        -2434         6738        20700        22100  [-28386,47343]
deposit_nbfi_ytd               50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6 -11100.000000000002        -3713        18800        64100        67400  [-89600,145900]
loan_balance                   50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6       153.11        165.2       248.73       281.02       282.63  [23.590000000000032,412.15]
loan_balance_yoy               50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6          5.1          5.5          8.8         12.8         13.2  [-3,21.299999999999997]
loan_flow_ytd                  50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        35800        49000       127600       199500       227500  [-155900,419200]
loan_hh_short_ytd              50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        -9281        -7328         1324        18400        19800  [-38362,48881]
loan_hh_mlt_ytd                50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6          628         1199        11800        54500        60800  [-59544,120972]
loan_corp_total_ytd            50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        25500        38600       104800       156800       179100  [-128100,332700]
loan_corp_short_ytd            50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         5755        10100        28200        45300 48099.99999999999  [-36589.99999999999,90444.99999999999]
loan_corp_mlt_ytd              50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        20400        30400        66200       110600       135700  [-94900,251000]
loan_bill_ytd                  50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6       -15000       -11000         3645        21100        29600  [-59600,74200]
loan_nbfi_ytd                  50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        -4720        -4223         -185         4943         5946  [-15386,16612]
rate_ibo                       50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         1.26          1.3          1.7         2.09         2.16  [0.3599999999999999,3.0600000000000005]
rate_repo                      50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         1.31         1.36         1.72         2.16         2.19  [0.43000000000000016,3.07]
fx_reserve                     27 annual×7,h1×7,monthly×2,q1×5,q1_q3×6         3.03         3.06          3.2         3.36         3.42  [2.6399999999999997,3.81]
fx_rate                        27 annual×7,h1×7,monthly×2,q1×5,q1_q3×6       6.3482       6.3757       7.0074       7.1884       7.2258  [5.470600000000001,8.103399999999999]

环比变化率分布：tsf_stock 相邻期（按 period_type 分档）
period_type                     n          min           p5       median          p95          max  建议区间
annual                          6 0.0800074056441588 0.0800074056441588 0.09575653391907804 0.13338108312442792 0.13338108312442792  [0.026633728163889675,0.18675476060469703]
monthly                        68 0.0008756327815022389 0.0018847039818110972 0.007176791670878336 0.022912621359223333 0.026130169926258526  [-0.024378904363254048,0.051384707071014814]

解析失败（该支持却失败了，M1c-3 入库前要清零）（38 篇）
  2020-01  金融统计数据报告  articles/2025092212550543854.html  hestia: 货币 M1 not found (pattern 狭义货币[(（]M1[)）]余额([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元)，同比(净融资|融资|增加|减少|增长|下降)([0-9]+(?:\.[0-9]+)?)%)
  2020-02  金融统计数据报告  articles/2025092212550537670.html  hestia: 存款分部门 住户存款 matched 2 sentences (pattern 住户存款(净融资|融资|增加|减少|增长|下降)([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元)): refusing to pick one — a template is expected to hit exactly once; more than one means the section carries twin sentences (e.g. a month-to-date and a current-month figure) and leftmost-first would choose silently, with both values looking plausible
  2020-04  社会融资规模增量统计数据报告  articles/2025092212550698119.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2020-05  社会融资规模增量统计数据报告  articles/2025092212550754182.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2020-07  社会融资规模增量统计数据报告  articles/2025092212550867086.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年7月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2020-08  社会融资规模增量统计数据报告  articles/2025092212550983034.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年8月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2020-10  社会融资规模增量统计数据报告  articles/2025092212551089565.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年10月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2020-11  社会融资规模增量统计数据报告  articles/2025092212551158195.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年11月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-02  社会融资规模增量统计数据报告  articles/2025092212551348672.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-04  社会融资规模增量统计数据报告  articles/2025092212551557589.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-05  社会融资规模增量统计数据报告  articles/2025092212551650395.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-07  社会融资规模增量统计数据报告  articles/2025092212551764950.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年7月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-08  社会融资规模增量统计数据报告  articles/2025092212551890602.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年8月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-10  社会融资规模增量统计数据报告  articles/2025092212552026494.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年10月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-11  社会融资规模增量统计数据报告  articles/2025092212552056909.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年11月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2022-02  社会融资规模增量统计数据报告  articles/2025092212552466986.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2022-04  社会融资规模增量统计数据报告  articles/2025092212552570253.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2022-05  社会融资规模增量统计数据报告  articles/2025092212552660938.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2022-07  金融统计数据报告  articles/2025092212552757258.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [7月份/人民币 7月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-07  社会融资规模增量统计数据报告  articles/2025092212552780603.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2022-08  金融统计数据报告  articles/2025092212552776983.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [8月份/人民币 8月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-08  社会融资规模增量统计数据报告  articles/2025092212552713374.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2022-10  金融统计数据报告  articles/2025092212552824305.html  hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [10月份/人民币 10月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-10  社会融资规模增量统计数据报告  articles/2025092212552885604.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2022-11  金融统计数据报告  articles/2025092212552893388.html  hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-11  社会融资规模增量统计数据报告  articles/2025092212552812893.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2023-02  社会融资规模增量统计数据报告  articles/2025092212553041814.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2023-04  社会融资规模增量统计数据报告  articles/2025092212553273042.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2023-05  社会融资规模增量统计数据报告  articles/2025092212553232699.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2023-07  金融统计数据报告  articles/2025092212553361129.html  hestia: 人民币存款分部门段的期次前缀 "7月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
  2023-07  社会融资规模增量统计数据报告  articles/2025092212553390836.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2023-08  金融统计数据报告  articles/2025092212553479819.html  hestia: 人民币存款分部门段的期次前缀 "8月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
  2023-08  社会融资规模增量统计数据报告  articles/2025092212553432047.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2023-10  金融统计数据报告  articles/2025092212553553490.html  hestia: 人民币存款分部门段的期次前缀 "10月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
  2023-10  社会融资规模增量统计数据报告  articles/2025092212553537621.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2023-11  金融统计数据报告  articles/2025092212553629758.html  hestia: 人民币存款分部门段的期次前缀 "11月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
  2023-11  社会融资规模增量统计数据报告  articles/2025092212553691180.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2026-01  金融统计数据报告  articles/2026021314205610794.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [无期次前缀/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd

本迭代不解析（不是失败）（19 篇）
  2020-04  金融统计数据报告  articles/2025092212551389740.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [4月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2020-05  金融统计数据报告  articles/2025092212550715747.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [5月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2020-07  金融统计数据报告  articles/2025092212550828957.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [7月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2020-08  金融统计数据报告  articles/2025092212550941703.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [8月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2020-10  金融统计数据报告  articles/2025092212551011240.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [10月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2020-11  金融统计数据报告  articles/2025092212551123039.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-02  金融统计数据报告  articles/2025092212551391606.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [2月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-04  金融统计数据报告  articles/2025092212551536969.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [4月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-05  金融统计数据报告  articles/2025092212551690890.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [5月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-07  金融统计数据报告  articles/2025092212551732948.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [7月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-08  金融统计数据报告  articles/2025092212551874357.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [8月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-10  金融统计数据报告  articles/2025092212552053662.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [10月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-11  金融统计数据报告  articles/2025092212552039519.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-02  金融统计数据报告  articles/2025092212552450108.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [2月份/人民币 2月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-04  金融统计数据报告  articles/2025092212552571879.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [4月份/人民币 4月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-05  金融统计数据报告  articles/2025092212552655029.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 失败的成因是**期次前缀不被识别**，不是本节没有累计句: 正文里有 1 句人民币合计句的句尾完全正确，但它们的前缀 今年前5个月 不在 periodAlt 里，于是整条模板不命中、根本没进候选集。这是**解析器缺口**——该往 periodAlt 与 cumulativePeriods 同步加一项；与「本节确实没有累计句」的后续动作相反：后者修不了，正确的做法是标注。误判成后者会让真实可恢复的数据被永久写销（M1c-3a 的 TASK-012 的 R3 就是这么发生的）
  2023-02  金融统计数据报告  articles/2025092212553043320.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [2月份/人民币 2月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2023-04  金融统计数据报告  articles/2025092212553299625.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [4月份/人民币 4月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2023-05  金融统计数据报告  articles/2025092212553223669.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: section ordinals are not consecutive from 一: got 4 sections, section[0] is "二、" but should start with "一、". A section title that fails to anchor at line start is dropped silently, and a rule@v2 report missing exactly its two 社融 sections becomes indistinguishable from a valid rule@v1 one — so this is refused rather than guessed. Common cause: leading whitespace before the ordinal (stripHTML folds runs of spaces but does not remove them at line start)
```

---

# 附：原报告嵌入的输出 @`f74dc49d`（184 行，逐字原样，保留以便对照）

⚠️ **这一份已过期**，只用于与上一节对照。**划范围请用上一节。**

```
标定输入: data/hestia-backfill-2026-08-14
  待解析（金融统计数据报告，受支持期次）: 195 篇
  本迭代不解析: 23 篇
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [4月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-04, 2021-04
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [5月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-05, 2021-05
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [7月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-07, 2021-07
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [8月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-08, 2021-08
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [10月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-10, 2021-10
    - 3 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2020-11, 2021-11, 2022-11
    - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [2月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2021-02
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [2月份/人民币 2月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-02, 2023-02
    - 2 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [4月份/人民币 4月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-04, 2023-04
    - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [5月份/人民币 5月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-05
    - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [7月份/人民币 7月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-07
    - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [8月份/人民币 8月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-08
    - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [10月份/人民币 10月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning: 2022-10
    - 1 × [金融统计数据报告] 本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: section ordinals are not consecutive from 一: got 4 sections, section[0] is "二、" but should start with "一、". A section title that fails to anchor at line start is dropped silently, and a rule@v2 report missing exactly its two 社融 sections becomes indistinguishable from a valid rule@v1 one — so this is refused rather than guessed. Common cause: leading whitespace before the ordinal (stripHTML folds runs of spaces but does not remove them at line start): 2023-05
  解析失败（M1c-4 的兜底工作量）: 34 篇
    - 2020-01  articles/2025092212550543854.html  hestia: 货币 M1 not found (pattern 狭义货币[(（]M1[)）]余额([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元)，同比(净融资|融资|增加|减少|增长|下降)([0-9]+(?:\.[0-9]+)?)%)
    - 2020-02  articles/2025092212550537670.html  hestia: 存款分部门 住户存款 matched 2 sentences (pattern 住户存款(净融资|融资|增加|减少|增长|下降)([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元)): refusing to pick one — a template is expected to hit exactly once; more than one means the section carries twin sentences (e.g. a month-to-date and a current-month figure) and leftmost-first would choose silently, with both values looking plausible
    - 2020-04  articles/2025092212550698119.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2020-05  articles/2025092212550754182.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2020-07  articles/2025092212550867086.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年7月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2020-08  articles/2025092212550983034.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年8月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2020-10  articles/2025092212551089565.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年10月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2020-11  articles/2025092212551158195.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年11月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-02  articles/2025092212551348672.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-04  articles/2025092212551557589.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-05  articles/2025092212551650395.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-07  articles/2025092212551764950.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年7月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-08  articles/2025092212551890602.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年8月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-10  articles/2025092212552026494.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年10月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2021-11  articles/2025092212552056909.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年11月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2022-02  articles/2025092212552466986.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2022-04  articles/2025092212552570253.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2022-05  articles/2025092212552660938.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2022-07  articles/2025092212552780603.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2022-08  articles/2025092212552713374.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2022-10  articles/2025092212552885604.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2022-11  articles/2025092212552812893.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2023-02  articles/2025092212553041814.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2023-04  articles/2025092212553273042.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2023-05  articles/2025092212553232699.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
    - 2023-07  articles/2025092212553361129.html  hestia: 人民币存款分部门段的期次前缀 "7月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
    - 2023-07  articles/2025092212553390836.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2023-08  articles/2025092212553479819.html  hestia: 人民币存款分部门段的期次前缀 "8月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
    - 2023-08  articles/2025092212553432047.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2023-10  articles/2025092212553553490.html  hestia: 人民币存款分部门段的期次前缀 "10月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
    - 2023-10  articles/2025092212553537621.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2023-11  articles/2025092212553629758.html  hestia: 人民币存款分部门段的期次前缀 "11月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
    - 2023-11  articles/2025092212553691180.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
    - 2026-01  articles/2026021314205610794.html  hestia: 社融增量总量 not found (pattern 社会融资规模增量累计为([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  标题解析不出期次: 0 条
  fetch 阶段未抓到: 0 篇
  ⚠ manifest 没有 reconcile 对账摘要（出自 TASK-010 之前）：看不出这份分布是不是算在一条有洞的序列上
⚠ 该 manifest 无完成标记（completed_at），已按 --allow-incomplete 放行；若它出自 TASK-010 之前的 fetch，属预期。**这不代表已确认完整** —— 夭折的产物与正常完成的在结构上无法区分。

标定报告：尝试解析 195 期

存疑（1 条，不阻断，说的是语料的性质）
  - ⚠ manifest 没有 reconcile 对账摘要（出自 TASK-010 之前）：看不出这份分布是不是算在一条有洞的序列上

字段分布（尝试 195 期；n = 该字段实际取到的样本数）
字段                              n 期次类型                            min           p5       median          p95          max  建议区间
tsf_stock                      79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1       251.31       262.24       359.02       456.89       463.27  [39.35000000000002,675.23]
tsf_stock_yoy                  79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1          7.4          7.8          9.6         13.3         13.7  [1.1000000000000014,20]
tsf_flow_ytd                   52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6        50700        59200       188700       348600       356000  [-254600,661300]
tsf_stock_rmb_loan             79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1       151.57       158.82       223.96        277.3       279.16  [23.97999999999996,406.75000000000006]
tsf_stock_rmb_loan_yoy         79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1          5.2          5.6         10.9         13.3         13.5  [-3.1000000000000005,21.8]
tsf_stock_fx_loan              79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         1.05         1.13         1.86         2.41         2.49  [-0.3900000000000001,3.9300000000000006]
tsf_stock_fx_loan_yoy          79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        -34.5        -30.9           -7          9.6         12.6  [-81.6,59.7]
tsf_stock_entrust              79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        10.87        10.91        11.22        11.36        11.45  [10.29,12.03]
tsf_stock_entrust_yoy          79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         -7.6         -6.6         -0.7          3.6          4.1  [-19.299999999999997,15.799999999999999]
tsf_stock_trust                79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         3.74         3.75         4.36         7.43         7.49  [-0.009999999999999787,11.24]
tsf_stock_trust_yoy            79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        -32.1        -30.5         -5.1         11.1           13  [-77.2,58.1]
tsf_stock_bankaccept           79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         1.92         2.08         2.82         3.83         4.06  [-0.21999999999999975,6.199999999999999]
tsf_stock_bankaccept_yoy       79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        -24.7        -21.8         -8.2         15.2           32  [-81.4,88.7]
tsf_stock_corp_bond            79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        23.47        25.21        31.42        35.52        36.47  [10.469999999999999,49.47]
tsf_stock_corp_bond_yoy        79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         -0.7         -0.4          6.7         20.6         21.5  [-22.9,43.7]
tsf_stock_govt_bond            79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1        37.73        39.31        62.02        99.37       102.68  [-27.22000000000002,167.63000000000002]
tsf_stock_govt_bond_yoy        79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         10.1         12.2           16         21.3         22.1  [-1.9000000000000021,34.1]
tsf_stock_equity               79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1         7.36         7.48        10.85         12.4         12.6  [2.120000000000001,17.84]
tsf_stock_equity_yoy           79 annual×7,h1×1,monthly×69,q1×1,q1_q3×1          2.5          2.6          8.4           15         15.3  [-10.3,28.1]
tsf_flow_rmb_loan_ytd          52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6        34900        42000       123100       200300       222200  [-152400,409500]
tsf_flow_govt_bond_ytd         52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6         2437         4140        44500       119500       138400  [-133526,274363]
tsf_flow_corp_bond_ytd         52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6         1486         3865        15000        33300        44500  [-41528,87514]
tsf_flow_fx_loan_ytd           52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6        -5254        -3241          -80         2531         3482  [-13990,12218]
tsf_flow_entrust_ytd           52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6        -9396        -3190         -513         1203         3579  [-22371,16554]
tsf_flow_trust_ytd             52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6 -20099.999999999996       -11000          305         3734         3976  [-44175.99999999999,28051.999999999996]
tsf_flow_bankaccept_ytd        52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6        -4916        -3440          260         5635         5797  [-15629,16510]
tsf_flow_equity_ytd            52 annual×7,h1×7,monthly×25,q1×7,q1_q3×6          422          536         2149         8923        12400  [-11556,24378]
m2                             50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6       198.65       213.49       303.31       353.86       356.71  [40.59000000000003,514.77]
m2_yoy                         50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6          6.2          6.3          8.6         12.1         12.7  [-0.29999999999999893,19.2]
m1                             50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        57.51        60.23        67.17       115.93       119.32  [-4.299999999999997,181.13]
m1_yoy                         50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         -7.4         -6.6            4          8.1         14.7  [-29.5,36.8]
m0                             50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         7.72         8.24        11.88        14.75        15.14  [0.29999999999999893,22.560000000000002]
m0_yoy                         50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         -3.9          5.4         11.5         15.3         18.5  [-26.299999999999997,40.9]
deposit_balance                50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6       192.88       207.48       294.92       344.45       346.47  [39.289999999999964,500.06000000000006]
deposit_balance_yoy            50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6          5.8          6.3          8.6         11.3         12.7  [-1.0999999999999996,19.599999999999998]
deposit_flow_ytd               50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        35700        43200       153600 257399.99999999997       264100  [-192700,492500]
deposit_household_ytd          50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        14800        52400        89400       146400       178400  [-148800,342000]
deposit_corp_ytd               50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6       -32300       -28400 11399.999999999998        54900        65700  [-130300,163700]
deposit_fiscal_ytd             50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        -3143        -2434         6738        20700        22100  [-28386,47343]
deposit_nbfi_ytd               50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6 -11100.000000000002        -3713        18800        64100        67400  [-89600,145900]
loan_balance                   50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6       153.11        165.2       248.73       281.02       282.63  [23.590000000000032,412.15]
loan_balance_yoy               50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6          5.1          5.5          8.8         12.8         13.2  [-3,21.299999999999997]
loan_flow_ytd                  50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        35800        49000       127600       199500       227500  [-155900,419200]
loan_hh_short_ytd              50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        -9281        -7328         1324        18400        19800  [-38362,48881]
loan_hh_mlt_ytd                50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6          628         1199        11800        54500        60800  [-59544,120972]
loan_corp_total_ytd            50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        25500        38600       104800       156800       179100  [-128100,332700]
loan_corp_short_ytd            50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         5755        10100        28200        45300 48099.99999999999  [-36589.99999999999,90444.99999999999]
loan_corp_mlt_ytd              50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        20400        30400        66200       110600       135700  [-94900,251000]
loan_bill_ytd                  50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6       -15000       -11000         3645        21100        29600  [-59600,74200]
loan_nbfi_ytd                  50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6        -4720        -4223         -185         4943         5946  [-15386,16612]
rate_ibo                       50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         1.26          1.3          1.7         2.09         2.16  [0.3599999999999999,3.0600000000000005]
rate_repo                      50 annual×7,h1×7,monthly×25,q1×5,q1_q3×6         1.31         1.36         1.72         2.16         2.19  [0.43000000000000016,3.07]
fx_reserve                     27 annual×7,h1×7,monthly×2,q1×5,q1_q3×6         3.03         3.06          3.2         3.36         3.42  [2.6399999999999997,3.81]
fx_rate                        27 annual×7,h1×7,monthly×2,q1×5,q1_q3×6       6.3482       6.3757       7.0074       7.1884       7.2258  [5.470600000000001,8.103399999999999]

环比变化率分布：tsf_stock 相邻期（按 period_type 分档）
period_type                     n          min           p5       median          p95          max  建议区间
annual                          6 0.0800074056441588 0.0800074056441588 0.09575653391907804 0.13338108312442792 0.13338108312442792  [0.026633728163889675,0.18675476060469703]
monthly                        68 0.0008756327815022389 0.0018847039818110972 0.007176791670878336 0.022912621359223333 0.026130169926258526  [-0.024378904363254048,0.051384707071014814]

解析失败（该支持却失败了，M1c-3 入库前要清零）（34 篇）
  2020-01  金融统计数据报告  articles/2025092212550543854.html  hestia: 货币 M1 not found (pattern 狭义货币[(（]M1[)）]余额([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元)，同比(净融资|融资|增加|减少|增长|下降)([0-9]+(?:\.[0-9]+)?)%)
  2020-02  金融统计数据报告  articles/2025092212550537670.html  hestia: 存款分部门 住户存款 matched 2 sentences (pattern 住户存款(净融资|融资|增加|减少|增长|下降)([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元)): refusing to pick one — a template is expected to hit exactly once; more than one means the section carries twin sentences (e.g. a month-to-date and a current-month figure) and leftmost-first would choose silently, with both values looking plausible
  2020-04  社会融资规模增量统计数据报告  articles/2025092212550698119.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2020-05  社会融资规模增量统计数据报告  articles/2025092212550754182.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2020-07  社会融资规模增量统计数据报告  articles/2025092212550867086.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年7月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2020-08  社会融资规模增量统计数据报告  articles/2025092212550983034.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年8月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2020-10  社会融资规模增量统计数据报告  articles/2025092212551089565.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年10月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2020-11  社会融资规模增量统计数据报告  articles/2025092212551158195.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2020年11月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-02  社会融资规模增量统计数据报告  articles/2025092212551348672.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-04  社会融资规模增量统计数据报告  articles/2025092212551557589.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-05  社会融资规模增量统计数据报告  articles/2025092212551650395.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-07  社会融资规模增量统计数据报告  articles/2025092212551764950.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年7月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-08  社会融资规模增量统计数据报告  articles/2025092212551890602.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年8月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-10  社会融资规模增量统计数据报告  articles/2025092212552026494.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年10月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2021-11  社会融资规模增量统计数据报告  articles/2025092212552056909.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2021年11月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2022-02  社会融资规模增量统计数据报告  articles/2025092212552466986.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2022-04  社会融资规模增量统计数据报告  articles/2025092212552570253.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2022-05  社会融资规模增量统计数据报告  articles/2025092212552660938.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2022年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2022-07  社会融资规模增量统计数据报告  articles/2025092212552780603.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2022-08  社会融资规模增量统计数据报告  articles/2025092212552713374.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2022-10  社会融资规模增量统计数据报告  articles/2025092212552885604.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2022-11  社会融资规模增量统计数据报告  articles/2025092212552812893.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2023-02  社会融资规模增量统计数据报告  articles/2025092212553041814.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年2月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2023-04  社会融资规模增量统计数据报告  articles/2025092212553273042.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年4月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2023-05  社会融资规模增量统计数据报告  articles/2025092212553232699.html  hestia: 社融增量总量（年初至今累计口径）not found among 1 candidate sentence(s) [2023年5月/单月]: refusing to fall back to a current-month sentence — it has the right magnitude and format but the wrong caliber, and the field is named *_ytd
  2023-07  金融统计数据报告  articles/2025092212553361129.html  hestia: 人民币存款分部门段的期次前缀 "7月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
  2023-07  社会融资规模增量统计数据报告  articles/2025092212553390836.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2023-08  金融统计数据报告  articles/2025092212553479819.html  hestia: 人民币存款分部门段的期次前缀 "8月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
  2023-08  社会融资规模增量统计数据报告  articles/2025092212553432047.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2023-10  金融统计数据报告  articles/2025092212553553490.html  hestia: 人民币存款分部门段的期次前缀 "10月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
  2023-10  社会融资规模增量统计数据报告  articles/2025092212553537621.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2023-11  金融统计数据报告  articles/2025092212553629758.html  hestia: 人民币存款分部门段的期次前缀 "11月份" 不是累计口径，拒绝把当月分部门值装进 *_ytd 字段: 同一份观测里合计字段取的是累计值，混进当月的分部门值会让内部口径不一致，而两者都在合法量级内、下游没有任何闸门拦得住
  2023-11  社会融资规模增量统计数据报告  articles/2025092212553691180.html  hestia: 社融增量分项 对实体经济发放的人民币贷款 not found (pattern 对实体经济发放的人民币贷款(净融资|融资|增加|减少|增长|下降) ?([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))
  2026-01  金融统计数据报告  articles/2026021314205610794.html  hestia: 社融增量总量 not found (pattern 社会融资规模增量累计为([0-9]+(?:\.[0-9]+)?)(万亿美元|万亿元|亿美元|亿元))

本迭代不解析（不是失败）（23 篇）
  2020-04  金融统计数据报告  articles/2025092212551389740.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [4月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2020-05  金融统计数据报告  articles/2025092212550715747.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [5月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2020-07  金融统计数据报告  articles/2025092212550828957.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [7月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2020-08  金融统计数据报告  articles/2025092212550941703.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [8月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2020-10  金融统计数据报告  articles/2025092212551011240.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [10月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2020-11  金融统计数据报告  articles/2025092212551123039.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-02  金融统计数据报告  articles/2025092212551391606.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [2月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-04  金融统计数据报告  articles/2025092212551536969.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [4月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-05  金融统计数据报告  articles/2025092212551690890.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [5月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-07  金融统计数据报告  articles/2025092212551732948.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [7月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-08  金融统计数据报告  articles/2025092212551874357.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [8月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-10  金融统计数据报告  articles/2025092212552053662.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [10月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2021-11  金融统计数据报告  articles/2025092212552039519.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-02  金融统计数据报告  articles/2025092212552450108.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [2月份/人民币 2月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-04  金融统计数据报告  articles/2025092212552571879.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [4月份/人民币 4月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-05  金融统计数据报告  articles/2025092212552655029.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [5月份/人民币 5月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-07  金融统计数据报告  articles/2025092212552757258.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [7月份/人民币 7月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-08  金融统计数据报告  articles/2025092212552776983.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [8月份/人民币 8月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-10  金融统计数据报告  articles/2025092212552824305.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [10月份/人民币 10月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2022-11  金融统计数据报告  articles/2025092212552893388.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 1 candidate sentence(s) [11月份/人民币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2023-02  金融统计数据报告  articles/2025092212553043320.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [2月份/人民币 2月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2023-04  金融统计数据报告  articles/2025092212553299625.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: 人民币存款期内合计 not found among 2 candidate sentence(s) [4月份/人民币 4月份/外币]: refusing to fall back to a neighbouring one, it has the right magnitude and format but the wrong meaning
  2023-05  金融统计数据报告  articles/2025092212553223669.html  本迭代不解析：该期报告只有当月数、正文无任何期内累计口径的合计句，*_ytd 字段无源可抽（不是解析器不支持，LLM 兜底也变不出不存在的数）；原始解析错误：hestia: section ordinals are not consecutive from 一: got 4 sections, section[0] is "二、" but should start with "一、". A section title that fails to anchor at line start is dropped silently, and a rule@v2 report missing exactly its two 社融 sections becomes indistinguishable from a valid rule@v1 one — so this is refused rather than guessed. Common cause: leading whitespace before the ordinal (stripHTML folds runs of spaces but does not remove them at line start)
```
