# TASK-005（Arcforge）/ TASK-004（需求文档）交付记录：`warp-hestia` skill 文档

> 承载形态见 `.arcforge/docs/01-design/design-spec.md` §3：本文件**不是摘要，是证据载体**。
> 验证者不以本文件为准——按 ③节给出的全 sha 锚点去 nanoclaw 实跑。
>
> **编号映射**：Arcforge TASK-005 = 需求文档 `2026-09-06-hestia-m3-warp-hestia.md` 的「## TASK-004」节。
> atlas commit subject 用 Arcforge 编号（`docs(TASK-005):`），文档内注释里的里程碑编号用需求编号（`M3 的 TASK-004`）。
> 两处编号不同是**预期的，不是笔误**。
>
> **采样纪律**：本文件所有字节数、行数、计数、sha 均在**最后一次改动（nanoclaw commit `7ca6b57e64a561351d7108a937b8607a5e87f0a6`，即 ④节 G 的两处订正）之后统一重采**，与 ③节锚点同一时刻。
> ⚠️ **唯一的例外是 2.7 的 code-simplifier 那张表**——它记的是**第一轮**（`aff2521…`）当时的字节，用途是证明子代理没改动过那一版，性质上必须是那个时点的值；表内已标明。

---

## ① 改动清单

四个**新建**文件，全部在 nanoclaw 的 `container/skills/warp-hestia/` 下，**被 git 跟踪、进 PR**
（与 TASK-004 的 `groups/*` 不同，那两个是未跟踪的本机运行时数据）。

本任务在 nanoclaw 上有**两个 commit**（第二个是交付后按 Leader 裁决做的两处订正，见 ④节 G）：

```
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia show --numstat --format='' aff2521433eef8567addcedc988fecad3ed08c6f
112	0	container/skills/warp-hestia/SKILL.md
110	0	container/skills/warp-hestia/references/glossary.md
161	0	container/skills/warp-hestia/references/methodology.md
125	0	container/skills/warp-hestia/references/note-format.md

$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia show --numstat --format='' 7ca6b57e64a561351d7108a937b8607a5e87f0a6
3	1	container/skills/warp-hestia/SKILL.md
4	0	container/skills/warp-hestia/references/note-format.md
```

**累计**（相对分支起点 `d791101e1defcfd3d3d3fcf7ae32a85ec420547b`，即 TASK-004 交出时的 HEAD）：

```
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia diff --numstat d791101e1defcfd3d3d3fcf7ae32a85ec420547b 7ca6b57e64a561351d7108a937b8607a5e87f0a6
114	0	container/skills/warp-hestia/SKILL.md
110	0	container/skills/warp-hestia/references/glossary.md
161	0	container/skills/warp-hestia/references/methodology.md
129	0	container/skills/warp-hestia/references/note-format.md
```

合计 **514 行新增、0 删除**（纯新建；第二个 commit 的那 1 行删除是 Step 3 的整行替换，被累计 diff 吸收）。字节数：

```
$ wc -c container/skills/warp-hestia/SKILL.md container/skills/warp-hestia/references/*.md
    5667 container/skills/warp-hestia/SKILL.md
    5965 container/skills/warp-hestia/references/glossary.md
   12331 container/skills/warp-hestia/references/methodology.md
    6727 container/skills/warp-hestia/references/note-format.md
   30690 total
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
$ git diff --numstat d791101e1defcfd3d3d3fcf7ae32a85ec420547b 7ca6b57e64a561351d7108a937b8607a5e87f0a6 -- container/skills/warp-research | grep -c .
0                       (须 0 —— 两个 commit 累计在 warp-research 下改动 0 个文件)
```

---

## ② 实测输出

以下全部采于 nanoclaw commit `7ca6b57e64a561351d7108a937b8607a5e87f0a6`（④节 G 的订正）之后，与 ③节锚点同一时刻。
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

🔴 **求值工具必须是 `grep -Fxf`，不能是 `comm -12`——后者在 CJK 上给假阳，且假值会变**

DoD 的判据②在本轮被 Leader 追加了工具钉死条款。**这里给可复现的证据**，
因为验证者按「求集合交集」的直觉最可能选 `comm`，用它会 **reject 一份合格交付**。

两边的 `## ` 标题原样清单（先落成两个文件，三把仪器读同一对输入）：

