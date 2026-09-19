# M4-crisis 生产迁移日志

> **状态：迁移与部署均已完成。** 判据一～四在生产库上全部通过。
> 判据五、六、七按人类裁决**不在本任务范围**（见 §12）。
>
> 执行人：`dev-m4c-a`　任务：TASK-008　仓库 HEAD：`29337e8fb11c6b895f799b60937b520c7d0b3284`
> 本文由 `dev-m4c-a` 撰写、`ops-m4c-a` 落盘（`docs/07-deploy/*` 的写者是 `ops-*`，`dev-*` 无权）。

## 时间线（实际时刻）

| 时刻 (UTC) | 本地 | 事件 |
|---|---|---|
| 03:51:41 | 11:51:41 | 步 0 preflight |
| 03:51:55 | 11:51:55 | 备份（`cp` + backup API 两份） |
| 03:55:43 | 11:55:43 | 迁移前基线采集 |
| 04:25:57 | 12:25:57 | 迁移前最后一次当场核实 |
| **04:26:09** | **12:26:09** | **步 2 迁移执行** |
| 04:26–04:32 | 12:26–12:32 | 判据一～四首轮（迁移后、未部署） |
| **06:13:06** | **14:13:06** | **步 3 部署**（`cp` 覆盖 → 立即撞 SIGKILL） |
| ~06:15 | ~14:15 | SIGKILL 修复（换 inode），守卫验证通过 |
| 06:15:31 | 14:15:31 | 判据二～四用线上二进制重跑 |

迁移→部署窗口 **1 小时 47 分钟**（见 §5 的窗口期实证）。

---

## 1. 现场与身份

| 项 | 值 |
|---|---|
| 生产库 | `/Users/zuowei/workspace/runtime/atlas/data/crisis.db` |
| inode | `74731585`（用它证明相对路径解析到的就是这个文件，见 §9） |
| journal_mode | **wal** |
| 旧二进制（部署前） | sha256 `f86a46dc7e26e719ef104f6155b970255d7067bf832ca862cde958540d81f3f4` |
| 新二进制（部署后） | sha256 `a8142360e072f79376d97f5b7b5b930f816c7ba79ab6422969571badf83eeaea` |
| 新二进制构建自 | master `29337e8fb11c6b895f799b60937b520c7d0b3284` |

**二进制身份核实（带正负对照）**

| `strings` 命中 | 旧（部署前） | 新（部署后） |
|---|---|---|
| `crisis`（正对照，证明 `strings` 在工作） | 318 | **355** |
| `macro_observations`（正对照） | 9 | **19** |
| `still has the legacy two-part primary key`（守卫） | **0** | **1** |
| `migrate-bitemporal` | 0 | **2** |

正对照不可省：只看守卫串从 0 变 1，无法区分「守卫真的进去了」和「`strings` 根本没在工作」。

## 2. 步 0 — Preflight

**首次（03:51:41Z）与迁移前最后一次（04:25:57Z）都跑了**，两次结果相同：

```
NULL fetched_at = 0          ← 必须为 0，否则迁移必失败
空串 fetched_at = 0          ← 非阻塞，同趟查掉
count(*)        = 31098
形状 = CREATE TABLE macro_observations (ts TEXT NOT NULL, indicator TEXT NOT NULL,
       value REAL, source TEXT, fetched_at TEXT, PRIMARY KEY (ts, indicator))
view 数 = 0
索引    = idx_macro_obs_ind_ts, idx_crisis_eval_ind_ts
```

🔴 **`count(*)` 实测 31098，不是任务描述里写的 31092。** 那个数取自 2026-09-18，而 crisis
采集每天都在写。判据一的正确形态是「三数相等 ∧ 等于**迁移前当场实测值**」，**不得把会变的
数写死进判据**。

🔴 **并且迁移那一刻库 sha256 与备份逐字节相同**（`413f4ea4…`）⇒ 从 03:55 采基线到 04:26
迁移，**库一字未变**。所以判据二比的是同一份数据，不是「大概没变」。

空串那条为什么要查：`NOT NULL` **允许空字符串**，而空串在 `MAX(fetched_at)` 里永远排最低
⇒ 它过得了迁移，却会让那一行**永远不是当前行**。

## 3. 步 1 — 备份与迁移前基线

### 3.1 备份（两份）

| 文件 | 方式 | 大小 | sha256 |
|---|---|---|---|
| `crisis.db.bak-m4c-20260919T035155Z` | `cp` | 4005888 | `413f4ea4…`（与源库逐字节相同） |
| `crisis.db.walsafe-20260919T035155Z` | `sqlite3 .backup`（backup API） | 4005888 | `e039e5dc…` |

⚠️ **为什么补第二份**：生产库是 WAL 模式。当时 `-wal` 是 0 字节（已 checkpoint），所以单
`cp` 完整；但不该让备份的正确性依赖「那一刻恰好 checkpoint 过」这个偶然条件。走 backup API
的那份不依赖这个前提，**以它为权威副本**。（两份 sha256 不同是正常的——backup API 重排页
布局，语义相同、字节不同。）

两份与源库四项逐项一致：`count(*)` 31098 / `max(ts)` 2026-09-18 / `crisis_evaluations` 352
/ 长度校验和 31098/747281。

