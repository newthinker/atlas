# Changelog — Sprint M4（2026-09-17/18）

范围 `24c2702988f851b72d5e218b2a7811f3029185a1..bd31159ba70b9d664fed5b66b787b26a0a26cfb8`

## 新增

- **`internal/hestia/queue_health.go`**：`QueueHealth` / `QueueHealthOf(dir)` —— 契约队列四个子目录的件数与最旧年龄，**纯文件系统读**。
  目录读不到即返回错误，不退化成零件数（「正常空」与「被删空」在件数上同形，`deploy.sh` 删队列那次三天无信号）。
  计件只认常规文件，与 `hestia-warp-trigger.sh` 的 `find -type f` 同口径；条目在 ReadDir 与 Info 之间消失（正常流转）不算错误。
  `QueueHealth.ByState()` 是消费侧的唯一状态口径。
- **`internal/metrics` 指标**：`hestia_queue_items{state}`、`hestia_queue_pending_age_hours`、`hestia_queue_processing_age_hours`、
  `hestia_queue_errors_total`、`hestia_queue_up`、`hestia_db_up`。
- **`configs/config.example.yaml` 五条告警规则**：`hestia_queue_stuck`、`hestia_queue_failed`、`hestia_queue_processing_stuck`、
  `hestia_db_blind`、`hestia_queue_blind`。
- **`scripts/ops/hestia-warp-trigger.sh`** 与 `hestia-warp-trigger-test.sh`：先判队列再唤起 agent（队列空时零 agent 调用；
  `processing/` 非空即跳过，不引锁文件），自测用 `TRIGGER_CMD` 桩，不碰真 nanoclaw。
- **`deploy/launchd/com.newthinker.atlas.hestia-warp.plist`**：每 30 分钟一次，`RunAtLoad=false`；已登记进 `install-services.sh`。
- **守卫**：`TestHestiaWarpPlistSetsNoProxyKeys`（按 `_proxy` 后缀判 + 肯定式锚点）、`TestQueueHealthByStateCoversAllStates`、
  `TestHestiaCollector_QueueItemsFollowByState`。

## 变更

- **`Registry.Snapshot()`**：带 `state` label 的 gauge/counter 额外累加到 `<name>_<state>`（原求和键保留）。
  理由：告警求值器只认 `metric op number`，不支持 label 选择器；而 `hestia_queue_items > 0` 会因历史 `done/` 项恒真。
- **`NewHestiaCollector`**：新增 `queue QueueFunc` 参数（nil = 不采队列）；`Collect` 拆成 `collectDB` 与 `collectQueue`
  两个方法，使「一侧失败不影响另一侧」成为结构事实。
- **`cmd/atlas/hestia_health.go`**：队列目录取自 hestia 配置 `queue.dir`，每轮现读；队列目录缺失**不阻断 serve 启动**
  （与「库打不开即启动失败」刻意不同——队列缺失是运行期可告警事实）。

## 未变更（刻意）

- `configs/config.yaml`（仓库内未被 git 跟踪）与 runtime 那份：**由人类同步**，见 final-report 第 4 节。
- 未执行任何 `launchctl` / `install-services.sh`；`~/Library/LaunchAgents/` 无本 sprint 新建文件。
- `scripts/ops/hestia-warp-trigger.sh` 的计件注释：措辞与 `find -type f` 一致、无错误陈述（Leader 裁决，QA 复核成立）。

## 与计划的偏差

| 计划 | 实际 | 依据 |
|---|---|---|
| `hestia_queue_items{state="failed"} > 0` | `hestia_queue_items_failed > 0` | 求值器不支持 label 选择器（人类裁决 R1） |
| 一条 `hestia_health_blind`（含 `or`） | 拆两条 `hestia_db_blind` / `hestia_queue_blind`，用 `*_up == 0` | 求值器不支持 `or`；累计计数器不可恢复（R2） |
| plist 写入 `~/Library/LaunchAgents/` | 入 `deploy/launchd/` + install-services.sh | 仓库约定（R3） |
| C8「触发器要走代理」 | **不设任何代理键** | 代理键到不了容器 agent（R5，nanoclaw@e7c6278 取证） |
| TASK-003 的「queue.dir 为空则跳过」 | 不写该分支 | `internal/hestia/config.go:257` 拒绝空值，分支不可达 |
