# M3 第一轮 Code Review（常规）

- **审查者**：qa-m3
- **时间**：2026-09-09
- **判定对象**：atlas master `41a19d8739092b72f17ee390284f6c33129fd27c` · nanoclaw `feat/warp-hestia` `2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8`（PR #5 OPEN）· loom PR #13 head `6d6c38f96901dc60f516ef2154283a213782ad5e`（OPEN）
- **取证工作树**：nanoclaw 侧自建只读 worktree `scratchpad/wt-nc-qa-m3`（detached @ `2d2fbe8094…`），未触碰前三任验证者残留的两个 worktree。
- **采样纪律**：`git status --porcelain internal/hestia cmd/atlas` = **0 行**（采数前核空）；本报告的每个数字都在同一锚上采，未跨时间点拼接。

> ⚠️ **2026-09-09 订正（见文末 §7）**：C1 的**结论与等级不变**（CRITICAL 成立），
> 但**机制被 Leader 的区分性实验证伪**，波及场景也窄一档。
> **§1 的 C1 / C2 两节原文一字未改**（两次判断的证据不同，不该混着读）；
> **依据本报告下发修法之前，请先读 §7**——按 C1 原文去「修引号」是无谓改动。

## 🔴 结论：**REJECT**

一条 CRITICAL：**skill 的主执行路径（SKILL.md Step 3 的组装命令）在 create 与 update 两种场景下都恒定失败**，实测复现，且按 SKILL.md 自己的失败分支处置，每一份契约都会被移进 `failed/`。⇒ 这套 skill 按交付原文执行**一份笔记也产不出来**。

修复面很小（一行 shell + 一个存在性判断），但它落在「本 sprint 唯一一条跨任务集成指令」上，而该指令**结构上不可能被任何一个任务的 DoD 覆盖**（见 §4.1）——这才是要返工的理由，不是那一行本身。

---

## 1. Finding 清单

### 🔴 C1 [CRITICAL] SKILL.md Step 3 的组装命令恒定失败（词分割）

**位置**：nanoclaw `container/skills/warp-hestia/SKILL.md` §2 Step 3 代码块第 3 行

```bash
python3 /app/skills/warp-hestia/scripts/prepare.py $Q/processing/$F $Q/processing/$H ${EXISTING:+--existing "$EXISTING"} > /tmp/note.md
```

**缺陷**：`${var:+word}` 里的双引号**不保护展开结果**——`word` 展开后仍要过一次词分割。而笔记文件名**五种 `period_type` 全部含空格**（`note_name()`：monthly 是 `"%s 金融数据解读.md"`，其余四种是 `"%s %s金融数据解读.md"`）⇒ 路径被拆成两个 argv，argparse 判为多余位置参数。

**实测复现**（`scratchpad/wt-nc-qa-m3/container/skills/warp-hestia/scripts`）：

```
# create 场景（vault 里没有同名笔记）
N=$(python3 prepare.py --print-name fixtures/2025-12-annual.json)   # → "2025 全年金融数据解读.md"
EXISTING=/nonexistent/vault/Wiki/Macro/PBOC/$N
python3 prepare.py fixtures/2025-12-annual.json fixtures/2025-12-annual.history.json ${EXISTING:+--existing "$EXISTING"} > note.md
  prepare.py: error: unrecognized arguments: --existing /nonexistent/.../2025 全年金融数据解读.md
  退出码=2  产出字节=0

# update 场景（旧笔记真实存在，用夹具 existing-2026-06-h1.md 铺进临时 vault）
  同一形态失败，退出码=2  产出字节=0

# 对照组（加引号 + 存在性判断）
[ -f "$EXISTING" ] && python3 prepare.py fixtures/2026-06-h1.json fixtures/2026-06-h1.history.json --existing "$EXISTING" > note-C.md
  退出码=0  产出字节=3263 ；python3 verify.py note-C.md → 退出码=0
```

**危害**：Step 3 明写「`prepare.py` 退出码非零 ⇒ stderr 原文回复用户，契约与侧车对移 `failed/`，结束」。⇒ **每一期都进 `failed/`，永远产不出笔记**，而现象是「prepare.py 报了一个参数错误」，排查方向会被引向 prepare.py 而不是这一行。

**建议**：`--existing "$EXISTING"` 直接写在条件分支里，不要用 `${:+}` 传参：

```bash
N=$(python3 /app/skills/warp-hestia/scripts/prepare.py --print-name "$Q/processing/$F")
EXISTING="/workspace/extra/vault/Wiki/Macro/PBOC/$N"
if [ -f "$EXISTING" ]; then
  python3 /app/skills/warp-hestia/scripts/prepare.py "$Q/processing/$F" "$Q/processing/$H" --existing "$EXISTING" > /tmp/note.md
else
  python3 /app/skills/warp-hestia/scripts/prepare.py "$Q/processing/$F" "$Q/processing/$H" > /tmp/note.md
fi
```

（`$Q/processing/$F` 等也一并加引号：`$F` 当前不含空格，但那是**契约命名规则的巧合**，不是这一行的性质。）

---

### 🔴 C2 [HIGH] 同一行缺存在性判断：create 场景**修好 C1 后仍然失败**

