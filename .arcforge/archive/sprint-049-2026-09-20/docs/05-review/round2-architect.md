# 第二轮 · Architect lens 发现清单（原样转录 + qa-m4c-a 的复核标注）

- 来源：Architect lens 子代理 `a1cc1731ec7127820`，经 teammate message 交回 `qa-m4c-a`，由 `qa-m4c-a` 落盘（只读 lens 无写权）。
- 转录原则：**发现内容原样保留**，我只在每条后面加一行 `▸ qa-m4c-a 复核：` 说明我自己验了没有、验的结果。没验的就写没验。
- 它的方法学声明（原样）：全部实验在 `git archive HEAD` 的隔离副本与临时 sqlite 库上跑，生产目录 `/Users/zuowei/workspace/runtime/atlas/` 一次都没碰、没读没连；主工作区未改动（`git status` 仍是起始 4 条，HEAD 仍 `e799bf5`）；隔离副本基线三个包全绿 ⇒ **每条「现有测试没抓住」都是在一个绿套件上说的**。

---

## F1 [CRITICAL] `internal/hestia/CONTRACTS.md` §B（未提交）：回滚 SQL 在 `_v1` 已删时会永久删掉观测表

原文结论：§C 明确授权删 `_v1`；此后粘 §B 进 sqlite3 会话，实测 sqlite3 3.51.0 —— **stdin/heredoc 形态**下第 4 步 `ALTER` 报错后 CLI 继续执行，`COMMIT` 把前面的 `DROP TABLE macro_observations` 落盘，`sqlite_master` 变空；而 `sqlite3 db "<整段>"` 单参数形态会 bail 并回滚。**同一段 SQL 按怎么敲决定安全还是全损，文档没说。**
第二条触发路径：在**从未迁移过**的库上跑 §B，同样先 `DROP` 掉唯一那份表。§B **零前置检查**，而 `migrate.go:92-105` 在迁移方向是有 `_v1` 占名守卫的。
修法：块首加 `.bail on`，并把破坏性步骤挪到最后（`RENAME macro_observations→_v2` / `RENAME _v1→macro_observations` / `DROP _v2`）。

> ▸ **qa-m4c-a 复核：独立实测，成立，且比原文更糟。** sqlite3 3.51.0（`/usr/bin/sqlite3`），两种形态各跑一次，同一段 §B SQL、同一种库（已迁移 + `_v1` 已删 + 1 行观测）：
>
> | 形态 | rc | `sqlite_master` 存活对象 | 观测行数 |
> | --- | --- | --- | --- |
> | A：`printf '%s\n' "$SQL" \| sqlite3 db`（粘贴/heredoc 的自然形态） | 1 | **`[]`（完全为空）** | **表已不存在** |
> | B：`sqlite3 db "$SQL"`（单参数） | 1 | `macro_observations, sqlite_autoindex…, idx_macro_obs_ind_ts, v_macro_current` | 1 |
>
> 形态 A 下**表、索引、视图全没了**，不只是表。报错原文：`Parse error near line 5: no such table: macro_observations_v1` / `Parse error near line 6: no such table: main.macro_observations`，随后第 7 行 `COMMIT` 照常执行并提交了前面的 `DROP`。
> 🔴 **这条同时推翻了我第一轮的结论。** 我在第一轮报告里写「**`CONTRACTS.md` §B 的回滚 SQL 是对的**」，那次实测用的正是形态 B —— **我在它必然安全的那个分支里求了值，并且没写出这个条件**。这与我在同一份报告 R-4 里批评 Leader 的「只在判据必然成立的样本上求值」是同一个错误，出自我自己之手。第一轮那条结论**撤回**，以本条为准。

## F2 [HIGH] `internal/crisis/store.go:70-98` vs `internal/hestia/store.go:109-131`：crisis 缺 `verifyCurrentView`

hestia 查两样（`pragma_table_info` 列比对 + 视图定义**全等**比对，期望值由 `currentViewDDL(spec)` 同源派生），crisis 只查一样（`isBitemporal` 的 PK 文本）。列检查 crisis 不做可辩护（5 列固定、显式列名、缺列必响亮失败）；**视图检查缺失不可辩护 —— 它是全部 5 个读取点的入口**。
实测：在形状完全正确的已迁移库上把视图换成 `CREATE VIEW v_macro_current AS SELECT * FROM macro_observations`（掉了 `MAX` 过滤），`NewStore` 返回 nil，`Observation` 返回 10.0（真值 11.0，即被取代的旧修订）、`SeriesWindow` 一个交易日返 2 行、`EvalDates` 返 `[2026-01-02 2026-01-02]` —— **本次迁移要消灭的那组缺陷被静默恢复**；换成指向 `_v1` 则读到 999/stale。三条全部 `err=nil`。
且**没有修复路径**：库已是三段主键时 `MigrateBitemporal` 提前返回 `{AlreadyMigrated:true}` 不碰视图（`migrate.go:61-63`），`NewStore` 的 `CREATE VIEW IF NOT EXISTS` 空转，只能人工 `DROP VIEW` 而无人会被告知。
测试抓不住的原因：crisis 每个测试都用 `t.TempDir()+NewStore` 造库，**部署的视图按构造等于期望的视图**。