**备份位置在部署后被移动过 —— 见 §10。**

### 3.2 迁移前基线（判据二的「前」，事后补不出来）

```
生成时刻(UTC) = 2026-09-19T03:55:43Z
所用二进制    = 旧二进制 f86a46dc…（不含守卫）
当时 count(*) = 31098
命令          = crisis replay --from 2020-01-01 --to 2026-09-17 --json
exit code     = 0
stdout        = baseline-pre.jsonl   29 行 / 1725 bytes
  sha256      = d0ee798b9045a4c9f96e276da9c805644b1a67e6a6b15016ada668ffe36828f9
stderr        = 空

final state: NORMAL over 1719 eval days
entered WATCH    12 times
entered BREWING   0 times
entered CRISIS    4 times
```

24 条 transition，首条 `2020-02-24 NORMAL→WATCH`，末条 `2026-07-15 WATCH→NORMAL`。
元数据另存 `baseline-pre.meta.txt`。

## 4. 步 2 — 迁移（生产库实测）

```
时刻 UTC 2026-09-19T04:26:09Z / 本地 12:26:09 CST
二进制 atlas-new sha256=a8142360…（构建自 master 29337e8f）
目标   /Users/zuowei/workspace/runtime/atlas/data/crisis.db（绝对路径）

migrated /Users/zuowei/workspace/runtime/atlas/data/crisis.db
  RowsBefore 31098
  RowsAfter  31098
  ViewRows   31098
```

### 判据一 —— 命令输出不作数，独立复算

🔴 **为什么不能只读上面那三行**：`TestExecuteCrisisMigratePrintsCounts` 的断言是
`assert.Contains(t, s, "3")`，而 `t.TempDir()` 的路径几乎必然含数字 3 ⇒ 该断言近乎恒真，
**把三个计数全打成 0 也不会有测试变红**。故用 `sqlite3` 独立复算：

```
macro_observations_v1 (= RowsBefore) = 31098
macro_observations    (= RowsAfter)  = 31098
v_macro_current       (= ViewRows)   = 31098
迁移前当场实测 count(*)               = 31098
```

**双向 `EXCEPT`**（全量不抽样，全列 `ts,indicator,value,source,fetched_at`）：

```
v_macro_current EXCEPT macro_observations_v1 = 0
macro_observations_v1 EXCEPT v_macro_current = 0
```

**正对照 —— 证明这把尺有牙**：同一对 `EXCEPT`，左侧把 `value` 全部 `+1`：

```
v_macro_current(value+1) EXCEPT macro_observations_v1 = 31098
```

⇒ 非 0。**「0 行」和「这把尺根本量不出东西」在输出上长得一样**，没有这一步就分不出来。

### 迁移后形状与完整性

```
CREATE TABLE macro_observations (ts TEXT NOT NULL, indicator TEXT NOT NULL, value REAL,
  source TEXT, fetched_at TEXT NOT NULL, PRIMARY KEY (ts, indicator, fetched_at))

index  idx_crisis_eval_ind_ts   (tbl=crisis_evaluations)
index  idx_macro_obs_ind_ts     (tbl=macro_observations)   ← 已重建到新表
view   v_macro_current
table  macro_observations_v1                               ← 回滚依据，保留
crisis_evaluations 仍 352 行（未受影响）
```

⚠️ 核实索引时若用 `where name like '%macro%'` 过滤，`idx_crisis_eval_ind_ts` 会被滤掉、
看起来像丢了索引。**「没看见」不等于「不存在」**——要用无过滤的全量查询。

## 5. 步 3 — 部署，以及 🔴 一次真实的 SIGKILL 事故

### 5.1 部署方式（人类裁决：只覆盖 `bin/atlas`，不跑 `deploy.sh`）

`deploy.sh` 是 `rsync -a -m --delete` 全量同步整个 runtime，远超本次需要；而且它会
**删掉迁移产物目录**（详见 §10）。故只替换单个二进制。

```
时刻 UTC 2026-09-19T06:13:06Z / 本地 14:13:06 CST
旧二进制先留副本: m4c-migration/atlas-old-f86a46dc  sha256=f86a46dc…（回滚用）
```

### 5.2 🔴 `cp` 原地覆盖 ⇒ 线上 atlas 被 SIGKILL

部署后按要求实际调一次 crisis 命令验守卫，结果：

```
atlas crisis status   rc=137   stdout 空   stderr 空
atlas crisis replay   rc=137   stdout 空   stderr 空
```

`137 = 128 + 9` ⇒ **SIGKILL**。不是守卫拒绝（那会 rc=1 并打印错误信息），是进程被内核杀掉，
**没有任何输出**。

诊断时把「内容」与「路径」分开：

| | rc |
|---|---|
| `m4c-migration/atlas-new`（原位置） | **0** |
| `bin/atlas`（刚被覆盖的） | **137** |

而两者 `sha256` **完全相同**，`codesign -dv` 也完全相同（adhoc / linker-signed，
CodeDirectory size 336606、hashes 10516+0 一字不差）。

