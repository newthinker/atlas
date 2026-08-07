# TASK-001 验证报告 — Spec 与 NewSpec（标识符校验 + 键形状校验）

- **验证者**：test-agent-18（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：commit `224c960`（`internal/macro/bitemporal/spec.go` 107 行 + `spec_test.go` 149 行）
- **验证基线**：`feat/macro-bitemporal` @ `96641ec9d246e01438fff29d905cf9f9ba738f5a`
  （含 TASK-002 的 `classify.go`/`classify_test.go`，用于满足包级编译）
- **验证环境**：隔离 worktree `../wt-verify-TASK-001`，主工作区零污染；go1.24.4 darwin/arm64
- **判定：PASS（verified）**

> 标注约定：**【实测】**= 本报告作者在隔离 worktree 中亲自运行命令并观察到输出；
> **【推断】**= 由代码阅读或已实测事实推出、未单独跑命令验证。

---

## 一、完成标准覆盖矩阵（9 条 done_criteria 逐条）

| # | 完成标准 | 对应测试 | 守护证据（变异） | 判定 |
|---|---|---|---|---|
| functional[0] | NewSpec 接受合法形状（多列/单列键）；零值 Spec 被 zero() 识别 | `TestNewSpecAcceptsValidShapes` | MF0-a（zero 恒 false）、MF0-b（zero 恒 true）双向均转红 | **PASS**【实测】 |
| functional[1] | NewSpec 必须复制 businessKey；**DoD 原文变异判据：去掉复制须转红** | `TestNewSpecCopiesBusinessKey` | MF1（`append(...)` → `businessKey: businessKey`）转红 | **PASS**【实测】 |
| boundary[0] | correlate 只产 `col = alias.col`、无字面值（T1：本任务只保纯函数） | `TestCorrelate` | MB0-a（方向颠倒）、MB0-b（掺 `AND 1=1`）均转红 | **PASS**【实测】 |
| error_handling[0] | 非法标识符被拒：注入/空串/数字开头/含空格/含引号/含反引号 × table/key/revision 三入口 | `TestNewSpecRejectsInvalidIdentifiers`（18 子用例 + 尾列非法） | ME0-a/b/c（分别删三处防线）+ MX1（去尾锚 `$`）+ MX2（去头锚 `^`）+ MX5（出错返回半构造 Spec）全转红 | **PASS**【实测】 |
| error_handling[1] | **契约 T2**：注入用例不能只断 `require.Error`，至少一条断言 error 含被拒标识符 | 同上（`require.ErrorContains(err, fmt.Sprintf("%s %q", wantLabel, bad))`） | **M5b 对照实验独立复现**（见第三节） | **PASS**【实测】 |
| error_handling[2] | 键形状被拒：业务键为空 / 有重复 / 与 revision 列重名 | `TestNewSpecRejectsBadKeyShape` | ME2-a/b/c（分别删三条检查）各自转红 | **PASS**【实测】，附非阻断观察 F1 |
| error_handling[3] | checkKey 拒键集不匹配：缺键 / 多键 / 键名不同 | `TestCheckKey` | ME3-a（删长度检查）、ME3-b（删逐列检查）、MX4（`!=`→`<`）全转红 | **PASS**【实测】 |
| non_functional[0] | 包级绿、0 SKIP；头部写 `Context Checkpoint: done_criteria → test mapping`；中文注释解释「为什么」 | 全包 | — | **PASS**【实测】 |
| non_functional[1] | 不新增依赖（testify 已在 go.mod） | — | — | **PASS**【实测】 |

`verify_by` 说明：本任务 9 条全部为字符串形态（视同 `verify_by: test`），无 `review`/`manual`/`benchmark` 条目，故矩阵中无 N/A。

---

## 二、基础事实（全部实测）

### 包级复验（Leader 关注点 ③）
Dev 在 RED 阶段用 `go test spec.go spec_test.go` 做文件级隔离（因当时 `classify.go` 未落地），
并声称交付前已用包级命令复验。**该声称属实**：

```
$ GOTOOLCHAIN=local go test -count=1 -v ./internal/macro/bitemporal/
PASS  ok  github.com/newthinker/atlas/internal/macro/bitemporal  0.494s
--- PASS 计数: 46 | --- SKIP 计数: 0 | --- FAIL 计数: 0
```

