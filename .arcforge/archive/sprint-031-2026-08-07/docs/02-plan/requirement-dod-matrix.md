# 需求 ↔ DoD 双向追溯矩阵

**任务图**：方案 12 个任务 → 14 个 Arcforge 任务（拆分理由见 AD-2）
**validator**：`✓ 任务图校验通过（14 个任务）`

## 1. 方案任务 → Arcforge 任务

| 方案 Task | Arcforge | 标题 | packages | deps | wave |
|---|---|---|---|---|---|
| 1 | TASK-001 | policy 包 —— 策略表与查表 | policy | — | 1 |
| 2 | TASK-002 | Gate —— 节流/合并/TTL/超时 | policy | 001 | 2 |
| 3 | TASK-003 | QuotaStore 接口 + 内存实现 + Gate 接线 | policy | 002 | 3 |
| 4 | TASK-004 | QuotaStore 文件实现（跨进程） | policy | 003 | 4 |
| 5（前半） | TASK-005 | Default/SetDefault + Table.Override | policy | 004 | 5 |
| 5（后半） | TASK-006 | config 扩字段 + cmd/atlas 装配接线 | config, cmd/atlas | 005 | 6 |
| 6 | TASK-007 | yahoo 接入 Gate | yahoo | 006 | 7 |
| 7 | TASK-008 | twelvedata 接入 Gate | twelvedata | 006 | 7 |
| 8 | TASK-009 | tushare 接入 Gate + 配额 + 错误映射 | tushare | 006 | 7 |
| 9 | TASK-010 | lixinger 接入 Gate（仅 TTL） | lixinger | 006 | 7 |
| 10 | TASK-011 | 删除 CachedCollector 与 maybeCache | collector, cmd/atlas | 007-010 | 8 |
| 11 | TASK-012 | 路由表重写（黄金值先行） | collector | — | 1 |
| 12（前半） | TASK-013 | 防回潮 AST 测试 | collector | 011, 012 | 9 |
| 12（后半） | TASK-014 | prism 配额降级链集成回归 | prism | 009 | 8 |

## 2. 验收标准（设计 §8）→ DoD（机器检查结果）

| # | 验收标准 | 承接任务 | 状态 |
|---|---|---|---|
| 1 | `refresh.go` 零改动 | TASK-014 | ✓ |
| 2 | 路由黄金值新旧实现均全绿 | TASK-012 | ✓ |
| 3 | 配额跨「进程」生效 | TASK-004 | ✓ |
| 4 | 配额错误满足 `errors.Is(err, ErrRateLimited)` | TASK-009, TASK-014 | ✓ 双侧 |
| 5 | 各 collector 无 `lastReq` | TASK-007/008/009 + TASK-013（AST 统一守护） | ✓ |
| 6 | lixinger 进入缓存路径 | TASK-010 + TASK-011（扩展接口正向断言） | ✓ |

**孤儿需求（无 DoD 覆盖）：无。**

## 3. 硬约束 C1–C8 → DoD

| # | 约束 | 承接任务 |
|---|---|---|
| C1 | 零新增第三方依赖 | TASK-011（`git diff master -- go.mod go.sum` 无输出）|
| C2 | prism 零改动 + policy 错误不外泄 | TASK-009, TASK-014 |
| C3 | policy 不 import collector | TASK-001（`go list -deps`）|
| C4 | 路由公开 API 签名不变 | TASK-012 |
| C5 | 限流数值只平移不调整 | TASK-001, TASK-008, TASK-009 |
| C6 | 未登记主题 = 零策略 | TASK-001, TASK-002, TASK-005 |
| C7 | 配额账本 fail-open | TASK-003, TASK-004, TASK-006 |
| C8 | `go build` + `go vet` | TASK-006, TASK-011 |

**凭空 DoD（不对应任何需求）：无**（独立 reviewer 逐条核实四个疑似项——`maxCacheEntries`、`nil Gate` 透明、大小写不敏感、collector 未注册回退——全部有方案或现状依据）。

## 4. 独立 reviewer 反审结论与处置

reviewer 只读需求与设计文档独立推导验收标准清单，再与 DoD 比对。**结论：DoD 足以判定「包内单元行为做对了」，但不足以判定「装上去真的生效了」——遗漏集中在跨 package 的接缝处**（14 份 DoD 按 package 切分，接缝无人认领）。

**全部 12 条遗漏项已采纳并落入 DoD**（Leader 逐条核实源码后确认成立，非盲信）：

