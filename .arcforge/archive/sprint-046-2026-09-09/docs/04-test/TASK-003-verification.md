# TASK-003 验证报告（verifier: test-m3-b）

> 结论：**VERIFIED**。7 条 `done_criteria` 全部 PASS，另记 3 条不构成缺陷的偏差/订正（第 ⑤ 节）。
>
> **本报告的全部结论都来自我自己在 loom 锚点树上的实跑**，不采信交付文档写着「绿」。
> 凡引用交付文档的数字处，均标注我是否独立复核以及复核结果。

## ⓪ 验证元信息

| 项 | 值 |
|---|---|
| 任务 | TASK-003（≡ 需求文档 TASK-002），`verifying` → `verified`，`assignment_epoch=1` |
| verifier | `test-m3-b` |
| `verify_baseline.head` | `755016eba82daaebee01721d1b86311f9d50a373` |
| **裁决落盘当刻** atlas HEAD | `7e24b116faff2174771d4551b54aa582a874d209`（写通道自证：`INFO: HEAD 已从 755016eba82d 前进到 7e24b116faff,但声明范围内无变更,判定对象未漂移`） |
| 证据采集期间 atlas HEAD | `4c9e7fdc35a3d4d6a6ac4dbbb0099ff998de626f` → 裁决前又前移至 `7e24b1…`；**两个时点在声明范围内的 diff 均为 0 行** |
| **漂移判定** | **声明范围内 diff = 0 行 ⇒ AD-29 INFO 级放行**，未用 `--ack-drift`（详见 ⑥ 节） |
| `verify_baseline.discovery_sha256` | `a177ed04f9a4b858bae2ae49a405c5f24b466719c682f4764e7038cf739e36fe`（判定前后两次比对**均一致**，discovery 未被改写） |
| loom 锚点 | `6d6c38f96901dc60f516ef2154283a213782ad5e`（分支 `feat/spool-source-param`） |
| loom 基线（父提交） | `0466116cc8c15c14ab8632d30d9be542fca5068c`（= loom `main` 当前 HEAD） |
| 验证 worktree | 建于 scratchpad、`--detach` 钉在全 sha，验毕拆除；loom 主工作区指纹全程未变 |

**锚点现读核对**（不采信 dev 自陈）：
```
git rev-parse feat/spool-source-param  → 6d6c38f96901dc60f516ef2154283a213782ad5e   ✔ 与文档一致
git rev-parse 6d6c38f...^              → 0466116cc8c15c14ab8632d30d9be542fca5068c   ✔ 与文档一致
git rev-parse main                     → 0466116cc8c15c14ab8632d30d9be542fca5068c   ✔ 即父提交
gh pr view 13 → state=OPEN, headRefOid=6d6c38f96901dc60f516ef2154283a213782ad5e, baseRefName=main
```
PR #13 **OPEN 未合并**，且 `headRefOid` 与锚点**逐位相同**。「只开不合」是 Leader 与需求的既定裁决（人执行前置），**不据此判缺失**。

---

## ① done_criteria 覆盖矩阵（逐条）