### 覆盖率
```
$ go tool cover -func=cov.out
spec.go:40:  NewSpec    100.0%
spec.go:73:  zero       100.0%
spec.go:84:  checkKey   100.0%
spec.go:101: correlate  100.0%
total: 100.0% of statements
```
（`classify.go` 的 String/Classify 亦 100%，属 TASK-002 范围。）

### 静态检查与依赖
- `gofmt -l spec.go spec_test.go` → 无输出
- `go vet ./internal/macro/...` → 无输出
- `git show --stat 224c960 -- go.mod go.sum` → 无输出（**未触碰依赖清单**）；`testify v1.11.1` 早已在 go.mod

### 声明范围 vs 实际改动（越界申报检查）
任务声明 `writes = [spec.go, spec_test.go]`；`git show --stat 224c960` 实际改动**恰为这两个文件**。
**无越界申报。**

### 注释惯例
`spec_test.go` 头部 3–13 行的 `Context Checkpoint: done_criteria → test mapping` 格式与仓库既有惯例
（如 `cmd/atlas/policy_test.go:13`）一致；实现与测试注释均为中文且解释「为什么」而非「是什么」。

---

## 三、契约 T2 对照实验（Leader 关注点 ①，独立复现 + 加强）

我未采信 dev 的描述，独立构造了同型变异体并跑了三个分支。

**M5b 构造**（经 `assert count(old)==1` 校验靶点后替换，非静默 `sed`）：
删掉 table 的 `identRE` 防线，同时在原位插入一条**无关的先行错误**
`if len(table) > 8 { return Spec{}, fmt.Errorf("bitemporal: table name too long") }`。

### 分支 0 —— 探针：先证明防线**真的失守**（此步 dev 未做，是我对其证据的加强）
只证明「错误消息变了」是不够的——必须证明防线本身没了。用一个**短于 8 字符**的非法 table
（`a'b`，长度 3，不触发 `len>8`）探测：

```
PROBE NewSpec("a'b", ...) => err=<nil> / zero=false
>>> 防线已失守：含引号的非法 table 被接受
```
【实测】非法标识符在 M5b 下**确实被放行**，M5b 是真正的「防线失守」而非「消息改写」。

### 分支 A —— 弱化版测试（把强断言退化为只有 `require.Error`）
```
--- PASS: TestNewSpecRejectsInvalidIdentifiers/table_注入 (0.00s)  → rc=0
```
**假绿**：防线已失守，仅 `require.Error` 的测试照样通过（因为 `too long` 也是个 error）。

### 分支 B —— 交付版测试（`ErrorContains` 指名标识符）
```
Error: Error "bitemporal: table name too long" does not contain "invalid table name \"x\\\"; DROP TABLE y; --\""
--- FAIL: TestNewSpecRejectsInvalidIdentifiers/table_注入  → rc=1
```
**抓住了。**

**结论**：dev 关于契约 T2 的对照实验**独立复现成立**，且其结论比它自己声称的更强——
我补的探针排除了「只是消息变了」这一替代解释。`error_handling[1]` 有实证守护，非纸面声明。

（实验后 `spec.go` / `spec_test.go` 均以 md5 核实还原至基线 `59934cd2238daeeedb3ab9c8494cc437`，
临时探针文件已删除，未进入任何提交。）

---

## 四、变异测试（18 个，含 5 个 dev 未做的补充）

工具纪律：靶点先做 `count(old) == 1` 断言，不匹配即 abort（**禁止静默 sed**）；每个变异跑完立即
还原并以 md5 核实；对红结果区分「编译失败的红」与「断言失败的红」。

**结果：18/18 全部转红，且全部为 `RED-断言失败`，无一例是编译失败的假红。**【实测】

