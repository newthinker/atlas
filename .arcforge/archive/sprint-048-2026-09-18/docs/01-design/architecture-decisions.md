# 架构决策（M4 队列可观测）

- **AD-1 Snapshot 展开 state label**（人类 2026-09-17）：对带 `state` label 的 gauge/counter 额外累加到 `<name>_<state>`，原求和键保留；仅值匹配 `^\w+$` 时展开。仿 `addStatusClass` 先例。否决：扩展规则求值器（影响全部规则）、独立 gauge（加状态要改多处）。
- **AD-2 up gauge 替代累计计数器做告警**（人类 2026-09-17）：`hestia_db_up` / `hestia_queue_up`，`*_errors_total` 保留作审计。规则拆 `hestia_db_blind` / `hestia_queue_blind`，**不同名**（evaluator 按 rule.Name 存 pending/lastFired）。
- **AD-3 collectDB / collectQueue 两个方法**（计划 D1/C2）：失败隔离成为结构事实。
- **AD-4 队列缺失不阻断 serve 启动**：与「库打不开 ⇒ 启动失败」刻意不同，队列缺失是运行期可告警事实（C3）。
- **AD-5 TASK-003 顺带在 cmd/atlas 传 nil**：签名变更后保持 master 可编译；接线归 TASK-004。
- **AD-6 plist 真相源 deploy/launchd/**（人类 2026-09-17），agent 不执行 launchctl。