> ▸ **qa-m4c-a 复核：成立，与我第一轮独立查出的 R-1 是同一条，两个 reviewer 独立收敛。** 我的实测走的是「视图指向 `_v1`」那条路径（读到冻结值 10 而非 99）；它多测了「掉 `MAX` 过滤」这条，**那条更说明问题** —— 它恢复的正是 `EvalDates` 重复日期 + `SeriesWindow` 被重复行吃掉 `LIMIT` 这组缺陷，即本 sprint 的立项理由。「没有修复路径」那半边我未独立验，读 `migrate.go:60-63` 与代码一致。

## F3 [HIGH] `internal/crisis/schema.go:70-73`：视图定义演进时，新库对、生产库保留旧视图、测试全绿、运行时零报错、migrate 修不了

改共享包时只有 hestia 那半边会在启动时变红，容易被读成「crisis 没受影响」。Q1「该共用而各写一份」最实的一条：`hestia/schema.go:90-92` 的 `currentViewDDL(spec)` 与 `store.go:109-131` 的 `verifyCurrentView(db, spec)` 是一对**只认识 Spec 的通用机制**，却住在 `internal/hestia`；两个使用者共同 import 的 `bitemporal` 只给了 `CurrentQuery`。
实现坑（实测）：SQLite 存 `sqlite_master` 时去掉 `IF NOT EXISTS` 且**不带结尾分号**，而 crisis 的 `schemaDDL()` 末尾追加了 `";"`，照抄 hestia 那句 `strings.Replace` 不够。

> ▸ **qa-m4c-a 复核：未独立实测。** 我读过 `schema.go:70-73` 确认末尾确实追加了 `";"`，与该实现坑一致。「测试全绿」这半边与 F2 同源（测试库按构造视图正确）。

## F4 [HIGH] `store.go:100-114` + `cmd/atlas/crisis.go:705-720`：AsOf 四条实测全部 `err=nil`

(a) `past := s.AsOf(t)` 后 `past.UpsertObservations` **成功**、行落进实时基表、而 `past` 自己读不到（新行 `fetched_at > asOf`）—— 自称只读的历史视图静默改了生产数据再否认。
(b) `asOf` 只过滤 `macro_observations`，`RecentSystemEvals` / `RecentIndicatorEvals` / `LatestSystemEval` / `HasSystemEvalForDate` / `HasIndicatorEvalForDate` 五个方法完全不受约束，`AsOf("2026-07-20").HasSystemEvalForDate("2026-09-01")` 返 true；今天无害仅因 `executeCrisisReplay` 用 `NewMemHistory`（`replay.go:26`）从不调 `st.History(ctx)`。
(c) 文档（`store.go:102-104`）警告「不该调用两次」，**实测双 `Close` 无害（返 nil）**，真正会炸的是 `defer copy.Close()` —— 它关掉父的连接池，父此后读报 `sql: database is closed`。**文档把危险的那一半和安全的那一半说反了。**
(d) `asOf` 未导出且无 getter，于是 `executeCrisisReplay` 同时收 store 和 asOf 字符串且**不校验一致**，只用字符串决定空结果算成功还是报错 —— 传非空 asOf 配未时移的 store，会把 exit 1 的「run backfill first」变成静默 exit 0。
修法（不碰 C7）：副本 `Close()` 变 no-op + `asOf != ""` 时写方法返错 + 导出 `AsOfStamp()` 去掉冗余参数。

> ▸ **qa-m4c-a 复核：(b) 与我第一轮的 R-5 是同一条，独立收敛。(a)(c)(d) 未独立实测。** (c) 若成立则是文档把危害说反，值得单独修；(d) 与我 R-6 相邻但更强（我只说「空序列」半边未验，它指出两个参数可以不一致）。

## F5 [MEDIUM] `store.go:147-193` vs `internal/hestia/store.go:806-830`：冲突判定各写一份，问的不是同一个问题

不主张 crisis 改用 `Lookup+Classify`（它要的是「精确三段主键在不在」，`Classify` 答不了，TASK-006 的 R3 已钉成行为断言）。发现是**基座没提供这个原语**，导致第二个使用者只能离开抽象，结果 `bitemporal.Classify` 的 `OutOfOrder` 在 crisis **零消费者** —— 写入比现存最新更旧的 `fetched_at` 会被静默插入无任何信号。

