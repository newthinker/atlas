# Final Report — Pricewatch 资产价格波动提醒（sprint 2026-10-20）

- 分支：`feature/pricewatch`，起点 master `fb1fc62c2a795c470a9baeafd1bdd9502efa317d` → 终点 `bc33f71827179242e06d2e74129b0f603bd9f699`（48 个提交）
- 需求：`docs/superpowers/specs/2026-10-20-pricewatch-design.md` + `docs/superpowers/plans/2026-10-20-pricewatch.md`；设计决定 AD-1..32（`docs/01-design/architecture-decisions.md`），其中人类裁决 AD-27（KeepAlive）、AD-29（近期段 100% 覆盖）
- **本报告所有数字于 `bc33f71` 上、最后一次代码改动之后统一重采**（Leader；代码工作区脏文件 0）

## 1. 交付物

新增独立包 `internal/pricewatch` 与 `atlas pricewatch run / once [--dry-run] / backfill [--symbol]` 子命令：按配置监控加密货币价格的 24h/7d/30d/4y/全历史 波动，分档提醒，经 Telegram 发送带 AI 下单建议的消息；附健康告警（取价失败 / 数据缺口 / K 线同步失败）与每日心跳。

| 类别 | 文件 |
|---|---|
| 新包 `internal/pricewatch` | `rule.go` `state.go` `config.go` `store.go` `sync.go` `window.go` `health.go` `message.go` `advisor.go` `engine.go` `runner.go` + 各 `_test.go`、`testdata/valid.yaml` |
| 子命令 | `cmd/atlas/pricewatch.go`、`cmd/atlas/pricewatch_test.go`（init 注册，未改任何既有符号，AD-10） |
| 配置 / 部署 | `configs/pricewatch.yaml`（spec §3 示例）、`deploy/launchd/com.newthinker.atlas.pricewatch.plist` |
| 文档 | spec、plan |

相对 `fb1fc62`：29 个文件，**+7767 / −0**（只新增，不改既有文件）。

## 2. 任务与验证

| 任务 | 内容 | 返工 | 最终验证 |
|---|---|---|---|
| TASK-001 | 规则计算与分档状态机 | 1（QA：ParseRule 放行 NaN/Inf；同档第 3 次提醒无用例） | verified（test-pw-c） |
| TASK-002 | 配置加载与校验 | 1（QA：占位值静默失效、非严格解析、10 行字样断言被路径满足、超时/告警阈值未校验正值） | verified（test-pw-c） |
| TASK-003 | SQLite 存储 | 0 | verified（test-pw-b） |
| TASK-004 | K 线分段补数 | 1（QA：上线日保护无测试、AD-29 未落地、主回填源故障对健康不可见） | verified（test-pw-b） |
| TASK-005 | 提醒/心跳/健康消息 | 3（QA：整条 ≤4096；QA 迭代 2：健康消息不限长；复验：4096 边界无守卫 + 心跳同构问题） | verified（test-pw-c） |
| TASK-006 | AI 下单建议 | 0 | verified（test-pw-b） |
| TASK-007 | 单资产检测与健康 | **2**（首轮：并发用例读写不重叠；QA：健康告警发送前就标记 ⇒ 一次失败永久丢失）——rework_count 显示 1，见 §7-4 | verified（test-pw-b） |
| TASK-008 | 调度与每日心跳 | 1（首轮：替身遮蔽生产路径，run/once 可静默不检测） | verified（test-pw-b） |
| TASK-009 | 子命令与 launchd | 0（验证者失联改派 1 次，见 §7-1） | verified（test-pw-b） |
| TASK-010 | 窗口统计 | 0 | verified（test-pw-a） |
| TASK-011 | 健康追踪器 | 2（首轮：Fail/OK 只并发一半；QA：同 TASK-007 的 R1-1） | verified（test-pw-c） |

**首轮验证的 3 次返工同族**：完成标准点名的行为只在替身或不真实条件下被测（并发只测一半 / 读写不重叠 / 替身遮蔽生产路径）。验证者均以变异 + 消融证明缺口，修复只改测试。

## 3. 覆盖率与测试（`bc33f71` 重采，两把尺）

