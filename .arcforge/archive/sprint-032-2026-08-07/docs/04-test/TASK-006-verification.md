# TASK-006 验证报告 —— 包级收尾与四处缺口修复

- **验证者**：test-agent-19（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：commits `8c7c400` + `456c85a` @ `feat/macro-bitemporal`（master..HEAD 共 **7** 个 commit）
- **承接时 assignment_epoch**：**2**
- **隔离**：`git worktree add --detach ../wt-verify-TASK-006 456c85a`；全部变异在 worktree 内注入并逐条还原，收尾 7 个文件 md5 全部回到基线、`git status`/`git diff` 均空，worktree 已从主仓库拆除
- **基线**：**PASS=125 / FAIL=0 / SKIP=0 / vet exit 0**（与派验单一致）
- **判定**：**VERIFIED** —— 11 条 done_criteria 全部通过

> **DoD 订正已采纳**：`non_functional[0]`「应有 5 个 commit、本任务自身无代码提交」**已作废**（Leader 经 inbox 订正，因转 `verifying` 后无人可写该字段）。实际 7 个 commit（5 开发 + 2 修复），dev 亦在 discovery 的 `dod_inconsistency_reported` 里主动自报了同一冲突。**本条按订正判。**

---

## 1. 四处修复的前后对照（全部由 test-agent-19 独立注入实跑）

「修复前」一律用 `git show f2205ac:<file>` 取回修复前的测试文件（F3/F4 另需移走 `identifiers_test.go`，因该文件在 `f2205ac` 时尚不存在），再注入**同一个**变异。

| ID | 变异 | **修复前** | **修复后** |
|---|---|---|---|
| **F1** | `failureContext` 去掉形状名（B0-a） | **2 次断言失败**：`crisis`、`single-key`——**`hestia` 缺席** | **3 次断言失败**：`形状 hestia` / `形状 crisis` / `形状 single-key` |
| **F2** | MI-2：`fmt.Sprintf("%s = %q", c, key[c])` 拼值 + 去占位符 | **PASS=120 FAIL=0 vet=0**（自证三条齐备） | 三套形状**全红**：`TestLookupRejectsInjectionInKeyValues/{hestia,crisis,single-key}/注入载荷不得命中` |
| **F3** | SQL 里混入恒真的 `AND "x" = "x"` | **PASS=120 FAIL=0 vet=0**（自证三条齐备） | `TestQueriesContainNoLiteralValues` 转红 |
| **F4** | P2：`CurrentQuery` 加 `ORDER BY rowid` | **PASS=120 FAIL=0 vet=0**（自证三条齐备） | `TestSQLFragmentsIntroduceNoBareIdentifiers/query.go` 转红 |

（三处「修复前」的 `PASS=120` 正是移走 `identifiers_test.go` 后的正确基线：125 − 5 = 120，该测试贡献 1 个顶层 + 4 个子测试。）

### F1 的差异在测试粒度上完全不可见 —— 值得单独记

`TestFailureMessagesNameTheShape` **没有用 `t.Run`**，三套形状共用一个测试函数。所以修复前后 harness 读到的都是 **`FAIL=1`**：

```
修复前:  "（表 macro_observations）建表失败" does not contain "crisis"
        "（表 single_key_obs）建表失败"   does not contain "single-key"        ← 2 次,hestia 缺席
修复后:  "（表 hestia_observations）建表失败" does not contain "形状 hestia"
        "（表 macro_observations）建表失败"  does not contain "形状 crisis"
        "（表 single_key_obs）建表失败"     does not contain "形状 single-key"  ← 3 次
```

**只看 `--- FAIL` 计数，这个修复看起来什么也没改变。** 必须下沉到断言粒度（`grep -c 'Error Trace'`）才能看见。这是本 Sprint「0 红可被伪造」之外的另一种计数失真：**红了，但红的条数不对，而条数正是判据本身**。

### F2 的红因已核实（不只看红）

```
Messages: 形状 hestia（表 hestia_observations）注入取值查询失败：取值 "x\" OR 1=1 --" 绝不能命中任何行
Error:    Should be empty, but was 2026-12-31
```

`2026-12-31` 是**全表最大 revision、不属于被查的业务键** —— 红的确实是那次布尔注入，不是别的原因蹭红。

## 2. F4 实现的两点核查（Leader 点名）

