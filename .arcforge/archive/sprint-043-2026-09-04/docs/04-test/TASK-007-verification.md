# TASK-007 验证报告 · cmd 层：--only-period flag 与开库前校验、buildHestiaSender、plist 传 --config

- 验证者：test-m1d-b（第 3 轮；第 1–2 轮由 test-m1d-a）　判定：**VERIFIED（第 3 轮，epoch 3，rework 2，review_fix 第 1 轮返工）**
- 第 3 轮 verify_baseline.head：`b47e440501d0f2dd415f9c1a39e8f66f3dd63b8b`（master = 本任务 merge commit，验证时 HEAD 同值）；discovery sha256 与基线一致 `c83563d1bc496577468e81e998777058ac4b133c514cf87c0a81a0e3482c16d5`
- 返工 commit：`aba2fb38d82ba174a6e9952a736316add0897b24`（父 `f3d6eb282c83e0ca730b1713907f5220114ee86b`；merge `b47e440`），改 `cmd/atlas/hestia.go` 18+/8-、`cmd/atlas/hestia_test.go` 30+/7-；`git show --name-only` 恰这两文件，在 `writes` 内（plist 本轮未动）
- provenance（discovery `rework[1]` 如实记录）：代码由 dev-m1d-c 编写并提交，失联后 Leader 改派 dev-m1d-d（epoch 3）做 master 重采、补 discovery 与状态推进，未改代码。验证者核对：commit 12:36:39Z 早于 dev-m1d-d 认领 15:22:45Z
- 验证树：`git worktree add --detach ../wt-verify-TASK-004-007-m1d-b b47e440501d0f2dd415f9c1a39e8f66f3dd63b8b`（与 TASK-004 共用，串行）；变异在该树副本上做，每次后 `git checkout --` 还原、核实 sha256 前缀 `e613a32c09ec0b6c` 与 porcelain 为 0；主仓库 `hestia.go` 指纹 harness 前后同值

## 1. 结论

QA 终审 A5 已按「推荐」分支落地：`buildHestiaSender` 签名改为 `(hestia.Sender, error)`，`loadConfigOrDefaults` 出错（验证者核对 `export_ohlcv.go:283`：仅 `cfgFile != ""` 时才可能出错，dev 注释「err 恒等于主配置装不上」成立）⇒ 返回带 `cfgFile` 的错误，`runHestiaIngest` 在 `openHestia` 之后、通道状态行之前直接返回；未配置三形态仍返回字面量 nil。原「unloadable ⇒ nil」子用例改为「⇒ error, not nil-and-silent」，`TestHestiaIngestPrintsNotifyStatus` 增「main config unreadable ⇒ ingest fails loudly, no notify line」。A5 判据变异：折回 `nil, nil` ⇒ 两用例红；`runHestiaIngest` 忽略 err ⇒ ingest 层子用例独家红；把 err 检查挪到通道行之后 ⇒ 同一子用例独家红（钉住「不打 notify 行」）；未配置也报错 ⇒ `TestBuildHestiaSenderNoConfig` 等 4 用例红（钉住「三形态仍 nil」）。一组存活见 §3（等价变异，不阻断）。前两轮已过项无回退（M6/M7 接线守卫仍 KILLED、plist 四条测试绿、`plutil -lint` OK）。门禁全绿，`cmd/atlas` 76.3% ≥ 76.2（A/B 背对背两轮 `aba2fb3` 76.3% / `f3d6eb2` 76.2%），`hestia.go` import 块无 `path/filepath`，提交锚匹配，merge 早于 `dev_done`。

**DoD 文本与 fix_items 冲突（记录，非缺陷）**：任务文件 DoD functional[1] 原文「`loadConfigOrDefaults()` 出错 … ⇒ 返回字面量 nil」已被 Leader 拍板的 fix_items A5 推翻；在途任务 Leader 无权改 DoD 文本，故本轮以 `fix_items` 为准。建议 CONTRACTS 登记这条契约更正。