```
$ grep '^## ' '/Users/zuowei/Obsidian/ClawdVault/Projects/Hestia/PBOC2026年上半年金融数据解读-完整版.md' > /tmp/src-h2.txt
$ grep '^## ' container/skills/warp-hestia/references/methodology.md > /tmp/md-h2.txt

$ cat /tmp/src-h2.txt          # 源文件 12 个
## 一、这节课讲什么，目标是什么？
## 二、中国人民银行金融数据发布的时间及类目
## 三、开讲之前，先把 8 个概念用人话说清楚
## 四、2026 年上半年数据全景
## 五、用数据回答八个社会现实问题
## 六、指标之间的关联性：五条传导链
## 七、怎么用在自己的生活、工作和投资上
## 八、你自己的"每月 15 分钟"检查表
## 九、常见误读与陷阱
## 附录 A：2026 年上半年数据速查卡
## 附录 B：一句话结论
## 附录 C：数据来源与免责声明

$ cat /tmp/md-h2.txt           # methodology.md 5 个
## 八个概念（判读用得上的那一层）
## 八问框架（叙述骨架，逐问写）
## 五条传导链（跨字段关联的提示）
## 误读陷阱
## 写作纪律
```

**三把仪器读同一对输入**：

```
$ comm -12 <(sort /tmp/src-h2.txt) <(sort /tmp/md-h2.txt)
## 九、常见误读与陷阱
## 一、这节课讲什么，目标是什么？
## 五、用数据回答八个社会现实问题
## 六、指标之间的关联性：五条传导链
## 七、怎么用在自己的生活、工作和投资上
$ comm -12 … | wc -l
5                       ← ❌ 假阳

$ grep -Fxf /tmp/src-h2.txt /tmp/md-h2.txt | grep -c .
0                       ← ✅ 真值

$ python3 -c "…集合交集…"
python set 交集条数 = 0 []      ← ✅ 真值（独立复算）
```

🔴 **`comm` 输出的那 5 行，一行都不在 `methodology.md` 里**——把两份清单并排看就一目了然
（它报的全是源文件独有的章节名）。但**只看计数 `5` 是完全合理的**：源文件 12 个、本文件 5 个，
「5 个全重合」在数值上说得通。这就是这类失效最危险的地方。

成因：`comm` 要求两侧按**同一 collation** 排序，CJK 标题不满足该假设 ⇒ 它**不报错、只给垃圾**。
更麻烦的是**同一个坏仪器会给出不同的假值**——Leader 实测 5，另一次子代理跑出 3。
「重跑数变了」容易被读成「有别的东西在变」，而不是「仪器坏了」。

⇒ 判据仍是「交集 ≤ 1」，**求值方式钉死为 `grep -Fxf`**（整行定长匹配、与顺序无关）。

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
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia log --oneline -2
7ca6b57 fix(warp-hestia): Step 3 的 $N 改由 prepare.py --print-name 打印；note-format 点明 ## 计数的两个口径
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

| 文件 | 我写入时字节（**第一轮 `aff2521…`**） | 子代理运行后字节 |
|---|---|---|
| `SKILL.md` | 5259 | **5259** |
| `references/glossary.md` | 5965 | **5965** |
| `references/methodology.md` | 12331 | **12331** |
| `references/note-format.md` | 6219 | **6219** |

且四个文件 mtime 仍是我的写入时刻（11:51 / 11:52 / 11:53 / 11:55）。
⚠️ **上表是第一轮（`aff2521…`）的时点值，不是当前值**——它要回答的是「子代理有没有改动我写的那一版」，只有那个时点的字节才能回答。当前字节（④节 G 的订正之后）见 ①节，`SKILL.md` 5667、`note-format.md` 6727，另两份未变。
**更强的一层**：2.1–2.4 的全部 11 条判据是在子代理运行**之后**复跑一遍的，全部仍然通过——
即便发生了等长度的改写也会被这轮复跑逮住。

---

## ③ 锚点

