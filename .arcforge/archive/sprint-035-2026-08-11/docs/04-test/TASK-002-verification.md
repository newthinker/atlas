# TASK-002 验证报告 —— completeness 必填集派生

- **验证者**：test-agent-24（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：`master @ cfc24feb9afc926bd422895085c0c4b3417f77be`（全 sha）
- **verify_baseline.head**：`cfc24feb9afc926bd422895085c0c4b3417f77be`
- **结论**：**VERIFIED（8/8 PASS）**

---

## 0. 漂移核实与 scope 一致性

```
$ git rev-parse HEAD
cfc24feb9afc926bd422895085c0c4b3417f77be
$ shasum -a 256 .arcforge/discoveries/TASK-002.json
a0eadd3172d013a94d6a4076407f022479f5b8cd401a4c719f532687642b6bef
$ jq -r .verify_baseline.discovery_sha256 .arcforge/tasks/TASK-002.json
a0eadd3172d013a94d6a4076407f022479f5b8cd401a4c719f532687642b6bef
```

HEAD 与 discovery sha256 均与基线**逐字相同** ⇒ 零漂移，不需要 `--ack-drift`。

```
$ git show --numstat --oneline e24c0627df4adb3abcc662dc5027cc9dd48490c2
e24c062 feat(hestia): 从模板表派生 completeness 的必填集
51	0	internal/hestia/required.go
110	0	internal/hestia/required_test.go
```

实际改动 2 个文件，与 `writes` 声明**逐项一致**，无越界；与 discovery 自报的
`commit_numstat`（51/0、110/0，纯新增）一致。未碰 `store_test.go`，与「本任务不新增导出物」相符
（`tsfSectionFields`/`requiredFields` 均非导出，我已在 §4 用 M14 型消融确认导出面守卫本身仍是精确相等）。

---

## 1. 整包门禁（隔离 worktree `/tmp/verify-035 @cfc24fe`）

```
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  	github.com/newthinker/atlas/internal/hestia	0.718s	coverage: 90.2% of statements
$ GOTOOLCHAIN=local gofmt -l internal/hestia/      → 无输出
$ GOTOOLCHAIN=local go vet ./internal/hestia/      → 无输出
```

新增函数逐函数覆盖率：`required.go:15 tsfSectionFields 100.0%`、`required.go:28 requiredFields 100.0%`。

---

## 2. done_criteria 覆盖矩阵

每条配消融实验，先过语法闸（`gofmt -l`）与编译闸（`go vet`）以排除
「编译失败伪装成 KILLED」，再核对红是**被哪条断言**杀的（附 `Error Trace` 行号）。
消融只作用于一次性 worktree `/tmp/mut-035`，主工作区 sha256 全程未变。

