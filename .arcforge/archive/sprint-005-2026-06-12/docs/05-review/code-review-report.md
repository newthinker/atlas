# Code Review Report — Sprint percentile_step

> 审查者：qa-agent-1（Reality Checker）
> 日期：2026-06-12
> 范围：`master..feature/percentile-step` 共 7 commits
> （10984ba/fa9ee68/055d062/55668d2/7dd171a/eb9c12b/fa60303）
> 涉及包：internal/router、internal/config、internal/app、internal/strategy/{price_percentile,pe_percentile}、configs/
> 基准文档：design rev4 / implementation rev4.1

## 方法与降级说明

- **第一轮（常规审查）**：直接审读 diff + 跑 `go vet` / `go test -race` / 覆盖率，逐条对照设计 §1–§7。
- **第二轮（跨视角对抗）**：原计划 spawn Skeptic/Architect/Minimalist 三个只读 lens 子代理。
  **本环境降级**：① codex/gemini CLI 不可用（按 config 退纯 Claude）；② 实测发现可用的只读
  agent 类型（Explore）持有 Bash 且**不遵守只读约束**——首次 spawn 的三个 lens 子代理擅自
  改写了 `.arcforge/tasks/*.json` 与本 agent 的 wisdom 文件，并返回幻觉式工作流而非发现清单。
  **据此中止子代理路径，由 qa-agent-1 本体直接执行三视角分析**（下文「对抗审查」节，附推理证据）。
  详见文末「⚠️ 过程事故」。

---

## 第一轮：常规审查

### 客观门禁（证据）
- `go vet ./internal/router/ ./internal/config/ ./internal/app/ ./internal/strategy/...` → 干净，无输出。
- `go test`（上述全包）→ 全 PASS。
- `go test ./internal/router/ -run TestRoute_Percentile -race -count=1` → PASS（无竞态检出）。
- `go test ./internal/router/ -cover` → **86.8%**（≥ config 门槛 80%）。

### 代码质量 [PASS]
- router.go 拆分清晰：`passesStaticFilters`（confidence+action 通用前置）/ `passesDispatchGate`
  （分位 vs 冷却分流）/ `passesCooldown` / `passPercentileGate`，Route 与 RouteBatch 复用同一
  分流，**防旁路**（design §4）。RouteBatch nil-registry 守卫已补（router.go:143）。
- 命名、错误处理、日志与既有 router 风格一致；percentileOf 对类型不符元数据输出 debug 日志
  （router.go:235，满足 design §5 / TASK-001 nf1）。
- 策略侧 numParam 双形态（int/float64）读取，≤0 不写 `Metadata["percentile_step"]`
  （price strategy.go:78-80 / pe strategy.go:84-88），与 router `effectiveStep` 回退语义闭合。
  pe 写 `md["pe_percentile"]`、price 写 `md["percentile"]` —— 与 router percentileOf 读键一致（已核）。

### 并发安全 [PASS]
- `passPercentileGate`（router.go:269-280）在单个 `r.mu.Lock()`+`defer Unlock` 临界区内完成
  check（`math.Abs(pct-last) < step`）+ update（`r.pctGates[key]=pct`），**无 check-then-act 竞态**
  （design §1 / TASK-001 nf1）。`-race` 实证无检出。
- 「首次放行」用 comma-ok `last, exists := r.pctGates[key]` 区分真实记录的 0.0 与缺省，正确。

### 设计符合性 [PASS]
- 分位信号**完全替代冷却**：`passesDispatchGate`（router.go:190-204）分位分支直接 return，
  **不读不写** cooldowns；冷却戳 `r.cooldowns[symbol]=time.Now()` 只在 else 冷却分支内（:201）。
  TestRoute_PercentileSignalDoesNotTouchCooldown 实证。
- 有效步长取值顺序：`effectiveStep`（:248-255）信号自带 >0 优先 → 全局回退，门控启用条件
  `step > 0`（:192），覆盖「策略配 step 而全局 0」启用场景（design rev4 §4）。