| 包 | `go tool cover -func` | profile 逐块去重求和 | 门槛 |
|---|---|---|---|
| internal/pricewatch（新） | 94.3% | 783/830 = 94.34% | 80 |
| cmd/atlas | 80.4% | 1410/1764 = 79.93% | coverage_floor 78（AD-11） |

cmd/atlas 两把尺差 10 条语句：全在包级 `var` 函数字面量里且未覆盖（pricewatch.go 注入点 7 条、既有 version.go 3 条），`-func` 不计它们（PENDING #10）。

`go build ./...` ok；`go vet ./internal/pricewatch/ ./cmd/atlas/` 无输出；本 sprint 文件 `gofmt -l` 无输出；两包 `go test -race -count=1` ok；**全仓 `go test -count=1 ./...`：rc=0，68 个包 ok、0 FAIL（上一 sprint 67 个，多出的 1 个即新包 internal/pricewatch）**。

## 4. Code Review（三轮迭代，qa-pw）

| 迭代 | 判定 | 要点 |
|---|---|---|
| 1（`qa-review-round1.md` / `round2.md`） | **REJECT** | 必修：QA-8、AD-29、R1-5（TASK-004）；R1-1 健康告警发送前就标记（TASK-011+007，4 个独立来源含 codex 一致）；QA-14 占位值、R1-2 非严格解析（TASK-002）；QA-6 整条 ≤4096（TASK-005，QA 建议降级、Leader 维持必修 AD-31） |
| 2（`qa-review-iter2.md`） | **REJECT** | 第 1 轮全部闭合；新必修 N1：FormatHealth 不限长 × R1-5 多段错误拼接 × AD-30 不 Ack ⇒ 冷启动 binance 不可达时「监控异常」永不送达（实测 5229/4998/4933 码元）——**两个各自正确的修复叠加才出现的回归** |
| 3（`qa-review-iter3.md`） | **PASS** | N1 闭合（同一系统级探针：5229 → 4096）；生产代码 3 处 SendText 的文本全部封顶，「超长 → 400 → 永久重试必败」类失效结构上消除；4 条 LOW 记入已知限制 |

codex 跨模型：可用但需把源码内联经 stdin 喂入（sandbox 工具超时）；第 1 轮与 Claude lens 结论一致，迭代 3 为 None。

## 5. 与 spec / plan 的差异（全部有据）

- **人类裁决**：AD-27 KeepAlive `true` + ThrottleInterval 60（覆盖 spec §9 的 `SuccessfulExit:false`——后者语义是「非 0 退出才重启」，以 0 退出会静默停）；AD-29 与最短窗口相交的段要求 100% 覆盖、更早段 90%。
- **Leader 决定（依据独立 reviewer 对 okx/binance 的实测）**：K 线回填 binance 优先、取价 okx 优先（AD-20）；增量补数重取最新一根（AD-12）；缺口按槽位覆盖率（AD-13，两源日线对齐不同）；「历史不够长」只在记录了上线日时成立（AD-21）；AI 输出宽松提取（AD-23，claude provider 不处理 JSONMode）；新增「K 线同步失败」健康告警（AD-25）；健康告警送达后才确认（AD-30）；三类消息统一 ≤4096（AD-31/32）；plist 用绝对路径 + WorkingDirectory、仓库内只放占位（AD-27）；`all` 窗口不标注「仅 N 天」（AD-14）。
- **任务拆分**：窗口统计（TASK-010）与健康追踪（TASK-011）从 plan 的 Task 4/7 拆出（AD-26）。
- **超出 plan 的校验**：配置严格解析（未知键报错）、拒绝 `REPLACE_WITH_` 占位、超时/告警阈值须为正。

## 6. 已知限制（不阻断，记录在案）

