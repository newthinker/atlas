# 第二轮 · Minimalist lens 发现清单（转录 + qa-m4c-a 的复核标注）

- 来源：Minimalist lens 子代理。⚠️ **它的结论发给了 `team-lead`，没有作为返回值交回 `qa-m4c-a`** —— 我看到的最终回复是空的。经 Leader 转述到达，由 `qa-m4c-a` 落盘。
- Leader 已独立核实 F3、F5 两条；**其余八条 Leader 明确标注「未独立核实，原文转述，请自行判真伪」**。
- 下面每条后的 `▸ qa-m4c-a 复核：` 是我自己求值的结果。**没验的写没验。**

---

## F1 [MEDIUM] `cmd/atlas/crisis.go:626-628`「拒绝是这里唯一不产生新错位的选项」可证伪

原文：被接受的整秒 Z 形态已产生 ≤1s 同方向过包含；用来否定转换的 RFC3339Nano 截尾错位只有 ≤1µs（**更小**）；定宽 layout `"2006-01-02T15:04:05.000000000Z07:00"` 实测零错位 ⇒「唯一」为假。且 `store_test.go` 的 `TestAsOfNanoPrecisionBoundary` 把整秒过包含钉成「方向无害」—— **同一失效方向在两处得到相反判断**。

> ▸ **qa-m4c-a 复核：独立实测，成立，且比原文更强。** Go 1.24.4，`row = 2026-07-14T05:42:08.150777000Z`（生产 `fetched_at` 的纳秒形态）：
>
> | 分支 | 结果 |
> | --- | --- |
> | (a) **被接受**的整秒 `…:08Z` | `row <= asof` = **true** —— row 实际晚 **150.777ms**，过包含 |
> | (b) 注释用来**否定转换**的 `RFC3339Nano` 截尾 | 得 `…08.150777Z`（尾零被吃），`row <= trunc` = **true**，过包含但仅 ≤1µs |
> | (c) 定宽 layout `…05.000000000Z07:00` | 纳秒输入**逐字节等于原值**；整秒输入补成 `…08.000000000Z`，`row <= 它` = **false**，**正确排除** |
>
> ⇒ 注释拿「(b) 会产生新错位」当理由否掉转换，而**它自己放行的 (a) 错位大三个数量级**（1s vs 1µs），且 (c) 错位为零。**「唯一」为假，且理由的方向是反的。**
> ⚠️ **这不推翻「拒绝带偏移形态」这个决定本身**（拒绝仍是安全的），坏的是**写下的理由**，而那个理由正是「不必做规范化」的依据 —— 属于「结论对但理由错」，理由才是别人复现时的入口。

## F2 [MEDIUM] `store.go:116-128` / `:183` `sameObservationValue` 的「两边都 nil」生产不可达，两处注释直接矛盾

原文：`Observation.Value` 是 `float64`（非指针），唯一非测试调用点传 `&o.Value` ⇒ 第二参**恒非 nil**；`store.go:119`「所以这条不是假想情形」与 `store_test.go:728`「走不到 `UpsertObservations`（API 写不出 NULL）」直接矛盾；`TestSameObservationValueNullSemantics` 把不可达语义钉成了契约。

> ▸ **qa-m4c-a 复核：成立。** `types.go` 确认 `Value float64`；唯一生产调用点 `store.go:183` 传 `&o.Value` ⇒ 恒非 nil ⇒ `a == nil && b == nil` 从生产路径不可达。两句注释我逐字对读过，确实相反：`store.go:119`「value 列可空（REAL 无 NOT NULL），所以这条不是假想情形」vs `store_test.go:728`「走不到 UpsertObservations」。
> **两句各对一半**：列可空是真的（迁移搬过来的老行可以是 NULL ⇒ `prev` 可为 nil），但「**两边都**没有值」要求双 nil，而入参永不为 nil ⇒ 该分支确实不可达。⇒ 要改的是 `store.go:119` 那句的射程，不是删函数（一边 NULL 一边有值仍要判异值，那条是活的）。
> 🔴 **这条我第一轮读到过并判为「低价值、未报」——判错了。** 它不是风格问题：`TestSameObservationValueNullSemantics` 把一条不可达语义钉成契约，后人改 `Observation.Value` 为指针时，这条测试会让他以为语义已被覆盖。