### ① 扫描范围确实是全目录，不是写死两个文件 —— ✅ 成立
基线运行实测扫到 **4 个**非测试源文件，各自一个子测试：
`classify.go` / `lookup.go` / `query.go` / `spec.go`。
实现用 `os.ReadDir(".")` 过滤 `.go` 且非 `_test.go`，**新增文件自动纳入**——dev 给的理由（「否则『守将来』会被『将来新建一个文件』绕过」）成立。
另有 `require.NotZero(t, scanned)` 兜底：**扫描目标为空时整条测试会红而非静默全绿**——这正是它要防的那类失效的自我应用。

### ② 白名单确实会在新增 SQL 语法时触发 —— ✅ 成立（且我第一次的验证是无效的，见 §5）
构造了一个**无害**改动：加 `ORDER BY %s`，其 `%s` 参数是 `spec.revisionCol`（**来自 Spec 的合法列**）。结果：

```
query.go:31 的 SQL 片段 "... ORDER BY %s" 里有裸标识符 "ORDER"。……若 "ORDER" 其实是 SQL 关键字，把它登记进 sqlKeywords
query.go:31 的 SQL 片段 "... ORDER BY %s" 里有裸标识符 "BY"。……
```

**即使新增的标识符本身完全合法，新增的 SQL 关键字仍被报出，并明确引导登记。** 强制登记的设计意图成立。
代价是合法的 `ORDER BY` 也会先报红一次——但这是**刻意的摩擦**，dev 写明了理由（「白名单越短，这道摩擦越有效」），属设计取舍而非缺陷。

## 3. 限度声明：**诚实、准确，但落点不对**

dev 声明的限度是「只检查字符串**字面量**，`c + " = " + someVar` 这类变量拼接抓不到」。

**我实证了，且区分出它比 dev 自己举证的更精确的两层**：

| 引入方式 | 实测 | 说明 |
|---|---|---|
| **字面量**拼接 `c + " = ? AND rowid > 0"` | **抓得到**（dev 已验，我认可） | 该字面量含 `AND` 关键字 ⇒ 被识别为 SQL 片段 ⇒ `rowid` 被报出 |
| **变量**拼接 `"... " + " ORDER " + "BY " + evilCol` | **抓不到**：**PASS=125 FAIL=0 vet=0**（自证三条齐备） | `evilCol` 的值不在任何字面量里，AST 扫不到 |

⇒ **限度声明属实，无夸大。** dev 的表述「它堵的是『顺手加一段 SQL』这条路，不是完备的污点分析」是准确的。

### ⚠ 但这条限度**只写在 discovery 里，没有写进 `identifiers_test.go` 的注释**

我 grep 了整个 `identifiers_test.go`：**没有任何一处说明变量拼接是旁路。**

**为什么这要紧**：这条测试的**全部目的是「守将来」**，而将来读它的人只会看到测试文件——**不会去读 `.arcforge/discoveries/TASK-006.json`**（那是 Sprint 归档产物，随 Sprint 关闭而离场）。一个读到 `TestSQLFragmentsIntroduceNoBareIdentifiers` 的人会合理地推断「标识符来源已被机制守住」，而不知道边界在哪。

**这正是本任务 `error_handling[0]` 那条契约——「沉默的排除等于没有排除」——在这条测试自己身上的应用。** 它把三件「本包刻意不做的事」郑重写进了 discovery，却把「这条守护刻意不覆盖的事」留在了代码之外。

**判定**：**不构成 criteria 失败**——`error_handling[1]` 的判据是「加一条静态检查测试，复现 P2 须转红」，做到了，且我验证了。
**建议（成本一段注释）**：把限度写进 `identifiers_test.go` 的文件头注释。

## 4. 回归抽验：TASK-003 / TASK-005 的守护**未被削弱**

本任务改了两个已 `verified` 任务的文件（`fixture_test.go`、`lookup_test.go`、`query_test.go`），故须抽验其原有守护。