| # | 完成标准 | 对应测试 | 消融证据（KILLED 及致红断言） | 判定 |
|---|---|---|---|---|
| functional[0] | `tsfSectionFields()` 恰好 27 个**且无重复** | `TestTSFSectionFieldsIsExactAndDistinct` | 长度侧：N2 去掉 3 个总量 ⇒ `required_test.go:22 should have 27 item(s), but has 24`；N3 存量表只取 balanceField ⇒ `:22 … but has 19`。**查重侧单独立得住**：N4 把总量写成 `FieldTSFStock, FieldTSFStockYoY, FieldTSFStockYoY`（**长度仍是 27**）⇒ `:27 Should be false` ⇒ seen map 断言不是 `assert.Len` 的附庸 | PASS |
| functional[1] | `requiredFields(extractorV2)` = 54，与 `golden2025` **双向一致** | `TestRequiredFieldsMatchGoldenKeySets/rule@v2` | N7' 保持长度 54、只把 `c[0]` 换成 `"bogus_field"` ⇒ **两个方向同时红**：`:81 golden does not contain "bogus_field"`（必填集⊆golden）与 `:84 Should be true`（golden⊆必填集）⇒ 双向断言各自有效，不是长度断言的重复 | PASS |
| functional[2] | `requiredFields(extractorV1)` = 27，与 `golden2020` **双向一致** | `TestRequiredFieldsMatchGoldenKeySets/rule@v1` | N2 ⇒ `:74 … but has 30`；N3 ⇒ `:74 … but has 35`；N4 ⇒ `:74 … but has 28`。三次均由该子测试致红 | PASS（附强度说明，见 §3） |
| boundary[0] | 返回**副本**，改动返回值不得影响 `fieldOrder` | `TestRequiredFieldsReturnsCopy` | N5' `slices.Clone(fieldOrder)` → `fieldOrder[:len(slices.Clone(fieldOrder))]`（**刻意保留 `slices` 用法**）⇒ `:97 expected "tsf_stock" / actual "tampered"`。⚠ 首版 N5 写成裸 `return fieldOrder` 时也红，但根因是 `"slices" imported and not used` **编译失败**，被我的编译闸拦下判为无效 —— 与 dev-agent-46 记录的陷阱同形态 | PASS |
| error_handling[0] | `llm-fallback@v1` / `rule@v3` / `""` **均返回 nil** | `TestRequiredFieldsRejectsAmbiguousExtractor` | N6' default 分支 `return nil` → `return slices.Clone(fieldOrder)` ⇒ `:107`、`:108`、`:109` **三条 `assert.Nil` 全部**致红 ⇒ 三个输入各自被覆盖，不是只测了一个 | PASS |
| non_functional[0] | `tsfSectionFields` 必须**遍历模板表派生**，不得用 `strings.HasPrefix(f,"tsf_")` | `TestTSFSectionFieldsDerivesFromTemplateTables`（哨兵分项） | **本报告的核心消融，见 §3** | PASS |
| non_functional[1] | RED **因预期原因**失败（`undefined: tsfSectionFields` / `undefined: requiredFields`），原文入 discovery | discovery `red_1/red_2_actual_verbatim` + §4 独立复现 | 见 §4 | PASS |
| non_functional[2] | import 按需增补（不得照抄计划的 `slices`）；gofmt/vet 无输出、整包绿 | §1 + 源码审读 | `required.go` 的 `import "slices"` 确实**只在 `requiredFields` 里用到**（`slices.Clone`），`tsfSectionFields` 零 import 依赖；§4 的删文件复现里也未出现 `imported and not used` | PASS |

---

## 3. non_functional[0] 的关键消融：前缀实现必须被杀死

派单点名要我独立复核这一条，且**不要把 functional[2] 的强度读高**。两件事我都做了。

### 3.1 golden2020 的强度：确认 dev-agent-47 的自陈成立

`golden_test.go:288` 的 `TestGoldenTablesAreWellFormed/2020` 把 `golden2020` 钉成
「**恰好** = `nonTSFFields()`」，而 `nonTSFFields()`（`golden_test.go:246`）自身就是
`strings.HasPrefix(f, "tsf_")` 筛 `fieldOrder`。

⇒ v1 方向的双向一致验证的是「**模板表派生 == 前缀口径**」这一当前事实，
**不是**派生正确性的独立佐证。这不是推理，下面的实测直接证实了它。

### 3.2 消融：把实现换成前缀筛选

在 `/tmp/mut-035` 里把 `tsfSectionFields` 整体换成：

```go
func tsfSectionFields() []string {
	out := make([]string, 0, 27)
	for _, f := range fieldOrder {
		if strings.HasPrefix(f, "tsf_") {
			out = append(out, f)
		}
	}
	return out
}
```

**有效性闸先行**：`gofmt -l` 无输出（语法有效）；`go vet ./internal/hestia/` 无输出
（**编译通过 ⇒ 后续任何红都不是编译失败**）。

整包结果 —— **全包唯一的 FAIL 就是那条哨兵测试**：

