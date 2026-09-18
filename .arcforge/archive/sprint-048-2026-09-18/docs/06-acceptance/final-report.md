# Sprint M4 交付报告 — Hestia 队列可观测与自动触发

**需求**：`hestia/docs/superpowers/plans/2026-09-17-hestia-queue-observability-and-trigger.md`
**交付 HEAD**：`bd31159ba70b9d664fed5b66b787b26a0a26cfb8`（master）
**起始基线**：`24c2702988f851b72d5e218b2a7811f3029185a1`
**范围**：13 个 merge commit，16 个文件，+1489 / -19

## 1. 完成任务

| 任务 | 交付物 | dev / verifier | 返工 |
|---|---|---|---|
| TASK-001 | `QueueHealthOf` + `QueueHealth.ByState()`（纯文件系统读，目录缺失即报错） | dev-m4-a → dev-m4-c（QA 返工） / test-m4-b、test-m4-a | QA 返工 1 |
| TASK-002 | `Snapshot` 对 `state` label 展开 `<name>_<state>` 键 | dev-m4-b / test-m4-a | 0 |
| TASK-003 | collector 队列指标 + `hestia_db_up` / `hestia_queue_up`，DB 与队列失败隔离 | dev-m4-b / test-m4-a | QA 返工 1 |
| TASK-004 | serve 接线（队列目录取自 hestia 配置，每轮现读） | dev-m4-a → dev-m4-b（改派） / test-m4-a | 0（改派为 env 原因） |
| TASK-005 | 五条告警规则 + 表达式可求值的机制化证明 | dev-m4-b / test-m4-b | 1（dod_defect，根因为 Leader 措辞歧义） |
| TASK-006 | `hestia-warp-trigger.sh` 先判队列再唤起 + bash 自测 | dev-m4-c / test-m4-a | 0 |
| TASK-007 | `deploy/launchd/com.newthinker.atlas.hestia-warp.plist` + 安装清单 + 无代理键守卫 | dev-m4-c → dev-m4-b（QA 返工） / test-m4-b | 1（dod_defect）+ QA 返工 1 |

## 2. 验收状态

| 项 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `go test -count=1 ./...` | rc=0，65 包 ok，0 FAIL（非缓存） |
| 覆盖率 | `internal/hestia` 96.5%，`internal/metrics` 99.3%，`cmd/atlas` 78.0%（既有欠账，任务级 floor=78） |
| 触发脚本自测 | 9 passed, 0 failed |
| 两条写口守卫 | AST 与 reflect 均精确集合相等且绿（新增 `QueueHealthOf`、`QueueHealth.ByState`） |
| validator | rc=0（含 transition-audit、epoch 不变量、scope 互斥） |
| Code Review | PASS（有条件）→ 返工后增量复审 **PASS**，四条 WARNING 全部闭合 |

计划验收判据一（`go test ./...` 非缓存全绿 + 守卫精确集合相等）**已达成**；判据二～六属生产实测，见第 4 节。

## 3. 人类裁决（开发期，均已落地）

| # | 裁决 | 影响 |
|---|---|---|
| R1 | Snapshot 展开 `state` label，规则引擎不动 | 计划的 `hestia_queue_items{state="failed"}` 求值器不支持（`internal/alert/rules.go` 只认 `metric op number`），改用展开键 `hestia_queue_items_failed` |
| R2 | 用可恢复的 `*_up == 0` 取代累计 `*_errors_total > 0` | 计划的 `hestia_health_blind` 拆成 `hestia_db_blind` / `hestia_queue_blind`（不同名，evaluator 按 rule.Name 存 for/cooldown 状态） |
| R3 | plist 真相源入 `deploy/launchd/` 并登记 install-services.sh；生产动作归人类 | 覆盖计划「写入 `~/Library/LaunchAgents/`」 |
| R4 | 仓库侧只改 `configs/config.example.yaml` | `configs/config.yaml` 未被 git 跟踪 |
| R5 | **plist 不设任何代理键** | **覆盖计划 C8**。证据：nanoclaw@`e7c6278` 的 `scripts/chat.ts` 只连本地 `cli.sock`、零处读 env；daemon 的 LaunchAgent 仅 PATH/HOME；容器代理由 OneCLI gateway 注入。守卫 `TestHestiaWarpPlistSetsNoProxyKeys` |
| R6 | TASK-004 停掉 dev-m4-a 改派 dev-m4-b | 其消息通道单向失效（收不到，4 次批准全丢） |

Leader 自决（有先例）：TASK-004/005 `coverage_floor=78`（`task-completed.sh:483` 既有机制；`cmd/atlas` 往期设过 75/77）。

## 4. 交人类执行的清单（agent 不代做）

1. **runtime 告警规则同步**：把五条规则追加到 `/Users/zuowei/workspace/runtime/atlas/configs/config.yaml` 的 `alerts.rules`。原文取自
   `jq -j '.human_sync_required.paste' .arcforge/discoveries/TASK-005.json`（2582 字节，**用 `-j` 不用 `-r`**，后者多一个换行）。
   前提：runtime 的 serve 已部署 TASK-002/003/004；追加后重启 serve。
2. **装载 launchd**：`bash scripts/ops/install-services.sh`（已登记 `com.newthinker.atlas.hestia-warp`）。装载前确认
   `/Users/zuowei/.nvm/versions/node/v22.22.0/bin/pnpm` 仍在（plist 写死了版本号）。
3. **判据二（失败隔离，C2 的唯一诚实证明）**：把 runtime 队列目录改名 ⇒ `hestia_queue_*` 消失、`hestia_queue_errors_total` 加一、
   `hestia_queue_up` 为 0，而 `hestia_last_run_timestamp` 等 DB 指标照常输出；改回后恢复。
