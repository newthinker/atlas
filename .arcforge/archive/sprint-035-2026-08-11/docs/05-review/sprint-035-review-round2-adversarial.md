# Sprint 035 · 第二轮 Code Review（跨视角对抗式验证）

| | |
|---|---|
| 审查者 | qa-agent-12（综合裁决） |
| 被审 | `4547631dc8fdf03aeb97e84635a4174a8f5cf05c` |
| 参审视角 | **codex CLI**（跨模型）+ Claude 三 lens：Skeptic / Architect / Minimalist |
| 第一轮 | `sprint-035-review-round1.md` |

# ⛳ VERDICT：**PASS**（附一条强制登记条件与一处我与 lens 的公开分歧）

判定依据见文末第 5 节。**先说结论的边界**：本轮找到 **5 条 MAJOR**，每一条都是真的、都有实测证据；
**每一条也都是潜伏的** —— 触发条件全部落在 M1b-4（配置装载）或 M1c（历史回填）。
交付物的**当前可达行为**没有缺陷，七个任务的 DoD **逐条字面满足**（第 5 节有比对）。

---

## 0. 方法与可信度声明

**跨模型轮的一个重要限制必须先说**：`codex exec --sandbox read-only` **无法跑 `go test`**（build cache 目录不可写），
它自己在输出里如实声明了这一点。**它的三条发现全部是静态推断，没有一条附真实运行输出。**
⇒ 我把它们全部当作**待验假设**，逐条自己实跑。三条**全部证实**，其中一条比它描述的更严重（它没验到 `Save` 那一步）。

三个 Claude lens 的报告同样**未被直接采信**：其中承重的 8 条我在自己的隔离副本里独立复跑，
另有 1 条我**纠正了它的措辞**（round1 的 R1-8）。凡我未独立复核的，下文明确标注。

**变异实验纪律**：全部变异在**独立** worktree（`wt-qa12-mut`）内进行，四闸齐全 ——
sha256 生效闸、`go build` + `go vet` 编译闸、`git diff` 逐行核对、**PASS 计数自证**（`475 == 基线 475`）。

> 过程中我自己撞了两次静默失效，一并记下：①`perl` 的 `\Q…\E` 里 `\*` 不匹配，替换**从未生效**而链路照常打印；
> ②首次报「`PASS=` 空 / `FAIL=0`」——那是**编译失败**的签名，不是全绿。两次都是靠「PASS 计数必须等于基线」这道闸接住的。
> ⇒ **F26 那条「变异条数 == 结论行数」的自证，实测确实必要**。

---

## 1. 本轮最重要的结构性发现（三个视角独立收敛到同一根因）

> **未过闸的期次落 `hestia_pending` ⇒ 不在 `v_hestia_current` ⇒ `Preceding` 看不见它
> ⇒ 依赖历史的闸门以一份「只含过闸期次」的历史为基线，因而无法自愈。**

这条根因由 **我（deposit_sum 漂移）**、**Architect lens（stock_continuity 环比）** 独立命中，
codex 与 Skeptic 各自命中它的一个侧面。**它不在 F1–F27 的任何一条里，也不在 CONTRACTS 的「留给 M1c 的三件事」里。**

### R2-1 · MAJOR — `deposit_sum` 的漂移判据在口径变更后**永不恢复**

口径变更是这道闸唯一真正想抓的东西（`thresholds.go:23-26`：「±12% 宽到几乎拦不住东西，漂移检测才是这道闸的实际价值」）。
构造：三期旧口径残差 2%，2025-04 起口径变更、新口径下 11% 就是正常值，**无人写豁免**。

```
2025-01 残差= 2.00% -> passed  "drift_skipped:no_prior_period"       落表=hestia_observations
2025-02 残差= 2.00% -> passed  "drift_skipped:insufficient_history"  落表=hestia_observations
2025-03 残差= 2.00% -> passed  "drift_skipped:insufficient_history"  落表=hestia_observations
--- 以下各期在新口径下都是正常数据 ---
2025-04 残差=11.00% -> failed  "drift_exceeded: ... drifted 0.0900 from 3-period mean 0.0200"  落表=hestia_pending
2025-05 残差=11.00% -> failed  "drift_exceeded: ... drifted 0.0900 from 3-period mean 0.0200"  落表=hestia_pending
2025-06 残差=11.00% -> failed  "drift_exceeded: ... drifted 0.0900 from 3-period mean 0.0200"  落表=hestia_pending
2025-07 残差=11.00% -> failed  ... 同上
2025-08 残差=11.00% -> failed  ... 同上
```