```
--- FAIL: TestTSFSectionFieldsDerivesFromTemplateTables (0.00s)
    required_test.go:52: "[…27 项…]" should have 30 item(s), but has 27
        Messages: 往模板表加了 3 个字段，派生结果却没跟着长——没在遍历模板表
    required_test.go:54: […] does not contain "sentinel_balance"
    required_test.go:54: […] does not contain "sentinel_balance_yoy"
    required_test.go:54: […] does not contain "sentinel_flow_ytd"
FAIL	github.com/newthinker/atlas/internal/hestia	0.550s
```

同轮其余四条**全部 PASS**：

```
--- PASS: TestTSFSectionFieldsIsExactAndDistinct
--- PASS: TestRequiredFieldsMatchGoldenKeySets/rule@v2
--- PASS: TestRequiredFieldsMatchGoldenKeySets/rule@v1     ← 前缀实现下**照样全绿**
--- PASS: TestRequiredFieldsReturnsCopy
--- PASS: TestRequiredFieldsRejectsAmbiguousExtractor
```

**两个结论**：

1. `TestTSFSectionFieldsDerivesFromTemplateTables` **真的能杀死前缀实现**，且是被断言杀的
   （`assert.Len` + 三条 `assert.Contains`，带 Messages），不是编译失败。
2. 计划原有的三条断言（27 个 / 无重复 / golden 双向）对 non_functional[0] **零守护** ——
   dev-agent-47 的「守卫在场但参数无守」判断成立，增补那条测试是**唯一**有效的执行机制。
   ⇒ 该增补属**必要**，不属越权扩大范围（且改动仍落在声明的 `writes` 内）。

变异体已还原：`required.go` sha256 回到 `739eda4d3c325c3b4bcd00fc17667d49d4b1f5c1bb10e4eab015bb333549a67c`，
`git status --porcelain internal/hestia/` 为空。

---

## 4. non_functional[1]（RED 因预期原因失败）的独立核查

discovery 的两段 `*_actual_verbatim` 均为 `undefined: …`，无 `imported and not used`。
我另做直接观察确认失败**类型**（而非采信原文）：把 `required.go` 整个移走再跑整包 —

```
internal/hestia/required_test.go:21:9: undefined: tsfSectionFields
internal/hestia/required_test.go:41:14: undefined: tsfSectionFields
internal/hestia/required_test.go:51:9: undefined: tsfSectionFields
internal/hestia/required_test.go:73:11: undefined: requiredFields
internal/hestia/required_test.go:107:16: undefined: requiredFields
FAIL	github.com/newthinker/atlas/internal/hestia [build failed]
```

失败形态与计划预期一致。`required_test.go` 的 import（`slices`/`testing`/`assert`/`require`）
**每一个都被真正使用** ⇒ 结构上不可能出现 `imported and not used`。文件已还原（sha256 `739eda4d…`）。

---

## 5. 未据以判不通过的项

- validator 报 2 条 `TASK-002: scope-writes-outside-packages` —— **AD-035-4 已裁定为形状级假阳**，
  不作为判定依据。validator 整体 `exit=0`，`✓ 任务图校验通过（7 个任务）`。
- discovery `decisions[3]` 记录接受了 code-simplifier 的一处改动（v1 分支 `make(...,27)` → `len(section)`）。
  我在被验树上核实该改动已生效（`required.go:31 tsf := make(map[string]bool, len(section))`），
  实现里**不再有硬编码 27**，行为不变，且本报告的全部数字均采自最终提交版。

## 6. 主工作区完整性

```
$ shasum -a 256 internal/hestia/required.go
739eda4d3c325c3b4bcd00fc17667d49d4b1f5c1bb10e4eab015bb333549a67c
$ git status --porcelain   → 仅 .arcforge/ 条目，internal/ 一个字节未动
```

---

## 结论：**VERIFIED**

8 条 done_criteria 全部有对应测试、全部有消融证明断言在守卫、全部核对了致红归因。
派单点名的两个薄弱点均已独立复核：functional[2] 的强度**确实有限**（§3.1，已如实记录，
但 DoD 本身只要求双向一致的断言存在且有效，N2/N3/N4 证明它确实会红 ⇒ PASS），
而「必须走模板表」由哨兵测试**真实守住**（§3.2 实测）。