**位置**：同上，`EXISTING=/workspace/extra/vault/Wiki/Macro/PBOC/$N` 这一行。

**缺陷**：`EXISTING` 被赋成一个**字面路径**，恒非空 ⇒ `${EXISTING:+…}` 恒展开 ⇒ **create 场景也会传 `--existing`**，而 `prepare.py` 对读不到的 `--existing` 返回 2。`${:+}` 这个写法本身就说明作者的意图是「文件不存在时 EXISTING 为空」，缺的正是那个 `[ -f ]`。

**实测复现**（只修引号、不加存在性判断）：

```
--existing 读取失败: [Errno 2] No such file or directory: '…/2025 全年金融数据解读.md'
退出码=2  产出字节=0
```

**为什么单列而不并进 C1**：C1 **掩盖**了 C2——只修引号会让人以为修好了，而 create（即**第一次给某个 period_type 写笔记**，冒烟场景恰是它）仍然全灭。两个都要修。

**旁证**：`references/note-format.md` 的「`mode` 规则」写的是「**没有同名文件 ⇒ `create`**」，与 SKILL.md 这一行**不自洽**。规格那边是对的，是 SKILL.md 没接上——与 TASK-005 交付记录 ⑤节 ① 修的是**同一处、同一类**问题（那次修的是 `$N` 的取法），这次是 `--existing` 的取法。

---

### 🟠 H1 [HIGH] 八问框架有 5 问所需的字段不在笔记里，而指令要求「引用表里的数字」

**位置**：`references/methodology.md`「八问框架」 × `SKILL.md` §2 Step 4 × `scripts/prepare.py:reading_hints()`

**实测**：

| 量 | 值 | 取法 |
|---|---|---|
| 契约 `data` 有值字段 | **54** | `2026-06-h1.json`，`len(data)` |
| 契约 `absent_fields` | 22 | 同上，合计 76 |
| 笔记本期表渲染的指标 | **7** | `prepare.py:ROWS` |
| 笔记前 12 期表的数据列 | **4** | `render_table_history` |
| golden 笔记正文出现的字段名 | **3**（`m1_yoy`/`m2_yoy`/`tsf_stock_yoy`） | `grep -oE 'loan_[a-z_]+\|deposit_[a-z_]+\|rate_[a-z_]+\|tsf_[a-z_]+\|m[012]_yoy\|fx_[a-z_]+' fixtures/golden/2026-06-h1.md \| sort -u` |

八问逐问核对（✗ = 该问要看的字段**一个都不在两张表里**，但**都在契约里且有值**）：

| 问 | 要看的字段 | 在表里？ |
|---|---|---|
| 一 经济扩张还是收缩 | `deposit_household_*` vs `loan_hh_*` | ✗（存款段整段不在表里） |
| 二 房地产 | `loan_hh_mlt_*` | ✓ |
| 三 消费 | `loan_hh_short_*` ✓ + `m0_yoy` ✗ | 部分 |
| 四 企业扩张还是维持 | `loan_corp_mlt_*` / `loan_corp_short_*` / `tsf_flow_govt_bond_ytd` | ✗ |
| 五 贷款有没有水分 | `loan_bill_*` / `loan_corp_total_*` | ✓ |
| 六 钱去哪儿了 | `deposit_nbfi_*` / `loan_nbfi_*` / `rate_ibo` | ✗ |
| 七 谁在加杠杆 | 四部门存贷 + `tsf_stock_govt_bond` | ✗ |
| 八 钱贵不贵 | `rate_ibo` / `rate_repo` | ✗ |

**指令面的矛盾**：三处说法不一致——
- `SKILL.md` Step 4：「按八问框架写，**每一问引用数据表里的数字**」
- `prepare.py:reading_hints()`：「每一问引用**上面表里的**数字，不自己算」
- `methodology.md` 写作纪律：「不引入**契约与侧车之外**的数据」（契约在范围内）+「**每个判断句后面括注你引用的字段与数值**，没有数字支撑的判断句一律删掉」

⇒ 一个**严格照 Step 4 执行**的模型，对五问无数可引；而写作纪律又要求每句都带数字。三条约束叠加，**留给模型的出路只剩「跳过五问」或「编一个数」**，而 narrative 段不受 `verify.py` 覆盖、没有任何下游检查看得住编造。

**这不是理论推演**：`prepare.py` 明确拒绝把这 47 个字段渲染进表（`ROWS` 只有 7 项，是规格定的），而 `SKILL.md` 全篇没有一句让模型去 `cat` 那份契约 JSON。

**建议**（最小改动，不动 `ROWS`、不动规格）：Step 4 加一句——「表里没有的字段（存款四部门、企业贷款期限拆分、非银、利率、政府债、外储）**去读 `$Q/processing/$F` 这份契约 JSON 的 `data` 段**；契约与侧车之外的数据一律不引」。同时把 `reading_hints()` 里「上面表里的数字」改成「契约与侧车里的数字」，让三处口径一致。

---

### 🟠 H2 [HIGH] CONTRACTS §D 第六条低估了未受保护面 —— 人要拍的板比登记的大

**位置**：`internal/hestia/CONTRACTS.md:3647-3655`

