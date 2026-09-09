# M3 第二轮 Code Review（跨视角对抗）

- **审查者**：qa-m3
- **时间**：2026-09-09
- **判定对象**：atlas master `41a19d8739092b72f17ee390284f6c33129fd27c` · nanoclaw `feat/warp-hestia` `2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8` · loom PR #13 head `6d6c38f96901dc60f516ef2154283a213782ad5e`
- **前置**：第一轮结论见 `code-review-round1-2026-09-09.md`（REJECT，1 CRITICAL / 3 HIGH / 2 MEDIUM / 2 LOW / 2 INFO）。本轮**不重复**第一轮的 finding。

## 🔴 结论：**REJECT**（与第一轮同向，本轮新增一条 HIGH 与一条会导致永久卡死的 MEDIUM）

第二轮的核心结论不是「又找到几个 bug」，而是**这道闸守的位置**：

> `verify.py` 保护的是**没有人会篡改的部分**（机器渲染的两张表），
> 不保护**承载结论的部分**（frontmatter、叙述段），
> 也不保护**喂给它的输入是不是对的**（契约与侧车的配对）。

要让这套 skill 产出一份被篡改而不被发现的笔记，**根本不需要攻击 `verify.py`**。

---

## 0. 本轮的方法与一处必须交代的缺口

**变更规模判定 Large**（跨三仓库、7349 行新增），按角色定义开三个 lens：Skeptic / Architect / Minimalist，各自独立 context、只读、禁写 `.arcforge/`。

🔴 **三个 lens 的最终回复都被 idle hook 顶掉了**（这是本 sprint 已实测三次的已知形态）：三者分别跑了 32 / 36 / 35 次工具调用、177K / 211K / 197K token，做了实事，但返回给我的只剩 hook 应答文本，**发现清单没有回传**。我已分别要求原文重发，并已报 Leader 请求直发消息。

⇒ **本报告的每一条 finding 都是我（qa-m3 本体）自己实测的**，不含任何未回收的 lens 结论。这不是「三个 lens 白跑了」的等价物——**独立 context 的交叉验证这一层缺失**，本报告的对抗强度因此低于原定标准，与 §D 第六条那一格同类，此处留痕。

**唯一回收到的 lens 信号**：Architect 的 hook 应答里带了一句摘要——「14 条发现，首要项为 `SKILL.md` 的 `--existing` 无条件传入缺陷」。**我给三个 lens 的已知项清单里没有这一条**，所以这是一次独立命中，与第一轮 C1/C2 同源。**一条摘要不能替代那 14 条**，但它把 C1 的可信度从「我一个人测出来」提到了「两个独立 context 各自撞到」。

**证据纪律**：本轮每个篡改/错配实验都先做「变异是否真的生效」自检（与基线逐字节相同即判为不构成证据）——第一轮做这项自检时抓到过一次假阳（替换串多一个空格导致变异未生效，输出却是一个看起来正常的 `exit 0`）。

---

## 1. 从「我要让它产出一份被篡改而不被发现的笔记」出发

### 🔴 A1 [HIGH] 契约与侧车**配对无校验**，而校验所需的字段就在侧车里、产出方专门写了、消费侧从不读

**位置**：nanoclaw `prepare.py:main()` / `build_note()` × atlas `internal/hestia/history.go:70`（`For` 字段）

侧车顶层有一个 `for` 字段，值形如 `"2026-06-h1"`，就是「这份侧车属于哪一期」。`prepare.py` **从不读它**——我用精确判据核过：从 `history` 对象上读到的键**只有 `same_type` 一个**（唯一入口 `_series()`，被调用 3 次）。

**实测（错配一对喂进去）**：

```
python3 prepare.py fixtures/2026-06-h1.json fixtures/2025-12-annual.history.json
  prepare 退出码=0     verify 退出码=0
产出的笔记：period: 2026-06 / period_type: h1 / 标题「# 2026 年上半年金融统计数据解读」
「前 12 期」表里的期次：2024-12  2022-12  2021-12  2020-12  2019-12   ← 全是 annual
侧车自报 for = 2025-12-annual ；契约自报期次 = 2026-06-h1
```

