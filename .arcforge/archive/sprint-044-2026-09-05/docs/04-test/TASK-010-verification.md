# TASK-010 验证报告 · 主配置两个新键 `AlertRule.Cooldown` / `HestiaConfig{ConfigPath}`；`mapRules` 透传

- 验证者：test-m15-a · 2026-09-04
- 判定对象：`verify_baseline.head = 5c1f8e878768c03a5dbeb0375d41c644439f8004`（master，merge of dev commit `2658e5dcbfd02b9d8a1b606ec8b0aad46d0a0e75`）；当前 HEAD 与之一致，无漂移
- discovery：`.arcforge/discoveries/TASK-010.json` sha256 `5cea3067f8970f5a082e8a59945a8cb3f75db846c2f9de45f5e3efc854777302`（与基线一致）
- 验证环境：`git worktree add --detach ../wt-verify-TASK-010 5c1f8e878768c03a5dbeb0375d41c644439f8004`；变异与红阶段复现在 `mktemp -d` rsync 副本；主仓库与验证树四文件 sha256 前后一致（harness 自检 PASS）
- 基线对照锚：`037d1eb1e4f827c415319519e40f4e2208968920`（config 83.3% / cmd/atlas 76.3%）；本任务隔离锚 `511ee4264d1df3129b986e4f8857e3284c06d754`

## 结论：**VERIFIED**

7 条 done_criteria 全部有我自跑的输出作证据；范围核对无越界；4 个变异全部 KILLED。

## 范围核对

`git diff --numstat 511ee42 5c1f8e8`：`cmd/atlas/alert_runner.go` 1/0、`alert_runner_test.go` 8/1、`internal/config/config.go` 11/0、`config_test.go` 47/0——恰为声明 `writes` 四文件；dev commit `2658e5d` numstat 同四文件（显式 pathspec）。`go.mod`/`go.sum`/四冻结文件 diff 为空。

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据（均在 5c1f8e8 上自跑） | 判定 |
|---|---|---|---|
| functional[0] | `AlertRule.Cooldown time.Duration \`mapstructure:"cooldown"\``；`HestiaConfig{ConfigPath \`mapstructure:"config_path"\`}` + 三行注释（只指向 hestia 配置 / `db_path` 不复制 / 相对 WorkingDirectory / 空 = 不暴露 / `M1.5 的 TASK-010`）；`Config.Hestia` 在 `Prism` 之后 | diff 逐行核对：`config.go:30 Prism` / `:31 Hestia` 相邻；注释三行内容齐全、任务号写 TASK-010；`AlertRule.Cooldown` 与 `For` 同形。**M1**（`cooldown`→`cooldownx`，Leader 指定）⇒ `config_test.go:759 Rules[0].Cooldown = 0s, want 24h` KILLED；**M2**（`config_path` 改名）⇒ `:753` KILLED；**M3**（`hestia` 改名）⇒ `:753` KILLED | PASS |
| functional[1] | `mapRules` 加 `Cooldown: r.Cooldown,`；`TestMapRules_FieldMapping` r1 `Cooldown: 24h` ⇒ `out[0].Cooldown == 24h`，r2 不写 ⇒ `out[1].Cooldown == 0`，既有五字段比对不动 | diff：`alert_runner.go` 仅 +1 行；测试 diff 只改 r1 输入并追加两条断言，既有比对循环未动；`--- PASS: TestMapRules_FieldMapping`；**M4**（透传改 `Cooldown: 0`）⇒ `alert_runner_test.go:189 rule 0 Cooldown = 0s, want 24h` KILLED | PASS |
| functional[2] | `config_test.go` 新增一条（`writeTempConfig` → `Load`）：yaml 含 `hestia.config_path` 与 `rules[0].cooldown: 24h`，断言两值；另一段不写 `hestia:` ⇒ `ConfigPath == ""` | `TestLoad_HestiaConfigAndRuleCooldown_FromYAML` 主体断言 `configs/hestia.yaml` 与 `24*time.Hour`；`/absent` 子测试断言空串；两者 `--- PASS` | PASS |
| boundary[0] | `ConfigPath` 不解析、不 `filepath.Abs`、不校验存在、`Load`/`Validate` 不加 hestia 校验、`config.go` 不新增 import；`Cooldown` 不校验负值 | `git diff 511ee42 5c1f8e8 -- internal/config/config.go`：新增 11 行全是两个 struct 字段 + 注释；新增行中以引号开头（import）= **0**、含 `filepath`/`Abs(`/`os.Stat`/`Validate`/`func` = **0**；删除行 **0**（`Load` 函数体未动，gitnexus HIGH 是 `Config` 固有扇出 + 行号位移） | PASS |
| error_handling[0] | 红阶段两段留痕 | 副本把 `config.go`/`alert_runner.go` 换回 `511ee42` 保留新测试 ⇒ `alert_runner_test.go:174:105: unknown field Cooldown in struct literal of type config.AlertRule` / `config_test.go:752:9: cfg.Hestia undefined` + `:758:25 …Cooldown undefined`；discovery `red_phase` 两段同文案 | PASS |
| non_functional[0] | 门禁 | `gofmt -l` 五包 = 恰三既有欠账；`go vet` rc=0；五包 `-count=1` 全 ok：hestia 96.5% / metrics 98.9% / alert 92.6% / **config 83.3%**（= 基线）/ **cmd/atlas 76.3%**（= 基线）；无新增依赖；四冻结文件 diff 空；注释含 `M1.5 的 TASK-010` | PASS |
| non_functional[1] | AD-6 交付流程 | 提交 `feat(TASK-010): M1.5 …` 匹配门禁 grep；merge `5c1f8e8` 在 master；`git worktree list` 无 `wt-TASK-010-m15`（已拆）；discovery 同时写 `my_commit`/`master_after_merge`，自证数字锚 5c1f8e8 与我复采一致；code-simplifier 为 dev 自述（review 级） | PASS（review） |

## 变异汇总（隔离副本，被测树 5c1f8e8）

| 变异 | 位置 | 结果 |
|---|---|---|
| M1 `mapstructure:"cooldown"` → `cooldownx` | config.go | KILLED |
| M2 `mapstructure:"config_path"` → `configpath` | config.go | KILLED |
| M3 `mapstructure:"hestia"` → `hestiax` | config.go | KILLED |
| M4 `Cooldown: r.Cooldown` → `Cooldown: 0` | alert_runner.go | KILLED |

## 备注（不影响判定）

- 门禁口径旁证：`-coverpkg=./internal/config/,./cmd/atlas/` 两包合并时 `cmd/atlas` 行报 74.5%（AD-4 预期的拉低形态）；门禁按 coverprofile total 与 `coverage_floor: 75` 比较且 `dev_done` 已过，DoD 判据「各包不低于基线」满足（83.3 / 76.3 均持平）。
- 分支 `task/TASK-010-m15` 仍在（已合入）。
- 复现命令（锚全 sha）：`git worktree add --detach <dir> 5c1f8e878768c03a5dbeb0375d41c644439f8004 && cd <dir> && GOTOOLCHAIN=local go test ./internal/config/ -run TestLoad_HestiaConfigAndRuleCooldown_FromYAML -count=1 -v && GOTOOLCHAIN=local go test ./cmd/atlas/ -run TestMapRules_FieldMapping -count=1 -v`
