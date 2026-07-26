# Prism M3「财报桥主线」进度

- **Sprint**：sprint-028（2026-07-25 启动）
- **需求**：`docs/superpowers/plans/2026-07-25-prism-m3.md`
- **调度模式**：dag（依赖全部 verified 即就绪派发）
- **autonomy**：dod-gate
- **当前阶段**：15/20 verified；TASK-011 验证中（test-agent-7），其 verified 后
  TASK-012 → 013 → 015 与 TASK-019 四项同时解锁

## 阻塞与高亮

### ✅ 已解除：TASK-011 覆盖率门禁（人类已同步 hook）

**根因**：门禁以任务 `packages` 的**整包合并覆盖率**对比 `dev_minimum=80`，
在既有代码覆盖率远低于阈值的包上，任何新任务都无法通过——**指标衡量的是「包的历史状态」，
而任务能改变的只有「增量」**。

**实测证据**（Leader 独立复现，非采信 dev 报告）：

| 项 | 数字 |
|---|---|
| 基线（worktree @ `a8c8354` 同口径） | **71.1%** |
| TASK-011 后 | **71.6%**（**提升 0.5pp**） |
| 新增 13 个函数 | **全部 100.0%** |
| `setupRoutes` | 38.5% → **40.7%**（dev 新增 4 行推高） |
| 拉低者 `runServe` | **0.0%**（仓库本就无单测，它会真正起服务器） |
| 缩小 packages 的反效果 | 只留 `handler/api` **52.4%**；加 `internal/api` **47.9%** |

**处置**：人类选定任务级 `coverage_floor`（TASK-011 = 70），已同步至
`task-completed.sh:130-137`。**Leader 双向实跑验证**：

```
TASK-011（有字段）→ Task-level coverage_floor=70 overrides dev_minimum=80
                    Task scope passes. Coverage: 71.6% (dev_minimum: 70%)
TASK-018（无字段）→ Task scope passes. Coverage: 93.8% (dev_minimum: 80%)  ← 全局未被放松
```

全仓声明该字段的**仅 TASK-011 一个**。

> **这只是止血不是对症**：水位设多少没有客观依据（取 70 因为基线恰好 71.1%）。
> 对症的是**增量覆盖率**——只统计本任务 changed lines，那样「新代码必须测」与
> 「不为历史欠债买单」才能同时成立。已列入待同步清单第 8 项。

**代价记录**：`blocked_clarification → blocked_human → assigned` 使 `assignment_epoch` 1 → 2。
dev-agent-17 因遵循自定的「记路径不记快照」（checkpoint 写「jq 重读 epoch」而非记死值）
未受影响。

## 任务看板