**更隐蔽的一档（同 `period_type`，肉眼可信）**：

```
python3 prepare.py fixtures/2023-08-monthly.json fixtures/2022-07-monthly.history.json
  prepare=0  verify=0     与正确配对的产出相差 40 行
正确配对表头：| 本期 2023-08（口径 2023-01） | 上期 2023-07 | 去年同期 2022-08 ⚠️口径 2015-01 |
错配后表头：  | 本期 2023-08（口径 2023-01） | 上期 2022-06 ⚠️口径 2015-01 | 去年同期（无） |
```

**危害**：
- 错配污染的**不只是「前 12 期」表**——`render_table_current` 的「上期 / 去年同期」两列同样取自侧车的 `same_type`（`series[0]` 即「上期」）。⇒ **本期数据表的对比列也是错的**，而那正是解读的起点。
- frontmatter、标题、四信号、温度全部来自**契约**，所以它们是对的 ⇒ 笔记**自洽地看起来完全正常**，`verify.py` exit 0，Spool 照写。
- 它把 glossary 的第一条禁忌（跨口径/跨期不可直接比）从「模型要遵守的纪律」变成了「表本身就是错的」——模型再守纪律也救不回来。

**它怎么会发生**（不是纯理论）：
1. SKILL.md Step 2 的分支「**同名已在 `processing/` ⇒ 直接用 `processing/` 里那对，不重新占位**」——上一轮中断留下的残留对没有任何一致性检查。
2. 侧车缺失时 SKILL.md 让人跑 `atlas hestia contract emit --period …` **手工补发**；手工正是错配发生的地方。
3. 第一轮 C1 使 Step 3 恒定失败 ⇒ **运维必然会手工跑这些命令**，把 (2) 的概率从「偶尔」变成「常态」。

**建议**（一行，且不需要新规格）：`build_note()` 开头断言
`history.get("for") == "%s-%s" % (contract["period"], contract["period_type"])`，不符 ⇒ stderr 写明两边的值、`return 2`（与其他输入不可用同码）。`for` 字段存在的**唯一理由**就是这个，现在它是一个纯装饰。

### 🟠 A2 [MEDIUM，但后果是永久卡死] 批注区提取用「首次出现 + 子串」匹配，模型写下 `## 我的批注` 会让该期永久卡在 `processing/`

**位置**：`prepare.py:extract_annotations()` × `SKILL.md` Step 5 的失败处置

```python
if not existing_md or ANNOTATION_HEADING not in existing_md:   # 子串判断
    return ""
return existing_md.split(ANNOTATION_HEADING, 1)[1].lstrip("\n")  # 首次出现
```

`note-format.md` **明确允许**模型在叙述里写 `## ` 标题（「narrative 段被扣除，所以模型在叙述里写 `## ` 标题、写表格……都不会影响封条」）。于是：

**实测三轮**：

```
R1  正常生成                                   verify=0
R1' 在 narrative 段里写下「## 我的批注」这一行     verify=0     ← 完全合规，闸不管叙述
R2  把 R1' 当旧笔记做 update                    verify=1
    「封条 seal 行应为 1 条，实得 2 条 —— 判为被篡改」
    R2 里 check=2  seal=2  begin=1  字节 2381 → 2619（逐轮累积）
    批注区首 120 字：'（模型在叙述里提到了这个标题）… <!-- /narrative --> <!--…'
                      ↑ 机器区尾部连同真 seal 行被整段吞进了批注区
```

**为什么后果是永久卡死而不是一次失败**：SKILL.md Step 5 对 `verify.py` 非零的处置是「🔴 **留在 `processing/`**，不移 `failed/`……下一次会话按 Step 2 自然重试」。这条处置本身是对的（数据没毛病，是叙述的问题）——但**这里的毒源不在队列里，在 vault 的旧笔记里**。重试会读同一份旧笔记、再吞一次、再产出 2 条 seal。⇒ **该期既产不出笔记，也永远不进 `failed/`，无声地占着 `processing/`。**

**这是三个各自合理的决定合成出来的**：(a) 叙述里可以写 `## ` 标题；(b) 批注按首次出现的子串切；(c) verify 失败留在 processing 重试。单看每一条都对。