登记原文把缺口界定为「**笔记 frontmatter** 不在任何校验作用域内」，并按「14 个基础字段 + 9 个派生指标 + 4 个信号」估算面。**实测未受保护面比这更大**——我用 `2025-12-annual-rev` 夹具生成基线笔记（`verify.py` exit 0），逐项做单点篡改后重跑：

| # | 篡改 | `verify.py` |
|---|---|---|
| 00 | 未改动基线 | **exit 0**（对照） |
| 01 | frontmatter 改 `tsf_stock_yoy` | exit 0 ←（已登记） |
| 02 | 一级标题 `# 2025 年全年…` 改成 `# 2024 年…` | **exit 0** ←**未登记** |
| 03 | **删掉** `> 本文取代 2026-01-15 版本。` | **exit 0** ←**未登记** |
| 04 | 把该行日期改成 `2019-01-15` | **exit 0** ←**未登记** |
| 05 | 机器区 `end` 之后追加一张伪造的 `## 本期数据（更正）` 表 | **exit 0** ←**未登记** |
| 06 | 机器区 `end` 之后插入「## 编辑说明：本期数据经人工更正，M2 同比实为 99」 | **exit 0** ←**未登记** |
| 07 | 改机器区 `## 信号` 段标题 | exit 1（封条拦下） |
| 08 | 改本期数据表里的数字 | exit 1（分段 check 拦下） |
| 09 | 删掉一条 check 行 | exit 1（条数写死拦下） |
| 10 | 删掉整段（标题 + 表 + check） | exit 1 |
| 11 | 机器区首插一行**缩进的**伪 seal 行 | exit 1（封条不匹配） |
| 12 | narrative 段内写任意内容 | exit 0（设计如此） |
| 14 | 把 `## 信号` 段搬进 narrative 并全改绿 | exit 1（封条拦下） |

（第 09/10/11/14 条我先做了「变异是否真的生效」自检——第一次写的 09 因替换串多了一个空格而**未生效**，输出却是一个看起来正常的 `exit 0`；重做后才是上表的值。凡与基线逐字节相同的变异一律不计入证据。）

**为什么这条要改**：§D 第六条是**明确要交人拍板的一条**，而拍板的人会照着登记的范围估代价。实际未受保护的还包括：
- **一级标题**——笔记最显眼的一行，改年份/期次类型无人拦；
- **`> 本文取代 X 版本。`**——`note-format.md`「修订」节白纸黑字写「这一行由 `prepare.py` 生成，**模型不写、不删**」，这是一条**有禁令、零守卫**的规则，而它承载的是「这份笔记取代了哪一版」这个**修订披露**；
- **`<!-- machine-generated: end -->` 之后的全部内容**（批注区）——可以整张伪造数据表贴进去，渲染出来与机器区的表**在视觉上无从分辨**。

**建议**：§D 第六条的标题与「实测行为」两段按上表重写，把「frontmatter」换成「**机器区之外 + narrative 之内的全部文本**」，并单独点名「修订披露行有禁令无守卫」。**不要求本 sprint 改实现**（规格取舍仍归人），但**要求登记如实**——人是照这份登记做决定的。

---

### 🟡 M1 [MEDIUM] `monthly_recent` 在交付管道里零消费者，而它的理由引了一条不存在的方法论条目

**位置**：`internal/hestia/history.go:62-66`（`BuildHistory` 的 doc 注释）× `history.go:20-38`（`omitzero` 那段）× nanoclaw `prepare.py:_series()`

`history.go:62-66` 写：

> 非 monthly 再附最近 12 个 monthly（半年报的「前 12 期」是 12 年，**方法论里的「连续第 N 个月」靠后者**）。

**实测**：
- 全 skill 目录 grep「连续第」= **0 命中**；`methodology.md` 没有任何「连续第 N 个月」的判读项。
- `prepare.py:_series()` 注释明写「🔴 **一律取 `same_type`**……`monthly_recent` 只是非 monthly 期次的近月补充上下文，**不是本表的数据源**」，且 `render_table_current` / `render_table_history` **都只调 `_series()`**。
- 跨仓库 grep `monthly_recent` 的消费点：只有 `test_prepare.py:86/393-396` 两处**断言它存在/为空**，没有任何一处读它的内容。
- 体量：`fixtures/2025-12-annual.history.json` 共 1053 行，`monthly_recent` 自第 346 行起 ⇒ **约 2/3 的侧车字节没有任何读者**。

⇒ 结论（`omitzero` 的取舍、`[]` vs 缺键的区分）本身是对的、实现也对；**错的是理由**：它指向一个在交付的方法论里不存在的消费者。按「结论对但理由错」的处置——**不动实现**，把注释改成如实的那句：「`monthly_recent` 为非 monthly 期次保留近月上下文，**当前 `prepare.py` 不渲染它**，消费者可自行读取；键的存在/缺失语义由 `omitzero` 保证」。

**另**：`Preceding` 用 `period < ?`，故 annual `2025-12` 的 `monthly_recent` **不含 `2025-12` 月报**。这与 `TestBuildHistoryH1CarriesBothSeries` 的期望一致（h1 `2026-06` → 5 条），是**有意的**，此处只作登记，不是缺陷。

---

