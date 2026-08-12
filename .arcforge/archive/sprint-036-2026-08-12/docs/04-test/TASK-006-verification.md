# TASK-006 验证报告 — 口径豁免的键补上 PeriodTypes

- **验证者**：test-agent-25（Reality Checker，默认判定 NEEDS WORK）
- **判定对象**：`verify_baseline.head = cfcbdbb668496da25a6a8dd7cca012258e1d23e7`
- **TASK-006 自身提交**：`0601b63e29d864d766eb77111f7007d5edaceed3`
- **验证 worktree**：`git worktree add --detach /tmp/verify-036-6 cfcbdbb668496da25a6a8dd7cca012258e1d23e7`
- **结论：VERIFIED（8/8 DoD 全部 PASS）**

## 0. 漂移核验（范围外漂移，INFO 级，**未使用 `--ack-drift`**）

验证期间主仓库 HEAD 从 `cfcbdbb` 前进到 `0597fcaae306d299aea593957b696d69770641c4`：

```
$ git log --oneline cfcbdbb..HEAD
0597fca Merge task/TASK-002: index 快照与分页模板解析
99c8c6a feat(hestia): 分页模板解析与 index 页快照

$ git diff --stat cfcbdbb..HEAD -- internal/hestia/thresholds.go internal/hestia/validate.go \
                                    internal/hestia/thresholds_test.go internal/hestia/validate_test.go
（空 —— TASK-006 声明范围内零改动）
```

漂移全部落在 TASK-006 的 `writes` **之外**（TASK-002 的 `discover.go` / `testdata/`）⇒ 按
CLAUDE.md 的收窄判据属 **INFO 级放行**，**本次未使用 `--ack-drift`**。

discovery 零漂移：`shasum -a 256 .arcforge/discoveries/TASK-006.json` = `558709d0eb8075…`
== `verify_baseline.discovery_sha256`，逐字相同。

范围核验：`git show --numstat 0601b63` → `thresholds.go 38/7`、`thresholds_test.go 151/12`、
`validate.go 1/1`、`validate_test.go 21/11`，与 `writes` **四项逐项一致，无越界**。

## 1. DoD 逐条覆盖矩阵

| # | DoD 条目 | 对应测试 | 承重证据 | 判定 |
|---|---|---|---|---|
| F1 | 只声明 `[annual]` 时 monthly/h1 **都**返回 nil | `TestExemptionIsolatesPeriodType` | M1 KILLED | **PASS** |
| F2 | 一条豁免可覆盖多种 period_type | `TestExemptionCanCoverMultiplePeriodTypes` | M2 KILLED | **PASS** |
| B1 | 空切片报错含 `PeriodTypes`，**理由写进注释** | `TestExemptionRejectsBadPeriodTypes/为空` | M3、M7 KILLED；注释见 `thresholds.go:107-112` | **PASS** |
| B2 | `PeriodTypes` 不是新的宽度绕过路径（三子判据） | `TestAllPeriodTypesIsNotAWidthBypass` ×3 | M9、M10 KILLED（**只被新增这组杀**）；M8 **只被既有组杀** | **PASS** |
| E1 | 未知取值报错含**非法值本身**与合法取值列表；须查 `validPeriodTypes` 不用 `checkEnum` | `TestExemptionRejectsBadPeriodTypes/含未知取值` | M4、M5、M6、M11 KILLED | **PASS** |
| N1 | 改动顺序：先签名后字段 | RED 独立复现（§4） | — | **PASS** |
| N2 | `mutate` 一并 Clone `PeriodTypes`；每条转红须因**自己的判据**红 | 代码在场；M13 KILLED | 见 §5 观察 1 | **PASS** |
| N3 | RED / gofmt / vet / 整包绿 / 覆盖率 ≥ 92.1% / 不打红导出面守卫 | §2 | — | **PASS** |

## 2. N3 的命令与输出

在判定对象 `cfcbdbb` 的隔离 worktree 上：

```
$ GOTOOLCHAIN=local go vet ./internal/hestia/   → 无输出，exit 0
$ gofmt -l internal/hestia/                     → 无输出，exit 0
$ GOTOOLCHAIN=local go build ./...              → 无输出，exit 0
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  github.com/newthinker/atlas/internal/hestia  0.883s  coverage: 92.3% of statements
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -race
ok  github.com/newthinker/atlas/internal/hestia  3.519s
```

覆盖率 **92.3% ≥ 92.1%**。导出面守卫 `TestPackageExposesNoWriteFunctions` /
`TestStoreExposesNoWriteMethods` **均未出现在任何 FAIL 中** ⇒ 确认本任务不新增导出物、
`writes` 不含 `store_test.go` 是对的。

**集成回归（当前 HEAD `0597fcaa`，含 TASK-002）**：

