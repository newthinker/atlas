# TASK-002 验证报告：Snapshot 展开 state label 为 <name>_<state> 键

- 验证者：test-m4-a
- 判定对象：master @ `717c627d09f35251c247ad0fc5f2d0f79322745c`（= verify_baseline.head）；实现提交 `8d13abb16d9272db5a34f09d138e2e9dfde37eab` 是其祖先，`git diff --stat 8d13abb..717c627 -- internal/metrics/` 为空（合入后该包无其他改动）
- discovery sha256：`40c65f47cb14ee076f507f15de1e3187a37520e444cd5bf2b417b05822fed12a`（与 baseline 一致）
- 运行环境：独立 worktree `../wt-verify-TASK-002`（detached @ 上述全 sha），GOTOOLCHAIN=local
- 结论：**VERIFIED**（8/8 条 done_criteria 通过）

## 一、验证者亲自运行的证据

| 检查 | 结果 |
|---|---|
| `go test ./internal/metrics/... -count=1 -cover` | ok，coverage 99.2% |
| `go vet ./internal/metrics/` | 通过 |
| 下游 `go test ./internal/alert/... ./cmd/atlas/...` | 均 ok |
| `git diff --numstat 8d13abb^ 8d13abb` | snapshot.go 28/1，snapshot_test.go 126/0 |
| test 文件 hunk | 唯一一段 `@@ -168,0 +169,126 @@`，位于文件末尾既有 `TestSnapshot_GatherError_NoPanic` 之后 |
| `gofmt -l` | snapshot_test.go 被列出；`gofmt -d` 差异位于第 83–93 行（既有 `TestSnapshot_StatusClassKeys` 区域），父提交 `8d13abb^` 的同文件同样被 gofmt 列出 ⇒ 既有问题，非本任务引入（discovery 已如实申报） |
| 生产代码 `"state"` label 现存用法 | `grep -rn '"state"' --include='*.go' internal cmd`（排除 _test）仅命中本次新增的 snapshot.go:94 ⇒ 无既有键冲突 |

## 二、done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | gauge failed=2/done=5 ⇒ x=7、x_failed=2、x_done=5；status=500 与 state=failed 同在 ⇒ x_5xx 与 x_failed 都产生 | `TestSnapshot_StateGaugeExpands`（三值断言 + 键集合精确）；`TestSnapshot_StateAndStatusBothExpand`；实现为独立函数 `addState`，与 `addStatusClass` 并列调用 | PASS |
| functional[1] | counter y failed=3 ⇒ y=3、y_failed=3 | `TestSnapshot_StateCounterExpands`；变异 V7 被杀 | PASS |
| functional[2] | 既有测试函数体零改动且全绿 | review：test 文件 numstat -0、hunk 仅末尾追加；既有测试随包全绿 | PASS |
| boundary[0] | 无 label gauge z ⇒ 键集合精确等于 {z} | `TestSnapshot_NoStateLabel_NoExtraKeys`（`assertKeys` 比长度 + 逐键） | PASS |
| boundary[1] | 同 state 多序列（shard 区分）⇒ 求和 | `TestSnapshot_StateSameValueSums`（2+6=8）；V5（`+=`→`=`）被杀 | PASS |
| boundary[2] | 空串 / `in-flight` / 含空格 ⇒ 不产生 `<name>_` 键 | `TestSnapshot_StateInvalidValue_NoKey` 三个子测试；V1（去锚）、V2（`\w*` 放行空串）、V3（`\S+`）各自被对应子测试杀死 ⇒ 空串用例确实走到了正则（dto 中保留空值 label），非空洞 | PASS |
| error_handling[0] | histogram 带 state ⇒ 只有 `_count`/`_sum` | `TestSnapshot_HistogramWithState_NoStateKey`（键集合精确）；验证者变异「histogram 分支补调 addState」被杀 | PASS |
| non_functional[0] | 注释写明立项理由 + Snapshot 函数注释补 state 展开 | review：`stateValueRe` 注释（求值器只认 `\w+`）；`addState` 注释（求值器无 label 选择器、snapshot 跨 label 求和、2026-09-17 human ruling、与 addStatusClass 分离的原因）；`Snapshot` 函数注释新增 state 展开一句且写明 base 键保留 | PASS |

## 三、验证者独立变异（本人专用 worktree，每个变异后还原并比对 sha256，最终 `git status --porcelain` 0 行）

对照组全绿；10 个变异全部 KILLED，每个均 `go vet` 通过（排除编译失败致红）：

| 变异 | 杀死它的测试 |
|---|---|
| V1 正则去锚 | StateInvalidValue_NoKey/hyphen、/space |
| V2 `\w*` | StateInvalidValue_NoKey/empty |
| V3 `^\S+$` | StateInvalidValue_NoKey/hyphen |
| V4 label 名 state→status | 4 个 State 用例 + 既有 StatusClassKeys 等 |
| V5 `+=`→`=` | StateSameValueSums |
| V6 gauge 漏调 addState | StateGaugeExpands 等 3 个 |
| V7 counter 漏调 addState | StateCounterExpands |
| V8 去掉 `_` 分隔符 | 4 个 State 用例 |
| V9 带 label 序列不再累加 base 键 | StateGaugeExpands、StateInvalidValue_NoKey |
| V10 histogram 分支补调 addState | HistogramWithState_NoStateKey |

## 四、非阻断观察

1. state 值与其他派生后缀重名时会与既有键合并累加：例如 gauge `x` 的 state=`count` 产生 `x_count`，state=`5xx` 与 status 分类键 `x_5xx` 相加。DoD 未要求处理，当前生产代码无其他 `state` label，无现实影响；将来给 histogram 同名指标或带 status 的指标加 state label 时需留意。
2. snapshot_test.go 既有 gofmt 不合规（第 83–93 行）仍在，属既有问题，未在本任务范围。
