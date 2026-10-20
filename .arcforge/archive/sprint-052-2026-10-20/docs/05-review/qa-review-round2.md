# QA Review 第 2 轮（跨视角对抗审查）— pricewatch sprint

- 审查者：qa-pw ｜ 日期：2026-10-20
- 对象：`fb1fc62c2a795c470a9baeafd1bdd9502efa317d..53d255acd5368815720f464818f6c8d9eccee987`
- 第 1 轮报告：`.arcforge/docs/05-review/qa-review-round1.md`（含 QA-1..15 逐条判定、两个 simplifier 提交审查）

## 0. 审查组成
| Lens | 形态 | 结果 |
|---|---|---|
| Skeptic | Claude 只读子代理，独立 context；worktree 内探针后已恢复（HEAD 不变、porcelain 0 行，我已复核） | 4 条（1 CRITICAL / 1 WARNING / 2 LOW），全部实测 |
| Architect | Claude 只读子代理；`go test -overlay` 探针，未落文件 | 9 条（2 WARNING / 7 LOW）；Review Focus 1–5 逐条作答 |
| Minimalist | Claude 只读子代理 | 9 条（3 WARNING / 6 LOW，我统一降为 LOW，理由见 §3） |
| **codex（跨模型）** | `codex exec -s read-only`。40s 探测可用；第 1 次正式调用因 sandbox「code-mode host 协商超时」未能读库、**零发现**；第 2 次把 spec+全部生产源码内联经 stdin 喂入（无工具），成功返回 | 2 条 WARNING |

（规模 200+ 行 ⇒ 三个 lens 全上；子代理结论均落盘 scratchpad 后由我读取，不依赖返回值。）

## 1. 共识矩阵（high-severity 与本轮新增）

| ID | 发现 | Skeptic | Architect | Minimalist | codex | 本人复核 | 最终 |
|---|---|---|---|---|---|---|---|
| R1-1 | 「监控异常」一次发送失败即永久丢失（health.go:51-55 + engine.go:293-300） | CRITICAL（实测：gap 故障 + 第 30 分钟 Telegram 失败 ⇒ 0–120 分钟仅 1 次发送尝试，心跳 `Active:[]`、`LastOK` 新鲜，看起来健康） | WARNING（实测：前 35 分钟 Telegram 报错 ⇒ 2h 内只送达「✅ 已恢复」，没有对应的异常） | — | WARNING（独立发现） | 读代码确认 | **WARNING，必修**；3 个独立来源 + 跨模型一致 ⇒ 共识。级别分歧（CRITICAL vs WARNING）不影响「必修」结论：不是 CONTESTED |
| R1-5 | 主回填源故障对 `sync:` 健康不可见、每轮重取老段（sync.go:173-197）；okx 替身无 1440 上限 | WARNING（实测） | — | — | — | 读代码确认 `anyOK` ⇒ 不返回 error | **WARNING，必修**（同 TASK-004 返工一并修） |
| R1-2 | 非严格 Unmarshal ⇒ `heartbeat_at` 拼错静默关心跳（config.go:71） | — | WARNING（实测） | — | — | overlay 验证 `UnmarshalExact` 对两份现有配置 err=nil，修法可行 | **WARNING，必修** |
| R1-4 | AI 只回一个方向即通过（advisor.go:122-139） | — | — | — | WARNING | 读代码确认；提醒照发不触反目标 | **LOW** |
| R2-1 | 只配 `sources: [okx]` 时 okx 1440 根上限被记成上线日、永久（sync.go:97-113） | LOW（实测：上线日记为 2022-11-10） | — | — | — | 读代码确认；默认配置 `[okx, binance]` 不触发 | LOW |
| R2-2 | `step_pct` 很小时 0→≥2 档直接发、绕过 repeat_interval（state.go:30-33） | LOW（实测：threshold 2 / step 0.5，1h 内 6 条） | — | — | — | 与 spec §5 第 2 条字面一致（「跌进第 2 档以上不压」）⇒ 是 spec 隐含假设「step ≫ 5 分钟噪声」未被校验 | LOW；修法需人类决定（最小 step 或改 spec） |
| R2-3 | 健康告警按「资产×规则」粒度，单一根因一次可出 ~8 条异常 + 8 条恢复（engine.go:84、101） | — | LOW | — | — | 每事件一次，不是每 5 分钟 ⇒ 非 spec 定义的刷屏 | LOW |

**本 sprint 同族模式专项扫描（行为只在替身/不真实条件下被测）**——本轮新找到 2 处：
1. **健康告警只在「发送成功」的 sender 下被测**（engine_test.go:544）⇒ 掩盖 R1-1。
2. **okx 替身缺 1440 根上限**（sync_test.go 的 `okxSem` 只模拟两端不含 + 300 根/次）⇒ 掩盖 R1-5、R2-1。
另复核旧条目：QA-11 在系统级已被 cmd 层真实 Engine 用例杀死（变异实测，见第 1 轮）；Engine 的 Sync+WindowStats+Decide 联合生产路径只在 cmd 层 `pwFakeProvider`（全部 bar 对齐 00:00、无缺口）下跑过，未见缺陷但覆盖弱，记 LOW 不派修。