### 🟡 M2 [MEDIUM] 变异测试的 harness **未留存**，13/13 KILLED 无法被重跑，只能被重写

**位置**：nanoclaw `2d2fbe8` 全树 × `.arcforge/discoveries/TASK-007.json` `verification.mutation`

**实测**：`git ls-tree -r --name-only 2d2fbe8 | grep -iE 'mutat|harness|mut_'` → **无命中**；`discovery.files_modified` 列出的四个文件里也没有 harness。留下的证据是 `docs/hestia-m3/TASK-007-warp-hestia-verify.md` §2.7 的一张 13 行表（每行「变异内容 + 红 N」）。

**危害**：这直接决定了 §B「变异测试」那一格能被复核到什么程度——**不是「跑一遍」，是「照散文重新实现 13 个变异」**。这一点没有登记，而它正是那一格该不该被信任的关键参数。同样适用于 TASK-006 的 `prepare.py 23/23`。

**建议**：harness 补进 PR #5（`scripts/` 下或 `tests/` 下均可），或至少把「harness 未留存 ⇒ 复核成本 = 重新实现」写进 §B 那一格。**这条与 §3.2 的措辞问题是同一件事的两面**：前者是事实缺失，后者是措辞越界。

---

### 🟢 L1 [LOW] nanoclaw 里的任务归属注释系统性错位（7 文件 11 处）

| 文件 | 注释写的 | 实际 owner（`writes` 为准） |
|---|---|---|
| `SKILL.md:11` | M3 的 TASK-004 | **TASK-005** |
| `references/glossary.md:3` | M3 的 TASK-004 | **TASK-005** |
| `references/methodology.md:3` | M3 的 TASK-004 | **TASK-005** |
| `references/note-format.md:3` | M3 的 TASK-004 | **TASK-005** |
| `references/note-format.md:4` | prepare.py / verify.py 是 TASK-005 | **TASK-006 / TASK-007** |
| `prepare.py:2,391,395` | M3 的 TASK-005 | **TASK-006** |
| `test_prepare.py:2,130,271` | M3 的 TASK-005 | **TASK-006** |
| `verify.py:2,132` | M3 的 TASK-005 | **TASK-007** |
| `test_verify.py:2` | M3 的 TASK-005 | **TASK-007** |

**为什么不是纯洁癖**：nanoclaw 仓库里**没有 `.arcforge/`**，这些注释是这批跨仓库产物**唯一**的任务溯源链。11 处里没有一处指对，下一个人拿 `TASK-005` 去查会查到文档任务、拿 `TASK-004` 会查到 worktree/挂载任务。

**建议**：一次性订正（纯注释，零行为风险）。与 Leader 交底 ③ 里那条「引错来源任务」同源——DoD 层的编号错**已经流进产物**了。

### 🟢 L2 [LOW] CONTRACTS §B「导出面」那格的位置数字索引基未标注

`CONTRACTS.md:3560` 写「位置 `Contract.JSON`=6 → `ContractHistory.FileName`=7 → `ContractHistory.JSON`=8 → `DefaultSignals`=9」。我用三把独立的尺复算 `store_test.go:453` 的 `want`（引号对 / 逗号分割 / 去重后集合，三者同为 **38**，且已按字典序）——这四项的**序数**是 **7 / 8 / 9 / 10**。

两种读法都能对上：0 基索引下原文正确；1 基（中文「第几位」的默认读法）下四个都差 1。**数字不是错的，是缺一个限定词**。TASK-001 的 DoD 里是同一个写法，属同源。

**建议**：写成「0 基下标」或直接改成 7/8/9/10 并写「第 N 位」。

### ℹ️ I1 [INFO] 两个残留 worktree 未收

`wt-nc-t7-test-m3-b` / `wt-nc-t7-test-m3-c`（均 detached @ `2d2fbe8`）仍挂在 nanoclaw 的 `git worktree list` 上，属前三任失联验证者的残留。test-m3-d 已在验证报告 §2 声明「留待 Leader 在阶段边界收」。我自建的 `wt-nc-qa-m3` 由我自己在收尾时拆。

### ℹ️ I2 [INFO] loom PR #13 审查通过，无 finding

`gh pr diff 13` 逐行读过：`source` 走闭合白名单、**校验在任何写盘之前**（`DENIED "source not allowed: …"` 早退不落盘）、非字符串与空串经 `params["source"].(string)` 的失败断言统一回落到 `web-research`、`reviewed: false` 对任何 `source` 仍无条件覆写（`TestArchiveSourceHestiaAllowed` 用一份自带 `reviewed: true` + `source: hestia` 的入站内容钉住了「不可洗白」）。既有 10 处调用点显式传 `web-research`，不靠缺省。测试 35 → 39。**这是本 sprint 三个仓库里证据链最完整的一块。**

---

## 2. 我独立复算过的数字（不是引用他人）