4. **判据三**：手工造 `failed/` 文件 ⇒ 30 分钟内收到 Telegram；清掉后不再重复。
5. **判据四（C4 的唯一诚实证明）**：队列空时跑触发脚本 ⇒ nanoclaw 无新会话（比对 `data/v2-sessions/` mtime），
   **且**脚本 exit 0、输出含 `queue empty`（只比 mtime 会假通过——目录缺失时脚本 exit 1 也不唤起）。
6. **判据五（端到端）**：投一份契约，什么都不做，等 launchd 自动唤起（≤30 分钟）⇒ 笔记自动出现在 `Wiki/Macro/PBOC/`。
   🔴 **必须等自动唤起**：`|| true` 使 pnpm 缺失时退出码仍为 0，写死的 node 版本失效也不会报错，手工跑脚本证明不了这两条路径。
7. **判据六**：`processing/` 非空时跑脚本 ⇒ 打印 `busy` 且不唤起。
8. **W-3 阈值校准**（QA 转人类）：`hestia_queue_processing_stuck > 0.5h` + `for 10m` ⇒ 契约进 processing/ 40 分钟即告警，
   而脚本注释自称「agent 常跑更久」。请在判据五跑通时记下实际停留时长再校准，并把实测写进注释。
9. **W-4 消费者契约**（QA 转人类）：互斥判据是 `processing/` 非空，而搬文件的是 nanoclaw warp skill。
   若 agent 从被唤起到 rename 超过 30 分钟，下一轮会重复唤起同一份契约。建议要求 skill 第一件事就是 rename 进 `processing/`。
10. **CONTRACTS 登记**：`internal/hestia/CONTRACTS.md` 新开一节，记四条规则实测、判据二的隔离证明、判据四的零唤起证明，
    以及「plist 写死 node 版本号」这个已知脆弱点。

## 5. 已知缺口与待办（不阻断交付）

| # | 事项 | 判断 |
|---|---|---|
| 1 | 四处「恰好四个状态」的断言（`hestia_collector_test.go:265/:314/:376`、`queue_health_test.go:192`）无提示 | 人类裁决记待办：加状态时它们必然变红，而**把等值放宽成包含最省事且不会有任何东西变红**，会删掉「加状态必须被看见」的信号 |
| 2 | `hestia_queue_errors_total` 当前零消费者（规则改用 `*_up == 0`） | Leader 裁决保留：与 up gauge 分工不同（累计审计 vs 本轮状态），接 Prometheus 后有用 |
| 3 | `cmd/atlas` 覆盖率 78.2%（门禁口径），距 80% 差 31 条语句 | 缺口在 `serve.go`(116)/`crisis.go`(42)/`broker.go`(34) 等本 sprint 范围外文件 |
| 4 | `hestia-ingest.plist` 头注释过期（第 25/39 行称「无代理键」、第 40 行守卫名 `TestHestiaPlistSetsNoProxyKeys` 已不存在） | **本 sprint 两次 `dod_defect` 的源头**，范围外，建议另开任务 |
| 5 | `snapshot_test.go` 83–93 行既有 gofmt 不合规 | 改动前即存在，未引入 |
| 6 | `queueReadDir` 是生产代码里的包级可变接缝 | QA I-15：当前用法规范（`t.Cleanup` 还原、无 `t.Parallel`、`-race` 干净），别让它成为第二个注入点的先例 |
| 7 | 遗留分支 `task/TASK-001..007` | 归档后可删；内容均已合入 master |

## 6. 过程事件（机制问题，已汇总进 `PENDING-MECHANISMS.md`）

- **teammate 消息单向失效 ×2**：dev-m4-a 收不到（4 次批准全丢，经人类批准 TaskStop 改派）；Leader→dev-m4-b/dev-m4-c 的回复丢失（两人各催办三次，实际每次都已处理）。
  **`stale-dispatch` 与 `ListAgents` 对这两类均不可见**；dev-m4-b 靠轮询 master 自愈，是有效的下游纪律。
- **子代理自报与实际不符 ×2/4**：code-simplifier 两次回「无动作」而实际改了文件（dev 靠指纹比对发现并在最终版本重采数字）。
- **QA lens 子代理结论丢失 ×2**：Skeptic/Architect 完成审查但未回传，由 qa-m4-a 本体补做并加变异实证（报告 §7 注明）。
- **写通道限制**：收回边 `in_progress → assigned` 不接受 `reason_class`，env_infra 归因只能记在 plan。
- **Leader 失误 ×2**：①一条脚本串联 `update`+`transition`，update 被 DENY 后 transition 照跑（已在 assigned 态补写）；
  ②TASK-005 的 DoD 措辞「与 description 逐项相等」有歧义，致一次 `task_defect` 误记在 dev 头上（验证者事后订正，真实根因为 `dod_defect`）。
- **dev 操作失误**：dev-m4-c 变异时 `git checkout --` 抹掉未提交实现（按原文重写后 sha256 逐字节相同，改用文件快照还原）。
- **已处理**：dev-m4-b 的 `ARCFORGE_TOKEN` 曾在 `ps` 中明文可见（挂起的后台命令），已轮换。

### 待同步 hooks 清单（人类执行）

| 文件 | 变更摘要 | 同步命令 |
| --- | --- | --- |
| （无） | 本仓库是 Arcforge 的**消费项目**，无 `project-template/`；本 sprint 未改动 `.claude/hooks/`、`.claude/scripts/`、`settings.json` 与运行时 `CLAUDE.md` | — |

机制改动落点在上游仓库 `newthinker/ArcForge`，本 sprint 的机制发现已汇总进本仓库根 `PENDING-MECHANISMS.md`，由人类择机推到上游。
