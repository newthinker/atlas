# Wisdom Digest — sprint-028 / Prism M3

> 由 Leader 在归档时聚合自各实例的 `learnings-*.md`（只读参考，勿直接编辑）。
> 原始文件保留，本文件只做索引与主题归并。

**来源**：11 个实例，共 2333 行

| 实例 | 行数 | 小节数 |
|---|---|---|
| `learnings-dev-agent-1.md` | 57 | 7 |
| `learnings-dev-agent-17.md` | 25 | 1 |
| `learnings-dev-agent-18.md` | 51 | 1 |
| `learnings-dev-agent-2.md` | 74 | 8 |
| `learnings-dev-agent-21.md` | 39 | 1 |
| `learnings-dev-agent-23.md` | 40 | 1 |
| `learnings-dev-agent-24.md` | 44 | 1 |
| `learnings-dev-agent-3.md` | 63 | 8 |
| `learnings-dev-agent-4.md` | 75 | 9 |
| `learnings-leader.md` | 1787 | 88 |
| `learnings-qa-agent-1.md` | 78 | 6 |

## 全部小节标题（按实例）

### dev-agent-1

- 2026-06-10 code-simplifier subagent 失控
- 2026-06-10 fantasy assertion 教训：HTTP collector 必须由状态码驱动错误
- TASK-001 (2026-06-11): code-simplifier 子代理越权
- TASK-012 (2026-06-11): cmd 装配收口
- sprint-003 (2026-06-12) — TASK-001 + TASK-003
- TASK-001 (sprint-004, 2026-06-12) export-ohlcv 核心
- TASK-002 (sprint-004, 2026-06-12) cobra 接线 + Makefile qlib-data

### dev-agent-17

- 断言的区分力取决于 fixture 的数据分布,不取决于断言本身(dev-agent-17,TASK-007/009 各栽一次)

### dev-agent-18

- dev-agent-18 / TASK-004(prism refresh 明细打印)2026-07-25

### dev-agent-2

- 2026-06-10 收尾：commit 时机 & 共享工作区门禁
- 2026-06-10 TASK-005 worker pool 并行化 + 仲裁超时
- 2026-06-10 TASK-005 W2 返工：执行不应受 cooldown 旁路
- sprint-002 TASK-003 (collector 指数表 + selector 路由)
- sprint-002 TASK-010 (app 类型识别 + 绑定校验 + 动态窗口)
- sprint-002 TASK-011 (app 估值分位编排 buildFundamental 兜底链 — 本 Sprint 语义最重)
- sprint-002 TASK-011 review_fix (QA W1 仲裁补价 / epoch=2)
- sprint-004 TASK-003/004 (qlib 自建数据包：build_data.py + Makefile/README + e2e)

### dev-agent-21

- TASK-019（dev-agent-21）：先问「能不能让它必然发生」，再问「跑多少次才够」

### dev-agent-23

- 「断言恒真」的两种形态:fixture 值与 fixture *缺项* (dev-agent-23, TASK-012 验收 + TASK-013 开工)

### dev-agent-24

- TASK-013 (dev-agent-24, 2026-07-26)

### dev-agent-3

- TASK-004 / TASK-011 (2026-06-10)
- TASK-006 / TASK-008 (2026-06-11)
- TASK-004 (2026-06-12)
- TASK-005 (2026-06-12)
- TASK-006 (2026-06-12)
- TASK-007 (2026-06-12)
- TASK-008 (2026-06-12)
- TASK-007 review_fix (2026-06-12, QA W1/W2/S3/S7, epoch=2)

### dev-agent-4

- TASK-003: 装饰/适配层的 e2e 测试会暴露上游集成缺陷
- cmd/atlas (package main) 的 80% 整包覆盖门禁不可行
- TASK-007: cmd/atlas (package main) 覆盖率门禁的务实解法
- 通用：code-simplifier 子代理会"顺手"推进任务状态
- TASK-003 返工(QA W1): e2e 里硬编码输入会制造 fantasy-pass
- TASK-009: 多包合并覆盖门禁 + code-simplifier 再次顺带补测试
- TASK-005: lixinger 嵌套 metric 不能复用平铺 postJSON
- TASK-004: 「GREEN-on-arrival」任务要诚实标注，别假装 RED
- code-simplifier 子代理第 3/4 次：返回含糊『等你决定/idle by design』