**建议**：`extract_annotations` 改为**按行匹配 + 取最后一次出现**（批注区在文件末尾，语义上就该取最后一个整行 `## 我的批注`）；并在 SKILL.md Step 5 补一句「**同一期连续两次 verify 失败 ⇒ 移 `failed/`**」，给这条重试环一个出口。

### 🟡 A3 [MEDIUM] 真正的篡改面是叙述段，而规格**要求**它承载大量数字、没有任何东西核对这些数字

这不是缺陷，是**这套设计的性质**，但它没有被登记，所以写在这里。

- `methodology.md` 写作纪律：「**每个判断句后面括注你引用的字段与数值**……没有数字支撑的判断句一律删掉——**这是本 skill 与「聊宏观」的唯一区别**」。
- ⇒ 一份合格的笔记，其叙述段**按设计**含有大量模型敲进去的数字。
- ⇒ 而叙述段是封条**明确扣除**的区域，`verify.py` 对它一个字都不管（实测：叙述里写「本期 M2 同比 99」，exit 0）。
- ⇒ `note-format.md` 还明确祝福「模型在叙述里……**写表格**」⇒ 一张伪造的数据表可以合法地出现在叙述段里，渲染出来与机器区那两张表**视觉上无从分辨**。

**量化这道闸实际管多大**（新生成的笔记，`2025-12-annual-rev` 夹具）：

| 区段 | 字节 | 占比 |
|---|---|---|
| **check + seal 实际覆盖** | **1204** | **48.1%** |
| BEGIN 之前（frontmatter + 标题 + 修订行） | 758 | 30.3% |
| narrative 段（被封条扣除） | 410 | 16.4% |
| seal 行 + END 之后（批注区） | 133 | 5.3% |
| 合计 | 2505 | 100%（四部分求和 == 总字节，自洽校验通过） |

🔴 **48.1% 是上界，不是典型值**：这是模型**还没开始写**的状态，narrative 段里只有 `prepare.py` 生成的判读提示。一份真笔记的叙述段会长几倍，**覆盖率只会更低**。

**建议**：不要求本 sprint 改实现。但 §D 第六条那一条交人拍板的登记里应当出现这个比例——「这道闸覆盖笔记的不到一半，且模型写得越多覆盖率越低」比「frontmatter 不在作用域内」更接近人要拍的那块板。若将来要收紧，**代价最低的一档不是把封条上移到 frontmatter**，而是「把叙述段里出现的每个数字与契约 `data` / 两张表比对，对不上就列出来」——那正好卡在「模型不自己算」这条既有纪律上。

### 🟢 A4 [LOW] `verify.py` 对「BEGIN 不成整行」抛未捕获异常，把格式问题误报成篡改

实测：把 `<!-- machine-generated: begin -->` 行首加一个空格 ⇒

```
ValueError: list.index(x): x not in list      退出码=1
```

`prepare.BEGIN not in md` 这道前置检查是**子串**判断（过），而 `bare.index(prepare.BEGIN)` 要求**整行相等**（抛）。docstring 明写「退出码 2 = 输入不可用 / 不是笔记」，而这里给的是 1（= 被篡改）+ 一段 traceback，而 Step 5 会把它「原文回复用户」。

**建议**：`begin_i`/`end_i` 用 try/except 包起来，落到与「缺少机器区标记」同一条 `return 2` 上。

### ℹ️ A5 [INFO] 其余 fail-closed 行为经实测全部正确

| 篡改 | 结果 |
|---|---|
| 改机器区 `## 信号` 段 | exit 1（封条） |
| 改数据表里的数字 | exit 1（分段 check） |
| 删一条 check 行 / 删整段（标题+表+check） | exit 1（条数写死） |
| 机器区首插一行**缩进的**伪 seal 行 | exit 1（封条不匹配） |
| 把 `## 信号` 段搬进 narrative 并全改绿 | exit 1（封条） |
| 叙述里行首写合法 hex 的 `<!-- check: … -->` | exit 1（3 条 ≠ 2 条） |
| 叙述里行首写 `<!-- seal: 举例 -->` / `<!-- check: 说明 -->` | exit 1（格式损坏，**不当成「这里没有校验行」**） |
| 叙述里写整行 `<!-- machine-generated: end -->` | exit 1（校验行落在机器区之外） |

