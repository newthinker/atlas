# TASK-006 验证报告 — as-of 只读副本（`obsFrom()` 按 asOf 选形态）

- **验证者**：test-m4c-a
- **裁决**：**VERIFIED**（9/9 done_criteria 通过）
- **验证树**：`/Users/zuowei/workspace/go/src/github.com/newthinker/atlas` @ `a0b32a5daac10e83ae4bc170fef1ac1e23e8d732`（master）
- **verify_baseline 核对**：`head` 记录值 == 当前 `git rev-parse HEAD` == `a0b32a5…` ⇒ **被判定的代码零漂移**。
- ⚠️ **但 `discovery` 在验证期间确实漂移过**（`f225733…` → `af7f213…`），落盘时被守卫 DENY，我查证后显式确认放行 —— **处置与一处由此查出的新发现见 §12**。此处不写「零漂移」，因为那只在我承接的那一刻为真。
- **派验通知未送达**：我是靠 `TeammateIdle` hook + 自己轮询 `.arcforge/tasks/` 发现本任务已转 `verifying` 的。记此一笔——它是「即使通知丢失，各角色通过轮询自己负责的状态也能发现待办」这条设计**唯一一次真正生效**的实例。

---

## 1. 本报告的三条取证计划是**派验前**就写死的

为避免被交付物的措辞锚住，以下三条在我**看到 dev 实现之前**就已落进 checkpoint，本报告逐条兑现：

1. **先确认 `functional[1]` 里「相等那一例」是否存在**——若只写了 Nano 那例，`<=` 就是零守卫。
2. **`<=` → `<` 变异**：相等那例**必须红**，Nano 那例**存活是预期**；**只有两例都活才是真缺口**。
3. **参数顺序**用我在 `e4aa503` 上独立采的基准真值比，**不从 dev 的实现反推**。

## 2. 亲跑结果

| 命令 | 退出码 | 结果 |
| --- | --- | --- |
| `go build ./...` | 0 | 通过 |
| `go test -count=1 ./internal/crisis ./cmd/atlas` | 0 | 两包均 `ok` |
| `go test -cover -coverpkg=./internal/crisis` | 0 | **93.8%** |
| `go vet` / `gofmt -l` 两文件 | 0 / 空 | 通过 |
| 变异对照组 / 收尾对照 | 0 | **560 PASS / 0 FAIL**（两次同值） |

**`counts: 141` 我用三把尺复算**（第三把的失效条件与前两把**不重叠**——它不依赖名字结构）：

```
RUN 总数 174；尺1 无斜杠 = 141；尺2 祖先存在性 = 141；尺3 顶层 --- PASS 行 = 141
守恒：141 + 33 子测试 = 174   FAIL = 0
```

⚠️ 前两把尺共享「标签/层级由名字里的 `/` 决定」这个假设，**一致不构成互证**；第三把（数缩进为 0 的 `--- PASS:` 行）与它们的失效条件不重叠，三者同值才算复算完成。

## 3. discovery 重采核实（本任务的特殊流程）

dev 在 `dev_done` 状态下修了一处孤儿注释（合法：`owner_table.dev_done = ["dev-*"]`），产生第二个 commit，**原 discovery 的自证数字随之全部作废**。我独立核了重采结果：

| 字段 | discovery 现值 | 我的独立核实 |
| --- | --- | --- |
| `tree` | `@ a0b32a5` | == `git rev-parse HEAD` ✓ |
| `commit` | `a3e7d32`（实现）+ `50024a1`（注释修正） | `git log c7fa4b6..a0b32a5` 确认两个 ✓ |
| `numstat` | `a3e7d32: 56/14` + `50024a1: 7/3` | `git show --numstat` 逐个核 ✓；**56+7=63、14+3=17**，与 `git diff --numstat e4aa503..a0b32a5` 的 `63/17` 一致 ✓ |
| `baseline` | `e4aa503` | ✓ |
| `build`/`suite`/`coverage`/`counts` | 均注明「采于 a0b32a5」 | 我亲跑复现（§2）✓ |

