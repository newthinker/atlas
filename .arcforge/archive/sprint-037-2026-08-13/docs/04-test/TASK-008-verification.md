# TASK-008 验证报告 —— T6：cobra 装配（`atlas hestia ingest` / `status`）

- **验证者**：test-agent-26 ｜ **交付**：dev-agent-53，提交 `d86de295b5405242c445d712fa7281c0a66dca94`
- **验证基线**：`verify_baseline.head = d86de295…` = 承接时 HEAD ｜ `assignment_epoch`：1
- **结论**：**VERIFIED**（8/8 条 done_criteria 全部 PASS）

---

## 0. 承接核实

| 核项 | 结果 |
|---|---|
| HEAD 漂移 | 无 ✅ |
| discovery 内容漂移 | 无 ✅（`f4118812bef6…` 与基线一致） |
| **DoD 未被改写** | 指纹 `04cef643d2bb146c` == 我 `16:49:58Z` 存的 wave3 基线 ✅ |
| 实际改动 vs `writes` | `hestia.go` + `hestia_test.go`，与声明**逐字一致**，无越界 ✅ |
| `discovery` 指针 | ✅ 已挂，**dev 自己挂的**（第二次走通「dev 在 `dev_done` 前自挂」这条路） |

### 🔴 pre-image 比对：**它只加测试、没动实现**（独立证据，非采信自陈）

dev-53 曾 `in_progress` 冻结约 40 分钟（mtime 停在 `01:16`），我在 UTC `17:52:34` 存了它在途文件的
pre-image。**交付后比对**：

| 文件 | pre-image | 交付态 | 判定 |
|---|---|---|---|
| `cmd/atlas/hestia.go` | `eb6ce9d67c2ed98e`（131 行） | **同**（131 行） | **一个字节未变** |
| `cmd/atlas/hestia_test.go` | `2bf290436fc6827e`（178 行） | `f762ae92843de138`（223 行） | **+45 行，纯加测试** |

⇒ **「它没有为覆盖率重构 `runHestiaIngest`、也没写低价值用例凑数」这句话，我有独立证据**，
不依赖它或 Leader 的转述。这正是本 Sprint 那条通则（**在对象可能被改动之前存一份带 sha 的副本**）第一次真正兑现。

---

## 1. 完成标准覆盖矩阵（8 条）

| # | done_criteria | 证据 | 判定 |
|---|---|---|---|
| functional[0] | 交付 `hestia ingest` / `status` 两子命令，装配七个组件 | `TestHestiaCommandIsRegistered`；`hestia.go` 装配 `LoadConfig`/`NewStore`/`NewPBOCFetcher`/`Ingest`/`RecentObservations`/`RecentPending`/`RenderStatus` | **PASS** |
| functional[1] | **不在 cmd 层解析绝对路径**（归 `RenderStatus`） | `hestia.go` 内 `filepath` 出现 **0 次**；`TestHestiaCmdDoesNotResolveDBPath` 用 **AST 检查 import** 钉住。消融 M2 证明其鉴别力（见 §3） | **PASS** |
| boundary[0] | `--hestia-config` / `--force` flag；配置不存在/非法 ⇒ 错误传播到退出码 | `TestHestiaFlags`；`TestHestiaStatusFailsOnBadConfig`、`TestHestiaIngestPropagatesConfigError`、`TestHestiaStatusFailsWhenStoreCannotOpen` | **PASS** |
| boundary[1] | `status` 在**空库**上跑通 | `TestHestiaStatusOnEmptyStore` | **PASS** |
| error_handling[0] | 配置装载失败的错误信息**含配置文件路径** | `TestHestiaConfigErrorNamesThePath` | **PASS** |
| error_handling[1] | **必须设 `SilenceUsage`**，且**不要动 `main.go` 的 `rootCmd`** | `hestia.go:26/33/40` 三个命令各设一次；**`main.go` 改动 0 处**；`crisis` 测试实跑仍绿。消融 M1 证明其鉴别力 | **PASS** |
| non_functional[0] | 覆盖率处置 + **验证者文件级独立复核** | 门禁放行（`75.6% ≥ coverage_floor 75`）；**我的独立复核见 §2** | **PASS** |
| non_functional[1] | `gofmt -l cmd/atlas/` 空、`go vet ./...` 空、`go build ./...` 绿、`go test ./... -count=1` 全绿 | vet 空、build OK、全包测试全绿；**`gofmt` 一项见 §4** | **PASS**（见 §4 的限定） |

**九个新测试与 DoD 逐条对应**，无未覆盖项。

---

## 2. 文件级独立复核（DoD `non_functional[0]` 明文要求的那条）

**三个口径我独立复算，与 Leader 报的逐字一致**：

```
hestia.go        25/30 = 83.3%
整包             1006/1334 = 75.41%
排除 hestia.go    981/1304 = 75.23%
```

**5 条未覆盖语句，逐条处置**：