⇒ 不是内容问题，也不是签名内容问题：**macOS 内核对那个 inode 缓存了旧二进制的代码签名**，
`cp` 原地覆盖保持 inode 只换内容，新内容对不上缓存的签名 ⇒ 内核直接杀进程。

**修法是换 inode**：

```bash
rm -f  /Users/zuowei/workspace/runtime/atlas/bin/atlas      # 旧 inode 释放
cp -p  <新二进制> /Users/zuowei/workspace/runtime/atlas/bin/atlas
```

实测 inode `216433589` → `220870847`，随后：

```
atlas crisis --help    rc=0
atlas crisis status    rc=0
  system state: NORMAL (as of 2026-09-17, 44 eval days)
    vix        GREEN     15.44  p5y=0.26
    move       GREEN     76.22  p5y=0.18
    sofr_effr  GREEN     -3.00  p5y=0.39
    hy_oas     AMBER    270.00  p5y=0.06  COMPLACENCY
    t10y2y     GREEN     27.00  p5y=0.56
    nfci       GREEN     -0.56  p5y=0.06
    usdjpy     GREEN    155.69  p5y=0.79
```

⇒ 守卫放行新形状库（`crisis status` 走 `openCrisisStore → NewStore → verifyBitemporalShape`）。

🔴 **判据是首行那句 `system state: …`，不是 `rc=0`。** `rc=0` 无法区分两件事：

| | rc | 首行 |
|---|---|---|
| 守卫在**生产库**上通过 | 0 | `system state: NORMAL (as of …, 44 eval days)` |
| 守卫在**它刚自己建出来的空库**上空转 | **0** | `no evaluations yet — …` |

成因：`crisis.NewStore` 对 `storage.path` 先 `os.MkdirAll` 再让 sqlite 建库，而
`verifyBitemporalShape` **对全新空库刻意放行**（表不存在 ⇒ `return nil`，否则谁都建不了新库）。
⇒ 在**错误目录**下跑（`storage.path` 是相对路径！）会当场新建一个空 `data/crisis.db`，
`rc=0`、有输出、守卫「通过」——**而验的是那个空库**。

⇒ `system state:` 这一行在空库上**构造上不可能出现**，所以它是这里唯一可用的判据。
**判性质，不判退出码。**

本次实测首行为 `system state: NORMAL (as of 2026-09-17, 44 eval days)`，且已排除误建空库
（源码仓库目录 `data/crisis.db` 的 mtime 仍是 `2026-07-14 19:51:56`，runtime 下今日被改的
`crisis.db` 只有生产库一个）。

### 5.3 🔴 这件事的教训在验证方法上，不在 `cp` 上

部署后这三样**全部通过**：

```
sha256 与期望一致 ✓    codesign 正常 ✓    可执行位在 ✓
```

**而它跑不了。** 静态检查证明「文件是对的」，证明不了「它能运行」。

⇒ **部署后必须实际跑一次会开库的命令**，把「能运行」变成被观察到的事实。

⚠️ **而「实际跑一次」本身也要有正确的判据** —— 见 §5.2 末：判据是首行 `system state: …`，
不是 `rc=0`。这两条合起来才完整：静态属性证明不了能运行，而 `rc=0` 证明不了验的是哪个库。

⚠️ 若当时只做静态核实就宣布部署完成，第一个撞上的会是 **daily 22:45 的无人值守唤起**，
排查的人看到的是「sha256 对、签名对、可执行位对，一跑就死，没有任何日志」——这是本次迁移
全过程最难归因的失效形态，且它**不由迁移引入，纯粹由部署手法引入**。

### 5.4 窗口期实证：迁移后未部署的 1h47m 里什么都没发生

计划里担心的是「反序会让 crisis 的三个 launchd 任务全部失败」。**「失败」这个预期是错的**
——旧二进制在新形状库上**不会失败**，它的 `INSERT OR REPLACE` 本来就带 `fetched_at`，
三段主键下 `fetched_at` 不同就不冲突 ⇒ `OR REPLACE` 从「替换」变成「**追加**」，
而它读裸表、无 `fetched_at` 维度 ⇒ `Window` 覆盖天数被重复行吃掉 ⇒ 分位数错 ⇒
状态判定错 ⇒ 落进 `crisis_evaluations` 并可能发告警，**全程不报错**。

⇒ **反序的危害不是「任务会失败」（那还是好事，会被发现），是「任务会成功但算错」。**

**但本次窗口实际是安全的，原因是 `executeCrisisIntraday` 结构上不写观测**：
它碰 store 只有 `LatestSystemEval` / `HasIndicatorEvalForDate` / `SeriesWindow`（读）
+ `AppendEvaluations`（写**评估表**），且 `case "intraday":` 的 `deps` 构造里**没有
`ingest*` 字段**（对比 `case "nfci":` 有 `ingestNFCI`）。而上述污染场景**需要一次写入**。

窗口期（04:26 → 06:13，intraday 每 1800 秒，应唤起 3~4 次）的实测：

```
库 sha256   = 4ef6a7819ec…   ← 与迁移完成时记录值完全相同
macro_observations = 31098    多修订键数 = 0
crisis_evaluations = 352      ← 也未变
```

