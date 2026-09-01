# TASK-012 验证报告 —— 补 magnitude_ranges 的 NaN/Inf 穿透 + 五条守卫缺口

- **验证者**：test-m1c3b-b
- **判定对象树**：`f14fd2850bfb75dbca8d56dd7120bdb676692714`（= `verify_baseline.head`）
- **dev 交付**：`18eb496`（feat）→ `f14fd28`（merge）；改动前树 `962c3acb2970…`
- **结论**：✅ **VERIFIED** —— 9 项 DoD 全部 PASS
- **本任务是 TASK-004 验证发现的闭环**：我在 TASK-004 报的消融 M4（`>=`→`>` 五条全绿）指出
  「`min == max` 无守卫，而该契约已被广告给 TASK-005」。**现在它有守卫了，且我实测确认守得住。**

## 0. 基线核对

| 项 | 记录 | 实测 | 判定 |
|---|---|---|---|
| `head` | `f14fd2850bfb75dbca8d56dd7120bdb676692714` | 同值 | ✅ 零漂移 |
| `discovery_sha256` | `fad3482cf51775c59f2ff7dd35e07d1b2aede74e851dd036f0a7494a81be9122` | 同值 | ✅ |

`assignment_epoch=1`。

---

## 1. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|
| functional[0] | 🔴 堵 NaN/Inf 穿透 + 注释写明机制与后果 | `thresholds.go` +22，守卫置于 `Min>=Max` **之前**；`TestMagnitudeRangesRejectNaNOrInf` 6 格 PASS；**变异下 6/6 格转红**（§3.3） | ✅ PASS |
| functional[1] | YAML 可达性用例（`.nan`/`.inf` 从字面量进表） | `TestMagnitudeRangesRejectNaNFromYAML` 6 格 PASS；**变异下 6/6 格转红，无假绿**（§3.3） | ✅ PASS |
| functional[2] | 🔴 补两条存活变异用例（`Min==Max` / `Unit:"   "`） | 变异 M1 ⇒ `RejectDegenerateRange` **独家红**；M2 ⇒ `RejectBlankUnit` **独家红**（3 子格全红）（§3.1） | ✅ PASS |
| functional[3] | 🔴 补 F12 / F17#5#6 消费端守卫 | 变异 M4（**函数级定位**）⇒ `ReportsEarliestFieldInFieldOrder` 独家红；M5/M6 ⇒ `BoundariesAreInclusive` **精确到单个子格**（§3.2） | ✅ PASS |
| boundary[0] | 空表仍合法（TASK-004 的 boundary[0] 不得被破坏） | `TestEmptyMagnitudeRangesStillValid` PASS | ✅ PASS |
| boundary[1] | 🔴 不得破坏 TASK-005 已填的 54 项 | `TestShippedConfigLoadsAndIsCalibrated` PASS；`configs/hestia.yaml` 改动 **0 行** | ✅ PASS |
| error_handling[0] | 新增用例先 RED；**三把不同的尺**；变异在隔离副本 | dev 的方法与指纹记录完整；**六个变异我全部独立复现，逐条一致**（§3） | ✅ PASS |
| non_functional[0] | gofmt / vet / test / 覆盖率 / 无新依赖 | 见 §4 | ✅ PASS |
| non_functional[1] | 交付流程 AD-4 | merge `02:28:12Z` < dev_done `02:30:19Z`，早 2 分 7 秒 | ✅ PASS |

---

## 2. 实现核对：守卫顺序是对的

```go
if math.IsNaN(r.Min) || math.IsNaN(r.Max) ||
    math.IsInf(r.Min, 0) || math.IsInf(r.Max, 0) {        // ← 在 Min>=Max 之前
    return fmt.Errorf("… min/max 必须是有限实数 … NaN 参与的比较恒假、Inf 区间永不越界，
                       两者都会让这道闸对该字段完全不设防且报 passed —— YAML 里写 .nan / .inf 也会走到这里")
}
if r.Min >= r.Max { … }
if strings.TrimSpace(r.Unit) == "" { … }
```

顺序要紧：NaN 守卫若放在 `Min >= Max` **之后**，`{Min:NaN, Max:NaN}` 会先被倒置校验放行（NaN 比较恒假）
再落到 unit 检查 —— 报出的理由会指错方向。实现放在最前，正确。

