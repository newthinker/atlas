# TASK-005 验证报告 — Lookup（Querier 接口 + 状态查询）

- **验证者**：test-agent-18（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：commit `f2205ac`（`lookup.go` 67 行 + `lookup_test.go` 334 行）
- **验证基线**：`feat/macro-bitemporal` @ `f2205ac1b462436ed322e11a5ed4cd71a25ab4e1`
- **验证环境**：隔离 worktree `../wt-verify-TASK-005`，主工作区零污染；go1.24.4 darwin/arm64
- **判定：PASS（verified）** —— 附一条**已实证的、需要 Leader 处理的注入面残留缺口**（不构成本任务返工）

> 标注约定：**【实测】**= 本报告作者在隔离 worktree 中亲自运行命令并观察到输出；
> **【推断】**= 由代码阅读或已实测事实推出、未单独跑命令验证。

**DoD 条数**：我按任务文件逐条点数为 **18 条**（functional 3 / boundary 4 / error_handling 7 /
non_functional 4）；派验消息写 17。以文件为准，本报告按 18 条列矩阵。

---

## 一、完成标准覆盖矩阵

| # | 完成标准 | 对应测试 | 守护证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 命中/未命中；多版本取最大 | `TestLookupHitAndMiss`、`TestLookupUsesEveryKeyColumn` | F0-a（MAX→MIN）、F0-b（`!latest.Valid` 取反）、F0-c（命中返 false）、F0-d（WHERE 只用首列）全 KILLED | **PASS**【实测】 |
| functional[1] | 两套 Spec 形状行为一致 | HitAndMiss / IgnoresInsertOrder / UsesEveryKeyColumn 均对 `bothShapes` 跑 | 变异红名单中 hestia 与 crisis 子测试成对出现 | **PASS**【实测】 |
| functional[2] | `*sql.Tx` 真的传进去 | `TestLookupAcceptsTx` | 走真事务真查询，非编译期接口断言 | **PASS**【实测】 |
| boundary[0] | 空表 → Exists:false 且**不是** error | HitAndMiss 第一段 | B0-a（空表改返 error）KILLED | **PASS**【实测】 |
| boundary[1] | 乱序插入后仍取最大 | `TestLookupIgnoresInsertOrder`（bothShapes） | F0-a、B1-a（MAX→`LIMIT 1`）KILLED | **PASS**【实测】 |
| boundary[2] | **契约 T5**：零值 Spec 必须报错 | `TestLookupRejectsZeroSpec` | B2-a（删检查）、B2-b（错误信息不提 NewSpec）KILLED | **PASS**【实测】 |
| boundary[3] | C8 适用范围裁定须写进 discovery | — | discovery 载有裁定执行 + 一处加强 | **PASS**【实测】 |
| error_handling[0] | Key 键集不匹配 → 报错 | `TestLookupRejectsMismatchedKey` | E0-a KILLED | **PASS**【实测】 |
| error_handling[1] | SQL 错误被包装且**含表名** | `TestLookupWrapsSQLError`／「列不存在」子测试 | E1-a（删表名）、E1-b（`%v` 断 `%w` 链）KILLED | **PASS**【实测】，见 3.2 |
| error_handling[2] | **契约 T1 对偶**：行为侧证明 Lookup 真调了 checkKey | 同 [0] | E0-a（跳过 checkKey）KILLED，红的正是该测试三个子用例 | **PASS**【实测】 |
| error_handling[3] | **注入防护** | `TestLookupRejectsInjectionInKeyValues` | **MI-1 = DoD 原文变异判据，KILLED**，且红名单**排他性**只有它 | **PASS**【实测】，**但见第二节 F1** |
| error_handling[4] | 标识符只能来自 `Spec`，不得自行拼接 | — | 静态核查：`lookup.go` 进 SQL 的标识符仅 `spec.table` / `spec.businessKey` 各列 / `spec.revisionCol`，**无自造 alias** | **PASS**【实测】 |
| error_handling[5] | **「只加一个 getter」也在禁止范围** | — | `spec.go` 五个 commit blob hash 全同 | **PASS**【实测】，见 3.3 |
| error_handling[6] | ISO 形态**三选一并写下来** | `TestLookupRevisionFormIsCallerGuaranteed` | 选「本包不管」+ `lookup.go:29-39` 写明 + 测试钉住；E5-a（加形态校验）KILLED | **PASS**【实测】，见 3.4 |
| non_functional[0] | 绿/0 SKIP/头注释/中文/`?` 占位符/`go vet`/`-race` 显式排除 | — | 120 PASS、0 SKIP、vet exit 0、`lookup_test.go:38-39` 显式记录 -race 排除 | **PASS**【实测】 |
| non_functional[1] | 包级封装重查 | — | 我独立 grep 复现 | **PASS**【实测】，见 3.3 |
| non_functional[2] | 报「存活」须自证三条 | — | 唯一存活变异 MI-2 由**我的 harness 独立跑出**三条 | **PASS**【实测】 |
| non_functional[3] | 子代理报「无改动」后须 diff 核实 | — | dev 以 md5 核实，本次声明属实并全量复跑 | **PASS**【实测】 |