## 2. Review Focus 1–5（plan）——采纳 Architect 作答，我抽查确认
1 阈值边界：通过（Tier 有 NaN/Inf 拦截与 1e-9 容差；ParseRule 侧 QA-1 仍在）。2 重复/乱序 K 线：通过（主键 + INSERT OR REPLACE，缺口按槽去重）。3 行情中途重启：通过（规则状态落库；健康状态只在内存为 AD-15 已接受）。4 Telegram 发送失败：**规则提醒通过，健康告警不通过（R1-1）**。5 价格 0/NaN/Inf：通过（engine.go:261 与 sync.go:236-243 两侧过滤）。

## 3. 降级/不采纳说明
- Minimalist 的 F1（`Store.Stats`/`CandleStats` 仅 backfill 用 `.Count`）、F2（`backfillStart` 与 `readListing` 重复解析同一 kv）、F3（槽位计数三套实现，sync 侧分母向下取整、末尾半槽进分子不进分母，覆盖率可 >1）：都是真实的重复/口径不一，但**不改变任何判定结果**（F3 对 `≥0.9` 判定无害，Minimalist 自述为推理未实测）⇒ 统一 LOW。F2 建议在 TASK-004 返工时顺手让 `backfillStart` 调 `readListing`。
- Architect #7 / Minimalist F6（window/sync/engine 绕过 Store 直接写 SQL）：风格与分层问题，LOW。
- Architect #8（Engine 写死 crypto）：spec 第一版只要求配置格式留位，LOW、留待以后。

## 4. 总判定

**VERDICT: REJECT**

理由：存在多个**有共识**的必修项——QA-8（我独立变异复现）、QA-9/AD-29（人类已裁决、代码未落地）、QA-14、R1-1（4 个独立来源含跨模型一致）、R1-2、R1-5。无 reviewer 间对「是否需修」的分歧 ⇒ 不是 CONTESTED。
唯一与 Leader 定级不一致处：**QA-6 我建议由必修降为 LOW**（生产路径最坏实测整条恰 4096，证据见第 1 轮 §2）——这是给 Leader 的定级建议，不影响 REJECT 结论。

建议 reason_class：`task_defect`（全部为实现/测试缺陷；无 DoD 自相矛盾项。QA-9 源于 Leader 设计层 AD-22×AD-13 冲突，但人类已裁决方案 A，现为实现未落地）。

## 5. fix_items 汇总（任务 ID → 条目）

| 任务 | 必修（WARNING） | 顺手修（LOW，可选） |
|---|---|---|
| **TASK-004** sync.go | ① QA-8：补两条上线日保护用例（源码在 `.arcforge/docs/02-plan/qa-8-probe.md`，S29/S30 我已复现被杀）+ 第三个同类变异「跳过的完整段未让上线日判定停下」用例；② QA-9/AD-29：与最短窗口（1d→30d，5m→24h）相交的段要求 100% 覆盖，早期段仍 90%；③ R1-5：`fetch` 在 best 覆盖 <90% 且有源报错时，照常返回 best 入库**并**上抛错误使 `sync:` 健康键可报；okx 替身加 1440 根上限并补「binance 全挂」用例 | S33/S11（QA-8 低优先）；R2-1（单 okx 源时上线日误记）；Minimalist F2（backfillStart 复用 readListing） |
| **TASK-011** health.go + **TASK-007** engine.go | ④ R1-1：Down 只在发送成功（或 DryRun 写出）后确认；发送失败下一轮重发；补「第 30 分钟发送失败、之后恢复 ⇒ 补发异常」用例（用真实 HealthTracker + 会失败的 sender，不得用替身遮蔽） | R1-6 心跳列出当前 Down 键（TASK-007 Snapshot + TASK-005 格式）；WindowStatsFor 报错计入健康键；状态写入事务化 |
| **TASK-002** config.go | ⑤ QA-14：拒绝以 `REPLACE_WITH_` 开头的 telegram 值，报错指向 plist 注释；⑥ R1-2：`v.UnmarshalExact`（已验现有两份配置可过）+ 未知键报错用例 | QA-4 断言剥路径前缀；R1-3 校验 `fail_alert_after>0`、`ai.timeout>0` |
| TASK-001 | — | QA-1 NaN/Inf；QA-2 补 2→3 用例；R2-2（需人类决定） |
| TASK-005 | — | QA-6（降级后）：FormatAlert 末尾整条兜底截断 + T34 恰 4096 用例；QA-7；R1-6 格式 |
| TASK-006 | — | R1-4 缺方向写「AI 分析不可用」；QA-10 A04 |
| TASK-008 | — | QA-11 包内补真实 Engine 断言（系统级已闭合） |
| TASK-009 | — | QA-12 ExitTimeOut；QA-13 断言 `app.syncer.Providers` 顺序；QA-15 SetContext 复原 |
| TASK-010 | — | QA-5 测试另一半「5m 窗口读 5m 键 → 判 Short」 |
| TASK-003 | — | QA-3 |

（TASK-004 与 TASK-011/007 的必修项互不重叠，可并行返工；TASK-011 与 TASK-007 的 ④ 是一个接口改动的两侧，建议派给同一个 Dev。）
