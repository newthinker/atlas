# TASK-003 验证报告 —— History 窄接口与 Store.Preceding

- **验证者**：test-agent-24（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：`master @ c693177fc1e14b1bf8d07e7ccffaf72b8a157881`（全 sha）
- **verify_baseline.head**：`c693177fc1e14b1bf8d07e7ccffaf72b8a157881`
- **结论**：**VERIFIED（8/8 PASS）**，附 **4 项发现**（均不构成 DoD 失败，但其中 2 项会传给下游）

---

## 0. 漂移核实与 scope

```
$ git rev-parse HEAD                     → c693177fc1e14b1bf8d07e7ccffaf72b8a157881
$ shasum -a 256 .arcforge/discoveries/TASK-003.json
5ec1fce227c3172187f2d6e12922d83ef2fd1f2e19b06fbd167d4d738d5c97ee
$ jq -r .verify_baseline.discovery_sha256 .arcforge/tasks/TASK-003.json
5ec1fce227c3172187f2d6e12922d83ef2fd1f2e19b06fbd167d4d738d5c97ee
```

HEAD 与 discovery sha256 均与基线**逐字相同** ⇒ 零漂移，不需要 `--ack-drift`。

```
$ git show --numstat --oneline c693177
70	0	internal/hestia/store.go
168	4	internal/hestia/store_test.go
31	0	internal/hestia/validate.go
```

实改 3 文件 == `writes` 三项，无越界；与 discovery 自报 `+70/-0`、`+168/-4`、`+31/-0` 一致。
（`M .arcforge/write-matrix.json` 是 Leader 登记 token 造成的，已按其说明排除，非 dev 越界。）

## 1. 门禁（隔离 worktree `/tmp/verify-003 @c693177…`）

```
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  	github.com/newthinker/atlas/internal/hestia	0.763s	coverage: 90.0% of statements
$ gofmt -l internal/hestia/   → 无输出
$ go vet ./internal/hestia/   → 无输出
$ go test -run TestPreceding -v → 七条全 PASS
```

逐函数：`Preceding 88.9%` / `scanObservation 92.3%` / `noHistory.Preceding 0.0%`（见发现 4）。

---

## 2. 消融方法学（Leader 事项③：编译失败闸）

harness `/tmp/mh3.sh` 带**四道闸**，全部作用于一次性 worktree `/tmp/mut-003`：

1. **sha 生效闸** —— 变异体落盘后 sha 必须变化，否则判「未生效、作废」
2. **语法闸** —— `gofmt -l`
3. **编译闸** —— `go test -c -o /dev/null ./internal/hestia/`；**有任何输出即判假 KILLED、作废**
4. **还原闸** —— 收尾 sha256 必须回到变异前

**对照组先验**：未变异副本 `go test` 全绿，否则一切结论作废。

> ⚠ **编译闸实际拦下了 2 次假 KILLED**，都不是靠人眼：
> - **B15 首版**：正则误匹配到 `Save` 里的另一处 `rows.Err()` ⇒ `undefined: out` / `too many return values`
> - **B13 首版**：改签名时删掉 `periodType` 参数 ⇒ 函数体 `undefined: periodType` 四处，
>   **把本该出现的「接口不满足」错误盖住了**
>
> 这是本 Sprint 第三、四次出现同一形态（TASK-001 的裸 `true`、TASK-003 的 `tableObservations` 小写在前）。

---

