# TASK-006 验证报告 · `serve` 按 `hestia.config_path` 注册健康度 collector——未设跳过、装不上即启动失败；样例配置与部署文档

- 验证者：test-m15-a · 2026-09-04
- 判定对象：`verify_baseline.head = e5ada52396fc088434a5def263fb7c23c5347607`（master，merge of dev commit `6007f72fca87dd1bcaf97b1c219b9d677a8e769c`，dev-m15-b）；当前 HEAD 与之一致，无漂移
- discovery：`.arcforge/discoveries/TASK-006.json` sha256 `b590f3dbb48e947cb394dec2dbc0fcaa093a46a7ad8048845467fff20708b5c2`（与基线一致）
- 验证环境：`git worktree add --detach ../wt-verify-TASK-006 e5ada52396fc088434a5def263fb7c23c5347607`；变异与红阶段复现在 `mktemp -d` + `git archive` 副本；主仓库与验证树五文件 sha256 前后一致（harness 自检 PASS；主仓库 `configs/config.example.yaml` sha256 `472ce93ec4f6d41fed85a9c263504d3671dd01ce0917445c3e1ffea5e8366fc6` 前后不变）
- 隔离锚：`4c14d7903cbd01258ddeab9e5f3aab5641871c47`；覆盖率基线锚 `037d1eb`（cmd/atlas 76.3%）

## 结论：**VERIFIED**

7 条 done_criteria 全部有我自跑的输出作证据；范围核对无越界；9 个变异（2 yaml + 7 代码）全部 KILLED。

## 范围核对

`git show --numstat 6007f72`：`cmd/atlas/hestia_health.go` 50/0、`hestia_health_test.go` 150/0、`serve.go` 7/0、`configs/config.example.yaml` 20/0、`docs/deployment.md` 1/1——恰为声明 `writes` 五文件（显式 pathspec）。`go.mod`/`go.sum`/四冻结文件 diff 为空。

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试 / 证据（均在 e5ada52 上自跑） | 判定 |
|---|---|---|---|
| functional[0] | `buildHestiaHealth(cfg, reg, log) (func(), error)` 三种启动语义（注释带「沿 M1d 挂账 C3」与 WAL 多读者说明）：`reg == nil` ⇒ `Info("hestia health disabled (metrics disabled)")` no-op nil；`ConfigPath == ""` ⇒ `Info("… (hestia.config_path not set)")`；`LoadConfig` 失败 ⇒ `hestia health: loading %s: %w`；`NewStore` 失败 ⇒ `hestia health: opening %s: %w`；成功 ⇒ `MustRegister(NewHestiaCollector(HealthSummary(ctx, st.DB()), time.Now))` + `Info("hestia health enabled", config, db)` + cleanup 关 Store。`serve.go` 接线在 `metricsReg` 段之后、`appCtx` 之前 | `hestia_health.go` 50 行逐条核对（`:21` C3 句、`:24-25` WAL 句、`:28-49` 五段与 DoD 逐字对应）；`serve.go:180-185 metricsReg` → `:187-191` 接线（`cleanupHestiaHealth, err := …; if err != nil { return err }; defer cleanupHestiaHealth()`）→ `:198 appCtx`，位置恰在中间（Leader ④）；**M4**（装不上静默返回 nil）⇒ `:88` KILLED；**M5**（成功不注册）⇒ `:116/:117` KILLED；**M6**（reg nil 分支文案改）⇒ `:126` KILLED | PASS |
| functional[1] | `DisabledWhenUnset`（无错、`hestia_pending_review` 不在 Gather；`zaptest/observer` 断言**恰一条** `hestia health disabled`）、`FailsLoudlyWhenUnloadable`（`nope.yaml` ⇒ 错误含路径）、`RegistersCollector`（`pending_review`/`runs_total` 可见、`last_run_timestamp` 不可见；`reg == nil` ⇒ nil）；夹具过不了 `LoadConfig` 时补键不放宽 `validate` | 五条 `--- PASS`；`DisabledWhenUnset :75-79` 用 `observer.New` + `FilterMessageSnippet` `require.Len 1` + `logs.Len()==1`（Leader ①）；**M1**（不打日志）⇒ `:77 got 0` KILLED；**M2**（多打一行）⇒ `:79` KILLED ⇒「恰一条」两侧都在守；`RegistersCollector` 断言三项可见性与 TASK-004 discovery 一致，`reg nil` 子段也用 observer 断恰一条；夹具四键（storage.db_path/snapshot_dir、discover.index_url/max_pages/timeout）即过 `LoadConfig`，未补键（discovery 已记，AD-10 备用出路未触发） | PASS |
| functional[2] | yaml `alerts.rules` 末尾追加 `hestia_stalled`/`hestia_no_ingest`（expr/for/cooldown/severity/message 与四行注释按需求原样）；`prism:` 之前加 `hestia:` 段 + 三行注释；`deployment.md` serve 行末尾那句 | yaml 新增 19 个非空行**逐行** `grep -F` 在需求原文命中（含两条 message 与全部注释）；`hestia:` 段位于 `:259-260`、紧接其后是 `# Prism 估值面板` / `prism:`；`deployment.md:256` serve 行含「`hestia.config_path` 设了则暴露 `hestia_*` 健康度指标（读 `data/hestia.db`），装不上即启动失败」，该句 `grep -F` 在需求原文命中（Leader ⑤） | PASS |
| boundary[0] | `TestExampleConfigDeclaresHestiaRules`：整份示例 yaml `config.Load` 无错、`ConfigPath == "configs/hestia.yaml"`、两条规则 `Cooldown == 24h`、severity `critical`/`warning`；变异删 `cooldown: 24h` 一行必红（隔离副本、主工作区 yaml sha256 不变）；S4 若其他段装不上走澄清环；`hestia_health.go` 不 import `path/filepath` | 测试 `--- PASS`（断言逐项对 DoD）；**M-yaml-1**（删 `hestia_stalled` 的 cooldown 行）与 **M-yaml-2**（删 `hestia_no_ingest` 的）⇒ 均 `:145 cooldown` 红 KILLED（Leader ②），主仓库与验证树 yaml sha256 前后一致；S4 预检 discovery 记录「改动前整份可装载」，且改动后测试 PASS 即为证；`grep -c '"path/filepath"' hestia_health.go` = **0**（`TestHestiaCmdDoesNotResolveDBPath` 仍绿） | PASS |
| error_handling[0] | 红阶段 `undefined: buildHestiaHealth`；错误串以 `hestia health:` 开头，`HasPrefix` 断言进 `FailsLoudly` | 副本移走 `hestia_health.go` ⇒ `serve.go:187:30 undefined: buildHestiaHealth` + `hestia_health_test.go:71/:87/:102`；discovery `red_phase` 同形；`FailsLoudlyWhenUnloadable :90` 与 `StoreUnopenable :104` 均 `strings.HasPrefix(…, "hestia health:")`（Leader ③）；**M3**（loading 去前缀）⇒ `:90` KILLED；**M7**（opening 前缀改 `open`）⇒ `:104` KILLED | PASS |
| non_functional[0] | 门禁；`buildHestiaHealth` 三条分支都有测试覆盖 | `gofmt -l` 五包 = 恰三既有欠账；`go vet` rc=0；五包 `-count=1` 全 ok：**cmd/atlas 76.4%**（基线 76.3，与 Leader 预演一致）/ hestia 96.6% / metrics 99.2% / alert 92.6% / config 83.3%；无新增依赖；四冻结文件 diff 空；`M1.5 的 TASK-006` 注释三文件均有；三分支覆盖：跳过（M1/M2/M6）、失败（M3/M4/M7）、成功（M5）各有变异被杀 ⇒ 都在测试射程内 | PASS |
| non_functional[1] | AD-6 交付流程 | 提交 `feat(TASK-006): M1.5 …` 匹配门禁 grep；merge `e5ada52` 在 master；`git worktree list` 无 `wt-TASK-006-m15`（已拆）；discovery 同时写 `my_commit`/`master_after_merge`，自证数字锚 e5ada52 与我复采一致；dev 补的 `FailsLoudlyWhenStoreUnopenable` 在 `writes` 内、已申报、覆盖 opening 分支；code-simplifier 为 dev 自述（review 级） | PASS（review） |

