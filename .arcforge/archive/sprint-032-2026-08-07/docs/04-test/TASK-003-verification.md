# TASK-003 验证报告 — 测试基建：两套 Spec 形状的 fixture

- **验证者**：test-agent-18（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：commit `d928dd8`（`internal/macro/bitemporal/fixture_test.go`，566 行，单文件）
- **验证基线**：`feat/macro-bitemporal` @ `d928dd8d5d835cee4fa48b841a3febe8ad56ee1a`
- **验证环境**：隔离 worktree `../wt-verify-TASK-003`，主工作区零污染；go1.24.4 darwin/arm64
- **判定：PASS（verified）**

> 标注约定：**【实测】**= 本报告作者在隔离 worktree 中亲自运行命令并观察到输出；
> **【推断】**= 由代码阅读或已实测事实推出、未单独跑命令验证。
> 本报告的每一条判定均为【实测】，推断处逐条标出。

---

## 一、完成标准覆盖矩阵（10 条 done_criteria 逐条）

| # | 完成标准 | 对应测试 | 守护证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 三套形状；bothShapes 供并行跑 | `TestFixturesAreUsable` | F0-a（bothShapes 只返 1 套）、F0-b（形状重名）、F0-c（allShapes 漏单列键）、F0-d（crisis revision 列改成与 hestia 同名）全 KILLED | **PASS**【实测】，附 F3 |
| functional[1] | `t.TempDir()` + crisis 惯例 DSN；每用例独立库 | `TestEachCallGetsAnIndependentDB` | F1-a（固定路径共享库）、F1-b（去 journal_mode）、F1-c（去 busy_timeout）、F1-d（busy_timeout 值改错）全 KILLED | **PASS**【实测】 |
| functional[2]① | 键对共享【恰好一列】 | `TestProbeKeepsT1MutationObservable` + `TestProbeDistinguishesAndFromOr` | F2-a（改成两列都不同）KILLED | **PASS**【实测】 |
| functional[2]②**（已修正）** | 两键最大 revision 必须【不相等】，方向无关 | 同上 | F2-b（改成相等）KILLED；F2-c（改小但不等）存活且直接证据测试保持绿 | **PASS**【实测】，见第三节 |
| functional[2]③ | revision 以具名常量暴露 | `TestProbeCurrentMatchesInsertedData` | F2-e（破坏常量序）KILLED；F2-f（裸字面量）存活，但**组合变异 F2-f+F2-e KILLED** | **PASS**【实测】，见第四节 |
| boundary[0] | **契约 T3**：失败信息须指名形状 | `TestFailureMessagesNameTheShape` | B0-a（failureContext 去形状名）KILLED；**B0-b = DoD 原文判据 M14**（hestia DDL 少一列）KILLED 且消息指名 hestia | **PASS**【实测】，附 F1 |
| boundary[1] | insert 支持乱序插入 | `TestInsertSupportsOutOfOrderRevisions` | B1-a（probe 改为 revision 递增）KILLED | **PASS**【实测】 |
| error_handling[0] | fixture 失败立即中止 | `TestFixtureFailuresAbortImmediately` | E0-a（newDB 建表 require→assert）、E0-b（insert require→assert）KILLED | **PASS**【实测】 |
| non_functional[0] | 绿、0 SKIP、Context Checkpoint 头注释、中文注释 | 全包 | — | **PASS**【实测】 |
| non_functional[1] | 只产 `_test.go`，`go doc` 公开面不得增加符号 | — | — | **PASS**【实测】 |
| non_functional[2] | 文档样例缺 Context Checkpoint，须自行补上 | `fixture_test.go:3-13` | — | **PASS**【实测】 |
| non_functional[3] | 报「存活」须额外自证三条 | — | 两个存活变异经**我的 harness 独立跑出**三条 | **PASS**【实测】，见第二节 |

---

## 二、Leader 关注点 ① —— 「存活」结论的三条自证

DoD `non_functional[3]` 是本 Sprint 新加的（来源：dev-agent-40 的发现——**KILLED 天然自证，
只有「0 红」会被编译失败完美伪造**）。

### 2.1 dev 是否真的把「没红」和「跑起来了」分开陈述了？—— 是【实测代码/记录核对】

`.arcforge/discoveries/TASK-003.json` 的 `surviving_mutants_self_evidence` 字段，对
**两个存活变异各自**列出了三条，且三条是分列的：

