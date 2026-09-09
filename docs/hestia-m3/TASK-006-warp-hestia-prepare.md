# TASK-006（Arcforge）/ TASK-005 前半（需求文档）交付记录：`prepare.py` + 夹具 + 测试

> 承载形态见 `.arcforge/docs/01-design/design-spec.md` §3：本文件**不是摘要，是证据载体**。
> 验证者不以本文件为准——按 ③节给出的全 sha 锚点去 nanoclaw 实跑。
>
> **编号映射**：Arcforge TASK-006 = 需求文档「## TASK-005」节的**前半**（`verify.py` 归 TASK-007）。
> atlas commit subject 用 Arcforge 编号（`docs(TASK-006):`），脚本注释里的里程碑编号用需求编号（`M3 的 TASK-005`）。
> 两处编号不同是**预期的，不是笔误**。
>
> **采样纪律**：本文件所有计数、sha、行数均在**最后一次改动（nanoclaw commit
> `e8d0decaab2a9bcfd29c952ebff11356f63dd3ba`）之后统一重采**，与 ③节锚点同一时刻。
>
> **锚点树标注**：本文件凡出现 `文件:行号` 或 `符号名`，都注明锚在哪棵树
> （atlas 主仓库 / nanoclaw worktree）。立此规矩是因为 TASK-004 验证报告点出过一处漏网：
> 一句「worktree 同」把两个各自锚在不同树上的行并成了一个断言。

---

## ① 改动清单

nanoclaw commit **`e8d0decaab2a9bcfd29c952ebff11356f63dd3ba`**，**17 个文件、+6364 行、0 删除**（纯新建）。

```
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia show --numstat --format='' e8d0decaab2a9bcfd29c952ebff11356f63dd3ba
27	0	container/skills/warp-hestia/scripts/fixtures/existing-2026-06-h1.md
86	0	container/skills/warp-hestia/scripts/fixtures/golden/2023-08-monthly.md
80	0	container/skills/warp-hestia/scripts/fixtures/golden/2026-06-h1.md
433	0	container/skills/warp-hestia/scripts/prepare.py
560	0	container/skills/warp-hestia/scripts/test_prepare.py
（另 12 个 fixtures/*.json：5 期契约 + 5 份侧车 + 1 份修订契约 + 其侧车）
```

| 文件 | 内容 | 对应 DoD |
|---|---|---|
| `scripts/prepare.py` | 十一个模块函数 + `seal_digest`；两级校验；派生指标与 Atlas `Evaluate` 同源 | functional[1][2][3] |
| `scripts/test_prepare.py` | **53 个测试**，标准库 `unittest` | functional[4]、boundary、error_handling |
| `scripts/fixtures/*.json` | **13 个**（5 契约 + 5 侧车 + 修订契约与其侧车 + 手写旧笔记） | functional[0] |
| `scripts/fixtures/golden/*.md` | 2 份**期望输出**，逐字节比对 | functional[4]② |

⚠️ **不写死夹具总数**（DoD reviewer N2 明示）：`ls fixtures/` 实测 **13** 个条目
（12 个 `.json` + 1 个 `existing-2026-06-h1.md`），`fixtures/golden/` 另有 2 个。初稿 DoD 里的「10 个文件」
是把修订契约算漏了。

**`warp-research` 一行未动**（DoD non_functional 明写的约束）：

```
$ git -C /Users/zuowei/workspace/ai/wt-warp-hestia diff --numstat d791101e1defcfd3d3d3fcf7ae32a85ec420547b e8d0decaab2a9bcfd29c952ebff11356f63dd3ba -- container/skills/warp-research | grep -c .
0
```

`__pycache__/` 已从暂存区剔除（首次 `git add` 带进来 2 个 `.pyc`，`git reset` 后删除目录重新暂存；
本仓库 `.gitignore` **没有** `__pycache__` 规则，这一步靠人工，不靠机制）。

---

## ② 实测输出

### 2.1 构建树核对（functional[0]，reviewer G5/B7）

🔴 **DoD 建议的裸 grep 会跨 sprint 假阳**——任务号在各 sprint 复用：

```
$ git -C <atlas 主仓库> log --oneline master | grep -cE '\(TASK-001\)'
32
$ git -C <atlas 主仓库> log --oneline master | grep -cE '\(TASK-002\)'
28
```