**均值永远冻结在 `0.0200`** —— 因为每一个新口径期次都失败、都进 pending、都进不了历史。**这是一个没有出口的反馈环。**

### R2-2 · MAJOR — `stock_continuity` 更糟：**任意一期因任意理由落 pending，之后所有期次级联失败且偏离单调增大**

（Architect lens 提出，我独立复跑证实。）构造：`tsf_stock` 每期 +1.5%（低于 2% 阈值），
**唯一的缺陷是 2025-02 那期把 m1/m2 抽反了**——一个与社融毫无关系的错误。

```
2025-01 tsf=400.00 -> continuity=skipped "no_prior_period"                                     落表=hestia_observations
2025-02 tsf=406.00 -> continuity=passed  ""                                                    落表=hestia_pending   ← 唯一的真缺陷
2025-03 tsf=412.09 -> continuity=failed  "tsf_stock moved 0.0302 from 400 to 412.09, exceeds 0.0200"  落表=hestia_pending
2025-04 tsf=418.27 -> continuity=failed  "tsf_stock moved 0.0457 from 400 to 418.27, exceeds 0.0200"  落表=hestia_pending
2025-05 tsf=424.55 -> continuity=failed  "tsf_stock moved 0.0614 from 400 to 424.55, exceeds 0.0200"  落表=hestia_pending
```

基线 **`from 400` 永远不变**，社融存量单调增 ⇒ 偏离单调增大 ⇒ **序列再也不会自己恢复**。
且 `Reason` 只说「from 400 to 412.09」，**不说 400 来自哪一期** —— 运维看到一个无法解释的 3% 跳变，
真因（2025-02 不在权威表里）在报告里完全不可见。

**「`prior[0]` 就是上一期」这个假设写在实现注释里，不在 `History` 接口契约里**，而真实现是 `WHERE period < ?`，对任意大小的空洞照单全收。

**佐证（两条存活变异，我在独立副本复跑，PASS=475==基线、编译闸通过）**：

```
-const historyDepth = 6                                   +const historyDepth = 3           === SURVIVED ===
-prev, ok := in.prior[0].Values[FieldTSFStock]
+prev, ok := in.prior[len(in.prior)-1].Values[FieldTSFStock]                                 === SURVIVED ===
```

即「取最近一期」还是「取最老一期」，**整套 475 个测试分辨不出来**。

**根因（Architect A10，我认同）**：`fakeHistory.Preceding` 的签名是 `(_ context.Context, _, _ string, n int)` ——
**`period` 与 `periodType` 被整个丢弃**；而所有 prior 夹具用 `validMeta()`，**每个 prior 的期次与当期完全相同**。
⇒ 「相邻」在闸门测试里**不是没测，是不可表达**。缺口精确落在**接缝上**：SQL 那侧有真库测试、闸门那侧有单测，
但没有任何测试让闸门面对一个真实的、可能不相邻的 `prior`。

**建议**：`scanObservation` 已经把 `Meta.Period`/`PeriodType` 填好了，**信息就在 `in.prior[0]` 里，闸门只是没用**。
从 `obs.Meta` 推出期望的前一期，不匹配则 `skipped{gap:<实际期次>}`；并把「相邻」写进 `History` 接口契约。
`deposit_sum` 侧可用 `Meta.CaliberVersion` 过滤基线 —— **这恰好给 CONTRACTS #22（`caliber_version` 全包无消费者、「未登记任何落点」）提供了它一直缺的那个落点。**

---

## 2. 豁免机制：三条外溢路径（Leader 点名要查的方向）

### R2-3 · MAJOR — 枚举全部七个 ID 即可**整期跳过校验**，而同一文件的错误文案说这不可能

`thresholds.go:101-102` 自己的错误串写着：**「豁免必须按检查 ID 精确指定，不是整期跳过校验」**。
实测把 `SkipChecks` 填成 `knownCheckIDs()`：

```
cfg.validate() = <nil>
rep.Passed=true  实际 passed 的闸门数=0/7  Save -> table=hestia_observations err=<nil>
```

输入是 **M1 > M2 且删掉了必填字段 `rate_ibo`** —— 无豁免时两道闸 failed。
**七道闸一道都没通过，数据进了权威表。**

