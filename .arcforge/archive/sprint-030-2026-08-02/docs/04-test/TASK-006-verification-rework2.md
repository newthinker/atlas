# TASK-006 复验报告(rework 2)— QA D5-D8

验证者: test-agent-15 | 日期: 2026-08-02 | epoch=1 | rework=2 | 判定: **VERIFIED (PASS)**

前序:`TASK-006-verification.md`(首轮 5 条 DoD)、`TASK-006-verification-rework1.md`(D1-D4)、
`TASK-006-D5-addendum.md`(上轮 D5 未落地的核查)。本轮只验 D5-D8;D1-D4 上轮已验且
本轮 dev 未再动(`git diff 356d07c` 仅 3 份文档、20 增 10 删,均属 D5-D8 范围)。

## 一、D5 港股跳如实标注(MAJOR)—— PASS

上轮我的核查结论是「四要素中三项缺失」。本轮逐项复查:

| 要素 | 上轮 | 本轮实测 | 判定 |
|---|---|---|---|
| ① spec:35 港股行加不可用注记 | 备注栏**完全空白** | 已按 A 股估值行体例补齐:「⚠ 该二跳已接线但**从未取到过数据,当前不构成可用兜底**(2026-08-02)」+ 两个未解障碍 + 归一未做的理由 + 修复指向 §9 第 4 面 | PASS |
| ② 形态未归一升级为**待修缺陷** | 仅记现象 | design §9:439 开篇即「**待修缺陷:symbol 形态未归一**——配置为 4 位 `0700.HK`,tushare hk_daily 需 5 位 `00700.HK`,**客户端不做归一**」 | PASS |
| ③ 明确「不构成可用兜底」 | 全 docs/ grep **命中 0** | **design §9:439 与 spec:35 两处命中**(Leader 要求的两处) | PASS |
| ④ 后续任务三面→四面 + ADR#8 | 三面,无港股 | **1 个任务、4 面**(顶层条目计数=1,面计数=4),第 4 面为「港股 symbol 归一 + 取数实证」 | PASS |

**Leader 点名要确认的「顺序不能颠倒」——在**。§9:447 原文:

> **顺序不能颠倒**:先靠本任务第 1 面解除限频阻塞拿到正向证据,**再**一次做对归一
> ——否则是拿假设换假绿。届时**同步修订 ADR#8**。

这句确实堵住了「直接补个 `%05s` 就宣告完成」的路径:它把前置条件(先拿正向实证)
写进任务描述本身而非评审记录,后续执行者绕不开。

**两处「归一未做的理由」表述一致且成立**:design 与 spec 都写「5 位形态能否真取到数据
本身未实证(探针均撞 1 次/小时限频),先做归一等于拿假设换假绿」。我复核了这个推理链——
它与 `TestRefreshHKProductionSymbolHitsKnownGap` 的 fake 设计(按真实上游契约只认 5 位,
不迁就被测代码)相互印证,逻辑自洽。

## 二、D6 §9 第 1 面措辞(MINOR)—— PASS

**Leader 要求我独立核实「只做文案分叉、无退避逻辑」,我没有采信 dev 陈述**:

| 核查项 | 实测 |
|---|---|
| `ErrRateLimited` 全部非测试消费点 | 仅 `internal/prism/refresh.go:450`(在 `tushareFallback` 内) |
| 该 `case` 分支内容 | 只有一条 `degs = append(...)` 文案,随后与 `ErrNoPermission` 分支落到**同一个** `return degs, err` |
| `refresh.go` 全文 `time.Sleep` | **0** |
| `refresh.go` 全文 `retry`/`Retry` | **0** |
| `refresh.go` 全文 `backoff`/`退避` | **0** |

⇒「**已被消费于 Degraded 文案分叉,但尚未据此退避重试**」**准确**。
Degraded 文案「限频,本次跳过,下次自动重试」中的「下次」指下一轮 refresh,
不是轮内退避,与「尚未退避」不矛盾。

