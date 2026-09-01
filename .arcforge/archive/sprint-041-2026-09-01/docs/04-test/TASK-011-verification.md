# TASK-011 返工验证报告（第 2 轮）—— C-1：stock_continuity 不再拿「最近已接受的一期」冒充「相邻上一期」

- **验证者**：test-m1c3b-b ｜ **`rework_count`**: 1 ｜ **`reason_class`**: `dod_defect`
- **判定对象树**：`188eb212009a0bd7f210059ddd15a4c3b759e96f`（= `verify_baseline.head`）
- **返工前树**：`21815be` ｜ **结论**：✅ **VERIFIED**

## 0. 基线与门禁时点

| 项 | 记录 | 实测 | 判定 |
|---|---|---|---|
| `head` | `188eb212009a0bd7f210059ddd15a4c3b759e96f` | 同值 | ✅ 零漂移 |
| `discovery_sha256` | `8f613166b783901ed46b3385c75e97b27b142f3c636ca6734cbd4ecdfbbb2e12` | 同值 | ✅ |

**返工链**（审计行）：`05:47:50Z` verified→review_fix（leader）→ `05:49:31Z` →in_progress（dev）→
`05:50:35Z` **越界申报**（`update` 补 `store.go` 进 `writes`）→ `06:02:17Z` →dev_done → `06:03:18Z` 派验。
⇒ 越界申报早于 `dev_done` **11 分 42 秒**，走的是 `update` 非 `transition`，合规。

---

## 1. 🔴 裁决 7 那道闸：判据必须先被证明能红 —— **通过**

> 一条从没有人见它红过的判据，不是被验证了，是被声称了。

⚠️ 该闸不在 `done_criteria` 里（grep 计数 0，与 Leader 所述一致）——唯一载体是 `plan.md`「Leader 裁决记录」裁决 7。
我读了原文（41 行起），不采信转述。

**M1（整段拿掉相邻性判据 = C-1 缺陷原状）**，隔离 worktree 钉死 `188eb212…`：

```
转红(顶层): TestStockContinuitySkipsNonAdjacentPrior + TestStockContinuityDoesNotUseRejectedPeriodAsBaseline
转红的格 (4/8): monthly 跨一期 / monthly 跨 13 个月 / annual 跨 3 年 / q1 跨两期
```

⇒ **新断言确实能红**，且红的恰好是 `adjacent=false` 的 4 格。闸通过。

---

## 2. 🔴 我加做的 M2：**空实现检测** —— dev 的消融里没有这条

Leader 点名要我独立复核 `passed=54`（「排除空实现的唯一证据」）。我没有停在复核那个数，
而是先构造了**空实现本身**：

**M2（相邻性判据恒真 ⇒ 一律 skip）**：

```
转红(顶层): SkipsNonAdjacentPrior + DetectsJump + SkipReasons + SkipsOnZeroDenominator
转红的格 (4/8): monthly 相邻 / monthly 跨年相邻 / annual 相邻 / h1 相邻
```

### M1 与 M2 的结果**完全对称**，这是本次验证最强的一处

| 变异 | 转红的格 | 守的方向 |
|---|---|---|
| **M1** 拿掉判据（缺陷原状） | 恰好 4 格 `adjacent=false` | 「不相邻必须 skip」——防 C-1 复现 |
| **M2** 判据恒真（空实现） | 恰好 4 格 `adjacent=true` | 「相邻超限必须 failed」——**防空实现** |

⇒ 8 个格分成两组、各守一个方向，**无一格是摆设**。
`adjacent=true` 那 4 格的注释自陈「这一格是防『一律 skip』的空实现」——**M2 证明这句话属实**。

---

## 3. 🔴 `passed=54` 的复核 —— 数字命中，且我证明了它**为什么是必需的**

### 3.1 独立复核（探针重放 96 组，用真跑后的 store 作 History）

| 项 | dev 自报 | 我实测 | 一致 |
|---|---|---|---|
| 三态 | passed=54 / failed=0 / skipped=42 | **54 / 0 / 42** | ✅ |
| `absent_field:tsf_stock` | 17 | **17** | ✅ |
| `non_adjacent_prior` | 20 | **20** | ✅ |
| `prior_absent_field` | 3 | **3** | ✅ |
| `no_prior_period` | 2 | **2** | ✅ |

`non_adjacent_prior = 20` 印证了 dev 那条 QA 没看见的发现：**4 条伪环比超限被拒（QA 看见的）
+ 16 条碰巧没超限、静默通过** —— 假通过是假拒绝的 4 倍。

### 3.2 🔴 我在探针前推断、随后被证实的一点：**真跑那三个数区分不了空实现**