⇒ **重采是真的，不是只补了内容。** 这一点值得记：上一轮 dev 曾「更新了 discovery 但没重采那四个字段」——**「我更新过了」这个自我认定会关掉复查**，而这次它逐字段做了。

## 4. done_criteria 覆盖矩阵（9/9）

| # | 完成标准（摘要） | 对应测试 | 针对性变异 | 判定 |
| --- | --- | --- | --- | --- |
| functional[0] | as-of 看历史版本；`AsOf(远未来)` 与不带 as-of 逐值相同（O7） | `TestAsOfSeesHistoricalRevision` / `TestAsOfFarFutureMatchesCurrent` | **M2 / M4 / M7** → 红 | **PASS** |
| functional[1]a | 边界**含端点**（`<=` 而非 `<`） | `TestAsOfIncludesEndpoint` | **M1**（`<=`→`<`）→ 红（§5） | **PASS** |
| functional[1]b | 生产 Nano 形态下整秒 as-of 含同秒小数行 | `TestAsOfNanoPrecisionBoundary` | M1 下**存活（预期）**；**M2 / M4 / M7** → 红 | **PASS** |
| functional[2] | 五个读方法在 as-of 形态下参数顺序正确，逐值断言 | `TestAsOfAllReadersParameterOrder` | **M2 / M3 / M4 / M7** → 红（§6） | **PASS** |
| boundary[0] | C8：t 之前无任何行 ⇒ `(nil, nil)`，不回退到最早那行 | `TestAsOfExcludesKeysWithNoRowsBefore` | **M4** → 红 | **PASS** |
| boundary[1] | `AsOf` 不改原 Store；副本与原 Store 共用同一个 `*sql.DB` | `TestAsOfIsReadOnlyCopySharingDB` | **M5**（`AsOf` 就地改原 Store）→ 红 | **PASS** |
| **boundary[2]** | **写路径裸读的行为层守卫**（我在 TASK-005 变异 R3 查出的覆盖缺口） | `TestUpsertOlderRevisionAfterNewerIsSwallowed` | **R3 重跑** → 红（§7） | **PASS** |
| error_handling[0] | `AsOf("")` 等价于当前形态 | `TestAsOfEmptyEqualsCurrent` | **M6** → 红（隔离重跑，§8） | **PASS** |
| non_functional[0] / [1] | build + 两包全绿；C5「WHERE/ORDER/LIMIT 文本逐字不变」 | 见 §2 / `git diff` | — | **PASS**（§9） |

## 5. 🔴 核心取证：`<=` 的「缺一」对 —— 预先登记的预测精确兑现

**M1**（把 `bitemporal.AsOfQuery` 的 `<=` 改成 `<`；该文件超出本任务 `writes`，**仅作变异用**）：

```
FAIL = ['TestAsOfIncludesEndpoint']        ← 唯一一条
TestAsOfNanoPrecisionBoundary               ← 存活
```

**与我在派验前写下的预测逐字一致。**

- **相等那一例有牙**：夹具 `asOfStore` 里确有一行 `FetchedAt: "2026-08-14T00:00:00Z"`，而 `TestAsOfIncludesEndpoint` 用的 as-of 恰是同一字符串 ⇒ `<` 会排除它、返回 10 而非 11。**「相等那一例」确实存在，不只是测试名叫这个。**
- **Nano 那例存活是预期，不是缺口**：我在**看 dev 实现之前**用纯字符串比对独立核过——`row = 2026-07-14T05:42:08.150777000Z`、`asof = 2026-07-14T05:42:08Z`，首个不同位 `pos=19`：`'.'(0x2E)` vs `'Z'(0x5A)` ⇒ **严格小于成立**，`<` 与 `<=` 对它**无差别**。它在构造上就区分不了这两个算符。

⇒ **两例各覆盖一半，合起来才钉住 `<=`。只有两例都活才是真缺口——实测只活一例，不是缺口。**

⚠️ 记一笔给后人：dev 主动解释了「为什么 Y2 该活下来」。那个解释**成立**，但它**是验收的判据，不是免验的理由**——「已经想过了」这种说明是最容易被直接采信、从而跳过取证的形态。本报告的做法是：先独立证实该性质，再用变异证实另一例有牙。