**§10 前向兼容措辞按 Leader 要求保留且仍准确**:§10:482 仍写「分类**被消费**
(限频触发退避而非放弃)属后续任务「限频感知退避」」——退避确实未实现,该句成立。
另注:§10 中原有的 `refresh.go:450` 行号引用本轮也一并改成了
「`internal/prism/refresh.go` 的 `tushareFallback`」,与 D7 一致。

## 三、D7 行号引用改函数名(MINOR)—— PASS

**按 Leader 认可的范围限定判定**(限 dev 的 writes 内 4 份文档,全仓治理另立任务):

| 文件 | `\.go:[0-9]+` 残留 |
|---|---|
| `docs/deployment.md` | **0** |
| `docs/prism/atlas_prism_design.md` | **0** |
| `docs/superpowers/plans/2026-08-02-prism-m3.5a-datasources.md` | **0** |
| `docs/superpowers/specs/2026-08-02-prism-m3.5-design.md` | **0** |

**Leader 点名要复核的那处腐烂,我独立验证了,且危害确实超出「过期」的程度**:
计划文档原写「沿 `refresh.go:80-90` 的 edgar→engine 同构降级结构复制改写」。
实读 `internal/prism/refresh.go:80-92`:

```go
// Report summarizes one refresh run. Partial failures do not abort the run.
type Report struct { ... }

// degrade records observable degradations; empty strings mean "none".
func (r *Report) degrade(msgs ...string) { ... }
```

⇒ 该行号现在指向的是 **`Report` 结构体定义与 `degrade` 方法**,与它声称的
「edgar→engine 降级结构」**毫无语义关系**。这不是「引用过期」,而是**静默指向完全无关的
代码**——照此指引施工会直接走错。dev 已改为 `refreshEdgar`→`refreshEngine` 函数名引用。

**新引用的函数名全部经核实真实存在**(避免用一个错引用换另一个错引用):

| 函数 | 位置 |
|---|---|
| `refreshEngine` | `internal/prism/refresh.go:265` |
| `tushareFallback` | `:426`(第 450 行的 ErrNoPermission/ErrRateLimited 消费确在其函数体内) |
| `refreshTushareValuation` | `:470` |
| `refreshEdgar` | `:635` |

## 四、D8 区间记法(MINOR)—— PASS

§9:433 现写:

> NVDA **半开区间 [2026-07-16, 07-31) 共 11 个交易日**(该日期**闭**区间为 12 根,
> 末日未纳入本次比对)与库内 `price_daily` 逐日比对,最大相对偏差 ~2e-8

**两个数字共存,且我逐一查库复核**:

| 记法 | 库内 NVDA 根数 | 与文档 |
|---|---|---|
| 闭区间 `[07-16, 07-31]` | **12** | 一致 |
| 半开 `[07-16, 07-31)` | **11** | 一致 |

⇒ 改的是**记法**而非把 11 篡改成 12。原「11」被保留为**当次比对的真实覆盖根数**
(TASK-002 首轮确实只比了 11 根),这是正确的处理——把它改成 12 会把一次真实的实验记录
改写成未发生的事。dev 在 `decisions` 中的同款理由成立。

## 五、两项需 Leader 处置(均非 dev 过失,**不作为 reject 依据**)

我认真权衡过是否因这两项判 NEEDS WORK,结论是不应当,理由逐条列明。

### ① 两条用例名未在 docs/ 点名

Leader 要求确认「后续任务里点名了 `TestRefreshHKProductionSymbolHitsKnownGap` 与
`TestRefreshHKPriceOnlyHopWiring` 两条用例必须同步改写」。实测:**两个名字在 docs/ 命中均为 0**。
design §9:439 只有一句泛指——「TASK-005 侧有测试红线锁定生产形态走该跳会零行判失败」,
未点名、也未说明「修好后需同步改写」。

