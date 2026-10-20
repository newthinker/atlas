# QA Review 迭代 2/3（返工 diff 审查）— pricewatch sprint

- 审查者：qa-pw ｜ 日期：2026-10-20
- 对象：返工 diff `git diff 53d255acd5368815720f464818f6c8d9eccee987..8ed58bc17d3179e32c56b9d5caf9199484dcc32e`（13 文件，+650/−115）
- 取证环境：隔离 worktree detached @ 8ed58bc17d3179e32c56b9d5caf9199484dcc32e；`GOTOOLCHAIN=local go test -race ./internal/pricewatch/ ./cmd/atlas/` 两包 ok；全部探针经 `-overlay` 注入，worktree `status --porcelain` 每步核对为 0 行，主工作区未改动
- 审查组成：Skeptic / Architect / Minimalist（Claude 只读子代理，结论各自落盘 scratchpad，我从文件取）+ **codex 跨模型**（源码内联经 stdin，成功返回 1 条）
- 上轮报告：`docs/05-review/qa-review-round1.md`、`qa-review-round2.md`

## 1. 第 1 轮必修 fix_items 闭合表

| # | 条目 | 归属 / 提交 | 闭合？ | 证据 |
|---|---|---|---|---|
| ① | QA-8 上线日保护 | TASK-004 / f8e37c2 | **闭合** | 新增 `TestSync_ListingNotRecordedOnError`、`…AllError`、`…OnPartialWithError`、`TestSync_ListingStoppedBySkippedSegment`（第三个同类变异也覆盖）；sync.go:94-101 报错段 `leadingEmpty=false`、sync.go:123-125 本地已有数据的段也置 false。复验 test-pw-b 39 变异 36 KILLED（存活 3 个为等价） |
| ② | QA-9 / AD-29 近期段 100% | TASK-004 / f8e37c2 | **闭合** | sync.go:196-205 `requiredCoverage`；`TestSync_RecentSegmentsNeedFullCoverage` 1d/5m 两子测试。Architect 对照 AD-29 原文无偏离；Skeptic 实测 5m 起点每轮移动、1d okx 16:00/binance 00:00 混存均**无永久重取**。Minimalist 疑「7d 也用 5m 却只对 24h 要求 100%」——我核算不构成缺口：90% 覆盖的 25h 段最多缺 2.5h，占 7d 的 1.5%，多段合计仍受 `GapRatio > 0.10` 约束，且 AD-29 原文即「最短窗口」 |
| ③ | R1-5 主回填源故障可见 | TASK-004 / f8e37c2 | **闭合，但引入新问题 N1/N2（见 §2）** | sync.go:187-188 有源报错且无源达 90% 时返回 best + error；`TestSync_BinanceDownReportsError`；okx 替身加 1440 上限（Minimalist 核实 `okxHorizon`）。Skeptic 实测稳态下 binance 挂掉 Sync 返回 nil、不误报 |
| ④ | R1-1 / AD-30 健康告警送达后才确认 | TASK-011 77c49ef + TASK-007 8ed58bc | **闭合（R1-1 原路径）；但同一失效模式经新路径复现，见 N1** | health.go:54-93 与 engine.go:293-304 与 AD-30 逐条一致（Architect 对照）；全仓 Fail/OK/Ack 调用点只有 `track`（Architect grep）；`TestCheckAsset_HealthSendFailureRetries` 用真实 HealthTracker + 失败 sender（不再只测顺利路径） |
| ⑤ | QA-14 REPLACE_WITH_ 占位 | TASK-002 / 943dacc | **闭合** | config.go:220-223；`TestLoadConfig_PlaceholderRejected`；复验 M2 变异被杀 |
| ⑥ | R1-2 UnmarshalExact | TASK-002 / 943dacc | **闭合** | config.go:72；Architect 实际加载 `configs/pricewatch.yaml` 与 `testdata/valid.yaml` 均通过；~/Library/LaunchAgents 下无已部署 pricewatch plist ⇒ 无存量部署被误拒 |
| 顺手 | QA-1 / QA-2 | TASK-001 / 56c7c6f | **闭合** | rule.go:89-95 `!(x > 0) || IsInf`；state_test 补 2→3 |
| 顺手 | QA-4 / R1-3 | TASK-002 | **闭合** | 复验 12 个删字段名变异新测试 12/12 KILLED；config.go:101-106 |
| AD-31 | QA-6 整条 ≤4096 | TASK-005 / ebc366e | **闭合，实现合理** | message.go:95-125：先截 AI 分析、再截 AI 结论、最后 `capUTF16` 兜底，无 AI 也兜底；`TestFormatAlert_LimitBoundary/LongAdviceFields/WholeMessage`。Skeptic 对 3500–4300 码元逐档扫描（ASCII / 中文 / 代理对 😀）最大恰 4096、**0 例超限**；Minimalist 核实预算为负时 `truncateUTF16` 返回空串、只剩截断标记再截下一字段，逻辑正确。我在上轮建议降级，Leader 维持必修，现实现无异议 |