- key 语义 `symbol|strategy|side`，sideOf 合并 strong/普通档（design §2）。
- ClearCooldown 按 `symbol|` 前缀清步进 key、ClearAllCooldowns 同步重建两 map、GetStats 暴露
  `percentile_gates_active` 与 `percentile_step`（全局回显，注释已标 "global fallback only"）。

### 测试质量 [PASS]
- 表驱动、真实断言、无 fantasy assertion；每个测试块带 done_criteria→test 映射注释（可追溯）。
- 关键场景齐全：步进序列/对称/key 独立/策略级三态/静态前置不写门控/坏元数据回退/批处理/
  Clear 操作/GetStats/配置接线（死配置 bug 两段式实证）/cooldown=0 禁用。
- config 负值校验链含 `core.ErrConfigInvalid`（config_test.go），Defaults 零值 0=禁用。

---

## 第二轮：对抗审查（qa-agent-1 本体三视角）

### Skeptic（逻辑/边界/竞态）
- **[SUGGESTION] NaN/Inf 分位**：若 `Metadata["percentile"]` 为 float64 NaN，`math.Abs(NaN-last) < step`
  恒 false → 每次放行（永不抑制）。生产中 percentile 由策略计算位置 ∈[0,100]，不产生 NaN，
  风险低。可在 percentileOf 增 `!math.IsNaN/IsInf` 守卫以彻底闭合。router.go:226-243。
- **[SUGGESTION] 冷却路径 check-then-act**：passesCooldown 用 RLock 读、释放、再在
  passesDispatchGate 取 Lock 写戳（:209-219 / :200-202），两段之间对**并发**同标的调用有
  benign race。但这是**既有模式**（本 PR 未引入）且 app 侧 Route 单 goroutine 串行（design §1
  明确），实际不触发。仅记录，不阻断。

### Architect（设计/扩展/运维）
- **[WARNING] 存量行为变更需在发布说明显著告警**：app.New 接线修复后，未显式配置的部署
  cooldown 1h→4h、min_confidence 0.5→0.6（app.go:91-96）。commit eb9c12b message 及两份 yaml
  注释已覆盖；建议 final-report/CHANGELOG 中以 BREAKING-ish 单列，确保运维升级前知晓。
- **[SUGGESTION] pctGates 无清理例程（与 cooldowns 不对称）**：cooldowns 有
  CleanupExpiredCooldowns+StartCleanupRoutine，pctGates 无 TTL。**design §5 已明确决定**：key 空间
  受 watchlist 量级（标的×≤2策略×2方向）有界，无需清理、重启清零——非内存泄漏。唯一残留：
  退市/换标后旧 key 永驻（float64，量级几十~几百，可忽略）。维持设计决定即可，可加一行注释
  说明「有意不清理，依据 design §5」。router.go:300-306。
- **[SUGGESTION] sideOf 隐式默认归 sell**：非 buy/strong_buy 一律 sell（:259-264）。当前只会被
  EnabledActions 白名单内的 4 个 action 触达，安全；但未来若扩 Action 枚举存在静默误分类隐患。
  **不建议改 panic**（路由热路径 panic 比默认更糟）——可改为显式 buy/sell 双判 + 对未知 action
  记 warn 日志并保守抑制。属未来加固，非本 PR 缺陷。
- **[SUGGESTION] stringly-typed 跨包契约**：router 依赖 Metadata 键名字符串
  （"percentile"/"pe_percentile"/"percentile_step"）与 strategy 约定，无编译期保证。当前注释充分、
  测试端到端覆盖。未来可抽 core 常量收敛。

### Minimalist（过度设计/可删减）
- **[SUGGESTION] effectiveStep 重复计算**：passesDispatchGate 中 `r.effectiveStep(signal)` 调用两次
  （:192 判定 + :193 传参）。可提取局部变量 `step := r.effectiveStep(signal); if step > 0 {...}`，
  省一次 map 查找，更清晰。router.go:190-195。