> ▸ **qa-m4c-a 复核：未独立实测。** 与 `TestUpsertOlderRevisionAfterNewerIsSwallowed`（`store_test.go:938`）的语义一致 —— 那条测试断言的正是「静默吞掉」，即该行为是**有意的**；F5 的落点是「基座的 `OutOfOrder` 信号没人消费」，不是「行为错了」。

## F6 [MEDIUM] `cmd/atlas/crisis.go:635-673`：`--as-of` 的文本序守卫离它保护的三处比较两个包远

三处比较是 bitemporal 的 `Lookup` MAX / `Classify` 的 `>=` / `AsOfQuery` 的 `<=`；基座里没有任何一行指向该守卫，也没有共享的规范化入口。hestia 今天不暴露只因 `published_at` 是纯日期 —— **那是它数据的性质不是设计的性质**。修法：`bitemporal.NormalizeRevision()` 放在比较器旁边。

> ▸ **qa-m4c-a 复核：未独立实测。** 论证形态可信（我第一轮确认过 `parseAsOf` 只在 `cmd/atlas` 里，`internal/crisis` 与 `bitemporal` 均无对应校验）。

## F7 [MEDIUM] `internal/crisis/migrate.go:70-84`：`isBitemporal` 用 DDL 文本子串判形状，错误文案会说假话

实测三个语义等价的建表语句里，`PRIMARY KEY (ts,indicator,fetched_at)`（**无空格**）被 REJECTED，而错误文案说它「still has the legacy two-part primary key (ts, indicator)」—— **这句是假话**，并让运维去跑 `migrate-bitemporal`（会把一张本来正确的表改名 `_v1` 再复制）。反方向安全（只产假阴）。可达性限手搓表，但 **§B 的运维手册本身就是手写 SQL**。修法：`pragma_table_info WHERE pk>0 ORDER BY pk` 比对 `[ts,indicator,fetched_at]`。

> ▸ **qa-m4c-a 复核：我第一轮独立想到同一条并判为 LOW（未找到可达路径）。它给出的「§B 手册本身就是手写 SQL」是我没想到的可达性论证，我接受升到 MEDIUM。** 文本判据的细节我验过一半：`strings.Join(strings.Fields(ddl)," ")` 只归并空白、不会在逗号后补空格 ⇒ 无空格形态确实落空。

## F8 [MEDIUM] `store.go:70-98` + `schema.go:36-39`：守卫只读 PK 文本，`NOT NULL` 只对我们自己建的表生效

`schema.go` 的注释明确点名「SQLite 主键允许 NULL」这个危害并靠 `NOT NULL` 挡，但守卫只读 PK 文本。实测：`fetched_at` **无** `NOT NULL` 的三段主键表被 ACCEPTED，已有的 NULL 行经视图**永久不可见**（base=2 view=1），同键再插一行 NULL 也不撞主键。同类还有 TASK-008 已记录的 `fetched_at=''`（过 `NOT NULL` 但在 `MAX` 里永远垫底）。
⚠️ 原文自述：**可达性我没查到代码路径**（`migrate.go` 会因 `NOT NULL` 在含 NULL 的老库上原子失败），**成因未定**。

> ▸ **qa-m4c-a 复核：未独立实测。原文已自述可达性未知，保留该限定，不要在转述时丢掉。**

## F9 [MEDIUM] §B 开头那条导出命令并不能让回滚不丢数据

实测：`.mode insert` 产出裸 `INSERT INTO`，灌回两段主键表时撞 `UNIQUE constraint failed (19)`，留下的是旧修订（10.0）而**新修订（11.0）丢了**。要可回放必须 `INSERT OR REPLACE` 且先按 `MAX(fetched_at)` 投影成每键一行。

> ▸ **qa-m4c-a 复核：未独立实测。** 机制上自洽（两段主键表不接受同键多修订）。

## F10 [LOW] §B 与 `migrate.go` 的对称性

实测往返（老形状库+视图+2 行 → migrate → §B 回滚）：数据逐值一致 ✔；老二进制 `INSERT OR REPLACE` 覆盖语义真的回来 ✔；新二进制正确拒绝（守卫在 `schemaDDL` 之前，被拒的库一字节不变）✔。
两处不对称：① 迁移的 CREATE 是 `schemaDDL()`（表+索引+视图），回滚的 CREATE **只建索引** ⇒ `v_macro_current` 不恢复，效果上无害但 §B 说的是「还原成迁移前形状」而 TASK-003 DoD 明写真实老库是带视图的；② 回滚方向零前置检查（即 F1）。schema 文本差异两处：引号（§B 已警告）+ 索引 DDL 丢掉源码常量的多余空格。

