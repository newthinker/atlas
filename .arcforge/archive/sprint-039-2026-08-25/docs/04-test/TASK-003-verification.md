# TASK-003 验证报告 — 分布统计与报告渲染

- **验证者**：test-m1c2-v1　　**判定：VERIFIED（8/8 done_criteria 通过）**
- **被验对象**：`9c74a13ee2a8072723da7ce057cf58d29d690386`（主线 merge commit）
- **基线**：`6bb60b4689eaecc8cd6979d17cd3c2b4f2164ae7`
- **HEAD 漂移**：当前 HEAD == `verify_baseline.head` == `9c74a13ee…` ⇒ **无漂移**
- **discovery 漂移**：🔴 **有**（见 §五 V3），转 `verified` 时已显式 `--ack-discovery-drift`
- **测自哪棵树**：`../wt-v-T003 @ 9c74a13ee…`，基线数字取自 `../wt-v-T003-base @ 6bb60b4689…`；
  主工作区源码一个字节未碰（每组消融后 `git status --porcelain` 均为空）

---

## 一、Done Criteria 覆盖矩阵

| # | 完成标准（摘要） | 对应测试 | 我做的独立证据 | 判定 |
|---|---|---|---|---|
| **f0** | `computeFieldStats` nearest-rank 不插值；1..100 乱序 ⇒ P5=5/Median=50/P95=95/Min=1/Max=100/N=100 | `TestComputeFieldStatsUsesNearestRank`、`TestComputeFieldStatsDoesNotMutateInput` | 读断言：两条前提（无序、恰是 1..100 置换）均为 `require`，挡住「不排序直接取下标」 | PASS |
| **f1** | 按 `fieldOrder` 全序渲染，**`n` 列不可省**；断言须钉整行形状 | `FollowsFieldOrder`（整序列 `Equal`）、`ShowsPerFieldSampleCount`（按列取） | **A1 复现**：n 列恒打 0 ⇒ **恰好 1 条红**，符合 DoD「红且只红那一条」 | PASS |
| **f2** | 每行标注样本来自哪几种 `period_type`（含条数） | `AnnotatesPeriodTypeMix` | 读断言：`m2[2]` 逐列取，含 `annual×3/h1×2/q1×1`；单一来源格断 `Equal` 防虚报 | PASS |
| **f3** | 新增一节：`tsf_stock` 逐 `period_type` 的**相邻期**环比 n/min/p5/median/p95/max | `Grouped`／`PairAdjacentSamplesAcrossGaps`／`SkipZeroDenominator`／`IncludesStockContinuitySection` | **B2/B3 独立复现**（§三）＋**渲染实际输出**确认六个统计量都在（§四） | PASS（带 V1） |
| **b0** | 加性余量 `[min-span,max+span]`，负值样本；**且有上界 K** | `TestSuggestionIsAdditiveNotMultiplicative` | 读断言：`K=3` 写死在测试里并注明推导 `(max+span)-(min-span)=3*span`；`assert.False(IsInf)` 另挡一层 | PASS |
| **b1** | `n<3` 不给建议；**n=2 两值必须不同** | `TestSuggestionWithheldBelowMinSamples` | 读夹具：n=2 那格用两个**不同**值 ⇒ 挡得住 `span>0` 写法（dev 的 A4 证实） | PASS |
| **n0** | 不得引入文件系统/网络访问 | — | `grep -E 'os\.(Open\|ReadFile\|WriteFile)\|filepath\.\|net/http'` **无匹配**；导入块仅 `bufio/fmt/io/maps/math/slices/strings` | PASS |
| **n1** | gofmt / vet / build / test / 覆盖率不低于基线 | — | 见 §二 | PASS |

---

## 二、n1 逐项证据（全部跑出来的）