`stock_continuity` 无论 `skipped` 还是 `passed` 都不翻转 `Passed`（只有 `CheckFailed` 翻转）
⇒ 空实现下权威表/pending/拒绝数**应当完全相同**。我在 M2 变异树上跑同一个探针验证：

| | 原实现 | M2 空实现 |
|---|---|---|
| **passed** | **54** | **0** |
| skipped | 42 | 96 |
| `non_adjacent_prior` | 20 | 77 |

⇒ **`passed=54` 确实能区分（54 vs 0），而 79/17/0 不能。**
dev 那句「拒绝 0 是个会骗人的数」完全正确，它主动补报三态是必要的，不是冗余。

⇒ **三条证据层层递进**：真跑三数（必要不充分）→ `passed=54`（充分区分）→ M2 变异（测试侧守卫）。

---

## 4. `fix_items` 逐条验收

| # | 要求 | 证据 | 判定 |
|---|---|---|---|
| [0] | C-1 缺陷描述 | —— | 已修，见下 |
| [1]a | `prior[0]` 不相邻时不按原阈值判 | `validate.go:399-404` 跳过并记 `non_adjacent_prior:<期次>(gap=N,want=M)`；M1 证明其存在 | ✅ |
| [1]b | 订正「`[0]` 就是上一期」注释 | `validate.go:136`「**必须自己核对期次跨度**，别沿用『[0] 就是上一期』那个假设」 | ✅ |
| [1]c | `Preceding` 文档写明只返回已入权威表的期次 | `store.go` +12 行：两条射程限制（① 只看得见已入权威表、落 pending 的不存在 ② 无相邻性约束）+ 「`out[0]` 不等于上一期」+ 真跑证据 | ✅ |
| [1]d | 补跨接缝测试（造「中间期被拒」场景） | `TestStockContinuityDoesNotUseRejectedPeriodAsBaseline`：A 入权威表 → B 落 pending → C 看不见 B 而拿 A 当上一期 | ✅ |
| [2] | 真跑 75→79、21→17、拒绝 4→0 | **我独立真跑**：见 §5 | ✅ |
| [2] | 阈值不许动 | `defaultStockContinuityMax` 未在返工 diff 中 | ✅ |
| [2] | 生产库不许碰 | 真跑**前后各验一次**，见 §6 | ✅ |

> ⚠️ [1]b 的核实用**看行**而非 `grep -c`：该行命中是因为它**引用那句假设是为了否定它**，
> 计数会给出假阳（Leader 踩过一次并提醒了我）。

---

## 5. 独立真跑（探针直调 `BackfillLoad`，临时库，非生产库）

```
权威表(ToObservations) = 79     pending(ToPending) = 17
Total=218  Attempted=199  Unsupported=19  ParsedOK=161  ParseFailed=38
Merged=96  SingleArticle=54  MergedGroups=42
pending 判因 17 条：全部为 deposit_sum（drift_exceeded 10 + tolerance_exceeded 7）
                    —— 无一条 stock_continuity  ⇒ 拒绝 = 0
```

⇒ **三项全中**（79 / 17 / 0），且判因分布提供了比「拒绝数为 0」更强的证据：
我看到的是 17 条 pending 的**每一条理由**，其中没有 `stock_continuity`。

顺带印证 TASK-010 的验收值：`SingleArticle=54` / `MergedGroups=42`，与 QA 独立复算一致。

---

## 6. 生产库（`/Users/zuowei/workspace/runtime/atlas/data/hestia.db`）

| 时点 | sha256 |
|---|---|
| 真跑**前** | `478d40c079c8b0eab7d089bb6f1926725b361a6dc6c850f4c4a651406f3ec28c` |
| 真跑**后** | 同值 |

⇒ 与 DoD 要求逐字符相同，未被触碰。

> ⚠️ 取证时我差点误报：先算的是**仓库内** `data/hestia.db`（sha `2d1388fc…`，不匹配），
> 一度以为库变了。**问「我们量的是同一个文件吗」才发现生产库在 `workspace/runtime/atlas/`**，
> 路径写在 `fix_items[2]` 与 TASK-010 的 DoD 里，不在 Leader 的消息里。

---

## 7. dev 主动申报的两件事 —— 照常核，均属实

### 7.1 消融 4/5 KILLED，A4 SURVIVED（纵深分支）

**理由成立，我独立核了**：A4 是 `periodGapMonths` 的 `!ok` 分支（`time.Parse("2006-01", …)` 失败）。
而 `types.go:225` 的 `Meta.validate` 有 `if !periodRE.MatchString(m.Period)` **强制 YYYY-MM** ⇒
任何通过校验的 Observation 其 period 必然可解析 ⇒ **该分支在合法数据上不可达**，测试到不了。
是纵深防御，SURVIVED 属诚实记录，不是漏测。