**行为**
1. 健康状态与 `Sent24h` 只在内存（AD-15）：进程崩溃循环时健康告警可能永不触发（每日心跳兜底）；重启后心跳的 24h 提醒条数少计。
2. SIGTERM 时等在跑的检测返回；provider HTTP 超时 10s 且不收 ctx ⇒ 退出最坏约「数据源数 × 10s」，可能超过 launchd 系统默认等待而被 SIGKILL；若杀在消息发出后、状态写库前，重启后重发一条（plist 未设 `ExitTimeOut`，QA-12）。
3. AD-30 是「至少一次」送达：Telegram 持续不可用时未确认的「监控异常」每轮重发（宁可重复不可丢）。
4. 迟到 Ack 跨失败周期会造成一条无对应异常的「已恢复」——生产不可达（发送与确认同步、调度串行）。
5. `backfill` 遇 Sync 错误（如 binance 不可用）直接退出，不再补其余资产/周期（okx 已取到的照常入库）。
6. 只在 okx 上市的币会得到一条永不恢复的 sync 告警（R1-5 回归，现配置 BTC/ETH 不触发，N2）。
7. 近期窗口内交易所真实缺 K 线时每轮多 1 个重取请求（AD-29 代价）；`fetch` 内另有 90% 阈值与 AD-29 的 100% 不一致，代价同（N3）。
8. 心跳超长（约 30 个资产以上）时截尾，末行「过去 24 小时发出提醒 N 条」先被截掉；非 AI 部分本身超 4096 时提醒消息会截掉部分触发规则。
9. 改了规则阈值但不改 id 时沿用旧状态（要重置就改 id）；notify 只写一部分字段视为配置错误；`backfill`/`once --dry-run` 也要求 Telegram env（AD-28）。
10. 价格舍入跨档外观（999.996 →「1,000.00」，QA-7）。

**测试缺口（LOW，均无生产影响或方向保守）**：WAL 无断言守卫（QA-3）；上线日 5m 键「判 Short」方向无用例（QA-5 测试半边）；回填顺序传给 Syncer、心跳时刻传给 Runner 两处 cmd 接线无测试（QA-13、QA-16）；三处断言要的字样由测试夹具自带（QA-4 已修；QA-17 与 config_test.go:333 未修）；测试 SetContext 未复原（QA-15）；截断预算少 1 / 心跳阈值 4095 两个保守方向变异存活（L1、HB4）；`Ack` 依赖调用点约定而非类型保证（L3）。

## 7. 过程与机制问题（供 PENDING-MECHANISMS / 上游 ArcForge）

1. 🔴 **改派验证者后原实例醒来覆盖了报告**：TASK-009 验证者 test-pw-a 80 分钟零落盘后被改派（逃生边 `verifying → verifying`），12:51 醒来按自己的 checkpoint 完成验证并经 `doc` 覆盖了新验证者的 `TASK-009-verification.md`（任务状态未受影响）。**机制缺口**：逃生边只挡 `transition`（校验 verifier），`doc` 子命令只按角色 `test-*` 放行、不校验本任务 verifier。Leader 差错：改派时以「写通道会 DENY」为由未 TaskStop 原实例。处置：TaskStop、备份其报告、新验证者恢复原报告并附旁证。
2. **validator `stale-dispatch` 盲区**：派验当时写的一次 checkpoint 被算作「本轮产物」⇒ 之后 80 分钟零落盘全程 0 告警；靠 Leader 手查 scratchpad / worktree 文件 mtime + `ps` 才看出。
3. **消息丢失 ×3（PENDING #4 同形）**：dev-pw-b → team-lead 两次（TASK-003、TASK-008 返工，后者等 merge 17 分钟）；Leader → dev-pw-a 两次「TASK-007 现在开工」均未达，dev 缺省「等 011 合入」而 011 被刻意扣住等 007 ⇒ **险成互等死锁**（文件层与「在写代码」同形）。处置：把开工方式写进任务 `fix_items`，观察一周期后 TaskStop dev-pw-a 并经收回边改派 dev-pw-c。教训：「扣住 X 等 Y」类安排必须落在任务文件。
4. **收回边 `review_fix → assigned` 的两个副作用**：拒收 `reason_class`（PENDING #4-15 再撞，实例失联本应标 env_infra）；绕开 `review_fix → in_progress` 的自动 +1 ⇒ TASK-007 rework_count 显示 1、实际 2。
5. **任务 ID 跨 sprint 复用（PENDING #5）**：门禁每个任务都混入往届同号提交（全 11 个任务均报 WARN）；本轮有界集合判定正确（TASK-010 验证者重放门禁查询确认认领到本轮提交）。
6. **write-guard 拦命令文本（PENDING #11 同族）×2**：dev 把 `.arcforge/` 文件复制到 scratchpad 被拦；Leader 在 wisdom 正文里写了「复制命令字面量 + 运行时资产路径」整条被拦。
7. **Jev 未判定任何条目**：TASK-010 门禁日志 shadow 模式 `n_dod=0 skipped=no_judgeable_criteria`。
8. **gitnexus 索引陈旧**：停在 `b3232d6`，早于整个 pricewatch 包 ⇒ impact 全为 UNKNOWN、detect-changes「无已索引符号重叠」**不是干净信号**（各 dev 均以全仓文本搜索补查，本 sprint 未改任何既有符号）。detect-changes 报 32 files 与 git 29 files 差 3，来源未查明。
9. **Leader 自身差错（如实记录）**：①token 明文曾短暂写入 scratchpad 文件（读出后立即删除）；②wisdom 写「探针已持久化到 docs/04-test/」而写通道实为 DENY（未先看 rc，已订正）；③两次把「dev_done 前补写 discovery」的要求放在 merge 通知里，必然晚于 dev 收尾（改为放进派发消息）；④§7-1 的改派未停原实例；⑤DoD「防切片别名」断言措辞有盲点（被观察方须在写入方之后读，dev 自行补测才守住）。
10. 残留两个 `tail -f` 进程监视 test-pw-b 的 T007 结果文件（任务结束未清理，无害）。

