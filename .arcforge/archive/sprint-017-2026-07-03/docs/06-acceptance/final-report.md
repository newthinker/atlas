# Sprint 017 交付报告 — 落地核查优化轮

> 需求: `docs/superpowers/specs/2026-07-02-audit-optimization-round-design.md`
> 周期: 2026-07-02 ~ 2026-07-03 ｜ 团队: Leader + dev×3 + test×1 + qa×1 + 独立 DoD reviewer×1
> 结论: **9/9 任务 accepted，3 个 PR 全部合并进 master（#41/#42/#43），合并后全量 go test ./... 50 包全绿**

## 1. 交付内容

| PR | 内容 | 任务 | 合并 |
|---|---|---|---|
| #41 清理加固 | FutuBroker 清理（Provider 缺省 futu→mock、live 校验收敛 paper-only）；okx/coingecko/binance 集成测试 `//go:build integration` 隔离 + `make test-integration`；文档治理（runbook 补 analysis 服务、架构文档 superseded 注记、crypto 设计未实现标注） | 101/102/103 | MERGED |
| #42 alert 接线 | `Registry.Snapshot()`（含 `_2xx/_4xx/_5xx` 状态类键）；notifier 适配器（SendText 直发/系统信号回退）+ telegram `SendText`；serve 装配评估循环 + 派生指标（http_error_rate delta、signals_24h）+ 示例规则；**QA 修复**: W1 告警纯文本发送（去 parse_mode）、W2 evaluator Notify 失败不吞错不进冷却 + 生产 logger 注入 | 201/202/203/204 | MERGED |
| #43 sqlite 持久化 | `NewSQLiteStore`（v1.38.2 锁定、WAL、契约测试钉死双实现语义）；`storage.signals` 配置节（**默认 sqlite**，打开失败快速失败不降级） | 301/302 | MERGED |

## 2. 整体验收对照（需求 §6）

- ✅ 三个 PR 各自 `go test ./...` 全绿；Wave1 后不联网、确定性（test-agent 实证无 Integration 用例执行）
- ✅ `make test-integration` 实跑通过（binance 网络超时属容忍范围）
- ✅ alert 链路：配置规则 + 触发条件 → telegram 收到 `[SEVERITY] name: message`（装配/适配/发送三层均有测试实证；W1 修复后含特殊字符的告警不再被 Telegram 400 拒收）
- ✅ 重启进程 `/api/v1/signals` 仍能查到重启前信号（TASK-301 持久化测试 + TASK-302 装配往返实证）
- ✅ 每波提交前 `gitnexus_detect_changes` 核对（3 次波次门禁 + 1 次修复轮门禁，全部符合预期范围）

## 3. 质量数据

- 覆盖率（变更包）: config 94.7% ｜ storage/signal 89.7% ｜ alert 92.3% ｜ metrics/notifier 全绿 ｜ cmd/atlas 整包 65%（package main 存量样板，按先例裁决 cov_min=35，新增代码逐函数 93~100%）
- `go test -race`: alert / storage/signal / cmd/atlas 关键包全绿
- 返工: TASK-202 rework=1（QA W1）；其余 0。独立 reviewer 反审 + 4 次实施期裁决（AD-13a/14a/键名更正/闭区间）均以 DoD 澄清闭环，未消耗返工额度
- QA 两轮审查（常规 + 四视角对抗）: 0 CRITICAL；3 WARNING 中 W1/W2 本轮修复并复验，W3 转 backlog

## 4. 行为变化（运维须知）

1. **信号存储缺省从内存变 sqlite**（`data/signals.db` 自动落盘；opt-out: `storage.signals.backend: memory`；打开失败启动即退不降级）
2. **broker.provider 缺省 futu→mock**；`mode: live` 现直接报 `live trading not supported (paper-only)`
3. alert 规则不再是死配置：`alerts.enabled: true` 即生效（缺省 false 零变化）

## 5. Backlog（QA/测试非阻断项，未消化）

- W3: metadata JSON 往返 int→float64 类型强制未在契约钉死（memory 侧归一化属行为变更，建议独立立项）
- S1 sqlite `SetMaxOpenConns(1)` 消除潜在 BUSY ｜ S2 alert goroutine WaitGroup ｜ S3 运维文档补 WAL（-wal/-shm）备份与相对路径说明 ｜ S4 DSN URI 转义 ｜ S7 契约测试并入跨时区/亚秒用例（已独立实证正确）
- retention 清理（设计 §5 明确留 backlog）
- 用户本地 gitignored `configs/config.yaml` 仍 `provider: "futu"`（enabled=false 无害，建议手动改 mock）

## 6. 流程记录

- validator / arcforge-write.sh 缺失 → 降级执行（Leader 手工任务图校验 ×4、with-task-lock 临界区、epoch 认领协议全程无双写）
- QA 对 W1/W2 修复的最终确认以 test-agent 逐条复验 + 用户合并决定为准（用户 2026-07-03 指令直接合并 #42/#43）
- PR#41 合并时 `--delete-branch` 误关堆叠 PR#42 的事故已修复并记录教训：先 REST API retarget 下游 PR，再删分支
