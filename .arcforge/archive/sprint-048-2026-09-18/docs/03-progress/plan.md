# Sprint M4 — Hestia 队列可观测与自动触发

> 状态：**QA 返工中**。7 任务均已首轮 verified；QA verdict PASS（有条件），W-1/W-2/W-5/W-6 本轮修（人类批准）。

| 任务 | 标题 | 状态 | owner / verifier |
|---|---|---|---|
| TASK-001 | QueueHealthOf（+W-1/W-2/W-6 返工） | verifying | dev-m4-c / test-m4-a |
| TASK-002 | Snapshot 展开 state label | verified | dev-m4-b / test-m4-a |
| TASK-003 | collector 队列指标 + up gauge | verified（待开 W-2 collector 侧返工） | dev-m4-b / test-m4-a |
| TASK-004 | serve 接线 | verified | dev-m4-a→dev-m4-b / test-m4-a |
| TASK-005 | 五条告警规则 | verified | dev-m4-b / test-m4-b |
| TASK-006 | Warp 触发脚本 | verified | dev-m4-c / test-m4-a |
| TASK-007 | plist 入库（+W-5 守卫返工） | verifying | dev-m4-b / test-m4-b |

当前 HEAD e6f4e9d493fb9e3bcebb0adef9a9edc3fbae8101。master 合入序：TASK-002 8cd4306 → 006 d44e389 → 001 717c627 → 007(r1) 1a89e27 → 003 8b1b999 → 004 415c53e → 007(r2) c4d651c → 007(r3) 6d5230e → 005 0af7c8d → 005(r2) 4cd6ccc → 007(W-5) 3d94a52 → 001(W-1/2/6) e6f4e9d。

## QA 结论（`docs/05-review/m4-code-review.md`，PASS 有条件，0 CRITICAL / 6 WARNING / 14 INFO）
- W-1 符号链接口径分歧、W-2 状态名副本（hestia 侧）、W-6 Info() 竞态 → TASK-001 返工（已交付，验证中）
- W-5 plist 无代理键只由散文守着 → TASK-007 返工（已交付，验证中）
- W-2 collector 侧改用 ByState() → 待 TASK-001 verified 后开 TASK-003 返工
- **W-3 / W-4 转人类**：processing_stuck 0.5h+for10m 可能常态假红，须在计划判据五实测后校准；「已唤起但尚未进 processing/」的重复唤起窗口属消费者（warp skill）契约，建议要求 skill 首步即 rename。
- Leader 裁决：I-3 `hestia_queue_errors_total` 保留（审计用途，零消费者状态写进 final-report）；不采纳 Minimalist 两条建议（弱化 message 全等 = 回退 TASK-005 已付费的加固；QueueFunc nil 分支三行成本保留）。
- **Leader 裁决（W-1 第二处注释）**：不改 scripts/ops/hestia-warp-trigger.sh。其措辞与 `find -type f` 一致、无错误陈述；且属正在复验的 TASK-007，改它会让判定对象漂移。dev-m4-c 已在 TASK-001 discovery 的 decisions 记同样内容（缺「Leader 裁决」来源，故在此补记）。

## 过程事件（供 PENDING-MECHANISMS 汇总）
- **单向消息失效两例**：dev-m4-a（收不到，4 次批准全丢，经人类批准 TaskStop 改派 dev-m4-b，epoch 收回）；Leader→dev-m4-b/dev-m4-c 回复丢失（两人各催办三次，实际每次都已处理；dev 靠轮询 master 自愈）。stale-dispatch 与 ListAgents 对这两类均不可见。
- **子代理自报与实际不符第三例**：code-simplifier 报「无动作」实改两处（dev-m4-b 靠指纹比对发现，已在最终版本重采数字）。前两例报无改动属实。
- **QA lens 子代理结论丢失**：Skeptic/Architect 两视角完成但未回传，由 qa-m4-a 本体补做并加变异实证，已在报告 §7 注明。
- 写通道限制：收回边（in_progress→assigned）不接受 reason_class；env_infra 归因只能记在此处。
- Leader 失误：一条脚本串联 update+transition，update 被 DENY 后 transition 照跑（已在 assigned 态补写，dev 未受影响）。
- dev-m4-c 变异时 `git checkout --` 抹掉未提交实现（按原文重写后 sha256 逐字节相同，已改用文件快照还原）。
- **待办：轮换 dev-m4-b token**（ps 明文可见）。

## 范围外 / 交人类
- 验收判据二～六生产实测（判据四须同时看脚本 exit 0 且输出含 queue empty；判据五是 `|| true` 与写死 PATH 两条「失败也 exit 0」路径的唯一诚实证明）。
- runtime configs/config.yaml 追加五条规则（取 discoveries/TASK-005.json 的 human_sync_required.paste，用 `jq -j`）；追加后重启 serve，前提是 serve 已部署 TASK-002/003/004。
- install-services.sh 实际装载；CONTRACTS 登记。
- hestia-ingest.plist 头注释过期（第 25/39 行「无代理键」、第 40 行守卫名 TestHestiaPlistSetsNoProxyKeys 已不存在）——计划两条假前提的源头。
- cmd/atlas 覆盖率欠账：门禁口径 78.2%，距 80% 差 31 条语句，缺口在 serve.go(116)/crisis.go(42)/broker.go(34) 等 writes 外文件。