| # | 完成标准（摘要） | 我跑的验证 | 判定 |
|---|---|---|---|
| **functional[0]** | 四条新测试全 PASS，输出贴文档②节 | 锚点树实跑 4 条（含 2 子测试）全 PASS，退出码 0，输出与文档 §2.2 **逐字节一致**；另做 4 个定向变异逐条证实断言在守卫（②节） | **PASS** |
| **functional[1]** | `injectTaint(content, source string)`；**11** 个既有调用点全改；`allowedSources` 闭集；`defaultSource`；`reviewed: false` 无条件覆写不放松 | **在基线树 `0466116` 上独立复核** 11 这个数（③节）；锚点树 0 处仍单参；读 `taint.go` 确认闭集与两常量；变异 M1 证实覆写无条件 | **PASS** |
| **functional[2]** | `config.yaml` 加 `Wiki/Macro/PBOC`（dev 改）；`config.local.yaml` **不改**（人改）；`README.md` 字段表 + `source` 说明段 | `config.yaml` grep=1 ✔；`config.local.yaml` grep=**0**（**预期**，见 ⑤.3）且文档 4.1 所贴内容与我实读一致 ✔；README diff 两处均到位、明写「产物溯源，不是信任标记」✔ | **PASS** |
| **boundary[0]** | 空串 / 非字符串 `source` 走缺省不 panic；DENIED **不落盘** | 子测试 `empty-string` / `non-string(42)` 均 PASS；`os.Stat`+`os.IsNotExist` 断言 PASS；**另做结构性复核**：DENIED 在 `archive.go:37` return，`os.MkdirAll` 在 `:69` ⇒ 不落盘不依赖测试运气 | **PASS** |
| **error_handling[0]** | TDD RED 留痕，预期 `too many arguments in call to injectTaint` | 照文档配方在锚点上重放，输出与文档 §2.1 **逐字节一致**，退出码 1；并**直接证实**了「10 行是编译器截断上限」（④节） | **PASS** |
| **non_functional[0]** | loom 门禁全绿；39 条两把尺；gofmt/vet 零输出；无新依赖；code-simplifier；注释带 M3 前缀 | 逐项实跑，见 ②/③ 节；**四把尺同为 39**；基线 35(18/6/11) 亦由我在基线树独立跑出 | **PASS** |
| **non_functional[1]** | 交付流程：worktree 隔离、四节文档、锚全 sha、atlas merge 后才 dev_done、数字统一重采 | 15 处 sha **全为 40 位**、0 短 sha、`HEAD`/分支名出现处**全是叙述而非命令锚**；atlas merge 零越界；numstat 复核一致；`=== RUN 48` 一处错值（⑤.1） | **PASS** |

---

## ② 实跑证据

### 2.1 四条新测试（functional[0]）

```
$ go test ./internal/executors/spool/ -run 'TestArchiveSourceDefaultsToWebResearch|TestArchiveSourceHestiaAllowed|TestArchiveSourceUnknownDenied|TestTaintSourceParameterized' -v
=== RUN   TestArchiveSourceDefaultsToWebResearch
=== RUN   TestArchiveSourceDefaultsToWebResearch/empty-string
=== RUN   TestArchiveSourceDefaultsToWebResearch/non-string
--- PASS: TestArchiveSourceDefaultsToWebResearch (0.00s)
    --- PASS: TestArchiveSourceDefaultsToWebResearch/empty-string (0.00s)
    --- PASS: TestArchiveSourceDefaultsToWebResearch/non-string (0.00s)
=== RUN   TestArchiveSourceHestiaAllowed
--- PASS: TestArchiveSourceHestiaAllowed (0.00s)
=== RUN   TestArchiveSourceUnknownDenied
--- PASS: TestArchiveSourceUnknownDenied (0.00s)
=== RUN   TestTaintSourceParameterized
--- PASS: TestTaintSourceParameterized (0.00s)
PASS
ok  	github.com/newthinker/loom/internal/executors/spool	0.704s
退出码=0
```

### 2.2 断言不空洞 —— 4 个定向变异（隔离副本）

「测试 PASS」不等于「断言在守卫」。故在 **scratchpad 内的独立 worktree**（与 loom 主工作区、与我的验证 worktree 都不是同一份）上做单行变异，每体先过编译有效性闸、打印 diff 过语义闸，跑完立即还原并 **sha256 核对**。loom 主工作区 `git status --porcelain` 全程只有交付前就存在的那两项（`.claude/hooks/arcforge-write.sh` 改动 + 一个未跟踪脚本），**spool 文件一个字节未被碰**。