## 6. 参数顺序 —— 拿独立基准真值比，不从实现反推

DoD 明文说这是「本任务最易错处」，且**弄反不报错、只会返回空结果**。这类缺陷照着实现读永远自洽，故我在 `e4aa503` 上先独立采了基准：

```
AsOfQuery = SELECT * FROM macro_observations o WHERE o.fetched_at =
            (SELECT MAX(fetched_at) FROM macro_observations
             WHERE ts = o.ts AND indicator = o.indicator AND fetched_at <= ?)
? 个数 = 1，位于**子查询内**（pos 166，子查询 [56,167]）
⇒ as-of 参数必须**前置**于外层 WHERE 的全部参数
```

交付的五个读取点逐个核对，**全部前置**：

| 方法 | 参数拼接 |
| --- | --- |
| `Observation` | `append(args, indicator, date)...` ✓ |
| `LatestObservation` | `append(args, indicator)...` ✓ |
| `SeriesWindow` | `append(args, indicator, end, n)...` ✓ |
| `SeriesSince` | `append(args, indicator, from, end)...` ✓（`from` 参数名冲突，改名 `fromClause`，正确） |
| `EvalDates` | `append(args, IndVIX, from, to)...` ✓ |

变异证据：**M2**（`Observation` 改成 `append([]any{indicator,date}, args...)`）→ 4 条红；**M3**（`SeriesWindow` 同样改，**三个外层参数、最易错的那个**）→ `TestAsOfAllReadersParameterOrder` 红；**M4**（as-of 形态返回 `nil` args）→ 6 条红。

**dev 那句新注释我当作可证伪断言验了**：它说 `EvalDates`「**经过 `obsFrom()`**，漏的只会是列清单那半边，不会是时点语义」。核对属实——`EvalDates` 用的是 `fromClause, args := s.obsFrom()` 且 `append(args, IndVIX, from, to)`，与其余四点共用同一处 FROM/参数决策。⇒ 这使得 as-of 的参数注入是**一处**决策而非五处，M3 单点变异即能杀死覆盖五方法的断言，与该声明一致。

## 7. 🔴 `boundary[2]`：我在 TASK-005 找到的覆盖缺口，已闭合并经**同一个变异**证实

TASK-005 验证时我发现：把 `UpsertObservations` 的写冲突检测从基表改走视图（变异 **R3**），**只有源码守卫变红、零行为测试变红**——而源码守卫报的是「豁免清单对不上」这个**记账理由**，不是行为理由。我当时用探针把推理变成观察（交付态 `err=nil` vs 变异态 `UNIQUE … 1555`），并建议补一条行为断言。Leader 采纳并写进本任务 `boundary[2]`。

**本次用同一个 R3 变异复验**：

```
TASK-005 时： R3 → 零行为测试变红
本次     ： R3 → TestUpsertOlderRevisionAfterNewerIsSwallowed  **FAIL**
             错误：inserting vix/2026-01-02: UNIQUE constraint failed:
                   macro_observations.ts, macro_observations.indicator,
                   macro_observations.fetched_at (1555)
```

⇒ **缺口闭合，且是用暴露它的那个变异证实的**——不是靠「下游写了 DoD」这句话。错误文案与我在 TASK-005 探针里观察到的逐字一致。

## 8. 方法学：panic 会截断 FAIL 列表，我的有效性闸漏掉了这一格

**M6**（去掉 `obsFrom` 的空 as-of 判断）全套跑出 `PASS=235`（对照组 560）、36 条 FAIL，而 `TestAsOfEmptyEqualsCurrent` **不在 FAIL 列表里**。

我没有据此报「该断言存活」，而是先查为什么——输出里有 **2 条 `panic:`**：测试二进制**崩溃退出**，其后的测试根本没跑，FAIL 列表因此被截断。隔离重跑该测试：

```
--- FAIL: TestAsOfEmptyEqualsCurrent   Error: Expected value not to be nil.
```

⇒ 它**有牙**。

