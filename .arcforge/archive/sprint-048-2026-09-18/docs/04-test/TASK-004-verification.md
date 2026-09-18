# TASK-004 验证报告：serve 接线（队列目录从 hestia 配置传入 collector）

- 验证者：test-m4-a
- 判定对象：master @ `6d5230e893a329870a943261858cbbc5a8bdeeae`（= verify_baseline.head，判定时主仓库 HEAD 相同）；实现提交 `be8aa67f71857408a56b83e8b3dca757b4963aac` 是其祖先，`git diff --stat be8aa67..6d5230e` 在两个声明文件上为空
- discovery sha256：`bea728f98d339ba2b2400175d49402772dba6f6c3dffd3c575fcd444c89290e0`（与 baseline 一致）；assignment_epoch=2
- 声明范围：`git diff --numstat be8aa67^ be8aa67` 只涉及 `writes` 里的两个文件（hestia_health.go 13/2、hestia_health_test.go 93/0），无越界
- 交接核对：`tasks/transitions.jsonl` 显示 dev-m4-a 认领（13:51:08Z）→ Leader 收回（14:51:35Z）→ dev-m4-b 认领（14:52:01Z）→ dev-m4-b 写 `coverage_floor`（op=update，absent→78）→ dev_done → verifying，与 discovery 的 `provenance` 叙述一致
- 运行环境：独立 worktree `../wt-verify-TASK-004`（detached @ 上述全 sha），GOTOOLCHAIN=local，go1.24.4
- 结论：**VERIFIED**（6/6 条 done_criteria 通过）

## 一、验证者亲自运行的证据

| 检查 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `go test -count=1 -cover ./cmd/atlas` | ok，78.0% |
| `go test -count=1 -run 'TestBuildHestiaHealth\|TestExampleConfig' -v ./cmd/atlas` | 10 个 PASS，0 FAIL（5 个既有 + 5 个新增） |
| 全量回归 `go test -count=1 ./...` | rc=0，65 个包 ok，0 FAIL |
| `go vet ./cmd/atlas` / `gofmt -l` 两个文件 | 通过 / 无输出 |
| 覆盖率「不回退」（背对背）：`c4d651c` 和 `6d5230e` 两个 worktree **同时**跑 `go test -count=1 -coverpkg=./cmd/atlas ./cmd/atlas` | 两边 total 都是 **78.2%**；`buildHestiaHealth` 100.0% ⇒ 「本任务未拉低覆盖率」属实。门槛 78 由 Leader 批准，不是本报告的判定项 |