「条数写死为 2、不从文档结构推」这个决定在**删除类篡改**上确实是有效的；`scan_markers` 把「格式损坏」与「没有校验行」分开也是对的。**这一层没有 finding。**

---

## 2. Architect 视角（跨三仓库的接口与耦合）

### 🟡 B1 [MEDIUM] 侧车 5 个顶层键里 **4 个零消费**；契约 17 个顶层键里 5 个零消费

判据：源码里以 `["k"]` / `.get("k")` 形式出现，且区分是从 `contract` 还是从 `history` 对象上读（`history` 的唯一入口是 `_series()`）。

| 侧车键 | 消费者 |
|---|---|
| `same_type` | ✅ `_series()` |
| `schema_version` / `for` / `generated_by` / `monthly_recent` | 🔴 **零** |

| 契约零消费键 | 说明 |
|---|---|
| `schema_version` | 版本协商机制**有字段、无实现**——两侧都产出，消费侧从不检查 |
| `extracted_at` | ⚠️ 这正是 M2a 的 QA C1 返工专门修的字段（从库回读，不再是空串），M3 消费者不读它 |
| `absent_fields` | 契约明说了哪些字段缺失，笔记里不体现（表里只打 `—`） |
| `units` | 单位在 `prepare.py:UNITS` 里**另写了一份**（见 B2） |
| `validation` | `passed` 恒真（没过闸的期次不产契约）故无信息；但 `checks` 逐条结论对**人工复核**有用，从不进笔记 |

全 skill 目录 grep（`--include='*.py' --include='*.md'`）：`extracted_at` / `units` / `absent_fields` / `validation` / `schema_version` **命中文件数均为 0**。

**建议**：`schema_version` 至少加一条断言（不认识的大版本 ⇒ exit 2），这是跨仓库唯一能自动发现「上游换了形状」的机制；其余几个在 §D 记一句「本迭代不消费」即可，不必删。

### 🟡 B2 [MEDIUM] 同一事实在三仓库各存一份副本，没有任何机制让它们一起变红

| 事实 | 副本所在 |
|---|---|
| 单位映射 `balance/flow/ratio → 万亿元/亿元/百分数` | atlas `contract.go:BuildContract` 的 `Units` map **×** nanoclaw `prepare.py:UNITS` |
| 「分段 check 恰 2 条、封条恰 1 条」 | `note-format.md` **×** `prepare.py` docstring **×** `verify.py:EXPECT_CHECKS/EXPECT_SEALS` |
| 期内月数 3/6/9/12 与月均取法 | atlas `signals.go:Evaluate` **×** `prepare.py:MONTHS/monthly_average` **×** `glossary.md` 三、 |
| 四信号判定（含「楼市/消费无黄灯」） | atlas `signals.go` **×** `prepare.py:_eval_*` **×** `glossary.md` |

**已有的缓解**：三期 golden 温度 `2 / 0 / 1` **两侧都钉住**（CONTRACTS §A4 / §B），这确实能在四信号口径漂移时报警——**这是本 sprint 做得对的一处**，值得点名。

**没被缓解的**：单位映射与 check 条数没有任何跨仓库对照。改 atlas 的 `Units` 不会让 nanoclaw 变红（`units` 键零消费，见 B1），反之亦然 ⇒ **两边可以静默地对同一字段说不同的单位**。

**建议**：让 `prepare.py` 读契约的 `units` 而不是自带一份 —— 这一条同时消掉 B1 的一格与 B2 的一行，改动很小。

### 🟢 B3 [LOW] 队列里契约与侧车靠**文件名后缀**区分，而生产方不知道这件事

atlas 把 `<p>-<pt>.json` 与 `<p>-<pt>.history.json` 写进同一个 `pending/`；SKILL.md Step 1 靠 `grep -v '\.history\.json$'` 把侧车滤掉。这能工作，但契约命名规则若哪天加了后缀（或出现 `*.history.json` 之外的第二种侧车），过滤式会静默失效。另：若 `WriteHistory` 成功而 `WriteContract` 失败，`pending/` 里会留一个**孤儿侧车**，Step 1 的过滤让它永远不可见、也无人清理（重新 emit 会同名覆盖，不会累积，故只是 LOW）。