⇒ **「intraday 不写观测」由代码推断升级为生产实证。** `crisis_evaluations` 也未变，
与「系统态 NORMAL ⇒ `executeCrisisIntraday` 第一个判断即 `return nil`」一致
（实测最新系统态 `NORMAL`，`ts 2026-09-17`，`eval_at 2026-09-18T14:45:19Z`）。

**真正的截止线是 daily 22:45 本地**（它有 ingest、会写观测），不是 intraday 的 30 分钟。
本次部署完成时距该截止线还有约 8.5 小时。

⚠️ 顺带记下另一个判断：停 launchd（`bootout`）自带一个**更糟**的失效模式 ——
**忘了 `bootstrap` 回去，监控就静默关闭**，比要防的东西更隐蔽。本次未停。

## 6. 判据二 —— 两个方向都做了，它们回答的不是同一个问题

| 方向 | 两侧配置 | 回答什么 | 结果 |
|---|---|---|---|
| 一 | 旧二进制 + 老形状 **vs** 旧二进制 + 新形状 | **迁移没改数据** | sha256 均 `d0ee798b…` ✅ |
| 二 | 旧二进制 + 老形状 **vs** 新二进制 + 新形状 | **新代码在新形状上给出旧结果** | sha256 均 `d0ee798b…` ✅ |

两者都逐字节一致，`stderr` 均空，各 29 行。

⚠️ **为什么必须做两个**：只有方向二时，「库形状」与「二进制版本」两个变量同时变，
一致无法归因；只有方向一时，证明不了新代码的行为。方向一同时也验证了 §5.4 那条论证
（无重复行时旧二进制读裸表无害）。

⚠️ **仍存的限制**：迁移**前**那一侧只能用旧二进制采（新二进制的守卫会拒绝打开老形状库）
⇒ 不存在「新二进制 + 老形状」这一格。这是构造上不可避免，不是疏漏。

## 7. 判据三 —— 🔴 它是【守住】，不是【实现】

用**线上二进制**重跑：`--as-of 2026-09-19T06:15:31Z` 与不带 `--as-of` **同一 sha256**。

**不要把这个「相同」读成 as-of 的验收证据。** 在本次数据上它是**恒等式**：

```
键总数 (distinct ts,indicator)          = 31098
其中 count(distinct fetched_at) > 1 的键 = 0      ← 每键恒一个修订
distinct fetched_at 取值数              = 43
```

老表是两段主键，**结构上就不可能**让同一 `(ts,indicator)` 有两个 `fetched_at`；迁移是
1:1 复制，所以迁移后每键仍恒为一个修订。⇒ `AsOf(任意未来时刻)` 与当前形态**必然**逐值
相同，**即使 as-of 逻辑完全写错**（只要它没把行过滤掉）。

⇒ 判据三能拦住的是「迁移弄坏了当前读」，**不能**证明 as-of 正确。as-of 的正确性证据在
TASK-006 的单测（含变异 Y1 参数顺序、Y2 `<=`→`<`）与 31098 行探针，**不在这里**。

正对照：在一个注入过一条修订的探针库上，多修订键数 = **1** ⇒ 这把尺本身有区分力，
只是本次数据里没有可测的东西。

**这个恒等状态会在第一次迁移后采集时结束**（新 `fetched_at` 落到已有键上），届时判据三
才开始有区分力。**后人请勿拿本次的「完全相同」当 as-of 的验收证据。**

## 8. 判据四 —— 空结果的两种含义

用线上二进制：

```
$ crisis replay --from 2020-01-01 --to 2026-09-17 --json --as-of 2026-07-13T00:00:00Z
rc=0   stdout 0 行   stderr 空
```

为什么该是 0 行：生产库 `fetched_at` 最早 `2026-07-14T05:42:08.150777000Z`、
最晚 `2026-09-18T14:45:05.472055000Z`、43 个取值 —— 2026-07-13 早于全部取值。

**负对照**（让「0 行」有正反两面）：同样 0 行但**不带** `--as-of`：

```
rc=1
Error: no observations between 1990-01-01 and 1990-12-31 — run backfill first
```

⇒ 带 `--as-of` 时「空」是**正确答案**（exit 0），不带时是**错误**（exit 1）。
同一个空结果，两种语境两种含义，**靠退出码区分**，不靠提示语。

### 顺带：`--as-of` 的口径是文本，不是时刻

`fetched_at` 是 TEXT，SQLite 按**字典序**比较。`'.'(0x2E) < 'Z'(0x5A)`，所以
`2026-07-13T00:00:00+00:00` 与 `…Z` 指同一时刻却落在比较的不同位置。

```
$ crisis replay --as-of '2026-07-13T00:00:00+00:00'
Error: --as-of "2026-07-13T00:00:00+00:00": need UTC with a trailing Z — fetched_at is
compared as text, so any offset form (including +00:00) silently misaligns; write e.g.
2026-07-13T00:00:00Z
```

判据是 `strings.HasSuffix(v, "Z")` 而**不是** `off != 0` —— 后者问的是「语义上是不是 UTC」，
而比较的是文本。`+00:00` 语义上就是 UTC，`off == 0` 会放行它然后静默错位。
**判据要和被比较的东西同口径。**

`--as-of` 也接受纯日期 `2026-07-13`（补成当天 `00:00:00Z`，实测与 RFC3339 形式同结果）。

