# Sprint 034 交付报告 —— hestia M1b-2（解析层）

**读数时刻**：2026-08-11 02:05Z ｜ **交付 commit**：`defdc5e68ac8a0443f00e5b823f4579eabe30948`
**分支**：`feat/hestia-parse`（基线 `6c51f788f1600aa4366a215b099ee6f929cb7ba5`，= 本地 master，领先 origin/master 21 个 commit）

## 结论

**8/8 accepted**，validator **0 条告警 / EXIT=0**。QA 两轮评审：round1 判 **REJECT**（1 CRITICAL + 3 WARNING），三条返工后 round2 判 **PASS**。

```
380 PASS / 0 FAIL / coverage 89.5% / go vet exit=0 / go build ./... exit=0 / gofmt 0
```

| 任务 | 标题 | rework | verifier |
|---|---|---|---|
| TASK-001 | 样本装载与 golden 基线 | 0 | test-agent-22 |
| TASK-002 | HTML strip 与文本规整 | 0 | test-agent-23 |
| TASK-003 | 板块切分与 extractor 探测 | **1** | test-agent-22 |
| TASK-004 | amount / ratio 数值原语 | 0 | test-agent-23 |
| TASK-005 | 模板表与字段抽取 | **1** | test-agent-22 |
| TASK-006 | Parse 组装、期次与口径推导 | **1** | test-agent-22 |
| TASK-007 | golden 端到端与收尾 | 0 | test-agent-22 |
| TASK-008 | Parse 值正确性的常驻守护 | 0 | test-agent-22 |

TASK-008 是本 Sprint 中途新建的（verifier 在为 T7 准备判据时发现「`Parse` 层的 Values 值正确性没有任何常驻断言」）。**用新建任务而非 `review_fix`**，因为那是 DoD 的覆盖缺口、责任在写 DoD 的 Leader，走返工环会把 `rework_count` 记到错的人头上。

## QA round1 的三条与闭合证据

| 级别 | 发现 | 修复 | 闭合证据（消融，非「全绿」） |
|---|---|---|---|
| **CRITICAL** | `detectExtractor` 在「恰好丢掉社融两节」处静默判成 `rule@v1`——`(6, 无社融)` 与合法 v1 逐位相同 | T3：`checkSectionOrdinals`，校验序号从「一、」起连续，**置于版式判定之前** | 恒返回 nil ⇒ **375 PASS / 5 FAIL**；QA 全格穷举 **255+63 个丢失组合**，修复前静默成功 2 个、修复后 **0** |
| WARNING | `mustMatch` 无唯一性校验，约 30 条清单模板走最左优先且该选择零覆盖 | T5：改 `FindAllStringSubmatch`，`len != 1` 报错 + 常驻断言 | 退回最左优先 ⇒ **378 PASS / 2 FAIL**；危害探针 `deposit_nbfi_ytd=+500`（应 **−7446，符号翻转**）而 `err=nil` |
| WARNING | `Parse` 放行空/形态非法 PubDate，错误延到 `Store.Save` 才现场 | T6：`!ok \|\| pubDate == ""` + `publishedAtRE` 形态校验 | 四种输入全部报错 + 0 Values；**阴性对照**（原样样本）仍产出 54 项 |

**QA 穷举找到一个此前无人发现的第二碰撞点**：`丢{一,八}` 同样落到 `(6,false)`——第二节标题是「社会融资规模**增量**」，不含判据词「**存量**」。⇒「单个反例」的顾虑有实据，而**两个碰撞点被同一行代码一并关掉，包括那个没人知道的**。这是修法（校验报告自带的序号冗余，而非逐个堵成因）正确性的最强证据。

## 交接项（详见 `internal/hestia/CONTRACTS.md` 的 Sprint 034 节）

1. **`1e-6` 容差既缺机制也缺依据**——改成 `1e-2` 无任何测试转红；且这个数从来没有人算过（太松漏真错、严格相等被 ULP 打红，两个方向都不成立）。修法两步：严格相等 + 显式豁免清单；先枚举哪些换算 bit-exact。
2. **「社融两节位于开头」无守护**——返工后两条守卫的覆盖并集封闭，**依赖这个当前为真、无断言钉住的前提**。低成本机制化：断言社融节序号为 1 与 2。
3. **`fields.go` 单位缺口**（`fx_reserve` 万亿美元 / `fx_rate` 元/美元不属任何一类）——**QA 判「现在就该改」**：现在是纯注释 0 行代码，等 M1b-3 照错误前提写完闸门表再改就是改一张已上线的表，**而那张表错了不会响亮失败**。
4. **`loan_corp_short_ytd` 的 1-ULP 表示误差**——M1b-3 若用 `==` 或恒等式做闸门（如分项加总 == 总额），会在毫无问题时报红。
5. **completeness 闸门恒真，M1b-3 开工前必须二选一**（派生必填集 / 部分成功）。`types.go` 的 `absent_field:` 与 pending 机制读起来就是为后者准备的，**而 `extract.go` 是全有或全无——这条张力从未被声明**。
6. **`parse_test.go` 两处注释描述错误**（`functional[1]` 的理由、`require.NotEmpty` 结构上不可达）。后者是**一颗看起来一直在守着的哑弹**：DoD 明说 `Len` 可被取代，将来有人删掉 `Len` 时那行才第一次生效，而没人知道它此前从未生效。

### 待同步 hooks 清单（人类执行）

**本 Sprint 未改动 `project-template/hooks/` 或 `project-template/scripts/` 的任何文件**（改动全部在 `internal/hestia/`），因此**无文件需要同步**。

但本 Sprint 实测暴露了下列框架级机制问题，**建议在 ArcForge 仓走 `project-template/` → TDD → 人类确认的路径**：