⇒ 「两个都在」在这条命令下**恒真**，即便 M3 的那两个不在。用 M3 限定后：

```
$ git log --oneline master | grep -E '\((TASK-001|TASK-002)\): M3'
1c7af81 merge(TASK-001): M3 返工——补侧车失败⇒契约不写出的反方向断言
71a985d test(TASK-001): M3 返工——补侧车失败⇒契约不写出的反方向断言（QA review_fix）
d62ff27 merge(TASK-002): M3 contract emit 同产侧车——回放路径与实时同形
7b38ef9 feat(TASK-002): M3 contract emit 同产侧车——回放路径与实时同形
4c9e7fd merge(TASK-001): M3 history 侧车——ContractHistory + ingest 先侧车后契约 + AST 守卫 34→38
ac1acc5 feat(TASK-001): M3 history 侧车——同类型前 12 期 + 最近 12 个 monthly，先侧车后契约
M3 TASK-001 = 4 条    M3 TASK-002 = 2 条
```

**更硬的是内容判据**（不看提交信息，锚在 **atlas 主仓库**）：

```
internal/hestia/history.go 存在        : yes
func BuildHistory                      : 1 处
func WriteHistory                      : 1 处
cmd/atlas/hestia.go 的 emit 路径调它们   : 2 处
  453:		hist, err := hestia.BuildHistory(ctx, st, obs, c.GeneratedBy)
  457:		if _, err := hestia.WriteHistory(cfg.Queue.Dir, hist); err != nil {
```

**构建时 atlas 全 sha = `1c7af81846a4fba3fbb76a84e42eb232b8178f81`**（分支 `master`）。

侧车抽查（证明 TASK-001 + TASK-002 确实生效，不是「文件存在」这种弱证据）：

```
$ jq '{schema_version, for, generated_by, same_type: (.same_type|length), monthly_recent: (.monthly_recent|length)}' fixtures/2026-06-h1.history.json
{
  "schema_version": "1.0",
  "for": "2026-06-h1",
  "generated_by": "contract@v1/replay",
  "same_type": 5,
  "monthly_recent": 12
}
```

### 2.2 🔴 修订夹具：路径 (a) 实测**不可行**，走的是 (b)

DoD 原文：「218 篇语料里是否真有 **无人验证过**，不要假定」。**现在验证了——没有**：

```
$ sqlite3 <临时库> "SELECT period, period_type, COUNT(DISTINCT published_at) n
                    FROM hestia_observations GROUP BY period, period_type HAVING n > 1;"
（无输出）
$ sqlite3 <临时库> "SELECT COUNT(*) FROM (SELECT period, period_type FROM hestia_observations
                    GROUP BY period, period_type HAVING COUNT(DISTINCT published_at) > 1);"
0
$ sqlite3 <临时库> "SELECT COUNT(*) FROM (SELECT period, period_type FROM hestia_observations GROUP BY period, period_type);"
76
$ sqlite3 <临时库> "SELECT COUNT(DISTINCT article_id) FROM hestia_observations;"
76
```

**76 观测 / 76 期次 / 76 个 article_id，同期两版的期次 0 个。** ⇒ 只能走 (b)。

照 `TestHestiaContractEmitRevisionPeriod`（符号名；实测在 **atlas 主仓库** 的
`cmd/atlas/hestia_test.go:1701`，**DoD 里写的 1570 已漂**——正是 DoD 说的「行号必然会漂，用符号名自己 grep」）
的配方，写了个一次性 Go 程序 `cmd/m3seedrev`：读原契约 54 个 `data` 值作底、改 `m2`=332.0 / `m1`=112.9，
`Save` 一条 `2025-12/annual` + `published_at=2026-02-20` + `article_id=2026022009294440746` 的观测：

```
saved: {Verdict:Revision Table:hestia_observations}  values=54
$ sqlite3 <临时库> "SELECT period,period_type,published_at,article_id FROM hestia_observations
                    WHERE period='2025-12' AND period_type='annual' ORDER BY published_at;"
2025-12|annual|2026-01-15|2026011509294440745
2025-12|annual|2026-02-20|2026022009294440746
```

再 emit 到**独立队列目录**（不覆盖原契约）：

