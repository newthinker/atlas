# TASK-004 验证报告 — Store.HasPeriod 与 PeriodChecker 接口

- **验证者**：test-agent-25（Reality Checker，默认判定 NEEDS WORK）
- **判定对象**：`verify_baseline.head = 2b93ccd3916ec1cb52111da28cd2514dc9d73c37`（== 当前 HEAD）
- **验证 worktree**：`git worktree add --detach /tmp/verify-036-4 2b93ccd3916ec1cb52111da28cd2514dc9d73c37`
- **结论：VERIFIED（8/8 DoD 全部 PASS）**，另报 1 条不影响 DoD 的缺陷（§7）

## 0. 承接与漂移

⚠️ **派验通知未送达。** 本任务是 `TeammateIdle` hook 唤醒 + 重扫循环发现的
（`verifier == test-agent-25 ∧ status == verifying`）。这正是 CLAUDE.md「即使通知丢失，
各角色通过轮询自己负责的状态也能发现待办」所设计的自愈路径，本次是它第二次实际生效。

**双零漂移**：
```
git rev-parse HEAD                      2b93ccd3916ec1cb52111da28cd2514dc9d73c37
verify_baseline.head                    2b93ccd3916ec1cb52111da28cd2514dc9d73c37
shasum -a 256 discoveries/TASK-004.json ec3c941775b7ccf8a771bc38f314da186a0b8174b7b8e15a04d223a688719d9c
verify_baseline.discovery_sha256        ec3c941775b7ccf8a771bc38f314da186a0b8174b7b8e15a04d223a688719d9c
```
⇒ **未使用任何 `--ack-*`**。

**discovery 指针核对**（核内容非文件名）：`.task == "TASK-004"`、
`.commit == 2b93ccd3…` == `verify_baseline.head`、`.by == "dev-agent-49"`。
`git show --numstat 2b93ccd` → `discover.go 14/0`、`store.go 36/0`、`store_test.go 131/4`，
与 `writes` **三项逐项一致，无越界**。

## 1. DoD 逐条覆盖矩阵

| # | DoD 条目 | 对应测试 | 承重证据 | 判定 |
|---|---|---|---|---|
| F1 | 已入库 `true` / 未入库 `false` | `TestHasPeriod` 两子测试 | M7（`ErrNoRows` 返回 true）、M8（恒返回 true）KILLED | **PASS** |
| F2 | **修订后仍然命中** | `TestHasPeriod/修订后仍然命中` | 见 §3.1 —— 我构造的探针**只被它杀** | **PASS** |
| B1 | `period_type` 隔离 | `TestHasPeriod/period_type 不同不算命中` | M6（`WHERE` 去掉 `period_type`）**只被它杀** | **PASS** |
| B2 | **pending 不算已入库**（须先断言落在 `TablePending`） | `TestHasPeriodIgnoresPending` | M1（UNION 上 pending）**只被它杀**；前置断言在场 | **PASS** |
| E1 | 查库失败返 error、`%w` 包住、带 period/periodType；用 `NotNil(Unwrap)` 而非 `NotErrorIs` | `TestHasPeriodWrapsQueryError` | M3、M4、M5 KILLED；写法逐字核实（§4） | **PASS** |
| N1 | 编译期断言 `var _ PeriodChecker = (*Store)(nil)`；接口定义在消费方；注释写明 `article_id` 理由 | — | 见 §3.2 —— 改签名后**编译错误直接点名该行** | **PASS** |
| N2 | **两条守卫都登记**、精确相等、在 TASK-001 基础上**追加不覆盖** | 两条导出面守卫 | **M9a 与 M9b 都 KILLED**（§3.3） | **PASS** |
| N3 | RED / gofmt / vet / 整包绿 / 覆盖率 ≥ 92.1% | §2 | — | **PASS** |

## 2. N3 的命令与输出

```
$ GOTOOLCHAIN=local go vet ./internal/hestia/   → 无输出，exit 0
$ gofmt -l internal/hestia/                     → 无输出，exit 0
$ GOTOOLCHAIN=local go build ./...              → 无输出，exit 0
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  github.com/newthinker/atlas/internal/hestia  0.792s  coverage: 92.8% of statements
$ go tool cover -func | grep -E 'HasPeriod|total'
store.go:237:  HasPeriod   100.0%
total:         (statements) 92.7%
```

**覆盖率口径说明**（沿用本 Sprint 的 F39 纪律）：`go test -cover` 读 **92.8%**、
`go tool cover -func` 读 **92.7%**，两者是**同一棵树、同一口径**，0.1 的差是真值落在
~92.75% 上的舍入产物（TASK-001 出现过同形态）。两个读数都 ≥ DoD 下限 92.1%。