## 变异汇总（隔离副本，被测树 e5ada52）

| 变异 | 位置 | 结果 |
|---|---|---|
| M-yaml-1 删 `hestia_stalled` 的 `cooldown: 24h`（Leader ②） | config.example.yaml | KILLED |
| M-yaml-2 删 `hestia_no_ingest` 的 `cooldown: 24h` | config.example.yaml | KILLED |
| M1 未设 config_path 时不打日志 | hestia_health.go | KILLED（恰一条 ⇒ 0） |
| M2 未设 config_path 时多打一行 | hestia_health.go | KILLED（恰一条 ⇒ 2） |
| M3 loading 错误去掉 `hestia health:` 前缀 | hestia_health.go | KILLED |
| M4 装不上静默返回 nil | hestia_health.go | KILLED |
| M5 成功时不 `MustRegister` | hestia_health.go | KILLED |
| M6 reg nil 分支文案改 | hestia_health.go | KILLED |
| M7 opening 前缀改 `open` | hestia_health.go | KILLED |

## 备注（不影响判定）

- gitnexus HIGH 按 diff 判：`serve.go` 仅 +7 行，`reg == nil` / `ConfigPath == ""` 时 no-op，既有流程不变。
- 运维语义（discovery 已记、deployment.md 已写）：样例 yaml 声明 `hestia.config_path: configs/hestia.yaml`，照抄部署而无该文件时 serve 以 `hestia health: loading …` 启动失败，这是 spec 要的响亮失败。
- 分支 `task/TASK-006-m15` 仍在（已合入）。
- 复现命令（锚全 sha）：`git worktree add --detach <dir> e5ada52396fc088434a5def263fb7c23c5347607 && cd <dir> && GOTOOLCHAIN=local go test ./cmd/atlas/ -run 'TestBuildHestiaHealth|TestExampleConfigDeclaresHestiaRules' -count=1 -v`
