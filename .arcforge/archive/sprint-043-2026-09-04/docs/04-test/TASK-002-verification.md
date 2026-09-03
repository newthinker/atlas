# TASK-002 验证报告 · saveSnapshot：同字节跳过、改稿另存不覆盖、改稿后重抓幂等

- 验证者：**test-m1d-c**（第 2 轮，接手 test-m1d-b）　承接 epoch：**3**　rework_count：1　判定：**VERIFIED**
- `verify_baseline.head`：`290370feaa7e69b1eb533ca3c0a0683000853de4`；当前 master HEAD 相同 ⇒ **无对象漂移**，无需 `--ack-drift`
- `verify_baseline.discovery_sha256`：`1d2d8e3c8ce6197092c378814103173d8ef74745e9583308c424569a6891876a`；实测 `.arcforge/discoveries/TASK-002.json` sha256 逐字符相同 ⇒ **discovery 未漂移**
- 返工 commit：`d24197853f995538dfa8abe2a1433b4b6c5bc99e`（父 `f3d6eb282c83e0ca730b1713907f5220114ee86b`，分支 `task/TASK-002-m1d-fix`，merge `290370f`）
- 验证树：`git worktree add --detach ../wt-verify-TASK-002-m1d-c 290370feaa7e69b1eb533ca3c0a0683000853de4`；变异在同一棵隔离树上做，每个窗口内 + 收尾各校验一次主仓库两文件 sha256 与 `git status --porcelain`，四次全部一致

## 1. 结论

QA 的两条 A 组缺陷（A3 改稿后幂等失效、A8 时区断言缺口）均已闭合，且**原 8 条 done_criteria 一条未回退**。三条变异全部 KILLED，其中两条独立复现了 Leader 的复核结论、一条为验证者自加。另用一条一次性探针在被验树上复现了 QA 的原始四次调用形态，得 `written/diverged/unchanged/unchanged` 与目录恰 2 文件（返工前 QA 实测为 4 文件）——**缺陷本身在现场可复现地消失了**，而不是仅靠新增测试自证。门禁全绿：`internal/hestia` 96.5%（门槛 96.3%）、`cmd/atlas` 76.3%（门槛 75），`snapshot.go` 三个函数逐函数覆盖率 100%。

## 2. Done Criteria 覆盖矩阵（原 8 条，全部 `verify_by: test`）

全量套件 `GOTOOLCHAIN=local go test ./internal/hestia/... ./cmd/atlas/... -count=1 -v` 于 `290370f`：**exit=0，964 PASS / 0 FAIL / 1 SKIP**。唯一 SKIP 是 `TestMagnitudeRangesCoverEveryFieldWhenCalibrated`，与本任务无关。

| # | 完成标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | 首次落盘路径固定 `<dir>/<id>.html`、`snapshotWritten`、内容逐字节相同 | `TestSaveSnapshotWritesNewFile` PASS | PASS |
| functional[1] | 目录不存在时 `MkdirAll(dir, 0o755)` 自建 | `TestSaveSnapshotCreatesDir` PASS | PASS |
| functional[2] | 同 id 同字节 ⇒ `snapshotUnchanged`、返回原 path、**mtime 不变**；判等用 `bytes.Equal` 且理由写注释 | `TestSaveSnapshotUnchangedKeepsMtime` PASS（`Chtimes` 到 2020-01-01 后重跑，`ModTime().Equal(old)`）；`snapshot.go:42` 注释写明「两者判等语义相同，前者少一处能算错的地方」 | PASS |
| boundary[0] | 同 id 不同字节 ⇒ `snapshotDiverged`、**不覆盖**、另存 `a1.20260904T083015Z.html`，两版都在 | `TestSaveSnapshotDivergedKeepsBothVersions` PASS（断言路径逐字符相等，并回读两版内容） | PASS |
| error_handling[0] dir 不可用 | `MkdirAll` 失败被 `snapshot dir %s: %w` 包住 | `TestSaveSnapshotFailsLoudlyWhenDirUnusable` PASS | PASS |
| error_handling[0] R-002 | `writeAtomic` 的 `WriteFile` 失败分支有测试，错误含 `snapshot write` | `TestSaveSnapshotFailsWhenDirReadOnly` **PASS 而非 SKIP**（本机非 root，已在 `-v` 输出中核对），首次落盘与改稿另存两路各断言一次 | PASS |
| error_handling[0] read 默认分支 | 非 `ErrNotExist` 的读失败 ⇒ `snapshot read %s: %w` | `TestSaveSnapshotFailsWhenExistingPathIsDir` PASS（EISDIR） | PASS（DoD 允许不测，实际已测） |
| error_handling[0] rename 失败 | `os.Remove(tmp)` 后返回 `snapshot rename %s: %w` | `TestWriteAtomicRenameFailureRemovesTmp` PASS（rename 到已存在目录，并断言 `.tmp` 已不存在） | PASS（DoD 允许不测，实际已测） |
| non_functional[0] | 两条守卫保持绿**且断言不改**；本文件两函数非导出 | `TestPackageExposesNoWriteFunctions`、`TestFieldNamesAppearOnlyInFieldsGo` 均 PASS；返工 diff 只含 `snapshot.go`/`snapshot_test.go`，守卫测试文件一个字节未动；新增的 `findSnapshotCopy` 为非导出，守卫仍绿 | PASS |
| non_functional[1] 门禁 | gofmt 两欠账 / vet 0 / 两包绿 / 覆盖率 ≥ 96.3% / 五个不动文件 / 无新依赖 / 注释前缀 | 见 §3 门禁表，逐条实测 | PASS |
| non_functional[2] 交付流程 | worktree / 提交锚 / merge 先于 dev_done / merge 后重采 / discovery | 提交 subject 匹配 `^[a-z]+\(TASK-002\):`（实测 grep -c = 1）；merge `290370f` 提交时刻 **2026-09-03T20:23:57Z** 早于 `dev_done` **2026-09-03T20:38:20Z**；`git worktree list` 无 `wt-TASK-002-m1d-fix` 残留 | PASS |