### 基础事实【实测】

```
$ go test -count=1 -v ./internal/macro/bitemporal/
ok  ...  PASS=120  SKIP=0  FAIL=0
$ go test -cover  → coverage: 100.0% of statements（Lookup 100.0%）
$ go vet ./internal/macro/...  → exit 0，无输出
$ gofmt -l lookup.go lookup_test.go → 无输出
$ git show --stat f2205ac -- go.mod go.sum → 无输出（未触碰依赖）
```

声明 `writes = [lookup.go, lookup_test.go]`，`git show --stat f2205ac` 实际改动恰为这两个文件。
**无越界申报。** 公开面新增 `Querier` 与 `Lookup`，均为本任务的正当产出。

### 变异测试汇总：15 个，**14 KILLED / 1 存活**

其中 **7 个是我新增的**：MI-2（`%q` 拼接）、MI-3（args 全填首列）、B0-a、B1-a、B2-b、E1-b、E5-a。

---

## 二、头号目标：注入守护 —— 判据满足，但我打出了一个**真缺口**

### 2.1 DoD 原文判据（MI-1）：**KILLED**，且 reviewer 的前提实测成立【实测】

按 DoD 原文注入（靶点经 `count(old)==1` 校验，多点替换）：
把 `where[i] = c + " = ?"` 改成 `fmt.Sprintf("%s = '%s'", c, key[c])`，删掉 `args` 与其传参。

全套跑完，**PASS=110（基线 120），红 10 条，红掉的顶层测试函数只有一个**：

```
TestLookupRejectsInjectionInKeyValues
TestLookupRejectsInjectionInKeyValues/{hestia,crisis,single-key}
TestLookupRejectsInjectionInKeyValues/{hestia,crisis,single-key}/注入载荷不得命中
TestLookupRejectsInjectionInKeyValues/{hestia,crisis,single-key}/含单引号的合法取值仍能被正确命中
```

⇒ **独立 reviewer 的判断实测成立**：其余全部 criteria（命中/未命中、多版本、乱序、
键不匹配、零值 Spec、表名包装、`*sql.Tx`）**无一转红**。没有这条测试，注入改写会完全无声通过。
DoD `error_handling[3]` 的明文变异判据**满足**。

补充：该测试的两个子测试**双向**都红——「注入载荷不得命中」抓拼接放行，
「含单引号的合法取值仍能被正确命中」（`O'Brien`）抓拼接导致的语法错误。
这是「不是一律查不到，而是真的走参数化」的反向证明，比 DoD 要求的更强。

### 2.2 【F1，重要】MI-2 存活 —— 我实证它**不是等价变异，是能实际得手的注入**