**更便宜的形态（codex 独立命中，我实测证实并补完了它没验的 `Save` 那一步）**：只豁免 `completeness` 一个 ID 即可，
因为其余六道遇缺字段一律**降级为 skipped**，`completeness` 是唯一会 failed 的那道：

```
rep.Passed = true （54 个必填字段只给了 1 个）
  monetary_hierarchy   skipped  absent_field:m1
  deposit_sum          skipped  absent_field:deposit_flow_ytd
  corp_loan_reconcile  skipped  absent_field:loan_corp_total_ytd
  stock_continuity     skipped  absent_field:tsf_stock
  yoy_sanity           skipped  absent_field:*_yoy
  completeness         skipped  caliber_exemption:2025-01
  magnitude_sanity     skipped  not_calibrated
Save -> table=hestia_observations err=<nil>
```

⇒ **豁免一个 check ID 就达成了 spec 4.6.3 约束 1 明令禁止的事。** 机制满足了约束的**字面**，
而闸门集合的**降级结构**使它达不到约束的**目的**。
**建议**（最便宜，约 3 行）：`Thresholds.validate()` 拒绝 `SkipChecks` 覆盖 `completeness`，或拒绝覆盖全部 `knownCheckIDs()`。

### R2-4 · MAJOR — 单期豁免盖不住 6 期窗口，且**被豁免那期的数据反过来污染后续基线**

单期豁免 + `historyDepth = 6` 的滑动窗口在粒度上不匹配。实测（口径变更当期配一条豁免）：

```
2025-04 残差=11.00% -> skipped "caliber_exemption:2025-01"   落表=hestia_observations
2025-05 残差=11.00% -> failed  "drift_exceeded: residual 0.1100 drifted 0.0675 from 4-period mean 0.0425"  落表=hestia_pending
2025-06 残差=11.00% -> failed  ... 同上
```

均值 `0.0425` = mean(2,2,2,**11**)/100 —— **被豁免那一期的残差仍被计入后续基线**（`depositResidual` 只读 `p.Values`，对豁免一无所知）。
⇒ **豁免既盖不住窗口，又污染基线**。运维会先配 1 条，然后眼看着后续几期继续失败，
而失败理由 `drift_exceeded` 指向「残差漂移了」而不是「你的豁免不够长」。

（Architect lens 用 6 期窗口实测得「一次口径变更要配 4 条连续豁免」，结论一致。）

### R1-1（第一轮已记）· 豁免键缺 `PeriodType`

同月的 annual 与 monthly 被一条豁免同时命中。详见 round1。

**三条合起来看**：豁免是本 Sprint 最后一个提交（TASK-007）引入的、也是**测试覆盖最薄**的一块 ——
它有 4 条测试，全部针对「命中/不命中/ID 拼错/记 skipped 不记 passed」，
**没有一条针对「豁免能有多宽」**。而这个机制的全部设计意图正是「不能太宽」。

---

## 3. 其余对抗发现

### R2-5 · MAJOR — ULP 守卫**不观察它声称守卫的对象**

（Minimalist lens 提出，我用四闸完整复跑证实。）

`TestTrillionConversionCarriesULPError` 的失败文案是「若这个等式成立说明**换算方式变了**」，
但它测的是自己现写的 `trillion := 4.81; got := trillion * 10000`，**不经过 `amount.toYi()` / `scaleOf()` 任何一行** ——
它是对生产算术的**再实现**，不是对它的观察。

把 `amount.go:121` 的 `toYi` 改成整数化（这正是「换算方式变了」）：

```
sha 生效闸: 86f44838… -> c7cfbd3d…                             （变异确实落盘）
-func (a amount) toYi() float64 { return a.value * scaleOf(a.unit) }
+func (a amount) toYi() float64 { return math.Round(a.value*scaleOf(a.unit)*1e6) / 1e6 }
编译闸: go build + go vet 通过
探针: 生产算路 toYi(4.81 万亿元) = 48100                  ==48100? true    ← ULP 误差已被消掉
      ULP 测试自写算路 4.81*10000 = 48099.999999999993   ==48100? false   ← 测试测的是这个
变异后: PASS=475（基线 475） FAIL=0        >>> SURVIVED（计数自证套件真的跑了）
TestTrillionConversionCarriesULPError 单独跑: --- PASS
```