| 变异 | 单行改动 | 转红的测试 | 判定 |
|---|---|---|---|
| **M1** | `taintReviewed = "reviewed: false"` → `"reviewed: true"`（放松 Global Constraint） | **`TestArchiveSourceHestiaAllowed`** + 13 条既有 taint 测试，共 14 红 | KILLED ✔ |
| **M2** | `if !allowedSources[source] {` → `if false {`（闭集失效） | **`TestArchiveSourceUnknownDenied`**，**独家 1 红** | KILLED ✔ |
| **M3** | `defaultSource = "web-research"` → `"hestia"` | **`TestArchiveSourceDefaultsToWebResearch`** + 6 条既有，共 7 红 | KILLED ✔ |
| **M4** | `isTopTaintKey` 不再丢弃入站 `source`（洗白入口） | **`TestTaintSourceParameterized`** + 4 条既有洗白测试，共 5 红 | KILLED ✔ |

**M2 独家转红**这一条最有分量：`allowedSources` 闭集这个性质，在整个 39 条套件里**只有** `TestArchiveSourceUnknownDenied` 守着 —— 它不是锦上添花的重复覆盖，它是该性质的唯一闸。

`reviewed: false` 无条件覆写另有**代码侧**佐证：`injectTaint` 里是
`kept = append(kept, "source: "+source, taintReviewed)` —— 无分支、无条件，与 `source` 取值无关。

### 2.3 全量回归（non_functional[0]）

```
$ go test -count=1 ./internal/executors/spool/ -v
退出码=0   顶层 --- PASS = 39   任意层 --- FAIL = 0

$ go test ./...
退出码=0   ^FAIL 行数 = 0   ^ok 行数 = 23

$ gofmt -l internal/executors/spool        → 零输出，退出码 0
$ go vet ./internal/executors/spool/...    → 零输出，退出码 0
$ git diff --numstat 0466116cc8c15c14ab8632d30d9be542fca5068c 6d6c38f96901dc60f516ef2154283a213782ad5e -- go.mod go.sum
                                           → 0 行 ⇒ 无新增 Go 依赖
```

### 2.4 测试条数 —— 我用了**四把**尺，四把同值

| 尺 | 口径 | 值 |
|---|---|---|
| 1 | 静态 `grep -c '^func Test'` | archive 21 / commit 6 / taint 12 = **39** |
| 2 | 动态 `go test -v` 顶层 `--- PASS` | **39** |
| 3 | 动态 顶层 `=== RUN`（`^=== RUN   [^/]*$`） | **39** |
| 4 | 静态 唯一测试函数名去重计数 | **39** |

**基线亦由我独立跑出**（基线树 `0466116`）：尺1 = 18/6/11 = **35**，尺2 = **35**。故 35 + 4 = 39 成立，两端都不是转述。

> ⚠️ `=== RUN` 总数（含 `t.Run` 子测试）与上面四把尺**口径不同，不可相减**。其真值见 ⑤.1。

### 2.5 配置与文档（functional[2]）

`configs/config.yaml`：
```yaml
spool_write_allow:
  - Wiki/Loom-Research           # Plan 2 Warp 调研正式落点
  - Wiki/Macro/PBOC              # Hestia M3 解读笔记落点
```
`configs/README.md` 两处均到位：字段表那一行改为「`Wiki/Loom-Research`(Plan 2)与 `Wiki/Macro/PBOC`(Hestia M3)」；Reed 段之前新增 `## spool.archive 的 source 参数(M3)`，正文明写「**它是产物溯源,不是信任标记——`reviewed: false` 对任何 source 都无条件覆写**」。

---

## ③ 「11 个既有调用点」—— 我在**基线树**上独立求值

DoD 与交付文档都是**在改后的树上**倒推这个数的。我改用更直接的仪器：**直接去基线树数**。

```
$ git grep -n 'injectTaint(' 0466116cc8c15c14ab8632d30d9be542fca5068c -- internal/
  archive.go:31            content = injectTaint(content)          ← 生产调用 1
  taint.go:57         func injectTaint(content string) string {    ← 函数定义（非调用点）
  taint_test.go:33/49/72/103/119/129/146/195/226/262               ← 测试调用 10
基线总命中 = 12
```

