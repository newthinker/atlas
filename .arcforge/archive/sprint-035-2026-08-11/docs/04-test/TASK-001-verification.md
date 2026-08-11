# TASK-001 验证报告 —— Thresholds 与配置自校验

- **验证者**：test-agent-24（Reality Checker，默认判定 NEEDS WORK）
- **被验对象**：`master @ cfc24feb9afc926bd422895085c0c4b3417f77be`（全 sha）
- **verify_baseline.head**：`cfc24feb9afc926bd422895085c0c4b3417f77be`
- **结论**：**VERIFIED（8/8 PASS）**

---

## 0. 漂移核实（先做，因为它决定后面所有证据测的是不是同一棵树）

```
$ git rev-parse HEAD
cfc24feb9afc926bd422895085c0c4b3417f77be
$ shasum -a 256 .arcforge/discoveries/TASK-001.json
23d297e36ec58a0fdf128a2fee01fc4b3b81cc4a388180f2f087211f509582c0
$ jq -r .verify_baseline.discovery_sha256 .arcforge/tasks/TASK-001.json
23d297e36ec58a0fdf128a2fee01fc4b3b81cc4a388180f2f087211f509582c0
```

HEAD 与 discovery sha256 **均与基线逐字相同** ⇒ 零漂移，不需要 `--ack-drift`。
dev-agent-46「交付后未再改动任何交付物」的**声称**据此**核实为真**（不是采信）。

### scope 一致性（越界申报核查）

```
$ git show --numstat --oneline 234baea4159d854dcf97c49cf8ace02d25f901f3
234baea feat(hestia): 加 Thresholds 与 M0 校准的默认阈值及口径豁免自校验
9	2	internal/hestia/store_test.go
120	0	internal/hestia/thresholds.go
101	0	internal/hestia/thresholds_test.go
9	3	internal/hestia/types.go
```

实际改动 4 个文件，与声明的 `writes` 四项**逐项一致**，无越界。
discovery 自报的 `types.go +9/-3`、`store_test.go +9/-2` 与提交本身的 numstat 一致。

---

## 1. 整包门禁（在隔离 worktree `/tmp/verify-035 @cfc24fe` 内执行）

```
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  	github.com/newthinker/atlas/internal/hestia	0.718s	coverage: 90.2% of statements
$ GOTOOLCHAIN=local gofmt -l internal/hestia/      → 无输出
$ GOTOOLCHAIN=local go vet ./internal/hestia/      → 无输出
```

基线（`/tmp/probe-A @125ad896fb096f7766cbb3c958ba2635a311c6ba`）：

```
ok  	github.com/newthinker/atlas/internal/hestia	0.666s	coverage: 89.5% of statements
```

> ⚠ **偏差记录（不影响判定）**：DoD 与 discovery 都写基线 **89.4%**，我实测 **89.5%**。
> 判据是「不得跌破 80」，实测 90.2%，PASS。

新增函数逐函数覆盖率（`go tool cover -func`）：

```
thresholds.go:75  DefaultThresholds  100.0%
thresholds.go:87  validate           100.0%
thresholds.go:112 exemptionFor       100.0%
types.go:116      checkEnum          100.0%
```

---

## 2. done_criteria 覆盖矩阵

判据是「**改坏它有东西会红吗**」，不是「测试过了吗」。每条都配了消融实验，
且**每次消融都核对了红是被哪条断言杀的**（附 `Error Trace` 行号），先过语法闸（`gofmt -l`）
与编译闸（`go vet`）排除「编译失败伪装成 KILLED」。消融全部只作用于一次性 worktree
`/tmp/mut-035`，主工作区 sha256 全程未变。