> M7b —— 变异 diff 非空、go vet exit 0、**PASS 计数 66 与基线一致**；
> M8 —— 变异 diff 非空、go vet exit 0、**PASS 计数 66 与基线一致**

「diff 非空」= 靶命中；「vet exit 0」= 编译得过；「PASS 计数一致」= 测试真的执行了。
**三个断言指向三件不同的事，没有把「没看到红」当成「跑过了」。**

### 2.2 我不采信记录，让自己的 harness 独立跑一遍

我的变异驾驶器（`mutate3.py`）把这三条**内建**为：凡判定「存活」，自动附
① `difflib` 算出的变异 diff 行数 ② `go vet` 的 exit code ③ `--- PASS` 计数与基线 66 的比对。
两个存活变异的实测输出：

```
F2-c [M7b] 共享方 revision 改小但不等
   自证① diff 行数=5(非空=True) | 自证② go vet exit=0(干净) | 自证③ PASS=66 vs 基线 66(一致)
F2-f [M8]  探针 revision 改用裸字面量(值与常量相同)
   自证① diff 行数=5(非空=True) | 自证② go vet exit=0(干净) | 自证③ PASS=66 vs 基线 66(一致)
```

**dev 的两条「存活」结论独立复现成立，且三条自证均由我这一侧的独立测量得出。**

**方法论旁注**（我这轮亲历，与该条 DoD 同源）：我第一次核 M14 的失败消息时，用整包
`go test -v` 的输出做分桶，结果**被交错输出污染**——其它测试的 `=== RUN` 行被归到了
`TestEachCallGetsAnIndependentDB` 名下。我改用 `-run '^测试名$'` 逐个隔离重跑才拿到干净结果
（第三节的数据是重跑后的）。这正是「拿到红时先问它红的是不是我以为的那个原因」——**这次
问题不在红本身，而在我把红归因给了谁**。

---

## 三、Leader 关注点 ② —— 契约 T3 与 `TestEachCallGetsAnIndependentDB`

### 3.1 M14（DoD 原文判据）实证 —— 通过【实测】

删掉 hestia DDL 的 `m2` 列后（靶点经 `count(old)==1` 校验），逐个测试用 `-run '^名$'` 隔离重跑：

| 测试 | 红 | 有 t.Run 子测试红 | 输出含「形状 hestia」 |
|---|---|---|---|
| `TestEachCallGetsAnIndependentDB` | 是 | **否**（该测试无 t.Run） | **是** |
| `TestFixturesAreUsable` | 是 | 是 | 是 |
| `TestInsertSupportsOutOfOrderRevisions` | 是 | 是 | 是 |
| `TestProbeCurrentMatchesInsertedData` | 是 | 是 | 是 |
| `TestProbeDistinguishesAndFromOr` | 是 | 是 | 是 |

实际消息：`Messages: 形状 hestia（表 hestia_observations）插入失败`。**DoD 原文判据满足。**

### 3.2 但 dev 对这一条的描述部分不实 —— 需更正【实测】

discovery 原话：

> 非子测试形态的 `TestEachCallGetsAnIndependentDB` 也带上了形状名——**t.Run 子测试名与消息本身构成双保险**。

**「带上了形状名」成立；「双保险」不成立，两个保险各缺一半：**

1. **该测试根本没有 `t.Run`**（源码 54-74 行，无子测试）——所以「t.Run 子测试名」这一保险对它不存在。
2. **它自身断言失败时不带形状名。**【实测】我把 `assert.Equal(t, 0, n, ...)` 改成期望 `1`
   制造一次自身断言失败：
   ```
   Error:      Not equal:  expected: 1  actual: 0
   Messages:   第二个库不应看到第一个库写入的行     ← 无形状名
   ```
   带形状名的**只有 fixture helper（`newDB`/`insert`/`readAll`/`currentRowsWith`）经
   `failureContext` 抛出的失败**。M14 恰好是 fixture 侧失败，所以显示了形状名。

**是否违反 DoD？否。** `boundary[0]` 的原文限定是「**bothShapes 并行跑时**」；
`TestEachCallGetsAnIndependentDB` 只用 `hestiaShape` 单一形状，不存在「不知道是哪套」的歧义。
**结论：DoD 不违反，但 discovery 的这句表述会误导 TASK-004**——若下游据此以为「非 t.Run 的
bothShapes 循环也自动带形状名」，写出自有断言的两形状循环时就会踩空。已列为 F2。