## 9. 回滚（O4：逐字写出，事故当下没人有心情现推）

⚠️ **先更正一处**：任务描述写的是「两次 RENAME 反着来」，**与实现不符**（该说法源自
`migrate.go:23` 一句含糊的注释）。`MigrateBitemporal` 实际只有**一次** RENAME：
`DROP VIEW` → `DROP INDEX` → `RENAME TO macro_observations_v1` → 建新表 → `INSERT SELECT`。
所以回滚是 **DROP + 一次 RENAME**。

**存成 `rollback.sql`，用输入重定向跑**：

```bash
sqlite3 /Users/zuowei/workspace/runtime/atlas/data/crisis.db < rollback.sql
```

🔴 **不要写成 `sqlite3 db "$SQL"`**（单参数形态）：`.bail on` 是 dot-command，单参数形态
**不识别**它，会报 `Usage: .bail on|off` 然后**整段 SQL 一条都不执行** —— 实测 `_v1` 仍在时
该形态 `rc=1`、`_v1` 照旧、表仍是三段主键，**回滚根本没做**。

`rollback.sql` 的内容：

```sql
-- 🔴 前置：回滚会丢掉迁移之后写入的一切，先导出 —— 但导出命令不能照直觉写，
--   见本节末「9.1 回滚前先导出」。直觉写法产出的东西灌不回去。

.bail on
BEGIN;
DROP VIEW  IF EXISTS v_macro_current;
DROP INDEX IF EXISTS idx_macro_obs_ind_ts;
DROP TABLE macro_observations;
ALTER TABLE macro_observations_v1 RENAME TO macro_observations;
CREATE INDEX idx_macro_obs_ind_ts ON macro_observations(indicator, ts);
CREATE VIEW v_macro_current AS SELECT * FROM macro_observations o WHERE o.fetched_at = (SELECT MAX(fetched_at) FROM macro_observations WHERE ts = o.ts AND indicator = o.indicator);
COMMIT;
```

### 🔴 `.bail on` 与 `CREATE VIEW` 两行都不是可选的

**少了 `.bail on` 会永久删库。** 条件：`_v1` 已按 §14 授权删除之后（或在从未迁移的库上误跑），
且经 stdin 执行。sqlite3 CLI 默认不 `.bail on`，`ALTER TABLE … RENAME` 报
`no such table` 之后**继续往下执行**，最后那句 `COMMIT` 把已经生效的 `DROP TABLE` 提交掉。
**`BEGIN` 在场，但它没有保护任何东西。**

**少了 `CREATE VIEW`，正常回滚会「成功」但留下坏的读路径**：`rc=0`、31098 行一行不少、
索引齐全，**而 `v_macro_current` 不存在** —— `DROP VIEW` 在第 2 行，而重建它的语句原本没有。
视图的 DDL **逐字取自 `sqlite_master`**（`schema.go:71` 是 `bitemporal.CurrentQuery(obsSpec)`
拼出来的，源码里没有字面量可抄）。

**六种形态实测**（sqlite3 3.51.0，生产库 `.backup` 快照，SQL 用 `sed -n` 从文件逐字提取）：

| 调用形态 | `.bail on` | `_v1` | rc | 结果 |
| --- | --- | --- | --- | --- |
| 管道 | **无** | 已删 | **1** | 🔴 **观测表不存在，31098 行全丢** |
| 管道 / heredoc / `< file` | 有 | 已删 | **1** | ✅ 31098 行、索引 2、视图 1 全在 |
| `< file` | 有 | **仍在** | **0** | ✅ 两段主键、31098 行、视图可查 |
| **单参数** | 有 | 仍在 | **1** | 🔴 `Usage:`，**整段未执行，回滚没做** |

⚠️ **`rc` 在这张表里几乎没有信息量**：五行 `rc=1` 里一行删光了库、三行毫发无伤、一行什么都没做。
运维对 `rc=1` 最自然的解读是「报错了所以没执行」，而在第一行那个场景里这个解读会让人不去检查。

⇒ **回滚后的判据只能是直接查表**：

```bash
sqlite3 <db> "SELECT COUNT(*) FROM macro_observations;"                       # 期望回滚前的行数
sqlite3 <db> "SELECT COUNT(*) FROM sqlite_master WHERE type='view';"          # 期望 1
sqlite3 <db> "SELECT sql FROM sqlite_master WHERE name='macro_observations';" # 期望含 PRIMARY KEY (ts, indicator)
```

**二进制也要一起回滚**（否则新二进制的守卫会拒绝打开回滚后的老形状库）：

```bash
rm -f /Users/zuowei/workspace/runtime/atlas/bin/atlas     # 🔴 换 inode，见 §5.2
cp -p <atlas-old-f86a46dc 所在路径> /Users/zuowei/workspace/runtime/atlas/bin/atlas
```

**回滚 SQL 的验证覆盖范围**（⚠️ 不要写成无条件的「已实跑验证」—— 本文初稿就是那么写的，
而那次验证用的恰好是三种调用形态里唯一不会删库的那一种，见上表）：调用形态 **四种全测**
（管道 / heredoc / `< file` / 单参数）× `.bail on` 有无 × `_v1` 已删或仍在；
sqlite3 版本仅 **3.51.0**（dot-command 行为可能随版本变，换版本请重测）。