| # | 遗漏项 | Leader 核实 | 落入 |
|---|---|---|---|
| **A1** | `atlas prism refresh` 入口无任何 DoD 覆盖 | **成立且最严重**：`cmd/atlas/prism.go:171` 调 `loadConfigOrDefaults()`（`export_ohlcv.go:283-292`，当前只装载配置）后于 `:191-194` 构造 collector。若 `initPolicyGate` 加在调用点而非 helper 内部，**prism 拿到无账本的懒构造 Gate，配额彻底失效而全部测试仍绿**——设计 §1.5 的立论正是「短命进程」，prism refresh 是配额唯一真正生效的进程 | TASK-006（并把该条 `review` 升级为可执行 `test`）|
| **A2** | 执行链顺序未钉死：缓存命中不得消耗配额/等待节流 | 成立。quota 前置会让缓存命中吃掉 daily_basic 的 5 次/天；throttle 前置会让 twelvedata 缓存命中白等 8s | TASK-002, TASK-003 |
| **A3** | coalesce 合并的 N 个请求只消耗 1 次配额 | 成立。quota 须在 singleflight 内侧，否则并发下一轮吃掉 N 次配额 | TASK-003 |
| **A4** | `go.mod/go.sum` 零变更无任何 DoD 承接 | 成立。C1 是硬约束，而 flock 与 glob 两处高诱惑点加依赖不会有门禁报警 | TASK-011 |
| **A5** | 缓存 key 必须含 topic | 成立。`Fetch` 是泛型的，`yahoo.chart`/`yahoo.eps` 同 symbol 天然同 key | TASK-002 |
| **A6** | coalesce 后 fn 失败的错误传播 | 成立（设计 §5.2 明列）。返回零值+nil error 会让调用方拿到「空数据但无错误」，降级链不触发 | TASK-002 |
| **A7** | tushare 缓存返回值所有权 | **成立**：`row.values` 是 `map[string]float64`，`slices.Clone([]row)` 浅拷贝挡不住；且方案的 clone 只出现在 yahoo/twelvedata | TASK-009 |
| **A8** | 扩展接口正向断言缺失 | 成立。TASK-011 删掉了唯一覆盖「包装遮蔽 `FundamentalCollector`」的测试却无正向补充——只删证据不算修复 | TASK-011 |
| **A9** | `IsAShareIndex` 前置规则未锁 | **成立**：`selector.go:73,109` 确有该前置。漏掉会让 `930713.CSI` 落到 `*` 兜底→yahoo/US，A 股指数行情全错且不报错 | TASK-012 |
| **A10** | 账本损坏后能否自愈 | 成立。只 fail-open 不重建 = 配额永久静默失效 | TASK-004 |
| **A11** | 无全仓 `-race` | 成立。`internal/app` 的 errgroup 是 Gate 的真正并发消费方，逐包 `-race` 从未覆盖它 | TASK-011 |
| **A12** | 旧 config.yaml 兼容性 | 成立。新字段指针语义 + 装载路径改动，生产在用的旧配置无用例 | TASK-006 |

**B 类「不可测试项」修正**（7 处）：TASK-006 的装配顺序由 `review` 升级为可执行断言；TASK-004 的「进程被杀不留半截 JSON」改为可判定的「无残留临时文件」+ `review`；TASK-013 的一次性变异实验内化为 `testdata/` 假源码用例（守护测试自身被守护）；TASK-007/008/009/010 的「行为不变」改为可判定的「master 基线既有用例一条不删不改且全绿」；TASK-012 的 BTC.HK 期望值直接写死进 DoD（`SelectExternalForSymbol→yahoo`、`MarketForSymbol→MarketHK`，方案行 3930-3938）；TASK-007 的 grep 手段与 `verify_by:test` 标注不符，改由 TASK-013 统一守护。

## 5. Realistic Scope 符合性

| 维度 | 要求 | 实况 |
|---|---|---|
| packages | ≤ 1 | 12/14 满足；TASK-006、TASK-011 各跨 2 包（AD-2 说明理由：拆开会产生无消费者的空洞任务）|
| DoD 条数 | ≤ 8 | 7/14 为 5–8 条；7 个任务为 9 条 —— 超出的 1 条均为采纳反审遗漏项所致，**正确性优先于形式约束** |
| 改动文件 | ≤ 5 | 全部满足（最多 5 个）|