## 2. 新发现（修复引入）

### N1 WARNING — 健康消息不限长：R1-5 要报的「binance 不可达」恰好发不出去，且 AD-30 下每轮重发必败（静默死亡经新路径复现）
- 位置：`internal/pricewatch/message.go:216` `FormatHealth`（detail 原样拼接，无长度上限）× `sync.go:142` `errors.Join` 把每个失败段的完整错误（含请求 URL）拼进一条 × `engine.go:300-304` 发送失败不 Ack
- 证据（**3 次独立观察**）：
  - 我的探针：binance 用真实 HTTP 客户端连 `https://127.0.0.1:1`（连接拒绝，真实 `*url.Error` 文本，包装同 binance.go）、okx 成功 0 根、冷启动 1d ⇒ `Sync` err≠nil，`FormatHealth` 结果 **5229** UTF-16 码元。
  - Architect：Go 真实 DNS 超时文本 + okx 1440 上限 ⇒ **4998**；报错较短时 3990。
  - Skeptic：`Client.Timeout exceeded` 文本 ⇒ **4933**；DNS 报错 4005（离上限 91）。
- 后果：Telegram 对超 4096 的 sendMessage 返回 400 ⇒ 不 Ack ⇒ 每轮重发同一条必败消息 ⇒ 用户永远收不到「监控异常」。触发条件恰是 R1-5 要覆盖的场景（冷启动/上线日未记录时 binance 被墙或不可达，约 16 个老段同时报错）。这是 AD-30 要消除的失效，从另一条路径回来了。
- 建议修法（必修）：`FormatHealth` 返回前过 `capUTF16`（一行）；补用例「detail 长 10000 码元 ⇒ ≤4096 且以截断标记结尾」。可选（LOW）：Sync 的聚合错误改为「N 段失败，首个：…」，消息更可读。
- 归属：**TASK-005**（FormatHealth）；可选项归 TASK-004

### N2 LOW — 只在 okx 上市的币会得到一条永不恢复的 sync 健康告警
- 位置：`sync.go:187-188`
- 证据（Skeptic 观察，新旧对照）：binance 回 400 Invalid symbol、okx 正常 0 根，本地历史已完整 401/401 ⇒ 新代码每轮 `err≠nil`，旧代码 `err=nil` ⇒ R1-5 引入的回归；上线日也因报错永远记不下。
- 定级理由：当前配置 BTC/ETH 都在 binance 上，不触发；后果是一条（已 Ack 后不重复）误报，非刷屏。
- 建议：记入已知限制；以后若加 okx 独有币，改为「报错段完全早于本地最早一根且其余源成功 0 根时不计错」。归属 TASK-004。

### N3 LOW — fetch 接受线 90% 与 AD-29 的 100% 不一致
- 位置：`sync.go:180` vs `:127`（Skeptic 观察：binance 自身在近 30 天缺一天 ⇒ 拿到 299/300 即返回、不问 okx，缺口补不上、每轮多 1 请求，1d 最长约 330 天）。不误报、不影响规则（缺 1 天远低于 10%）。属 AD-29 已接受的「每轮多 1 个请求」代价。记入已知限制。归属 TASK-004。