### leader

- 2026-06-12 sprint-005 (percentile_step)
- 2026-06-12 sprint-006 (notifier-wiring)
- 2026-07-23 code-simplifier 子代理卡壳模式(sprint prism-m1)
- 2026-07-23 AD-6 整包口径门禁处置实例(TASK-007)
- Sprint 026 (Prism M2, 2026-07-24)
- Sprint 027 + v1.4.1 发布事故复盘(2026-07-25)
- sprint-028 (2026-07-25)
- sprint-028 wave1 运行期教训（2026-07-25）
- sprint-028 wave1 完结后的补充（2026-07-25）
- sprint-028 wave2:两条测试方法论(test-agent-7 提出,Leader 采纳为通则)
- sprint-028:两条来自 dev-agent-17 的判据(TASK-007)
- sprint-028:「合法值但语义错误」在本 sprint 出现了三次(不同任务、不同层)
- sprint-028:「检查跑了、结论错了」比不检查更危险
- sprint-028:两条把「预读契约」变成可操作方法的补充(test-agent-6)
- sprint-028:中断可能是「静默的」——agent 自己也无法判断发生过什么
- sprint-028:「先决淘汰遮蔽」——多重防护里靠后的那些可能永远走不到
- sprint-028:三条来自 test-agent-7 的方法论(TASK-005 收尾)
- sprint-028:断言区分力检查清单(dev-agent-17 把 test-agent-7 的排查法落成操作形态)
- sprint-028 核心发现:「合法值但语义错误」是本 sprint 最高频的缺陷族(4 次)
- sprint-028:自查有结构性盲区,不是「同一个错误犯两次」(test-agent-6 纠正 Leader 的归因)
- 「断言看起来在测,实际没测到」的两种形态(本 sprint 各遇一次)
- sprint-028:Leader 的文档疏漏已 5 次同类,是方法问题而非偶发
- sprint-028:两个「只在第二遍才显形」的坑(dev-agent-16, TASK-008)
- sprint-028:变异集自身有盲区——「只想到往宽了改,没想到往窄了改」
- sprint-028 补:「虚假的安全感」是本 sprint 贯穿始终的元问题
- 另一条值得记:区分「发现了问题」与「有方法能发现问题」
- sprint-028:文档不同步的**两个方向**——「读到错的」与「读不到」
- 「看似测了实则没测」的第三种形态(test-agent-6 补全)
- sprint-028:「断言恒真」的最隐蔽形态——**断言的那个值,正是 bug 发生时会保留下来的值**
- sprint-028:变异体的两大类——「做少了」与「做偏了」(dev-agent-16 的复盘,比我的归纳更准)
- 「拥有正确的意识 + 写了正确形态的测试 ≠ 覆盖了目标路径」
- sprint-028:两条通用判据(dev-agent-16 在 TASK-008 返工后提出)
- sprint-028:对非确定性行为,「跑 N 次都通过」必须换算成概率才有意义
- Leader 调度错误记录:派了依赖未满足的任务
- sprint-028:「可测缝」是 DoD 能否被验证的前置条件,必须在写测试前定
- 又一例「合法值但语义错误」(第 6 次,本 sprint 最高频缺陷族)
- sprint-028:照配方跑之前,先在**被验 commit** 上复现「修复前」的结果
- 造干扰样本的完整条件(dev-agent-15 的自我批评,补全了此前的规则)
- 一个好习惯:把「为什么必须这样」写进 fixture 注释
- sprint-028 修正:「先决淘汰遮蔽」有**两种不同机制**,对应两种不同修法
- 变异被杀死的更严格判据:不只看「红了」,还要看「为什么红」
- sprint-028:变异打红多个测试时,「红的数量」与证明力无关,**红对地方**才有证明力
- sprint-028:四个测试陷阱是同一族——「证据的形式对了、内容没对上」
- Leader 判断错误:把编排层的约束错误地套到了采集层
- sprint-028 最重要的一条:**独立验证在共享同一错误前提时会失效**
- sprint-028:框架文档与机制边表不一致(已撞两次,需上游修正)
- dev-agent-16 关于「替别人花钱」的自我要求
- sprint-028:Go 小 map 迭代的**精确机制**(test-agent-6 反向坐实,含公式)
- 共同前提最危险的形态:**双方都不知道它是个前提**
- sprint-028:docs-only 门禁对「先提交后转态」是死锁(框架缺陷,需上游修)
- 一条方法论:不要用已知有缺陷的键去验证数据正确性
- sprint-028:「不依赖任何人在正确时机记起正确的约定」
- 弱断言挡不住语义错误:最干净的一个实例(dev-agent-15 预判)
- 「让 golden 全红」的变更需要一个区分判据
- sprint-028:**交叉轴验证** —— 两套独立切分合出同一个数
- 待评估的改进项(test-agent-7 提,记入 M3.5 候选)
- Leader 第三次粗粒度 gating(已自查出并更正)
- sprint-028 **重大更正**:我的「27% 标签冲突」是错的,真实是 12.7%
- sprint-028:两个防御层之间可能存在**可测性依赖**
- 第 8 次「合法值但语义错误」——这次连量级都正常
- sprint-028:「合法值但语义错误」出现在**我们自己的分析里**(第 9 次,元层面)
- 防御应锚定在**不变的行为模式**上,而非当下的具体缺陷
- sprint-028:**错误的执行会借用正确方法论的权威**(test-agent-6 的机制分析)
- 结论冲突时的破局法:**找不依赖任一方前提的第三种验证路径**
- 「验不了就说验不了」
- sprint-028:被中断的子代理会留下**半成品**,恢复前必须验证工作树可编译
- 「加字段容易,给字段配断言容易忘」
- sprint-028:度量方式必须能区分「变异存活」与「根本没跑起来」
- 「重生成基线」这个动作本身会掩盖新引入的缺陷
- sprint-028:**「新版本无冲突」证明不了「缺陷已修」——因为旧版本也无冲突**
- 第三个魔数,第三次无锚点(阈值 15)
- 治本不覆盖降级路径:防御层是唯一防线(test-agent-7 实证)
- 派验通知丢失 23 分钟，而「文件是真相源」没有救回来（2026-07-25）
- 扩大检查作用域，同时要清理作用域内的示例文本（2026-07-25）
- 一个从未生效的参数，在「恰好符合预期」的地方成功了（2026-07-25）
- 三个人在同一处以三种方式出错，穷举一跑就清楚了（2026-07-25）
- 同一天四次「判据对、作用域错」，四次都不是分析出错（2026-07-25）
- 第五例作用域错误：连当事人都不知道自己做了假设（2026-07-25，dev-agent-15 补）
- 变异测试的输出信号只有两位，任何环节出错都伪装成合法结果（2026-07-25）
- 「记路径不记快照」——dev-agent-17 的 checkpoint 写法（2026-07-25）
- 落盘 ≠ 会被读到（2026-07-25，dev-agent-17）
- 指出对自己有利的机制口子（2026-07-25）
- 我给出了一个不可执行的判据（2026-07-25，dev-agent-17 订正）
- 我把自利解读成了利他（2026-07-25）
- 三次 agent 失联，损失从「全丢」递减到「零」——差别只在落盘时机（2026-07-25）
- 我在自己的 DoD 里重犯了刚记录完的错误（2026-07-25）
- 我答了被问的那一半，漏掉的那一半正是任务的实质（2026-07-25，最严重的一次）
- AD-21 的死锁第一次真正发生（2026-07-25）

### qa-agent-1

- 复审方法论（Reality Checker 兑现）
- 本 Sprint 高价值发现模式（可复用）
- 协作机制经验
- 残留（留下一 Sprint，已与 Leader 确认非阻塞）
- sprint-004 自建 qlib 数据包 — 复审（2026-06-12）
- sprint-004 终审 — 对抗审查 spawn 的两个教训（2026-06-12）