**RED 独立复现**：把 `store.go`/`discover.go` 回退到 TASK-003 版（`7576ad3`）、测试保持交付版：
```
internal/hestia/store_test.go:1731:17: s.HasPeriod undefined (type *Store has no field or method HasPeriod)
（及 4 处同形）
```
因**预期原因**失败，未被 `imported and not used` 污染。

## 3. 变异/消融独立复验（harness 自写）

`scratchpad/test25-TASK-004-ablation.sh`，锚点 `ARCFORGE_MUT_REF` 可覆写、默认**全 sha**；
变异在隔离 worktree。四道闸：基线闸（`--- PASS` = 547 全绿）、生效闸、**编译失败闸**、
**计数自证 10 == 10 → OK**。加 M1/M2 重跑与两个探针，共 **13 条，12 KILLED / 1 SURVIVED**。

| 变异 | 结果 | 唯一/主要杀手 |
|---|---|---|
| M1 `HasPeriod` 也认 pending（UNION 上 `TablePending`） | KILLED | **只被** `TestHasPeriodIgnoresPending` 杀 |
| **M2** 查 `TableObservations` 而非 `viewCurrent` | **SURVIVED** | 见 §3.4 —— 已验证为**真等价变异** |
| M3 出错时返回 `(true, err)` | KILLED | `assert.False`：「出错时不得报告『已入库』」 |
| M4 `%w` 改 `%v` | KILLED | `ErrorIs` + `require.NotNil(errors.Unwrap(err))` |
| M5 错误信息去掉 period/periodType | KILLED | `does not contain "2025-12"` / `"monthly"` |
| M6 `WHERE` 去掉 `period_type` 条件 | KILLED | **只被** `period_type 不同不算命中` 杀 |
| M7 `ErrNoRows` 分支返回 `true` | KILLED | `未入库返回 false` 等三处 |
| M8 恒返回 `true`（不查库） | KILLED | 四处 |
| **M9a** reflect 版期望列表删掉 `HasPeriod` | **KILLED** | `TestStoreExposesNoWriteMethods` |
| **M9b** AST 版期望列表删掉 `Store.HasPeriod` | **KILLED** | `TestPackageExposesNoWriteFunctions` |
| 探针① 改 `HasPeriod` 签名 | 编译错误**点名断言行** | §3.2 |
| 探针② 权威表 + `COUNT(*)==1` | KILLED | **只被** `修订后仍然命中` 杀（§3.1） |
| 探针③ 删掉 `var _ PeriodChecker` 那一行 | SURVIVED（**预期**） | 它是编译期守卫，删掉守卫本身当然不红 |

首轮 M1/M2 被**编译失败闸**拦为 `INVALID`（我猜错了常量名 `tablePending`/`tableObservations`，
实际是导出的 `TablePending`/`TableObservations`）——**闸正确地没有把它们记成 SURVIVED**，
改用正确常量后重跑。

### 3.1 F2「修订后仍然命中」不是 F1 的冗余 —— 我构造了它唯一排除的实现

初看这条像是被 F1 蕴含（视图下修订仍只有一行）。我按判据
「**有没有一个我想排除的实现，能让 F1 绿而 F2 红？**」构造了探针②：

```go
"SELECT COUNT(*) FROM "+TableObservations+" WHERE period = ? AND period_type = ?"
return one == 1, nil
```
这是一个完全合理的写法（数表里的行数、要求恰好一行）。实测**唯一失败测试**：
```
--- FAIL: TestHasPeriod|--- FAIL: TestHasPeriod/修订后仍然命中
```
⇒ 该子测试排除的是「按原始表计数、把修订产生的第二行当成异常」这一类实现，**承重**。

### 3.2 N1 编译期断言确实承重 —— 错误直接点名那一行

把 `HasPeriod` 的签名改掉（去掉 `periodType` 参数）后：
```
internal/hestia/store_test.go:1829:23: cannot use (*Store)(nil) (value of type *Store)
  as PeriodChecker value in variable declaration: *Store does not implement PeriodChecker
  (wrong type for method HasPeriod)
```
**报错位置正是 `var _ PeriodChecker = (*Store)(nil)` 那一行** ⇒ 签名漂移在编译期就红，
不是靠调用点偶然拦下的。DoD `non_functional[0]` 满足。
接口确实定义在**消费方** `discover.go`（不是 store 侧），注释写明了
「判停用期次而不用 `article_id`」的理由（M0 实测 id `2025092212550713215`、
央行 2026-06-26 批量重建站点）。

### 3.3 N2 两条守卫 —— **M9 同形跑了两遍，各自 KILLED**（Leader 点名的要求）

