# TASK-005（Arcforge）/ TASK-004（需求文档）交付记录：`warp-hestia` skill 文档

> 承载形态见 `.arcforge/docs/01-design/design-spec.md` §3：本文件**不是摘要，是证据载体**。
> 验证者不以本文件为准——按 ③节给出的全 sha 锚点去 nanoclaw 实跑。
>
> **编号映射**：Arcforge TASK-005 = 需求文档 `2026-09-06-hestia-m3-warp-hestia.md` 的「## TASK-004」节。
> atlas commit subject 用 Arcforge 编号（`docs(TASK-005):`），文档内注释里的里程碑编号用需求编号（`M3 的 TASK-004`）。
> 两处编号不同是**预期的，不是笔误**。
>
> **采样纪律**：本文件所有字节数、行数、计数、sha 均在**最后一次改动（nanoclaw commit `aff2521433eef8567addcedc988fecad3ed08c6f`）之后统一重采**，
> 与 ③节锚点同一时刻。

---

## ① 改动清单

四个**新建**文件，全部在 nanoclaw 的 `container/skills/warp-hestia/` 下，**被 git 跟踪、进 PR**
（与 TASK-004 的 `groups/*` 不同，那两个是未跟踪的本机运行时数据）。

```
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia show --numstat --format='' aff2521433eef8567addcedc988fecad3ed08c6f
112	0	container/skills/warp-hestia/SKILL.md
110	0	container/skills/warp-hestia/references/glossary.md
161	0	container/skills/warp-hestia/references/methodology.md
125	0	container/skills/warp-hestia/references/note-format.md
```

合计 **508 行新增、0 删除**（纯新建）。字节数：

```
$ wc -c container/skills/warp-hestia/SKILL.md container/skills/warp-hestia/references/*.md
    5259 container/skills/warp-hestia/SKILL.md
    5965 container/skills/warp-hestia/references/glossary.md
   12331 container/skills/warp-hestia/references/methodology.md
    6219 container/skills/warp-hestia/references/note-format.md
   29774 total
```

### 逐文件要点

| 文件 | 内容 | 对应 DoD |
|---|---|---|
| `SKILL.md` | frontmatter（三触发语 + 两条边界声明）；§1 三条 I/O 约定 + **模型边界**小节；§2 六步；**四条失败分支一览表**；§3 **四条**不做 | functional[0]、boundary[0] |
| `references/note-format.md` | 五种 `period_type` 命名规则（**取无月份版**，冲突裁决写在文内）；frontmatter 字段表；正文骨架；**两级校验规格**（分段 check 恰 2 条 + 封条 seal 恰 1 条）；`mode` 规则；修订 | functional[1] |
| `references/glossary.md` | 三个 `caliber_version` 与跨口径禁忌；`_ytd`/`_mom` 且**口径按段不按观测**；月均取法三条；**四信号判定表**；单位 | functional[2] |
| `references/methodology.md` | 八概念、八问框架、五条传导链、误读陷阱、写作纪律 —— **提炼非复制**（12331 字节 = 源 41113 的 30.0%） | functional[3] |

### 未跟踪文件：无

本任务四个文件全部被 git 跟踪，`sha256sum` 替代口径**不适用**，改动清单以 `git show --numstat` 为准。

### 🔴 `warp-research` 一行未动（DoD non_functional 明写的约束）

```
$ cd /Users/zuowei/workspace/ai/wt-warp-hestia
$ git show --numstat --format='' aff2521433eef8567addcedc988fecad3ed08c6f -- container/skills/warp-research | grep -c .
0                       (须 0 —— 该 commit 在 warp-research 下改动 0 个文件)
```

---

## ② 实测输出

以下全部采于 nanoclaw commit `aff2521433eef8567addcedc988fecad3ed08c6f` 之后，与 ③节锚点同一时刻。
**每条判据都给命令 + 原样输出 + 计数**；`(须 N)` 是 DoD 写死的期望值。

### 2.1 SKILL.md 结构（functional[0]）