### ℹ️ B4 [INFO] 两条生产路径的同形性、失败原子性——独立复核通过

我在**隔离 worktree** 上对 atlas 侧做了三个变异（基线全绿；每个变异体打了 diff；锚点命中 0 次即 FATAL 不静默跳过；收尾核对主工作区两个文件的 sha256 与 `git status --porcelain` 均未变；树已拆）：

| 变异 | 结果 |
|---|---|
| M1 侧车写失败不再阻断契约（`return fail` → 忽略） | **KILLED**，3 处 FAIL |
| M2 改成先契约后侧车 | **KILLED**，3 处 FAIL |
| M3 去掉 `cmd/atlas` 的 `!hestiaEmitStdout` 副作用保护 | **KILLED**，3 处 FAIL |

⇒ CONTRACTS §A2 声称的「该顺序**两个方向都有闸**」**经独立复核成立**。三条守卫测试的符号名也逐一核对存在。**这是本 sprint 唯一一处「QA 返工补的断言」，形状正确、确实在守。**

### ℹ️ B5 [INFO] 夹具是 atlas 真产的，不是手搓的

`fixtures/*.json` 由 `hestia contract emit` 针对真语料库产出（`generated_by: contract@v1/replay` 可证），修订夹具走的是「造一条 Revision 观测再 emit」的路径（TASK-006 记录 §2.2，并**实测证伪了**「218 篇语料里有现成修订期次」这个假定：76 观测 / 76 期次 / 同期两版 **0** 个）。⇒ 跨仓库的 JSON 形状**有一个真实锚点**，比我预期的强。

⚠️ 但那是**一次性快照**：夹具冻结之后，atlas 改键名不会让 nanoclaw 的 84 条测试变红（它们比对的是冻结的夹具）。这就是 B1 建议给 `schema_version` 加断言的理由。

---

## 3. Minimalist 视角

### 🟡 C1 [MEDIUM] 侧车约 2/3 的字节没有读者（与第一轮 M1 同源，此处补体量）

`fixtures/2025-12-annual.history.json` 共 1053 行，`monthly_recent` 自第 **346** 行起 ⇒ **约 67% 的侧车字节零消费**。第一轮 M1 已记「理由引了一条不存在的方法论条目」，这里补的是代价的量级。

### 🟢 C2 [LOW] `render_table_history(history, ptype=None)` 的 `ptype` 是死参数

函数 docstring 自己写「`ptype` 仅为兼容保留，不参与取数」，而调用点 `build_note` 仍在传 `ptype`。**没有任何「兼容」对象**（本函数只有这一个调用点，且是同一次交付新建的）。⇒ 删掉参数与实参各一处。

### 🟢 C3 [LOW] 两份高度相似的期次后缀映射

`NAME_SUFFIX`（`{q1:"一季度", h1:"上半年", …}`，给文件名用）与 `TITLE_SUFFIX`（`{monthly:"金融统计数据解读", q1:"一季度金融统计数据解读", …}`，给一级标题用）。后者的四项就是前者四项 + 固定后缀。**保留是合理的**（文件名叫「金融数据解读」、标题叫「金融统计数据解读」，两个串确实不同），但**这一点没写在任何地方**，下一个人很可能把它们合并然后弄错其中一个。⇒ 建议加一行注释说明「两处的名词刻意不同」，不建议合并。

### ℹ️ C4 [INFO] 我特意去查而**没有**找到的东西

- `verify.py` 从 `prepare` 导入 `seal_digest` / `with_check` 而不是自己实现——**这是对的**，且理由写在 docstring 里（两份定义必然漂移，两个漂移方向都致命）。这一处是本次审查里最不需要改的设计决定。
- `prepare.py:months_in_period` 对 `00`/`13`/长度不对/未知 `period_type` 一律返回 0 且**明确不拿它做除数**（÷0 得 ±Inf 会被阈值判定当成「很大」而误判绿灯）——理由与实现一致，有测试。
- `evaluate()` 的温度分母用 `temp_known` 而不是恒定的 4，判读提示里也点了这一句——**这是全交付里我最认可的一处细节**。
- 我逐个核了 `EMOJI` 四态、`_pick` 的口径标注、`same_caliber_pair` 的「不许跨口径配对」，**都有实际调用点，没有为不会发生的情况写的兜底**。

