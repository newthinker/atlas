# 第二轮裁决 — sprint M4-crisis

审查者 `qa-m4c-a`。base `e799bf5a41d9049b0a987f189fab3be50b987186`。

## 产物清单（`.arcforge/docs/05-review/`）

| 文件 | 内容 |
| --- | --- |
| `code-review-m4c-20260919.md` | **第一轮**报告（192 行，16:24 落盘，sha256 `1754175b…`）。⚠️ 其中「§B 回滚 SQL 是对的」一条**已撤回**，见下 V-0 |
| `round2-architect.md` | **第二轮 · Architect lens** 发现清单（F1–F10 + Q3 读取点复算），逐条带我的复核标注 |
| `round2-minimalist.md` | **第二轮 · Minimalist lens** 发现清单（F1–F10），逐条带我的复核标注 |
| `round2-verdict.md` | 本文件（**最终版**） |

**Skeptic lens 至今没有任何产物** —— 它的发现全部丢失。Minimalist 的十条已由 Leader 转交并落盘。见文末「审查过程的失效」。

---

## 裁决：**REJECT**

第一轮我给的是 CONTESTED。**第二轮的证据把它改成了 REJECT**，依据两条（我的角色定义：REJECT = 有 high-severity 且 reviewer 共识一致）：

1. **出现了 CRITICAL**（V-1 / Architect F1）：一份**未提交**的运维回滚手册，按运维最自然的粘贴方式执行会**永久删除观测表、索引与视图**。我独立复验成立。
2. **HIGH 上有跨 reviewer 共识**（V-2）：缺 `verifyCurrentView` 这条，我第一轮独立查出（R-1），Architect 独立查出（F2），两条实验路径不同、结论同向。

---

## 🔴 V-0 先撤回我自己第一轮的一条结论

第一轮报告「已核实通过」一节写着：**「`CONTRACTS.md` §B 的回滚 SQL 是对的」**。**该结论撤回。**

成因：我那次实测用的是 `sqlite3 db "$SQL"` **单参数形态**，而这恰好是该 SQL **唯一安全**的调用方式。我在它必然成立的分支里求了值，并且**没把这个条件写出来**。

⚠️ 这与我在同一份报告 R-4 里批评 Leader 的错误（「判据只在它必然成立的样本上被求值过」）**是同一个形状**，出自我自己之手、写在同一份文档里。**不要因为它在「已核实通过」一节就当它还成立。**

---

## V-1 [CRITICAL] `internal/hestia/CONTRACTS.md` §B 回滚 SQL：按自然粘贴方式执行会全损

- 位置：`internal/hestia/CONTRACTS.md`（**未提交**，+219 那段的 §B），归属 **TASK-008**（该文件在 TASK-008 的 `writes` 里）
- 触发条件（两条，任一即可）：① `_v1` 已按 **§C 的明确授权**删除；② 在**从未迁移过**的库上误跑 §B。两种情况下第 4 步 `ALTER TABLE macro_observations_v1 RENAME TO macro_observations` 都会失败。

**实测**（`/usr/bin/sqlite3` 3.51.0，隔离临时库：已迁移 + `_v1` 已删 + 1 行观测；同一段 §B SQL，只改调用形态）：

| 形态 | rc | `sqlite_master` 存活对象 | 观测行数 |
| --- | --- | --- | --- |
| A：`printf '%s\n' "$SQL" \| sqlite3 db`（粘贴 / heredoc） | 1 | **`[]` 完全为空** | **表已不存在** |
| B：`sqlite3 db "$SQL"`（单参数） | 1 | `macro_observations` / autoindex / `idx_macro_obs_ind_ts` / `v_macro_current` | 1 |

形态 A 的报错原文：
```
Parse error near line 5: no such table: macro_observations_v1
Parse error near line 6: no such table: main.macro_observations
```
第 5、6 行报错后 CLI **继续执行**，第 7 行 `COMMIT` 照常提交了前面已执行的 `DROP VIEW` / `DROP INDEX` / `DROP TABLE macro_observations`。

**为什么这是 CRITICAL 而不是 HIGH**：
- 它是**回滚路径**——人会去用它的那一刻，正是已经出事、压力最大、最可能直接粘贴的时刻。
- `BEGIN` 在场会让人以为有事务保护。**这里没有**：sqlite3 CLI 默认不 `.bail on`，错误不中断脚本，`COMMIT` 照常落盘。
- rc=1，但**损害已提交**。「有报错」与「没事」在这里同时成立，最容易被读成「报了个错但事务回滚了」。
- **不对称**：§B 的正向（迁移）有 `_v1` 占名守卫（`migrate.go:92-105`），**反向零前置检查**。

**为什么现有测试抓不住**：§B 是文档里的手写 SQL，**没有任何测试执行它**。本 sprint 唯一执行过它的是我和 Architect，而我那次用的是安全形态。