| ID | 变异 | 结果 |
|---|---|---|
| **R3-a** | `correlate` 的 `AND`→`OR` | **KILLED** 108/125，`TestProbeDistinguishesAndFromOr` 三处红 |
| **R3-b** | 探针改成不共享列（破坏 T1 可观测性前提） | **KILLED** 115/125，`TestProbeCurrentMatchesInsertedData` + `TestProbeDistinguishesAndFromOr` 红 |
| **R3-c** | `seedProbe` 改成**顺序**插入 | **存活** 125/125（见下） |
| **R5-a** | 去掉零值 Spec 检查 | **KILLED**，唯一红 `TestLookupRejectsZeroSpec` |
| **R5-b** | 去掉 `checkKey` | **KILLED**，`TestLookupRejectsMismatchedKey/{缺键,多键}` |
| **R5-c** | 去掉 `NullString.Valid` 分支 | **KILLED** 115/125，`TestLookupHitAndMiss` 三形状红 |
| **R5-d** | 错误包装去掉表名 | **KILLED**，红的是 `TestLookupWrapsSQLError/列不存在——表名只可能来自包装`——**正是 TASK-005 为隔离「驱动噪声」补的那条子测试，它确实在守** |

**⇒ 零削弱成立。**

### R3-c 存活的处置（**先于本次改动存在，非本任务引入**）
`R3-c` 是我自造的、不在 dev 的 12 条矩阵内。它在 **`f2205ac`（修复前）同样是 0 红**（PASS=120，自证三条齐备）⇒ **不是本次改动造成的削弱**。

成因：`TestInsertSupportsOutOfOrderRevisions` 断言的是 **`f.probe` 数据本身的排列是乱序的**，而非 `seedProbe` 真的按该顺序插入。所以「seedProbe 顺带覆盖乱序写入」这个承诺**只由数据的静态排列守护，不由插入行为守护**。
**影响很小**：乱序本身另有 TASK-004 的 `TestCurrentQueryIgnoresInsertOrder`（显式乱序插入）独立守护，且插入顺序对 `MAX(revision)` 语义无影响。**列为观察项，不建议本 Sprint 处理。**

## 5. 我自己两次拿到无效结果，被自证三条拦下 —— 如实记录

| 次 | 我的变异 | 症状 | 后果 |
|---|---|---|---|
| 1 | MI-2 首版：把 `args[i] = key[c]` 换成 `_ = i` | `declared and not used: args` ⇒ **PASS=0 vet=1** | 若只看 `--- FAIL` 会报「MI-2 修复后仍 0 红」——**一个指向「修复无效」的假结论** |
| 2 | 核②首版：加 `ORDER BY %s` 时同时改到了 `AsOfQuery` 的参数行 | `fmt.Sprintf call needs 8 args but has 9 args` ⇒ **PASS=0 vet=1** | 我**已经据此写下过一句「⇒ 也被报出」的结论，随即作废重做** |

第 2 次尤其值得记：**我是在编译失败的输出上下的结论**，而那正是 `non_functional[2]` 这条纪律要防的事——它这次防的是**验证者**，不是被验者。两次都由「PASS 计数 + vet 退出码」当场拦下，重做后结论才成立（§1 F2、§2②）。

另有一次同源：核对「无既有包 import 本包」时，我写的 `grep --include=*.go`（**未加引号**）被 zsh 当 glob 展开失败，命令报错、计数得 0。**那个 0 是命令失败造成的，不是真的没有 import。** 加引号重做并用 `go list` 反查依赖者交叉验证，才确认确为 0——**与 dev 踩的是同一个 zsh 陷阱**。

## 6. `detect_changes` 降级：**接受**

dev 改用等效 git 验证，我**独立复跑了全部替代证据**，不是采信：

| 问题 | 我的实测 |
|---|---|
| 改动是否仅在本包 | `git diff master 456c85a --name-only` = **10 个文件全在 `internal/macro/bitemporal/`** |
| 本包外是否有 `.go` 改动 | **0** |
| crisis / prism / collector 是否触及 | **0** |
| 是否有既有包 import 本包 | **0**（`grep` 与 `go list` 反查依赖者**双路**验证） |
| 全仓构建与测试 | `go build ./...` exit 0；`go test ./...` **FAIL 行数 0** |

**索引 stale 已量化确认**：`.gitnexus/{gitnexus,meta}.json` 的 `lastCommit` = `18339d6ffbe380074cd4ce109cb776e32e34382d`，**落后被验 commit 8 个提交**——涵盖本任务全部 7 个 commit。