| 项 | 值 |
| --- | --- |
| nanoclaw worktree **绝对路径** | `/Users/zuowei/workspace/ai/wt-warp-hestia`（**TASK-004 建的，本任务复用，未新建**） |
| nanoclaw 分支 | `feat/warp-hestia` |
| nanoclaw 本任务 commit ① **全 sha** | **`aff2521433eef8567addcedc988fecad3ed08c6f`**（四份文档新建） |
| nanoclaw 本任务 commit ② **全 sha** | **`7ca6b57e64a561351d7108a937b8607a5e87f0a6`**（交付后两处订正，见 ④节 G）= 分支当前 HEAD |
| nanoclaw 本任务的父 commit（= TASK-004 交出时的 HEAD） | `d791101e1defcfd3d3d3fcf7ae32a85ec420547b` |
| nanoclaw 主 checkout 分支 / HEAD 全 sha | `feat/vendor-agent-reach-skill` / `aefea6ce5439cfcf15b3dc54ea9bd507c5846c95` |
| nanoclaw PR 链接 | **无——本任务不开 PR**（见 ④节 A） |
| methodology.md 的源文件 | `/Users/zuowei/Obsidian/ClawdVault/Projects/Hestia/PBOC2026年上半年金融数据解读-完整版.md`（41113 字节） |
| atlas 第一轮 merge commit 全 sha | `1991c49ec5541fd4d4c0d0cd23117b389e7d00d1` |
| atlas 基线 HEAD 全 sha（**本轮**修订写于其上） | `1991c49ec5541fd4d4c0d0cd23117b389e7d00d1` |
| atlas worktree / 分支（第一轮，已拆） | `wt-TASK-005-m3` / `task/TASK-005-m3` @ `c73dbe3b7aabf85df0f31e503d2ca50aa83a8d36` |
| atlas worktree / 分支（**本轮**） | `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-TASK-005-m3b` / `task/TASK-005-m3-fix` |

> 🔴 **worktree 交给下游时是干净的**（`git status --short` 无输出）。
> TASK-006 继续在同一个 worktree 的同一分支上加 `scripts/`，**接着 `7ca6b57e64a561351d7108a937b8607a5e87f0a6` 往下走**；**TASK-007 负责拆**。
>
> ⚠️ **本文件里每个 `file:line` 引用都注明了锚在哪棵树**（nanoclaw worktree / nanoclaw 主 checkout / atlas）。
> 立此规矩的原因是 TASK-004 的验证报告点出过一处漏网：那份文档的表头写「主 checkout `aefea6c…`；worktree `d791101…` **同**」，
> 而两树该文件实际差 9 行——表内两行各自锚在不同的树上，却被一句「同」并成了一个断言。

---

## ④ 未做与理由

### A. 本任务**不开 PR**（DoD non_functional 明写）

需求 TASK-004 的 Step 5 只有 `git add` + `git commit`，没有 `gh pr create`。
DoD 进一步写明「本任务**先不开 PR**（TASK-006 的脚本要进同一个 PR）」。
⇒ ③节的 PR 链接为「无」，这是**预期结果**，不得据此判 rejected。
`feat/warp-hestia` 分支上现在有 **2** 个 commit（`aff2521433eef8567addcedc988fecad3ed08c6f` + `7ca6b57e64a561351d7108a937b8607a5e87f0a6`，后者见 ④节 G），TASK-006 会在其上再加脚本与测试，届时一并开 PR。

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

### G. 交付后按 Leader 裁决做的两处订正（nanoclaw commit `7ca6b57e64a561351d7108a937b8607a5e87f0a6`）

第一轮交付（`aff2521…` + atlas `c73dbe3b…`，已 merge 进 master `1991c49e…`）之后，
Leader 裁决了两件事，都在**本任务内**改完，不留给 TASK-006。

**① `SKILL.md` Step 3 与 `note-format.md` 自相矛盾 —— 改 `SKILL.md`（`3/1` 行）**

原文 Step 3 是 `N=<按 references/note-format.md 的命名规则算出的笔记文件名>`，
而 `note-format.md` 明写「**不要自己拼文件名**，用 `prepare.py --print-name`」。
⇒ 同一份交付里留下了一处**自相矛盾的可执行指令**（不是表述问题）。

改为：

```bash
N=$(python3 /app/skills/warp-hestia/scripts/prepare.py --print-name $Q/processing/$F)
```

并补一句为什么：命名规则有五种 `period_type` 分支、只有 `monthly` 带月份，
拼错会让 Step 5 的 `mode` 跟着判错（`update` 判成 `create`，Spool 拒绝覆盖已存在的笔记）。