**修法**（Architect 给的，我认同）：块首加 `.bail on`；把破坏性步骤挪到最后 —— `RENAME macro_observations→_v2` / `RENAME _v1→macro_observations` / `DROP _v2`；并在块首加一条 `_v1` 存在性前置检查。

**相关**：Architect **F9** 指出 §B 开头那条「先导出」命令也不成立（`.mode insert` 产出裸 `INSERT INTO`，灌回两段主键表撞 `UNIQUE constraint failed (19)`，新修订丢失）。⇒ §B 声称的两道保险（事务 + 先导出）**实测都不成立**。F9 我未独立复验。

---

## V-2 [HIGH] 启动守卫不校验视图定义（两个 reviewer 独立收敛）

- 位置：`internal/crisis/store.go:70-98`，归属 **TASK-003**
- 我的实验（R-1）：视图指向 `macro_observations_v1` ⇒ 守卫放行、读到迁移前冻结值。
- Architect 的实验（F2）：视图掉了 `MAX` 过滤 ⇒ 守卫放行、`Observation` 返回被取代的旧修订、`SeriesWindow` 一个交易日返 2 行、`EvalDates` 返重复日期 —— **本 sprint 立项要消灭的那组缺陷被完整恢复**，全部 `err=nil`。
- 两条路径都无修复通道：库已是三段主键时 `MigrateBitemporal` 提前返回 `AlreadyMigrated` 不碰视图（`migrate.go:61-63`），`CREATE VIEW IF NOT EXISTS` 空转。
- 先例：`internal/hestia/store.go:109-131` 的 `verifyCurrentView` 正是为此而存在；crisis `store.go:64-65` 的注释**点名引用了 hestia 的两个函数、只实现了第一个**。

**✅ 生产已闭合（Leader 2026-09-19 实测）**：生产库视图定义文本里 `macro_observations_v1` 出现 **0** 次，两处引用均为正确的 `macro_observations`。⇒ **生产不受影响**，本条是「相对 hestia 先例缺的一道防御」，不是在途故障。
⚠️ Leader 同时如实声明：它顺手跑的「视图/基表/`_v1` 三者 `MAX(fetched_at)` 一致」那组正对照**没有区分力**（三者同为 `2026-09-18T14:45:05.472055000Z`），因为两表内容按构造相同、指向任一张都同值。**闭合 R-1 的只有视图定义的文本判据，那组数字不支持任何结论。** 这个自我否定是对的，也正是本 sprint 反复要求的「先问这把尺有没有牙」。

⚠️ **归属有争议，请你裁**：这不是任何一条 TASK-003 `done_criteria` 的违反（DoD 承诺的是「老形状拒绝启动并指路」，守卫做到了）。它是**相对于本仓库自己先例缺的一道防御**。按 `review_fix` 派回 TASK-003 可以，开新任务也可以 —— 我倾向**开新任务**，因为 `review_fix` 会让 `rework_count` +1，而 dev 并没有没做到被要求的事。

---

## V-3 [HIGH] `migrate_test.go:230` 是恒真断言（Minimalist F3，Leader 已独立核实，我第三次核实）

- 位置：`internal/crisis/migrate_test.go:230`，归属 **TASK-002**
- 原文：`assert.False(t, objectExists(t, db, "macro_observations_new"), "不得留下 _new 残留")`
- **`macro_observations_new` 在全仓 `*.go` 里只出现 1 次，就是这行断言自己**（口径：`grep -rn 'macro_observations_new' --include='*.go' .`，单位=匹配行，范围=仓库全部 `.go`）。迁移走的是 rename-first（`_v1`），**从来不存在 `_new` 这个中间表**。⇒ 该断言**永远为真，不可能变红**。
- 与本 sprint 刚修掉的 `assert.Contains(err.Error(), "+00:00")` 同形：**断言的目标在被断言对象里构造上不可能出现**。
- 同一个测试里另外 4 条断言是有效的（PK 文本、`fetched_at TEXT NOT NULL` 缺席、`_v1` 不残留、行数 2），所以**测试整体仍有价值**，坏的只是这一条。

> ⚠️ 口径更正，不影响结论：Leader 转述时说「`CREATE TABLE` 命中 0」，我在 `migrate_test.go` 里数到 **4** 处 `CREATE TABLE`（测试夹具建老形状库用的）。两个数是两把尺（Leader 大概率在数「migrate.go 里的 CREATE TABLE」，那里确为 0，`TestMigrateReusesSchemaDDL` 正是钉这个的）。**F3 的结论只靠 `_new` 那条 grep，不依赖这个数**，成立不受影响。**我不为这个差额编解释，只标明两把尺不同。**

---

## V-4 [MEDIUM] 本 sprint 订正过的部署判据只落进 3 份文件里的 1 份（Minimalist F5 —— Leader 自己的缺陷）