⇒ **既有调用点 = 12 − 1（定义行）= 11**，构成为 `archive.go` 1 + `taint_test.go` 10。
**DoD 初稿的「12 处」确系把函数定义行算了进去；订正为 11 正确。**

锚点树复核：总命中 **13** = 1 定义 + 1 生产调用（`injectTaint(content, source)`）+ 11 测试调用（10 既有 + 1 新写）。
```
$ grep -rn 'injectTaint([^,)]*)' internal/ | grep -v 'func injectTaint' | wc -l   → 0
```
**零处仍为单参** ⇒ 11 个既有调用点全部改完。

> DoD 字面写「全部改为 `injectTaint(in, "web-research")`」。实际 `taint_test.go:262` 是
> `injectTaint("---\n---\n", "web-research")`（第一个实参本就是字面量，不是 `in`），
> `archive.go` 是 `injectTaint(content, source)`。这是 DoD 的简写指代「补第二个实参」，
> **不是不符**——第一个实参本就该保持原样。

---

## ④ 「RED 输出 10 行是编译器截断上限」—— 直接证实，不是推理

Leader 要求复核 dev 的这个解释。我没有停在「读起来合理」，而是**取到了一个观察**：在同一棵 RED 树上加 `-gcflags=-e`（解除 gc 每包错误上限）重跑。

| 跑法 | `too many arguments in call to injectTaint` 行数 | `too many errors` 截断标记 |
|---|---|---|
| 默认（文档所贴） | **10** | **1**（`taint_test.go:262:35: too many errors`） |
| `-gcflags=-e` | **11** | **0** |

（`go version go1.24.4 darwin/arm64`）

⇒ **dev 的解释成立**：10 是 gc 的默认上限，不是调用点计数。那份 RED 证据应当这样读。

⚠️ 但请注意一个**易被误连的巧合**：这里解除上限后的 11，与 ③ 节的「11 个既有调用点」**恰好同值，却不是同一个集合**——RED 树把 `taint.go`/`archive.go` 回退到了基线，故 `archive.go` 的调用是单参、不报错；这 11 个错全在 `taint_test.go`（10 个既有 + 第 270 行**新写**的那个）。两个 11 各自成立，**不要把其中一个当成另一个的佐证**。

---

## ⑤ DoD 与交付的已知偏差（**留痕用，均不构成 rejected**）

### ⑤.1 🔴 一处**错值**：`=== RUN` 是 **50** 不是 48

交付文档 §2.5 与 `discoveries/TASK-003.json` 的 `key_findings[3]` 都写「`=== RUN` 是 48 条」。**我实测是 50**，且经自洽校验：

```
顶层 === RUN = 39   子测试 === RUN = 11   39 + 11 = 50 = 总数 ✔
（照文档自己给的命令 `go test ./internal/executors/spool/ -v` 跑，同样得 50）
```

**成因**（我用基线树定位）：基线树 `=== RUN` 总数 = 44 = 35 顶层 + **9** 子测试。dev 的 48 = 39 + **9** —— 沿用了基线的子测试数，**漏计本次自己新增的 2 条子测试**（`empty-string` / `non-string`），差值恰为 2。

**严重程度：minor，不构成 rejected。** 理由：
- 它不是任何 `done_criteria` 的判据。DoD 要的是 39，而 39 经**四把独立的尺**同值确认，两端（基线 35 / 锚点 39）都由我独立跑出。
- 这句话在文档里的**功能**是一句告诫（「`=== RUN` 与 `func Test` 口径不同，不可相减」），该功能与它是 48 还是 50 无关。
- 但它仍是一个**自证数字**，而自证数字是验证者的判定原料 ⇒ 必须留痕。**建议**：若后续有人引用「48」去做任何计算，以本报告的 50 为准。

### ⑤.2 DoD `functional[1]` 的「12 处」→ 订正为 **11**（Leader 计数错误，dev 提出，已写进 `done_criteria`）