我另造了一个变异：取值改用 **`fmt.Sprintf("%s = %q", c, key[c])`** 拼进 SQL（同样去掉占位符）。

**结果：存活。**三条自证齐备（`diff=11 行` / `go vet exit=0` / `PASS=120 == 基线 120`）——
即**全套 120 条测试无一转红**。

「存活」有两种可能：等价变异（无害），或缺口（有害但没人看见）。我没有停在这里，写了探针直接观察：

| 载荷 | MI-2（`%q` 拼接）下 | 基线（占位符）下 |
|---|---|---|
| `x' OR 1=1 --`（现有测试用） | 被挡住（未命中） | 被挡住 |
| `' OR '1'='1`（现有测试用） | 被挡住（未命中） | 被挡住 |
| **`x" OR 1=1 --`（双引号）** | **命中！`LatestRevision=2026-12-31`** | 被挡住 |
| `x" OR "1"="1` | 返回 SQL 语法 error | 被挡住 |
| `x" OR published_at>"0" --` | 返回 SQL 语法 error | 被挡住 |

**`2026-12-31` = `revLatest`，是全表的最大 revision，不属于被查的那个业务键** —— 这是一次
成功的布尔注入，不是「碰巧查到了」。【实测】

**机理**：Go 的 `%q` 用**反斜杠**转义双引号（`"` → `\"`），而 **SQLite 不认反斜杠转义**——
它在 `\` 后的 `"` 处就结束了双引号 token，剩下的 ` OR 1=1 --` 被当作 SQL 解析，
`--` 再把尾部的 ` AND period_type = "h1"` 注释掉，WHERE 恒真。
（探针文件用后即删，未进入任何提交；`lookup.go` 还原后 md5 与基线一致。）

### 2.3 为什么这**不构成本任务返工**

1. **DoD 的明文变异判据是「把取值拼进 SQL（去掉占位符）后本条须转红」——MI-1 满足了。**
2. **缺口的根源在 DoD 的示例载荷本身**：DoD 给的例子是
   `Key{"period": "2026-06' OR '1'='1", ...}` —— **单引号系**。
   现有三条载荷（`x' OR 1=1 --`、`' OR '1'='1`、`2026-06'; DROP TABLE ...; --`）全部覆盖了
   DoD 点名的形态，dev 还额外做了 `allShapes` 与反向证明。**照 DoD 执行不会产生双引号载荷。**
3. **交付的实现本身没有漏洞**——它走的是真占位符，MI-2 描述的是一个假想的未来回归。

⇒ 判定为**非阻断发现**，但**建议值高**：`%q` 恰恰是「被告知不要裸拼值」的人最可能选的写法
（它看起来像在转义），而它对 SQLite 静默无效。

**建议（1 行成本）**：在 `payloads` 里加一条双引号载荷，例如
`` `x" OR 1=1 --` ``。我实测该载荷在基线下被正确挡住（返回未命中），
所以**加进去当前即绿**，同时能杀掉 MI-2。
**提请 Leader**：把「注入载荷须同时覆盖单引号与双引号系」补进 TASK-006 的终验清单
（或作为 DoD 示例的修订），因为这一条的根源在 DoD 示例，会同样影响后续任何抄它的任务。

---

## 三、三条「TASK-001 的前提在包写完后还成不成立」

### 3.1 （a）标识符只能来自 `Spec` —— 成立【实测】

`lookup.go` 中进入 SQL 字符串的全部内容：

- 格式串常量 `"SELECT MAX(%s) FROM %s WHERE %s"`、`" = ?"`、`" AND "`
- `spec.revisionCol`、`spec.table`、以及循环变量 `c`（来自 `spec.businessKey`）