下面这组是**单参数形态、`_v1` 仍在**条件下的数据（含 1 条注入修订、共 31099 行的探针）：

```
rc=0
回滚后 macro_observations = 31098   ← 迁移后写入的 1 条按设计丢失
_v1 是否还在              = 0
view 数                   = 0
索引                      = idx_crisis_eval_ind_ts, idx_macro_obs_ind_ts
```

与 walsafe 备份四项一致。🔴 **决定性证据不是 DDL 比对，是产出比对**：用旧二进制在回滚后的
库上跑同区间 `replay`，与迁移前基线**逐字节一致**（同为 `d0ee798b…`）。DDL 比对只能证明
形状像，产出比对才证明行为同。

⚠️ `ALTER ... RENAME` 会给 DDL 加引号（`CREATE TABLE "macro_observations"`）。功能等价，
但**与原始 DDL 不逐字节相同** —— 拿 DDL 文本做比对的脚本会在这里绊一跤。

### 9.1 回滚前先导出 —— 直觉写法产出的东西灌不回去

**❌ 错误（初版本文写的就是它）**：

```bash
sqlite3 <db> ".mode insert macro_observations" "SELECT * FROM macro_observations;" > out.sql
```

实测（生产库 `.backup` 快照 + 注入 1 条修订模拟「迁移后写入」）：

```
导出           ⇒ 31099 行
回滚（表变两段主键）
灌回           ⇒ rc=1，stderr 31099 行，全是
                 UNIQUE constraint failed: macro_observations.ts, macro_observations.indicator (19)
该键最后留下   ⇒ 旧修订。新修订丢失。
```

成因：三段主键表里同一 `(ts, indicator)` 可有多个修订，而**回滚之后的表是两段主键**
（就是 §9 那段 SQL 恢复出来的形状，实测 `PRIMARY KEY (ts, indicator)`）。多行挤进一个键
必然撞唯一约束；而回滚后的表里**本来就有**这些键（来自 `_v1`），裸 `INSERT` 一行都进不去。

🔴 **最坏的是失败形态**：每一行都失败，**但表里仍有数据**（回滚留下的那份）。
运维看到一个有数据的表，会以为恢复成功了。

**✅ 正确写法**（实测 `rc=0`、stderr 0 行、新修订与新键都恢复）：

```bash
sqlite3 /Users/zuowei/workspace/runtime/atlas/data/crisis.db \
  ".mode insert macro_observations" \
  "SELECT ts, indicator, value, source, fetched_at FROM macro_observations o
   WHERE o.fetched_at = (SELECT MAX(fetched_at) FROM macro_observations
                         WHERE ts = o.ts AND indicator = o.indicator);" \
  | sed 's/^INSERT INTO/INSERT OR REPLACE INTO/' > post-migration-rows.sql
```

| 改动 | 没有它会怎样 |
| --- | --- |
| `MAX(fetched_at)` 投影成每键一行 | 多修订挤进两段主键表，撞唯一约束 |
| `INSERT INTO` → `INSERT OR REPLACE INTO` | 回滚后表里已有这些键，裸 `INSERT` 仍冲突 |

🔴 **代价：这份导出物不是无损的。** 投影**丢掉历史修订，只保留每键最新值** —— 回滚到
两段主键模型的**必然代价**（目标表结构上容不下多修订），不是实现缺陷。
⇒ 若历史修订有价值，**回滚前另外 `sqlite3 <db> ".backup '<副本>'"` 一份三段主键的完整库**。

⚠️ **路径必须绝对。** 实测错误目录下：相对路径 `crisis.db` ⇒ `rc=1`、导出 **0 行**、
并**当场建出一个 0 字节的 `crisis.db`**；绝对路径 ⇒ `rc=0`、31098 行。
**「导出文件存在」不是成功的判据，要看行数。**

ℹ️ 本节结论曾写反：初版说这条直觉写法「实测成立、可回灌」。那次把导出物灌进了**空表** ——
空表没有约束冲突，所以必然成功。**在一个结论必然成立的条件下求了值，而没写出那个条件。**
该问没问的是「**灌回哪种表？**」

## 10. 🔴 备份位置：原位置会被 `deploy.sh` 删掉

`scripts/ops/deploy.sh` 的 `rsync -a -m --delete` 的 `--exclude` 保护的是 `/data/`、
`/logs/`、`/queue/` 等，**没有** `/m4c-migration/`。⇒ 任何人任何时候跑一次 `deploy.sh`，
两份库备份、迁移前基线、以及**旧二进制**都会被 `--delete` 清掉。

🔴 旧二进制 `atlas-old-f86a46dc`（7 月构建）**无法重建** —— 新二进制可以从 master 的
commit 重新 `go build`，旧的不行。它是回滚路径的一半。

**处置：复制**（不是移动 —— 移动会让 checkpoint、本日志、其他 agent 记下的路径全部失效，
而多占几十 MB 没有代价）一份关键项到受保护位置：