## 2. 覆盖矩阵（第 3 轮：fix_items 逐条 + 前两轮项回退检查）

| # | 标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| fix A5 ① 主配置装不上 ⇒ 错误 | `cfgFile != ""` 且装载出错 ⇒ 不折成 nil，返回错误 | `hestia.go:281-283`；`TestBuildHestiaSenderFromMainConfig/unloadable main config ⇒ error, not nil-and-silent`（`require.Error` + `Contains cfgFile` + `s == nil`）；变异 H-A5a（折回 `nil, nil`）⇒ 该子用例 + ingest 层子用例红（顶层 FAIL 2） | PASS |
| fix A5 ② `runHestiaIngest` 响亮失败 | 不进 Ingest、不打 notify 行 | `hestia.go:309-312`；`TestHestiaIngestPrintsNotifyStatus/main config unreadable ⇒ ingest fails loudly, no notify line`（`require.Error`、`Contains cfgFile`、`NotContains "notify:"`、`NotContains "no new reports"`）；变异 H-A5b（`sender, _ :=` 忽略 err）⇒ 该子用例独家红；H-A5e（err 检查挪到通道行之后）⇒ 该子用例独家红 | PASS |
| fix A5 ③ 未配置仍 nil | 没传 --config / 未启用 / 缺 chat_id ⇒ 字面量 nil、无错误 | `TestBuildHestiaSenderNoConfig`（`require.NoError` + `s == nil`）、`FromMainConfig/disabled`、`/missing chat_id`；变异 H-A5d（未配置也返回错误）⇒ 4 顶层用例红 | PASS |
| fix A5 ④ 改旧用例 | 「unloadable ⇒ nil」改成新契约 | diff 里该子用例名与断言均已改；旧断言 `assert.True(buildHestiaSender() == nil)` 已不存在 | PASS |
| fix 非功能 | 只改两文件；`cmd/atlas` ≥ 76.2%；无 `path/filepath`；锚；交付流程 | `git show --name-only aba2fb3` 恰两文件；树 `b47e440` `cmd/atlas` **76.3%**、`internal/hestia` 96.4%；`grep path/filepath hestia.go` 2 处均为注释（`:144`、`:254`），import 块为 fmt/io/regexp/time/cobra/hestia/telegram；`fix(TASK-007): M1d buildHestiaSender …` 匹配锚；merge `b47e440` 12:37:41Z 早于 `dev_done` 15:24:56Z；discovery `rework[1]` 含 fix_commit / merge_commit / master_sample（锚 `b47e440`、content_check 两文件与 aba2fb3 一致）/ provenance；`wt-TASK-007-m1d-fix2` 已拆 | PASS |
| functional[0] flag / 校验 / 接线 | 前两轮已 PASS，查回退 | `TestHestiaFlags`、`TestHestiaOnlyPeriodValidation`、`TestHestiaIngestWiresOnlyPeriod` 基线绿；回归 M7（删 `OnlyPeriod:` 接线）⇒ WiresOnlyPeriod 独家红 | PASS |
| functional[1] `buildHestiaSender` / 通道行 / 接线 | 同上（原文「出错 ⇒ nil」子句已由 A5 推翻，见 §1） | `TestHestiaIngestPrintsNotifyStatus` 其余子用例、`TestHestiaIngestWiresNotify` 基线绿；回归 M6（删 `Notify:` 接线）⇒ WiresNotify 独家红 | PASS |
| functional[2] plist | 同上 | `TestHestiaPlistPassesMainConfig` 等四条 plist 测试基线绿；`plutil -lint` OK；plist 本轮未改（`208a77c..b47e440` 对该文件 numstat 空） | PASS |
| boundary[0] / error_handling[0] | 同上 | `TestHestiaOnlyPeriodHelp`、`TestHestiaCmdDoesNotResolveDBPath` 基线绿；通道行仍在 `openHestia` 之后（且现在也在 err 检查之后） | PASS |
| non_functional[0] 门禁 | floor 75 且新增分支有测试；gofmt / vet / 两包 / 五个不动文件 / 无新依赖 | 76.3% ≥ 76.2 ≥ floor；新增分支（err 返回）有两条用例；`gofmt -l` 仅两欠账；`go vet` rc=0；两包 `-count=1` ok；`git diff --stat 4916106 b47e440 -- {store,validate,parse,extract,fields}.go` 0 行；`go.mod/go.sum/types.go` 相对 `ae088eb` 0 行；注释写 `M1d 的 TASK-007 返工` | PASS |
| non_functional[1] 交付流程 | 见 fix 非功能行 | — | PASS |