**改动全是纯新增，零删除**：`thresholds.go +22/-0`、`thresholds_test.go +147/-0`、`validate_test.go +86/-0`。
`validate.go` **根本不在改动列表里** ⇒ 直接印证「F12/F17 的实现一个字没动，补的是守卫不是 bug」。

---

## 3. 变异实测（隔离 worktree `wt-t012-b`，钉死 `f14fd285…`）

对照组：全量 `go test ./internal/hestia/ -v` ⇒ **零 FAIL**。收尾核实两个被变异文件 sha256 均复原。

**六个变异我全部独立复现，与 dev 所报逐条一致**（Leader 只要求至少两个）：

| # | 变异 | 转红（顶层） | 外溢度 | 与 dev 报的一致 |
|---|---|---|---|---|
| M1 | `r.Min >= r.Max` → `r.Min > r.Max` | `TestMagnitudeRangesRejectDegenerateRange` | **1** | ✅ |
| M2 | `strings.TrimSpace(r.Unit)==""` → `r.Unit==""` | `TestMagnitudeRangesRejectBlankUnit`（3 子格全红） | **1** | ✅ |
| M4 | F12：`for _, f := range fieldOrder` → `range in.cfg.MagnitudeRanges` | `TestMagnitudeSanityReportsEarliestFieldInFieldOrder` | **1** | ✅ |
| M5 | F17：`v < r.Min` → `v <= r.Min` | `TestMagnitudeSanityBoundariesAreInclusive/恰好等于_min` | **1** | ✅ |
| M6 | F17：`v > r.Max` → `v >= r.Max` | `TestMagnitudeSanityBoundariesAreInclusive/恰好等于_max` | **1** | ✅ |
| M3′ | NaN/Inf 判据整条置 false | `RejectNaNOrInf` + `RejectNaNFromYAML` | 2（预期内，同守一条实现） | ✅ |

M5/M6 值得单说：它们**精确到单个子格**（分别只红「恰好等于 min」与「恰好等于 max」那一格）。
这正是 DoD 要补 F17 的理由 —— 原用例用 `42` 对区间 `[3,4]`，离边界两个数量级，`<`→`<=` 不会有任何反应。

### 3.1 我在 TASK-004 报的缺口，现在闭环了

TASK-004 时消融 M4（`>=`→`>`）**五条测试全绿**；现在同一个变异 ⇒ `TestMagnitudeRangesRejectDegenerateRange`
**独家转红**。那条被广告给 TASK-005 的契约（「`min == max` 也会被拒」）现在有守卫了。

### 3.2 F12 的定位：我用**行号（函数级）**，确认了 Leader 的警告成立

`for _, f := range fieldOrder {` 在 `validate.go` 有 **2 处**，我逐个查了所属函数：

```
行 442 → func yoyFields() []string
行 512 → func gateMagnitudeSanity(in gateInput) Check     ← 目标
```

⇒ **全文件替换会先命中 442 行的 `yoyFields`**，得到的 KILLED 是假的（红的是别的性质）。
我的变异按行号 512 精确定位；dev 的脚本按**函数体**定位（`verification.mutation.method` 明写「F12 必须如此」）。
两种做法等效，都避开了这个陷阱。

### 3.3 🔴 假绿检测：十二格逐格确认（这一步 dev 未做，我补的）

dev 自己抓到过一个假绿 —— YAML 用例第一版 `min: .inf, max: 1000` 时 `+Inf >= 1000` 为真，
被**既有的倒置区间校验**拦下，与 NaN 守卫毫无关系，而 RED 阶段显示 PASS。它重构为六格、每格刻意避开 `Min >= Max`。

**它只报了顶层测试转红，没有逐格确认。** 我把 M3′（NaN 判据整条置 false）的结果解析到子测试级：

| 测试组 | 转红格数 | 判定 |
|---|---|---|
| `TestMagnitudeRangesRejectNaNOrInf` | **6/6** | ✅ 无假绿 |
| `TestMagnitudeRangesRejectNaNFromYAML` | **6/6** | ✅ 无假绿 |

十二格逐格转红 ⇒ **每一格都只能因目标性质（NaN/Inf 守卫）而红**，没有一格是被倒置区间校验或别的分支拦下的。
dev 那次重构确实修干净了。

> 这与我在 TASK-004 对 `TestEmptyMagnitudeRangesStillValid` 做的分析同类：
> **一个为错误理由变绿的测试，从外面看和真正守住了完全一样**，只有变异能分开。

### 3.4 一个方法学观察：**「删整块」在有 import 依赖时会失效**