## 8. 验收前漂移核对（`verified → accepted` 无机制守卫，PENDING #14，故手工补）

逐任务 `git diff --numstat <verify_baseline.head>..bc33f71 -- <writes>`：**9 个任务为 0**；2 个非 0，均已归因：

| 任务 | 漂移 | 唯一来源 | 审查 |
|---|---|---|---|
| TASK-006 | advisor.go +2/−12 | `53d255a` refactor（Leader 在 QA 前补跑 code-simplifier，AD-6） | QA 第 1 轮逐处对照，语义不变（round1 §112–113） |
| TASK-010 | window.go +2/−5 | `0dbad5f` refactor（同上；私有 listingKey → ListingKey，键格式逐字相同） | 同上 |

仪器自证：阳性对照 `f3d3223..HEAD -- message.go` 报非 0（278/0），与已知答案一致。

## 9. 待人类决定 / 执行

**人类裁决（2026-10-20 终验收）：验收通过，11 个任务转 accepted 并归档。**§6 已知限制按记录接受。

**上线（AD-9，均为人类动作）**
1. 准备 runtime 配置：把 `configs/pricewatch.yaml` 放到 `/Users/zuowei/workspace/runtime/atlas/configs/`（主配置 `llm` 段已配 claude，pricewatch 沿用）。
2. 部署二进制（`bash scripts/ops/deploy.sh`，先 `rsync -n -i` 预演）。
3. 先手动冒烟：`atlas -c <runtime config.yaml> pricewatch --pricewatch-config <runtime pricewatch.yaml> backfill`，再 `once --dry-run`（真实 Telegram env）。
4. 安装 plist：复制到 `~/Library/LaunchAgents/`，**只在副本里**把 `REPLACE_WITH_BOT_TOKEN` / `REPLACE_WITH_CHAT_ID` 换成真值（仓库内只留占位；未替换会被配置校验拒绝启动），可考虑加 `ExitTimeOut 30`；`launchctl load`。
5. 观察：次日 09:00 应收到「💓 监控仍在运行」；未收到即进程停了 / 机器睡眠 / Telegram 不通。
6. 合并 `feature/pricewatch` → master 的 PR 由人类决定。

**待决（不阻断）**：§6 测试缺口 LOW 是否排期；§7 机制问题的上游处置（建议 PENDING-MECHANISMS 新增 §7-1、§7-2、§7-4 三条）。

### 待同步 hooks 清单（人类执行）
| 文件 | 变更摘要 | 同步命令 |
| --- | --- | --- |
| （无） | 本 sprint 未改动任何 `project-template/hooks/` 或 `project-template/scripts/`（本仓库为消费项目，无 `project-template/`）；机制建议见 §7，落点为上游 ArcForge 仓库 | — |

未改动 `templates/CLAUDE.md.template`，无运行时 `CLAUDE.md` 文档漂移需同步。