```
$ jq '{period,period_type,published_at,article_id,is_revision,supersedes_published_at}' fixtures/2025-12-annual-rev.json
{
  "period": "2025-12",
  "period_type": "annual",
  "published_at": "2026-02-20",
  "article_id": "2026022009294440746",
  "is_revision": true,
  "supersedes_published_at": "2026-01-15"
}
原版 sha256 = 9084c2fd58fc3ee9517c0f2ba2153b152e26a0a56720b25226f8d6f4d4fd4118
修订 sha256 = 3af69ad78ba5250054d776e9cf17f19c655f5fc4425a93d741bd521901a44ac7
```

**一次性程序用完已删**，atlas 工作区无残留（`git status --short` 只剩本 sprint 既有的 `.arcforge/` 条目）。

### 2.3 TDD 红阶段留痕（error_handling 要求）

实现前跑，`prepare.py` 不存在：

```
$ cd <worktree>/container/skills/warp-hestia/scripts && python3 -m unittest -v 2>&1 | tail -5
Ran 44 tests in 0.508s
FAILED (failures=3, errors=40)
$ echo "EXIT=$?"
EXIT=1
```

（当时 44 个测试：40 ERROR + 3 FAIL 红；另 5 个通过的是 `FixturesShape` 那组，它们只验夹具本身、
本就不依赖 `prepare.py`——**如实记下，不粉饰成「全红」**。）

### 2.4 最终测试（functional[4]）

```
$ cd <worktree>/container/skills/warp-hestia/scripts && python3 -m unittest -v 2>&1 | tail -3
Ran 53 tests in 1.397s

OK
$ echo "EXIT=$?"
EXIT=0
$ grep -c 'def test_' test_prepare.py
53
```

`Ran 53` 与 `grep -c 'def test_'` = **53**，两个口径一致。

### 2.5 只用标准库（functional[1]）

```
$ grep -n '^import \|^from ' container/skills/warp-hestia/scripts/prepare.py
14:import argparse
15:import hashlib
16:import json
17:import re
18:import sys
```

五个全是标准库，无第三方依赖。`--print-name` 输出（无月份版，TASK-005 的 G6b 裁决）：

```
$ python3 prepare.py fixtures/2026-06-h1.json fixtures/2026-06-h1.history.json --print-name
2026 上半年金融数据解读.md
```

### 2.6 两级校验（functional[3]）

对 `2026-06-h1` 的实际输出：分段 `<!-- check: -->` **恰 2 条**、封条 `<!-- seal: -->` **恰 1 条**，
而**机器区内 `## ` 标题 3 个**（`## 本期数据` / `## 前 12 期` / `## 信号`）。

🔴 **3 ≠ 2 就是「不能按 `## ` 标题数推 check 行数」的实证**——照 spec 正确实现的笔记，
若用「标题数 == check 数」判据会当场判红。测试 `test_heading_count_differs_from_check_count`
把这个差钉住了。

`## 信号` 段**不发** check（`test_signal_section_has_no_check`）；
封条**扣除** narrative 块（`test_seal_excludes_narrative`：在 narrative 里插入带 `## ` 的文本，封条不变）；
删掉一条 check 行封条必变（`test_seal_catches_deleted_check_line`）。

### 2.7 两张表（functional[3]）

`2026-06-h1` 的本期表（五列，三个值列）：

```
| 指标 | 单位 | 本期 2026-06（口径 2025-01） | 上期 2025-06 | 去年同期 2025-06 |
```

`2023-08-monthly` 的本期表——**口径标注真的触发**（去年同期口径与本期不同）：

```
| 指标 | 单位 | 本期 2023-08（口径 2023-01） | 上期 2023-07 | 去年同期 2022-08 ⚠️口径 2015-01 |
```

🔴 **前 12 期表一律取侧车的 `same_type`**。这条是实现时撞出来的真缺陷：我最初写成
「monthly 走 `monthly_recent`、其余走 `same_type`」，结果 `2023-08-monthly` 的前 12 期表**变成 0 行**——
因为 **`monthly_recent` 在 `period_type == monthly` 时被 Atlas 侧刻意省略**（`omitzero`，
见 **atlas 主仓库** `internal/hestia/history.go:20` 与其后的注释段）：对 monthly 契约，
`same_type` 本身就是月度序列。实测各夹具：