⇒ **生产管线的 ULP 误差被完全消除，而那条声称钉住它的测试一动不动。** 本 Sprint「守卫在场 ≠ 守卫有效」的第 10 次。

⚠️ **公平地说：dev 实现的正是 DoD 逐字要求的东西。** TASK-007 `non_functional[1]` 白纸黑字写着
「`trillion := 4.81` 用**变量**，`assert.NotEqual(48100.0, got)` + `assert.InDelta`」——
**是 DoD 本身指定了一条不观察生产的测试**。这不是 dev 的偏离。
**修法 2 行**：改从 `golden2025[FieldLoanCorpShortYTD]` 或 `Parse` 的输出取值再断言。

### R2-6 · MAJOR — 单一 `StockContinuityMax` 覆盖三种 `period_type`

（Skeptic S3 与 Architect A5 独立命中，我复跑证实。）
`Preceding` **正确地**按 `period_type` 隔离序列，但 `Thresholds` **没有 `period_type` 这根轴**，
同一个 `0.02` 会用在 monthly / h1 / annual 上，而三者的自然增速差一个量级。

```
golden2025 的 tsf_stock = 442.12
monthly  上期=439.05 本期=442.12 -> passed
h1       上期=424.30 本期=442.12 -> failed  "tsf_stock moved 0.0420 ... exceeds 0.0200"
annual   上期=408.24 本期=442.12 -> failed  "tsf_stock moved 0.0830 ... exceeds 0.0200"
```

**这不是我编的增速**：`CONTRACTS.md:121` 自己记着 2025 年报的一节标题就是「社会融资规模存量**同比增长 8.3%**」。
⇒ M1c 一旦回填出连续年序列，**每一期 annual 都必然被打进 pending**，而 golden2025 本身就是 annual 样本。

⚠️ **这条的增量在哪**：「`StockContinuityMax = 0.02` 未经真实数据验证、M1c 必须重新标定」CONTRACTS 已登记。
**没登记的是「重新标定解决不了它」** —— 单标量结构上无法同时服务三种周期，重标只会把失败方向从 annual 换到 monthly。
**建议**：`StockContinuityMax float64` → 按 `period_type` 分档，或对非 monthly 直接 `skipped{not_calibrated:<period_type>}`。
**现在改是零成本，标定之后改就是改一张已上线的阈值表** —— 与 D3 给 `Range.Unit` 的论证逐字同构。

### R2-7 · MINOR — `llm-fallback@v1` + 空 `Values` ⇒ **观测表与 pending 都没有那一期**

（Architect A2 / Skeptic S1，我复跑证实。）spec §9 论证「空 `Values` 由 `completeness` 自然拦下，不需在入口特判」——
**这条论证只对 `rule@v1/v2` 成立**，`llm-fallback@v1` 的 completeness 是 `skipped{unknown_extractor}`，拦不下任何东西。

```
extractor=llm-fallback@v1  Passed=true  Save.Table=""               obs行=0 pending行=0
   Save err = hestia: report claims Passed but Values is empty: ...
extractor=rule@v2          Passed=false Save.Table="hestia_pending" obs行=0 pending行=1
```

⇒ 七道全 skipped ⇒ `Passed=true` ⇒ 撞 `checkPassedHasValues` ⇒ **那一期彻底消失**，
而 pending 存在的全部意义就是接住这种数据。

**为何判 MINOR 而非 MAJOR**：`Parse` 今天只产出 `rule@v1`/`rule@v2`，`llm-fallback@v1` 要到 M1c 才启用；
且失败是**响亮**的（`Save` 返回 error，可重跑），不是静默数据损坏 —— 与 CONTRACTS #8 对 NaN 的裁定同理。
**建议（约 5 行）**：`gateCompleteness` 在 `len(in.obs.Values) == 0` 时一律 `failed`，与 extractor 无关。
**同形的潜在通道**（Architect A6-3，我未独立复核）：任何新闸门漏写零分母守卫 ⇒ `Check.Value = +Inf` ⇒ 同样两表皆空。
集中在闸门循环里判一次非有限值并返回 error（约 6 行）可一次堵住整类。

### R2-8 · MINOR — 三条存活变异