| 项 | CONTRACTS §B 记的 | 我实测 | 取法 |
|---|---|---|---|
| `internal/hestia` 覆盖率 | 2785 / 2880 = 96.7014%，未覆盖块 89 | **2785 / 2880 = 96.7014%，未覆盖块 89** | `-coverprofile` 按语句数累加，非显示值 |
| `cmd/atlas` 覆盖率 | 76.7% | **76.7%** | `go test -cover` 显示值 |
| AST 守卫 `want` | 38 项 | **38**（三把尺同值、已排序、无重复） | 引号对 / 逗号分割 / 去重集合 |
| nanoclaw 测试 | 84 条 | **84 条，2.277s，OK** | 自建 worktree `-m unittest discover` |
| 工作区核空 | 0 行 | **0 行** | `git status --porcelain internal/hestia cmd/atlas` |

⚠️ 第一次数 `want` 时我的正则匹配到了 `store_test.go:400` 的**另一个** `want`（14 项）而不是 453 行那个——是「数错对象」，靠「含 History 的项 = 0」这个不合理的中间量暴露。上表是订正后、锚定行号重算的值。

---

## 3. Leader 交底的三处已知缺口 —— 处置复核

### 3.1 ① 变异测试未独立复核 / 缩范围的理由

**Leader 给的理由**：缩范围是为了减少工具调用窗口，**不是那活重**；证据是「84 tests / 2.313s ⇒ 证伪『活太重』」。

**复核结论：结论大概率成立，但给出的证据支撑不住它，且真正的成本项没被算进去。**

1. **2.3 秒量的是测试套件的 CPU 时间**，而「活重不重」对 agent 而言是**工具调用与上下文的量**，不是 CPU。用 CPU 时间去证伪一个没人在 CPU 意义上提出的主张——这是「结论对但理由错」的标准形状。我自己实测同样是 2.277s，所以我不是在质疑那个数，是在说它**回答的不是那个问题**。
2. **真正的成本项 §1-M2 才查出来**：harness 未留存 ⇒ 独立复核不是「重跑 13 次」，而是**照散文重新实现 13 个变异**。这一项没有被计入「活重不重」的判断，而它恰恰是这个阶段唯一的重活。
3. **三任在同一阶段（建完 worktree 之后）失联**是很强的共因信号，但「工具批次挂起」目前是**推断**不是观察。若真因另在（例如某个变异体让套件挂起、或 harness 某步等 stdin），缩范围只是**碰巧绕开**，下个 sprint 会原样复现。

⇒ **建议措辞**（登记进 §B 或 PENDING）：「缩范围的直接动因是三任在同一阶段连续失联；**共因未查实**。已排除『测试套件本身耗时长』（84 条 2.3 秒，两人独立实测）。**未排除**变异 harness 内部有挂起点——harness 未留存，无法事后检查。」

### 3.2 ② CONTRACTS §B 那句充分性声明

**原文**（`CONTRACTS.md:3571`）：

> **证据强度低的是复核环节，不是 dev 的工作**——**dev 侧的 harness 记录本身是充分的**。

**独立判断：必须改，而且 test-m3-d 提的改法只修对了一半。**

1. **后半句越界成立**。「充分」是对**证据**的判断，只有做过复算或至少审过 harness 的人才给得出；test-m3-d 在报告 §0 明写「未做任何独立复算，也未运行变异 harness」。读一份记录可以支持「记录里写了什么」，支持不了「它足以成立」。
2. **它与同段后文自相矛盾**：紧接着写「不要把它当作与其他数字同等硬」。若记录已经充分，它**就该**与其他数字同等硬。同一格里两句互相削弱，读者会各取一句。
3. **前半句有同样的毛病，test-m3-d 的改法把它留下了。**「证据强度低的是复核环节，**不是 dev 的工作**」同样是一句无人背书的判断——没有任何人检查过 dev 的工作，因此说不出「问题不在那儿」。可说的只有：**没有任何东西检查过它，它是否成立未知**。
4. **「记录详实（可核查）」这个替代措辞仍偏强**（这是我与 test-m3-d 分歧的一点）。§1-M2 查实 harness 未留存 ⇒ 记录**可重新实现**，但**不可重跑**。「可核查」会让读者以为存在一条低成本的核查路径。

**建议措辞**（把「记录里有什么 / 什么没做 / 因此推不出什么」三层拆开，一句不越界）：

> 🔴 **verify.py 这一格：`13/13 KILLED` 仅由 dev 自陈支持，未经任何独立复核。** 成因：前三任验证者在此阶段连续失联（87 / 94 / 62 分钟），人类拍板换人并缩小第四任范围；test-m3-d 已在验证报告 §0 独立成节留痕。
> **记录的形态**：`docs/hestia-m3/TASK-007-warp-hestia-verify.md` §2.7 逐条列出 13 个变异（V1–V12 + P1）的内容与致红条数，并记了隔离副本、`ast.parse` 语法闸、锚点命中数 ≠1 即 `sys.exit(3)`、每轮主工作区指纹校验。
> **未做的事**：无人重跑、无人复算、无人审 harness 源码；**harness 未随 PR 留存**，故事后复核的成本是「照散文重新实现 13 个变异」，不是「重跑一遍」。
> ⇒ **这一格的强度低于本 sprint 其余任何数字，不要与它们同等引用。** 「dev 的工作是否可靠」**未知**——不是「已确认可靠」，也不是「已发现问题」。

### 3.3 ③ DoD 自身的质量（作为独立审查维度）

见 §4。

---

