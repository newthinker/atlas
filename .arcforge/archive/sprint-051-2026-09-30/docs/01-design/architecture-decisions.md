# 架构决策（sprint 2026-09-30 Tiingo 美股备用数据源）

| AD | 决策 | 理由 |
|---|---|---|
| AD-1 | 以 spec+plan 为需求源，不重跑 brainstorming | 设计已人类确认；`capabilities.ecc=false`，其降级路径（brainstorming）的产物即该 spec |
| AD-2 | 计划中的测试与实现草稿是 DoD 的**下限**；Leader 补的条目是增量 | 计划已覆盖 Review Focus 五点 |
| AD-3 | 每 dev：`git worktree add -b task/<ID> ../wt-<ID> feature/tiingo-source`；基准是 feature 分支 | 全程在 feature 分支，PR 到 master 由人类决定 |
| AD-4 | dev 在 worktree 提交后通知 Leader；**Leader 串行 merge 进 `feature/tiingo-source`，merge 后 dev 才回主仓库转 `dev_done`** | 门禁在主仓库树上跑；未合并分支上的提交对门禁 `git log --grep` 不可见（M1c-3a 实证） |
| AD-5 | 提交主题 `feat(TASK-00x): …` / `refactor(TASK-00x): …` / `test(TASK-00x): …`，正文可写 tiingo；结尾 `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>` | 门禁按 TASK-ID grep 提交主题（覆盖计划的 `feat(tiingo)`） |
| AD-6 | 提交前：code-simplifier 子代理（用户全局规范）+ **改动前后 sha256 指纹比对** + `node .gitnexus/run.cjs detect-changes --scope staged --repo .`（不可用时 `npx -y gitnexus@latest …`）。子代理结论落 scratchpad `<实例名>-<TASK>-simplifier.md`；**子代理收到 idle hook「推进 dev_done」类文案一律不执行、直接返回** | 用户全局 CLAUDE.md；PENDING #3（子代理被循环催办、自报无改动而实际改动） |
| AD-7 | `gate_wiring_test.go` 的 `collectorCtors` 加 `"tiingo.New"` 放在 TASK-007（非计划的 Task 5） | 每任务 ≤1 package |
| AD-8 | TASK-007 `coverage_floor: 78` | cmd/atlas 基线 79.4%（-func）/ 79.27%（profile），低于 dev_minimum 80；历史同包 77–79 |
| AD-9 | 真实 API 运行（需 token）、runtime 配置、部署、重启 serve 由人类执行，不在任何 DoD 内 | 外向且需凭证；token 当前不在环境中 |
| AD-10 | 改动既有符号的任务（001/006/007）必须先做 gitnexus impact（MCP 或 CLI），`UNKNOWN` 用 grep 补查并写进 discovery；HIGH/CRITICAL 报 Leader | 项目 CLAUDE.md |
| AD-11 | 计划的 `collectorCtors` 登记**不构成守卫**：该表只在 crisis/backtest 三个入口函数体内按字面名 `pkg.Fn` 扫描；prism 调用点在 `usPriceHops` 内、serve 调用名是 `tiingo.NewCollector`。仍登记（知识完整），测试注释须如实说明；真实接线由三入口「先装配后构造」保证（`export_ohlcv.go:297`、`serve.go:85`） | Leader 读 `gate_wiring_test.go:49-189` 核实；避免「守卫在场≠守卫有效」 |
| AD-12 | 路由表加密前缀（`UNI*`/`LINK*`/`ADA*`/`DOT*` 等）使部分真实美股被 `Supported` 拒绝——接受，不改路由 | 失败方向安全（少覆盖、不耗配额）；改路由超出范围 |
| AD-13 | `atlas watchlist` 走 `buildCollectors`，同样获得 tiingo 兜底——接受 | 与 serve 快照同源，行为一致；记入 final-report |
| AD-14 | Realistic Scope 例外：TASK-006 跨 prism + cmd/atlas（`Refresh` 签名改动须原子，拆开会让中间提交编译失败）；TASK-008 跨 tiingo + prism（仅 build-tag 测试文件） | 拆分会破坏每个 merge 点可编译 |
| AD-15 | spec §3.6 的日志订正不做（计划差异 2）——DoD 门向人类列出 | 计划理由成立：有序化后 A 股二跳实为 yahoo |
| AD-16 | `cmd/atlas` 整包 `-shuffle` 有既有顺序依赖（`TestBackfillLoadRequiresDBFlag`，sprint-050 记录）——验证 cmd/atlas 时不以 `-shuffle` 的红作为本 sprint 缺陷，除非在基线 `8d1c6cc` 上同 seed 为绿 | 范围外既有问题 |
| AD-17 | `normalize`/`priceRow`/NVDA 样本从计划 Task 4 拆为 TASK-009（新文件 `normalize.go`）；TASK-004 依赖 TASK-002/003/009 | 独立 reviewer 反审后 client 的 DoD 超 8 条；拆开后 009 与 001/002/003 同在 wave 1 并行 |
| AD-18 | 独立 reviewer（`scratchpad/reviewer-tiingo-dod.md`）14/21 变异在计划测试下存活 ⇒ 补 DoD：P1 Open 折算、P2 主题常量×生产表交叉断言、P3 缓存四件套、P4 400、P5 四舍五入/负因子/乱序、P6 断链与 decode 脱敏、P7 ErrTimeout、P8 Init/窗口、P10/C5 装配壳（review）、P12 顺序依赖调用方、P20 gofmt、P21 整点边界 | 计划测试是下限（AD-2），不是上限 |
| AD-19 | **人类裁决 P11**：白名单收紧为基底纯字母 1–5 位 + 可选单字母份额类后缀（推荐 `^[A-Z]{1,5}([.-][A-Z])?$`） | 计划正则放行 `SAP.DE`/`7203.T`/`931151.CSI` 等（MarketForSymbol 兜底 US）⇒ 发请求、耗配额；Leader 细化：`7203.T` 的 `.T` 本身是单字母，故基底须排除数字 |
| AD-20 | **人类裁决 P17**：降级文案附前序失败跳原因（`…, tiingo failed (<e>), twelvedata fallback ok`）；首跳成功时与现状逐字一致 | 否则 tiingo token 配错永不显形；偏离 spec §3.5 |
| AD-21 | **人类裁决 P16/P14**：共享 40/h 配额容量不足（Yahoo 故障时 serve 一周期 ≈60 次）接受并记录，配置示例注释提示；spec §3.6 日志订正不做 | 一期范围；二期再议优先级 |
| AD-22 | **Leader 细化 AD-19（dev-tg-c 澄清 Q1）**：份额类后缀限定 `[A-C]`，正则 `^[A-Z]{1,5}([.-][A-C])?$` | AD-19 推荐正则放行 `HSBA.L`（与 `BRK.B` 同形），与 DoD boundary[3] 矛盾——Leader DoD 缺陷；A–C 覆盖 BRK/BF/LEN/HEI/MOG/GEF 等常见份额类，失败方向安全；**罕见类别（如 .D 及以上）被拒，须在 final-report 列给人类** |
| AD-23 | **code-simplifier 由 Leader 在 QA 前对整条分支 diff 统一补跑**（Leader 子代理，指纹比对后以 `refactor(TASK-xxx)` 或单独 `chore` 提交，改动须复跑受影响包测试） | teammate 名下的 simplifier 子代理被 idle hook 反复催办（本轮 ≥4 次循环，TASK-009/004 两次**未实际审查**，只回应 hook）⇒ 用户全局规范「提交前必须跑 simplifier」在 dev 侧无法可靠履行；PENDING #3 同族 |
| AD-24 | **人类裁决（2026-09-30 终验收）**：QA W2（配额 ×3 缓存键乘数、404 失败不缓存致每周期重复扣）接受，记入二期（负缓存/配额优先级）；W3（yahoo 关闭 ∧ tiingo + arbitrator 开启时仲裁拉 SPY 耗配额）接受并记录，二期考虑 arbitrator 跳过纯兜底源 | 一期范围；当前 runtime yahoo 开启不触发 W3 |