| # | 完成标准 | 对应测试 | 消融证据（KILLED 及致红断言） | 判定 |
|---|---|---|---|---|
| functional[0] | 阈值容得下 M0 极值（Deposit>0.0906 / CorpLoan>0.0158 / YoY>25） | `TestDefaultThresholdsAdmitM0Measurements` | M3a `0.12→0.02` ⇒ `thresholds_test.go:24 "0.02" is not greater than "0.0906"`；M3b `0.05→0.01` ⇒ `:26 "0.01" is not greater than "0.0158"`；M3c `50→20` ⇒ `:28 "20" is not greater than "25"`。**三条断言各自独立吃重** | PASS |
| functional[1] | `MagnitudeRanges` 必须为空 | `TestDefaultThresholdsLeaveMagnitudeRangesUncalibrated` | M4 填一条 m2 区间 ⇒ `:37 Should be empty, but was map[m2:{200 400 万亿元}]`（同轮 functional[0] 仍 PASS ⇒ 归因干净） | PASS |
| functional[2] | `exemptionFor` 精确命中返回非 nil 且 Version 正确 | `TestExemptionForMatchesPeriodAndCheckExactly` 前两段 | M5a 命中返回 nil ⇒ `:94 Expected value not to be nil`；M5b 返回 Version 为空的副本 ⇒ `:95 expected "2025-01" / actual ""` | PASS |
| boundary[0] | 豁免不外溢（①未列入 SkipChecks ②相邻期次），**两个方向** | 同上测试后两条 `assert.Nil` | M1 `Period == → <=` ⇒ `:99`（相邻期次 2025-02 被命中）；M2 `slices.Contains(...) → (... \|\| len(checkID)>0)` ⇒ `:97`（yoy_sanity 被豁免）。M2 刻意**保留 `slices` 用法**，避开 dev-agent-46 记录的「裸 `true` ⇒ 未使用 import ⇒ 编译失败伪装 KILLED」陷阱 | PASS |
| error_handling[0] | `validate()` 拒绝四类畸形豁免且关键字正确；**先 `require.NoError` 确认 base 合法** | `TestThresholdsRejectMalformedExemptions`（1 前置 + 4 子测试） | 缺陷侧：M6 删 Version 枚举校验 / M7 删 Period 校验 / M8 Reason 漏 TrimSpace / M9 删 SkipChecks 校验 ⇒ **各自对应的子测试**在 `:77 An error is expected but got nil` 红。关键字侧：M10/M11/M12 分别把 `Period`/`Reason`/`SkipChecks` 字样换掉 ⇒ `:78 ... does not contain "Period"/"Reason"/"SkipChecks"`。前置侧：M13 让 validate 无条件报错 ⇒ `:53`（即 `require.NoError(t, ok.validate(), "base 必须合法…")`）红 ⇒ **前置断言确实吃重，不是摆设** | PASS |
| error_handling[1] | `checkEnum` 解前缀后两处 Meta 调用**输出逐字不变**；判据「既有 Meta 测试不得变红」 | `go test -run TestMeta` 全 PASS + 本报告 §3 的独立 A/B 探针 | 见 §3 | PASS（附重要局限） |
| non_functional[0] | RED **因预期原因**失败，实际输出原文入 discovery | discovery `red_step2/7/10_actual_verbatim` + 本报告 §4 独立复现 | 见 §4 | PASS |
| non_functional[1] | gofmt/vet 无输出、整包绿、覆盖率不跌破 80、导出面守卫**登记而非放宽** | §1 + `TestPackageExposesNoWriteFunctions` | M14 伪造一个新导出 `InsertRow` ⇒ `store_test.go:400 expected [...6 项] / actual [... "InsertRow" ...]` ⇒ 断言仍是**全导出面精确相等**，未被改成 `Contains`、未被删除 | PASS |

---

## 3. error_handling[1] 的独立验证（探针已被 dev 删除，故重新自证）

dev 的背对背探针跑完即删 ⇒ **那份证据不可直接复验**。我独立做了一次，方法不同、覆盖更宽。

### 3.1 直接观察：diff 本身就是证明

```
$ git diff 125ad89..cfc24fe -- internal/hestia/types.go
-	return fmt.Errorf("hestia: unknown meta.%s %q (want %s)",
+	return fmt.Errorf("hestia: unknown %s %q (want %s)",
-	if err := checkEnum("caliber_version", ...
+	if err := checkEnum("meta.caliber_version", ...
-	if err := checkEnum("extractor", ...
+	if err := checkEnum("meta.extractor", ...
```

且 `grep -rn "checkEnum(" internal/` 确认 **@125ad89 恰有 2 个调用点**、@cfc24fe 是同样这 2 个
再加 `thresholds.go:90` 一个新调用点 —— 没有第三个 Meta 侧调用点被漏改。

### 3.2 背对背 A/B 探针（观察，不是推理）

自写探针 `zz_probe_test.go`，穷举 `Meta.validate()` 的 **17 条错误路径**（不止 DoD 关心的 2 条），
把逐字输出写文件；在 `/tmp/probe-A @125ad896fb09…`（改动前）与 `/tmp/probe-B @cfc24fe`（改动后）
**背对背**各跑一次：

```
$ diff /tmp/probe-A.txt /tmp/probe-B.txt   → 无输出（exit 0）
$ shasum -a 256 /tmp/probe-A.txt /tmp/probe-B.txt
a87c42d92a3dc2823b773d0a39212c6c65b5b3cdc88ca6972507ffeb3bf496b2  /tmp/probe-A.txt
a87c42d92a3dc2823b773d0a39212c6c65b5b3cdc88ca6972507ffeb3bf496b2  /tmp/probe-B.txt
```