dev 还申报：A4 第一次变异体因 `ok` 未使用而**不编译**，被有效性闸挡下记 SKIP，换等价形式重跑才得结论。
⇒ 与我在 TASK-006/012 踩的是同一类（改写破坏变量/import 引用），**它的闸响了**。

### 7.2 改了既有 stock 夹具（原先 prior 与本期同 period，现实中不可能）

**核实：改的是夹具调用行，没有删任何测试**（wisdom 18 的固定动作）：

```
返工前 37 个测试函数 → 现在 39 个
消失的: （无）✅        新增的: SkipsNonAdjacentPrior, DoesNotUseRejectedPeriodAsBaseline
validate_test.go 的 6 行删除：全部是夹具调用行的修改（stockObs(400) → 带 period 的形式），
                              分布在 SkipReasons / SkipsOnZeroDenominator / SkipsWhenPeriodTypeHasNoThreshold 三个既有测试里
```

**`TestStockContinuityDetectsJump` 与 `atBoundary` 精确相等格：`git diff` 确认一行未动** ✅
（Leader 点名要核的那条。）

---

## 8. 门禁（采于 `188eb212…`）

| 项 | 实测 | 判据 | 判定 |
|---|---|---|---|
| `go test ./internal/hestia/... -cover` | ok, **96.1%**（`go test -cover` 口径，尺B） | ≥ 95.9% | ✅ |
| `go test ./cmd/...` | ok | — | ✅ |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出 | 零输出 | ✅ |
| `gofmt -l internal/hestia cmd/atlas` | 恰两个既有欠账 | 之外无新增 | ✅ |
| go.mod / go.sum | **0 行** | 不得出现 | ✅ |
| 改动范围 vs `writes` | **改了未声明 = 空**（零越界） | — | ✅ |

返工改动量：`store.go +12/-0`、`validate.go +60/-3`、`validate_test.go +140/-6`。
（`writes` 里的 `required.go` / `types.go` 本轮未改——它们是第一轮改的，`writes` 覆盖两轮全部改动。）

`stock_continuity` 族测试 **11 条顶层全 PASS**（含新增 2 条）。

---

## 9. ⚠️ 与本任务判定无关的一处**文件结构异常**（需 Leader 处置）

`TASK-011.json` 的 `done_criteria` 里有一个**非法 key `writes`**：

```
done_criteria 的 key: boundary / error_handling / functional / non_functional / writes
                                                                                ^^^^^^
done_criteria.writes = [types.go, validate.go, validate_test.go, required.go]   ← 4 项
顶层     .writes     = [types.go, validate.go, validate_test.go, required.go, store.go]  ← 5 项
```

- 它是顶层 `writes` 的一份**过期副本**（缺返工时补进的 `store.go`）
- **12 个任务里只有 TASK-011 有这个 key**（我扫了全部）
- 疑似 `03:05:45Z` 那次 `update --json-field done_criteria=…` 整体替换时误并入
- 危害：① `done_criteria` 多了一个不是完成标准的条目 ② **同一事实的两个副本，改一处不会让另一处变红**
  —— 正是 dev 在 `store_test.go` 注释里记的那条教训

⇒ 不影响本次判定（`fix_items` 才是返工的验收依据），但建议 Leader 清掉。

---

## 10. 复现命令（锚钉全 sha）

```bash
git worktree add --detach <dir> 188eb212009a0bd7f210059ddd15a4c3b759e96f

# 那道闸 + 空实现检测（结论须解析到 --- FAIL: X/Y 子测试级）
#   M1 删掉 validate.go:399-404 的相邻性判据整段 ⇒ 应红 4 格 adjacent=false
#   M2 该判据条件前加 `true ||`（一律 skip）    ⇒ 应红 4 格 adjacent=true
# 三态复核：BackfillLoad 灌临时库 → 重开 store 作 History → 对 res.Groups 逐组 Validate
#   原实现 passed=54 / M2 空实现 passed=0   ← 这一对才证明 54 这个数有区分力
# 生产库：shasum -a 256 /Users/zuowei/workspace/runtime/atlas/data/hestia.db  （真跑前后各一次）
```

---

## 11. 结论

**VERIFIED。** `fix_items` 三条全部满足，裁决 7 那道闸通过。

C-1 的修复不是空实现，有**三层独立证据**：真跑 79/17/0（必要不充分）、`passed=54`（充分区分，
我实测 M2 空实现下为 0）、M1/M2 对称变异（8 格分两组各守一个方向）。dev 主动补报三态分布
是必要的——**真跑那三个数在空实现下完全相同**，这一点我用变异实测证实。

生产库真跑前后 sha256 逐字符不变；`DetectsJump` 边界格一行未动；测试函数消失 0 个。
§9 的 `done_criteria.writes` 是文件结构问题，与判定无关，建议清理。