| # | 机制问题 | 实测依据 | 建议 |
|---|---|---|---|
| 1 | **`discovery` 指针靠自觉，命中率 1/3** | 5 个任务里 **3 个由非 owner 补写**，前两次未报告 ⇒ 缺陷静默发生三次才浮现；3 个 dev 里 2 个无此习惯 | 并进 `dev_done` 门禁——**那是唯一「产物已备好 ∧ 责任人仍有写权 ∧ 已有校验时机」三者同时成立的时刻**。现设计放在 `verified`，那时三个条件全部不成立 |
| 2 | **`TeammateIdle` 对子代理反复误报** | 本 Sprint **5 次**：两个 dev 的 code-simplifier 子代理各被拦 2–4 次，QA 的两个只读 lens 各被拦 2–3 次 | 触发条件已收窄为「**子代理继承父实例身份 ∧ 父实例状态在保活集内**」。它错的不是判断（任务确实没走完），**是收件人** |
| 3 | **verifier 缺一个正当等待态** | dev 有 `blocked_clarification`（不在保活集）可正当停等；verifier「已核验完、等 Leader 定夺」在文件系统上仍是 `verifying`，被当成「该干活没干」而保活 | ⚠️ **补这个不对称之前先读这条**：本 Sprint 一次错误 reject 正是被这个「缺口」挡下的（verifier 在等待中未停机，用那段时间算出了推翻前提的结果）。**补上等待态需同时解决「verifier 等待期间谁保证决策到达」** |
| 4 | **门禁的 `COMMITTED_MINE` 对本 Sprint 恒空** | `task-completed.sh` 按 `^[a-z]+\(TASK-00N\):` 匹配提交主题，而本 Sprint 主题是 `fix(hestia):` ⇒ 该路径一直是空的，门禁实际只靠工作树推断 | 这解释了为什么「范围外漂移」告警**总匹配到 2026-06 的老提交**——只有老提交才用 `feat(TASK-001):` 写法。**不是「任务号跨 Sprint 复用污染」**（此前的登记），根本原因是新提交根本不进那条匹配 |
| 5 | **worktree 隔离 × `dev_done` 门禁的结构冲突** | 本 Sprint **5 次**（3 个 dev + 2 个 simplifier 子代理各自独立说出）。门禁在主仓库跑，测的是一棵没有交付物的树 ⇒ **它会「成功」**——`verify_baseline` 随后钉在不含交付物的 sha 上 | 唯一可靠时序：**Leader 先 `git merge`，dev 再 `transition dev_done`**。本 Sprint 全程照此执行，无一次假绿 |
| 6 | **CLAUDE.md 提到的 `transition-audit` 不存在** | `validator-run.sh` 只支持 `validate\|progress`，QA 前置检查跑它报「未知工具」 | 文档与实现漂移，二选一修 |
| 7 | **scratchpad 撞名** | 587 个文件里**只有 39 个带实例名**；Leader 与 verifier 各自违反 2/3 与 8/8 | **改用 `$SCRATCHPAD/<实例名>/` 子目录**——前缀需要每次写文件都做一次正确决定（N 次机会出错），子目录只需一次 |
| 8 | **`fix_items` 写绝对数会过期** | 我写「必须仍 358 PASS」，而三条返工并行、每条落地都改变它（交付态 372）。这违反了 CLAUDE.md 已有的「跨时间点保存的绝对指纹不构成基线」 | 判据改成「不低于 `<commit>` 的基线」或「不引入新的 FAIL」 |
| 9 | **返工的 discovery 应追加而非覆盖** | 同一 Sprint 三个任务两种做法（T5/T6 追加、T3 覆盖），而**无任何 `fix_items` 要求过** | 机制级理由：`verify_baseline` 把 `discovery_sha256` 记为判定对象的一部分，**原地覆盖会让首轮的 `verify_baseline` 不可重建**——而那份首轮交付正是 QA 判出 CRITICAL 时面对的东西 |
| 10 | **code-simplifier 归属不可事后核实** | 该 sub-agent 本 Sprint **5/6 次自述与实际不符**；且三轮返工中每轮相关文件只有一个提交 ⇒ **无人能把 dev 写的行与它改的行分开** | **跑 simplifier 前先 commit（哪怕 WIP）**，让「哪些行不是我写的」从自述变成可查事实 |

复制后对新增/改动脚本补可执行位，再运行 `bash project-template/scripts/check-runtime-sync.sh` 核对运行时副本与模板一致（agent 只呈现清单，不执行这些命令）。

## 方法论沉淀（跨 Sprint 可复用）

- **一致性 ≠ 必要性**：verifier 一度用「三种别的成因也被拦下」证明「修法覆盖根因」，复查发现**那三种在修复前也会被拦下**（产生 7 板块、版式分支本就报错）。⇒ 反例必须**落到要堵的那个碰撞点上**才算数。
- **穷举子集 > 穷举成因**：任何失锚成因最终归结为「哪些标题失去锚定」，成因只决定哪个子集。**全格枚举 318 个组合，不依赖任何人的想象力**。
- **证据「刚好够」是危险区**：不够会让人继续查，太够会让人多余确信，**只有刚好够会让人停在那里且自我感觉良好**。
- **纠正结论不会纠正模型**：被推翻后同一小时内，我用同一个被推翻的模型设计了新判据。**改产物不改产线，下一件产品还是原样**。
- **预先声明「什么结果算证伪」**比事后要求诚实有效——它保护的是**提出判据的人**（本 Sprint 那次拦下的正是 Leader 自己的判据缺陷）。
- **正确性附着在载体上**：记错 sha 的 baseline、跑错目录的自查、改前的树上跑的变异、同一事实多副本只改对一处——**作者是对的，而载体是错的**，这类错误结构上不可能被自查抓到。