**越界申报核对**：`git show --name-only aba2fb3` ⇒ `cmd/atlas/hestia.go`、`cmd/atlas/hestia_test.go`，在 `writes` 内；两文件在 `aba2fb3` 与 `b47e440` 逐字节一致（merge 为 fast-path 合并，无冲突改写）。

## 3. 变异汇总（第 3 轮，7/8 KILLED，1 存活判定为等价变异）

| 变异 | 期望转红 | 实测 |
|---|---|---|
| H-A5a `return nil, fmt.Errorf(…)` 折回 `nil, nil`（**A5 判据**） | FromMainConfig/unloadable + PrintsNotifyStatus/unreadable | KILLED（顶层 FAIL 2） |
| H-A5b `runHestiaIngest` 忽略 err（**A5 判据**） | PrintsNotifyStatus/unreadable | KILLED（独家） |
| H-A5c 错误串去掉 `%s`(cfgFile) | 两处 `Contains cfgFile` | **SURVIVED**（见下） |
| H-A5d 未配置也返回错误 | NoConfig 等 | KILLED（顶层 FAIL 4） |
| H-A5e err 检查挪到通道行之后（**A5 判据**：不打 notify 行） | PrintsNotifyStatus/unreadable | KILLED（独家） |
| M6-reg 删 `Notify: sender,`（回归） | WiresNotify | KILLED（独家） |
| M7-reg 删 `OnlyPeriod: hestiaOnlyPeriod,`（回归） | WiresOnlyPeriod | KILLED（独家） |

**H-A5c 存活的成因与判定**：两条用例都用「文件不存在」造装载失败，`config.Load` 的 `reading config: %w` 包着 `os.PathError`，内层文本已带完整路径，外层 `%s`(cfgFile) 去掉后 `Contains cfgFile` 仍过。fix_items A5 要求的是「返回错误、响亮失败」而非「错误串含路径」，该性质在被测输入下由内层保证，故判等价变异、不阻断。**加固建议**（不在本轮 DoD 内）：若要让外层 `%s` 有守卫，用「文件存在但 YAML 非法」作输入（yaml 解析错误不带路径），一条子用例即可杀死 H-A5c。与 TASK-005 证否账本 #3「内层文本替它过关」同形，记入 wisdom。

每组：`python3` 字面替换 → `go build ./cmd/atlas/` 有效性闸 → 打印 diff → `go test ./cmd/atlas/ -count=1` 全包 → `git checkout --` 还原 → sha256 前缀与 porcelain 核实。