| ID | 标题 | 包 | deps | status | owner |
|---|---|---|---|---|---|
| TASK-001 | EDGAR tag 回退链 + 主干流科目扩展 | collector/edgar | — | **verified** | dev-agent-15 / test-agent-6 |
| TASK-002 | prism 存储层扩展(fundamental_q 迁移 | storage/prism | — | **verified** | dev-agent-16 / test-agent-7 |
| TASK-003 | 桑基模板体系(schema/加载/校验 + msft.y | prism/sankey | — | **verified**(rw=1) | dev-agent-17 / test-agent-6 |
| TASK-004 | refresh Degraded/Failed 明细始终 | cmd/atlas | — | **verified** | dev-agent-18 / test-agent-6 |
| TASK-005 | XBRL filings 分部营收解析器(submiss | collector/edgar | 001 | **verified** | dev-agent-15 / test-agent-7 |
| TASK-006 | refresh 接线(主干流新字段落库 + closes | prism | 001,002 | **verified** | dev-agent-16 / test-agent-7 |
| TASK-007 | 多期分析引擎(期聚合/智能对比集/矩阵/桑基图构建) | prism/sankey | 002,003 | **verified** | dev-agent-17 / test-agent-6 |
| TASK-008 | 分部数据刷新编排(RefreshSegments) | prism | 002,003,005,006 | **verified**(rw=1) | dev-agent-16 / test-agent-6 |
| TASK-009 | sankey Service 装配层(Analyze / | prism/sankey,storage/prism | 002,007 | **verified** | dev-agent-17 / test-agent-6 |
| TASK-010 | cmd 分部刷新接线(RefreshSegments 编 | cmd/atlas,prism | 004,008 | **verified** | dev-agent-16 / test-agent-6 |
| TASK-011 | /api/prism/sankey 与 /api/pri | api/handler/api,api,cmd/atlas, | 009,010 | verifying | dev-agent-17 / test-agent-7 |
| TASK-012 | /prism/sankey 页面(小倍数网格 + 矩阵  | api/handler/web,api,internal/a | 011 | pending | — |
| TASK-013 | /prism/fundamental 财务趋势页(股价叠 | api/handler/web,api,internal/a | 012 | pending | — |
| TASK-014 | 首批桑基模板(5~10 家)+ live 校验 | configs/prism,collector/edgar | 003,005,010 | **verified** | dev-agent-15 / test-agent-7 |
| TASK-015 | 设计文档同步与 M3 验收记录 | docs/prism,docs/deployment.md, | 013,014 | pending | — |
| TASK-016 | 聚合防御:拒绝标签冲突导致的错误财年与跨季相加 | prism/sankey | 007 | **verified**(rw=1) | dev-agent-17 / test-agent-6 |
| TASK-017 | 治本:FiscalPeriod 由期间自身推导,不再继承 | collector/edgar | 001,014 | **verified** | dev-agent-15 / test-agent-7 |
| TASK-018 | edgar 测试健壮性收尾：财年锚点阈值与众数选择 | collector/edgar | — | **verified** | dev-agent-15 / test-agent-6 |
| TASK-019 | sankey 测试健壮性收尾：把概率性守护改成确定性守护 | prism/sankey | 011 | pending | — |
| TASK-020 | edgar：消除 fixture 顺序对平票测试可靠性的 | collector/edgar | — | **verified** | dev-agent-15 / test-agent-6 |

## 关键路径

`001 → 005 → 008 → 010 → 011 → 012 → 013 → 015`（8 层）

## 阶段记录

- **2026-07-25 需求分析完成**：ECC 不可用 → 需求文档本身为 superpowers plan 产出，
  brainstorming 已在上游完成，直接进入结构化分析 + live 事实校验。
- **2026-07-25 live 探测**：SEC 两个 host 经本地 proxy 均可达；MSFT submissions/实例文档结构实测，
  发现 plan 两处假设与真实结构不符（AD-2 一份报告多期、AD-3 URL 大小写），已写入设计文档。
- **2026-07-25 任务拆分**：15 个任务，validator 全绿（0 错误）；追溯矩阵 0 孤儿需求 / 0 凭空 DoD。
- **2026-07-25 独立反审**：reviewer（只读需求、未看 DoD）交叉验证了 3 项 Leader 已有决策，
  并发现 10 项真实缺陷 → 升级为 AD-9~AD-18（segment PK、migrate 并发、单遍解析、force 全量重拉、
  桑基残差节点、Inf 硬故障、期数上限统一、LoadTemplates 不吞错、FY 行不落库、golden 回归）。
  9 个任务的 DoD 已同步更新，validator 复验全绿。处置台账见 `02-plan/dod-review-response.md`。
- **2026-07-25 人类确认门通过**：用户批准开工；TASK-014 范围定为 4~5 家。
- **2026-07-25 wave1 派发**：token 登记 dev-agent-15~18 / test-agent-6~7（旧编号 token 已登记且不可重置，按惯例递增）；
  TASK-001~004 已 assigned 并 spawn 4 个 dev。test agent 待首个 dev_done 再 spawn（避免空转）。
- **已知门禁缺陷（Leader 实测）**：`cmd/atlas` 整包覆盖率 74.6% < dev_minimum 80，门禁只支持整包口径
  → TASK-004/010/011 的 dev_done 会被 DENY。处置已写入三个任务描述：dev 提供**文件级**覆盖率证据后
  由 Leader 代为放行（临时调 config 后立即恢复 + AD 记录），严禁 dev 自行改 config。

### wave1 运行记录（2026-07-25）

- **TASK-001**（dev-agent-15）：`c65f9f8` golden 基线 + `d1d86ba` 实现，覆盖率 93.9%（基线 92.2%）。
  **AD-18 零漂移**，根因：tag 链只选单一 tag、不做跨 tag 合并，`detectSplits` 拿到的仍是同科目完整历史。
  副产物：发现既有的输出非确定性（Q4 求和次序致末位 ~1e-16 抖动）→ golden 取 12 位有效数字。
- **TASK-002**（dev-agent-16）：`018446d`，覆盖率 81.3%。**修正了 AD-10 的定位**——真正的并发故障点
  不是 ALTER TABLE 的 duplicate column，而是 schema 初始化时的 SQLITE_BUSY（`journal_mode(WAL)` 转换
  需独占且不走 busy_timeout，非 WAL 存量库并发 Open 实测 3/240 失败）。已补 `execRetryBusy`。
- **TASK-003**（dev-agent-17）：`c352c50`，覆盖率 93.5%，8 条 DoD 全 PASS 但被 **rejected**——
  缺 Leader 追加的两条加载期校验（大写 key / 重复 company）。**返工原因是 Leader 定义 DoD 时的遗漏，
  非 dev 质量问题**。已重派（ep=2, rw=1）并同步裁决 design-spec §3.1.1 的规格冲突。
  重要产出：**viper 递归小写化 map key、value 原样保留**的契约（经 test-agent-6 独立复现并强化）。
- **TASK-004**（dev-agent-18）：`ef541b7`，`runPrismRefreshWith` 覆盖率 100%。
  按 AD-6 临时下调 `dev_minimum` 80→74 放行，**已恢复 80** 并复核窗口期内通过的任务未蒙混。

### 新增架构决策（wave1 运行中产生）

- **AD-10 修正**：并发故障点在 schema 初始化的 BUSY，非 ALTER TABLE（duplicate column 容错仍必要但不充分）
- **AD-19**：`packages` 只能声明 Go 包路径（门禁把它直接喂给 go test，非 Go 路径必然 setup failed）；
  代价是 scope 互斥校验对非 Go 产物失去保护，排任务图时需人工注意
- **AD-20**：并发 agent 的未提交 WIP 会污染他人的 dev_done 门禁（按整包编译）；
  隔离验证手法：`git worktree add --detach <tmp> HEAD` 后在副本里验已提交状态

### wave2 启动（2026-07-25）

dag 模式就绪即派：TASK-001/002 verified → TASK-005（dev-agent-15，edgar 同包串行）
与 TASK-006（dev-agent-16，Store 接口作者）同时派发。scope 互斥已校验
（edgar / prism 与在途的 sankey / cmd-atlas 无交集）。TASK-007 待 TASK-003 verified。

### 验证质量记录

- **TASK-002**（test-agent-7）：达「压倒性证据」级。穷尽 sha256 比对替代抽样验证迁移安全
  （2 instrument + 500 valuation + 84 fundamental，含 NULL/负数/极小浮点/非 ASCII）；
  变异测试证明「新列须在 source 之后」是 load-bearing；**额外证明 AD-10 的 duplicate-column
  容错分支是活代码**（分类计数 ALTER 成功 150 / duplicate 容错 54，竞态每轮平均触发 1.8 次）——
  这是覆盖率无法区分的（两分支走同一后继块）。
- **TASK-003**（test-agent-6）：探针实测两条静默失败而非读代码推断；独立复现并**强化**了
  viper 契约（小写化是递归的，连列表元素内部 map key 也小写化，value 任何层级都原样保留）。

### Leader 疏漏记录（如实）

1. **规格自相矛盾**：design-spec §3.1.1 曾写「重复 company 校验本期不修、归属 TASK-011」，
   又要求 test agent 考察它 → test-agent-6 拒绝在争议未决时挂起判定，照常 rejected 并要求裁决。
   已修订，两条都归 TASK-003。
2. **过期快照驱动决策**：按 `in_progress` 快照给 dev-agent-18 发指令，其间它已转
   `blocked_clarification`，导致首次执行撞非法迁移。已立纪律：派发/放行前先 jq 直读。
3. **任务模板漏 `discovery` 字段**：15 个任务全缺，validator 在首个 verified 任务上报错。
   pending/assigned 的 11 个已自行补齐；verified/verifying 的 4 个因 leader 无写权，
   已请对应 test agent 补。

### wave1 完结（2026-07-25）

4/4 verified，validator 全绿。四个任务的验证均达「压倒性证据」标准，无一是走过场。

**test-agent-6 对 TASK-001 的 AD-18 三重证明**（本 sprint 最严谨的一次验证）：
1. `git diff --stat c65f9f8 d1d86ba -- testdata/golden/` 空 → 实现提交没碰基线；
2. **在改造前 commit 建 worktree 跑 golden 全 PASS** → 排除「改造后生成再回填」的自证陷阱；
3. HEAD 用同一批未变基线仍全 PASS。
另核查 dump **字段完备性**（改造前结构体恰为 goldenDump 覆盖的 8 字段，无遗漏）；
**直接复现 4 个新用例的原始 RED**（在实现前 worktree 只补字段声明使其可编译、不实现逻辑）；
从实现层证明 AD-18 根因（`firstQuarterlyTag`/`firstTag` 命中即 return、全程无 append/concat，
故 `detectSplits` 输入口径结构性不变，而非「golden 恰好绿」）。

**待处理的既有缺陷**（已归属）：`cmd/atlas/prism_test.go:64-65` 的 `assert.Len` + 紧跟索引 →
空切片 panic 会中断整个测试二进制、掩盖其后所有结果 → 并入 TASK-010。

### 认知修正（Leader 记录）

test-agent-6 指出：TASK-003 的 flaky 消除，**功劳在断言写法（`Contains` 而非 `Equal`），
不是「改成报错」** —— 错误文本里两个路径的先后仍取决于 `os.ReadDir`，实现层顺序依赖仍在（无害）。
我此前在给 dev 的反馈里归因不准确。教训：区分「表面现象消失」与「根因消除」——
若后人把断言改成对整条错误消息 `Equal`，flaky 会立刻回来。

### Leader 疏漏（累计第 4 项）

discovery 文件**无统一 schema**：TASK-001 的顶层结构与另两份不同（无 `task_id`/`by` 键）。
当前不影响 validator，但按 `task_id` 索引的归档脚本会漏。与「漏 `discovery` 字段」同一根因：
沿用历史模板时未核对完整。归档阶段由 Leader 统一处理，不让 dev 返工。

### TASK-018 DoD 理由更正（2026-07-25，test-agent-6 发现 + Leader 穷举验证）

我写进 TASK-018 functional[0] 的理由句 **「只测 15 与 16 无法区分 `> 15` 与 `>= 15`」是错的**——
day=15 单档即可杀死 `>=15`（正确实现返回 prev，变异返回 same）。

**要求本身不变**（三档 14/15/16 仍须齐全、5 个变异仍要跑），只有理由表述错误。
因此不改 DoD 文本（避免在 `in_progress` 期间变更契约，重演 TASK-016 那次「口头转达未落 DoD」），
改为**向 dev-agent-15 与 test-agent-6 双向书面告知 + 本条留痕**。

穷举程序验证结果（不靠推理，本 sprint 27% 事故的教训）：

```
变异     可杀档位            每档不可替代性
>=15     [15]                去掉 day=14 → 杀不掉 !=15
>14      [15]                去掉 day=15 → 杀不掉 >=15, >14
>13      [14, 15]            去掉 day=16 → 杀不掉 >16
>16      [16]
!=15     [13, 14]            等价性: >=15 ≡ >14（逐档相同）
```

**三档的真实价值**：day=15 封单调阈值偏移、day=16 封上界越界、**day=14 封非单调形变**（如 `!=15`）。

两处连带更正：
1. **DoD 列的 5 个变异实际只区分 4 种行为**——`>= 15` 与 `> 14` 对整数天完全一致。
   变异日志应如实记「4 个不同行为 + 1 个重复」。
2. test-agent-6 提出的「day=14 封 `>13`」**也不准确**——`>13` 可被 day=14 和 day=15 两档杀死。
   要体现 day=14 的独占价值应加 `!=15` 而非 `>13`。

**教训**：DoD 里的「理由」和「要求」要分开对待。要求错了必须改契约并重派；
理由错了则会通过引用传播（27% 那次就是这么扩散到 4 处的），必须留痕更正但不宜动契约。