```
$ awk '/^## §3 不做/{f=1;next} /^## /{f=0} f&&/^- /{n++} END{print n+0}' SKILL.md
4
$ sed -n '/^## §3 不做/,$p' SKILL.md | grep -c '^- '
4
```
🔴 **§3 恰 4 条**，两把独立的尺（awk 段内计数 / sed 切段后 grep）各算一遍，结果一致。
DoD 明写「初稿写『五条』是错的，实测需求 `## §3 不做` 恰 4 个 bullet」——这里就是那 4 条，没有硬造第五条。

```
$ grep -n '^\*\*Step [1-6] ' SKILL.md
35:**Step 1 扫队列**
43:**Step 2 占位**
57:**Step 3 组装**
70:**Step 4 写叙述**
75:**Step 5 校验并写回**
93:**Step 6 收尾**
$ grep -c '^\*\*Step [1-6] ' SKILL.md
6                       (须 6)

$ awk '/^## §1 /{f=1;next} /^### |^## /{f=0} f&&/^[0-9]\. /{n++} END{print n+0}' SKILL.md
3                       (须 3 —— 队列 / vault / 写回)
```

frontmatter 六个必含字串，逐条 `grep -qF`：

```
$ for k in 'name: warp-hestia' '处理 hestia 队列' '生成金融数据解读' '解读这期央行数据' \
           '一次只处理一份' '不用于回答一般宏观问题'; do grep -qF -- "$k" SKILL.md && echo x; done | wc -l
6                       (须 6/6)
```

### 2.1b SKILL.md 的四条失败分支与两个边界（boundary[0]）

DoD 要求 Step 2 处理两个边界、Step 3/5/6 各有失败分支，其中 Step 5 的队列处置是 Leader 裁决项。
SKILL.md 把四条汇成一张表（表体**恰 4 行**），各步正文里另有一份展开说明：

```
$ awk '/^### 失败分支一览/{f=1;next} /^## /{f=0} f&&/^\| Step/{print}' SKILL.md
| Step 2 | history 侧车缺失 | 契约**单独**移 `failed/` | 补发命令 |
| Step 3 | `prepare.py` 退出码非零 | 契约与侧车**对移 `failed/`** | stderr **原文** |
| Step 5 | `verify.py` 退出码非零 | 🔴 **留在 `processing/`**，不移 | 差异；下次会话自然重试 |
| Step 6 | `spool.archive` 返回 `DENIED` / `ERROR` | 契约与侧车**对移 `failed/`** | 返回**原文**，不重试 |
$ awk '…同上，把 print 换成计数…' SKILL.md
4                       (须 4)
```

关键字逐条 `grep -cF`（数字是全文件命中次数，均 ≥1）：

```
history 侧车缺失                     3
contract emit --period           2
同名已在 `processing/`               1     ← Step 2 边界②：不重新占位
不重新占位                            1
退出码非零                            4     ← Step 3 与 Step 5 各在正文与表里出现
对移 `failed/`                     3     ← Step 3 与 Step 6
留在 `processing/`，不移 `failed/`    1     ← 🔴 Step 5 的 Leader 裁决项
不重试                              3     ← Step 6 与 §1 第 3 条
```

🔴 **Step 5 那一条是本任务唯一的规范新增**：需求 line 711 与 spec §7 都只说「拒绝写回」，
**没写队列怎么处置**。裁决与理由写在 ④节 F。

### 2.2 note-format.md 的两级校验规格（functional[1]）

骨架代码块**内部**的计数（只数 ``` 围栏内的行，不含正文散文里的引用）：

```
骨架内 check 行 = 2 (须 2)
骨架内 seal  行 = 1 (须 1)
骨架内 ## 标题 = 4 (机器区 3 + 批注区 1；正是「不能按标题数推 check 数」的原因)
    ## 本期数据
    ## 前 12 期
    ## 信号
    ## 我的批注
```

🔴 **这 4 与 2 的差正是 DoD 要防的那个装反的闸**：机器区有 3 个 `## `（本期数据 / 前 12 期 / 信号）
而只有 2 条 check 行（`## 信号` 段不发）；`## 我的批注` 在机器区之外。
所以 `note-format.md` 把条数**写死为 2**，并显式写了「不要按 `## ` 标题数去推 check 行数」。