| 夹具 | `same_type` | `monthly_recent` |
|---|---|---|
| `2023-08-monthly` | 12 | **0** |
| `2022-07-monthly` | 12 | **0** |
| `2026-06-h1` | 5 | 12 |
| `2020-06-h1` | 0 | 4 |
| `2025-12-annual` | 5 | 12 |

⚠️ **这个缺陷不会报错，只会静默给出 0 行表。** 已由
`test_history_table_uses_same_type_even_for_monthly` 钉住（断言 monthly 的前 12 期表恰 12 行）。

### 2.8 派生分支全覆盖（functional[4]④，reviewer U7）

| 分支 | 断言 | 值 |
|---|---|---|
| `_mom` 优先（夹具） | `test_mom_preferred_2023_08` | `hh_mlt_monthly == 1602` |
| `_mom` 优先（夹具） | `test_mom_preferred_2022_07` | `1486` —— **需求自标的订正**：该期贷款分部门是 `_mom`，走的**不是** `_ytd ÷ MM` |
| `_ytd ÷ MM` | `test_ytd_divided_by_month_number` | 内联最小契约 `loan_hh_mlt_ytd: 7000` + `2022-07` ⇒ **1000** |
| `÷ 3` / `÷ 9` | `test_divided_by_three_and_nine` | 内联 `9000` ⇒ 3000 / 1000 |
| `÷ 6` | `test_divided_by_six` | **368.67**（2212/6）+ `signal_housing: red` + `signal_credit: green` |
| `÷ 12` | `test_divided_by_twelve` | `2025-12-annual` 的 `loan_hh_mlt_ytd / 12` |
| 剪刀差 | `test_scissors` | **−4** |

三期 golden 温度（`test_three_periods`）：`2020-06-h1` → **2**、`2025-12-annual` → **0**、
`2026-06-h1` → **1**，`temp_known` 恒 **4**。手算复核：

| 夹具 | scissors | 活化 | 楼市 | 消费 | 信贷 | temp |
|---|---|---|---|---|---|---|
| 2020-06-h1 | 6.5−11.1=−4.6 | red | 28000/6=4666.67 green | 7552/6=1258.67 green | 9697/87700=11.06% yellow | **2/4** |
| 2025-12-annual | 3.8−8.5=−4.7 | red | 12800/12=1066.67 red | −8351/12=−695.92 red | 16600/154700=10.73% yellow | **0/4** |
| 2026-06-h1 | 4−8=−4 | red | 2212/6=368.67 red | −5881/6=−980.17 red | 8143/111300=7.32% green | **1/4** |

### 2.9 🔴 golden 逐字节比对（functional[4]②，reviewer O2）

`test_golden_files_byte_exact` 与入库的期望文件**逐字节**比对：

```
fixtures/golden/2026-06-h1.md       80 行  sha256=13d6e47bd775bd3d3b4bfcd60ffc40975d648b8e33dcebb5598ef80ac6f6bfc7
fixtures/golden/2023-08-monthly.md  86 行  sha256=638cb1a52f585d99d101c741af92a0a9324ef5ca04ceba2d9ab687a9811d1586
```

`test_deterministic`（自比）**保留但不替代它**——自比在「表乱序 / 表头错 / 少一列」时全绿。
证据见 2.10 的变异 M13（截断表到 3 行）与 M22（数值格式全改）：**两者自比都通过，是 golden 把它们杀死的**。

**期望文件的生成与申报**：先跑通全部**非 golden** 断言 → 人工逐行读了两份输出（表结构、口径标注、
校验行位置、批注区）→ 才把输出入库为期望。理由与「更新期望必须申报」已记进 discovery `decisions`。

### 2.10 变异测试：23 个变异全部 KILLED，0 存活

⚠️ **变异只在 `mktemp -d` 隔离副本上跑**（CLAUDE.md 纪律：就地变异会让并发者读到变异态）。
每轮校验主工作区 `prepare.py` 的 sha256 与 `git status --porcelain` 指纹，**每轮均未变**。
每个变异体落盘后过 `ast.parse` 语法闸（M3 首次尝试即被语法闸拦下，**未记 KILLED**）。