- DoD 初稿写「**12 处**既有调用点全部改为 `injectTaint(in, "web-research")`」。
- 真相：12 是 `grep -rn 'injectTaint(' internal/executors/spool/` 的**命中数**，其中 `taint.go:57` 是**函数定义行** ⇒ 既有调用点 **11** 个。
- **我在基线树 `0466116` 上独立复核，确认订正正确**（③节）。dev 已把订正写进 `done_criteria functional[1]`，载体正确。
- **按 11 验，数出 11 不是缺陷。** 此处单列以留痕：这是 DoD 与实际之间的一处已知偏差，由 Leader 的计数错误引入。

### ⑤.3 DoD `functional[2]` 的「两处都加」→ 裁决为 dev 只改一处

`grep -c 'Wiki/Macro/PBOC' configs/config.local.yaml` = **0** 是**预期结果**。该文件被 loom `.gitignore` 忽略（`.gitignore:54`），属需求 TASK-006 Step 0 的人执行前置。裁决已由 dev 写进 `done_criteria functional[2]`。我独立实读了该文件，其 `spool_write_allow` 仍只有 `Wiki/Loom-Research`，**与交付文档 4.1 所贴逐字一致**。**不据此判 rejected。**

### ⑤.4 `error_handling[0]` 的口径偏离：原始 RED → 锚点上确定性重放

DoD 字面要「**实现前**的失败输出原样贴」。dev 改为给出「在锚点上把 `taint.go`/`archive.go` 单独 checkout 回父提交」的可复现配方（`decisions` 有 rationale）。

**判 PASS，且认为这是更强的证据形态**：原始 RED 跑在一棵已不存在的树上，验证者只能选择相信贴出来的文本；重放配方我照跑了，拿到的输出与文档所贴**逐字节一致**。DoD 的目的（证明测试确曾因签名不符而红）被完全满足，且可被任何后来者复现。

---

## ⑥ 越界申报与漂移

**声明范围**：`writes` = `packages` = `["./docs/hestia-m3/TASK-003-loom-spool-source.md"]`。

```
$ git show --numstat --format='' 755016eba82daaebee01721d1b86311f9d50a373
421	0	docs/hestia-m3/TASK-003-loom-spool-source.md      ← 唯一一个文件
```
**零越界**：合入 master 的 merge commit 只动了声明内的那一个文件。分支 `task/TASK-003-m3` 相对分叉点亦只有这 1 个文件、421 行新增。

**内容判据核实合入**（不用拓扑判据——rebase/amend 会换 sha）：
```
master(755016e) 上该文件 sha256 = 39542d6e3f7dbf13c52957217ed7210987344d1403dbb52a176f5d56a0f11bf8
09e9573 版             sha256 = 39542d6e…（相同）
工作树当前            sha256 = 39542d6e…（相同）
discovery 记录值               = 39542d6e…（相同）
```
四方一致 ⇒ 合入的确实是 `09e9573fb733833db99fa6946ec721dc6daec1c5` 那一版。commit subject 为 `docs(TASK-003): …`，符合门禁锚定要求。

**验证对象漂移（AD-29）**：本次验证期间 atlas HEAD 前移了**两次**（`755016e` → `4c9e7fd` → `7e24b1`，均为 TASK-001 的 `internal/hestia/*` 提交）。按判据（`writes` 优先）在**两个时点各核查一次**：

```
$ git diff --numstat 755016e… 4c9e7fd… -- docs/hestia-m3/TASK-003-loom-spool-source.md   → 0 行
$ git diff --numstat 755016e… 7e24b1… -- docs/hestia-m3/TASK-003-loom-spool-source.md   → 0 行
```

**声明范围内两次均 0 行变化** ⇒ INFO 级放行，未使用 `--ack-drift`。写通道在 `verified` 迁移时自行打出的判定与此一致：
`INFO: HEAD 已从 755016eba82d 前进到 7e24b116faff,但声明范围内无变更,判定对象未漂移`。
`discovery_sha256` 判定前后两次比对均与 baseline 一致，**判定原料未被改写**。