## 3. fix_items 覆盖矩阵（本轮返工）

| fix_item | 对应测试 / 证据 | 判定 |
|---|---|---|
| **A3** 字节不同后再与 `<id>.*.html` 逐个 `bytes.Equal`，命中即 `snapshotUnchanged`（`Path` 指向命中副本）；补三次调用测试 v1/v2/v2 ⇒ `written/diverged/unchanged` 且目录恰 2 文件 | `TestSaveSnapshotDivergedTwiceIsUnchanged` PASS：三次调用 Kind 逐条断言，`r3.Path == r2.Path`，`os.ReadDir` 长度 2。实现见 `snapshot.go:61-67`（default 分支内先查重）与新增 `findSnapshotCopy`（`snapshot.go:78-94`） | PASS |
| **A3** 查重的错误路径不得降级为「没有副本」 | `TestSaveSnapshotFailsOnUnglobbableID` PASS（`a[1` ⇒ err 含 `snapshot glob`）；`TestSaveSnapshotFailsWhenCopyIsDir` PASS（副本位置是目录 ⇒ err 含 `snapshot read`） | PASS |
| **A8** Diverged 用例改传非 UTC 时区的 `now`，断言文件名仍 `…T083015Z` | `TestSaveSnapshotDivergedKeepsBothVersions` 的 `now` 已改为 `snapNow.In(time.FixedZone("CST", 8*3600))`（`snapshot_test.go:83`），断言路径 `a1.20260904T083015Z.html`。若实现漏 `.UTC()`，本机东八区会得 `163015` ⇒ 断言不成立（变异 M2 实测转红） | PASS |
| 非功能：只改两个文件、全包绿、覆盖率 ≥ 96.3%、提交锚、走完整交付流程 | 见 §2 non_functional 两行与下表 | PASS |

### 门禁实测（全部钉在 `290370feaa7e69b1eb533ca3c0a0683000853de4`）

| 项 | 命令 | 实测 |
|---|---|---|
| gofmt | `gofmt -l internal/hestia cmd/atlas` | 恰 `cmd/atlas/backtest_test.go`、`cmd/atlas/crisis_test.go` 两行，无新增项 |
| vet | `go vet ./internal/hestia/... ./cmd/...` | 零输出，rc=0 |
| 测试 | `go test ./internal/hestia/... ./cmd/atlas/... -count=1 -v` | exit=0；964 PASS / 0 FAIL / 1 SKIP |
| 覆盖率 | 同上加 `-cover` | `internal/hestia` **96.5%**（≥ 96.3%）、`cmd/atlas` **76.3%**（≥ 75） |
| 逐函数覆盖 | `go tool cover -func` | `saveSnapshot` / `findSnapshotCopy` / `writeAtomic` 均 **100.0%** |
| 五个不动文件 | `git diff --stat 4916106 HEAD -- internal/hestia/{store,validate,parse,extract,fields}.go` | 0 行输出 |
| 依赖 | `git diff --stat ae088eb HEAD -- go.mod go.sum` | 0 行输出 |
| 命名测试 | `-run 'TestSaveSnapshot|TestWriteAtomic'` | 11 条全 PASS（含返工新增 3 条） |

## 4. 变异测试（隔离 detached worktree，3/3 KILLED）