**三个占位全部由 `Spec` 的未导出字段填充，无任何自造标识符、无自造 alias**（对比 `query.go`
有 `queryAlias`，`lookup.go` 连 alias 都不需要）。业务键取值一律经 `args` 走 `?`。
⇒ 「注入面为零」的第三个入口（001 守进 Spec、004 守 query.go、005 守 lookup.go）在本侧成立。

### 3.2 error_handling[1] 的一处独立复现（值得单独记）【实测】

dev 报告它的 MT4 首轮**存活**：把包装里的 `spec.table` 删掉后全套无一转红——因为
需求文档样例只测「表不存在」，而 sqlite 的原始错误 `no such table: no_such_table` **本身就带表名**，
表名是驱动给的，包装删不删都绿。它随后补了「列不存在」子测试隔离包装的贡献。

**我独立复现确认**：E1-a（删表名）红名单为
`TestLookupWrapsSQLError` + `TestLookupWrapsSQLError/列不存在——表名只可能来自包装`，
**原「表不存在」子测试仍然绿**。⇒ dev 的诊断准确，补的那条子测试是这条 criteria 的唯一守护。
我另加 E1-b（改用 `%v` 断掉 `%w` 链）同样 KILLED，说明 `errors.Unwrap` 那条前提断言也有守护。

### 3.3 （b）包级封装重查 + （③）没加 getter —— 均成立【实测】

**getter（error_handling[5]）**：`spec.go` 的 **git blob hash 在五个 commit 中完全相同**：

```
224c960 / 96641ec / d928dd8 / 89bc09c / f2205ac  →  7aee7f42be28649c3fd9fa7a971f070d65240fd7
$ git log --oneline 224c960..f2205ac -- internal/macro/bitemporal/spec.go   → 无输出
$ 工作区 md5 = 59934cd2238daeeedb3ab9c8494cc437（与 TASK-001 验收记录一致）
$ grep -nE '^func \(s Spec\)' *.go → 仍只有 zero() / checkKey() / correlate() 三个
```

**`spec.go` 自 TASK-001 交付后从未被任何 commit 触碰。承诺守住了。**

**封装重查（non_functional[1]）**：我独立 grep 复现 dev 的结论。全包 `Spec{` 共 11 处：

- `spec.go` 7 处：6 处是 `NewSpec` 的错误返回 `Spec{}`（零值）+ 1 处 `spec.go:64` 是 `NewSpec` 自身的带字段构造
- `lookup_test.go:175`、`query_test.go:127`、`query_test.go:132`、`spec_test.go:38` 共 4 处：**全为零值 `Spec{}`**，且正是用来测 T5 / 零值守卫本身

⇒ **包内没有第二处带字段的 `Spec` 构造点**。TASK-001 建立的「未校验的 Spec 构造不出来」
在包写完之后**仍然成立**。未导出字段的包内访问全部是**读**（拼 SQL / 建测试 SQL），
不违反该守卫——守卫防的是「构造出未校验的 Spec」，不是「读已校验 Spec 的字段」。

**这正是我在 TASK-001 报告第六节留下、又在 TASK-003 复查过一轮的那条。现在 001-005 全部落地，
这是最终确认。**

### 3.4 （c）ISO revision 形态 —— 三选一已选，且不止「写下来」【实测】

判定标准是「三选一并写下来；仍只是注释 = 没选 = FAIL；选『本包不管』但写下来了 = PASS」。

dev 选的是**第三项**（本包不管，由建表方与写入方保证），并做了三层落实：

1. **写在 `lookup.go:29-39` 的文档注释**，含三条不校验的理由（薄机制层不认识列语义 /
   成本落在正常路径 / 把数据质量问题转成读路径失败）；
2. **写进 discovery 的 `decisions`**；
3. **用可执行测试钉住**：`TestLookupRevisionFormIsCallerGuaranteed` 插入 revision `"9"` 与 `"10"`，
   断言返回 `"9"` —— 即**字典序的结果，明确标注「这不是语义上最新的版本」**。