| 变异 | 内容 | 结果 |
|---|---|---|
| M1 | 前 12 期改取 `monthly_recent` | ✅ 红 2 |
| M3 | 给 `## 信号` 段也发 check（3 条） | ✅ 红 5 |
| M4 | `bill_ratio` 允许跨口径配对 | ✅ 红 1 |
| M5 | `months_in_period` 越界不返回 0 | ✅ 红 2 |
| M6 | `temp_known` 恒为 4 | ✅ 红 1 |
| M7 | 修订不写 `supersedes_published_at` | ✅ 红 1 |
| M9 | `extract_annotations` 无标题时抛错 | ✅ 红 1 |
| M10 | 月数为 0 时照除（÷0） | ✅ 红 1 |
| M11 | `note_name` 给 h1 带月份 | ✅ 红 5 |
| M12 | frontmatter 写入 `reviewed`/`source` | ✅ 红 3 |
| M13 | 前 12 期表截断到 3 行 | ✅ 红 4 |
| M14 | 去掉口径标注 | ✅ 红 2 |
| M15 | seal 整行去掉 | ✅ 红 6 |
| M16 | check 摘要不含 `## ` 标题行 | ✅ 红 4 |
| M17 | `_pick` 改 `_ytd` 优先 | ✅ 红 1 |
| M18 | `monthly_average` 改 `_ytd` 优先 | ✅ 红 1 |
| M19 | `same_caliber_pair` 改 `_ytd` 优先 | ✅ 红 1 |
| M20 | 剪刀差算反（m2−m1） | ✅ 红 7 |
| M21 | 活化 `>=` 改 `>` | ✅ 红 1 |
| M22 | `fmt` 一律两位小数 | ✅ 红 5 |
| M23 | 活化 `<=` 改 `<` | ✅ 红 1 |
| M24 | 楼市/消费 `>=` 改 `>` | ✅ 红 1 |
| M25 | 信贷 `<` 改 `<=` | ✅ 红 1 |
| M26 | 信贷 `>=` 改 `>` | ✅ 红 1 |
| M27 | `total==0` 不判 unknown | ✅ 红 1 |

🔴 **其中 4 个是先存活、补断言之后才被杀死的真实缺口**——这是本节最有价值的部分：

| 存活的变异 | 为什么当时杀不死 | 补了什么 |
|---|---|---|
| **M4** 跨口径配对 | 唯一用到 `bill_ratio` 的断言走 `2026-06-h1`，而它两个字段**都是 `_ytd`** ⇒ 配对顺序怎么改结果都一样 | `test_bill_ratio_rejects_cross_caliber`（构造只有一侧是 `_mom` 的输入，正反两个方向）+ `test_bill_ratio_total_zero_is_unknown` |
| **M17/M18/M19** `_mom` 优先 | 夹具里**每个字段只有 `_mom` 或 `_ytd` 之一**，「优先」这条语义在夹具上**根本行使不到** | 三条「两者都在」的断言：`test_mom_beats_ytd_when_both_present` / `test_pick_prefers_mom_when_both_present` / `test_same_caliber_pair_prefers_mom_when_both_present` |
| **M14** 去掉口径标注 | 只被 golden 逐字节比对杀死，**专职的 `test_current_table_has_caliber_annotation` 没响**——它断言的是「出现了口径字样」，而表头恒含「本期 …（口径 X）」 | `test_caliber_mark_fires_when_calibers_differ`（口径不同必须带 ⚠️）+ `test_caliber_mark_absent_when_calibers_match`（相同则不该有，否则标注成噪声） |
| **M21** `>=` 改 `>` | 只在**恰好等于阈值**时有别，而没有夹具落在边界上 | `test_signal_thresholds_at_exact_boundary`：四个信号的临界值逐个钉住（含「绿是严格小于 healthy」「恰为 healthy 是黄」） |

### 2.11 边界与错误路径

```
$ python3 prepare.py fixtures/2026-06-h1.json /nonexistent.json ; echo "EXIT=$?"
history 侧车读取或解析失败: [Errno 2] No such file or directory: '/nonexistent.json'
EXIT=2
```

stderr 含 `history`、退出码 **2**（`test_missing_history_exit_2`）；契约 JSON 解析失败同样非零
（`test_bad_contract_json_nonzero`）。

其余边界：B1 `months_in_period` 解析失败返回 0 且**不拿 0 做除数**；
B2 `ok=False` ⇒ 信号 `unknown` 且 `temp_known < 4`（实测 `known == 1`）；
B6 旧笔记无 `## 我的批注` ⇒ 视为空不报错；O3 修订路径（`supersedes_published_at` + 「本文取代 2026-01-15 版本」）；
批注两行原样保留 + **七个标记**齐全。