我第一版 M3 是**删掉 NaN 守卫整块** ⇒ **编译失败**（`math` 包变成未使用的 import）。
若不检查编译结果就记 KILLED，会得到一个假 KILLED —— 崩溃型变异让全套变红，与「守卫生效」在输出上不可分。

改用**条件整条置 false**（保留 `math` 引用）后变异有效。
**dev 用的正是这个手法**（`verification.mutation.results` 写「M6 NaN 判据整条置 false」）—— 它避开了我踩的这个坑。

---

## 4. `min(100)` 断言的射程（Leader 点名要核）—— 精确，两个方向都确认了

新用例断的是 `min(100)` 而非字段名。我用探针打印五个分支的**实际**错误串：

| 用例 | 含 `min(100)` | 含字段名 `m2` |
|---|---|---|
| `{Min:100, Max:100}`（目标） | **true** | true |
| `{Min:100, Max:200, Unit:""}` | false | true |
| `{Min:100, Max:200, Unit:"   "}` | false | true |
| `{Min:200, Max:100}`（倒置） | false | true |
| `{Min:NaN, Max:100}` | false | true |

⇒ **五个分支全部含字段名**（只断字段名确实分不清是哪条在响，判据会与用例数据耦合）；
**只有目标用例含 `min(100)`**。配合 §3 的 M1 变异（该断言独家转红），射程从**两个方向**得到确认：
横向区分于其它分支，纵向由变异证明它守的正是 `>=` 这个比较符。

> 这条理由与我在 TASK-004 §4.3 指出的判据射程问题同源 —— 断言红得对、给的理由却指错方向，是同一类问题。

---

## 5. 门禁项（采于 `f14fd285…`）

| 项 | 实测 | 判据 | 判定 |
|---|---|---|---|
| `go test ./internal/hestia/... -cover` | ok, **96.1%** | ≥ 95.9%（现基线 96.1%） | ✅ |
| `go test ./cmd/...` | ok | — | ✅ |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出 | 零输出 | ✅ |
| `gofmt -l internal/hestia cmd/atlas` | 恰两个既有欠账 | 之外无新增 | ✅ |
| go.mod / go.sum | **0 行** | 不得出现 | ✅ |
| `configs/hestia.yaml` | **0 行** | 本任务不改（不在 writes） | ✅ |
| 改动范围 vs `writes` | **完全吻合，零越界** | — | ✅ |

**两条不得破坏的**：`TestEmptyMagnitudeRangesStillValid`（TASK-004 的 boundary[0]）与
`TestShippedConfigLoadsAndIsCalibrated`（TASK-005 的 54 项）**均 PASS** ⇒ 新守卫没有让已填的任何一项失败。

---

## 6. 复现命令（锚钉全 sha）

```bash
git worktree add --detach <dir> f14fd2850bfb75dbca8d56dd7120bdb676692714

# 六个变异（⚠️ F12 必须按行号/函数体定位：validate.go 的 442 行是 yoyFields，512 行才是目标）
#   M1 thresholds.go: `r.Min >= r.Max`            → `r.Min > r.Max`
#   M2 thresholds.go: `strings.TrimSpace(r.Unit)==""` → `r.Unit==""`
#   M3′ thresholds.go: NaN/Inf 判据整条置 false   （⚠️ 不要删整块 —— math import 会变未使用而编译失败）
#   M4 validate.go 行 512: `for _, f := range fieldOrder {` → `for f := range in.cfg.MagnitudeRanges {`
#   M5 validate.go 行 521: `v < r.Min` → `v <= r.Min`
#   M6 validate.go 行 521: `v > r.Max` → `v >= r.Max`
# 解析到子测试级（`--- FAIL: X/Y`）才能做假绿检测

go test ./internal/hestia/... -count=1 -cover      # ⇒ 96.1%
```

---

## 7. 结论

**VERIFIED。** 9 项 DoD 全部 PASS。

本任务是 TASK-004 验证发现的闭环，而且闭得实：我在 TASK-004 报「`>=`→`>` 五条全绿」，现在同一变异
让新用例**独家转红**。六个变异我全部独立复现、与 dev 所报逐条一致，M1/M2/M4/M5/M6 **外溢度均为 1**，
M5/M6 精确到单个子格。

我额外补了 dev 未做的一步 —— **十二格假绿检测逐格确认**（两组各 6/6 转红），证实它那次「六格重构避开
`Min >= Max`」确实修干净了自己抓到的假绿。