```
/Users/zuowei/workspace/runtime/atlas/data/m4c-migration-safe/
  atlas-old-f86a46dc                    43375234   sha256 f86a46dc…   ← 两处比对一致
  crisis.db.walsafe-20260919T035155Z     4005888   sha256 e039e5dc…   ← 两处比对一致
  crisis.db.bak-m4c-20260919T035155Z     4005888   sha256 413f4ea4…
  baseline-pre.jsonl / baseline-pre.meta.txt / baseline-deployed.jsonl ← 两处比对一致
```

原目录 `/Users/zuowei/workspace/runtime/atlas/m4c-migration/` 仍在，另含 `atlas-new`
（可从 master `29337e8f` 重建，故未复制）。

## 11. 两个会骗人的坑

1. **`storage.path` 是相对路径**：`configs/crisis-monitor.yaml` 写的是 `data/crisis.db`，
   靠 launchd 的 `WorkingDirectory=/Users/zuowei/workspace/runtime/atlas` 解析。
   手工跑命令时 cd 错目录就指向别的库。**核实方法是比 inode，不是比路径字符串**：
   ```
   stat -f %i data/crisis.db                                       → 74731585
   stat -f %i /Users/zuowei/workspace/runtime/atlas/data/crisis.db → 74731585
   ```
2. 🔴 **源码仓库目录下也有一个 `data/crisis.db`**
   （`/Users/zuowei/workspace/go/src/github.com/newthinker/atlas/data/crisis.db`，
   3883008 bytes，Jul 14 的旧库）。在源码目录误跑 `crisis replay` 会读到它，
   **输出有数据、有状态机摘要、不报任何错** —— 看起来和跑对了完全一样。

## 12. 🔴 重启 `serve` 不验证守卫

plan Step 3 写的是重启 `com.newthinker.atlas.serve`。但 `crisis.NewStore` 的唯一非测试
调用点是 `cmd/atlas/crisis.go` 的 `openCrisisStore`，只服务 `atlas crisis *` 命令族
（backfill / eval / status / replay / report），`serve.go` 无引用。

⇒ **「serve 起来了」不是守卫通过的证据。** ⚠️ 但「跑了 `crisis status` 且 `rc=0`」**同样不是**
——它在一个自己刚建出来的空库上也会 `rc=0`（见 §5.2 的判据表）。可用的判据只有首行
`system state: …`。本次是实际跑 `atlas crisis status` 验的
（并因此撞出了 §5.2 那个 SIGKILL）。

### 12.1 判据：首行必须是 `system state: …`（不是 `rc=0`）

```bash
cd /Users/zuowei/workspace/runtime/atlas   # ⚠️ 必须，storage.path 是相对路径
./bin/atlas crisis status \
  --config configs/config.yaml --crisis-config configs/crisis-monitor.yaml | head -1
# 必须输出: system state: …
```

**为什么 `rc=0` 不够**：`crisis.NewStore` 对 `storage.path` 先 `os.MkdirAll` 再让 sqlite
建库，而 `verifyBitemporalShape` **对全新空库刻意放行**（表不存在 ⇒ `return nil`，否则谁都
建不了新库）。⇒ 在错误目录下会**当场新建一个空 `data/crisis.db`**，`rc=0`、有输出、
守卫「通过」——而验的是那个空库。

**三场景实测**（team-lead 2026-09-19 在临时目录实测，未碰生产；数据来源不是本文作者）：

| 场景 | rc | 首行 | 副作用 |
| --- | --- | --- | --- |
| 在 `$ATLAS_RUNTIME` 下（正确） | 0 | `system state: NORMAL (as of …, 44 eval days)` | 无 |
| 错目录 + **绝对**配置路径 | **0** | `no evaluations yet — …` | **新建 24576 字节空库，主键正确** |
| 错目录 + 相对配置路径 | 1 | — | `Error: reading crisis config: …` |

⚠️ 第三行那个 `rc=1` 的成因是**配置文件路径**、不是 `storage.path`：`LoadConfig` 先失败，
根本走不到 `storage.path`。**换成绝对配置路径就只剩第二行那条假成功路径。**

⇒ `system state:` 这一行在全新空库上**构造上不可能出现**（空库恒为 `no evaluations yet — …`），
所以它是这里唯一可用的判据。**判性质，不判退出码。**

### 12.2 F7 —— 这条对所有「空则放行」的守卫都成立

本次的 `verifyBitemporalShape` 只是一个实例。一般规则：

> **验证任何带「空则放行」分支的守卫时，判据必须能区分「守卫真的检查过」与「守卫面对
> 空对象直接返回」** —— 因为**「守卫没报错」同时兼容这两种情形**。

「空则放行」本身是合理设计（否则无法初始化），但它使得守卫的沉默有两个来源。⇒ 凡
「表不存在 / 集合为空 ⇒ `return nil`」形态的守卫，都要用**输出的性质**而非退出码来验。

⚠️ 记这条的直接代价：本文 §12 最初写的判据（「本次是实际跑 `atlas crisis status` 验的」）
**与它自己要防的失效是同形的** —— 照它复现的人可能只看 `rc=0`，而 `rc=0` 在空库上也成立。
**一份运维手册里写着一个自我击穿的判据，比不写更糟**，因为它给人一种已经验过的感觉。