### 2.12 code-simplifier（全局规范 + DoD non_functional）

跑了。**它的报告是一句无意义的字符串（`No action. Twelfth identical fire — unchanged.`），
但 `diff` 显示它确实改了两个文件** ⇒ **报告与事实相反，按 DoD「子代理回复不可采信，以 diff 为准」核实**。

实际改动（都在约束内，我逐条看过 diff 后接受）：

- `prepare.py`：把重复定义两次的信号图标提成模块常量 `EMOJI`；`x in values and values[x] is not None`
  简化为 `values.get(x) is not None`；`fmv` → `fields`；`_pick` 用 `dual` 变量表达「该字段有无口径之分」。
- `test_prepare.py`：把散在 20 处的 `import prepare as P` 提到模块级；抽出 `load_history()` /
  `section()` / `header_cells()` 三个 helper。

约束逐条核实：

```
golden 两份 sha256                未变 ✓（13d6e47b… / 638cb1a5…）
def test_ 计数                    53 → 53 ✓
prepare.py 的 import 行           5 行，全标准库 ✓
函数名与签名                       未变 ✓
python3 -m unittest              Ran 53 / OK / EXIT=0 ✓
```

🔴 **更强的一层**：简化改了 `monthly_average` 与 `_pick` 的代码形状，我的变异锚点随之失效
⇒ **把 23 个变异按新形状全部重跑了一遍**（2.10 的表就是简化**之后**的结果），
仍是 **KILLED 23 / SURVIVED 0**。等长度改写或语义走样都会被这一步逮住。

---

## ③ 锚点

| 项 | 值 |
| --- | --- |
| nanoclaw worktree **绝对路径** | `/Users/zuowei/workspace/ai/wt-warp-hestia`（TASK-004 建，本任务复用，**未新建**） |
| nanoclaw 分支 | `feat/warp-hestia` |
| nanoclaw 本任务 commit **全 sha** | **`e8d0decaab2a9bcfd29c952ebff11356f63dd3ba`** = 分支当前 HEAD |
| nanoclaw 本任务的父 commit（TASK-005 交出时） | `7ca6b57e64a561351d7108a937b8607a5e87f0a6` |
| nanoclaw 分支起点（TASK-004 交出时） | `d791101e1defcfd3d3d3fcf7ae32a85ec420547b` |
| nanoclaw 主 checkout 分支 / HEAD | `feat/vendor-agent-reach-skill` / `aefea6ce5439cfcf15b3dc54ea9bd507c5846c95`（**未触碰**） |
| nanoclaw PR 链接 | **无——本任务不开 PR**（TASK-007 统一开，含 005/006/007 三批提交） |
| **构建夹具时** atlas 全 sha | **`1c7af81846a4fba3fbb76a84e42eb232b8178f81`**（分支 `master`） |
| atlas 基线 HEAD（本文件写于其上） | `9ce20f3120ffbfcb679a0d5c30fe1a80c71893d6` |
| atlas worktree / 分支 | `/Users/zuowei/workspace/go/src/github.com/newthinker/wt-TASK-006-m3` / `task/TASK-006-m3` |

> 🔴 **给 TASK-007 的验证者**：`verify_baseline` **只锚 atlas，够不到 nanoclaw**。
> 请在判定前后各记一次 `git -C /Users/zuowei/workspace/ai/wt-warp-hestia rev-parse HEAD`，
> 与上表的 `e8d0decaab2a9bcfd29c952ebff11356f63dd3ba` 比对；不一致就当漂移处理（报 Leader，别自行决定验哪一版）。
> ⚠️ **worktree 的工作树默认就是最新版**，照命令裸跑只会量到当前版，量不出「判的是不是同一棵树」。

---

## ④ 未做与理由

### A. `verify.py` **不实现**——它是 TASK-007 的产物

需求「## TASK-005」节同时给了 `prepare.py` 与 `verify.py`，但 Arcforge 侧
**reviewer 反审后把它拆成了 TASK-006（`prepare.py`）+ TASK-007（`verify.py`）**——
初稿一个任务装不下（5 条实质业务义务零覆盖 + 5 条边界缺口，8 条 DoD 放不下）。