**但该保护实际存在,且在更有效的位置**。`internal/prism/refresh_test.go:1445-1447`:

> ⚠ 后续任务(D4)做完归一后,本用例必须同步改写为「归一后能命中 5 位并成功」,
> 否则它会反过来把修复判成失败。改写前置条件:先拿到 5 位形态的正向实证
> ——截至 2026-08-02 两次探针均撞 hk_daily 限频,尚无证据。

这正是 Leader 担心的那个陷阱的解药,而且它**就写在会变红的那条用例正上方**——
实施者见红必读此处。写进 design §9 是加保险,不是唯一防线。

**且该要求不在 `fix_items`**(D5 原文四点不含点名用例),仅存在于 Leader 给我的派单消息。
按本轮刚固化的纪律——**追加需求必须进 fix_items,验证者只能对在册项负责**——
我不因在册外要求而 reject。若 Leader 认为仍需写进 design §9,建议按纪律补进 fix_items 再派。

### ② ADR#8 权威条目未标注待修订 —— **dev 机制上写不了**

D5-④ 原文含「ADR#8 处同步登记待修订」。核查:

- ADR#8 的权威定义在 **`.arcforge/docs/01-design/architecture-decisions.md` 第 8 条**
  (该文件用纯编号列表 `8. **symbol 形态** — …`,不带 "ADR#8" 字样,故常规 grep 找不到)。
- 该条目**无「待修订」标注**(`grep -c 待修订` = 0)。
- **`.arcforge/write-matrix.json` 中 `docs/01-design/*` 的 writers 只有 `leader`**
  ⇒ dev-agent-34 经写通道写该文件会被 **DENY**,即便补报进 writes 也无法执行。

⇒ 这是**权限边界所致,不是 dev 疏漏**。dev 已把能力范围内的两处引用
(design §9:439「对港股不成立,**待修订**」、§9:447「届时**同步修订 ADR#8**」)全部标注。
**该条目的更新是 Leader 的动作项。**

**附带发现(同一文件,建议一并处理)**:ADR **第 3 条**同样已过时——
现写「40203 定义为永久性错误(ErrNoPermission),不触发降级链重试」,
而 TASK-001 已拆出 `ErrRateLimited` 把限频与权限分开。两条 ADR 都需 Leader 亲自更新,
否则下游按 ADR 施工会得到与现行代码相反的前提。

## 六、范围与哨兵

**改动范围**:`git diff --name-only 356d07c -- docs/ deploy/ scripts/` 仅三份文档——
`docs/prism/atlas_prism_design.md`、`docs/superpowers/specs/…-m3.5-design.md`、
`docs/superpowers/plans/…-m3.5a-datasources.md`,**全部在 7 项 `writes` 声明内,无越界**。
首轮已提交于 356d07c 的 plist / 两个 ops 脚本 / deployment.md **本轮未再改动**
(deployment.md mtime 仍为 05:22:16Z),首轮 5 条 DoD 结论继续成立。

**密钥哨兵(亲跑)**:两个 key 前 8 位在 `docs/` 全目录、`git diff`、`git diff --cached`、
`git log -p --all -S`(全历史全分支)命中**全为 0**;反向对照 `configs/config.yaml`
命中 **2** 次,证明检索有效。

## 七、判定

D5-D8 **四条逐条通过**,且每条的关键事实我都独立核实而非采信 dev 陈述:
D5 的四要素逐项复查(对照我上轮的缺失清单)、D6 经 grep 最新代码确认无任何退避原语、
D7 亲验了行号腐烂到 `Report`/`degrade` 的实证并核对四个新函数名真实存在、
D8 两个数字均经查库复核。范围无越界,哨兵全历史 0 命中。

两项未尽事宜(用例点名、ADR#8 条目标注)分别属于**在册外要求**与**dev 无写权的
leader 专属路径**,已在上文详列并交 Leader 处置,不构成本任务缺陷。

→ **VERIFIED**