**为什么改 `SKILL.md` 而不是 `note-format.md`**：`note-format.md` 那句是 DoD `functional[1]`
明确要求的（「`prepare.py --print-name` 打印同一规则，SKILL 用它取 `$N`」），没错；
是 `SKILL.md` 没接上。**为什么不留给 TASK-006**：`--print-name` 这个接口已经写死在
TASK-006 的 DoD 里，不是本任务替它预设；而留到那时意味着 TASK-005 的验证会先于修复发生，
验证者要对着一份自相矛盾的可执行指令做判断。

⚠️ `prepare.py` 此刻**仍不存在**（TASK-006 才实现）——这与 ④节 B 是同一件事，
Step 3、Step 5 本来就引用了它。本次改动只是让 `$N` 的取法与 `note-format.md` 一致。

**② `note-format.md` 点明 `## ` 计数的两个口径（`4/0` 行）**

Leader 与我在「骨架里有几个 `## `」上报了不同的数（3 与 4），核下来**两个口径都对**：

| 口径 | 值 | 范围 |
|---|---|---|
| 机器区内 | **3** | `<!-- machine-generated: begin -->` → `end` 之间（`## 本期数据` / `## 前 12 期` / `## 信号`）|
| 整份骨架 | **4** | 再加机器区外的 `## 我的批注` |
| **check 行** | **恒 2** | —— |

⇒ 按机器区口径推得 3、按全文口径推得 4，**两个都错，后者错得更远**。
这正是「条数写死为 2」而不是「从文档结构推」的理由，已补进 `note-format.md`
——否则后来者按任一口径去推都会得到错的期望值。

**改动后全部硬性判据复跑，仍全部通过**（见 ②节各小节，那些数字都是订正之后重采的）。

---

# 返工记录（QA REJECT 后的 review_fix，2026-09-09）

> `reason_class = task_defect` —— **这条是我的缺陷**：命令本身写错了。
> 本次改动与 TASK-006 / TASK-007 的返工同在 nanoclaw commit
> **`2a6d39388e4648eb3be0f15b42ee28b52428c5d8`**（三条 fix 一个 commit，但三个任务各走各的状态机）。
> 全部数字采于该 commit 之后。

## F1 · `SKILL.md` Step 3 的 `--existing` 无条件传入【CRITICAL】

### 缺陷与复现

`EXISTING=/workspace/extra/vault/Wiki/Macro/PBOC/$N` 是**无条件赋值**、恒非空，
而 `${EXISTING:+--existing "$EXISTING"}` 的 `:+` 判的是「**变量非空**」不是「**文件存在**」
⇒ **create 场景（每期首次生成，笔记尚不存在）把不存在的路径传进去**：

```
$ python3 prepare.py fixtures/2026-06-h1.json fixtures/2026-06-h1.history.json \
    --existing /nonexistent/note.md
$ echo "EXIT=$?"
--existing 读取失败: [Errno 2] No such file or directory: '/nonexistent/note.md'
EXIT=2          ← stdout 0 字节
```

按 Step 3 自己的失败分支，**每期首次生成都会被移进 `failed/`**。

⚠️ **严重性描述订正**：不是「一份笔记也产不出」，而是「**新笔记一份也产不出，已有笔记的更新正常**」
——update 场景（`$EXISTING` 真的存在）本来就是好的，下面的 2×3 矩阵可证。

### 🔴 修法：显式 `if/else`，**没有**采用 fix_items 建议的 `ARGS=()` 数组

fix_items 建议 `ARGS=(); [ -f "$EXISTING" ] && ARGS=(--existing "$EXISTING")`。
**我实测发现它在 bash 3.2（macOS 自带）配 `set -u` 时会崩**——空数组的 `"${ARGS[@]}"`
被当成 unbound variable（bash < 4.4 的已知行为），而 **create 场景下 `ARGS` 恰恰是空的**：

```
create  new(数组)  bash → new.sh: line 6: ARGS[@]: unbound variable
create  new(数组)  sh   → new.sh: line 6: ARGS[@]: unbound variable
```

⚠️ **我先前只在 zsh 里试过，zsh 不报这个错**——是按 shell 分别验才看见的。
⇒ 改用显式 `if/else`（POSIX，sh / bash 3.2 / zsh 都对）：