### N4 LOW — AD-30 是「至少一次」送达
- 位置：`engine.go:298-304`（Skeptic 推理）：Telegram 实际送达却返回超时 ⇒ Down 每轮重复一条；此时若在 Ack 前恢复则不发 Up（用户见 Down 不见 Up）。概率低。建议在 AD-30 注明「至少一次」语义。记入已知限制。

### 不采纳
- **codex WARNING「Down 未送达就恢复 ⇒ 状态被删、不发任何事件」（health.go:69-71）**：这正是 AD-30 原文的设计（「未确认 Down 就恢复 ⇒ 无事件并清除计时」），且有 `TestHealth_RecoverBeforeDownAcked` 与 engine 子测试「异常未送达就恢复 ⇒ 不发孤立的已恢复」守卫。用户从未收到异常、故障已消失，此时不打扰是合理取舍，不构成静默死亡（死亡已结束）。不采纳为缺陷，LOW 备注。
- Architect F2（并发注释过时，同键需串行调用）、F3（DryRun Ack 分支在 once 中不可达）、F5（ai.enabled=false 仍要求 timeout>0；有默认 60s 兜底）、Minimalist 全部 6 条：LOW，不派修。Minimalist 逐条核对 health_test 被删/改写断言，均有替代守卫，无行为失去守卫。

## 3. Leader 交来的待定级项

| 项 | 定级 | 理由 |
|---|---|---|
| QA-16 cmd 侧心跳接线无测试 | **记入已知限制（LOW）** | 当前接线正确（cmd/atlas/pricewatch.go:164 `HeartbeatAt: a.cfg.Health.HeartbeatAt`，我读过）；只是回归风险，与 QA-13 同族。建议下次触碰 cmd 时补 |
| QA-17 R6 断言被替身错误文本满足 | **记入已知限制（LOW）** | R1-5 的核心（返回 error）有守卫，只是「错误含源名」这层空洞。同族新例：Architect F6——`config_test.go:333,339` 断言的 `REPLACE_WITH_` 由夹具值 `REPLACE_WITH_SOMETHING` 自带（生产改为回显原值仍绿），LOW |
| TASK-011 迟到 Ack 跨周期误报 | **记入已知限制（LOW）** | 生产不可达：`track` 同步 Fail→send→Ack，Runner 串行（runner.go:16） |
| TASK-004 返工后 backfill 遇 Sync 错误直接 return | **记入已知限制（LOW）** | `cmd/atlas/pricewatch.go:245-247` 本是旧逻辑，R1-5 让 Sync 更常报错而放大：binance 不可达时手工 backfill 在第一个资产/bar 就中止（数据已入库）。手工命令、错误可见；`run` 每轮照常 Sync 不受影响 |
| TASK-007 rework_count 少计 | **不在代码审查范围**（机制问题） | 记 final-report / PENDING-MECHANISMS |

## 4. 总判定

**VERDICT: REJECT**（迭代 2/3）

理由：N1 是 WARNING——修复引入、触 spec §2 静默死亡，**3 次独立观察**（我 + Skeptic + Architect）一致超 4096；codex 未提及但无反对意见 ⇒ 共识，非 CONTESTED。其余第 1 轮必修项全部闭合，无其他 CRITICAL/WARNING。修法一行 + 一条用例，成本极低。

建议 reason_class：`task_defect`。

### 新 fix_items
| 任务 | 条目 |
|---|---|
| **TASK-005** | N1：`FormatHealth` 结果过 `capUTF16` 保证 ≤4096；补用例「detail 10000 码元 ⇒ ≤4096、以截断标记结尾」（用 FormatHealth 真实函数，不经替身） |

可选（不阻断）：TASK-004 让 Sync 聚合错误摘要化（N1 可读性）；N2/N3/N4 及 §3 各项记入 final-report 已知限制。