口径：`grep -c 'system state:' <file>`，单位=匹配行。

| 文件 | 命中 |
| --- | --- |
| `docs/deployment.md` | **3** |
| `scripts/ops/deploy.sh` | **0** |
| `internal/hestia/CONTRACTS.md` | **0** |

`deploy.sh:28-33` 的注释与 CONTRACTS §H 都写了「部署后必须实际跑一次 `crisis status`」，但**都停在旧判据**（跑一次 / rc），没有跟上 `deployment.md:292` 订正后的「**判性质：首行须为 `system state:`**」。⇒ 照 `deploy.sh` 或 CONTRACTS §H 操作的人，拿到的仍是那个**与它要防的失效同形**的判据（M-17 原缺陷）。

**Leader 要求如实记录且不得因是 Leader 而弱化，照办。** 另附一条我第一轮的 R-4：`deployment.md` 那条订正后的判据**今天成立，但写下的理由不是真正起作用的那个**（真正挡住诱饵库的是形状守卫，不是「空库恒为 no evaluations yet」），且那组实测在临时目录做的（那里不存在 `data/crisis.db`）。

⚠️ **这三份文件不在任何任务的 `writes` 里**（TASK-008 的 `writes` 是迁移日志 + CONTRACTS.md）。⇒ **没有角色负责在判据变更后同步它们**，这与归档里记过的「无 owner 的产物」是同一形态。

---

## V-5 [MEDIUM] `parseAsOf` 注释「拒绝是唯一不产生新错位的选项」为假，且理由方向是反的

- 位置：`cmd/atlas/crisis.go:626-628`，归属 **TASK-007**。来源 Minimalist F1，**我独立实测**（Go 1.24.4，`row = 2026-07-14T05:42:08.150777000Z`）：

| 分支 | `row <= asof` | 错位量 |
| --- | --- | --- |
| (a) **被接受**的整秒 `…:08Z` | **true**（row 实际晚 150.777ms） | 最大 **1 秒** |
| (b) 注释据以**否定转换**的 `RFC3339Nano` 截尾（得 `…150777Z`） | true | ≤ **1 微秒** |
| (c) 定宽 layout `…05.000000000Z07:00` | **false（正确排除）** | **0** |

⇒ 注释拿「(b) 会用一个无声的错换另一个」当理由否掉规范化，**而它自己放行的 (a) 错位大三个数量级**，(c) 则完全无错位。「唯一」为假。
⚠️ **这不推翻「拒绝带偏移形态」这个决定** —— 拒绝仍是安全的。坏的是**理由**，而该理由正是「不必做规范化」的依据。属于「结论对但理由错」：结论正确让判断永不被复查，而理由是别人复现时唯一的入口。
⚠️ 同一失效方向在两处得到相反判断：`store_test.go` 的 `TestAsOfNanoPrecisionBoundary` 把 (a) 的过包含钉成「方向无害」，而 `crisis.go:626` 用同方向的过包含否掉了 (c)。

## V-6 [MEDIUM] 仓库 `bin/atlas` 从未用本 sprint 的代码重建过 —— 拿它验本 sprint 会得到无声的假结果

- 来源：Leader 提供的新事实，**我独立复核成立**。口径：`strings bin/atlas | grep -c <符号>`，单位=命中次数。

```
bin/atlas  mtime 2026-09-18 18:11   （本 sprint 的迁移与部署发生在 09-19）
  verifyBitemporalShape    0
  migrate-bitemporal       0
  as-of                    0
  v_macro_current          0
  crisis                 318   ← 正对照：二进制确实含 crisis 代码，grep 本身有效
```

⇒ 任何人在仓库里用 `./bin/atlas` 验本 sprint 的行为，会得到一个**语法正确、输出正常、结论全错**的结果。
🔴 **它给出的假结论方向可以与真相完全相反**（Leader 补充的细节）：那次它拿到的是 `rc=0` + `no evaluations yet` —— **一个完全正常的输出**。若没有接着去查二进制版本，它会作为「§G2 成立」的证据进入报告，**而 §G2 恰恰是反过来的**（守卫会响亮拦住）。⇒ 这个坑不只是「不告诉你」，它**会把你推向相反的结论**。
🔴 **它已经claim 过一个受害者**：Leader 在复核 R-3 时第一次就用了 `bin/atlas`，得到 `rc=0` + `no evaluations yet`，而正确结果是守卫 `rc=1` 拦截。**Leader 自己查出并如实披露了这次仪器错误**，否则那组数字会作为「§G2 成立」的证据留在记录里。
⚠️ 与 §G2 那个坑的关键差别：**§G2 的坑已被守卫堵上（会响亮失败），这个没有任何东西会告诉你。** 判据「错了的话，什么东西会告诉我？」在这里的答案是「没有」。
- 归属：**无 owner**（`bin/` 不在任何任务的 `writes` 里，也不受 `deploy.sh` 管）。建议处置：要么在仓库 `bin/` 放一个 `STALE` 标记或删掉它，要么在 `deployment.md` 明写「验证一律用现场构建的二进制，不要用 `bin/atlas`」。