## 4. DoD 质量维度

### 4.1 🔴 结构性缺口：本 sprint 唯一一条跨任务集成指令，**没有任何 DoD 覆盖得到它**

C1/C2 不是偶然的手滑，是一个**位置**的问题：

| 任务 | `writes` | 覆盖 SKILL.md Step 3 的执行结果吗 |
|---|---|---|
| TASK-005（写 SKILL.md） | `docs/hestia-m3/TASK-005-…md` | ✗ 写这条命令时 **`prepare.py` 还不存在**（TASK-006 才实现）——交付记录 ⑤节自己写明了这一点 |
| TASK-006（写 prepare.py） | `docs/hestia-m3/TASK-006-…md` | ✗ DoD 只管 `prepare.py` 与 `test_prepare.py`，不管 SKILL.md |
| TASK-007（写 verify.py） | `docs/hestia-m3/TASK-007-…md` | ✗ 同上 |
| TASK-008（CONTRACTS） | `internal/hestia/CONTRACTS.md` | ✗ 对账数字，不跑命令 |
| 集成冒烟 | — | ✗ **§C 明写本 sprint 不做，结转**（AD-M3-2） |

⇒ 这条命令在**写下时不可能被验证**（依赖物不存在），在**依赖物出现之后没有任何一条 DoD 要求回头跑它一次**，而唯一会跑它的环节被整体结转。**它是一条无主的接缝。**

而 84 条 python 测试**结构上看不见它**：测试用 `prepare("2026-06-h1", ["--existing", path])` 传 Python list，**不过 shell**，词分割在那条路径上根本不发生。⇒ 「测试全绿」与「这条命令能跑」是两件互不相干的事。

**建议**（这条比修那一行重要）：凡 DoD 里出现「照抄进产物的可执行命令」，必须有一条 done_criteria 要求**在依赖物齐备之后原样跑一次**并贴退出码；跑不了（如需容器环境）就在 DoD 里显式写「本命令本 sprint 不可执行，结转冒烟」——**让它有主，或者让它公开无主**，不要两者之间。

### 4.2 DoD 缺陷的分布特征（对 Leader 交底 ③ 的补充）

交底列的 17 处我不重数。补三条从**分布**上看出来的：

1. **17 处全部由下游发现，无一由上游自查出**——这不是「Leader 不够仔细」，是**没有任何环节以 DoD 为审查对象**。dev 读 DoD 是为了实现它、verifier 读 DoD 是为了对照它，**两者都把 DoD 当作前提而不是客体**。第一个把它当客体的环节是 QA，而 QA 在最后。
2. **计数类与位置锚类缺陷占多数**，它们的共同点是「**输出一个形状合理的值，没有任何东西会因为它错了而变红**」。L2 那条（索引基未标注）就是同一形状的又一例，只是这次没错、只是没说清。
3. **编号错已经流出到产物**（L1 的 11 处）。DoD 里的编号错在 sprint 内是内部问题，一旦被抄进跨仓库产物的注释里就变成了**长寿的错误溯源链**——nanoclaw 那边没有 `.arcforge/` 可以对照订正。

---

## 5. 代码质量 / 架构 / 安全 / 性能（常规维度，无 finding 的也记一句）

- **atlas `history.go`**：结构体转换 `historyMeta(o.Meta)` 拿编译器守住字段增删（注释里说明了为什么不逐字段抄）；`toEntries` 遍历 `fieldOrder` 而非 map，键序与「业务字段名不出现在本文件」两件事一起解决；`omitzero` 的取舍（nil = 不适用 / `[]` = 查不到）论证完整且被断言钉住。**无 finding。**
- **两条生产路径的同形性**：`ingest.go` 传 `contractGenerator` 常量、`cmd/atlas` 传 `c.GeneratedBy`（回放时带 `/replay`）——契约与侧车的 `generated_by` 因此**逐路径一致**，消费者能分辨来源而形状不变。`--stdout` 不产侧车（只读命令不产生副作用）也是对的。**无 finding。**
- **失败原子性**：先侧车后契约，两处失败都走 `contractError`（数据已在库、P1 照发、P2 不发）；两个方向都有闸（正向 `TestIngestHistoryLandsBeforeContractOnWriteFailure`，反向两条），反向那两条是 QA 在 TASK-001 复审时补的——**这是本 sprint 已经生效过一次的返工，形状正确。**
- **队列命名**：契约 `<p>-<pt>.json` 与侧车 `<p>-<pt>.history.json` 同在 `pending/`；SKILL.md Step 1 用 `grep -v '\.history\.json$'` 把侧车滤掉。**能工作，但两边靠约定对齐**（atlas 不知道消费者靠后缀过滤）——归到第二轮 Architect 视角，此处不重复计。
- **安全**：无注入面（无外部输入拼进 SQL/shell）；`WriteHistory` 用 `writeAtomic`、`0o755`，与既有 `WriteContract` 同形；`verify.py` 对格式损坏的校验行**报错而不是当作没有**（`scan_markers` 的 `malformed` 桶），方向正确。
- **性能**：`BuildHistory` 对非 monthly 多一次 `Preceding` 查询（`LIMIT 12`，走 `viewCurrent`），可忽略。侧车体积见 M1。