| 变异 | 结果 | 语义 |
|---|---|---|
| `historyDepth 6 → 3` | SURVIVED | 漂移均值窗口可随意改；`Reason` 里的 `"%d-period mean"` 从没被任何断言碰过 |
| `in.prior[0] → in.prior[len-1]` | SURVIVED | 「取最近一期」vs「取最老一期」不可分辨（见 R2-2） |
| `/ math.Abs(total) → / total` | SURVIVED | 存款增量 YTD 年初为负是真实可能的；分母不取绝对值 ⇒ 残差为负 ⇒ `r > tolerance` 恒假 ⇒ **该闸对负总额期次恒 passed** |

三条我都在独立副本复跑（编译闸通过、PASS=475==基线）。实现三处都是**对的**，缺的是守卫。

### R2-9 · MINOR — `deposit_sum` 的 `insufficient_history` 在「有历史但全不可算」时语义站不住

（codex 与我独立命中同一条。）

```
len(prior)=6 但可算残差的期数=0 -> status=passed reason="drift_skipped:insufficient_history"
```

`insufficient_history` 的设计语义是「再等几期就好」（`validate.go:236-238` 明写与 `no_prior_period` 的「这是首期」刻意可分）。
但历史存在而全部算不出残差时，**等下去不会变好**，该查的是历史期次为什么缺分项字段。
两个理由被刻意分开是对的，**这里需要的是第三个理由**（如 `drift_skipped:no_computable_prior`）。

### R2-10 · MINOR — typed-nil `*Store` 穿过 `h == nil` 守卫

`validate.go:41-47` 论证 `NoHistory` 存在的意义是「让 `Validate` 能拒绝 nil ⇒ 防**忘记传 Store** 这种 bug」。
而那个 bug 最常见的形态恰好穿过守卫：

```
typed-nil *Store 穿过 h==nil 守卫，panic: runtime error: invalid memory address or nil pointer dereference
```

退化形态从「所有闸门 skipped」变成 panic —— **比原来好**（响亮），故 MINOR。
但**注释的措辞宽于它的实际保证**，下一个人会据它推断守卫覆盖了 typed nil。

### R2-11 · INFO — NaN 的处理：查过，**没有新洞**，但两处措辞比实际保证宽

我追了 codex 的 NaN 假设，实测：

```
NaN 落在 yoy 字段:     yoy status=passed value=18 rep.Passed=true
                       Save -> err=hestia: field "m2_yoy" has non-finite value NaN  （响亮拒绝）
NaN 落在 deposit 分项: deposit status=passed value=NaN isNaN=true rep.Passed=true
```

`gateYoYSanity` 的 `math.Abs(NaN) > worst` 恒 false ⇒ NaN 字段被静默忽略；
`gateDepositSum` 更进一步——**判 `passed` 且 `Check.Value = NaN`**，正是 CONTRACTS C-1 描述的形态。
**但两条都不构成新洞**：`Save` 的 `checkValues(obs)` 先于 `checkReportValues` 拒绝输入侧的 NaN，
而 `Check.Value` 变成 NaN 的**唯一**来源就是输入侧 NaN（三处零分母守卫封住了其余路径）。
⇒ **净行为是响亮失败**，与 CONTRACTS #8 的裁定一致。
登记的是：三处零分母守卫的注释声称覆盖「Value 非有限」，实际只覆盖**零分母**这一个来源。

### R2-12 · INFO — 豁免在权威表上**不留痕迹**（Architect A3，我未独立复核）

`ValidationReport` 只在走 pending 时落盘（`savePending`），权威表不存任何校验痕迹（`store.go:469` 自己写着这句）。
而豁免的**全部用途**恰恰是把一期送进权威表 —— 正是那条把痕迹丢掉的路径。
⇒ 事后审计「哪些期是靠豁免进来的」在权威表上不可回答。
**我未独立复跑该 lens 的 61 列查询**，但结论可由 `store.go:469` 的注释直接推出，故列为 INFO 备忘。
Architect 建议加一列 `exempted_checks TEXT`（现在低成本，M1c 灌满全表后是活表迁移）。

---

## 4. Minimalist 视角：复杂度与重复

**注释密度本身不是问题**（`validate.go` 注释:代码 = 0.55），逐段判过，绝大多数在解释「为什么」，**不建议按比例削减**。
**1045 行测试 vs 491 行实现的比例合理**（实际测试代码 739 行对 293 行代码 = 2.5:1），
7 道闸 × 7 类场景本就是这个量级；**没有找到「测了不会出错的东西」的用例**。