```
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  github.com/newthinker/atlas/internal/hestia  0.494s  coverage: 92.5% of statements
```
TASK-006 相关的 11 个测试函数在最新树上**逐个 PASS**（`TestExemptionIsolatesPeriodType`、
`TestExemptionCanCoverMultiplePeriodTypes`、`TestExemptionRejectsBadPeriodTypes`、
`TestAllPeriodTypesIsNotAWidthBypass`、`TestThresholdsRejectWholePeriodSkip`、
`TestCaliberExemptionRecordsSkipNotPass`、`TestCaliberExemptionDoesNotLeakToOtherPeriods`、
`TestExemptionRejectsUnknownCheckID`、`TestReportKeepsEveryGateUnderExemption` 等）
⇒ 与 TASK-002 的合并**无接口冲突**。

## 3. 变异/消融独立复验（harness 自写，未复用 dev 的）

Harness：`scratchpad/test25-TASK-006-ablation.sh`，锚点 `ARCFORGE_MUT_REF` 可覆写，
默认**全 sha** `cfcbdbb…`，打印锚点与主仓库工作树 HEAD。变异作用在
`git worktree add --detach /tmp/mut-036-6` 的隔离树上。

四道闸全部内建并通过：**基线闸**（未变异全绿，`--- PASS` = 499）、**生效闸**（diff 非空 +
逐条打印 diff 原文）、**编译失败闸**（`go test -c -o /dev/null`，0 命中）、
**计数自证 12 == 12 → OK**。

| 变异 | 结果 | 实测死因 |
|---|---|---|
| M1 `exemptionFor` 不再匹配 `PeriodTypes`（回到旧行为） | KILLED | `TestExemptionIsolatesPeriodType`：`Expected nil, but got: &CaliberExemption{... PeriodTypes:["annual"] ...}`，Messages「只声明了 annual，monthly 不该被命中」 |
| M2 只认 `PeriodTypes[0]`（堵过头） | KILLED | `TestExemptionCanCoverMultiplePeriodTypes`：`Expected value not to be nil`，`period_type=h1` / `period_type=annual` |
| M3 删掉空切片校验 | KILLED | `/为空`：`An error is expected but got nil.` |
| M4 删掉未知取值校验 | KILLED | `/含未知取值`：`An error is expected but got nil.` |
| M5 错误文案不再指出是哪个值（去掉 `%q`） | KILLED | `"…含未知取值 (want monthly\|h1\|annual)" does not contain "quarterly"` |
| M6 错误文案不再列出合法取值（换成「见白名单」） | KILLED | `… does not contain "h1"` / `"annual"`，Messages「文案已与白名单脱节」 |
| M7 空切片文案去掉「留空不等于」 | KILLED | `… does not contain "留空不等于"`，Messages「必须由『为空』那条规则拒绝，而不是被『含未知取值』冒名满足」 |
| **M8** 宽度校验**收紧**成「仅覆盖三种才算整期跳过」 | KILLED | **只被既有** `TestThresholdsRejectWholePeriodSkip/枚举全部闸门即整期跳过` 杀 |
| **M9** 宽度校验改成「覆盖三种就放行」 | KILLED | **只被新增** `TestAllPeriodTypesIsNotAWidthBypass/三种序列 + 全部闸门：仍然被拒` 杀 |
| **M10** completeness 规则改成「覆盖三种就放行」 | KILLED | **只被新增** `TestAllPeriodTypesIsNotAWidthBypass/三种序列 + completeness：仍然被拒` 杀 |
| **M11【绊线】** 白名单加第四种取值 `weekly` 而文案没跟上 | KILLED | `… does not contain "weekly"`，Messages「说明文案已与白名单脱节」 |
| M12 `mutate` 闭包去掉 `slices.Clone(base.PeriodTypes)` | **SURVIVED** | 见 §5 观察 1 |
| **M13**（补充）删掉 `SkipChecks` 的 `checkEnum` | KILLED | `TestExemptionRejectsUnknownCheckID` —— 见 §4 |

### M8 / M9 / M10 的分工是本任务最强的证据

三条变异的失败测试集合**互不相交**：M8 只打红既有的 `TestThresholdsRejectWholePeriodSkip`，
M9/M10 只打红新增的 `TestAllPeriodTypesIsNotAWidthBypass`。⇒ **dev 声称的「新增那组不是对既有
断言的冗余确认，它堵的是既有测试够不着的方向」独立成立**：既有测试只能抓住「收紧」方向，
新增那组抓的是「放宽」方向，两者是不同的失效模式。

且 M8 的死因原文特别值得记：它红在
`"…不得豁免 completeness…" does not contain "跳过了全部"`
—— 打红它的是「**两种形态的错误信息要能分辨**」那条断言，而不是「有没有报错」。

### 主工作区完整性（双重核实，变异窗口内 + 收尾）

```
662a2bef6bb66afbf255412c8c6b73ff18333eed3c0900c88266c58851a1ef18  internal/hestia/thresholds.go
596bbec0f2a7782fd47a40a2ac338787f115f7db2526f21fa53a9484194d3f4f  internal/hestia/validate.go
b81b5284b60a5b9eb18db8fcef71b5657eedfa3276a2f660559156dc041b13e3  internal/hestia/thresholds_test.go
77df02725f199f34154d34157643120f248d003c686b5ef234887c49b1e86df7  internal/hestia/validate_test.go
16d6612e3fc18e57ced38a621108bdd1e61caffef2c522b44a57778c10467a03  internal/hestia/types.go
$ git status --porcelain   → 与变异前逐字相同
```
`thresholds.go` 的 sha256 与 dev discovery 记录的值逐字相同 ⇒ 两轮变异均未污染主工作区。
变异树收尾 sha256 一致（`OK`）；`/tmp/mut-036-6`、`/tmp/verify-036-6` 均已 remove + prune。