两条关键行（改动前后逐字相同）：

```
bad_caliber_1	hestia: unknown meta.caliber_version "2099-01" (want 2015-01|2023-01|2025-01)
bad_extractor_1	hestia: unknown meta.extractor "bogus@v9" (want rule@v1|rule@v2|llm-fallback@v1)
```

### 3.3 阴性对照（探针自身必须有杀伤力，否则「diff 为空」可能只是无力）

在 probe-B 里把调用点改成 `checkEnum("meta.caliber_versionX", …)`：

```
$ diff /tmp/probe-A.txt /tmp/probe-B-mut.txt
< bad_caliber_1	hestia: unknown meta.caliber_version "2099-01" (...)
> bad_caliber_1	hestia: unknown meta.caliber_versionX "2099-01" (...)
（exit 1）
```

探针有杀伤力 ⇒ §3.2 的「diff 为空」不是平凡为真。（probe-B 随后已还原，`git status` 为空。）

### ⚠ 附带发现：DoD 自带的判据守不住它自己要求的实质

同一次阴性对照里，我顺手跑了 DoD 明写的判据：

```
$ GOTOOLCHAIN=local go test ./internal/hestia/ -run TestMeta -count=1
ok  	github.com/newthinker/atlas/internal/hestia	0.256s
```

**变异后 `TestMeta` 仍然全绿** —— 既有断言是 `assert.Contains(err, "caliber_version")`，
而 `"meta.caliber_versionX"` 仍含该子串。

- 对**本任务判定的影响：无**。DoD 的实质要求是「**本次改动后**输出逐字不变」，
  这一点已由 §3.1 + §3.2 独立证成；DoD 明写的判据也确实满足。
- 但**代码库里不存在任何常驻守卫**能在未来阻止这两行输出漂移。属「守卫在场、参数无守」。
  建议 Leader/QA 记为后续机会（如把两条 golden 错误串钉进 `types_test.go`），**不构成本任务返工**。

---

## 4. non_functional[0]（RED 因预期原因失败）的独立核查

discovery 给出了三段 `*_actual_verbatim` 原文，三段全部是 `undefined: …` 形态，
**无一条**是 `imported and not used`。我另做一次直接观察确认「失败类型」而非采信原文：
在 `/tmp/mut-035` 里把 `thresholds.go` 整个移走再跑整包 —

```
internal/hestia/thresholds_test.go:22:7: undefined: DefaultThresholds
internal/hestia/thresholds_test.go:42:10: undefined: CaliberExemption
internal/hestia/thresholds_test.go:55:44: undefined: Thresholds
FAIL	github.com/newthinker/atlas/internal/hestia [build failed]
```

失败形态与计划预期一致。另核对 `thresholds_test.go` 的 import 块（`slices`/`testing`/`assert`/`require`）
**每一个都在文件里被真正使用** ⇒ 结构上不可能出现 `imported and not used`。文件已还原
（sha256 `0019d78f…ebda`，`git status --porcelain` 为空）。

---

## 5. 未据以判不通过的项

- validator 报 4 条 `TASK-001: scope-writes-outside-packages` —— **AD-035-4 已裁定为本仓库形状级假阳**，
  按派单要求不作为判定依据。validator 整体 `exit=0`，`✓ 任务图校验通过（7 个任务）`。
- discovery `decisions[4]` 申报「未运行 code-simplifier，主动跳过」—— 这是 AD-035-5 下的合规申报，
  **属 Leader 裁量范围，不属 done_criteria**，我不据此判定。

## 6. 主工作区完整性

全部消融只在一次性 worktree（`/tmp/mut-035`、`/tmp/probe-A`、`/tmp/probe-B`）内进行。收尾核实：

```
$ shasum -a 256 internal/hestia/thresholds.go internal/hestia/types.go internal/hestia/store_test.go
0019d78f2535fc0fa8f0163bcb36c1b8bac307293401857882fbd40927efebda  thresholds.go
16d6612e3fc18e57ced38a621108bdd1e61caffef2c522b44a57778c10467a03  types.go
25a12fde7cd4927bd4496608841c7b91cff26f599dd7c8bec293db60536fcd13  store_test.go
$ git status --porcelain   → 仅 .arcforge/ 条目，internal/ 一个字节未动
```

---

## 结论：**VERIFIED**

8 条 done_criteria 全部有对应测试、全部有消融证明断言在守卫、全部核对了致红归因。
两项附带发现（§1 覆盖率基线 0.1pct 出入、§3 DoD 判据强度不足）**均不构成本任务缺陷**，
已在此留档供 QA/Leader 参考。