| 守卫 | M9 同形变异 | 结果 |
|---|---|---|
| `:361` reflect 版 | 期望列表删掉 `"HasPeriod"` | **KILLED** |
| `:412` AST 版 | 期望列表删掉 `"Store.HasPeriod"` | **KILLED** |

⇒ 两条守卫都在按精确集合相等工作，且都确实盯着本次的登记项。

**逐字核对「追加而非放宽 / 追加而非覆盖」**（这一条按 DoD 明写必须靠人核对，不靠 M11）：
```
断言类型=Equal  项数=5   "Close", "DB", "HasPeriod", "Preceding", "Save"
断言类型=Equal  项数=10  "DefaultThresholds", "NewPBOCFetcher", "NewStore", "Parse",
                         "Store.Close", "Store.DB", "Store.HasPeriod", "Store.Preceding",
                         "Store.Save", "Validate"
assert.Subset / assert.Contains(t, got, …) 出现次数: 0
```
- 两条仍是 `assert.Equal` **全集合精确相等**，未改成 `Subset`/`Contains`
- AST 版**保留了 TASK-001 的 `NewPBOCFetcher`**（九项→十项），是**追加不是覆盖**
- 字典序正确（`Close < DB < HasPeriod < Preceding < Save`；`Store.DB < Store.HasPeriod < Store.Preceding`）
- 两条各补了说明段，并互相点名对方（「它是 `*Store` 方法所以同时打红两条」）

**Leader 采纳我上一轮的纠正是对的**：M11 同形（把 `Equal` 换成 `Subset`）**一定不红**，
跑它证明不了本次登记的正确性；有效的正向验证是 M9 同形，而本任务因为是 `*Store` 方法
需要跑两遍。本报告按此执行。

### 3.4 M2 SURVIVED —— dev 如实申报，我独立验证其「不可区分」论证成立

dev 在 discovery 与代码注释里主动写明：承重的是「**不查 pending**」，**不是**「查视图而不查权威表」，
并称后者是**不可区分**而非无人守。我没有采信，而是去看了视图定义：

```go
// currentViewDDL → bitemporal.CurrentQuery(spec)
"SELECT * FROM %s o WHERE o.published_at = (SELECT MAX(published_at) FROM %s WHERE <correlate>)"
// correlate: period = o.period AND period_type = o.period_type
// NewSpec(TableObservations, []string{"period", "period_type"}, "published_at")
```
⇒ 视图**只在业务键内部筛行**（每个 `(period, period_type)` 留 `published_at` 最大的），
**没有任何会整个抹掉一个业务键的过滤条件**；而 `HasPeriod` 的存在性判定用的正是同一组键。
**两者对本查询恒等价** ⇒ M2 是**真等价变异**，SURVIVED 不构成缺口。

#### 我追了唯一一个能推翻它的反例，被 schema 封闭了

上面的论证有一个隐含前提：「每个业务键在视图里**至少留下一行**」。它**可以不成立** ——
SQL 里 `MAX(col)` 在该键下所有行的 `col` **全为 NULL** 时返回 NULL，而 `o.published_at = NULL`
恒假 ⇒ **那个业务键会整个从视图消失**，此时 `viewCurrent` 与 `TableObservations` 对存在性
判定就**不再等价**，M2 也就从「等价变异」变成「真缺口」。

我去查了 DDL：

```go
for _, c := range metaColumns {            // period, period_type, published_at, …
    b.WriteString("  " + c + " TEXT NOT NULL,\n")
}
b.WriteString("  PRIMARY KEY (period, period_type, published_at)\n)")
```

`published_at` 是 **`TEXT NOT NULL`，且是主键的第三列** ⇒ NULL 在 schema 层面不可能出现
（两道各自独立的约束）。⇒ **唯一能推翻「恒等价」的情形被排除，结论封闭。**

**为什么专门追这一步**：本项目的判据是「证据刚好够才是危险区」。
「视图只按业务键筛行」听起来足以支持恒等价，我停在那里也不会有人来查；
但它只是**论证合理**，不是**穷尽了反例**。差别就在这个 NULL 边界上——
而它恰好是那种「读代码看不出、只有去查 DDL 才知道」的前提。

**这条值得记**：dev 对一个存活变异给出了「为什么它无法被区分」的机制解释，而不是
默认补一条断言把它「杀掉」——**为等价变异补断言只会制造一条钉住实现细节的假守卫**。
它的解释经独立复核成立，且我补的 NULL 边界检查把它从「合理」推到了「封闭」。

## 4. E1 的写法逐字核实（DoD 明确点名的 F8 形态）