⇒ 需求原文 `test_prepare.py` 里的 `class Verify`（三条：`test_accepts_untouched` /
`test_rejects_edited_table` / `test_narrative_edit_ok`）**本任务不写**，归 TASK-007。
`ls scripts/verify.py` 不存在**不是缺陷**。

⚠️ 但 `prepare.py` 已经把 `verify.py` 需要的两个函数备好并测过：
`seal_digest(md)`（封条重算）与 `with_check(section)`（分段摘要），TASK-007 直接 `import prepare` 复用，
**不要另写一份**——两份实现必然漂。

### B. 本任务**不开 PR**

DoD non_functional 明写「TASK-007 统一开，含 005/006/007 三个任务的提交」。
⇒ ③节 PR 链接为「无」，**不得据此判 rejected**。`feat/warp-hestia` 分支上现有 3 个 commit
（`aff2521…` 四份文档 → `7ca6b57e…` 两处订正 → `e8d0deca…` 本任务）。

### C. `examples/2026-06-h1.md` 不建

spec §5.1 目录树里有它，但注明是「冒烟产物经人审后回填」，属需求 TASK-006（**人执行**）范围。

### D. 🔴 产物在冒烟时看不见，需两步人执行动作（结转）

`container-runner.ts` 的 `projectRoot = process.cwd()`（**nanoclaw worktree** 的 `src/container-runner.ts:273`），
skills 从 `<projectRoot>/container/skills` bind mount 到 `/app/skills` ⇒ **容器只看得见主 checkout 工作树里的 skill**。
冒烟前必须 ①nanoclaw PR 合并到 `main` ②本机 checkout 切回 `main`——**两步都是人执行**（TASK-008 §D）。

⚠️ **不影响本任务交付**：开发与 `python3 -m unittest` 都在 worktree 里跑，不经容器。
**刻意没有**为了让容器看见而切主 checkout 的分支或把文件拷进去——那会动到别人未提交的改动
（主 checkout 现有 2 改 2 删）。

### E. `__pycache__` 未加进 `.gitignore`

首次 `git add` 把 2 个 `.pyc` 带进了暂存区，已 `git reset` + 删目录后重新暂存，最终提交 **17 个文件、0 个 `.pyc`**。
**没有改 `.gitignore`**——那是 nanoclaw 仓库级配置，超出本任务范围（`writes` 只声明了 atlas 的交付文档）。
⇒ 留给 TASK-007 或人：`container/skills/warp-hestia/scripts/__pycache__/` 会在每次跑测试后重新生成，
提交前需手工排除。这是**已知的、会复发的**小坑，写在这里免得下一个人重踩。

### F. 数值格式与 Atlas `fmtNum` 不强求一致

需求原文明写「与 Atlas `fmtNum` 的『最短精确』不强求一致——脚本输出的是给人看的表」。
本实现：整数打整数（`2212`、`1602`、`-4`），非整数保留两位（`368.67`、`7.32`）。
golden 逐字节比对已把这套格式钉死；变异 M22（一律两位小数）被 5 条断言杀死。

---

# 返工记录（QA REJECT 后的 review_fix，2026-09-09）

> 🔴 `reason_class = **dod_defect**` —— **这是 DoD 的缺口，不是我的缺陷**：
> TASK-006 的 `done_criteria` 从未要求配对校验，我实现的是被要求的东西。Leader 已明确记此归属。
> 本次改动与 TASK-005 / TASK-007 的返工同在 nanoclaw commit
> **`2a6d39388e4648eb3be0f15b42ee28b52428c5d8`**。全部数字采于该 commit 之后。

## F2 · 契约与侧车的配对无校验【HIGH】

### 缺陷与复现

`prepare.py` 从 history 对象上**只读 `same_type` 一个键**；侧车顶层的 `for` 字段
（atlas `history.go` 写出，值形如 `2026-06-h1`）**被读 0 次**：

```
$ grep -c '\["for"\]\|\.get("for")' prepare.py
0
$ jq -r '.for' fixtures/2025-12-annual.history.json
2025-12-annual
```

⇒ 错配的一对喂进去，**prepare 与 verify 双双 exit 0**：

```
$ python3 prepare.py fixtures/2026-06-h1.json fixtures/2025-12-annual.history.json --now 2026-09-12
$ echo "EXIT=$?"                              → EXIT=0，3161 字节
$ python3 verify.py <那份笔记>
$ echo "EXIT=$?"                              → EXIT=0
```