## F3 [HIGH] `migrate_test.go:230` 恒真断言（Leader 已核实）

`assert.False(t, objectExists(t, db, "macro_observations_new"), "不得留下 _new 残留")` —— `macro_observations_new` 全仓 `*.go` **仅 1 处命中，就是这行断言自己**；迁移走 rename-first（`_v1`），`_new` 这个中间表**从不存在**。映射块 `:5`/`:9` 也还在描述被否决的 `_new` 设计。

> ▸ **qa-m4c-a 复核：第三次独立核实，成立。** `grep -rn 'macro_observations_new' --include='*.go' .` ⇒ 1 行，即断言本身。与本 sprint 刚修掉的 `Contains("+00:00")` 同形。
> ⚠️ 口径差一处，**不影响结论**：Leader 说「`CREATE TABLE` 该表 0 处」，我在 `migrate_test.go` 数到 4 处 `CREATE TABLE`（测试夹具建老形状库）。两把尺 —— Leader 数的应是 `migrate.go` 里的，那里确为 0。**F3 只靠 `_new` 那条 grep，不依赖这个数。不为差额编解释。**

## F4 [MEDIUM] `store.go:69` 掉了否定词，读起来像在认可两份形状判据

原文：原句抄自 `schema.go:11-12` 的禁令（「**不要在别处再 `NewSpec` 一次**——两份…」），抄走后半句丢了框架。实际代码单副本 ⇒ **注释缺陷，不是设计缺陷**。

> ▸ **qa-m4c-a 复核：成立。** 两句对读：
> - `schema.go:11-12`：「**不要在别处再 NewSpec 一次**——两份 Spec 意味着…改一处不会让另一处变红。」（有禁令框架，「两份」是被警告的对象）
> - `store.go:68-69`：「判断本身**复用** migrate.go 的 isBitemporal —— 两份形状判据意味着改一处不会让另一处变红（同 AD-3）。」（**没有禁令**，破折号后半句于是像在给「复用」作注，语义不通）
>
> 代码侧确认单副本：`isBitemporal` 定义 1 处（`migrate.go:71`），非测试调用点 2 处（`store.go:85`、`migrate.go:56`）。⇒ 设计是对的，注释把理由说反了。
> 🔴 **这条是我第一轮派给 Minimalist 时点名要它判的**（「这句话是不是把一个缺陷说成了优点」），它给出的诊断（**漏了否定词**）比我的提问更准 —— 我当时以为是设计问题。

## F5 [MEDIUM] 订正后的部署判据只落进 3 份副本里的 1 份（Leader 已核实，**是 Leader 自己的缺陷**）

`grep -c 'system state:'` → `docs/deployment.md` **3** / `scripts/ops/deploy.sh` **0** / `internal/hestia/CONTRACTS.md` **0**。`CONTRACTS.md:4704` 仍在用被订正掉的 `rc=0` 判据。
同形第二例：`migrate.go:23`「回滚靠两次 RENAME」仍错（实际只 1 处 `RENAME TO`，在 `:125`），订正只写进了 `CONTRACTS.md` §B。

> ▸ **qa-m4c-a 复核：独立复算，逐数一致（3 / 0 / 0）。** Leader 要求不得因是 Leader 而弱化，照办。
> 补一条结构性观察：**这三份文件不在任何任务的 `writes` 里** ⇒ 判据变更后没有任何角色负责同步它们。这与归档里记过的「无 owner 的产物」同形。