## 其余发现

第二轮 Architect 的 F3/F4/F5/F6/F7/F8/F9/F10 与 Q3 复算，**逐条连同我的复核标注放在 `round2-architect.md`**，不在此重复。其中我要单独点出三条值得你优先看：

- **F4(c)**：`store.go:102-104` 的文档**把危害说反了** —— 它警告「不该调用两次」，实测双 `Close` 无害；真正会炸的是 `defer copy.Close()`（关掉父的连接池，父此后读报 `sql: database is closed`）。我未独立复验，但若成立，这是「注释与代码不符」的第三例。
- **F7**：`isBitemporal` 对 `PRIMARY KEY (ts,indicator,fetched_at)`（**无空格**）判假，而**错误文案会说「still has the legacy two-part primary key」——一句假话**，并指示运维去跑 migrate。我第一轮想到过并判 LOW（找不到可达路径）；Architect 指出 **§B 运维手册本身就是手写 SQL**，我接受升到 MEDIUM。
- **Q3 的一条补充**：`TestNoBareTableReadsOutsideMigration` 的正则**看不见运行时生成的两个查询**（`CurrentQuery` / `AsOfQuery`）。它的注释只为 `CurrentQuery` 写明了这件事、**没提 `AsOfQuery`**。豁免清单 3 条之所以仍对得上，正是因为两个都看不见。我第一轮漏了后半句。

---

## ⚠️ 处置成本更正：改 `CONTRACTS.md` 不是「顺手改一下」

第一轮报告 R-3 里我写了「该段未提交，**现在改零成本**」。**这半句错了，Leader 已更正，我确认。**

`internal/hestia/CONTRACTS.md` 在 **TASK-008 的 `writes`** 里（Leader `jq` 直读八个任务文件核实，只有 TASK-008 提到它），而 **TASK-008 已 `verified`**。⇒ 改它要走 `verified → review_fix → in_progress` 这条边、由 dev 领回改、再重新验证。

**「未提交」只说明 git 层面便宜，不说明流程层面便宜。** 这不改变 V-1 / R-3 的定级，只改变处置成本 —— 请人类按「要开一轮返工」而不是「顺手改一下」来决策。
⚠️ 受影响的不止 R-3：**V-1（CRITICAL，§B 回滚 SQL）改的也是这个文件**，同样要走返工边。

## 归属汇总（供你按 task 派 `review_fix`）

| 发现 | 级别 | 归属 | 建议 `reason_class` |
| --- | --- | --- | --- |
| V-1 §B 回滚 SQL 全损 | CRITICAL | **TASK-008** | `task_defect` |
| V-3 `migrate_test.go:230` 恒真断言 | HIGH | **TASK-002** | `task_defect` |
| V-2 缺 `verifyCurrentView` | HIGH | TASK-003（**建议改开新任务**，见 V-2） | 若派回：`task_defect` |
| V-4 判据未同步到 3 份文件 | MEDIUM | **无 owner**（不在任何任务 `writes` 里） | 需你新建任务或自行处置 |
| V-5 `parseAsOf` 注释「唯一」为假 | MEDIUM | **TASK-007** | `task_defect` |
| V-6 `bin/atlas` 从未用本 sprint 代码重建 | MEDIUM | **无 owner**（`bin/` 不在任何 `writes` 里） | 需你处置 |
| Architect F4/F7/F9 等 | MEDIUM | TASK-006 / TASK-002 / TASK-008 | 见 `round2-architect.md` 逐条 |

**已知残留**（你先前给的五条）不重报。按你的要求单独列出一条进 PENDING-MECHANISMS：

> `internal/crisis/migrate.go:23` 注释「回滚靠**两次 RENAME**」为假（实现只 1 处 `RENAME TO`）。**已三级传播**：注释 → Leader 写的 TASK-008 DoD（原样抄了那句）→ dev 发现真相。
> ⚠️ 补充一条本轮新证据：Architect **F10** 实测往返后指出，§B 的回滚**只建索引、不恢复 `v_macro_current`**，而 TASK-003 的 DoD 明写真实老库是带视图的 ⇒ 「还原成迁移前形状」这句在 §B 里**也**不完全成立。**同一条假注释影响了第四处。**

---

## 🔴 审查过程的失效（与裁决同级，不要压缩）