DoD 要求断言「包住了」必须用 `require.NotNil(t, errors.Unwrap(err))`，**不得**用
`NotErrorIs(errors.Unwrap(err), err)`（后者平凡为真）。核实 TASK-004 新增段（1720 行之后）：

```
1808| // require.NotErrorIs(t, errors.Unwrap(err), err) 在不包裹时 Unwrap 返回 nil、   ← 注释里的反面教材
1810| // 这里用 require.NotNil(t, errors.Unwrap(err))，它在不包裹时会真的红。
1825| require.NotNil(t, errors.Unwrap(err), "要的是「包住」：Unwrap 后必须还剩底层 err")  ← 实际写法
```
⇒ **本任务新增代码里零处** `NotErrorIs`。全文唯一那处在 **`:1717`**，是
`TestPrecedingWrapsQueryError` 的**存量**代码（Sprint 035 的 F8），dev 在注释里明确标注
「邻居那处存量写法属于 TASK-007 的清理范围，不在本任务 scope 内」——与我在 TASK-001 报告里
的观察 2 及 Leader 的裁定（并进 T7）一致。

`TestHasPeriodIgnoresPending` 的前置条件断言也在场（DoD B2 明写要求）：
```go
require.Equal(t, TablePending, out.Table, "前置条件：这一期必须落在 pending")
```
没有它，`failing()` 若哪天不再落 pending，那条测试会以「pending 不算已入库」的名义平凡通过。

## 5. 结论

**VERIFIED。** 8 条 done_criteria 逐条有对应测试、逐条有消融证据；13 条变异 12 KILLED，
唯一 SURVIVED 的一条经独立验证为真等价变异；Leader 点名的 **M9 同形两遍**均 KILLED；
编译期断言经改签名验证承重；F2 由我构造的探针证明非冗余；RED 独立复现；双零漂移；主工作区零污染。

## 6. 主工作区完整性

变异窗口内 + 收尾双重核实，三文件 sha256 与 `git status --porcelain` 前后逐字相同：
`store.go a3003852…`、`store_test.go b897a916…`、`discover.go 3a773773…`。
变异树收尾 sha256 一致；`/tmp/mut-036-4`、`/tmp/mut-036-4b`、`/tmp/verify-036-4` 均已 remove + prune。

## 7. 🔴 缺陷：`HasPeriod` 的文档注释脱离函数，`go doc` 输出为空

```
$ go doc ./internal/hestia Store.HasPeriod
func (s *Store) HasPeriod(ctx context.Context, period, periodType string) (bool, error)
        ← 仅签名，20+ 行理由一个字都没有
```

成因是 `store.go:236` 是一个**空行**，把注释块与 `func` 隔开了：
```
234| // 换成权威表后整包无一变红，且非「无人守」而是**不可区分**）。选视图是为了与
235| // Preceding 同源，也为了将来视图若加上过滤条件时这里能自动跟随。
236| ''                                                       ← 空行
237| func (s *Store) HasPeriod(ctx context.Context, ...
```

**它是 `store.go` 里唯一有此问题的方法**（我逐个扫过：`NewStore` / `DB` / `Preceding` / `Save`
的注释全部与函数相连）：
```
  49  func NewStore(...)              ✅ 相连
 214  func (s *Store) DB(...)         ✅ 相连
 237  func (s *Store) HasPeriod(...)  🔴 空行隔开
 258  func (s *Store) Preceding(...)  ✅ 相连
 332  func (s *Store) Save(...)       ✅ 相连
```

- **不违反任何 DoD 条目**：`non_functional[0]` 要求写明理由的是 **`PeriodChecker` 接口**的注释
  （在 `discover.go`，**已正确附着**），不是 `HasPeriod` 方法注释。故判定仍为 PASS。
- **`gofmt` 与 `go vet` 都不报**——这正是它能溜过所有自动门禁的原因。
- **为什么值得修**：那 20 多行恰好是本项目最看重的「为什么」内容（pending 不算已入库的理由、
  与 `CONTRACTS.md` 同一张表两面的辨析、「别把理由写强」的警告）。它们现在只有读源码的人看得到。

⚠️ **无法零成本并入后续任务**：`TASK-005` 的 `writes` 是
`discover.go`/`discover_test.go`/`store_test.go`，`TASK-007` 是
`config.go`/`config_test.go`/`thresholds.go`/`store_test.go`/`CONTRACTS.md`
—— **两者都不含 `store.go`**。这与 TASK-003 那条次缺口（T5 本就写 `discover_test.go`）情况不同，
处置需 Leader 决定：把 `store.go` 加进某个后续任务的 `writes`，或单开一条清理任务，
或接受它作为已记录的遗留。