## F6 [LOW] `crisis_test.go:1215` 与 `:1391` 自相矛盾；`:1394` 的 `buf` 是死变量

`:1215` 标【实现】并注明【守住】「是错的」，而 `:1391` 仍写【守住】；映射称「空序列且 rc=0」而测试只断言 rc。

> ▸ **qa-m4c-a 复核：成立，与我第一轮的 R-6 指向同一处，已合并。** `buf` 建了却从不断言我第一轮独立发现。标签自相矛盾那半边是它的补充。

## F7 [LOW] `store_test.go:524-525` 的「实测」在守卫自己的射程内没有主体

原文：按守卫自身扫描范围（`internal/crisis/*.go` 去 `_test.go`），带词边界正则与朴素子串**各命中 3 处，差值 0**。

> ▸ **qa-m4c-a 复核：独立复算，差值确为 0**（口径：`internal/crisis` 非 `_test.go` 的 `*.go`，去掉行注释，单位=匹配行；带词边界 3 / 朴素子串 3）。
> ⚠️ **但我把定级说清楚**：注释说的是「**同一行文本**，带词边界 0 次、朴素子串 1 次」—— 那是对一个**构造出来的行**（`FROM macro_observations_v1`）的陈述，作为孤立命题**是真的**。问题在于**守卫射程内不存在这样的行**（`migrate.go:128` 是 `FROM ` + 常量拼接，源码里 `FROM` 后面跟的是反引号不是表名）⇒ 那个「实测」数字**不是从守卫的语料里采的**，它论证的假阳风险在本语料中不存在。
> ⇒ 选带词边界正则**仍然是对的**（防的是将来有人写出那样一行），坏的只是「实测」二字暗示的采样来源。LOW。

## F8 [LOW] `replay_test.go:9`「`executeCrisisReplay` 逐字节不变」被本 sprint 证伪

> ▸ **qa-m4c-a 复核：未独立求值。** 机制上自洽 —— 本 sprint 确实给该函数加了 `asOf` 形参（我在 diff 里看到签名从 6 参变 7 参）。

## F9 [LOW] `store.go:104-106` 描述的「调用方常从可选 `--as-of` 取值」在生产不存在

唯一调用点 `crisis.go:689-696` **刻意分支避开** `AsOf("")`。

> ▸ **qa-m4c-a 复核：成立。** `crisis.go:695-697` 确为 `reader := st; if asOf != "" { reader = st.AsOf(asOf) }`，且其注释明写「分支而不是无条件 `st.AsOf(asOf)`」。⇒ `store.go` 那段把一个**被刻意避开**的用法写成了常态。LOW（注释漂移，无行为后果）。

## F10 [LOW] `migrate.go:127-128` 是观测列清单的第 4 份无保护副本

加列会静默不搬，而三计数是**行数**口径、仍会相等。原文自述：**未构造复现**。

> ▸ **qa-m4c-a 复核：未独立求值，保留其「未构造复现」的自述。** 机制可信：`INSERT INTO … (5 列) SELECT 5 列 FROM _v1` 的列清单与 `obsCols`、`tablesDDL`、`UpsertObservations` 的 INSERT 各自独立；而 `MigrateResult` 三计数只数行不看列 ⇒ 漏搬一列时三数仍相等。**与我第一轮记的「三计数相等但仍错」是同一条的具体实例。**

---

## Minimalist lens 自述「未查范围」（原样保留）

CONTRACTS §D/§H/§I 的**全部生产运行时数字**（rc=137、inode、codesign 字段、sha256、runs=726）一条未验**且不可复现**；§B 回滚 SQL 未实跑（⇒ 以 `qa-m4c-a` 的实跑为准，见 `round2-verdict.md` V-1）；**未跑变异测试** ⇒ 【守住】类断言的守卫力无独立证据；`bitemporal` 包、`.arcforge/` JSON、`store_test.go` 全文未逐行读。