1. **三个 lens 子代理的返回值全是空的。** Skeptic / Architect / Minimalist 各跑 28–41 次工具调用、合计约 67 万 token，作为**工具返回值**交回的是「结论已交回，不再回应。」／「静默（第 13 次）…」／「No action.」。三份结论后来分别经 **teammate message** 到达：Architect 的到了我这里（完整，已转录），**Minimalist 的到了 Leader 那里**（我只拿到 Leader 转述的 F3、F5 两条），**Skeptic 的我至今没有收到任何内容**。
   **Minimalist 的十条后来由 Leader 转交，已落盘为 `round2-minimalist.md`（其中 F1/F2/F4/F7 我独立复核，F3/F5 三方各自核实过）。**
   ⇒ **本次审查最大的单点故障，正确的记法是「子代理的结论可以绕过父代理直接到 Leader，也可以谁都不到」** —— Minimalist 走了第一条（结论到了 Leader，没到我），Skeptic 走了第二条（谁都没到）。**不要记成「Architect 没给 Leader 发消息」**：Leader 确认 Architect 从未给它发过任何消息，那条链路**根本不存在**，而不是「传了一手丢了限定词」。
   ⇒ **Skeptic 的发现全部永久丢失（其负责的四个方向已于第四轮由我另行覆盖，见文末）。** 它原本负责的范围是：`UpsertObservations` 三分逻辑的批内重复 / NULL / 事务内可见性；`migrate.go` 步骤表在半迁移库等病态输入下的静默失败；`MigrateResult` 三计数「相等但仍错」；`isBitemporal` 文本判据的误判构造；as-of 端到端是否有读取点没接上。**其中第 3、4 项后来由 Architect F7 与我自己部分覆盖，其余没有任何人查过。**
2. **Minimalist lens 的缺口已补上，而且它证明了我先前的担心是对的。** 它负责的正是「逐条核对注释里的可验证断言」这一类，十条里**有七条属于这一类**（F1 `唯一` / F2 `不是假想情形` / F4 漏否定词 / F6 标签自相矛盾 / F7 `实测` 无主体 / F8 `逐字节不变` / F9 描述不存在的用法）。本 sprint 已知五个缺陷里有两个正是这一类（`obsSelect` 过期注释、`migrate.go:23` 两次 RENAME），本轮又新增两例（F3 恒真断言、F4c 文档说反）⇒ **这一类在本仓库的密度明显高于其他类，而针对它的系统性检查没有跑完。**
3. **一条关于协作链路本身的观察（归因已更正）**：Leader 报告 `.arcforge/docs/05-review/` 为空，而第一轮报告 16:24 已落盘。
   🔴 **我最初把成因判成「转述时丢了 Architect 的快照限定词」，那是错的。** Leader 更正：它**从未收到过 Architect 的任何消息**（给它发消息的是 Minimalist），「目录为空」是**它自己在主仓库跑的 `ls`**，时间在 16:24 之前 —— **当时那个读数是对的，只是发消息前没有重跑**。
   ⇒ 真实形状是「**读数过期未重取**」，不是「转述丢限定词」。**两者处方相反**：前者要「发断言前重新求值」，后者要「转述时保留限定词」。按错的那个开处方，下次照样撞。
   ⚠️ **我这个误判本身属于同一族**：我从两份文本的表面相似**推断**出一条因果链（Architect 说过快照警告 → Leader 的说法必然源于它），而没有去问链路是否存在。这正是「别从上游文本推断对方做了什么，去看它实际做了什么」。

## 我明确没有查的范围（第一轮 + 第二轮合并）

- Skeptic lens 的全部范围（见上，已丢失）。
- Minimalist lens 未回来的部分：注释可验证断言的逐条求值、`done_criteria → test mapping` 映射块抽查。
- Architect F1 我复验了；**F3/F4/F5/F6/F8/F9 我未独立复验**，逐条在 `round2-architect.md` 里标了。
- CONTRACTS 新增节：只实跑了 §B；**§A/§C/§D/§E/§F/§G/§H/§I 未验**（含全部生产运行时数字：rc=137、inode、codesign 字段、sha256、runs=726）。
- 生产库与生产二进制**一个字节未碰** ⇒ V-2 的「生产当前视图指向哪张表」未核实。闭合只需一条只读查询：
  `sqlite3 <生产库> "SELECT sql FROM sqlite_master WHERE type='view' AND name='v_macro_current';"`
- 并发（WAL / busy_timeout / 多写者 / as-of 副本共用连接池）**一条没测**；性能未测（无 `EXPLAIN QUERY PLAN`）。
- 八份 `discoveries/TASK-00{1..8}.json` 与八份 `04-test/*-verification.md` **未读**。
- `plan.md` 802 行只读了 M-16~M-22；迁移日志 600 行只读约 60 行。
- 未跑仓库**完整**测试套件（只跑了 `internal/crisis` + `cmd/atlas` + `internal/macro/bitemporal` 三个包，全绿）。


---