---

## 4. 合并判定与 fix_items 增量

本轮**不新增 CRITICAL**。与第一轮合并后的 fix_items 增量：

| # | 级别 | 任务 | 动作 |
|---|---|---|---|
| 8 | **HIGH** | TASK-006（`prepare.py` 归它） | 加**契约↔侧车配对断言**（`history["for"]` 比对），不符 ⇒ exit 2；补一条测试 |
| 9 | MEDIUM | TASK-006 + TASK-005 | `extract_annotations` 改按行匹配 + 取最后一次；SKILL.md Step 5 补「连续两次失败 ⇒ 移 `failed/`」的出口 |
| 10 | MEDIUM | TASK-008 | §D 第六条补「闸实际覆盖 48.1%（上界）」这个量级，并点明**叙述段按设计承载大量模型敲进去的数字、无人核对** |
| 11 | MEDIUM | TASK-006 | `prepare.py` 读契约的 `units` 而不是自带一份；`schema_version` 加大版本断言 |
| 12 | LOW | TASK-007 | `verify.py` 的 `bare.index` 包 try/except，落到 `return 2` |
| 13 | LOW | TASK-006 | 删死参数 `ptype`；给 `NAME_SUFFIX`/`TITLE_SUFFIX` 补一行「刻意不同」的注释 |

`reason_class` 建议：**`task_defect`**。

## 5. 本轮自身的证据强度声明

- 每一条 finding 都由我实测复现，命令与输出在正文里；**没有一条是纯推理**。
- **缺的那一层**：三个 lens 的独立清单未回收（§0），跨 context 的交叉验证没有发生。⇒ **本报告的对抗强度低于原定标准**。若 lens 清单事后回收到，应当**补一节**而不是改写本节——两次审查的证据强度不同，不该混在一起读。
- 我用过的隔离 worktree：atlas 侧 `wt-atlas-mut` **已拆**；nanoclaw 侧 `wt-nc-qa-m3` 收尾时由我自己拆。前三任验证者残留的 `wt-nc-t7-test-m3-b` / `-c` 我未触碰，留待 Leader 在阶段边界收。

---

# 6. 订正：本报告对第一轮 C1 的引用需要收窄（2026-09-09，追加）

第一轮 C1 的**机制**已被 Leader 的区分性实验证伪、并经我跨 shell 复核确认（详见
`code-review-round1-2026-09-09.md` §7）。要点：`${var:+word}` 的引号在 bash / sh / dash 下**都生效**，
不发生词分割；我第一轮的观察是真的，但成因相反——本会话的 Bash 工具跑 **zsh**，zsh **不拆**而是把
`--existing` 与路径**粘成一个 argv**。运行环境（容器 `node:22-slim`，`entrypoint.sh` 是 `#!/bin/bash`）
是 bash ⇒ **只有 create 场景失败，update 正常**。

**波及本报告的一处**：§1-A1「它怎么会发生」的第 3 点原文写「第一轮 C1 使 Step 3 **恒定失败** ⇒
运维必然会手工跑这些命令」。**该句需收窄为**：

> Step 3 在 **create 场景**失败（每个 `period_type` 的**首次**生成，以及冒烟场景），
> 运维会在**这些场次**手工跑 `contract emit` 与 `prepare.py`——错配正是在手工调用里发生的。

**A1 本身不受影响**：配对无校验是 `prepare.py` 自身的性质，与 Step 3 能不能跑无关；我对 A1 的两组
错配实验都是**直接调 `prepare.py`**、不经 Step 3 的。Leader 亦已独立复核 A1 成立（其实测 3161 字节，
与我这份夹具组合的字节数不同是因为 `--now` 取值不同，不影响结论）。

**A2 / A3 / A4 / B / C 各条不受影响**——它们都不依赖 Step 3 的可执行性。

**本报告 §4 fix_items 第 8–13 条不变。verdict 不变：REJECT。**