对照尺（awk 全文件计数，含散文引用，故应 ≥ 骨架内计数）：

```
$ awk '/<!-- check:/{c++} /<!-- seal:/{s++} END{printf "%d %d\n", c, s}' references/note-format.md
3 2
```
全文件 check 3 次 / seal 2 次 ≥ 骨架内 2 / 1，方向自洽（多出的各 1 次是「作用域定义」小节里的行内引用）。

### 2.3 methodology.md 的三条可跑判据（functional[3]）

**判据①：字节数 ≤ 源文件 40%**

```
$ wc -c '/Users/zuowei/Obsidian/ClawdVault/Projects/Hestia/PBOC2026年上半年金融数据解读-完整版.md'
   41113 /Users/zuowei/Obsidian/ClawdVault/Projects/Hestia/PBOC2026年上半年金融数据解读-完整版.md
$ wc -c container/skills/warp-hestia/references/methodology.md
   12331 container/skills/warp-hestia/references/methodology.md
```
12331 / 41113 = **30.0%**，上限 41113 × 40% = **16445** ⇒ **通过，余量 4114 字节**。
（源文件 41113 字节与 DoD 记的实测值逐字相符。）

**判据②：`## ` 标题集合与源文件 `## ` 集合交集 ≤ 1（只比 `## `，不比 `### `）**

```
methodology 的 ## 标题 (5):
    ## 八个概念（判读用得上的那一层）
    ## 八问框架（叙述骨架，逐问写）
    ## 五条传导链（跨字段关联的提示）
    ## 误读陷阱
    ## 写作纪律
源文件的 ## 标题 (12)
交集条数: 0  (上限 1)
交集内容: []
```
源文件的 12 个 `## ` 是「一、二、三…九 + 附录 A/B/C」式章节名，本文件一个都没用 ⇒ 交集 0。
⚠️ 八问与五链在本文件里是 `### `（八问）与 `- `（五链），**不参与本判据**——
DoD 明写只比 `## `，因为把八问写成 `## ` 是最自然的选择，比 `### ` 会误伤合格文档。

**判据③：13 个名称逐条可 grep**

```
经济是在扩张，还是在收缩？                            命中 1 次
房地产是否真正复苏？                               命中 1 次
消费到底回暖没有？                                命中 1 次
企业是在扩张，还是在维持经营？                          命中 1 次
企业贷款增长有没有水分？                             命中 1 次
钱去哪儿了？—— 存款搬家与资金空转                       命中 1 次
谁在给经济加杠杆？—— 部门杠杆的转移                      命中 1 次
钱贵不贵？—— 利率与资金面                           命中 1 次
房地产链                                     命中 1 次
消费链                                      命中 1 次
财政替代链                                    命中 1 次
资金空转链                                    命中 1 次
外部链                                      命中 1 次
未命中条数: 0
```
八问的 8 个名称 + 五条传导链的 5 个名称 = **13/13 命中**（用 `grep -cF`，逐字面量，无正则）。

### 2.4 glossary.md 五块齐全（functional[2]）

```
$ grep -n '^## ' container/skills/warp-hestia/references/glossary.md
6:## 一、三个 `caliber_version`
30:## 二、`_ytd` 与 `_mom`：两个口径，别混比
50:## 三、月均取法（三条，按顺序试）
62:## 四、四信号判定表
100:## 五、单位
$ grep -c '^## ' container/skills/warp-hestia/references/glossary.md
5                       (须 5)
```

五块的关键内容逐条 `grep -cF`（数字是命中次数）：

```
2015-01                1     ← 三个 caliber_version
2023-01                2
2025-01                2
跨口径对比禁忌                1     ← 禁忌单列一小节
年初累计                   1     ← _ytd
当月                     4     ← _mom
口径是「按段」的               1     ← 存款/贷款可能不同口径（2023-08 实例）
signal_activation      1     ← 四信号判定表
signal_housing         1
signal_consumption     1
signal_credit          1
unknown                6     ← 四态：输入缺失是 unknown 不是红
万亿元                    1     ← 单位：存量
亿元                     2     ← 单位：流量
百分数                    2     ← 单位：比率
```