# 第三轮增补（2026-09-19，Leader 复现 V-1 之后）

## V-7 [HIGH] §B 的**正常**回滚路径也是坏的：rc=0、行数对、而读路径没了

- 位置：`internal/hestia/CONTRACTS.md` §B（未提交），归属 **TASK-008**
- Leader 在生产库隔离副本上实测（`_v1` **仍在**，即回滚的正常场景 + 管道形态）：

```
rc=0   表=1   行数=31098   索引=1   视图=0
                                    ^^^^^
```

`v_macro_current` **没有被恢复** —— §B 的 SQL 里 `DROP VIEW` 有、重建没有。**退出码 0、行数逐值对得上、没有任何东西会告诉你读路径已经坏了。**

🔴 **这条我在第一轮就有证据，却没有把它升级成发现。** 我第一轮跑 §B 往返时，探针打出的是 `库中视图定义 : `（空串），我把它作为 Architect F10 的一个 ① 子项记进了 `round2-architect.md`，标成 LOW。**我有数据、看见了、定级错了。** 正确的读法是：「rc=0 + 计数全对 + 读路径坏掉」与 V-1 是同一失效族（判据全过而结果错），只是这一条**发生在被认为成功的路径上**，因此更隐蔽。

⇒ **§B 现在有三条独立缺陷**：① V-1（`_v1` 已删时静默删库）；② 本条（正常路径不恢复视图）；③ F9（「先导出」那条不成立）。**它声称的两道保险都不成立，而它本身还有两条独立失效路径。**

## ✅ 修复已实测有效（Leader 跑的，我采信并记下其证据形态）

`_v1` 已删 + SQL 首行加 `.bail on` + 管道形态：

```
rc=1   表=1   行数=31098   索引=1   视图=1
Parse error near line 6: no such table: macro_observations_v1
```

数据、索引、视图**全部完好** —— `.bail on` 让 CLI 在第一个错误处停下，`COMMIT` 不再执行，事务正常丢弃。**一行修复。**

⚠️ Leader 另报一次**意外的正对照**：它自己把前置断言的 SQL 写错（`RAISE_ABORT` 不是有效列名），`.bail on` 照样安全停下、数据完好 ⇒ 这道闸对「SQL 写错了」这类更常见的情况同样有效。**意外产生的正对照比刻意构造的更有说服力，因为它不可能是为了通过而设计的。**

建议 §B 改三处：首行 `.bail on`；回滚块末尾补 `CREATE VIEW v_macro_current …`（逐字取自 `schema.go`）；按 F9 改掉或删掉「先导出」那条。

## V-8 [MEDIUM] 运维手册与应用跑在两个语义不同的 SQLite 上

- 我在复核 `migrate.go:115-119` 那句「`ALTER TABLE RENAME` 会**静默改写视图定义**（已实测）」时，先用 `sqlite3` CLI 测，得到**视图未被改写** ⇒ 一度准备报「该注释为假」。
- **换成代码实际使用的驱动重测，结论相反**：

| 引擎 | `legacy_alter_table` | `RENAME` 后视图 |
| --- | --- | --- |
| `modernc.org/sqlite`（**应用**） | **0** | 被改写成 `FROM "macro_observations_v1"` |
| `/usr/bin/sqlite3` 3.51.0（**运维手册**） | **1** | **不改写** |

⇒ **`migrate.go:115-119` 的注释是对的**，`DROP VIEW` 那一步是**承重的**，不是防御性的。
⇒ 但**同一条 SQL 在运维手册里与在代码里语义不同**。任何人用 `sqlite3` CLI 验证迁移/回滚的视图相关行为，会得到与生产不一致的结论。这与 V-6（`bin/atlas` 停在旧版）是同一族：**仪器与被测对象不是同一个，而它照样给出自信的数字。**
🔴 **这一族本轮已第三次出手**：V-6 让 Leader 的 R-3 首次复核得出相反结论；本条差一点让我把一条正确的注释报成缺陷；Leader 自己也记了两次仪器错误。**我是靠「结论与注释相反时先怀疑仪器」才没报错**，不是靠看出来。

## 注释可验证断言逐条求值（Minimalist 那一问的补做）

口径：`internal/crisis/{store,migrate,schema}.go` + `cmd/atlas/crisis*.go` 的注释行，筛出含「实测 / 唯一 / 恰 / 只有 / 不经 / 构造上 / 全部 / 两份 / 第 N」等断言标记者，逐条到代码或 sqlite 里求值。**下列五条全部为真**（引擎已按 V-8 校正为应用侧驱动）：