| ID | 变异 | 红掉的测试 |
|---|---|---|
| MF0-a | `zero()` 恒 false | AcceptsValidShapes + RejectsInvalidIdentifiers 全部子用例 |
| MF0-b | `zero()` 恒 true | AcceptsValidShapes |
| **MF1** | **[DoD 原文判据] 去掉切片复制** | **CopiesBusinessKey** |
| MB0-a | correlate 方向颠倒 `alias.col = col` | Correlate, CopiesBusinessKey |
| MB0-b | correlate 掺 `AND 1=1` | Correlate, CopiesBusinessKey |
| ME0-a | 删 table 的 identRE 防线 | RejectsInvalidIdentifiers（table 全 6 子用例） |
| ME0-b | 删 businessKey 的 identRE 防线 | RejectsInvalidIdentifiers（key 全 7 子用例） |
| ME0-c | 删 revisionCol 的 identRE 防线 | RejectsInvalidIdentifiers（revision 全 6 子用例） |
| **MX1** | **[补充] identRE 去掉尾锚 `$`** | 注入/含空格/含引号/含反引号 × 三入口 |
| **MX2** | **[补充] identRE 去掉头锚 `^`** | 上述 + 数字开头 × 三入口 |
| **MX3** | **[补充] 只复制业务键首列（多列键静默截断）** | CheckKey, Correlate, CopiesBusinessKey |
| **MX5** | **[补充] 出错返回 `Spec{table:table}` 而非零值** | RejectsInvalidIdentifiers（table 6 子用例） |
| ME2-a | 删「业务键为空」检查 | RejectsBadKeyShape |
| ME2-b | 删「重复列」检查 | RejectsBadKeyShape |
| ME2-c | 删「revision 重名」检查 | RejectsBadKeyShape |
| ME3-a | 删 checkKey 长度检查 | CheckKey |
| ME3-b | 删 checkKey 逐列检查 | CheckKey |
| **MX4** | **[补充] checkKey `!=` → `<`（多键漏网）** | CheckKey |

MX1/MX2 的意义：`identRE` 是本包注入面的唯一闸门，**正则锚点**是它最脆弱的一环（去掉任一锚都会
让常见注入串前缀/后缀合法化）。dev 的 13 个变异只覆盖了「防线在不在」，未覆盖「防线的边界对不对」。
两者都被现有断言抓住。

---

## 五、注入面边界探针（DoD 之外的独立取证）

`identRE` 用 Go 的 `regexp`，其 `$`（无 `(?m)` 时）语义为 `\z`（文本绝对结尾），
**不同于 PCRE/Python 的 `$`**（后者允许末尾一个换行，是经典的正则校验逃逸口）。
本包若照搬 Python 直觉即会开一个洞。实测确认 Go 语义符合预期：

| 探测值（table） | 结果 |
|---|---|
| `"obs\n; DROP TABLE x; --"` | 被拒 |
| `"obs\n"` / `"obs\r"` / `"obs\t"` | 被拒 |
| `"obs\x00; DROP TABLE x"` | 被拒 |
| `"obsервations"`（unicode 同形字） | 被拒 |
| `"obs；DROP"`（全角分号） | 被拒 |
| `"obs--x"`（SQL 注释符） | 被拒 |
| `"db.obs"`（点号跨库） | 被拒 |
| 200 字符纯字母 | 被接受 |

对 businessKey 与 revisionCol 同样探了换行逃逸，均被拒。
**200 字符纯字母被接受不是缺陷**——它是完全合法的标识符，无注入语义，DoD 也未要求长度限制。

---

## 六、对 dev 两条「论证过但未验证」边界的判定（Leader 关注点 ②）

### 主张 1：「Spec 字段未导出 ⇒ 未校验的 Spec 构造不出来，唯一例外是零值」
**边界划得对，但 dev 自己加的限定语（「这只对此刻的代码成立」）是准确且必要的。**

- 【实测】`grep -rn "Spec{" internal/macro/bitemporal/` 全部命中为：6 处错误返回的零值 `Spec{}`、
  1 处 `NewSpec` 内经校验的构造（spec.go:64）、1 处测试里的 `Spec{}.zero()`。**无第二处非零复合字面量。**
- 【实测】`grep -rn 'bitemporal\.' --include='*.go'`（排除本包）无输出——**包外尚无使用者**。
- 【推断】包**外**这是**机制**保证（字段未导出，Go 编译器强制）；包**内**只是**约定**——
  同包新文件写 `Spec{table: "x"}` 就能造出未校验的非零 Spec，且 `zero()` 认不出来。

**判定：这一条不该由 TASK-001 承担**——Go 没有语言机制能封住同包构造，TASK-001 也无从为尚不存在的
文件作证。但它是一条需要**随包演进复查**的约束。→ 建议登记给 TASK-003/004：向本包新增文件时，
Spec 一律经 `NewSpec` 构造，不得用复合字面量。（非阻断，非本任务返工项。）