第 3 层超出 DoD 要求。它的作用是：将来谁想给 Lookup 加形态校验，这条会转红，
**迫使他显式推翻这个判定，而不是悄悄改掉**。我用 E5-a（给 Lookup 加 ISO 长度校验）验证——
**KILLED**，确认这条契约真的钉住了。

⇒ **PASS**，且是三个选项里最强的落实方式。

---

## 四、Leader 关注点 ④ —— 「存活」三条自证

我的 harness 把三条**内建**为：凡判「存活」自动附
① `difflib` 算出的变异 diff 行数 ② `go vet` exit code ③ `--- PASS` 计数与基线 120 的比对。

本轮唯一存活的 MI-2 输出：`diff=11 行(非空) | vet exit=0(干净) | PASS=120 vs 基线 120(一致)`。
**三条齐备，「存活」是真的存活，不是编译失败伪造的。**

**旁证（dev 侧的真实事件）**：discovery 记载它的 MT4/MT5 首轮出现 `vet_exit=1`、`PASS=0`，
根因不是变异，而是 dev-agent-39 当时正在同包写 `query_test.go`，包内瞬时不可编译。
harness 的第 ③ 条自证发现 `PASS+FAIL==0`，直接以 exit 4 中止而非报「0 红」。
它随后改用文件级隔离（排除 `query*.go`，基线 29）完成余下变异。
**这条纪律在真实事件上生效了一次，不只是理论推演。**
我这一侧因 `query.go` 已稳定落地，直接用**包级**基线 120 测量，比它的隔离口径更强。

---

## 五、非阻断观察

**F1（重要，已实证，见第二节）** —— `%q` 形态的注入拼接可存活，且能实际得手。
建议加一条双引号载荷（1 行，加后当前即绿）；**根源在 DoD 示例载荷，提请 Leader 补进 TASK-006 终验**。

**F2（轻微）** —— DoD 条数：任务文件按维度点数为 **18 条**，派验消息写 17。不影响判定，仅记录口径差异。

**F3（记录）** —— discovery 的 `encapsulation_recheck` 引用了 `lookup_test.go:171`，实际零值
`Spec{}` 在 **175 行**（应为 code-simplifier 改动前后的行号漂移）。结论不受影响；
这也再次说明**行号引用不如函数名/符号名稳定**。

---

## 六、判定

**PASS（verified）。**

依据（全部为实际运行命令的输出，非推测）：
1. 18 条 done_criteria **逐条**有对应测试且**逐条**有变异实证；
2. 包级 120 PASS / 0 SKIP / 0 FAIL，覆盖 100%，vet exit 0，gofmt 干净，未触碰 go.mod/go.sum；
3. 15 个变异 14 KILLED / 1 存活，存活那条经三条自证确认为真存活，并被我进一步**实证为真缺口**（F1）；
4. **头号目标**：DoD 原文注入判据 MI-1 KILLED，且红名单**排他性**只有注入测试——
   reviewer「其余全部 criteria 仍然全绿」的前提实测成立；
5. **三条前提全部复核成立**：标识符只来自 `Spec`；包内无第二处带字段 `Spec` 构造点；
   ISO 形态三选一已选且用可执行契约钉住；
6. **`spec.go` 五个 commit blob hash 全同、方法集未变** —— 「只加一个 getter」这条自设约束守住了。

F1 不构成本任务返工（DoD 明文判据已满足、根源在 DoD 示例、交付实现本身无漏洞），
但**需要 Leader 在 TASK-006 终验前处置**。

## 收尾
验证 worktree `../wt-verify-TASK-005` 已从主仓库以绝对路径执行 `git worktree remove` 拆除；
`lookup.go` md5 `4000dbb25a03441e5da331c31250b68d`、`lookup_test.go` md5 `60f6d3b613302444d8dfed6f5666dd74`
还原后与基线一致；临时探针文件已删，主工作区无污染。