### 3.3 我另发现的一处：T3 测试对 hestia 形状是**空断言**（F1，非阻断）【实测】

`TestFailureMessagesNameTheShape` 的核心断言是
`assert.Contains(t, f.failureContext("建表"), f.name)`。
对 hestia 形状：`f.name = "hestia"`，而表名 `f.spec.table = "hestia_observations"` **恰好含有它**。

⇒ 在 B0-a（`failureContext` 去掉形状名、只留表名）下，实测失败输出只有两条：

```
Error: "（表 macro_observations）建表失败" does not contain "crisis"
Error: "（表 single_key_obs）建表失败"    does not contain "single-key"
```

**hestia 那次迭代照样通过** —— 它的断言被表名巧合满足，对这个变异完全无守护。
同函数末尾的 `assert.NotEqual(hestiaShape.failureContext, crisisShape.failureContext)`
在 B0-a 下也仍然成立（两个表名不同），同样抓不住。

整体仍 KILLED（靠 crisis / single-key 两条），故**不构成缺陷**。但这是本 Sprint 需求分析
开篇点名的风险——「**测试看起来齐全但守不住**」——的一个字面实例，值得记录。
可选加固：断言 `failureContext` 含 `"形状 "+f.name` 而非裸 `f.name`，一行改动即可让三套形状都有效。

---

## 四、functional[2]② 的实证修正 —— 机理核实通过

Leader 在派验时按 dev 的实证把「B 的 revision 必须**更大**」修正为「必须**不相等**」，
并要我核这个机理论证。**我独立复现，机理成立。**【实测】

| 变异 | 内容 | 结果 |
|---|---|---|
| F2-b（=M7a） | 共享方 revision 改成与对方**相等**（revLatest→revMiddle） | **KILLED** — `TestProbeKeepsT1MutationObservable/hestia` 转红 |
| F2-c（=M7b） | 共享方 revision 改**小**但不等（revLatest→revEarly） | **存活**，三条自证齐备 |

**关键在于「存活」的含义。** F2-c 存活意味着**全部 66 条测试通过**，其中包括
`TestProbeDistinguishesAndFromOr`——那条不是代理指标，它**直接**把 `correlate` 的真实输出做
`AND→OR` 替换、在探针数据上跑两版 SQL 比对结果。它保持绿，就等于**在把共享方 revision 改小
之后，T1 变异依然可观测**。这正是 Leader 转述的机理：

> `OR` 变异让两键互相拉进对方的 `MAX` 子查询，**较大的一方淹没较小的一方——谁大谁小都行；
> 相等时谁也淹没不了谁**。

改小之后只是「谁是较大方」换了人（原本是 monthly，现在是 h1），淹没关系依旧存在，故仍可观测；
改成相等则 `MAX` 子查询对两键给出同一个值，无人落选，`AND` 与 `OR` 输出相同，变异打不中。

⇒ **交付实现（`sharedColumnPair` 排序后断言严格大于）守护的正是「不相等」，既满足修正后的
DoD，也确实比原表述准确。** 【实测】

补充：我另跑 F2-e（把 `revMiddle` 常量值改成等于 `revLatest`，破坏三常量的序）→ **KILLED**；
而把 `revMiddle` 改成 `"2026-09-09"`（保序）→ **存活**。这说明守护钉住的是**序**而非绝对值，
是正确的设计——三个常量本就该是「有序的不透明记号」。

---

## 五、Leader 关注点 ③ —— code-simplifier 报告不实的处置复核

**背景**：code-simplifier 最终回复「No action — unchanged」，实际改了 4 处
（提取 `shapes` 局部变量、提取 `scanTargets` helper、`max`→`maxRev`、以及随之的两处调用点）。
dev 用 `diff` 直读发现，**没有假设等价，全量复跑 15 个变异**并针对新路径补了 M15。

### 5.1 四处改动是否语义等价？——【实测】用**反向变异**验证，不靠推理

我没有停在「读一遍觉得等价」。对三处实质改动**各自把它改回简化前的形态**，跑全包比对：

| 反向变异 | 结果 |
|---|---|
| D：把 `shapes := allShapes(t)` 还原为每次调用 `allShapes(t)` | rc=0，PASS=**66** → **行为一致** |
| E：把 `scanTargets(...)` 内联回原地取址循环（两处调用点都还原） | rc=0，PASS=**66** → **行为一致** |
| F：`maxRev` 改回 `max`（遮蔽内建） | rc=0，PASS=**66** → **行为一致** |