### 主张 2：「identRE 封死了本包的注入面」——只证了 NewSpec 拒收，没证「所有拼进 SQL 的标识符都取自 Spec」
**边界划得对，且这正是 DoD 白纸黑字的分工。**

- DoD `boundary[0]` 原文即写明：「按契约 T1 它**不能单独构成守护**——Task 4 须有行为侧证明 Query 真的
  用了它。**本任务只需保证纯函数本身正确**」。dev 的划界与 DoD 逐字一致，未越界也未缩水。
- 【实测】本包（TASK-001 范围）内**根本没有 SQL 构造**：`grep SELECT/FROM/WHERE` 在 `spec.go` 零命中，
  唯一的 `Sprintf` 就是 `correlate` 那行。「所有拼进 SQL 的标识符都取自 Spec」在当前代码里**无对象可证**——
  `CurrentQuery`/`AsOfQuery` 属 TASK-004。
- Leader 已把这点加进 TASK-004 的 DoD，**移交路径正确**。

**结论：两条边界我都认可，无一条应由本任务承担。**

---

## 七、非阻断观察（不构成返工，供 QA 与后续任务参考）

**F1（latent，测试设计）**：`TestNewSpecRejectsBadKeyShape` 是 `error_handling[2]` 的**唯一**守护，
其四条断言全部是裸 `require.Error(t, err, "...")`（第二参数是 testify 的失败提示文案，**不是**对错误内容的断言），
且该测试函数内**没有正路径对照**。这正是 DoD `error_handling[1]` 警告的 T2 形态。
【实测】M8（删掉三条键形状防线 + 插入 `len(table) < 2` 的无关先行错误）下，
`TestNewSpecRejectsBadKeyShape` **仍然 PASS**。

**但我必须如实说明这条证据的局限**：同一 M8 变异下整包是红的——红的来源是我注入的
`len(table) < 2` 人为副作用打断了其它用 `table = "t"` 的测试，**而不是**键形状防线的缺失被抓住。
因此我**没有**证明存在包级逃逸；这是一个**潜在**弱点，不是已实证的漏洞。
且 DoD `error_handling[1]` 只要求「至少对一个**注入用例**」做强断言，该要求已由
`TestNewSpecRejectsInvalidIdentifiers` 满足。**故 F1 不构成 DoD 违反，不作为返工项。**
建议（可选）：给该测试的三条键形状用例各加一条 `ErrorContains`（`must not be empty` /
`duplicate business key column` / `also appears in business key`），成本约 3 行。

**F2（轻微，测试结构）**：`TestNewSpecRejectsBadKeyShape` 用串行 `require` 且未拆 `t.Run` 子用例——
第一条失败即中止，后三条不再执行。ME2-b / ME2-c 已各自单独证明另两条防线确有守护，故不是缺陷，
但与同文件 `TestNewSpecRejectsInvalidIdentifiers` 的表驱动 + `t.Run` 风格不一致。

**F3（移交）**：见第六节主张 1 —— 「包内不得以复合字面量构造未校验 Spec」当前成立但非机制保证，
建议登记为 TASK-003/004 的复查项。

---

## 八、判定

**PASS（verified）。**

依据（全部为实际运行命令的输出，非推测）：
1. 9 条 done_criteria **逐条**有对应测试，且**逐条**有变异实证其断言非空洞；
2. 包级 46 PASS / 0 SKIP / 0 FAIL，语句覆盖 100%；
3. 18/18 变异被杀，全部为断言失败而非编译失败，其中 5 个是 dev 未做的补充变异（正则锚点、
   键截断、半构造 Spec、长度比较符）——**没有找到现有测试打不到的地方**；
4. 契约 T2 的对照实验独立复现成立，并经探针加强（排除「仅消息变化」的替代解释）；
5. 注入面边界探针（换行/回车/NUL/制表符/unicode/注释符/点号）全部被拒，Go `$` 语义无 PCRE 式逃逸口；
6. 声明范围与实际改动完全一致，无越界申报；未新增依赖；gofmt / vet 干净；注释格式对齐仓库惯例。

三条非阻断观察（F1/F2/F3）已记录，均不构成 DoD 违反。

## 收尾
验证 worktree `../wt-verify-TASK-001` 已由本验证者从主仓库执行 `git worktree remove` 拆除；主工作区无污染。