```bash
N=$(python3 /app/skills/warp-hestia/scripts/prepare.py --print-name $Q/processing/$F)
EXISTING=/workspace/extra/vault/Wiki/Macro/PBOC/$N
# 🔴 守卫判的是「文件**存在**」，不是「变量非空」——$EXISTING 是无条件赋值、恒非空。
# ⚠️ 刻意用显式 if/else 而不是数组：空数组的 "${ARR[@]}" 在 bash 3.2（macOS 自带）配 set -u
# 时会报 unbound variable，而 create 场景下它恰恰是空的。if/else 在 sh / bash 3.2 / zsh 都对。
if [ -f "$EXISTING" ]; then
  python3 .../prepare.py $Q/processing/$F $Q/processing/$H --existing "$EXISTING" > /tmp/note.md
else
  python3 .../prepare.py $Q/processing/$F $Q/processing/$H > /tmp/note.md
fi
```

Step 5 的 `mode` 判定**同源同判据**，一并改成对文件求值：
`[ -f "$EXISTING" ] && mode=update || mode=create`。

### 🔴 **没有动引号**（fix_items 的 `do_not`）

`${var:+… "$var"}` 的引号本来就是生效的——Leader 用区分性实验证伪了「引号不防词分割」那个说法
（让目标文件存在以屏蔽本缺陷后，bash 与 sh 下都 exit 0 / 3338 字节）。改它是无谓改动，
且会在验证时产生「改前改后一样」的困惑结果。

### 实测：{bash, sh, zsh} × {create, update} 六格，全部带 `set -u`

```
create  bash  → EXIT=0  3247 字节  批注:（手写区，永不被覆盖）
create  sh    → EXIT=0  3247 字节  批注:（手写区，永不被覆盖）
create  zsh   → EXIT=0  3247 字节  批注:（手写区，永不被覆盖）
update  bash  → EXIT=0  3263 字节  批注:这是我手写的第一行批注
update  sh    → EXIT=0  3263 字节  批注:这是我手写的第一行批注
update  zsh   → EXIT=0  3263 字节  批注:这是我手写的第一行批注
```

**六格全绿**，且两场景**行为可区分**（create 走占位批注区、update 保留手写批注）——
若两者输出相同，这个矩阵就只证明了「没崩」而非「守卫在起作用」。

对照组（旧命令，create 场景）：

```
create  old  bash → EXIT=2  0 字节  --existing 读取失败: [Errno 2] ...
create  old  sh   → EXIT=2  0 字节  --existing 读取失败: [Errno 2] ...
update  old  bash → EXIT=0  3263 字节        ← update 场景本来就好
update  old  sh   → EXIT=0  3263 字节
```

### 新增测试（fix_items 的 `verify`）

现有三处 `--existing` 用例**全部指向存在的文件**，所以这个洞从未被行使
——与「一条判据被更早的判据遮蔽」同族：**测试用例的取值分布让某条路径永不发生**。

`test_prepare.py` 新增 `ExistingGuard` 三条：`--existing` 指向不存在的文件 ⇒ exit 2 且 stdout 为空；
create 场景不传 ⇒ 正常产出且批注区是占位文本；update 场景传 ⇒ 批注保留。

⚠️ **unittest 够不到 `SKILL.md` 里的 shell 命令**——那三条钉的是 `prepare.py` 的**契约**
（从而让 `SKILL.md` 那侧的 `[ -f ]` 守卫成为必需）；shell 本身的证据是上面那个六格矩阵。

## 本任务返工的改动清单

```
$ git show --numstat --format='' 2a6d39388e4648eb3be0f15b42ee28b52428c5d8 \
    -- container/skills/warp-hestia/SKILL.md
16	2	container/skills/warp-hestia/SKILL.md
```

## 锚点（返工后）

| 项 | 值 |
|---|---|
| nanoclaw 返工 commit **全 sha** | **`2a6d39388e4648eb3be0f15b42ee28b52428c5d8`** = 分支 HEAD |
| 上一版（QA 判定的对象） | `2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8` |
| nanoclaw 分支 | `feat/warp-hestia`（本地 == 远端 fork） |
| PR | https://github.com/newthinker/nanoclaw/pull/5 — **OPEN**，6 commits，23 files |
| 全套测试 | `Ran 95 tests` / `OK` / EXIT=0（原 84 + 新增 11） |
| 两份 golden | 逐字节未变（独立复算：`13d6e47b…` / `638cb1a5…`） |
