# Changelog —— Sprint M3（hestia `warp-hestia` 解读 skill）

2026-09-06 ～ 2026-09-09 ｜ 8/8 accepted ｜ 跨 atlas / loom / nanoclaw 三仓库

---

## 交付了什么

一条从**金融数据契约**到**可读解读笔记**的完整管道，外加保证笔记不被静默篡改的两级校验。

### atlas（数据侧）

- **`history` 侧车**：`BuildHistory` / `WriteHistory` / `History.JSON` / `History.FileName`，ingest 路径改为**先侧车后契约**，并登记 AST 守卫防止顺序被改回。
- **`contract emit` 同产侧车**：回放路径与实时路径**同形**，两条生产路径产出的侧车逐字节可比。
- **`CONTRACTS.md` `## Sprint M3`**：§A 契约更正（四条）、§B 六行实测登记（逐行带来源注）、§C 冒烟结转、§D 六条结转（第六条为规格层待决，按「实测行为 / 为何不判红 / 待决问题」三段式登记）。136 行纯追加，删除 0 行。

### loom（写入侧）

- `spool.archive` 增加 `source` 白名单参数；`spool_write_allow` 增加 `Wiki/Macro/PBOC`。
- 测试 35 → 39（两把独立的尺同值），无新增 Go 依赖。

### nanoclaw（生成侧）

- **`warp-hestia` skill**：`SKILL.md` 六步流程 + `methodology` / `note-format` / `glossary` 三份 references。
- **`prepare.py`**：契约 + 侧车 → 笔记骨架（frontmatter 14 基础 + 9 派生 + 4 信号、两张数据表、判读提示、批注保留、修订处理）。
- **`verify.py`**：写回前校验机器区，校验行四态（含**删除校验行**这一态），两级校验条数写死为字面常量而非从结构推导。
- 夹具五期 + 修订契约 + 两份 golden；测试 95 个（`prepare` 63 + `verify` 32）。

---

## 三条返工（QA REJECT 后，人类裁决只修已复现的）

| | 缺陷 | 修法 |
| --- | --- | --- |
| **F1** | `SKILL.md` Step 3 判的是「变量非空」而非「文件存在」，而该变量恒非空 ⇒ **每期首次生成必失败** | 改用显式 `if [ -f "$EXISTING" ]` / `else` 两分支（POSIX，三个独立 shell 实现均验过） |
| **F2** | 契约与侧车配对无校验，侧车顶层 `for` 字段**被读 0 次** ⇒ 错配的一对能产出一份自洽而全错的笔记 | 加 `assert_pair`，判据是**期次**（`period-period_type`）不是文件名 |
| **F3** | 封条作用域到 seal 行**之前**为止 ⇒ seal 与 end 之间可插伪造表而校验放行 | 加形状断言：seal 行必须紧邻 end 标记之上。**不触碰摘要算法**，两份 golden 逐字节未变 |

每条修复都配了**消融实验**（把守卫拆回缺陷态确认缺陷复活），复现的数字与修前实测逐字吻合。

---

## 未完成 / 需人执行

1. **loom PR #13 未合并**（`main` 仍在父提交 `0466116c…`）
2. **nanoclaw PR #5 未合并**（6 commits，`feat/warp-hestia`）
3. `configs/config.local.yaml` 未改、selvage 未重启
4. `Wiki/Macro/PBOC/` 是否被 Spool 自动创建 —— **未实证**（该目录当前不存在；代码里有无条件 `os.MkdirAll`，但那是代码阅读不是运行结果）

前两条是刻意的：合并共享分支是人的决定。四条同属需求文档 TASK-006 Step 0 的人执行前置。

QA 提出而本轮未修的 finding（H1 / H2 / A2 / A3 / B1 / B2 等）见 `final-report.md` §4，各附不修的理由。

---

## 交付锚（全 sha）

| 仓库 | 锚 | 分支 |
| --- | --- | --- |
| nanoclaw | `2a6d39388e4648eb3be0f15b42ee28b52428c5d8` | `feat/warp-hestia` |
| loom | `6d6c38f96901dc60f516ef2154283a213782ad5e` | `feat/spool-source-param` |
| atlas | 见下方「归档实采」 | `master` |

### 归档实采（在最后一次改动之后统一重采）

**atlas 交付锚：`42195edd581703460301bf632397c2bcfb8d21bb`**

采样时点：11 个已合入的 `task/*` 遗留分支删除之后、归档提交之前。这是**本 sprint 全部交付合入后的 master**。

⚠️ **归档提交本身会排在它之后**，所以「归档目录里的这份 changelog 所在的 commit」与本行的 sha 不是同一个 —— 这是预期的，不是漏更新：本行要回答的是「交付内容锚在哪」，不是「这份文件躺在哪个 commit 里」。Sprint 收口 tag 打在归档提交上，两者各自回答各自的问题。

（本节刻意留到最后才填。理由：预写的值在写下时正确、在读它时未必 —— 本 sprint 已实测过一次同形失效，交接文档把依赖锚钉成 `HEAD`，后续提交使同一条命令从「隔离」静默变成「包含」，套件由 `577/0` 变成 `571/6`，而命令原文一个字都没变。）

---

**8 个任务、27 条 `TASK-00x` commit、五个跨仓库任务。** 详细验收见 `final-report.md`，待决机制见仓库根 `PENDING-MECHANISMS.md`。