| 注释 | 断言 | 求值结果 |
| --- | --- | --- |
| `store.go:144` | 同键两修订时，基表查旧修订得 1 行、视图得 0 行 | ✅ 基表 1 / 视图 0 |
| `migrate.go:115-119` | `ALTER TABLE RENAME` 静默改写视图定义 | ✅ 真（**驱动侧**；CLI 侧为假，见 V-8） |
| `migrate.go:120-124` | 索引名全局唯一；RENAME 后索引跟旧表走，`CREATE INDEX IF NOT EXISTS` 静默跳过 | ✅ 索引留在 `_v1`，新表无索引且不报错 |
| `migrate.go:38` | sqlite 的 `ALTER TABLE RENAME` 能被事务回滚 | ✅ `ROLLBACK` 后表名还原 |
| `schema.go:36-38` | SQLite 主键**允许 NULL**（故 `fetched_at` 要 `NOT NULL`） | ✅ 同键两行 NULL 都插入成功 |

**已证伪的在前面**：V-5（`crisis.go:628`「唯一」）、`round2-minimalist.md` 的 F2/F4/F7/F8/F9。

## 映射块抽查（`done_criteria → test`）

口径：四个测试文件的 `Context Checkpoint` 映射块里用 `→ TestXxx` 点名的测试，与仓库实际定义的 `func TestXxx` 取差集。

```
映射块点名: 74 个      仓库实际定义: 444 个
点名但不存在: 0 个
```

⇒ **没有指向不存在测试的映射。** 映射块的**失效形态不是「指错测试」，而是「测试存在但断言的不是那条性质」**（V-3 的恒真断言、V-6/F6 的标签自相矛盾都是这一种）—— 存在性检查查不出这一类，**这条负面结果不构成「映射块没问题」**。

## 第三轮之后的归属增补

| 发现 | 级别 | 归属 | reason_class |
| --- | --- | --- | --- |
| V-7 §B 正常路径不恢复视图 | **HIGH** | TASK-008（与 V-1 同一文件，**一次返工可一并修**） | `task_defect` |
| V-8 手册与应用跑在语义不同的 SQLite 上 | MEDIUM | 无 owner（文档/流程） | 你处置 |

## 第三轮没查的范围

- ~~Skeptic lens 的四个方向仍然全部没人查~~ 🔴 **本句已被第四轮取代，保留以留痕**：Leader 随后指派补跑，**四个方向已于第四轮由我直接覆盖**，并构造复现了三计数那条（→ V-9）。
  ⚠️ 但请区分两件事：**「方向已被覆盖」不等于「Skeptic 的发现已找回」** —— 我跑的是它负责的**问题**，不是它得出的**答案**。它当时可能查到了我没查到的东西，那部分仍然永久丢失。
- Architect F4(c)（文档把 `Close` 的危险半边说反）与 F9（「先导出」）**按 Leader 决定不复验**。
- 注释求值只覆盖了**生产代码**的注释；**测试文件里的大段说理注释未逐条求值**（V-3 那类恰好藏在那里）。
- `.bail on` 修复是 **Leader 实测的，我未独立复跑**。


---

# 第四轮：Skeptic 四个方向的补做（Leader 2026-09-19 指派）

**方法（V-8 的直接推论）：本轮全部探针一律走 `modernc.org/sqlite`（即应用实际使用的驱动），不使用 `sqlite3` CLI。** S1–S6 均为隔离副本内的 Go 测试，建库与断言都经该驱动 ⇒ `legacy_alter_table=0`，与生产语义一致。**本轮没有任何一个结论来自 CLI。**

范围就是原派给 Skeptic lens 的四项，不扩。全部在 `git archive HEAD` 的隔离副本里跑，主工作区未动（收尾核实：`store.go` sha256 仍 `f081dd6dfdd73c99…`，`git status` 与开工时逐行一致，主仓库无 `zz_` 测试文件残留）。

## 方向 1｜`UpsertObservations` 批内重复 / NULL / 事务内可见性 —— ✅ 未发现缺陷

| 探针 | 输入 | 结果 |
| --- | --- | --- |
| S1 | 一批三条，后两条**同三段主键、异值**（10 vs 99） | ✅ 报错 `already has a different value … refusing to overwrite`，**且第一条（无关的 01-01）也未落盘** ⇒ 整批回滚确实生效 |
| S2 | 一批两条，**完全相同** | ✅ `err=nil`，基表 1 行 ⇒ 事务内可见性正常（第二条查到了同事务中尚未提交的第一条），**不会撞主键** |
| S3 | 前置行 `value IS NULL`，新值 `0` | ✅ 报错 `(have NULL, got 0)` ⇒ 「0 不是没有值」的语义在**真实路径**上成立 |

⇒ **三条都正确。** 附带一个结论：S2 证明冲突检测**必须**留在同一事务内（它依赖未提交行可见）；S3 证明 `sameObservationValue` 的**活分支**（一边 nil）工作正常 —— 不可达的只有「两边都 nil」那一支（见 `round2-minimalist.md` F2）。

