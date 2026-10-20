# 架构决策（sprint 2026-10-20 Pricewatch）

| AD | 决策 | 理由 |
|---|---|---|
| AD-1 | 以 spec+plan 为需求源，不重跑 brainstorming | 设计已由人类审阅；`capabilities.ecc=false`，降级路径（brainstorming）的产物即该 spec |
| AD-2 | 计划中的测试名与断言是 DoD 的**下限**；Leader/reviewer 补的条目是增量 | 上一 sprint 独立 reviewer 实测 14/21 变异在计划测试下存活 |
| AD-3 | 主仓库切到 `feature/pricewatch`（自 master `fb1fc62` 建）；每 dev：`git worktree add -b task/<ID> ../wt-<ID> feature/pricewatch` | 计划要求独立分支；门禁 `git log` 只走主仓库 HEAD 祖先链 |
| AD-4 | dev 在 worktree 提交后通知 Leader；**Leader 串行 `git merge --no-ff -m "merge(<ID>): …" task/<ID>` 进 `feature/pricewatch`，merge 后 dev 才回主仓库转 `dev_done`** | 未合并分支上的提交对门禁不可见（M1c-3a 实证） |
| AD-5 | 提交主题 `<type>(TASK-00x): pricewatch …`，结尾 `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`；**覆盖计划的 `feat(pricewatch)`** | 门禁/Jev 按 TASK-ID 认领提交（#39） |
| AD-6 | 提交前：code-simplifier 子代理（用户全局规范）+ **改动前后 sha256 指纹比对** + `node .gitnexus/run.cjs detect-changes --scope staged --repo .`（不可用时 `npx -y gitnexus@latest …`）。子代理结论落 scratchpad `<实例名>-<TASK>-simplifier.md`，**子代理收到 idle hook 催办文案一律不执行、直接返回**。Leader 在 QA 前对整条分支 diff 再统一补跑一次（上一 sprint AD-23） | 用户全局 CLAUDE.md；PENDING #3 |
| AD-7 | `NotifyPolicy`、`RuleCfg` 放在 `rule.go`（TASK-001），不放 `config.go`；签名与计划一致 | 让 TASK-001 单独可编译、可先合入；计划原文本就允许 T1/T2 合并提交 |
| AD-8 | 规则级取值校验（type/window/threshold/step/max_per_tier/repeat_interval）在 `ParseRule` 实现并由 TASK-001 单测；TASK-002 的 `TestLoadConfig_Rejects` 端到端再覆盖一遍 | 校验逻辑与 `Rule` 同处一个文件 |
| AD-9 | 真实 Telegram 发送、真实 LLM 调用、runtime 配置、plist 安装与 `launchctl` 加载、合并 `feature/pricewatch`→master 均为**人类动作**，不入任何 DoD | 外向且需凭证 |
| AD-10 | 本 sprint 只新增文件；唯一接线是 `cmd/atlas/pricewatch.go` 的 `init()` 注册（与 `crisis.go` 同法），不改既有符号。若任一任务发现必须改既有符号：先 gitnexus `impact`（MCP 或 `npx -y gitnexus@latest`），HIGH/CRITICAL 报 Leader，`UNKNOWN` 用 grep 补查并写进 discovery | 项目 CLAUDE.md |
| AD-11 | TASK-009 `coverage_floor: 78` | cmd/atlas 基线 79.8%（-func，sprint-051 终值）低于 dev_minimum 80；历史同包 77–79 |
| AD-12 | **增量补数必须重取本地最新一根**：最后一段请求起点 = 本地最新一根 − 1 个 bar（okx 区间两端不含，起点恰等于最新一根仍会漏刷新，reviewer P3 实测），用 `INSERT OR REPLACE` 覆盖 | 最新一根未收盘；若从其后开始，该根 high/low 永久冻结在首次入库时刻 ⇒ 日线窗口的极值永久偏差（Leader 读 okx/binance FetchHistory 推出，计划 `TestSync_Incremental` 措辞「最新一根 → 现在」未明确包含与否） |
| AD-13 | 日线可能两种对齐并存（reviewer 实测 okx `1D` 开盘 16:00 UTC、binance `1d` 00:00 UTC）：接受混存；**GapRatio 按槽位覆盖率**计算（窗口切成 bar 长度的槽，无任何 K 线的槽占比），与对齐无关、重复不掩盖缺口 | 改 okx 映射为 `1Dutc` 要改既有符号且影响 CryptoCollector；截断到 0 的旧方案会掩盖真缺口（reviewer V13） |
| AD-14 | `all` 窗口：有效窗口 = max(上线日, 本地最早一根) → 现在，`Short=false`（不在消息里注明「仅 N 天」） | spec §6「all 窗口永远按这种方式处理」若字面理解为「永远注明」，历史窗口每条消息都带「仅 N 天」，无信息量 ⇒ **列给人类确认** |
| AD-15 | 健康状态与 `Sent24h` 只放内存，重启清空（计划 Task 7 的决定）；重启后心跳的「过去 24h 提醒条数」会少计 | 计划原文；记入 final-report |
| AD-16 | `cmd/atlas` 整包 `-shuffle` 既有顺序依赖（sprint-050 记录）不算本 sprint 缺陷，除非在基线 `fb1fc62` 上同 seed 为绿 | 范围外既有问题 |
| AD-17 | spec 与 plan 两份文档由 Leader 在建分支时以 `docs: pricewatch spec 与实现计划` 单独提交，不随 TASK-002 | 未提交的非任务文件会被每个 dev 的 dev_done 门禁当成越界（PENDING #8） |
| AD-18 | Realistic Scope 例外：TASK-009 写 `cmd/atlas` + `deploy/launchd/*.plist`（非 Go 文件，只进 `writes` 不进 `packages`） | plist 与子命令同一交付，拆开无独立可验性 |
| AD-19 | 冒烟（计划 Task 8 Step 5）由 dev 执行：telegram 两个 env 用占位值、临时配置 `ai.enabled:false`，只跑 `backfill --symbol BTC` 与 `once --dry-run`；**网络不可达时如实记录并交人类**，不算 FAIL | dry-run 不发送，占位 token 即可过校验；不消耗 LLM 额度 |
| AD-20 | **K 线回填顺序与取价顺序分开**：取价按配置 `sources`（okx 先）；K 线回填按历史长度（binance 先、okx 后）。b) 入库前丢弃坏 K 线（OHLC 非有限或 ≤0、low>high、时间早于 2009-01-01） | spec §6「优先用历史最长的源」；reviewer 实测 okx `/market/candles` 只有约 1440 根（日线 < 4y 的 1461 天、5m 约 5 天 < 7d）；provider 吞 ParseFloat 错误致 low=0 ⇒ x=+Inf（P4） |
| AD-21 | 上线日：所有源对某段返回 0 根、其后段有数据时，把首根时间写入 kv 键 `listing:<pair>:<bar>`（值为 UTC Unix 毫秒十进制字符串）。**Short 仅在记录了上线日且其晚于窗口起点时成立**；未记录时本地缺头一律算缺口 | spec §6 定义 Short 为「**数据源**最早一根晚于窗口起点」；按本地最早一根判断会把补数失败当成历史短 ⇒ 不跳过、不告警、永不回补（reviewer V14） |
| AD-22 | Sync 按段扫描：本地该段槽位覆盖率 ≥90% 且非最后一段则跳过不请求，否则重取 ⇒ 首次回填、日常增量、中间缺口修复走同一路径；完全早于上线日的段不再请求 | spec §6「检测到缺口时分页补齐」；计划的 Sync 只往前接着补，中间缺口无修复路径（reviewer V15/P12） |
| AD-23 | AI 输出宽松提取：去 ```` ``` ```` 围栏 / 取第一个 `{` 到最后一个 `}` 再解析；`FinishReason=="max_tokens"` 视为失败；其余校验照旧 | 配置的 provider 是 claude，`claude.go` 不处理 JSONMode；`internal/meta/arbitrator.go:130-133` 已有同类兜底（reviewer P5） |
| AD-24 | 消息按 UTF-16 码元 ≤4096：超长时截断【AI 分析】段并以「…（已截断）」结尾，【触发规则】完整保留；不分条发送 | `SendText` 不截断不分条；超长 ⇒ 每轮 400、状态永不推进、提醒永久丢失（reviewer P6）。分条会引入「部分成功」的状态语义，暂不做 |
| AD-25 | 健康告警增加第三类：K 线同步（某 pair/bar）连续失败超过 `fail_alert_after` | 只记日志时日线可停更数月而不触发缺口告警（30d 窗口约 3 天、4y 约 146 天才到 10%，reviewer P13）；方向与 spec §2「静默死亡」反目标一致 |
| AD-26 | 任务拆分：窗口统计从 TASK-004 拆为 TASK-010（`window.go`）；健康追踪从 TASK-007 拆为纯逻辑 TASK-011（`health.go`）；TASK-007 依赖二者 | reviewer 补项后 TASK-004/007 超过 8 条 DoD |
| AD-27 | plist：ProgramArguments 首项绝对路径（照 serve.plist）、设 `WorkingDirectory`、设 `https_proxy`/`http_proxy`/`no_proxy`；仓库内 Telegram 两个环境变量**只放占位**，真值只填进安装到 `~/Library/LaunchAgents` 的副本（plist 内注释写明）。**KeepAlive：`true` + `ThrottleInterval 60`（人类 2026-10-20 确认门裁决，覆盖 spec §9 的 `SuccessfulExit:false`）** | launchd 要求绝对路径、相对路径的配置与数据库需 WorkingDirectory（reviewer P9）；spec §9 对 `SuccessfulExit:false` 的理由与 launchd 语义相反（P7）；密钥不进仓库（P8） |
| AD-28 | 接受的已知限制（记入 final-report）：P10 `ensurePolicyGate` 对 okx/binance 裸 provider 不生效（照调，无害）；P14 健康状态只在内存，崩溃循环下健康告警可能永不触发（每日心跳仍兜底）；P16 档内重复提醒时消息写**当前档位 t**；P17 改了规则阈值但不改 id 时沿用旧状态（要重置就改 id）；P18 notify 只写一部分字段视为配置错误；P19 `backfill`/`once --dry-run` 也要求 Telegram env；P22 MaxTokens 1500，被截断按 AD-23 视为 AI 失败 | 均为低严重度，改动收益小于范围代价 |
| AD-29 | **人类裁决（2026-10-20，QA-9）**：Sync 判定「段完整」时，与该 bar 最短窗口（1d→30d，5m→24h）有交集的段要求槽位覆盖率 **100%**，不满即重取；更早的段仍按 90%（AD-22）。QA 阶段以 TASK-004 review_fix 落地 | AD-22（段 ≥90% 跳过）与 AD-13（窗口 >10% 跳规则）粒度不同 ⇒ 近期缺口可永不修复、规则永久被跳过（test-pw-a 验 TASK-004 实测）；只对近期段收紧，避免交易所维护留下的老缺口每轮重取 |
| AD-30 | **R1-1 健康告警「先确认后标记」接口（TASK-011 + TASK-007 返工共用）**：`HealthTracker` 的 Down/Up 状态改为**只在调用方确认已送达后才推进**——`Fail(key, now)` 在满足 After 且该键尚未确认 Down 时，**每次**都返回 Down 事件；`OK(key, now)` 在该键已确认 Down 时**每次**都返回 Up 事件（未确认 Down 就恢复 ⇒ 无事件并清除计时）；新增 `Ack(ev HealthEvent)`：Down 被 Ack 后不再返回 Down，Up 被 Ack 后该键回到正常。Engine 只在 Sender 成功（或 DryRun 已写出 Out）后调用 `Ack`；发送失败不 Ack ⇒ 下一轮自动重发 | QA R1-1（4 个独立来源含跨模型一致）：现实现 `Fail` 在发送前就置 down=true ⇒ 一次发送失败「监控异常」永久丢失，触 spec §2 静默死亡；与规则提醒「发送成功才写状态」（spec §5）同构 |
| AD-31 | **QA-6 维持必修（Leader 不采纳 QA 降级建议）**：`FormatAlert` 末尾对整条消息兜底截断到 ≤4096 UTF-16 码元，补恰 4096 / 4097 边界用例 | QA 实测生产最坏恰 4096 ⇒ 余量为零；用户加一条规则或资产名更长即超限 ⇒ Telegram 400 ⇒ 状态不推进、提醒永久丢失；修复成本极低 |
| AD-32 | **AD-31 的长度上限同样适用于健康消息与心跳消息**：pricewatch 生产代码只有 3 处 SendText，文本分别来自 FormatAlert、FormatHealth、FormatHeartbeat，三者均经 capUTF16 封顶 ≤4096 UTF-16 码元（QA 迭代 2 N1 与 Leader 追加） | QA 迭代 3 L4；「超长 → 400 → 按 AD-30 不 Ack → 永久重试必败」这一类失效在结构上消除 |