---

## 6. 给 Leader 的 fix_items 建议

| # | 级别 | 任务 | 动作 |
|---|---|---|---|
| 1 | CRITICAL | TASK-005（SKILL.md 归它） | 修 Step 3 的两个缺陷（引号 + 存在性判断），并**在依赖物齐备的当前状态下原样跑一次**贴退出码 |
| 2 | HIGH | TASK-005 | Step 4 补一句「表外字段读契约 JSON」；`prepare.py:reading_hints()` 口径同步（后者属 TASK-006 文件，二选一或并做） |
| 3 | HIGH | TASK-008 | CONTRACTS §D 第六条按 §1-H2 的实测表重写范围；§B 变异那格按 §3.2 的建议措辞重写 |
| 4 | MEDIUM | TASK-007 | 变异 harness 补进 PR #5；或在 §B 登记「harness 未留存」 |
| 5 | MEDIUM | TASK-001 | `history.go:62-66` 注释改成如实措辞（不动实现） |
| 6 | LOW | TASK-005/006/007 | 11 处任务归属注释订正 |
| 7 | LOW | TASK-008 | §B 位置数字标注索引基 |

`reason_class` 建议：**`task_defect`**（1、2、4、5、6 是实现/产物与 done_criteria 不符或产物自身有缺陷）。§4.1 那条结构性缺口属 **`dod_defect`**，但它**不适合走返工**——done_criteria 不矛盾也不是不可测试，是**缺一条**；建议作为流程改进记进 PENDING，不要为它开返工轮。

---

# 7. 订正：C1 的机制被证伪（2026-09-09，Leader 的区分性实验 + 我的跨 shell 复核）

**本节是追加，§1 的 C1 / C2 原文一字未改。** 两次判断依据的证据不同，混写会让后人分不清哪一句有观察支撑。

## 7.1 我原本写了什么，错在哪

原文 C1 断言：「`${var:+word}` 里的双引号**不保护展开结果**——`word` 展开后仍要过一次词分割」，并据此判「create 与 update **两种场景都恒定失败**」「修一个不够，两个都要修」。

**机制是错的。四个 shell 里没有一个发生词分割。**

## 7.2 Leader 的区分性实验（构造要点：让目标文件**存在**，以屏蔽 C2）

C1 与 C2 会产生同一个可见后果（exit 2、产出 0 字节），所以**只跑 Step 3 原文分不开这两者**——这正是我原文里把它们叠在一起看的原因。Leader 的做法是**先把 `--existing` 指向的笔记文件真的建出来**：这样 C2（文件读不到）被屏蔽，剩下的失败只可能来自 C1。

结果：

```
文件不存在（照 Step 3 原样）：
  --existing 读取失败: [Errno 2] ... '/tmp/vault/2025 全年金融数据解读.md'   ← 路径完整到达，含空格
  退出码=2  产出字节=0
文件存在（C2 被屏蔽，单独看 C1）：
  bash 退出码=0  产出字节=3338
  sh   退出码=0  产出字节=3338
```

⚠️ Leader 还指出了一处**我手上就有、却读反了的反证**：第一次实验的错误消息里那个**带空格的完整路径**本身就说明路径没被拆开——真发生词分割的话，argparse 会把 `--existing` 认成一个正常选项、只对多出来的那半截报错，而不是把整串一起吐出来。

## 7.3 我的独立复核：直接观察 argv，而不是从 argparse 的报错倒推

我原本的判据是**从 argparse 的错误消息倒推 argv 的形态**，而「`unrecognized arguments`」这一条**与「被拆开」和「被粘成一个」两种情况都相容**——它区分不了。换一个能直接观察的仪器：让被调用方把 `sys.argv` 逐项打出来。

```sh
N="2025 全年金融数据解读.md"; EXISTING="/tmp/vault/$N"
python3 argv.py contract.json history.json ${EXISTING:+--existing "$EXISTING"}
```

| shell | argc | argv 形态 |
|---|---|---|
| **bash** | **4** | `contract.json` · `history.json` · `--existing` · `/tmp/vault/2025 全年金融数据解读.md` |
| **sh** | **4** | 同上 |
| **dash** | **4** | 同上 |
| **zsh** | **3** | `contract.json` · `history.json` · **`--existing /tmp/vault/2025 全年金融数据解读.md`（整串一个 argv）** |

⇒ **Leader 是对的：引号在 bash / sh / dash 下都生效，路径带空格也完整传入，词分割根本不发生。**

⇒ **我的观察是真的，但成因与我写的正好相反**：本会话的 Bash 工具跑的是 **zsh**（`$0 = /bin/zsh`，`$SHELL=/bin/zsh`；本报告前面那条 `(eval):1: no matches found` 也是 zsh 的报错文案）。zsh **不做**词分割，于是 `--existing` 与路径被**粘成一个 argv**，argparse 认不得它 ⇒ `unrecognized arguments`。**「拆开了」与「粘住了」是相反的两件事，而它们在 argparse 的报错里长得一样。**

## 7.4 哪个 shell 才算数