判定表的三条易错点都写进去了，且与 atlas `internal/hestia/signals.go` 的 `Evaluate` 同源：
① **楼市与消费两个信号没有黄灯**（只有绿/红）；
② **信贷信号的分子分母必须同口径**（都 `_mom` 或都 `_ytd`，`_mom` 优先），企业贷款合计为 0 ⇒ `unknown`；
③ **温度分母是 `temp_known` 不是 4**——有 `unknown` 时分母减一，「温度 1/3」是合法写法。

### 2.5 禁用依赖自查（error_handling）

```
$ grep -rn 'pip install\|import requests\|import yaml\|curl \|wget ' container/skills/warp-hestia/
$ echo "grep 退出码=$?"
grep 退出码=1
```
**无输出、退出码 1 = 无命中 = 通过。** 四份文档只用容器内已有的东西：
`python3` 标准库、`ls` / `grep` / `mv` / `cat` / `rg`、`selvage_call`。

### 2.6 提交与工作区状态

```
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia log --oneline -1
aff2521 feat(warp-hestia): SKILL.md 六步流程 + methodology / note-format / glossary（Hestia M3）

$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia status --short
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia status --short | grep -c .
0                       (第一条命令无输出；第二条计数为 0 = 工作区干净)
```
提交信息按 **nanoclaw 规范**（`feat(warp-hestia): …`），**不是** atlas 门禁要求的 `docs(TASK-005):`
——那个前缀只用在 atlas 侧的提交（见 2.6）。

**主 checkout 未被触碰**：

```
$ git -C /Users/zuowei/workspace/ai/nanoclaw rev-parse --abbrev-ref HEAD
feat/vendor-agent-reach-skill
$ git -C /Users/zuowei/workspace/ai/nanoclaw status --short
 M CLAUDE.md
 M container/agent-runner/src/mcp-tools/index.ts
 D groups/global/CLAUDE.md
 D groups/main/CLAUDE.md
?? .claude/skills/gitnexus/
```
与 TASK-004 交付时逐字相同（2 改 2 删 1 未跟踪）——未 checkout、未 stash、未提交。

### 2.6b 任务编号引用带 milestone 前缀（non_functional[0]，reviewer N4/O7(a)）

需求 line 25 的全局约束「注释里引用任务编号带 milestone 前缀」是**义务**。
四份文档里一共 5 处引用任务编号，**5 处全部带前缀**：

```
$ grep -rn 'TASK-[0-9]' container/skills/warp-hestia/
container/skills/warp-hestia/SKILL.md:11:<!-- M3 的 TASK-004：本文件的六步与 §3 边界照需求文档原文，不得自行增删。 -->
container/skills/warp-hestia/references/methodology.md:3:<!-- M3 的 TASK-004：从 vault《PBOC2026年上半年金融数据解读-完整版》提炼，不是复制。 -->
container/skills/warp-hestia/references/glossary.md:3:<!-- M3 的 TASK-004：五块——caliber_version / _ytd 与 _mom / 月均取法 / 四信号判定表 / 单位。 -->
container/skills/warp-hestia/references/note-format.md:3:<!-- M3 的 TASK-004：命名规则、frontmatter 字段表、机器区边界与两级校验的唯一契约来源。 -->
container/skills/warp-hestia/references/note-format.md:4:<!-- prepare.py / verify.py（M3 的 TASK-005）与本文件必须逐字一致——它们三方共用这份规格。 -->

$ grep -rho 'TASK-[0-9][0-9][0-9]' container/skills/warp-hestia/ | wc -l
5
$ grep -rho 'M3 的 TASK-[0-9][0-9][0-9]' container/skills/warp-hestia/ | wc -l
5                       (5/5，无裸编号)

$ grep -rho 'M3 的 TASK-[0-9][0-9][0-9]' container/skills/warp-hestia/ | sort | uniq -c
   4 M3 的 TASK-004
   1 M3 的 TASK-005
```