| 项 | 结果 |
|---|---|
| `go build ./...` | OK |
| `go vet ./...` | 退出码 **0**，输出 **0 字节**（退出码单独取，未跨管道） |
| `go test ./... -count=1` | 退出码 **0**，**64** 个 `ok` 包 |
| **承重点** `TestCollectSamples*` | **27 PASS = 19 顶层 + 8 子测试**，**0 FAIL** |
| gofmt (a) 本任务 3 文件 | `gofmt -l` 输出 **0 行** |
| gofmt (b) 全仓 base vs post | `git ls-files '*.go'`（405 个）各 **28** 个未格式化，**逐字节一致**；本任务 3 文件不在名单 |
| scope | 实际改动 **3** 文件 == 声明 `writes` 3 条；无 `go.mod`/`go.sum` |

### 覆盖率（背对背 × 同轮同负载 × 各自树内渲染，NumStmt 加权）

| 包 | base `6bb60b4689…` | delivered `9c74a13ee…` | 结论 |
|---|---|---|---|
| `internal/hestia` | 95.241% (1501/1576) | **95.312%** (1606/1685) | 上升 |

两轮独立采样逐字节一致。**锚**：我的独立脚本复现出的两个值与 Leader 派验消息里给出的
`95.241%` / `95.312%` 逐位相同 —— 那两个数产生于我测量之前，构成对我口径的外部校验。
第二口径 `go tool cover -func` total：95.2% → 95.3%，**方向一致**。

⚠️ **不可与 TASK-001 报的 94.874% 比**：那是 `fba0feb→fa7c70f` 那一对，分母不同的树。

### ⚠️ 一条取数纪律（本次实撞，差点写进报告）

我第一次跑全仓 gofmt 用的是：

```bash
git ls-files '*.go' | xargs GOTOOLCHAIN=local gofmt -l 2>/dev/null
```

**`xargs` 会把 `GOTOOLCHAIN=local` 当成要执行的命令**（它不解析环境赋值），报错被 `2>/dev/null`
吞掉 ⇒ 输出为空、退出码 0，**看起来像「0 个未格式化文件，完美通过」**。而正确跑法得出的是
**28 个**。⇒ 取任何「0 / 空」结论前，先验证命令本身没有静默失效（这里的自证是
`git ls-files '*.go' | wc -l = 405`）。

---

## 三、Leader 点名的两格：B2 与 B3，我逐条看了断言消息

### B2（环比不分档，全混一起）—— dev 的自我订正**属实**

变异：`byType[r.PeriodType]` → `byType["all"]`。红 4 条。**逐条近因**：

| 红的测试 | 行号 | 断言消息 / 失败原因 |
|---|---|---|
| `Grouped` | `:401` | map 里只有 `"all"`，`Contains(got,"annual")` 失败 |
| `PairAdjacentSamplesAcrossGaps` | `:424` | `got["annual"]` 取到**零值** ⇒ N=0 |
| `SkipZeroDenominator` | `:441`/`:443` | 同上，N=0；`Max` 取到 0 |
| `IncludesStockContinuitySection` | `:459` | 报告里没有 `annual` 行 ⇒「实际 0 行」 |

⇒ **四条红出自同一个近因**（分组键消失 ⇒ `got["annual"]` 处处取到零值），
**不是四个性质各被破坏**。dev 在 discovery 里的订正与我实测**一致**。
B2 只证明了「分档」这一条性质，击杀数 4 **高估了它的鉴别力**。

### B3（改成「必须相差一年」配对）—— 复现成功，`PairAdjacentSamplesAcrossGaps` 在红名单里

红 3 条，近因同样单一（2019→2021 那对被丢 ⇒ annual 的 N 由 2 变 1）：
`:403`「annual 三期 ⇒ 两个相邻对」、`:408` Max 由 0.1 变 0.05、
`:424`「跨洞的 2019→2021 也是一对相邻样本」、`:460`「annual 两个相邻对」。

⇒ 结论成立：**「相邻期」= 排序后相邻的两个样本**，这条性质由 `:424` 那条**专门为它写的**断言
独立守住。击杀数 3 同样不构成「三个性质」。

