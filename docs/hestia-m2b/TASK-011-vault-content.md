# TASK-011 · vault 回写内容（供人粘贴）

> **这份文件不会自动写进 vault。** 按 AD-M2b-2，dev 不直接写 vault——下面两段是**可直接粘贴**
> 的成品，由人拷进对应笔记。每段都给了「找哪一行 / 换成什么」，不需要再改写。
>
> 全部数字由 dev-m2b-c 于 2026-09-17 在 master `477664a5651449490ddc602c090501bfd2ab9ead`
> 上实跑，非转抄。测量方法写在 `atlas/internal/hestia/CONTRACTS.md` 的 `## Sprint M2b-2`。

---

## → Projects/Hestia/README.md

### 动作一：替换「落地状态」表里的 M2b 行

**找这一行**（目前它是全表唯一还写着「未开始」的 M2b 行）：

```
| M2b | Sheets 投影：google api 依赖、Service Account、按年分表、首写前逐格对账、回填 80 期投影 | 2–3 天 + 对账 | 未开始。**前置 spike**：用凭据读一次表头——M0 没记列布局 |
```

**换成**：

```
| **M2b** | Sheets 投影：`internal/hestia/sheets` 子包（表头解析 / 三类 diff / API 薄壳 / push 编排 / 建缺失年度表）+ `atlas hestia sheets push` 子命令 + ingest 入库后自动投影 | 前置 spike 0.5 天 + 2 天 | ✅ **代码已完成·Arcforge Sprint M2b（2026-09-17）**：全仓 **65 个包 ok / 0 FAIL**；覆盖率 `internal/hestia` **96.6%**、`internal/hestia/sheets` **89.1%**、`cmd/atlas` **78.0%**（该包历史水位 76.8%，本 sprint 抬高 1.2pp，任务级 floor 77）；导出面 **+13**（`sheets.*` 11 项 + `BuildSheetRows` + `Store.AllPeriods`，**零移除**），两条写口守卫 `want` 由 14/38 变为 **15/51** 且仍是精确集合相等。**spec §10 判据一～三已过**（见 CONTRACTS `## Sprint M2b-2` ⑥），**判据四～七要对真表真网跑，待人执行**（[spec](superpowers/specs/2026-09-16-hestia-m2b-sheets-design.md) / [plan](superpowers/plans/2026-09-16-hestia-m2b-sheets.md)） |
```

### 动作二：把「下一步」那段的 M2b-2 措辞更新

**找这一段**（第 34 行附近，以「**下一步**：M2b-2 Sheets 写入。」开头）：

```
**下一步**：M2b-2 Sheets 写入。**M2b-1 已完成**（2026-09-16）：凭据就绪、spike 跑通、列布局已记，见 [[M2b-Sheets-接入手册]]。
```

**换成**：

```
**M2b-2 代码已完成**（2026-09-17），投影已接上：`atlas hestia sheets push`（默认 dry-run，`--apply` 才写）+ ingest 入库后自动投影。**M2b-1**（2026-09-16）的凭据与列布局见 [[M2b-Sheets-接入手册]]。
```

**并把同段末尾那句「M2b-2 动工前有**三条待决**……」整段替换为**：

```
M2b-2 动工前的**三条待决已全部实现**（不只是结案）：同月双记录 monthly 优先、楼市黄灯不碰信号列、缺失年度表复制 2024年。实现落点见 [[M2b-Sheets-接入手册]] §七与 CONTRACTS `## Sprint M2b-2`。

**下一步是人执行的真表验收**：spec §10 的判据四～七（真表 dry-run 三类计数、`--apply --create-sheets` 建 5 张新表、再跑一次 dry-run 验幂等、真实 ingest 后断网重试）。判据六是幂等的唯一诚实证明；判据七的后半段**必须真的断一次网，读代码不算**。
```

---

## → M2b-Sheets-接入手册.md

### 动作三：新增一节「投影已上线」

**插在 §七（「三条待决已结案」）之前**，即紧跟 §六「那一行的对账结果：35/35 全中」之后：

```markdown
## 六·五、投影已上线（2026-09-17）

M2b-2 代码已完成。从这一节起，本手册前面描述的「将来会怎么做」都已经是**现在的行为**。

**两条投影路径**

| 路径 | 触发 | 默认行为 |
|---|---|---|
| `atlas hestia sheets push --all`（或 `--period YYYY-MM --period-type TYPE`） | 人手动，巡检用 | **dry-run**，一个写请求都不发；`--apply` 才写，`--create-sheets` 才建缺失年度表 |
| ingest 入库后自动投影 | 每次 `atlas hestia ingest` 成功入权威表 | 固定 `Apply` + `CreateSheets`（新年份第一期必缺表，不建就整批失败） |