## 4. 复现命令（锚全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-004-007-m1d-b b47e440501d0f2dd415f9c1a39e8f66f3dd63b8b
cd ../wt-verify-TASK-004-007-m1d-b
GOTOOLCHAIN=local go test ./cmd/atlas/ -run 'TestBuildHestiaSender|TestHestiaIngestPrintsNotifyStatus|TestHestiaIngestWires' -v -count=1   # 全 PASS
# A5 判据：hestia.go 把 `return nil, fmt.Errorf("hestia notify: main config %s: %w", cfgFile, err)` 改回 `return nil, nil`
GOTOOLCHAIN=local go test ./cmd/atlas/ -run 'TestBuildHestiaSenderFromMainConfig|TestHestiaIngestPrintsNotifyStatus' -count=1   # ⇒ 两条 FAIL
git checkout -- cmd/atlas/hestia.go
# A/B 覆盖率（背对背两轮）
git checkout --detach aba2fb38d82ba174a6e9952a736316add0897b24 && GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -cover   # 76.3%
git checkout --detach f3d6eb282c83e0ca730b1713907f5220114ee86b && GOTOOLCHAIN=local go test ./cmd/atlas/ -count=1 -cover   # 76.2%
```

## 5. 前两轮记录（test-m1d-a）

### 5.1 第 2 轮（VERIFIED，epoch 2，rework 1）

- verify_baseline.head `540e84a0eee6e37c5a85cb4743a189a1744aae64`；discovery sha256 `e9415cd9…df87`；返工 commit `0cecbae5dadc985c9b84c2b6c1fc011ad6c5aaa1`（父 `a1f5bd4baac61d846a4813163af84df42fc95587`；merge `540e84a`），只改 `hestia_test.go` 100+/0-。验证树 `../wt-verify-TASK-007-r2-m1d @ 540e84a`。
- 结论：首轮唯一缺陷（`IngestDeps` 的 `Notify` / `OnlyPeriod` 两行接线零测试）已修：两条端到端用例 `TestHestiaIngestWiresOnlyPeriod`、`TestHestiaIngestWiresNotify` 收进 `hestia_test.go`。删 `Notify: sender,` ⇒ 只有 WiresNotify 红；删 `OnlyPeriod:` ⇒ 只有 WiresOnlyPeriod 红；原状两包绿。`cmd/atlas` 76.2% ≥ floor 75。
- provenance：两条用例与两个 helper 由 test-m1d-a 在 `a1f5bd4` 上编写并实测（scratchpad `test-m1d-a-TASK-007-wiring-guard_test.go`），dev-m1d-c 原样收进（四个函数体 sha256 与处方逐字节一致），加分节注释与映射注释、`import sync/atomic`。`why_round1_missed` 自省成立（正向路径的伪索引页刻意无文章链接，Ingest 在 `no new reports` 返回，两行接线在那条路径上不被读到）。
- 变异 5/5 KILLED：M6 删 `Notify:`、M7 删 `OnlyPeriod:`、M2 nil 指针装接口（3 处红）、M4 plist 删 `--config`、M5 flag 注册到 `PersistentFlags`。门禁（树 `540e84a`）：76.2% / 96.4%、gofmt 两欠账、vet 0、两包 ok、五个不动文件 0 行、无新依赖、新增 import 仅标准库 `sync/atomic`。新用例不碰外网。

### 5.2 第 1 轮（REJECTED，task_defect，2026-09-03 11:07:34Z）

- 基线 `a1f5bd4baac61d846a4813163af84df42fc95587`；dev commit `cdcc64d`（父 `3c56760`）；改动 `hestia.go` 58+/7-、`hestia_test.go` 194+/0-、plist 5+/0-。
- 缺陷：DoD functional[0]/[1] 明写的两行接线 `Notify: sender,` / `OnlyPeriod: hestiaOnlyPeriod,` 零测试；M6/M7 变异下 `cmd/atlas` 全包绿。前者是 M1d 立项原因那个「通知静默关闭且没有东西变红」形态，后者让 `--only-period X --force` 静默退化成 Force 重跑全部候选。
- 首轮其余全部 PASS：dev 自报 5 组变异（M1 校验挪到开库后 / M2 nil 指针装接口 / M3 通道行挪到开库前 / M4 plist 删 `--config` / M5 flag 注册到 PersistentFlags）独立复验全 KILLED；`buildHestiaSender`、`runHestiaIngest` 100% 覆盖；`cmd/atlas` 76.2%（A/B 背对背 `cdcc64d` 76.2% / `3c56760` 75.7%）；`path/filepath` 未 import；`plutil -lint` OK；既有四条 plist 测试绿；`--help` 两行都在、文案含 `requires --force`；校验在 `openHestia` 之前。
- 首轮给出的两条守卫（httptest 伪站点 + httptest 代理记录 CONNECT）即第 2 轮 dev 收进的形态。