**关于 dev 的自评判据**（「一个消融红几条，本身就是它精确度的度量」）：它在 A1 上执行了
（第一次红 2 条即判不达标并改成一测一属性），在 B2/B3 上事后如实标注了未执行。
Leader 已明示不阻断，我据此只报事实。

---

## 四、✅ 那处「真冒名」的修法：确认是消掉共有词，不是放松断言

`reportRow` 现在的形状（`calibrate_report_test.go:175-185`）：

```go
if cols := strings.Fields(ln); len(cols) > 0 && cols[0] == field {   // 首列**精确相等**
require.Len(t, hits, 1, "字段 %q 应当恰好占一行（首列精确等于字段名），实际 %d 行", ...)
```

**仍是「恰好 1 行」+「首列精确相等」，一个字都没放松。** 修法落在标题上：
`stockRateSectionTitle` 现为 `环比变化率分布：tsf_stock 相邻期（按 period_type 分档）`，
字段名已挪出行首。

**消融 T（我把标题退回 `tsf_stock 相邻期环比变化率…`）**：退出码 1，红 **4** 条 ——
`ShowsPerFieldSampleCount` / `MarksFieldsWithoutSamples` / `AnnotatesPeriodTypeMix` 三处
均报「字段 `tsf_stock` 应当恰好占一行，**实际 2 行**」，外加 `FollowsFieldOrder` 因序列
多出一项而红。⇒ **守卫真的还在守，且是多重的**；若当初在断言那边绕开，这 4 条会全绿。

## 四之二、f3 的六个统计量：我渲染实际输出看了

```
环比变化率分布：tsf_stock 相邻期（按 period_type 分档）
period_type                     n          min           p5       median          p95          max  建议区间
annual                          2         0.05         0.05         0.05          0.1          0.1  n<3
h1                              1         0.25         0.25         0.25         0.25         0.25  n<3
```

六个统计量（n/min/p5/median/p95/max）**全部在**，取值正确（annual 的两个环比是 0.10 与 0.05
⇒ min 0.05 / max 0.1；h1 是 |300-400|/400 = 0.25），且 `n<3` 规则**未破例**。
⇒ **DoD f3 的产品要求满足**。

---

## 五、发现（均不阻断，按先例处置）

### V1（中）min/p5/median/p95/max 五列**没有任何断言守护**

我自己设计的三组消融，**全部 SURVIVED（退出码 0，完全静默）**：

| ID | 变异 | 结果 |
|---|---|---|
| **C1** | 环比一节**丢掉 p5 列**（表头与数据行同时丢，形状自洽） | SURVIVED |
| **C2** | 环比一节 p5/median/p95/max **全打成 min** | SURVIVED |
| **C3** | 字段表 p5/median/p95/max **全打成 min**（对照组） | SURVIVED |

成因：`IncludesStockContinuitySection` 只读 `annual[1]`（n）与 `annual[len-1]`（建议列）；
`MarksFieldsWithoutSamples` 只断「有没有 `—`」；`ShowsPerFieldSampleCount` 只比第 2 列。
⇒ **中间五列的存在与取值，两张表里都没人守。**

**为什么仍判 PASS**（判据是先例，不是我的偏好）：
- 产品要求**已满足** —— 我渲染实际输出看过（§四之二），六个统计量都在且值正确；
- **计算层**（`computeFieldStats` / `stockContinuityRates`）被精确钉住，缺的只是**渲染层**的回归保护；
- DoD 为本任务点名了多条验收消融（f1 的「删 n 列 ⇒ 红且只红一条」、f3 的 n/HasSuggestion 不破例），
  **全部通过**；五列的回归保护**不在 DoD 点名之列**；
- 先例：同 sprint `TASK-002-verification.md` 的 **F5**（一条理由无测试守护、变异 SURVIVED）
  判定为「**DoD 未要求，故不阻断；建议后续任务顺手补**」，任务仍 VERIFIED。