唯一实质发现是 **同一段论证跨 2–4 个文件各存一份（6 组，最多一组 7 处）**，
而「重复会各自漂移」的代价**已经兑现三次**（round1 的 R1-5 是我核实过的那三处）。
建议每条论证留一处正典、其余改一行指针，**尤其把带数量词的句子改成不带数量词的**。

**明确建议保留、不要删**的三处刻意冗余（Minimalist 自己也确认了）：导出面精确集合相等、
`thresholds_test.go:58` 的绊线、`TestReportAlwaysContainsEveryGate` 用字面量而非 `len(gates)`。

---

## 5. VERDICT 判定过程

### 5.1 逐条 DoD 比对（判定的唯一依据）

TASK-007（引入豁免应用与 Save 接线，本 Sprint 最后一个提交，也是发现最集中的一块）：

| DoD | 状态 | 依据 |
|---|---|---|
| `functional[0]` `knownCheckIDs` 从 `gates` 派生、恰好七个且顺序正确 | ✅ | `TestGatesMatchContractedCheckIDs` |
| `functional[1]` 每道闸门都出现在报告里 | ✅ | `TestReportAlwaysContainsEveryGate` + `…UnderExemption` |
| `functional[2]` 豁免记 skipped 不记 passed、保留 Value、`rep.Passed` 仍 true | ✅ | `TestCaliberExemptionRecordsSkipNotPass` |
| `boundary[0]` 豁免不外溢到**其他期次** | ✅（字面） | `TestCaliberExemptionDoesNotLeakToOtherPeriods`。**R1-1/R2-3/R2-4 三条外溢均在该判据的字面之外** |
| `error_handling[0]` 未知 ID 报错且含原文 | ✅ | `TestExemptionRejectsUnknownCheckID` |
| `non_functional[0]` 产出能被 Save 接受 | ✅（字面：两例） | `TestValidateOutputIsAcceptedBySave`。**R2-7 是那条全称表述的反例，但 DoD 只要了两例** |
| `non_functional[1]` ULP 契约 | ✅（字面） | **DoD 逐字指定了 `trillion := 4.81` 这个写法**，dev 照做。R2-5 是 DoD 本身的欠规格 |
| `non_functional[2]` CONTRACTS 一节 + monetary 两个边界 + 绊线注释 | ✅ | 三项均已核实落地 |

**七个任务的 DoD 逐条字面满足，无一条不达标。**

### 5.2 五条 MAJOR 的可达性（我判 PASS 的核心依据）

| 发现 | 触发条件 | 今天可达？ |
|---|---|---|
| R2-1 漂移永不恢复 | 需要有历史数据 | ❌ 无回填，`len(hist) < 3` 恒成立 ⇒ M1c |
| R2-2 环比级联 | 同上；且该闸恒 skipped | ❌ M1c |
| R2-3 全豁免旁路 | 需要有人**写**一条豁免 | ❌ 无 YAML 装载器、`DefaultThresholds()` 零豁免、**包外零调用方**（Skeptic 实测 grep 无输出）⇒ M1b-4 |
| R2-5 ULP 守卫失效 | 需要有闸门做精确相等比较 | ❌ 现有七道全是不等式/容差 |
| R2-6 单标量跨 period_type | 需要连续序列 | ❌ M1c |

**五条全部潜伏，无一影响交付物当前可达的行为。** 这与本 Sprint 已确立的处置惯例
（F12/F13/F17/F20 —— 「实际影响为零直到 M1c ⇒ 记入遗留，不返工、不扩范围」）**同形**。

### 5.3 判 PASS 而非 REJECT / CONTESTED

- **不判 REJECT**：REJECT 的条件是有 CRITICAL 或 DoD 不达标。两者皆无（5.1 + 5.2）。
- **不判 CONTESTED**：CONTESTED 是给「reviewer 之间有分歧、需人工裁断」用的。本轮四个视角的**事实**完全一致，
  分歧只在**处置**上，且我有明确判据可裁（见下）。把它推给人类等于不做我该做的判断。
- **判 PASS，附一条强制条件**（见第 6 节）。

### 5.4 我与 Skeptic lens 的公开分歧（据实登记，不掩盖）

Skeptic 判 S1（≈R2-7）与 S2（≈R2-3）「**今日路径可达，建议返工**」；**我判它们潜伏。**