**dry-run 输出**三类明细行各自报类别（将写 / 一致跳过 / 库缺跳过），末尾三类计数分三行，
再加一句 `dry-run：未写入任何内容；确认后加 --apply`。缺表时会多一行「将新建工作表」。
三类分开显示不是排版偏好——合并成一类就看不出「这格没动」是因为已经对了还是因为库里没数，
而这两种情况的后续动作完全不同。

**没配凭据时是什么行为**：`hestia_sheets.credentials_file` 留空 = **能力禁用**。
ingest 不尝试投影、不报错、不打日志；CLI 打印 `hestia_sheets 未配置（credentials_file 留空）`
并**退出 0**——没配不是故障，退非零会让巡检脚本把它报成出错。

🔴 **投影失败不会挡住入库**。这是刻意的：契约写失败仍进错误链（M3 在等它），而投影没有下游、
按 ADR-0004 可再生，下次 ingest 或一次 `sheets push` 就补上了。所以投影失败时你看到的是
stdout 上一行 `sheets: 投影失败（不影响入库）: …`，而 run outcome 仍是 `ingested`、退出码仍是 0、
**不发 Telegram**。发现漏推靠 `atlas hestia sheets push --all`（默认 dry-run）巡检，
不攒「上次投影到哪」的状态文件——那份状态自己会过期。
连第三方客户端的 panic 也被转成普通错误走这条路（`defer recover()`，不是吞掉，原始 panic 值留在错误里）。

⚠️ **配置落位仍有一个未决项**：`hestia_sheets` 这个键实际由 `hestia.LoadConfig` 从
`hestia.config_path` 指向的文件（默认 `configs/hestia.yaml`）读，**不是**从 `configs/config.yaml` 读；
而 `configs/hestia.yaml` 进 git 且不在 `deploy.sh` 的 rsync 排除表里，运行时那份每次部署都会被覆盖。
填错位置**没有任何反馈**（主配置对未知顶层键不报错）。动手配之前先看 CONTRACTS
`## Sprint M2b-2` 的「未决项」一条，那里列了三种出路。

实现细节与全部实测数字见 `atlas/internal/hestia/CONTRACTS.md` 的 `## Sprint M2b-2`。
```

### 动作四：§七 标题与三条结论改「已实现」

**§七 的标题**，找：

```
## 七、三条待决已结案（2026-09-16）
```

**换成**：

```
## 七、三条待决已实现（结案 2026-09-16，实现 2026-09-17）

> 2026-09-17 更新：下面三条不再只是「决定」，都已落成代码并有测试守着。
> 实现落点与实测见 `atlas/internal/hestia/CONTRACTS.md` 的 `## Sprint M2b-2`。
```

**D1 标题**，找 `### D1｜同月双记录：**monthly 优先**`，在该小节正文末尾追加：

```
**已实现**（2026-09-17）：收敛发生在 `selectRows`（`internal/hestia/sheets_project.go`）——
每个 period 恰收敛成一条，有 monthly 用 monthly，否则取 `published_at` 最新的累计期次。
实测 77 条记录 → **61 行**，差的 16 条正是与同月 monthly 撞行的累计记录，由
`TestSelectRowsCollapsesSeventySevenToSixtyOne` 钉住。见 CONTRACTS `## Sprint M2b-2` ③。
```

**D2 标题**，找 `### D2｜楼市黄灯：**先只投影数值，信号列不管**`，在该小节正文末尾追加：

```
**已实现**（2026-09-17）：程序只写录入区 A–AI（`entryLastCol = 34`），计算区 AJ–BB 的 19 个公式
一个都不碰——这就是约束 C6，不是特例。见 CONTRACTS `## Sprint M2b-2` 的 C1–C11 落地对照表。
```

**D3 标题**，找 `### D3｜缺失年度表：**复制 2024年**`，在该小节正文末尾追加：

```
**已实现**（2026-09-17）：`sheets.Client.CreateYearTab` 把四步放进**一次** `spreadsheets:batchUpdate`
——复制模板 → 改工作表名 → 改表内第 1 行标题的年份 → 按年序排位。四步原子生效，任一步失败整批不生效，
不会留下一张「2024年 的副本」。模板固定 `2024年`；新表 index 取「现有年份表升序中第一个 > year 的位置，
无则末尾」。⚠️ **`--apply` 才会建表**：dry-run 短路在建表之前（`Push` 的第 5 步早于第 6 步），
否则就成了「dry-run 却改了表结构」——这个设计里最坏的失败。见 CONTRACTS `## Sprint M2b-2` ⑤。
```

---

## 粘贴后自查

1. README 的「落地状态」表里 M2b 行以 `✅` 开头，且**没有**留下第二行 M2b。
2. README 全文搜 `未开始`，应只剩 M4 那一行。
3. 手册 §七 标题含「已实现」，D1/D2/D3 三小节各有一段以 `**已实现**（2026-09-17）` 开头。
4. 手册里新增的 §六·五 排在 §六 与 §七 之间。
5. 四处指向 CONTRACTS 的引用都写的是 `## Sprint M2b-2`（不是 `M2b-1`）。