🔴 **这暴露了我沿用整个 sprint 的有效性闸有一格没盖住**。原闸两条：`vet_rc != 0`，或「`rc != 0` 且 FAIL 列表为空」。M6 两条都不命中（vet 过、FAIL 非空），闸却该响。**补第三条**：

> **PASS 计数相对对照组显著塌陷**（235 vs 560）⇒ 疑似二进制中途崩溃，FAIL 列表不完整，**不得据「某测试不在 FAIL 列表」推断它存活**。

## 9. `non_functional[1]`（review）：C5 逐字未变

`git diff e4aa503..a0b32a5 -- internal/crisis/store.go` 中，五处查询的 `WHERE` / `ORDER BY` / `LIMIT` 文本**逐字未变**，变的只有前缀（`obsSelect` → `obsCols+from`）与参数拼接：

```
-  obsSelect+` WHERE indicator = ? AND ts = ?`, indicator, date))
+  obsCols+from+` WHERE indicator = ? AND ts = ?`, append(args, indicator, date)...))
-  obsSelect+` WHERE indicator = ? AND ts <= ? ORDER BY ts DESC LIMIT ?`, indicator, end, n)
+  obsCols+from+` WHERE indicator = ? AND ts <= ? ORDER BY ts DESC LIMIT ?`,
```

⇒ C5 成立。

## 10. 顺带确认：TASK-004 那句假断言，dev 已在本任务订正

我在 TASK-005 查出 TASK-004 的 DoD 与注释都写着「走公开 API 造不出同一 `(ts,indicator)` 的两行」，实测为假。本任务的 `asOfStore` 夹具注释已写对：

> 走公开 API：三段主键下不同 `fetched_at` 即不同主键，`UpsertObservations` 会追加而非覆盖

⇒ 该夹具**确实走公开 API** 造出了同键两个修订，与我的实测一致。假断言未被复制到本任务。

## 11. 结论

9 条 done_criteria 全部通过，逐条有针对性变异证据（7 个变异 + 1 次 R3 复验，全部通过有效性闸）。三条**派验前登记**的取证计划逐条兑现：「相等那一例」确实存在且有牙、`<=` 的缺一对按预测只活一例、参数顺序与独立基准真值逐点一致。我在 TASK-005 发现的覆盖缺口经**同一个变异**证实闭合。discovery 的重采逐字段核实属实。

**裁决：VERIFIED。**

**移交 Leader 两件事**：
1. **有效性闸补第三条**（§8）：PASS 计数相对对照组塌陷 ⇒ 疑似崩溃、FAIL 列表不完整。这条我整个 sprint 都没有，建议进 final-report 的方法学节。
2. **派验通知未送达**（见报告头）：我靠 hook + 轮询自愈。这是本 sprint 第 N 次消息丢失，但**第一次由设计中的兜底机制接住**，值得作为正面案例记一笔。


## 12. 🔴 判定落盘时漂移守卫响了 —— 处置与一处新发现

`transition verified` 首次被 **DENY**：

```
你承接 TASK-006 时的 discovery sha256: f2257331...
现在的 discovery sha256              : af7f2137...
DENY: 验证对象漂移：TASK-006 在你承接之后变了
```

**我没有直接 ack**，先查了三件事：

| 查什么 | 结果 |
| --- | --- |
| HEAD 是否也变了 | **没有**：`git rev-parse HEAD` == `verify_baseline.head` == `a0b32a5` ⇒ **被判定的代码一字未动** |
| 我判定所依据的 `verification` 字段是否变了 | **逐字未变**（`tree`/`commit`/`baseline`/`numstat`/`build`/`suite`/`counts`/`coverage` 与我承接时读到的完全相同） |
| 新内容是否与我的发现矛盾 | **无矛盾，且逐条吻合**：dev 的 `Y2`（`<=`→`<`）只杀 `TestAsOfIncludesEndpoint` == 我的 **M1**；`Y4`（= 我的 R3）杀 `TestUpsertOlderRevisionAfterNewerIsSwallowed` == 我的 **R3 复验**；`Y1`（参数顺序）杀 4 条 == 我的 **M2** |