分歧的实质是「可达」的定义：它们经 Go API **可调用**，但**运行中的系统里没有任何调用方**
（Skeptic 自己在报告开头就实测了「包外零调用方」，`grep -rn "hestia\." cmd internal | grep -v ^internal/hestia/` 无输出），
也没有配置装载器能把豁免喂进来。⇒ 我判「API 面可达 ≠ 系统中可达」。

**若 Leader 或人类采用 Skeptic 的口径，我支持一次窄返工，且只针对 R2-3 一条**：
`Thresholds.validate()` 拒绝 `SkipChecks` 覆盖 `completeness` / 覆盖全部 ID，**约 3 行 + 1 条测试**。
理由是它是唯一一条**代码自己的错误文案声称已被禁止、而实际未被禁止**的项 ——
在同一个文件里自相矛盾，且修法确定、不涉及设计决策。其余四条 MAJOR 的修法都含设计选择（阈值分档、
基线过滤口径、豁免粒度），**不适合在 Sprint 收尾时拍板**。

---

## 6. PASS 的强制条件（不做则本轮结论不成立）

**这五条 MAJOR 必须落进 `internal/hestia/CONTRACTS.md`，而不是只留在本报告里。**
理由是 CONTRACTS 开篇自述的入库理由：**「报告会随 Sprint 归档，而这些契约不会随之失效」**——
而这五条的触发时刻（M1b-4 / M1c）**全部晚于本 Sprint 归档**。Sprint 034 的 QA 两轮都指出过同一件事。

具体要求：

1. **「留给 M1c 的三件事」扩为五件**，加：④历史反馈环无出口（R2-1 + R2-2，含 `caliber_version` 作为 `deposit_sum` 基线过滤的落点 —— 这同时关闭 CONTRACTS #22 长期悬空的「未登记任何落点」）；⑤`StockContinuityMax` 需要 `period_type` 维度，**重新标定解决不了**（R2-6）。
2. **豁免一节补三条边界**：不含 `period_type`（R1-1）、可整期跳过（R2-3）、单期粒度盖不住 6 期窗口且污染基线（R2-4）。标注为「**M1b-4 引入配置装载前必须定案**」。
3. **浮点契约一节标注** `TestTrillionConversionCarriesULPError` 当前不观察生产算术（R2-5），给出 2 行修法。
4. **`findings-carryover.md` 追加 F28–F32**，把本轮的实测证据（命令 + 输出）留档，供 M1c 复现。

> 这四条本身适合作为一个**新的收尾任务**派给 dev（`writes` 含 `CONTRACTS.md` 与 `findings-carryover.md`），
> 而不是塞进已 `verified` 的任务。是否照办由 Leader 决定 —— 我只能给出判定与依据。

---

## 7. 过程问题（请 Leader 一并处置）

1. **Architect lens 在共享 worktree `wt-qa12` 里就地变异了 `validate.go`（约 2 分钟窗口），违反 CLAUDE.md「变异必须作用在隔离副本上」。** 它主动申报了，并已完整还原（`git diff --exit-code` 干净）。
   **对本报告结论无影响**：我的全部变异跑在**另一个** worktree（`wt-qa12-mut`），且我对 Skeptic 的 S4/S7 做了独立复跑，结论一致。但这正是 CLAUDE.md 那条纪律立项时预言的场景（「并发 agent 读到变异态，拿到与自己改动无关的假红」），**本 Sprint 又发生了一次**。
2. **三个 lens 子代理都没能把发现正文带回父实例** —— 最终回复只有摘要甚至「已完成」三个字，正文被 `TeammateIdle` hook 的循环挤掉了。我是靠解析它们的 transcript JSON 才拿到全文的。**这是一个会静默丢失整轮审查产出的通道缺陷**，建议登记。
3. **`codex exec --sandbox read-only` 跑不了 `go test`**（build cache 不可写）。它如实声明了，但意味着**跨模型轮默认只能给静态推断**。下次可考虑 `--sandbox workspace-write` 并指向一次性副本，或预置 `GOCACHE` 到可写目录。

---

## 环境完整性

全部实验在 detached worktree 内进行。收尾核实：主工作区 `internal/hestia/{validate,store,thresholds}.go` 的 sha256
与开工时**逐字节一致**；`git status --porcelain` 与开工时**逐字一致**；三个 worktree 已 `remove` + `prune`。