nanoclaw 容器基于 `node:22-slim`（Debian）；`container/entrypoint.sh` 的 shebang 是 `#!/bin/bash` 且它确实在跑 ⇒ 镜像里有 bash，skill 的代码块由 bash 执行。**⇒ 以 bash 为准，Leader 的划界是生效的那一个。**

（⚠️ Debian 的 `/bin/sh` 是 **dash**；dash 与 bash 在这一点上行为一致，所以不影响结论。zsh 不在容器里，本报告 §1 的实验环境与运行环境不同——这也是我该在第一次就交代而没交代的。）

## 7.5 订正后的结论

| | 原文 | 订正后 |
|---|---|---|
| 缺陷条数 | 两条（C1 引号 + C2 缺存在性判断） | **一条**：`EXISTING=…/$N` **无条件赋值** ⇒ 恒非空 ⇒ `--existing` 恒传入 |
| 波及场景 | create 与 update **都**恒定失败 | **只有 create 失败**；**update 正常**（Leader 实测 exit 0 / 3338 字节） |
| 等级 | CRITICAL | **CRITICAL 维持**——每一期的**首次**生成都失败，而首次是常态 |
| 严重性表述 | 「一份笔记也产不出」 | **「新笔记一份也产不出，已有笔记的更新正常」** |
| 修法 | 「修一个不够，两个都要修」 | **只有一条**：把无条件赋值改成条件传参 |

🔴 **不要让 dev 去「修引号」**——那里没有引号缺陷。改它是无谓改动，而且 dev 验证时会得到「改前改后都一样」的困惑结果。

## 7.6 🔴 但 Leader 建议的那个修法在 dash 下语法错误——实测

Leader 建议 `ARGS=(); [ -f "$EXISTING" ] && ARGS=(--existing "$EXISTING")`，再 `"${ARGS[@]}"`。**数组不是 POSIX**：

| shell | `ARGS=()` 数组写法 | §1 建议的 if/else 写法 |
|---|---|---|
| bash | ✅ argc=2（分支正确） | ✅ argc=4，路径完整 |
| sh | ✅ argc=2 | ✅ argc=4 |
| zsh | ✅ argc=2 | ✅ argc=4 |
| **dash** | 🔴 **`Syntax error: "(" unexpected`** | ✅ **argc=4** |

容器里 bash 在，所以数组写法**能跑**；但 Debian 的 `/bin/sh` 是 dash，而这段代码是**照抄进 SKILL.md 给模型执行的**——模型用 `sh -c` 还是 `bash -c` 不由这份文档决定。

⇒ **修法取 §1-C1 已给的 if/else 形式**（四个 shell 全过，且不依赖任何非 POSIX 特性）：

```sh
N=$(python3 /app/skills/warp-hestia/scripts/prepare.py --print-name "$Q/processing/$F")
EXISTING="/workspace/extra/vault/Wiki/Macro/PBOC/$N"
if [ -f "$EXISTING" ]; then
  python3 /app/skills/warp-hestia/scripts/prepare.py "$Q/processing/$F" "$Q/processing/$H" --existing "$EXISTING" > /tmp/note.md
else
  python3 /app/skills/warp-hestia/scripts/prepare.py "$Q/processing/$F" "$Q/processing/$H" > /tmp/note.md
fi
```

⚠️ 这里的引号**仍然要加**，但理由与我原文写的不同：不是「防词分割」（`${:+}` 那处本来就不漏），而是 `$N` **必然含空格**，`--existing $EXISTING` 这种**裸展开**在 bash/sh/dash 下是真的会被拆开的。**「这个位置需要引号」是对的，「那个位置漏了引号」是错的**——两句话看起来很像。

## 7.7 §6 fix_items 的修订

§6 第 1 行「修 Step 3 的两个缺陷（引号 + 存在性判断）」⇒ 改为：

> **修 Step 3 的一个缺陷**：`EXISTING` 无条件赋值 ⇒ create 场景恒传 `--existing`。用 §7.6 的 if/else 形式（**不要用数组，dash 下语法错**）。验收：**create 与 update 各跑一次**并贴退出码与产出字节数——只跑 update 会看不出问题（update 本来就是好的）。

其余 12 条不受影响。**verdict 不变：REJECT。**

## 7.8 这个错误是怎么活下来的（值得记的那一条）

不是「我没实测」——我实测了，命令和输出都在 §1 里。**错在仪器**：我拿 argparse 的错误消息去推 argv 的形态，而那条消息**对「拆开」和「粘住」给出同一个输出**。两个相反的成因、一个相同的可观测量，我选了更眼熟的那个解释填进去，然后把它写成了机制。

能区分它们的仪器是**直接把 `sys.argv` 打出来**（§7.3），成本比我原本那次实验还低。

⇒ 判据：**当我要为一个现象写下「因为 X」时，先问「若成因是 X 的反面，我看到的输出会不一样吗」。** 答「一样」就说明手上的仪器答不了这个问题，此时该换仪器，而不是挑一个解释。

⇒ 附带一条：**§1 的实验跑在 zsh（本会话默认 shell），而运行环境是容器里的 bash。** 我当时没有交代实验环境——若交代了，「换个 shell 复跑一次」是任何读者都会想到的下一步。**报告里写清「这个数字是在哪个环境采的」，本身就是一道能被别人接手的闸。**