## 3. done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 | 消融证据（致红断言行号 + 内容） | 判定 |
|---|---|---|---|---|
| functional[0] | period 之前最近 n 期、降序、不含自身 | `TestPrecedingReturnsRecentPeriodsInDescendingOrder` | **B6** `period <`→`<=` ⇒ `:1562 最近的一期排最前` + `:1563`；**B7** `DESC`→`ASC` ⇒ `:1562`；**B8** 去掉 `LIMIT ?` ⇒ `:1561 should have 2 item(s), but has 3 / LIMIT 必须生效，且不含 period 自身`。三个子要求各有独立杀手 | PASS |
| functional[1] | 走视图 ⇒ 修订可见、**只返回一条** | `TestPrecedingSeesRevisions` | **B11** `viewCurrent`→`TableObservations` ⇒ `:1625 has 2 / 修订不产生第二期`；**B12**（见 §4.2）构造「只返 1 条但取到修订**前**的旧值」⇒ `:1626 必须取修订后的值`。⇒ 「只一条」与「取修订后的值」**各自独立吃重**，后者不是前者的附庸 | PASS |
| functional[2] | 编译期断言 `var _ History = (*Store)(nil)` | `store_test.go:1679` | **B13** `n int`→`n int64`（刻意让函数体仍合法）：`go build ./internal/hestia/` **无输出**（实现侧零错误）⇒ 唯一错因是 `store_test.go:1679: *Store does not implement History (wrong type for method Preceding) / have …int64… want …int…` | PASS |
| boundary[0] | absent 语义**双向** | `TestPrecedingRestoresAbsenceNotZero` | **B4** `vals[i].Valid`→恒真（等价裸 float64 扫描）⇒ 只红 `:1582 未写入的字段读回来不该出现在 Values 里`（方向①）；**B5** `Valid && Float64 != 0` ⇒ 只红 `:1585 显式写入的 0 必须保留`（方向②，DoD 点名更危险的那个）。**两条互不掩盖** | PASS |
| boundary[1] | period_type 序列隔离 | `TestPrecedingIsolatesPeriodType` | **B10** `period_type = ?`→`? IS NOT NULL` ⇒ `:1605 has 2 / annual 那条不该混进 monthly 序列` | PASS |
| boundary[2] | ①n=0/-1 返空 ②空历史非 error | `TestPrecedingRejectsNonPositiveN` / `TestPrecedingOnEmptyHistory` | **B9** 去掉 `if n <= 0` ⇒ `:1644 n=-1 应返回空而不是全部`；**B15'** 空结果改返 error ⇒ `:1632 首期入库是正常路径，不是错误`（见发现 3：n=0 那半不由此杀死） | PASS |
| error_handling[0] | 返 error + **包住底层 err**（`errors.Is` 可找到）+ 带 period/periodType | `TestPrecedingWrapsQueryError`（**dev 自补，计划未覆盖**） | **B1** 完全不包裹（`return nil, err`）⇒ `:1669`+`:1670`（缺 period/periodType 上下文）+ `:1675 应是包裹后的错误而不是裸 sentinel`；**B2** 三处 `%w`→`%v` ⇒ `:1668 Target error should be in err chain`；**B3** 去掉 `%s/%s` 上下文（保留 `%w`）⇒ `:1669`+`:1670`。三种破坏方式各被不同断言拦下 | PASS（附发现 1、2） |
| non_functional[0] | RED 因预期原因失败 / gofmt·vet / 整包绿 / **两条导出面守卫均登记且保持精确相等** | — | RED 独立复现见 §5；**B14** 伪造 `func (s *Store) Peek() {}` ⇒ `:361`（reflect 版）与 `:406`（AST 版）**同时**红，两条都仍是 `assert.Equal` 精确相等，`DefaultThresholds` 原样保留 | PASS |

---

## 4. Leader 点名的四件事，逐条回复

### 4.1 事项① —— 自补测试的两个实现选择：**只有一个成立**

**选择 A「用已取消的 context 而非关掉的库」——成立。**
`context.Canceled` 是标准 sentinel，B1 与 B2 的致红都由它驱动（`ErrorIs` / `ErrorContains`）。
若改用关库，只能比对未导出 `errDBClosed` 的错误串，DoD 说的「可被 `errors.Is` 找到」就测不出来。

**选择 B「只断言 `errors.Is` 不足以证明包住了，故补两条」——只有一条成立。**

Leader 要求「把实现改成不包裹的 `return nil, err`，看是否真有断言转红、且是被哪一条杀的」。
**我先声明了证伪条件再跑**（预测：`NotErrorIs` 那条平凡为真，真正杀死的是 `NotEqual` 与 `ErrorContains`）。
B1 实际输出：

```
store_test.go:1669  Error "context canceled" does not contain "2026-01"   （错误信息要带 period）
store_test.go:1670  Error "context canceled" does not contain "monthly"   （错误信息要带 periodType）
store_test.go:1675  Should not be: &errors.errorString{s:"context canceled"}（应是包裹后的错误而不是裸 sentinel）
```

**`:1674` 的 `require.NotErrorIs(t, errors.Unwrap(err), err)` 没有出现。**
它是 `require`，若失败会中止测试、`:1675` 就不会执行 —— `:1675` 执行了，说明 `:1674` 确实通过了。
成因：不包裹时 `err` 就是裸 sentinel，`errors.Unwrap(err)` = `nil`，而 `errors.Is(nil, x)` 恒为 `false`
⇒ 该断言在**它本该抓住的那个场景里平凡为真**。B2、B3 两次消融中它同样一次都没红。