三处改动的前后两版**在全套 66 条测试上表现完全相同**，语义等价成立。
（第 4 处「调用点」随 E 一并验证。）

【推断，未单独跑】`shapes` 提取的等价性还有一条静态理由：`fixture` 是值类型，其中唯一的
可变成员 `probeCurrent map[string]float64` 从不被就地修改——`seedProbe` 在返回前复制到新 map
（源码 330-334 行）。因此「调一次复用」与「每次新建」不可区分。反向变异 D 是这条推断的实证。

### 5.2 M15 是否真的覆盖了新引入的 `scanTargets` 路径？——覆盖了，但那一击偏弱；我补了两记更狠的

dev 的 M15 = 「`scanTargets` 不取址」（`dest[i] = vals[i]`）。它确实 KILLED，但这是**廉价的一击**：
`rows.Scan` 见到非指针必然直接报错，任何改动都能红。它证明的是「这行代码被执行到了」，
**不太能证明「列的对应关系被守住了」**。

我另加两个针对同一路径、但只破坏**语义**不破坏**类型**的变异：

| 变异 | 内容 | 结果 |
|---|---|---|
| S-b | `scanTargets` **逆序**取址（列顺序错位，类型完全合法） | **KILLED** — `TestProbeCurrentMatchesInsertedData` + `TestProbeDistinguishesAndFromOr`，hestia/crisis 双形状均红 |
| S-c | `scanTargets` 全部指向 `vals[0]`（所有键列取同一格） | **KILLED** — 同上 |

⇒ **simplifier 新引入的路径不是靠「一碰就崩」蒙混过关，而是有实质语义守护。**【实测】
（S-b/S-c 对单列键形状打不中——单列时逆序等于原序、`vals[0]` 就是唯一格；红的都是
hestia/crisis 两个多列形状，这与预期一致，红的原因即我以为的那个。）

---

## 六、TASK-001 遗留复查项的回应 —— 核实通过【实测】

我在 TASK-001 验证报告第六节留的复查项是：「向本包新增文件时，`Spec` 一律经 `NewSpec` 构造，
不得用复合字面量」（因为包**内**这只是约定，不是 Go 的机制保证）。

`grep -n 'NewSpec\|Spec{' internal/macro/bitemporal/fixture_test.go` 全部命中：

```
464:  s, err := NewSpec("hestia_observations", []string{"period", "period_type"}, "published_at")
496:  s, err := NewSpec("macro_observations", []string{"ts", "indicator"}, "fetched_at")
531:  s, err := NewSpec("single_key_obs", []string{"period"}, "published_at")
```

**三个形状构造函数全部走 `NewSpec`，`Spec{` 复合字面量零命中。** 约定被遵守。
该复查项对 TASK-004/005 继续有效。

---

## 七、基础事实（全部实测）

```
$ GOTOOLCHAIN=local go test -count=1 -v ./internal/macro/bitemporal/
ok  github.com/newthinker/atlas/internal/macro/bitemporal  0.684s
--- PASS 计数: 66 | --- SKIP 计数: 0 | --- FAIL 计数: 0
$ go test -cover  → coverage: 100.0% of statements
$ gofmt -l fixture_test.go → 无输出
$ go vet ./internal/macro/... → 无输出，exit 0
```

- **公开面（non_functional[1]）**：`go doc ./internal/macro/bitemporal` 输出为
  `Key / Spec / NewSpec / State / Verdict / New / Classify` —— 与 TASK-001+002 后的基线逐项一致，
  `_test.go` **未增加任何公开符号**。
- **依赖（C1）**：`git show --stat d928dd8 -- go.mod go.sum` 无输出（未触碰）；
  `modernc.org/sqlite v1.38.2` 早已在 go.mod 第 19 行，非本任务引入。
- **声明范围 vs 实际改动**：声明 `writes = [fixture_test.go]`，`git show --stat d928dd8` 实际改动
  恰为该单文件。**无越界申报。**
- **non_functional[2]**：文档样例缺 `Context Checkpoint` 头注释，交付已自行补上（3-13 行），
  且格式与仓库惯例（`cmd/atlas/policy_test.go:13`）一致。

### 变异测试汇总：22 个，**20 KILLED / 2 存活**