## 4. RED 复现与「红的理由对不对」

**N1（改动顺序）独立复现**：把生产侧两文件回退到改动前（`f5a17d5`）、测试文件保留交付版：

```
internal/hestia/thresholds_test.go:67:3:  unknown field PeriodTypes in struct literal of type CaliberExemption
internal/hestia/thresholds_test.go:81:6:  ex.PeriodTypes undefined (type CaliberExemption has no field or method PeriodTypes)
internal/hestia/thresholds_test.go:121:48: too many arguments in call to cfg.exemptionFor
	have (string, string, string)
	want (string, string)
```
与 dev 记录的 RED 原文**逐字同形**：字段与三参签名都还不存在，**因预期原因失败**，
未被 `imported and not used` 之类污染。

**R8 那条（「测试红了，但红的理由与它要守的东西无关」）我做了独立正向验证**：
DoD `non_functional[1]` 要求确认每条转红的用例是因自己的判据红、而非被新校验抢先返回。
`TestExemptionRejectsUnknownCheckID` 的 `PeriodTypes` 现在是合法的 `["monthly"]`，所以走得到
`SkipChecks` 的 `checkEnum`。补充变异 **M13**（删掉 `SkipChecks` 的 `checkEnum`）→ **KILLED，
唯一失败测试正是 `TestExemptionRejectsUnknownCheckID`** ⇒ 它现在确实因**自己的判据**而绿。

`validate_test.go:212`（`TestValidateRejectsInvalidConfig`）确认**未被改动**（不在
`0601b63` 的 diff 内），与 reviewer 的判断一致：`Version` 的 `checkEnum` 排在新增的
`PeriodTypes` 检查之前，先行返回。

## 5. 观察项（不影响判定）

1. **M12 SURVIVED：`ex.PeriodTypes = slices.Clone(base.PeriodTypes)` 在场但无守卫。**
   DoD `non_functional[1]` 明写「`PeriodTypes` 也必须一并 Clone」，代码**确实加了**
   （`thresholds_test.go:81`），DoD 条目按「在场」判 PASS。但 `TestThresholdsRejectMalformedExemptions`
   的 `mutate` 表四个用例分别改 `Version`/`Period`/`Reason`/`SkipChecks`，**没有一个改
   `PeriodTypes`** ⇒ 删掉这行 Clone 当前不会有任何测试转红。
   这是**防御性措施而非承重守卫**：它防的是「**未来**有人往表里加一条改 `PeriodTypes` 的用例」
   时的用例间污染。dev 在 discovery 里明确决定不加那一行（理由是与
   `TestExemptionRejectsBadPeriodTypes` 重复），该理由成立；但两个决定合起来的净效果是
   **这行 Clone 目前没有任何东西守着它**。建议记入 wisdom，不建议为此返工。

2. **任务文件原本缺 `discovery` 字段。** 转 `verified` 前 `jq 'has("discovery")'` 返回 `false`
   （TASK-001 同期是 `true`）。因为 validator 的 `missing-discovery` 是**阻断级且只在
   `status == verified` 时触发**，而进 `verified` 后只剩 verifier 能写该文件 —— 我在转之前
   经写通道 `task TASK-006 update --expect-epoch 1 --field discovery=…` 补上并 `jq` 直读核实。
   **`.arcforge/discoveries/TASK-006.json` 文件本身一直存在**（`verify_baseline.discovery_sha256`
   就是它的 sha），缺的只是任务文件里的指针字段。**建议 Leader 检查其余任务是否同样缺失**
   ——TASK-001 有而 TASK-006 没有，说明这个字段不是写通道自动维护的。

3. 合法取值 `monthly|h1|annual` 在错误文案里是**第三份硬编码副本**。dev 用
   `for pt := range validPeriodTypes` 的遍历断言做绊线，**M11 实测证明这条绊线有效**
   （加第四种取值 `weekly` 立刻红）。另外我按判据 3 检查过遍历型断言的**空集平凡为真**风险：
   该循环若遍历空集会平凡为真，但同一子测试里还有钉具体值的
   `assert.Contains(err.Error(), "quarterly")` 与 `require.Error` 作为肯定式锚点，
   两者互补 ⇒ 不构成平凡为真。

## 6. 结论

**VERIFIED。** 8 条 done_criteria 逐条有对应测试、逐条有消融证据；13 条变异中 **12 条 KILLED、
1 条 SURVIVED**（M12，DoD 要求的防御性 Clone，非承重守卫，已记为观察）；四道闸全通过、
计数自证 12==12；RED 独立复现且信号正确；主工作区零污染；判定对象与交付物在声明范围内零漂移。