## 二、done_criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | 绝对路径 queue.dir 下建好四个子目录 ⇒ nil error，Snapshot 里 `hestia_queue_up==1` 且有 `hestia_queue_items` 键 | `TestBuildHestiaHealth_WiresQueueDir`（经 `buildWithQueue` 调 `require.NoError`）；Y1/Y8 被杀 | PASS |
| functional[1] | 相对路径 `queue/hestia` 相对进程 cwd 解析（`t.Chdir`）⇒ queue_up==1；注释写明依赖 launchd WorkingDirectory 与 hestia-ingest 一致 | `TestBuildHestiaHealth_QueueDirRelativeToCwd`：四个目录建在 `root/queue/hestia`，`t.Chdir(root)`，hestia.yaml 放在**另一个** TempDir，所以「按配置文件目录解析」拿不到 1 ⇒ Y3 被杀，证明测试确实跑到了 cwd 解析。注释在 `hestia_health.go:49-50`（「依赖 launchd 给 serve 的 WorkingDirectory 与 hestia-ingest 一致」） | PASS |
| boundary[0] | reg==nil 或未设 config_path ⇒ no-op cleanup、nil error、不输出 `hestia_` 指标；既有测试函数体零改动且全绿 | `TestBuildHestiaHealth_SkippedPathsEmitNoHestiaMetrics`（未设 config_path 时遍历 Snapshot 的键做前缀检查；reg nil 时只断言 nil error，因为没有注册表可看）；Y9 被杀。测试文件 numstat 93/0，三处 hunk 分别在文件头映射注释、import、文件末尾，都不在既有函数体内；5 个既有测试全绿 | PASS |
| boundary[1] | 没有「queue.dir 为空」分支，恒传非 nil QueueFunc；理由引 config.go:257 并写进 decisions | review：`grep -rnE 'Queue\.Dir == ""\|queueDir == ""' cmd/atlas` 命中 0；`internal/hestia/config.go:257-258` 确实是 `case c.Queue.Dir == "": return errors.New("queue.dir must not be empty")`；discovery `decisions[0]` 引用了它 | PASS |
| error_handling[0] | queue.dir 不存在 ⇒ nil error，`hestia_queue_up==0` 且 `hestia_db_up==1` | `TestBuildHestiaHealth_MissingQueueDirDoesNotFailStartup`（先断言 queue_up 键**存在**再断言值为 0，防止缺席读出 0）；Y4/Y5 被杀 | PASS |
| error_handling[1] | 启动后删掉 pending/ ⇒ 下一次 Snapshot queue_up==0；建回 ⇒ 1 | `TestBuildHestiaHealth_QueueReadEveryScrape`：同一个 reg 依次 1 → 删 → 0（断言键存在）→ `EnsureQueueDirs` → 1；Y2（启动时快照）、Y6（成功后粘滞）、Y7（失败后粘滞）都被它杀死，证明测试确实跑到了运行期每轮现读 | PASS |

## 三、验证者独立变异（在我专用的 worktree 里做，每个变异后还原并比对 sha256，最后 `git status --porcelain` 0 行）

对照组全绿。10 个变异 `go vet` 全部通过，没有 panic 致红：

| 变异 | 结果 / 杀死它的测试 |
|---|---|
| Y1 传 nil QueueFunc | KILLED：4 个新增用例 |
| Y2 启动时读一次队列并快照 | KILLED：QueueReadEveryScrape |
| Y3 相对路径按配置文件目录解析 | KILLED：QueueDirRelativeToCwd |
| Y4 队列目录缺失即启动失败 | KILLED：MissingQueueDirDoesNotFailStartup、既有 RegistersCollector |
| Y5 吞掉队列错误（err→nil） | KILLED：MissingQueueDir…、QueueReadEveryScrape（本人新增，dev 没做过） |
| Y6 首次成功后粘滞缓存 | KILLED：QueueReadEveryScrape（本人新增） |
| Y7 首次失败后粘滞 | KILLED：QueueReadEveryScrape（本人新增） |
| Y8 读错目录（snapshot_dir） | KILLED：3 个用例 |
| Y9 未设 config_path 时仍注册 collector | KILLED：SkippedPathsEmitNoHestiaMetrics（本人新增） |
| Y10 启动时用 `filepath.Abs` 把 queue.dir 绝对化 | SURVIVED：等价变异，serve 进程运行期不会 chdir，启动时解析和每轮解析结果相同 |

## 四、观察（不在 DoD 内，不作判定依据）

1. 既有测试 `TestBuildHestiaHealth_RegistersCollector` 的 yaml 没写 queue.dir，会取默认相对路径 `queue/hestia`，而测试 cwd（cmd/atlas）下没有这个目录 ⇒ 该测试里 queue_up=0。现在无害，Y4 还靠它多了一道守卫；但如果有人在 `cmd/atlas/queue/hestia` 下建了目录，这个测试的队列侧行为会悄悄改变。
2. Y10 等价的前提是 serve 进程不 chdir。将来若改成启动时绝对化，日志里的 `queue=` 字段会从相对路径变成绝对路径，排查时更直观，可以考虑。
3. 交接那次迁移（`in_progress→assigned`，14:51:35Z）的 `reason_class` 是空串，不是 `env_infra`。`rework_count` 仍为 0，没有实际影响，但审计行上看不出这是环境原因的改派，只能靠 discovery 的 `provenance` 说明。