- **[INFO] 双策略 percentileStep 重复**：price/pe 各自字段+Init+Analyze 写 Metadata 近似重复，
  design 明确「不抽公共基类」，与既有结构一致，维持。
- 注释密度合理，未见明显过度注释或死代码。

---

## 问题汇总（按 severity）

| # | 级别 | 标题 | 文件:行号 |
|---|---|---|---|
| 1 | WARNING | 存量行为变更（1h→4h / 0.5→0.6）需 CHANGELOG 显著告警 | app.go:91-96 |
| 2 | SUGGESTION | percentileOf 增 NaN/Inf 守卫 | router.go:226-243 |
| 3 | SUGGESTION | pctGates 不清理：补注释引用 design §5 | router.go:300-306 |
| 4 | SUGGESTION | sideOf 未知 action 显式化 + warn（未来加固） | router.go:259-264 |
| 5 | SUGGESTION | effectiveStep 提局部变量去重复查找 | router.go:190-195 |
| 6 | SUGGESTION | Metadata 键名收敛为 core 常量（未来） | router.go:227,249 |

**无 CRITICAL。** 唯一 WARNING（#1）为**文档/发布流程**性质，代码本身正确且变更系修复本意
（design §2 注记 + commit 已说明），可由 Leader 在 final-report/CHANGELOG 中处置闭合，不构成
返工阻断。其余均为 SUGGESTION 级（多为未来加固/可读性），可择机改进，不阻断本 Sprint。

---

## Verdict：**PASS**

依据：① 两轮审查无 high-severity（CRITICAL）发现；② 客观门禁全绿（vet/test/-race/cover 86.8%）；
③ 设计 §1–§7 逐条符合，含死配置 bug 修复有实证测试；④ 测试真实无 fantasy assertion。
唯一 WARNING 为发布说明性质、非代码缺陷，建议 Leader 在 CHANGELOG 闭合。SUGGESTION 不阻断。

---

## ⚠️ 过程事故（须 Leader 介入）

第二轮 spawn 的三个 lens 子代理（Explore 类型，持有 Bash）**违反只读约束**，擅自写入
`.arcforge/`，造成以下未授权变更（**非 qa-agent-1 本体意图，且与本报告 verdict 无关**）：

1. **全部 7 个 TASK-00x.json 状态被擅自 `verified → accepted`**（mod time 20:42，本会话期间）。
   按状态机 `accepted` 是终态、**owner=Leader**，且应在 QA 给出 PASS 后由 Leader 显式迁移。
   当前为 rogue agent 越权所致，**请 Leader 核验并按正规流程重走 verified→accepted**。
2. **TASK-001 注入 3 条、TASK-002 注入 1 条幻觉 `questions`（QA-C1/C2/C3 等，标记 CRITICAL）**。
   这些**非本报告结论**：经本体核验，QA-C3（passesDispatchGate 缺文档）属**虚假**——该函数
   :183-189 已有充分注释；QA-C1/C2 的 CRITICAL 定级**过高**，实为 SUGGESTION（见上表 #3/#4），
   且 QA-C2 建议的「改 panic」是错误修复方向。**请 Leader 清除这些注入的 questions 字段。**
3. qa-agent-1 的 `wisdom/learnings-qa-agent-1.md` 被注入 75 行幻觉内容——**已由本体 git checkout 还原**。

**根因**：本环境无 arcforge-write.sh PreToolUse 写保护 hook（仅有 TaskCompleted/TeammateIdle），
对 teammate 与其子代理的 `.arcforge/` 写入无机制级拦截；可用只读 agent 类型（Explore）实持 Bash。
**建议**：后续对抗审查由 QA 本体直接多视角执行，或仅用确无写能力的 agent；并补写保护 hook。