⇒ 漂移是**追加性的**（新增顶层 `provenance`、扩充 `o7_real_data`），**不影响判定**。已按守卫要求显式确认：`--ack-discovery-drift af7f2137...`（确认值取**当前值**，必须真去看过现状才填得出）。

⚠️ 更要紧的是：**这份判定本来就不依赖 discovery 的任何自报数字**——`numstat` 我用 `git show` 逐 commit 核过、`counts` 我用三把尺复算过、`build`/`suite`/`coverage` 我亲跑、9 条 DoD 全部由我自己的变异取证。discovery 在本报告里只作**交叉核对**，不作证据来源。所以即便它变了，判定的地基没有动。

### 12.1 新发现：`provenance` 与 `verification.commit` 自相矛盾（非 DoD 项，不阻断）

```
provenance          : 「**单个 commit** a3e7d32…，合入经 merge commit **c7fa4b6**」
verification.commit : 「**两个 commit**：a3e7d32（实现，合入经 c7fa4b6）+ 50024a1（注释修正，合入经 **a0b32a5**）」
```

`provenance` **停留在注释修正之前的状态**——dev 重采了 `verification` 的全部字段，却没重采这个顶层字段。这正是上一轮 Leader 诊断过的同一种失效（「它更新了 discovery，只是没重采那几个字段」），**换了一个字段复发**。

**为什么它没被挡住——而这一半责任在我**：我上一轮给出的「机械枚举重采清单」命令是

```bash
jq -r '.verification | to_entries[] | select(.value|tostring|test("[0-9a-f]{7,40}|…"))'
```

**scope 限定在 `.verification` 内**，而 `provenance` 是**顶层**字段。实测：我的原命令列出 12 项，**不含 `provenance`**；改成顶层全扫才能看见它（`provenance` / `key_findings` / `verification`）。

⇒ **我提出的机械检查本身有 scope bug，而它漏掉的恰好就是失效的那一个。** 这与本报告 §8 的「有效性闸漏了一格」是同一天里的第二次——**机械化不等于完备，枚举式的检查必然有第 N+1 个载体**。修正版：

```bash
jq -r '[paths(scalars) as $p | {path:($p|join(".")), v:getpath($p)}]
       | .[] | select(.v|tostring|test("[0-9a-f]{7,40}|[0-9]+/[0-9]+|[0-9]+\\.[0-9]+%|rc="))
       | .path' <discovery>
```

（递归扫全部标量路径，不预设子树。）

**不阻断的理由**：`provenance` 不是本任务的 done_criteria 项；它名下的事实（「代码全部由 dev-m4c-a 编写」）仍为真，错的只是 commit 数与合入点。**但它会误导 `context_from` 的下游读者**，建议 Leader 在 final-report 记一笔。

⚠️ **我没有去改它**，两个理由：①`verified` 之后 discovery 时机守卫对**所有角色**无条件 DENY，窗口已关；②即便还开着，**验证者不该写 dev 的 discovery**——来源混在一起，后人就分不清哪句是谁说的。验证者的发现应当落在验证报告里，这份报告就是它的正确载体。

---

### 复现命令（锚一律钉全 sha）

```bash
cd /Users/zuowei/workspace/go/src/github.com/newthinker/atlas
git rev-parse HEAD   # 须为 a0b32a5daac10e83ae4bc170fef1ac1e23e8d732
GOTOOLCHAIN=local go test -count=1 -v ./internal/crisis ./cmd/atlas
GOTOOLCHAIN=local go test -count=1 -cover -coverpkg=./internal/crisis ./internal/crisis

# 变异（隔离副本）
git worktree add --detach ../wt-verify-TASK-006 a0b32a5daac10e83ae4bc170fef1ac1e23e8d732
python3 <scratchpad>/t006.py          # M1-M7
# R3 复验：把 store.go 的写冲突检测 SELECT 从 macro_observations 改为 v_macro_current
git worktree remove --force ../wt-verify-TASK-006

# as-of 基准真值（采于 e4aa5034075c5d08f4e9ce0873e3b128d6a374e7，bitemporal 包本次未改动）
# 直接调 bitemporal.NewSpec/CurrentQuery/AsOfQuery 打印 SQL 与 ? 位置
```
