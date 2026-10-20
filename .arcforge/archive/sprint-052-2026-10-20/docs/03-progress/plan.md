# Sprint 进度 — 2026-10-20 Pricewatch

> **Step 7 交付：等人类终验收**——QA 三轮（REJECT → REJECT → **PASS**）；11/11 verified；final-report / changelog 已落盘（docs/06-acceptance/）；验收前漂移核对 9×0、2 处归因于 simplifier 提交。
> 确认门已放行（人类 2026-10-20，dod-gate-release.json；P7 裁决 KeepAlive true，AD-27）。

- 需求：spec `docs/superpowers/specs/2026-10-20-pricewatch-design.md` + plan `docs/superpowers/plans/2026-10-20-pricewatch.md`
- 分支：`feature/pricewatch`（自 master `fb1fc62`；spec+plan `38e69db`；TASK-001 `f2fe680`、011 `d861889`、003 `e4d0aeb`、002 `f3d3223`、011 返工 `ccb5ff2`、010 `cea96ad`、005 `49f4b71`、004 `80a56f3`、006 `04bed52`、007 `8cf608a`、007 返工 `e9b963b`、008 `0f218bb`、008 返工 `1244e2a`、009 `5fc087c`）；调度 `dag`
- 设计/决策：`.arcforge/docs/01-design/`（AD-1..AD-28）；追溯矩阵：`02-plan/requirement-dod-matrix.md`（80 条 DoD，孤儿 0，凭空 0）

| 任务 | 标题 | 依赖 | wave | 状态 | 负责 | 验证 |
|---|---|---|---|---|---|---|
| TASK-001 | 规则计算与分档状态机 | — | 1 | verified | dev-pw-a | test-pw-a |
| TASK-002 | 配置加载与校验 | 001 | 2 | verified | dev-pw-a | test-pw-a |
| TASK-003 | SQLite 存储 | 001 | 2 | verified | dev-pw-b | test-pw-b |
| TASK-011 | 健康追踪器 | 001 | 2 | verified（返工 1） | dev-pw-c | test-pw-b（首轮 test-pw-a rejected） |
| TASK-004 | K 线分段补数 | 001, 003 | 3 | verified | dev-pw-b | test-pw-a |
| TASK-005 | 消息文本 | 001, 002 | 3 | verified | dev-pw-c | test-pw-b |
| TASK-010 | 窗口统计 | 001, 003 | 3 | verified | dev-pw-a | test-pw-a |
| TASK-006 | AI 下单建议 | 002, 005 | 4 | verified | dev-pw-c | test-pw-b |
| TASK-007 | 单资产检测与健康 | 002–006, 010, 011 | 5 | verified（返工 1） | dev-pw-a | test-pw-b（首轮 test-pw-a rejected） |
| TASK-008 | 调度与每日心跳 | 007 | 6 | verified（返工 1） | dev-pw-b | test-pw-b（首轮 test-pw-a rejected） |
| TASK-009 | 子命令与 launchd | 008 | 7 | verified | dev-pw-c | test-pw-b（12:09 自 test-pw-a 改派：80 分钟零落盘） |

## QA 必办（非 DoD，Step 6 走 review_fix）
- 🔴 QA-14 plist 占位值未替换 ⇒ 所有发送静默失败（推荐 LoadConfig 拒绝 REPLACE_WITH_ 前缀）
- QA-13 回填顺序传给 Syncer 无测试；QA-15 测试 SetContext 未复原
- QA-12 plist 加 ExitTimeOut（降低 SIGKILL 致重发窗口），低
- 🔴 QA-9 人类已裁决：AD-29（近期段 100%）
- QA-11 TASK-008 生产路径吞 Engine 错误无守卫（once 全失败仍 exit 0；009 派发已要求传错误）
- QA-10 TASK-006 两条 advice 只取第一条存活（已要求 007 补 2→2）；JSON 示例写死「低买」存活
- 🔴 QA-9 Leader 设计缺陷 AD-22×AD-13：段 ≥90% 跳过 vs 窗口 >10% 跳规则 ⇒ 近期缺口永不修复（需人类拍板修法）
- 🔴 QA-8 TASK-004 上线日两道保护（报错不得记上线日）无测试守卫，S29/S30 存活
- 🔴 QA-6 TASK-005 FormatAlert 整条 ≤4096（Term 等 AI 字段无界 ⇒ 400 ⇒ 提醒丢失）；QA-7 价格舍入跨档外观
- QA-5 TASK-010 上线日键按 bar 区分无守卫（X25）+ ListingKey/listingKey 收敛
- QA-4 TASK-002 Rejects 10 行字样断言被路径满足
- QA-3 TASK-003 WAL 无守卫等（见 wisdom）
- QA-1 TASK-001 ParseRule 对 NaN/Inf 放行；QA-2 Decide SentInTier 2→3 用例（见 wisdom/decisions-leader.md）

## TASK-007 派发备忘
- TASK-006 interfaces_exposed：Advise 返回 nil error 时不保证每个方向都有一条、也不去重 ⇒ Engine 拼消息前自行处理
- context_from 全部 7 个上游 discovery；QA-6/8/9 不在 007 范围

## TASK-008 派发备忘
- TASK-007 interfaces_exposed：同一资产不应并发调用 CheckAsset ⇒ Runner 对同一资产串行（上一轮未结束不得再起）

## TASK-009 派发备忘
- TASK-008 interfaces_exposed：run/once 下 Runner 的构造与调用；heartbeat_date 键；同一资产不得并发
- AD-20：取价顺序 = 配置 sources；K 线回填顺序 = 同组 provider 按历史长度（binance 先）——TASK-004 的上线日判定依赖此序
- AD-27：KeepAlive true + ThrottleInterval 60（人类裁决）；绝对路径 + WorkingDirectory；仓库内只放占位
- AD-19 冒烟；coverage_floor 78；SIGTERM 退出最长约十几秒（provider HTTP 10s）