**决定性实验 Y**（同一 B1 变异下，只把 `:1674` 换成它注释所声称的本意）：

```go
require.NotNil(t, errors.Unwrap(err), "【本意断言】Unwrap 后应还剩内层")
```
```
store_test.go:1674  Expected value not to be nil.   （【本意断言】Unwrap 后应还剩内层）  ← 立即致红
```

⇒ **本意的断言有杀伤力，写下的那条没有。**

**这不构成 DoD 失败**：DoD error_handling[0] 的三个实质要求（返 error / 包住可被 `errors.Is` 找到 /
带 period·periodType）分别由 `:1666`、`:1668`+`:1675`、`:1669`+`:1670` 守住，B1/B2/B3 全部 KILLED。
**但 discovery `key_findings[3]` 的表述失实，且下游会读它**：

- 「补两条：`errors.Unwrap(err)` 后仍有内层」—— 写下的代码并不断言这件事；
- 「A6 消融（`%w`→`%v`）确认**这组**断言确实在守卫」—— A6 是被 `:1668` 的 `assert.ErrorIs` 杀的，
  与那两条补充断言无关。

典型的「结论对、理由错」：结论（该测试守住了 DoD）成立，但被引用的理由不成立。
**建议 Leader 把这一条转达给 TASK-004/005/006 的 dev**，避免把
`require.NotErrorIs(t, errors.Unwrap(err), err)` 当成「证明包住了」的可复用范式照抄。
正确写法是 `require.NotNil(t, errors.Unwrap(err))`（或 `require.Error(t, errors.Unwrap(err))`）。

### 4.2 事项② —— absent 双向：**两条各自独立，方向②确有守卫**

B4 只打红方向①（`:1582`），B5 只打红方向②（`:1585`），互不掩盖 ⇒ 不是「一条断言顺带覆盖两件事」。
方向②正是 DoD 点名最危险的那个（NULL 读成 0 ⇒ `stock_continuity` 得出 -100% 假警报）。

顺带把 functional[1] 也做成了同样的强度。原测试是 `require.Len(got,1)` + `assert.Equal(305.0,…)`，
前者会先中止，后者是否吃重不显然。故构造 **B12**：改查裸表并按 `MIN(ingested_at)` 分组取每期最早一行 ——
**恰好返回 1 条，但是修订前的 300**。结果 `:1626 必须取修订后的值` 致红（`:1625` 的 Len 通过）。
⇒ 「只一条」与「取修订后的值」两半都有独立杀手。

### 4.3 事项③ —— 编译失败闸：**我的 harness 带了，并且真的用上了**

见 §2。两次拦截都发生在我自己的变异上，且两次若无闸都会被记成 KILLED。
特别是 B13：改签名时函数体里的 `undefined: periodType` 会**盖住**本该出现的
「`*Store` 不满足 `History`」——这正是本条 DoD 想验证的东西，闸不拦就会拿错误理由记 PASS。
重做时改成 `n int` → `n int64`（函数体保持合法），并**先跑 `go build`（无输出）证明实现侧零错误**，
才让 `:1679` 的接口不满足成为唯一错因。

### 4.4 事项④ —— 三条语义声称：**全部核实为真**

自写探针 `TestProbeSemanticClaims`（跑完即删，`git status` 已核实无残留），**全 PASS**：

| 声称 | 探针断言 | 结果 |
|---|---|---|
| `Preceding` 的 `Values` 只含非 NULL 字段，缺失是「键不存在」不是 0 | `_, ok := Values[FieldTSFStock]` 为 false；`Len(Values)==1`（只写了 1 个字段） | 真 |
| `saveMonthly` 用 monthly 而非 h1、内部已用 `passing()` | 存回的 `Meta.PeriodType == "monthly"`；查 `h1` 序列为空（反向确认）；`TablePending` 行数为 0（确实过闸，未被分流） | 真 |
| `NoHistory` 支持 `Validate` 据它拒绝 nil（TASK-004） | `NoHistory != nil`（`var NoHistory History = noHistory{}`，结构体值非 nil）；未赋值的 `var h History` 为 nil ⇒ **两者可区分**；`NoHistory.Preceding` 返回 `(empty, nil)` | 真 |

---

## 5. RED 因果的独立复现（non_functional[0]）

不采信 discovery 原文，自己复现失败**类型**：把 `Preceding` 方法整体从 `store.go` 删除后跑整包 —