| 位置 | 条数 | 是什么 | 我的判定 |
|---|---|---|---|
| `hestia.go:103`（两处） | 2 | `runHestiaIngest` 的 `defer st.Close()` | **理由成立**：要走到它必须先让 `openHestia()` 成功、再进入 `hestia.Ingest(...)`，而后者构造真实 `NewPBOCFetcher` 会发 HTTP |
| `hestia.go:105-111` | 1 | `return hestia.Ingest(...)` | **理由成立**：同上，**主路径本身需要真实网络** |
| `hestia.go:123-125` | 1 | `RecentObservations` 出错分支 | **理由成立**：需「查询中途出错」，可靠触发要引入时序竞争；且该错误行为**已在 `internal/hestia` 层测过**（TASK-006 的 `TestRecentObservationsWrapsQueryError`），cmd 层只是 `if err != nil { return err }` 的传播 |
| `hestia.go:127-129` | 1 | `RecentPending` 出错分支 | **理由成立**：同上 |

⇒ **5 条的理由全部成立。** 判据依据是 DoD `functional[0]` 自己写的边界：
「**这一层只做装配**，ingest 的真实行为已在 `internal/hestia` 测过，cmd 层只覆盖『装配对不对』」——
那 5 条恰好全部落在「**不是装配、而是被装配物的真实行为**」这一侧。

📌 **它没有为覆盖率做重构**（`hestia.go` 一字未动，见 §0），**也没有写低价值用例把整包灌高**
（它新增的测试全部指向具体 DoD 条目）。这两件正是本条判据要防的。

> 补一句同文件内的既定惯例（可供后人判断新函数该不该被要求覆盖）：`runHestiaStatus` 未覆盖的
> 也恰是错误分支 ⇒ **本文件已确立「主路径测、错误分支可不测」**。而 `runHestiaIngest` 的
> 特殊之处是**主路径本身测不了**，与「错误分支没测」不同类。

---

## 3. 消融（DoD 未强制，我作为独立验证做）

| # | 变异 | 结果 | 外溢 |
|---|---|---|---|
| M1 | 撤掉第一个 `SilenceUsage: true` | 仅 `TestHestiaCommandsSilenceUsage` 红在 `hestia_test.go:69` | 204+1=205 ✅ |
| M2 | 在 cmd 层加 `filepath.Abs(cfg.Storage.DBPath)` | 仅 `TestHestiaCmdDoesNotResolveDBPath` 红在 `hestia_test.go:97` | 204+1=205 ✅ |

⇒ 两条关键守卫都有**真实鉴别力**，且各自只红自己那条。
M2 尤其值得记：它守的是一个**否定性架构约束**（「这一层不该做某事」），而它选的判据是
**AST 层面「不 import `path/filepath`」** —— 比断言输出字符串更难绕过。

---

## 4. ⚠️ `gofmt` 那一条：DoD 字面不满足，**但不是本次交付的问题**

DoD `non_functional[1]` 要求「`gofmt -l cmd/atlas/` **空**」。**实测不空**：

```
gofmt -l cmd/atlas/  →  cmd/atlas/backtest_test.go   cmd/atlas/crisis_test.go
```

**核实结论：历史遗留，与 dev-53 无关**

- **在交付前的 base（`076998be`）上跑同一条命令，输出是同样这两个文件** ⇒ 它进场时就不合规
- **`gofmt -l cmd/atlas/hestia.go cmd/atlas/hestia_test.go` 输出为空** ⇒ 它自己交付的两个文件完全合规
- 差异内容是**注释对齐**（`gofmt` 版本间对齐规则变化的典型形态），非语法问题
- 那两个文件**不在它的 `writes` 里**，它去改反而是越界

⇒ **判 PASS（实质满足）**：DoD 的意图是「你交付的代码符合 gofmt」，这一点成立。

📌 **但这条 DoD 的写法有结构性问题，值得回流**：
「`gofmt -l <整个目录>` 空」在**存在历史遗留的目录**里是**任何任务都不可能满足**的要求
（除非先单独授权修那两个文件）。⇒ 正确写法应是「**`gofmt -l <本任务改动的文件>` 空**」。
本次没造成损害是因为我去核了 base；**若验证者不核 base，会把一个历史包袱记到当前交付头上**。

---

## 5. 实跑证据（隔离 worktree @ `d86de295`）

```
go vet ./...      → 空
go build ./...    → OK
go test ./... -count=1        → 全包全绿（含 crisis、router、storage、strategy 等）
go test ./cmd/atlas/ -count=1 -cover → ok  coverage: 75.4%
门禁放行记录：Task scope passes. Coverage: 75.6% (dev_minimum: 75%)
cmd/atlas 顶层 PASS 205 / FAIL 0
```

---

## 6. 复现（锚已钉全 sha）

```bash
git worktree add --detach ../wt-v-t008 d86de295b5405242c445d712fa7281c0a66dca94
GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -coverprofile=/tmp/c.out
# 文件级：按语句数汇总 cmd/atlas/hestia.go 的行 ⇒ 25/30
# base 对照（证明 gofmt 两文件是历史遗留）：
git worktree add --detach ../wt-base 076998be05fcc8961a5d28c18a1767cea70132dd && gofmt -l cmd/atlas/
```