## 方向 2｜`migrate.go` 病态输入 —— ✅ 未发现新缺陷

- S4：`macro_observations_v1` 这个名字被一个**视图**（而非表）占用时，占名守卫仍然拦住并给出正确文案。
  原因是判据写的是 `SELECT COUNT(*) FROM sqlite_master WHERE name = ?` —— **不带 `type` 过滤**，因此对表/视图/索引/触发器一视同仁。**这一处比它的注释所声称的更健壮**，是本次审查里少见的「实现强于文档」。
- 半迁移库在结构上不可达：全部步骤在单个事务内，且 `ALTER TABLE RENAME` 可被回滚（本文件第三轮已实测）。
- 已有覆盖的病态输入（NULL `fetched_at` 原子失败、空库、已迁移库、缺表、缺文件）我复核了对应测试确实存在且断言有效。

## 🔴 方向 3｜`MigrateResult` 三计数「相等但仍错」—— **已构造复现**（Minimalist F10 原标「未构造复现」）

**构造**：只改 `schema.go` 的 `tablesDDL`，给观测表加一列 `note TEXT`（模拟将来一次正常的 schema 演进）；老库为两段主键且 `note` **已有值**；然后跑 `MigrateBitemporal`。

```
三计数: RowsBefore=3  RowsAfter=3  ViewRows=3   三者相等 = true     ← 判据说「好」
note 有值的行数: _v1=3   迁移后=0                                   ← 数据已丢
🔴 3 行的 note 没有被搬过来，而三计数全部相等、迁移报成功
```

- 根因：`migrate.go:127-128` 的 `INSERT INTO macro_observations (5 列) SELECT 5 列 FROM _v1` 是**硬编码列清单**，是观测列清单的第 4 份无保护副本（另三份：`tablesDDL`、`obsCols`、`UpsertObservations` 的 INSERT）。
- **为什么判据抓不住**：`MigrateResult` 的三个计数是 **`COUNT(*)` 行数口径**，而丢失发生在**列**这个维度。`migrate.go:13` 写的是「三个计数是运维现场**唯一的**判据：三者相等才算好」——**这句话在加列之后为假**，而它恰恰是运维唯一被告知要看的东西。
- 可达性：需要一次未来的 schema 演进（加列）。**今天无害**，但加列是双时态表最可预期的演进方向（`CONTRACTS.md` 里 hestia 的字段数「刚从约 20 涨到 54 且还会再涨」正是先例）。
- **与 V-2 同构**：都是「守卫/判据检查的维度，比它声称守住的集合窄一维」——V-2 查表不查视图，本条数行不数列。
- 建议：迁移的列清单从 `pragma_table_info` 派生，或在迁移后加一条列集合比对（`_v1` 的列 ⊆ 新表的列，且逐列非空计数一致）。

## 方向 4｜as-of 端到端是否有读取点没接上 —— ✅ 未发现遗漏

- S5：五个观测读方法在 `AsOf(早于所有 fetched_at)` 下**全部**返回空/nil（`Observation`、`LatestObservation`、`SeriesWindow`、`SeriesSince`、`EvalDates`）⇒ 五个读取点**都真的接上了** as-of，不是只有被测过的那几个。
- 端到端链路复核：`executeCrisisReplay` 在 `ReplayRange` 之后**不再触碰 store**（`sed` 取函数体，`st.`/`reader.` 命中仅 `ReplayRange` 那一处）；`ReplayRange` 只经 `sr.WindowSince` 与 `EvalDay(… sr …)`，历史走 `MemHistory` 零读库。
- ⚠️ **但方向 4 的真实缺口不在观测读取点，而在评估读取点** —— 那是 `round2-verdict.md` V-5 相邻的 Architect F4(b) / 我的 R-5：`crisis_evaluations` 的五个读方法完全不受 `asOf` 约束。**本方向的「没有遗漏」只在观测表范围内成立。**

## 第四轮归属增补

| 发现 | 级别 | 归属 | reason_class |
| --- | --- | --- | --- |
| V-9 三计数是行数口径，加列后静默丢数据而判据仍报「好」 | **MEDIUM** | **TASK-002**（`migrate.go`） | `task_defect` |

方向 1 / 2 / 4 **未产生新发现**。这是阴性结果，不是「没查」—— 探针、输入与结果见上。

## 第四轮没查的范围

- 并发（WAL / `busy_timeout` / 多写者 / as-of 副本共用连接池）**仍然一条没测**，四轮下来无人碰过。
- 性能（`EXPLAIN QUERY PLAN`、`AsOfQuery` 相关子查询能否吃到索引）**仍未测**。
- 方向 2 我只构造了「`_v1` 被视图占名」这一种病态输入；**磁盘满、库被并发进程持锁、WAL 残留**等运行时病态未测。
- `.bail on` 修复仍是 Leader 实测，我未独立复跑。