> ▸ **qa-m4c-a 复核：往返成立 —— 我第一轮独立跑过同一条往返（形态 B），结论一致。** ①「视图不恢复」我复核成立：我那次回滚后探针读到的视图定义为空串。

---

## Q3 读取点独立复算（Architect 原文，两种方法互验）

- **方法一·文本**：正则 `FROM[[:space:]]+(macro_observations|v_macro_current)\b`，单位=匹配源码行，范围=`internal/` 与 `cmd/` 下非 `_test.go` 的 `*.go` ⇒ **5 行**：`store.go:158`（写路径冲突检测）、`store.go:225`（obsFrom 当前形态）、`migrate.go:108`、`migrate.go:136`、`migrate.go:140`。
  已知盲点（按构造）：as-of 形态的 `FROM macro_observations` 由 `bitemporal.AsOfQuery` **运行时生成**（`store.go:227`），源码无字面量；`TestNoBareTableReadsOutsideMigration` 用同一条正则、**继承同一盲点**，它的注释只为 `CurrentQuery` 写了这件事**没提 `AsOfQuery`**，豁免清单 3 条之所以仍对得上，是因为两个生成查询它都看不见。
- **方法二·结构（go/parser AST）**：取 selector ∈ {Query,QueryContext,QueryRow,QueryRowContext,Exec,ExecContext,Prepare,PrepareContext} 的 CallExpr，单位=调用点，范围=包 `internal/crisis`（39 文件）⇒ **46 个，非测试 23 个**。归类：读观测数据 5 个且**全部经 `obsFrom()`**（`store.go:232/238/257/275/298`）；写路径基表 4 个（157/164 prepare，174/177 exec）；`migrate.go` 6 个；`sqlite_master` 探形状 2 个（`store.go:76`、`migrate.go:73`）；DDL 1 个（`store.go:47`）；只碰 `crisis_evaluations` 7 个（322/330/342/351/369/380，`LatestSystemEval` 委托无自有调用点）。
- **跨包**：`cmd/atlas` 从不直接开 crisis.db（`cmd/` 里另一处 `sql.Open` 是 `collectors.go:101` 的 qlib warehouse，另一个库）。外部调用点只有 `crisis.go:503`（SeriesWindow）、`crisis_report.go:96`（EvalDates）、`replay.go:22` 经 `st.Reader(ctx)→WindowSince`。
- **结论**：**没有第六个 `macro_observations` 读取点，5 个全部经 `obsFrom()`。** 但 `obsFrom` 收的是「观测表的时点语义」不是「Store 的时点语义」（即 F4b），`store.go:219-222` 那句「加第 N+1 个读点时必漏」**在观测表范围内成立、对整个 Store 不成立**。
- 顺带 **[LOW] 注释漂移**：`EvalDates` 注释（`store.go:284`）称它是「回放的评估日历」，但 `ReplayRange` 用的是 `sr.WindowSince(IndVIX,"",to)`（`replay.go:22`），`EvalDates` 今天唯一调用方是 `cmd/atlas/crisis_report.go:96`。

> ▸ **qa-m4c-a 复核：「5 个读取点、全部经 obsFrom()」与我第一轮独立数出的结果一致（我用的是逐个读调用点，口径=方法二的子集）。** 「豁免清单对得上是因为两个生成查询都看不见」这条我第一轮漏了 —— 我当时只确认了注释为 `CurrentQuery` 写明这件事，**没注意它对 `AsOfQuery` 没写**，接受该补充。末尾那条 `EvalDates` 注释漂移我复核成立（`replay.go:22` 确为 `WindowSince`）。

---

## Architect lens 自述「没查的范围」（Leader 要求原样保留，不得压缩）

生产库与 runtime 目录完全未碰；**并发（WAL、busy_timeout、多写者、as-of 副本共用连接池）一条没测**；**性能未测**（没跑 `EXPLAIN QUERY PLAN`，`AsOfQuery` 的相关子查询能否吃到 `idx_macro_obs_ind_ts` 未知）；`internal/hestia` 本身的正确性（只读了两个 verify 函数、`NewStore`、`Save` 的 verdict 分支和 `schema.go:85-92`）；crisis 的 notify/render/rules/statemachine/derive/eval/ingest 路径（**§A 那条「vix 一炸整轮采集中断」的上抛链没独立复核**）；CONTRACTS 新增节**只实跑了 §B**，§E 的文本序论断只与 `parseAsOf` 对读未独立实测，**§A/§C/§D/§F/§G/§H 未验**；**未跑仓库完整测试套件**（只跑了 `internal/crisis` + `internal/macro/bitemporal` + `cmd/atlas` 三个包）。