⚠️ **这是本次唯一一处不同读法会得出不同结论的地方**，故显式标出：
若 Leader 认为 f3 字面列举的六个统计量应当逐个可消融，本条即构成 `task_defect`。
**建议的最小修法**（一行）：把 `IncludesStockContinuitySection` 的
`assert.Equal(t, "2", annual[1])` 扩成对整行列数与 min/max 两列的比对。
**我没有改** —— 验证者写被验代码 = 用自己的尺子量自己。

### V2（低）`nearestRank` 注释的理由，机制说错了（结论仍对）

注释称：「不写 `math.Ceil(0.05*float64(n))`：n=100 时它**碰巧**成立，**换个 n** 就可能差一名」。
消融 N1（改用浮点版）**SURVIVED** —— 但这**不是覆盖率缺口**，是**等价变异**：

实测（Go 与 Python 双实现互核，结果逐位一致）：

| 范围 | 整数版 vs 浮点版不同处 |
|---|---|
| **pct=5/50/95**（本实现实际只用这三个），n∈[1, 200000] | **0 处** |
| pct=1..99 全扫，n∈[1,5000] | 702 处，分布在 **12 个 pct**（7/14/17/27/28/34/54/55…） |
| **pct=7，首个差异** | **n=100** —— 恰好是 DoD 指定的夹具大小 |

⇒ **决策正确**（整数运算确实更稳，且一旦有人加 p7/p28 就立刻兑现价值），
但**理由的机制写反了**：脆弱性在 **pct** 上，不在 **n** 上。照注释去「换个 n」验证的人
会一无所获，进而可能误以为这条防御是多余的。**建议把注释改成按 pct 举例**（如 p7 在 n=100 即差一名）。

⚠️ 按 Leader 转述 dev 的判据「存不存在一个消融会让这条理由变成假的」——
本条**存在**（就是 N1），且它证明的正是「这条理由在现有参数下为假」，故不是零信息量。

### V3（流程）discovery 在 `verifying` 窗口内被改写

```
verify_baseline.discovery_sha256 = aa0b123be04d1c4d3e26dbec0b5d222082793a83fb215a2c5e565f1717887897
当前                              = ff55c61cf779ef45eedda3689142886eb007f28cd75c9ef2d142a8407a4ce9c8
派验时刻 = 2026-08-24T14:31:02Z（本地 22:31:02）
文件 mtime = 本地 22:32:48        ⇒ 派验后 **106 秒**被改写
```

内容判断（`transitions.jsonl` 不记 discovery 内容写，故无法逐字 diff）：Leader 派验时
discovery 尚无 B2/B3 的自我订正（Leader 正是因此让我去查），而当前版本已含带断言消息的详细订正
⇒ **dev 在我的验证窗口内补写了那两条订正**（Leader 说过不必回头补跑，dev 仍跑了）。

**这正是 AD-29 要防的形态，而它被逮住了。** 对本次判定**无实质影响**，理由是可核查的：
**我对 B2/B3 的结论来自我自己重跑并逐条读断言消息**（§三），不是采信 discovery 的陈述；
若我当时只读 discovery，就会是「拿 dev 在窗口内补交的证据去验 dev」。
⇒ 转 `verified` 时已按协议显式 `--ack-discovery-drift ff55c61c…`（取当前值，非记录值）。

---

## 六、结论

8 条 done_criteria **逐条有对应测试且断言非空洞**；DoD 点名的验收消融全部通过
（f1 的「红且只红那一条」实测**恰好 1 条**）。Leader 点名的两格已逐条核对断言近因，
dev 的自我订正**属实**；「真冒名」的修法确认是**消掉共有词**而非放松断言（消融 T 红 4 条）。
承重点 `TestCollectSamples*` **27 PASS/0 FAIL**，覆盖率 95.241% → **95.312%** 上升，
gofmt/vet/build/全量套件/无新依赖/无 scope 越界全部满足。

V1–V3 为改进建议与流程记录，**不构成退回理由**（V1 的不同读法已在 §五显式标出供 Leader 裁决）。

**判定：VERIFIED**
