# 需求分析 — Collector 策略闸门与路由表

**输入**：`docs/superpowers/plans/2026-08-06-collector-policy-gate.md`（4243 行实施方案）
**配套设计**：`docs/superpowers/specs/2026-08-06-collector-policy-gate-design.md`
**分析日期**：2026-08-06

## 0. 需求分析阶段的降级说明（如实记录）

`arcforge.config.json` 的 `capabilities.ecc = false` → **不调用 ECC `/multi-plan`**。

常规降级路径是改用 superpowers `brainstorming` 精炼设计，但本次**进一步降级为「方案解析 + 约束提取」**，理由：

- 输入文档本身就是 superpowers `writing-plans` 的**定稿产物**，且附有独立设计文档（含 §8 验收标准）与自查覆盖矩阵（设计条款 → 任务映射，声明「未覆盖条款：无」）。
- 方案已给出**逐任务的完整测试代码与实现代码**，设计空间已经收敛。此时再跑 `brainstorming` 只会推翻已定稿设计，是净损失而非增益。

因此本阶段 Leader 的实际职责是：**校验方案自洽性 → 提取硬约束 → 按 Realistic Scope 再拆分 → 转化为可机器校验的 done_criteria**，而非重新设计。

## 1. 目标

用新包 `internal/collector/policy` 的 Gate 中介层，统一承接各 collector 的限流／缓存／合并／配额；并把散落的符号路由谓词收敛为一张具体度优先的路由表。

**执行链**：TTL 缓存 → singleflight 合并 → 配额预判 → 限流域节流 → 执行 fn。

**架构要点**：Gate **不包装 collector**（不用装饰器），由各 collector 在**发 HTTP 请求处**调用 `policy.Fetch[T]` / `Gate.Do`。这避免了 `CachedCollector` 装饰器遮蔽扩展接口的问题，也让缓存下沉到 HTTP 调用处，顺带修复 lixinger 从未进入缓存路径的既有缺陷（设计 §1.3）。

## 2. 硬约束（违反即验收失败）

| # | 约束 | 来源 |
|---|---|---|
| C1 | 零新增第三方依赖（仅标准库 + 已有的 `golang.org/x/sync v0.16.0`） | 方案 Global Constraints |
| C2 | `internal/prism/refresh.go` **零改动**；`policy` 包的错误不得外泄到 prism 层 | 设计 §5、验收标准 1 |
| C3 | `internal/collector/policy` **不得 import** `internal/collector`（避免循环） | 设计 §3.1 |
| C4 | 公开路由 API 签名不变：`SelectForSymbol` / `SelectExternalForSymbol` / `MarketForSymbol` / `KnownIndexMarket`；6 个外部调用点零改动 | 设计 §6.1 |
| C5 | 限流数值全部**平移不调整**：yahoo 500ms、tushare 200ms、twelvedata 8s | 设计 §4.2 |
| C6 | 未登记主题 = 零策略；`eastmoney`/`akshare`/`crypto`/`fred`/`edgar`/`baostock` 行为零变更 | 设计 §4.1 |
| C7 | 配额账本异常一律 **fail-open**（放行 + 告警），绝不阻断降级链 | 设计 §4.4 |
| C8 | 每个任务结束 `go build ./... && go vet ./...` 通过 | 方案 Global Constraints |

## 3. 验收标准（设计 §8，逐条可执行）

1. `internal/prism/refresh.go` 零改动，既有测试全通过 — `git diff --stat master -- internal/prism/refresh.go` 无输出
2. 路由黄金值测试对**新旧实现均全绿** — `TestRouteGoldenValues`
3. 两个 Gate 实例共享文件时配额跨「进程」生效 — `TestFileStoreQuotaSurvivesProcessRestart`
4. tushare 配额耗尽返回的错误满足 `errors.Is(err, ErrRateLimited)` — tushare + prism 两侧各一条
5. 各 collector 源码中不再存在 `lastReq` 字段 — `TestNoPrivateThrottleState`（AST 断言）
6. lixinger 能进入缓存路径（§1.3 缺陷修复） — `TestLixingerRequestIsCached`

## 4. 方案自洽性校验（Leader 复核结论）

- **依赖链自洽**：方案声明「1→5 严格顺序；6/7/8/9 独立可并行；10 依赖 6-9；11 完全独立；12 最后」，与各任务 `Consumes` 段落一致，无矛盾。
- **签名演进已显式标注**：Task 2 的 `New(t *Table, opts ...Option)` 在 Task 3 变为 `New(t, q QuotaStore, opts ...Option)`，且 Task 3 明确列出要同步改 `gate_test.go` 的 helper。这是**有意的两步演进**，不是设计缺陷。
- **已知偏离均有理由**：`Gate.Wait` 第三入口、`yahoo.quote` 新主题、`<域>.*` 通配、`Take` 传 `Quota` 结构、缓存返回值由调用方 clone、`quota_mem.go` 并入 `quota.go`、`BTC.HK` 行为统一 —— 7 项偏离在方案「设计文档之外的实现决定」一节逐条说明理由，Leader 复核认可，全部转入对应任务的 done_criteria。
- **覆盖矩阵闭合**：方案自查记录声明「未覆盖的设计条款：无」，Leader 抽查 §3.5 / §4.4 / §5.1 / §7.4 均有对应任务，认可。

## 5. Leader 补充的机制性风险与对策（方案未覆盖，属 Arcforge 层面）

| 风险 | 事实依据 | 对策 |
|---|---|---|
| **R1 覆盖率门禁必卡 cmd/atlas** | 实测基线 `cmd/atlas` = **74.3%** < `dev_minimum` 80 | 触碰 `cmd/atlas` 的任务（T5b、T10）预设 `coverage_floor: 74` |
| **R2 scope 漂移误判连锁阻塞** | Sprint-030 TASK-001 实录：`task-completed.sh` 扣除「他人在途包」的白名单**不含 `verified`**，已验证但未提交的改动会被算作后来者的 scope 漂移 | **每个 Dev 必须先 `git commit` 再 `transition dev_done`**（Go 任务门禁跑 `go test $PKGS`，不依赖工作区脏否，故提交在前无损门禁效力）。已写入每个任务的 description |
| **R3 policy 包四任务同 scope** | T1-T4 同为 `./internal/collector/policy`，validator 要求在途任务 scope 互斥 | 该四任务本就严格串行，wave 递增，任意时刻仅一个在途，天然满足互斥 |
| **R4 T5 跨 3 package 超 Realistic Scope** | 方案 Task 5 含 9 个文件、跨 policy/config/cmd 三包 | 拆为 T5a（policy 侧 `Default`/`Override`）与 T5b（config + cmd 接线），见 02-plan |
| **R5 T12 跨 2 package** | AST 测试在 collector、集成回归在 prism | 拆为 T12a / T12b，天然分包且互斥 |

**其余覆盖率基线**（实测，`-count=1`）：`internal/collector` 98.2%、`yahoo` 86.2%、`lixinger` 92.1%、`tushare` 94.2%、`twelvedata` 92.7%、`config` 95.9%、`prism` 94.0% —— 均 ≥ 80，无需 floor。

## 6. 复杂度评估

**整体：复杂**（新包 + 5 个 collector 改造 + 装配层删除 + 路由重写，14 个拆分后任务）。
单任务复杂度见 `02-plan/`：T2/T5b/T8/T11 为 high，其余 medium/low。