> 📌 自证纪律留痕：本报告初稿把「判定时 HEAD」记为证据采集期的 `4c9e7fd`，而裁决实际落在 `7e24b1`
> ——采样时点早于最后一次相关事件。结论不受影响（两个时点范围内均 0 行），但数字本身已按
> 「一切自证数字必须在最后一次改动之后统一重采」订正。我在 ⑤.1 指出了 dev 的同类问题，
> 不能只律人不律己，故此处一并留痕。

---

## ⑦ 未做与结转（交付文档 ④ 节，我逐条独立核实）

| 项 | 文档所述 | 我的核实 |
|---|---|---|
| `configs/config.local.yaml` 未改 | 由人改，gitignored | ✔ 实读确认仍只有 `Wiki/Loom-Research`；`git check-ignore` 确认被 `.gitignore:54` 忽略 |
| selvage 未重启 | 人执行（需求 TASK-006 Step 0） | ✔ agent 明令不执行，无异议 |
| `Wiki/Macro/PBOC/` 是否自动创建 —— **未实证** | spec 声称自动建，本 sprint 未端到端验证 | ✔ 我独立 `ls`：`Wiki/` 存在，`Wiki/Macro/` 与 `Wiki/Macro/PBOC/` **均不存在**，与文档一致。**文档的写法是正确的**——它明确区分了「spec 的设计意图声称」与「本 sprint 的实测」，没有把断言当证据 |
| loom PR #13 未合并 | 合并由人决定 | ✔ `gh pr view` 确认 `state=OPEN`，`headRefOid` == 锚点 |
| loom 既有 4 条 `TASK-` 注释 | 非本次越界 | ✔ **在基线树 `0466116` 上独立核实**：那 4 条（`archive_test.go:336/358`、`commit_test.go:3`、`config.yaml:117`）确为既有。锚点树 `TASK-` 共 9 条，其中 5 条为本次新写且**全部带 `M3` 前缀** ⇒ `non_functional[0]` 的 milestone 前缀义务满足 |

**锚点纪律复核**：交付文档中 40 位全 sha 共 15 处（`6d6c38f…` 11 次、`0466116…` 4 次），**短 sha 0 处**。`HEAD` / `feat/spool-source-param` 的每一次出现都是**叙述性标注或反面告诫**（如「`main` 的当时 HEAD」「请勿改用 `main` 或 `HEAD` 复跑」），**没有一处被当作命令锚使用**。符合「验证/隔离命令里的锚必须钉全 sha」。

---

## ⑧ 结论

**VERIFIED。**

7 条 `done_criteria` 逐条 PASS，且关键性质（`reviewed: false` 无条件覆写、`allowedSources` 闭集、DENIED 不落盘、缺省回落）除测试通过外，另有**变异测试**与**代码结构**两路独立佐证。三条 DoD/交付偏差（⑤.2 计数订正、⑤.3 `config.local.yaml` 裁决、⑤.4 RED 口径）均已确认为**已裁决或已订正的已知偏差，不是交付缺陷**；一处自证数字错值（⑤.1 `=== RUN` 48→50）属 minor 旁注，不触及任何判据，已留痕。

**给 Leader 的一句话**：本 sprint 目前的 DoD 缺陷率仍指向同一方向——⑤.2 与 ⑤.3 两条都是 DoD 侧的问题，dev 两次都是对的，且两次都把订正写进了 `done_criteria`（正确的载体）。交付本身我没找到缺陷。

**清理**（下述动作已执行并核实后才落笔）：我建的 3 个 loom worktree（verify / red / base，均在 scratchpad）已全部 `worktree remove --force` + `prune`，`git worktree list` 只剩 loom 主工作区，scratchpad 无 `wt-*` 残留；loom 主工作区 `git status --porcelain` 与 HEAD 与开工时逐项相同。