用的是**需求文档编号**（本任务 = 需求 TASK-004；脚本任务 = 需求 TASK-005），
而 atlas commit subject 用 **Arcforge 编号** `docs(TASK-005):`。
🔴 **两套编号在同一次交付里并存是预期的**——读者不同：文档注释给三仓库开发者，
commit subject 给 atlas 的门禁 `task-completed.sh`。

### 2.7 code-simplifier（全局规范）

全局 CLAUDE.md 要求提交前跑 `code-simplifier`。本任务产物是纯 Markdown（无 Go、无 Python），
仍照跑了一次，并在 prompt 里把上面全部硬性判据作为不可破坏的约束交给它。

**结果：未做任何改动。** 核实方式不是采信它的报告（它只回了一句「Complete」），而是看载体：

| 文件 | 我写入时字节 | 子代理运行后字节 |
|---|---|---|
| `SKILL.md` | 5259 | **5259** |
| `references/glossary.md` | 5965 | **5965** |
| `references/methodology.md` | 12331 | **12331** |
| `references/note-format.md` | 6219 | **6219** |

且四个文件 mtime 仍是我的写入时刻（11:51 / 11:52 / 11:53 / 11:55）。
**更强的一层**：2.1–2.4 的全部 11 条判据是在子代理运行**之后**复跑一遍的，全部仍然通过——
即便发生了等长度的改写也会被这轮复跑逮住。

---

## ③ 锚点

| 项 | 值 |
| --- | --- |
| nanoclaw worktree **绝对路径** | `/Users/zuowei/workspace/ai/wt-warp-hestia`（**TASK-004 建的，本任务复用，未新建**） |
| nanoclaw 分支 | `feat/warp-hestia` |
| nanoclaw 本任务 commit **全 sha** | **`aff2521433eef8567addcedc988fecad3ed08c6f`** |
| nanoclaw 本任务的父 commit（= TASK-004 交出时的 HEAD） | `d791101e1defcfd3d3d3fcf7ae32a85ec420547b` |
| nanoclaw 主 checkout 分支 / HEAD 全 sha | `feat/vendor-agent-reach-skill` / `aefea6ce5439cfcf15b3dc54ea9bd507c5846c95` |
| nanoclaw PR 链接 | **无——本任务不开 PR**（见 ④节 A） |
| methodology.md 的源文件 | `/Users/zuowei/Obsidian/ClawdVault/Projects/Hestia/PBOC2026年上半年金融数据解读-完整版.md`（41113 字节） |
| atlas 基线 HEAD 全 sha（本文件写于其上） | `7e24b116faff2174771d4551b54aa582a874d209` |
| atlas worktree / 分支 | `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-TASK-005-m3` / `task/TASK-005-m3` |

> 🔴 **worktree 交给下游时是干净的**（`git status --short` 无输出）。
> TASK-006 继续在同一个 worktree 的同一分支上加 `scripts/`；**TASK-007 负责拆**。

---

## ④ 未做与理由

### A. 本任务**不开 PR**（DoD non_functional 明写）

需求 TASK-004 的 Step 5 只有 `git add` + `git commit`，没有 `gh pr create`。
DoD 进一步写明「本任务**先不开 PR**（TASK-006 的脚本要进同一个 PR）」。
⇒ ③节的 PR 链接为「无」，这是**预期结果**，不得据此判 rejected。
`feat/warp-hestia` 分支上现在有 1 个 commit（`aff2521433eef8567addcedc988fecad3ed08c6f`），TASK-006 会在其上再加脚本与测试，届时一并开 PR。

### B. `scripts/prepare.py` / `verify.py` **不实现**，且不因它们不存在而阻塞

`SKILL.md` 的 Step 3 与 Step 5 引用了 `/app/skills/warp-hestia/scripts/prepare.py` 与 `verify.py`，
这两个文件**现在不存在**——它们是需求 TASK-005（= Arcforge TASK-006）的产物。

**这是文档先行、脚本后到的有意安排**，DoD 原文：「⚠️ SKILL.md 的六步里引用了 `scripts/prepare.py` / `verify.py`
（TASK-006 才实现）——**这是预期的**，文档先行、脚本后到；本任务**不实现脚本**，也**不因脚本不存在而阻塞**。」