```
internal/hestia/store_test.go:1559:16: s.Preceding undefined (type *Store has no field or method Preceding)
... （共 7 处调用点，形态一致）
FAIL	github.com/newthinker/atlas/internal/hestia [build failed]
```

与计划预期 `s.Preceding undefined` 及 discovery 记录的原文形态一致，**无 `imported and not used` 干扰**。
另核对：`validate.go` 只 import `context`（唯一用到的包）、`store.go` 未新增 import、
`store_test.go` 新增的 `errors` 确实被 `:1674` 使用（删掉该行会立刻 `"errors" imported and not used`
——我的实验 Z 撞到过，被编译闸拦下）。

---

## 6. 四项发现（均不构成 DoD 失败，但请 QA/Leader 收）

1. **`store_test.go:1674` 不具判别力** —— 见 §4.1。**会传给下游**，建议转达。
2. **三处 `%w` 只有一处被守住（消融 B16 存活）** —— 只把**未被覆盖的** `store.go:244`、`:249`
   两处 `%w` 改成 `%v`，保留已覆盖的 `:236` 不动，整包**仍然全绿**（`ok`）：
   ```
   $ go test ./internal/hestia/ -count=1
   ok  	github.com/newthinker/atlas/internal/hestia	0.683s
   ```
   覆盖率数据佐证缺口位置：`Preceding 88.9%`，未覆盖语句正是 `store.go:243.17,245.4`（scan 错误包裹）
   与 `248.35,250.3`（`rows.Err()` 包裹）。
   **实现本身三处都写对了**（我逐行读过，B16 是我人为破坏的），故 DoD 满足；
   但「查库失败必须包住底层 err」这条守卫只覆盖 `QueryContext` 那一处。
   discovery 自己写着「三处 %w 的包裹都写着同一个前缀，漏掉任何一处都会让运维拿到一条不知道
   是哪个期次的错」——**那个担心是对的，而它只被守住了三分之一**。
3. **DoD boundary[2] 自带的理由对 `n=0` 是错的** —— DoD 写「一次 `n=0` 的调用会把整个序列拉回来」。
   实测：B9 去掉 `if n <= 0` 守卫后，**`n=0` 仍返回空**（SQLite `LIMIT 0` 就是零行），
   只有 `n=-1` 会拉回全部（`:1644` 的报错原文就是「n=-1 应返回空而不是全部」）。
   **要求本身（返空）满足**，仅括注的成因写反；测试对两个值都断言了，无实际风险。
4. **`noHistory.Preceding` 覆盖率 0.0%** —— 已交付的测试无一调用 `NoHistory`。
   我的探针调用后行为正确（返回空、无 error）。非 DoD 要求，但 TASK-004 若依赖 `NoHistory`
   的行为，建议届时补一条。

## 7. 未据以判不通过的项

- validator 的 3 条 `TASK-003: scope-writes-outside-packages`（AD-035-4 已知形状级假阳），
  validator 整体 `exit=0`、`✓ 任务图校验通过（7 个任务）`。

## 8. 主工作区完整性

全部消融只在一次性 worktree（`/tmp/verify-003`、`/tmp/mut-003`）内进行。收尾核实：

```
$ shasum -a 256 internal/hestia/store.go internal/hestia/store_test.go internal/hestia/validate.go
69b87d3f20efa9bab45120ca4f72f1462df42a82ac13956c54b8dc5905ed34d3  store.go
0168867496e0266b2e38f9cd7b37d40cda58ed90302b41cdb0ddd03803eacb54  store_test.go
3a7201fe77cda1ee6f767e03b17178d741a6a68af7aec72a175abacc083e0853  validate.go
$ git status --porcelain   → 仅 .arcforge/ 条目（含 Leader 登记 token 的 write-matrix.json）
```

三个文件 sha256 与开工时逐字节一致，`internal/` 一个字节未动。

---

## 结论：**VERIFIED**

8 条 done_criteria 全部有对应测试、全部有消融证明断言在守卫、全部核对了致红归因，
16 个变异中 15 个 KILLED（因果均为断言）、1 个**刻意构造的存活体**（B16）用于暴露守卫缺口，
另有 2 个被编译闸判为假 KILLED 并作废。
Leader 点名的四件事已逐条回复：事项①**推翻了 dev 的一半说法**（结论仍成立，理由不成立），
事项②③④确认成立。四项发现均不构成 DoD 失败，其中发现 1、2 会影响下游，建议转达。