---

# 复验（QA 第二轮 `review_fix`）—— **PASS**

- **判定对象**：`f4d601753ac323ca37d5757da5a547493e3de090`（= `verify_baseline.head`，**无漂移**）
- discovery 无漂移（`4e44dafe6208`）｜ DoD 指纹 `04cef643d2bb146c` 与 wave3 基线一致 ｜ `rework_count=1`
- **基线含那 38 行** ✅（`TestHestiaFlagsBindToVariables` 命中 1）｜ 改动 **38 行纯新增**，只动 `hestia_test.go`，在 `writes` 内

## 4 条 `fix_items` 逐条核

| # | 要求 | 证据 | 判定 |
|---|---|---|---|
| ① | CRITICAL：`--force`/`--hestia-config` 绑定无守卫 | 已补 `TestHestiaFlagsBindToVariables` | **PASS** |
| ② | **经 cobra 参数解析**验、判据用**肯定式** | `hestiaIngestCmd.Flags().Parse([]string{"--force"})` 后 `assert.True(hestiaForce)`；`PersistentFlags().Parse(...)` 后 `assert.Equal(want, hestiaCfgPath)`。**不是 `Lookup` 存在性** | **PASS** |
| ③ | 那 38 行是 dev-52 写的，须写清归属 | `discovery.files_modified` 明写「返工追加 38 行 flag 绑定守卫，**作者 dev-agent-52**」，另 `rework`/`notes_for_leader` 亦提及 | **PASS** |
| ④ | 验收：M13/M14 **分别做不合并**，各红自己那条 | 见下，**我独立复跑** | **PASS** |

守卫本身另有两处好设计：`t.Cleanup` 保存/恢复全局变量（不污染其他测试）；两个子测试**分开**，因而能区分是哪一个绑定断了。

## M13 / M14 对照（**含变异生效自证**）

| | 变异生效自证 | `--force` 子测试 | `--hestia-config` 子测试 |
|---|---|---|---|
| 基线（未变异） | — | PASS | PASS |
| **M13** `BoolVar`→`Bool` | ✅ `diff=1+1` 行 | **FAIL** `hestia_test.go:559` | PASS ← **不误伤** |
| **M14** `StringVar`→`String` | ✅ `diff=1+1` 行 | PASS ← **不误伤** | **FAIL** `hestia_test.go:567` |

**与旧树基线合起来才是完整对照**（旧树 = `b6b13a4`，那 38 行尚未提交）：

| 变异 | 旧树 | 新树 | 含义 |
|---|---|---|---|
| M13 | **全绿** | **红** | ✅ 修复堵住了「`--force` 绑定断裂」 |
| M14 | **全绿** | **红** | ✅ 修复堵住了「`--hestia-config` 绑定断裂」 |

⇒ **两个 flag 的绑定原本都无守卫**，不只是 `--force`；`fix_items` 要求「分别做不合并」是对的 ——
它们是**两个独立缺陷，只是形状相同**。

### 🔴 为什么这次特意自证「变异生效」

dev-53 在自测时第一次跑 M13 **perl 模式没匹配上**（`--force` 的帮助文本已被 TASK-009 的返工改过），
文件一个字节没变、测试自然全 PASS，**而它当时已经打印了「变异有效」**。它靠 diff 为空发现了。

⇒ **「测试没红」有三种成因，输出上完全同形而结论互斥**：

| 成因 | 结论 |
|---|---|
| 守卫无效 | ❌ 缺陷未修 |
| **守卫不在被测的树里** | ⚠️ 判定对象错了（**我上一轮跑 M4 正是这种**） |
| **变异没生效** | ⚠️ 取证无效（dev-53 这一轮） |

⇒ 故本次每条变异先打印 `git diff --numstat` 与实际替换内容，**非空才继续**；
变异锚点也刻意**只取到变量名为止、不含帮助文本**，避开那个坑。

## 实跑（隔离 worktree @ `f4d6017`，退出码不跨管道取）

```
gofmt -l <本任务文件> → 空   go vet ./... → exit 0   go build ./... → exit 0
go test ./... -count=1 → exit 0，无 FAIL      -race → exit 0
覆盖率 cmd/atlas 75.4%（≥ coverage_floor 75）
```

## 我在本次复验中两次差点误判，如实记

1. **查 `provenance` 字段 → 空** ⇒ 一度以为「作者归属没写」。实际这份 discovery **没有 `provenance` 字段**，
   归属写在 **`files_modified`** 里。⇒ **查错载体**。
2. 随即怀疑「这条要求会不会又是只在消息里说」⇒ 查 `fix_items`，**命中 1，是落盘了的**。⇒ 怀疑也错。

⇒ 两次都是继续查才排除。**与本 Sprint 那族（正确性附着在载体上）同形，只是这次载体是「discovery 的哪个字段」。**
📌 加上前一轮复验的 5 次 grep 误导，**本轮返工验证中我的工具/位置用错共 7 次，全部靠读原文或换位置排除，0 次误判落地。**