⇒ 验证者请勿因 `ls container/skills/warp-hestia/scripts/` 为空而判不通过。
本文件 `note-format.md` 的两级校验规格是**写给 `verify.py` 的契约**，TASK-006 照它实现即可。

### C. 文件名规则：取**无月份**版，与上游 spec 冲突（裁决已写进 `note-format.md`）

| 出处 | 写法 |
|---|---|
| 实施计划 line 853（`test_print_name` 断言） | `2026 上半年金融数据解读.md` |
| 实施计划 line 967（冒烟命令的 `$N`） | `2026 上半年金融数据解读.md` |
| 实施计划 line 1012（冒烟判据） | `2026 上半年金融数据解读.md` |
| **spec §6 路径示例（line 197）** | `2026-06 上半年…`（**带月份**） |
| **spec §8.2 判据 3（line 273）** | `2026-06 上半年金融数据解读.md`（**带月份**） |

**裁决：取无月份版**——实施计划内部三处自洽，且 `test_print_name` 的断言就是它；spec 那两处是孤例。
这条冲突与裁决**显式写进了 `note-format.md`**（DoD 要求），理由是它会在结转的集成冒烟里再被人看到一次；
不写在文档里，届时有人会照 spec 改回去，`prepare.py --print-name` 与冒烟判据就对不上了。

### D. `examples/2026-06-h1.md` 不建

spec §5.1 的目录树里有 `examples/2026-06-h1.md`，但它注明是「冒烟产物经人审后回填」，
属需求 TASK-006（人执行）的范围，不在本任务的 Files 清单里。⇒ 本任务不建 `examples/` 目录。

### E. 🔴 本任务的产物在冒烟时**看不见**，需要两步人执行动作（结转）

这是 TASK-004 查实、Leader 已复核的机制事实，**直接影响本任务产物能不能被调到**：

- `container-runner.ts:346-349` 把 `<projectRoot>/container/skills` **只读 bind mount** 到容器的 `/app/skills`；
  Dockerfile 无 `COPY skills`，头注释 `:9-11` 明写 source 从不烘进镜像 ⇒ **不需要重建镜像**。
- 🔴 但 `projectRoot = process.cwd()`（`container-runner.ts:273`）= nanoclaw 主进程工作目录 = **主 checkout**。
  ⇒ 我写在 worktree `/Users/zuowei/workspace/ai/wt-warp-hestia` 里的 `warp-hestia`，**容器看不见**。

⇒ 冒烟前必须两步，**都是人执行**（已在 TASK-008 §D 结转清单）：
① nanoclaw PR 合并到 `main`；② 本机 checkout 切回 `main`。

⚠️ **这不影响本任务的交付**——四份文档是纯 Markdown，开发与校验都在 worktree 里跑，不经容器。
**本任务刻意没有**为了让容器看见而去切主 checkout 的分支、或把文件拷进主 checkout：
那会动到别人未提交的改动（主 checkout 现有 2 改 2 删），代价远大于收益。

### F. 需求原文与本交付的两处有意偏离

1. **SKILL.md 多了「模型边界」小节与「失败分支一览」表。** 需求原文的 §1 只有三条 I/O 约定、
   §2 的失败分支散在各步里。DoD 要求「🔴 模型边界必须明写」并补了两条初稿漏掉的失败分支
   （reviewer O8）⇒ 把边界提成 §1 的独立小节、把四条失败分支汇成一张表。
   **§3 的四条一字未改**，与需求原文逐字相同。
2. **`verify.py` 失败时契约留 `processing/`。** 需求 line 711 与 spec §7 都只说「拒绝写回」，
   **没写队列怎么处置**。Leader 裁决：**留在 `processing/`，不移 `failed/`**——
   数据段被改是模型这一轮的问题，契约本身没毛病，移 `failed/` 会让一份好契约需要人工捞回；
   留 `processing/` 则下次会话按 Step 2 的「同名已在 processing ⇒ 直接用那对」自然重试。
   这条已按 DoD 要求写进 SKILL.md（Step 5 与失败分支表各一处）。