其中 **7 个是 dev 未做的补充变异**：F0-d（crisis revision 列名撞 hestia）、F1-d（pragma 值错而非缺失）、
F2-e（常量序被破坏）、S-b（`scanTargets` 逆序取址）、S-c（全指向 `vals[0]`），以及组合变异
F2-f+F2-e、单改常量保序对照。**没有找到现有测试打不到的地方。**

---

## 八、非阻断观察与提请 Leader 处理的事项

**F1（非阻断，实测）** —— 第 3.3 节：`TestFailureMessagesNameTheShape` 对 **hestia 形状是空断言**，
因 `"hestia"` 是表名 `"hestia_observations"` 的子串。整体仍 KILLED，不构成缺陷。
可选加固一行：断言含 `"形状 "+f.name`。

**F2（非阻断，实测）** —— 第 3.2 节：discovery 中「t.Run 子测试名与消息本身构成双保险」的表述
对 `TestEachCallGetsAnIndependentDB` 不成立（无 t.Run；自身断言不带形状名）。
**建议在 TASK-004 的 DoD 里写明**：两形状循环若不用 `t.Run`，其**自有断言**必须自带形状名——
fixture 的 `failureContext` 只覆盖 fixture 侧失败。

**F3（提请 Leader，非阻断，需要动 TASK-004 DoD 措辞）** ——
DoD `functional[0]` 写「提供 hestiaShape(多列业务键)与 singleKeyShape(单列业务键)**两套形状**；
bothShapes() 让下游测试对**两套**并行跑」。dev 实现的是
`bothShapes = {hestia, crisis}`（两套**真实使用者**形状）、`allShapes = 三套`。

**我核过权威源，dev 是对的**：
- `design-spec.md:42-43` 原文即「红了也不知道是 **hestia 形状**还是 **crisis 形状**的问题」；
- `requirement-dod-matrix.md` 把「`:660` 两套 Spec 形状行为一致」与「`:663` 单列业务键可用」
  映射为**两条独立需求**（分别落 004 functional[1] 与 004 **boundary[2]**）。

⇒ 「两套形状」= hestia + crisis（都是两列键），「单列业务键」是**另一条**边界需求。
DoD `functional[0]` 的措辞是被压缩失真的那一环。**请据此校准 TASK-004 的 DoD 措辞**，
否则 TASK-004 若按 C8「bothShapes 覆盖」字面执行，单列键只会由 boundary[2] 单点覆盖，
容易被误读为「已由 bothShapes 覆盖」而漏掉。

**F4（文档瑕疵，非阻断）** —— `.arcforge/docs/01-design/requirements-analysis.md` 写
「crisis 形状**单列键**」，与 `spec.go` 包注释 `macro_observations: (ts, indicator)`（**两列**）矛盾。
这处不一致很可能就是 F3 措辞失真的源头。

---

## 九、判定

**PASS（verified）。**

依据（全部为实际运行命令的输出，非推测）：
1. 10 条 done_criteria **逐条**有对应测试，且**逐条**有变异实证其断言非空洞；
2. 包级 66 PASS / 0 SKIP / 0 FAIL，语句覆盖 100%；gofmt / vet 干净；
3. 22 个变异 20 KILLED / 2 存活，两个存活变异的**三条自证由我独立测量复现**（非采信记录）；
4. DoD 原文的两条明文判据均单独实证：T3 的 M14（失败消息指名 hestia）、
   functional[2]② 修正后的「不相等」机理（M7a KILLED / M7b 存活且直接证据测试保持绿）；
5. code-simplifier 的 4 处改动**用反向变异逐一验证语义等价**（三处各自还原后 PASS 仍为 66），
   并对其新引入的 `scanTargets` 路径补了两个只破坏语义不破坏类型的变异，均 KILLED；
6. TASK-001 遗留复查项（Spec 一律走 NewSpec）grep 核实通过；
7. 公开面未增符号、未触碰 go.mod/go.sum、声明范围与实际改动一致。

四条观察（F1/F2/F3/F4）已记录，均不构成 DoD 违反；**F3 需要 Leader 在 TASK-004 派发前校准
DoD 措辞**，F2 建议一并写进 TASK-004 的 DoD。

## 收尾
验证 worktree `../wt-verify-TASK-003` 已从主仓库执行 `git worktree remove` 拆除；
被验文件还原后 md5 `121b1d74bf9277955c66ce9ae56d99f1` 与基线一致，主工作区无污染。
