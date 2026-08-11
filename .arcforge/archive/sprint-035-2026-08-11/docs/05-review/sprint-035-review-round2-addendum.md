# Sprint 035 · 第二轮增补（两条漏项）

**为什么有这份文件**：`sprint-035-review-round2-adversarial.md` 落盘之后，Skeptic 与 Minimalist 两个 lens
才把发现正文经 SendMessage 送达（此前它们的最终回复只有摘要，我是靠解析 transcript JSON 取得正文的 ——
即 round2 第 7.2 节登记的那个通道缺陷）。**逐条对照后发现两条我漏了**，均已自行实测证实，补录于此。

**⚠️ 不改变 verdict。** 两条都是 MINOR，都是潜伏项，`sprint-035-review-round2-adversarial.md` 的
**PASS（附强制登记条件）** 仍然成立 —— 判据（DoD 逐条字面满足 + MAJOR 全部潜伏）不受影响。

---

## R2-13 · MINOR — 两道加总闸对「同组内分项互换」**完全不敏感**，而那正是抽取层自认最脆的地方

**位置**：`validate.go:210` `gateDepositSum`、`validate.go:306` `gateCorpLoanReconcile`
**来源**：Skeptic lens MINOR-6，我独立复跑证实

加总是**置换不变量**：`a + b + c` 与 `c + a + b` 相等，所以两道对账闸对分项之间互相错位零鉴别力。

而 `profiles.go:196-198` 自己写着：「『短期贷款』在两个作用域里各出现一次、指向不同字段 ——
这是全篇唯一需要作用域的地方，**也是本任务存在的主要理由**」。
⇒ **分项错位是抽取层点名的头号风险，而校验层对它零覆盖。**

**证据**（隔离 worktree，`golden2025` 真实值）：

```
互换前: corp_short=48100 corp_mlt=88200 | dep_household=146400 dep_fiscal=6579

基线（不动）              rep.Passed=true  failed 的闸门数=0
企业短期 <-> 中长期 互换      rep.Passed=true  failed 的闸门数=0
住户存款 <-> 财政存款 互换     rep.Passed=true  failed 的闸门数=0
两组同时互换               rep.Passed=true  failed 的闸门数=0
```

**住户存款 146400 与财政存款 6579 相差 22 倍**，互换后**七道闸一道都没响**。

**为何判 MINOR 而非 MAJOR**：这不是实现缺陷 —— 加总闸在数学上**不可能**抓住置换，要求它抓是错的。
唯一能抓住组内互换的是 `magnitude_sanity`（`146400` 落进 `deposit_fiscal` 的合理区间会越界），
而该闸恒 `skipped{not_calibrated}` 至 M1c。

**建议**：**并入 round2 第 6 节的强制条件第 1 项** —— M1c 标定 `MagnitudeRanges` 时，
**优先覆盖这些互为同组的分项字段**，因为它是整套闸门里唯一能抓住组内错位的一道。
这给「为什么要标定 MagnitudeRanges」补上了一个此前没写下的、具体的理由。

> 顺带校准一处措辞（Skeptic 指出，我认同）：`validate.go:163` 称序关系「比任何容差检查都更直接地指向解析错误」——
> 那个论断只对 M0/M1/M2 成立（它们有真正的包含关系），**不要外推到另外两道加总闸**，
> 后者恰恰对最常见的解析错误（分项错位）失明。

---

## R2-14 · MINOR — `validate.go:111-112` 描述了一个**不可能发生**的失败模式

**位置**：`validate.go:109-112` `knownCheckIDs` 的文档注释
**来源**：Minimalist lens MINOR-4，我读码证实

原文：

```
// 手写的那份会在加闸门时静默过期：豁免配置照旧通过校验，而新闸门的 ID
// 被当成拼写错误拒掉——或者更糟，拼错的 ID 因为不在旧列表里反而被放行。
```

**后半句不成立。** `checkEnum`（`types.go`）的语义是：

```go
if slices.Contains(allowed, val) { return nil }
return fmt.Errorf("hestia: unknown %s %q (want %s)", ...)
```

⇒ **不在 allowed 内一律返回 error**。「不在旧列表里」的 ID 只会被**拒**，不可能「被放行」。

**手写清单真正的静默漏洞方向恰好相反**：清单里留着一个**已被删除**的闸门 ID，
于是「针对一道不存在的闸的豁免」被放行 —— 配置的人以为跳过了某道闸，而那道闸根本不存在了。

**`knownCheckIDs` 从 `gates` 派生这个决定是对的**（4 个调用点，是 `Thresholds.validate` 的真相源），
**只有这半句理由要改**。与 F8/F10/F16 同族：结论对、理由错，而理由是别人复现时的唯一入口。

---

## 附：与 lens 报告的差异（据实登记）

除上述两条外，两个 lens 送达的正文与我在 round1/round2 中已记录的条目**逐条对应**，无其他遗漏。
两处我**保留了自己的措辞而非 lens 的**，理由已写在正文里：

1. **Minimalist MINOR-5**（CONTRACTS 收口表）：它报「漏 6 个」暗示守卫缺口；我实测那 6 处**全部有测试**，
   判为**标签宽于规则**而非守卫缺口（round1 R1-8）。
2. **Skeptic MAJOR-1/2 的「今日可达」**：我判潜伏，分歧已在 round2 第 5.4 节公开登记，
   并给出了「若采用其口径，我支持哪一条窄返工」。

## 环境完整性

本次补验同样在 detached worktree 内进行，收尾已 `git worktree remove` + `prune`；
主工作区 `validate.go` 的 sha256 仍为 `84f502268a7056c1cec9cc0a3e566f55a802f38ea338c74c2c4541f3a60b10a4`
（与开工时一致），`git status --porcelain` 与开工时**逐字一致**。