## 13. 运维须知 —— 故障模式变了

旧写法 `INSERT OR REPLACE`：同键新抓取**静默替换**旧行，永不出错。
新写法裸 `INSERT` + 冲突三分：

| 情形 | 行为 |
|---|---|
| `(ts, indicator, fetched_at)` 不存在 | 插入 |
| 存在且值相同 | 跳过（同一批被重跑，无信息差） |
| **存在且值不同** | **报错**：`the same fetch reported two values; refusing to overwrite` |

### 🔴 中断范围是「整轮采集」，不是「整批报错」

两层，只看写路径会低估：

1. **事务层**：`UpsertObservations` 的 `return` 在遍历 observations 的循环**内部**，
   事务由 `defer tx.Rollback()` 兜底 ⇒ 这一批整批不落盘。
2. **采集层**：`IngestAll` 在 FRED 循环里是 `return nil, err` —— **直接上抛，不 `continue`**。
   而 `fredDirect` 第一个就是 **vix**（顺序 vix → hy_oas → t10y2y → nfci，随后
   `ingestSpread` 的 sofr_effr 同样 `return nil, err`）。

⇒ **vix 一炸，该轮后续指标根本不会被抓取。** yahoo 那两个（move / usdjpy）本身是
「错误只记进 `YahooErrs` 不中断」，但它们排在 FRED 之后，FRED 抛错时**走都走不到**。

⇒ 写成「**中断整轮采集**」，**不要写「整批报错」** —— 后者会让人以为只丢一个指标，
实际是七个指标一个都没抓。

### 它不会误报

`stamp := NowStamp(ig.now())` 取在 `IngestAll` 开头，**同一次运行内所有行共用一个 stamp**
（纳秒精度）⇒ **重跑采集是追加，不是冲突**。只有**上游在一次抓取里对同一 `(日期, 指标)`
返回两个不同值**才触发；各源指标互不重叠（FRED: vix/hy_oas/t10y2y/nfci，yahoo: move/usdjpy，
派生: sofr_effr），所以触发条件只能来自上游自身返回重复日期。

⇒ 迁移后若出现这个形态的采集失败，**那是数据源问题被暴露出来，不是本次迁移引入的缺陷**。
行为是对的 —— 但它把一条静默路径变成了会中断采集的响亮失败，运维得先知道，才不会在半夜
把它当成迁移事故。

## 14. `macro_observations_v1` 什么时候可以删

它是回滚的唯一数据依据，删掉就没有回滚了（另一半是旧二进制，见 §10）。

判据不是「过了几天」，是这三条同时成立：

1. 新形状二进制已部署并至少跑完一个完整 ingest 周期（daily + 一次 intraday），无报错；
2. `SELECT count(*) FROM v_macro_current` 与 `SELECT count(*) FROM macro_observations_v1`
   相等 —— 不等说明有修订行或丢行，任一情况下都还不该删；
3. `crisis replay` 的产出与迁移前基线一致（这条会随时间推移失效，因为新数据会进来；
   所以要在**迁移当天**跑，并把结果留档 —— 本文件 §6 就是那份留档）。

三条都成立后，删 `_v1` 只是回收空间（约 4MB）。**没有磁盘压力就别删**：留着的成本是
一次性几 MB，删掉的成本是回滚路径消失。**最终决定留给人类。**

## 15. 判据五、六、七 —— 留给人类，不在本任务范围

**不得以判据一～四通过为由声称七条判据全部完成。**

| 判据 | 内容 | 为什么必须由人类做 |
|---|---|---|
| **五** | **人造一次修订** | as-of **唯一**能自证的方式 —— 库里现有数据全是单版本（实测 31098 键**全部**单修订），不造修订就永远测不出差别。做完后 §7 的恒等式才被打破 |
| **六** | `crisis backfill` 重跑一段已有区间，若采集器给出与已有行相同的 `fetched_at` 但不同 value ⇒ 应**响亮失败** | 验的是 §13 那条新故障模式在真实 backfill 路径上确实触发 |
| **七** | **等一次真实采集**（launchd 自然唤起）后复核 | 🔴 **重复键计数 > 0 才算完成，恒为 0 不能销账** —— 缺陷就在那条路径上，不能用造数据代替 |

**下一个自然检查点是 daily 22:45 本地**：那是迁移后第一次会写观测的采集，
判据七的观察窗口从那时开始。

## 16. 库与二进制的最终状态

```
macro_observations    = 31098
macro_observations_v1 = 31098
v_macro_current       = 31098
多修订键数            = 0      （符合 1:1 复制；第一次真实采集后应 > 0）
库 sha256             = 4ef6a7819ec40b46f3f5c55451ab8ed5ceacf924cbafef883b10cabd90339b8d
crisis_evaluations    = 352

bin/atlas sha256 = a8142360e072f79376d97f5b7b5b930f816c7ba79ab6422969571badf83eeaea
bin/atlas inode  = 220870847   （🔴 不是 216433589 —— 见 §5.2）
实测可运行: crisis --help rc=0 / crisis status rc=0 并正常输出
```

回滚路径完整：两份库备份 + 旧二进制，且关键项在 `data/m4c-migration-safe/` 各有第二份。