每个变异窗口内落盘后先过**语义闸**（打印变异体与原文的 unified diff，逐行核对）与**语法闸**（`gofmt -e`，三次 rc 均为 0，无编译错误行），跑完再校验一次主仓库指纹。变异脚本用 `python3` 做精确文本替换，替换前断言原文出现次数恰为 1（否则中止）。

| 变异 | 期望 | 实测 |
|---|---|---|
| **M1** 删掉 `saveSnapshot` default 分支里的 `findSnapshotCopy` 调用块（等价于恒返回未命中，即返工前的行为） | 三条转红 | **KILLED**：恰 3 红 —— `TestSaveSnapshotDivergedTwiceIsUnchanged` / `TestSaveSnapshotFailsOnUnglobbableID` / `TestSaveSnapshotFailsWhenCopyIsDir`，与 Leader 复核结论逐条一致，无额外连带红 |
| **M2** `now.UTC().Format(...)` → `now.Format(...)` | A8 那条转红 | **KILLED**：恰 1 红 —— `TestSaveSnapshotDivergedKeepsBothVersions` |
| **M3**（验证者自加）命中副本时返回 `Path: path` 而非 `Path: hit` | 若只断言 Kind 则存活 | **KILLED**：恰 1 红 —— `TestSaveSnapshotDivergedTwiceIsUnchanged`，说明 `r3.Path == r2.Path` 这条断言确实在守卫「Path 指向命中副本」，不是装饰 |
| 原状（无变异） | 全绿 | 964 PASS / 0 FAIL，exit=0 |

**主工作区指纹**：`internal/hestia/snapshot.go` = `1df97426f3cb53326f54e7714dd02624dfd3a15050ddb3088c41952bb7d3f6ce`、`snapshot_test.go` = `a7d450cada7ece7d1f6f74b79204039a1ec06414c819c2928a6838775084831e`，三个变异窗口内各校验一次 + 收尾一次，**四次全部一致**；验证树收尾 `git status --porcelain` 为空。

## 5. 独立探针：QA 原始形态在现场复现

新增测试自证「已修」是必要不充分的——它可能只测了一个与 QA 观测不同的形态。故在验证树临时加了一个一次性探针文件（跑完即删，`porcelain` 已核实为空），**按 QA 报告里的原始四次调用形态** v1/v2/v2/v2 逐次调用 `saveSnapshot`（每次 `now` 递增一天）：

```
kinds=[written diverged unchanged unchanged] files=2 names=[a1.20260905T083015Z.html a1.html]
```

QA 返工前实测为 `written/diverged/diverged/diverged` + 4 文件。**第三、四次都命中副本查重，目录停在 2 文件**，与交付测试的三次调用形态结论一致且更进一步。

## 6. provenance 核对（三人接力）

discovery 的 `rework[0].provenance` 声称：代码由 **dev-m1d-b** 编写（停机前未提交）、**dev-m1d-e** 逐条核实 fix_items 并跑变异后原样提交为 `d241978`、Leader 合入 master 后两人先后因额度耗尽停机、按 AD-21 改派 **dev-m1d-f** 只做重采与状态推进（`files_modified` 为空）。

与 `.arcforge/tasks/transitions.jsonl` 的时间线**一致**：dev-m1d-b 于 12:27:50Z 领回 review_fix，Leader 15:21:51Z 收回；dev-m1d-e 15:22:52Z 认领，`d241978` 提交时刻 15:37:43Z（认领后 15 分钟，与「代码已写好、只做核实与提交」相符）；Leader 20:33:27Z 再次收回，dev-m1d-f 20:34:10Z 认领、20:38:20Z 转 `dev_done`（4 分钟，与「只重采不写码」相符）。`assignment_epoch` 三次 `assigned` 各 +1 ⇒ 3，与任务文件一致。

⚠️ 局限：git author 全仓统一为 `zuowei`，**作者字段不携带 agent 身份**，故「哪一行由谁写」无法从 git 独立证实，上述只是时间线自洽性核对。这不影响交付物本身的判定。

discovery 里的自证数字我逐条独立复采，**全部对得上**：覆盖率 96.5% / 76.3%、逐函数 100%、命名测试 11 条、gofmt 两欠账、vet 零输出、五个不动文件与 `go.mod/go.sum` 空 diff、两文件 sha256 与 `d241978` 逐字节相同。

## 7. 越界申报核对

`writes` = `["./internal/hestia/snapshot.go", "./internal/hestia/snapshot_test.go"]`。返工实际改动 `git show --numstat d241978` ⇒ `snapshot.go 30/0`、`snapshot_test.go 54/1`（共 84/1），**恰在声明内，无越界**。