**笔记自洽地看起来完全正常**——frontmatter / 标题 / 四信号 / 温度全对（都来自契约），
而**两张表全错**（都来自侧车）：

```
period: 2026-06        period_type: h1        ← 契约说这是 2026 上半年
前 12 期表的期次：2024-12 / 2022-12 / 2021-12 / 2020-12 / 2019-12   ← 全是 annual
```

🔴 **污染面不止「前 12 期」表**（fix_items 的 `note_scope`，我实测确认）：
`render_table_current` 的上期/去年同期两列同样取自 `same_type`（`series[0]` 就是上期）
⇒ **本期数据表的对比列也错**，而那是解读的起点：

```
| 指标 | 单位 | 本期 2026-06（口径 2025-01） | 上期 2024-12 ⚠️口径 2023-01 | 去年同期（无） |
                                              ↑ 错配后「上期」变成了 annual 期次
```

### 修法

新增 `pair_key()` 与 `assert_pair()`，在 `main()` 读完两份 JSON 之后立刻校验：

```python
history["for"] == contract["period"] + "-" + contract["period_type"]
```

不成立 ⇒ **exit 2** 并打印两边的值。这是这条链路上**唯一能机器判定「这两个文件是一对」的事实**。

🔴 **判据是期次，不是文件名**：修订夹具叫 `2025-12-annual-rev.*` 而它的 `for` 是 `2025-12-annual`
——拿文件名做判据会**误拒**这一对。已由 `test_revision_fixture_pairs_by_period_not_filename` 钉住。

### 实测（返工后）

```
$ python3 prepare.py fixtures/2026-06-h1.json fixtures/2025-12-annual.history.json --now 2026-09-12
契约与侧车不是一对：契约是 2026-06-h1，而侧车的 for 是 '2025-12-annual'。
两张表全部取自侧车，错配会产出「自洽但表全错」的笔记（frontmatter 与信号来自契约、两张表来自侧车），故拒绝。
EXIT=2          ← stdout 0 字节，不产出半份笔记
```

**正配一条不红**（五期 + 修订夹具）：

```
2020-06-h1         EXIT=0
2025-12-annual     EXIT=0
2026-06-h1         EXIT=0
2023-08-monthly    EXIT=0
2022-07-monthly    EXIT=0
2025-12-annual-rev EXIT=0   ← for=2025-12-annual，文件名带 -rev，判据是期次不是文件名
```

### 新增测试

`PairingGuard` 四条（错配 exit 2 且报错含两边的值、正配五期不受影响、修订夹具按期次配对、
`assert_pair` 存在）+ `Tables.test_current_table_comparison_columns_come_from_same_type`
一条——**后者钉的正是「污染面不止那张表」**：断言本期数据表的「上期」列必须取自 `same_type` 的最新一期。

### 变异（本轮针对本条的三个）

| 变异 | 内容 | 结果 |
|---|---|---|
| R2a | 配对校验整个不做 | ✅ KILLED 红 1 |
| R2b | 配对判据换成文件名（修订夹具会被误拒） | ✅ KILLED 红 1 |
| R2c | 配对不匹配时只警告不退出 | ✅ KILLED 红 1 |

## 本任务返工的改动清单

```
$ git show --numstat --format='' 2a6d39388e4648eb3be0f15b42ee28b52428c5d8 \
    -- container/skills/warp-hestia/scripts/prepare.py
38	0	container/skills/warp-hestia/scripts/prepare.py
```

## 锚点（返工后）

| 项 | 值 |
|---|---|
| nanoclaw 返工 commit **全 sha** | **`2a6d39388e4648eb3be0f15b42ee28b52428c5d8`** = 分支 HEAD |
| 上一版（QA 判定的对象） | `2d2fbe80947970c117b49fdabf0d3f2a3e8eefc8` |
| PR | https://github.com/newthinker/nanoclaw/pull/5 — OPEN，6 commits，23 files |
| 全套测试 | `Ran 95 tests` / `OK` / EXIT=0 |
| 两份 golden | 逐字节未变（独立复算 `13d6e47b…` / `638cb1a5…`）——配对校验只在错配时触发，正配路径一个字节没动 |
