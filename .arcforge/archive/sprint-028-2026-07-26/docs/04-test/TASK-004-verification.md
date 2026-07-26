# TASK-004 验证报告（refresh Degraded/Failed 明细始终打印到 stdout）

- 验证者: test-agent-6
- 交付 commit: ef541b7
- 承接时 assignment_epoch: 1
- **判定: VERIFIED（全部 5 条 done_criteria 通过）**

## 1. 实跑证据

```
GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1
ok  github.com/newthinker/atlas/cmd/atlas  1.209s  coverage: 74.4% of statements

go tool cover -func → runPrismRefreshWith  100.0%   （本次改动的函数）
                     runPrismRefresh         0.0%   （cobra 包装，需真实配置/网络）
git status --porcelain cmd/atlas → 空
```

## 2. done_criteria 逐条覆盖矩阵

| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| F0 | sender 非 nil 且 Report 含 Failed/Degraded → out 同时含两段明细 | `TestPrismRefreshAlwaysPrintsDetail`：断言 out 含汇总行 + `AAPL: edgar 404` + 完整 Degraded 文本；并用 `require.Len(sender.sent,1)` + 内容断言锁定「打印**不取代**通知」 | **PASS** |
| F1 | sender 为 nil 时行为不变：明细进 out 且返回 nil | `TestPrismRefreshDetailWithoutSenderUnchanged`：`NoError` + 两段明细 + `assert.Empty(errOut)`（正负成对） | **PASS** |
| B0 | Failed 与 Degraded 皆空时只打印汇总行，不打印明细段（负向） | `TestPrismRefreshNoDetailSectionWhenClean`：用 **`assert.Equal` 比对整个 out 字符串** 等于 `"prism refresh: 7 ok, 0 failed, 0 degraded\n"` —— 比 `NotContains` 强得多，任何多余输出都会被捕获；另断言 `sender.sent` 为空 | **PASS** |
| E0 | SendText 返回错误时仍只向 errOut 输出 warning 且返回 nil（既有语义零变更） | `TestPrismRefreshPrintsDetailEvenWhenSendFails`：`NoError` + out 含明细 + errOut 含 `warning` 与 `telegram down` | **PASS** |
| N0 (test) | go test 全绿；既有 9 个测试零修改 | 见 §1；「零修改」**机制性核实**：`git show ef541b7 -- cmd/atlas/prism_test.go \| grep -c "^-[^-]"` → **0 行删除**，即 diff 纯增量。改动前 `func TestPrismRefresh*` 计 8 个 + `TestHasEdgarInstrument` = DoD 所称的 9 个，现为 12 个（8 + 新增 4），既有测试原样保留 | **PASS** |

## 3. 实现审查

改动仅 3 行（+2/−1），完全 surgical：

```go
	msg := strings.Join(parts, "\n")
+	// 明细无条件进日志:配置了 telegram 时也要留下 stdout 痕迹,否则通知一挂就没有观测。
+	fmt.Fprintln(d.out, msg)
	if d.sender == nil {
-		fmt.Fprintln(d.out, msg)
		return nil
	}
```

与任务描述要求的「msg 组装移到 sender 判断之前，`fmt.Fprintln(d.out, msg)` 无条件执行」逐字对应。
汇总行、`len(Failed)==0 && len(Degraded)==0` 提前返回、SendText 失败仅 warning 并返回 nil
三处既有语义均未触碰。无附带改动、无顺手重构。

## 4. 变异测试（验证断言非空洞）

在 `d1d86ba` 的一次性 worktree 上破坏关键行为：

| 变异 | 结果 |
|---|---|
| M1 把 `Fprintln` 挪回 `sender==nil` 分支内（即回退本任务改动） | `AlwaysPrintsDetail` + `PrintsDetailEvenWhenSendFails` **FAIL** ✓ |
| M2 删掉「Failed/Degraded 皆空提前返回」 | `NoDetailSectionWhenClean` + 既有 `AllSuccessNoAlert` **FAIL** ✓ |
| M3 SendText 失败改为 `return err` | `PrintsDetailEvenWhenSendFails` + 既有 `WarnsOnSendFailure`×2 **FAIL** ✓ |
| M4 使 SendText 永不被调用 | `AlwaysPrintsDetail` **FAIL** ✓（详见 §5） |

还原后复跑 `ok`。**M1 直接证明本任务的核心断言真能守住这次改动**，不是形式存在。

## 5. 一处既有测试脆弱点（非本任务引入，但会掩盖回归）

M4 变异在**全包**运行时只显示既有的 `TestPrismRefreshNotifiesOnPartialFailure` 失败，
新测试 `AlwaysPrintsDetail` 未出现在 FAIL 列表——我没有就此下结论，而是单独重跑该测试，
确认它**确实捕获**了 M4（`"[]" should have 1 item(s), but has 0`，打印不取代通知）。

全包模式下被掩盖的根因是既有测试 `cmd/atlas/prism_test.go:64-65` 的写法：

```go
assert.Len(t, sender.sent, 1)        // 非致命，失败后继续
assert.Contains(t, sender.sent[0], "000300.SH")   // 空切片 → index out of range，panic
```

`assert.Len`（非致命）后紧跟 `sender.sent[0]` 索引，切片为空时 **panic 中断整个测试二进制**，
其后所有测试根本不会执行，结果被整体掩盖。经 `git show ef541b7^:cmd/atlas/prism_test.go`
比对，该写法在改动前**已存在**，非 TASK-004 引入。

值得一提的是本任务新增的 `AlwaysPrintsDetail` 用的是 **`require.Len`**（致命），正是正确写法。

**建议（非 DoD 强制，属既有技术债）**：把 `prism_test.go:64` 的 `assert.Len` 改为 `require.Len`。
一行改动，可避免未来任何使 `sender.sent` 为空的回归掩盖整包测试结果。归属由 Leader 判断。

## 6. 门禁预警的核实（Leader 关注项）

Leader 在任务描述中预警：`cmd/atlas` 整包覆盖率 74.6% < `dev_minimum` 80，会导致
`transition dev_done` 被门禁 DENY，属框架已知缺陷。**我独立核实该预警准确，且责任不在本任务**：

- 改动**后**：包 74.4% / `go tool cover` total 74.6%，`runPrismRefreshWith` **100.0%**
- 改动**前**（worktree @ `ef541b7^`）：包 74.4% / total **74.6%**，`runPrismRefreshWith` 已 **100.0%**

**两者逐位相同** —— 本任务既未改善也未恶化整包覆盖率；低于 80 完全由 `cmd/atlas` 其它文件
与无法单测的 cobra 包装 `runPrismRefresh`（0%）造成。dev 按 Leader 指示提供文件级证据并
请求代为放行的处置正确，未越权改动 `arcforge.config.json` 或 `.claude/`（我核对工作树与提交，
两者均无相关改动）。

## 7. 判定

**VERIFIED** —— 5 条 done_criteria 全部通过；改动极简且不触碰既有语义；4 项变异测试证明
断言有真实咬合力；「既有 9 个测试零修改」经 diff 零删除行机制性证实。无需返工。