⚠️ **仪器更正（不阻断，供后续复用）**：Leader 派验时给的 `git diff --numstat master...task/TASK-002-m1d-fix`（三点）**现在恒返回空**——分支已被 `290370f` 合入 master，`merge-base(master, 分支)` 就等于分支尖 `d241978`，三点 diff 自然为空。合入之后判「分支自身改动」的正确仪器是 `git show --numstat d241978` 或 `git diff --numstat f3d6eb2 d241978`，两者输出相同。三点写法只在分支**未合入**时成立，而 Leader 那条告诫（两点会把他人返工显示成反向删除）本身仍然正确。

## 8. 备注（不阻断）

- 上一轮报告 §4 的两条挂账（反事实覆盖率 96.19% 未复算、跨平台 rename/EISDIR 未验）本轮仍未复算，与最终交付无关，DoD 也不要求。
- `findSnapshotCopy` 的 `Glob` 每次改稿路径都会扫一次目录；单文章副本数量级极小，无性能问题，DoD 无非功能性能项。
- 探针显示第二次 `diverged` 落盘的时间戳副本名为 `a1.20260905T083015Z.html`（探针里 `now` 逐日递增），并非 DoD 里的固定值，属探针设计而非实现差异。

## 9. 复现命令（锚一律全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-002-m1d-c 290370feaa7e69b1eb533ca3c0a0683000853de4
cd ../wt-verify-TASK-002-m1d-c
GOTOOLCHAIN=local go test ./internal/hestia/... ./cmd/atlas/... -count=1 -cover   # 96.5% / 76.3%
GOTOOLCHAIN=local go test ./internal/hestia/ -run 'TestSaveSnapshot|TestWriteAtomic' -v -count=1   # 11 条
gofmt -l internal/hestia cmd/atlas          # 只许 backtest_test.go / crisis_test.go
go vet ./internal/hestia/... ./cmd/...      # 零输出
git show --numstat d24197853f995538dfa8abe2a1433b4b6c5bc99e   # 30/0 + 54/1，见 §7
```

---

## 10. 前两轮验证记录（保留）

### 第 1 轮 · test-m1d-a · epoch 1 · 判定 VERIFIED（2026-09-03T03:23:28Z）

- `verify_baseline.head`：`2f5ad513f7c0e7539c824e2e8f8a0f078baec316`（master）；discovery sha256 `9814bcb9…8b349`
- dev commit：`84c056bcaa5ee8b0696666917dd3bf684ba603f3`（父 `ae088eb`；merge `6a126d9`）
- 结论：接口、三态规则、错误文本、临时文件 + rename 与需求原文逐项一致；code-simplifier 把 `switch` 摊平为四个 case，逐路对照原文三分支等价。8 条测试全绿且 **7 组变异全部 KILLED**；`saveSnapshot`、`writeAtomic` 逐函数覆盖率 100%。DoD「允许不测」的两条错误分支 dev 都补了可移植测试。门禁全绿、覆盖率 96.4% ≥ 96.3%、无越界改动。
- 变异汇总：M1 `bytes.Equal` 恒真 ⇒ KILLED；M2 unchanged 分支仍 `writeAtomic` ⇒ KILLED；M3 删 `MkdirAll` ⇒ KILLED；M4 时间戳格式去 `Z` ⇒ KILLED；M5 rename 失败不清 tmp ⇒ KILLED；M6 `tmp := path` 直写 ⇒ KILLED；M7 diverged 改写原文件 ⇒ KILLED。
- 该轮挂账：discovery 称「只补原文 5 条 + R-002 时覆盖率 96.19% 跌破门槛」未复算；discovery 称 rename/EISDIR 两路「Windows 同样报错」未跨平台验证（DoD 不要求）。
- 该轮之后 QA Code Review 出 A 组两条缺陷（A3 改稿后幂等失效、A8 时区断言缺口），Leader 于 2026-09-03T12:27:21Z 转 `review_fix`（`reason_class: task_defect`）。**第 1 轮的 8 条 DoD 判定本身未被推翻**——A3 是 DoD 未覆盖的形态（DoD 只规定「不同字节 ⇒ 另存」，没规定「与旧副本同字节时怎么办」），A8 是断言强度不足，两者都在原 DoD 的射程之外。

### 第 2 轮（中断）· test-m1d-b · 未出裁决

承接后因账号额度耗尽停机、唤不回，未产出验证报告。Leader 按 AD-21 走 `verifying → verifying` 自环把 `verifier` 改派为 test-m1d-c（2026-09-03T21:01:39Z），`verify_baseline` 按规则**不刷新**。本报告即第 2 轮（改派后）的裁决。