⇒ **接受降级，且这不只是「退而求其次」**：索引落后 8 个提交意味着 `detect_changes` **必然看不到本任务的任何改动**，会返回「无变化」——那是个**假绿**，比 git 验证**弱**得多。用一个必然失真的工具去核对影响范围，比不用更危险。dev 的替代方案在这个语境下是**更强**的选择，不是降级的妥协。

## 7. 覆盖矩阵（11 条）

| # | 标准 | verify_by | 证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 导出面恰好为约定集合，无写操作/建表/Validate | manual | AST 精确列举 **15 个**导出标识符（见 §8） | **PASS**（清单缺陷见 §8） |
| functional[1] | `go.mod`/`go.sum` 无变化 | manual | `git diff master 456c85a -- go.mod go.sum` 无输出 | PASS |
| functional[2] | 全仓 build + test 无 FAIL | manual | build exit 0；`go test ./...` FAIL 行数 **0** | PASS |
| functional[3] | **F1** 修复 hestia 空断言 | test | §1：2 次 → **3 次**断言失败 | PASS |
| functional[4] | **F2** 补双引号载荷 | test | §1：MI-2 由存活 → 三形状全红，红因已核实 | PASS |
| boundary[0] | `detect_changes` 核对影响范围 | manual | §6：降级接受，5 项替代证据全部独立复跑 | PASS |
| boundary[1] | T1–T5 五条加强项逐条落实 | review | dev 的 `T1_T5_audit` 详实；我抽验了 T1(R3-a)、T3(F1)、T5(R5-a) 对应变异 | PASS |
| boundary[2] | 「断言被环境噪声满足」抽查 | review | dev 按 Leader 的**逐载荷形态**法做，产出 F2/F3/F4；与我独立结论一致；R5-d 证实 TASK-005 的隔离子测试在守 | PASS |
| error_handling[0] | discovery 写明三件刻意不做的事 | review | 三件**全部命中**（建表 55 列 / `UPDATE article_id` / **ADR-0009**，第三件尤其详实） | PASS |
| error_handling[1] | **F4** 抗回归静态检查 | test | §1 P2 由存活 → 转红；§2 两点核查成立；§3 限度属实 | PASS |
| non_functional[0] | commit 计数 | manual | **原文已作废**（Leader 订正）；实际 7 个，dev 已自报冲突 | PASS（按订正） |

## 8. 发现（均非 criteria 失败）

### 8.1 `functional[0]` 的导出符号清单**漏列了 `Classify`** —— DoD 文本缺陷
DoD 列举的是：`Spec / NewSpec / Key / State / Verdict(含 New,Duplicate,Revision,OutOfOrder,String) / Querier / Lookup / CurrentQuery / AsOfQuery` = 14 个。
**AST 实测 15 个**，多出的正是 **`Classify`**。

**这不是范围溢出**：`Classify` 是 TASK-002 的核心交付物，需求文档 Task 2 明确列在 `Produces` 里；且 DoD 的禁止清单是「写操作 / 建表函数 / Validate 方法」，`Classify` 三者皆非。
⇒ **判 PASS**，清单漏列是 DoD 笔误。**与 TASK-002 的 `boundary[1]` 同类：DoD 文本缺陷，只有 Leader 能改。** 建议归档前补上。

### 8.2 F4 的限度声明未进代码注释 —— 见 §3，建议补一段注释（成本极低）

### 8.3 `R3-c` 观察项 —— 见 §4，先于本次改动存在，有独立兜底，不建议本 Sprint 处理

## 9. 判定

**VERIFIED** —— 11 条 done_criteria 全部通过。

四处修复我**逐条做了「修复前 → 修复后」的独立对照**（而非只验修复后），四条在修复前的失效状态全部复现（其中三条附自证三条）、修复后全部转红且 F2 的红因经核实。TASK-003/005 的守护抽验 **7 条中 6 条 KILLED**，唯一存活的一条已证实先于本次改动存在。基线 **125 PASS / 0 SKIP / vet 0**，全仓 build+test 绿，`go.mod` 未动，改动全在本包内。

**须 Leader 处置（均为文本/文档层面）**：
1. `functional[0]` 的导出清单补上 `Classify`（§8.1）。
2. `non_functional[0]` 的 commit 计数表述（Leader 已知，归档时订正）。
3. 建议 dev 把 F4 的限度写进 `identifiers_test.go` 注释（§3）——**这条我认为值得做**：它是本 Sprint 自己那条「沉默的排除等于没有排除」契约的直接适用。
