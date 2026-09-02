# TASK-011 验证报告 —— load 报告的口径路由断言

- **验证者**：test-m1c4-a
- **判定对象**：`master @ 07d111309737de1aaa08bbe05dd22b31a1e07ffb`（= `verify_baseline.head`）
- **assignment_epoch**：**2**（两人接力，`rework_count` 未增——**这不是返工**）
- **结论：VERIFIED**

## 接力结构（据 discovery 的 `provenance`）

| 角色 | 实例 | commit | 内容 |
|---|---|---|---|
| 主体 | dev-m1c4-c | `ff49e79`（294/0 + 406/1），merge `328aad0` | `checkCaliberRouting`、`checkCaliberFamilyMedians`、报告两节、把判据换成累计恒等式、13 条测试、三处 DoD 订正 |
| 收尾 | dev-m1c4-b | `28ea907`（57/10 + 48/2），merge `07d1113` | **只改容差取值 + 跟着它的两处印法 + 补一条守卫测试**，前任其余代码一行未动 |

⚠️ `provenance` 的实际 key 是 **`🔴 provenance`**（带前缀），用 `.provenance` 取会得到 `null`。

---

## 0. 基线核对

| 项 | verify_baseline | 实测 | 一致 |
|---|---|---|---|
| `head` | `07d1113097 37…` | 同值 | ✅ |
| `discovery_sha256` | `1251d0b493cf…` | 同值 | ✅ |
| `assignment_epoch` | 2 | 2（我携带 `--expect-epoch 2` 出裁决） | ✅ |

`28ea907` 的 parent 实测确为 `328aad0`；两树间 diff 恰为本任务 2 个文件。
两个 commit 均**未碰** `go.mod`/`go.sum`；实改文件全在 `writes` 内，**无越界**。

---

## 1. done_criteria 覆盖矩阵（11 条）

| # | 维度 | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|---|
| F0 | functional | `checkCaliberRouting` + 复用 `caliberFamilies()`，导出面最小 | 类型名是**非导出**的 `caliberRouteViolation`（DoD 要求「先问需不需要导出」）；无第二份成对清单 | **PASS** |
| F1 | functional | 报告印四个数 + 逐对清单 + 族内中位数 | §3 真 load 输出 | **PASS** |
| F2 | functional | 四条测试存在且通过 | 13 条 caliber 测试全绿 | **PASS** |
| F3 | functional | **订正**：四类才构成划分 | `TestCaliberRoutingCountsArePartition`；实测 `4686 = 22×213` 闭合 ✅ | **PASS** |
| B0 | boundary | 单侧要计数；`ytd == mom` 合法 | `TestCaliberRoutingAcceptsEqualValues` 存在且绿 | **PASS** |
| B1 | boundary | 沿用既有辅助；`renderLoadReport` 三参数 | 复用 `mkMerged`；签名正确 | **PASS** |
| B2 | boundary | **订正**：换精确恒等式、取消异号跳过 | `…ChecksOppositeSignPairs` + `…AcceptsSignCrossingCumulative` 均在且绿 | **PASS** |
| B3 | boundary | **订正**：中位数取 `>=` | `TestCaliberFamilyMediansAcceptEqualMedians` 在且绿 | **PASS** |
| E0 | error_handling | 单测 + 全绿；discovery 显式写下**两个**消费判据 | 665 PASS / 0 FAIL；`interfaces_exposed` 写了两条（含「只看 V==0 不够」）✅ | **PASS** |
| N0 | non_functional | 门禁 + 覆盖率 ≥96.1% | §5：**96.3%**、vet 零、gofmt 仅两个既有欠账 | **PASS** |
| N1 | non_functional | 交付流程 | 无越界；commit 锚定正确 | **PASS** |

---

## 2. 🔴 Leader 那个数字疑问的独立复算（结论：**两个数都对，口径不同**）

dev-m1c4-b 在 `interfaces_exposed` 里标了「一处数字请 Leader 核」，并给出了它自己的推理
（「若观测集不同则单侧数不可能同为 1674」）。我**两个口径各跑一次**：

| 口径 | 可判 | 单侧 | 无上期 | 两侧皆空 | 合计 |
|---|---|---|---|---|---|
| **真 load**（合并观测 **97**） | 13 | 1674 | 20 | **427** | **2134 = 22 × 97** |
| **逐篇 Parse**（**213** 观测） | 13 | 1674 | 20 | **2979** | **4686 = 22 × 213** |

⇒ **前三类逐字相同，只有第四类变**。

**这不只是算术自洽，它直接证明了那条因果**：四类是一个**划分**（互斥且完备，由
`TestCaliberRoutingCountsArePartition` 与我实测的闭合共同保证）。前三类不变 ∧ 总数差 2552
∧ 第四类差 2552 ⇒ **被合并掉的 116 个观测所贡献的全部 `116 × 22 = 2552` 个 (观测, 对) 组合
必然全部落在 Absent 类**。这正是 Leader 给的成因（分篇 `tsf-stock@v1` / `rule-monthly@v1`
不含社融增量字段），且不需要额外假设。

差值验算：`2979 − 427 = 2552 = (213 − 97) × 22` ✅

⇒ **CONTRACTS 取 427（真 load 口径）正确**；dev-m1c4-b 的 2979 也不是错，是逐篇 Parse 口径。
建议 CONTRACTS 记数字时**连口径一起记**，否则下一个人拿到 2979 会以为 427 是笔误——
这恰是本次疑问的成因。

---

## 3. 容差改造：7 条假阳的复现（我在两棵树上各跑一次）

```
改前 = 328aad01e577446f6dd7d5b059949b8c0f1c66cc （= 28ea907 的父）
改后 = 07d111309737de1aaa08bbe05dd22b31a1e07ffb （判定对象）
```

| | 违反 | 可判 | 单侧 | 无上期 | 两侧皆空 | 逐对恒 0 |
|---|---|---|---|---|---|---|
| **改前** | **7** | 13 | 1674 | 20 | 2979 | 13/22 |
| **改后** | **0** ✅ | 13 | 1674 | 20 | 2979 | 13/22 |

⇒ **违反归零，其余四类计数逐字不变** —— dev 报的属实。

那 7 条与 discovery 逐条比对，**数值全部吻合**（我打印了 `YTD/MoM/PrevYTD/Expected` 原始字段）：

| 期次 | 字段 | ytd | expected | \|diff\| | rel |
|---|---|---|---|---|---|
| 2020-02 | `tsf_flow_rmb_loan_ytd` | 42100 | 42102 | 2 | 0.00475% |
| 2020-02 | `tsf_flow_corp_bond_ytd` | 7747 | 7725 | 22 | 0.28479% |
| 2020-02 | `tsf_flow_ytd` | 59200 | 59254 | 54 | 0.09113% |
| 2023-11 | `tsf_flow_ytd` | 336500 | 336400 | 100 | 0.02973% |
| 2023-08 | `tsf_flow_ytd` | 252100 | 252000 | 100 | 0.03968% |
| 2022-08 | `tsf_flow_ytd` | 241700 | 242000 | 300 | 0.12397% |
| 2022-11 | `tsf_flow_ytd` | 304900 | 306900 | **2000** | **0.65168%** |

我自己复算了 rel 列（`|diff| / |expected|`），七个值逐个吻合。
**噪声上限 0.65168% < K_rel = 2%** ⇒ 取值依据成立；
而真路由错实测 `|7202 − 42102| = 34900` vs 门限 `0.02 × 42102 = 842` ⇒ 约 **41 倍**超出（单条口径）。

### 真 load 的报告输出

```
口径路由核对（判据 ytd_n == ytd_n-1 + mom_n，容差 max(5 亿元, 0.02×|期望值|)；预期违反 0，共 0 违反）
  可判 13 对 / 单侧跳过 1674 / 无上期 20 / 两侧皆空 427（合计 2134 = 22 对 × 97 观测）
  （无违反）
  逐对可判观测数（0 表示这一对从未被比较过）: …
族内量级核对（按 period_type，预期违反 0，共 0 违反；样本不足未判 84）
  annual tsf_flow_rmb_loan: 样本不足未判（ytd n=7, mom n=0，门槛 3）
```

⇒ DoD 要求的三项都在：**四个数 + 自洽校验**（`合计 = 对数 × 观测数`）、**逐对清单**、
**样本不足要报出来说明**（不静默跳过）。

---

## 4. 变异矩阵（我自己跑，隔离 worktree）

主工作区 `backfill_load_report.go` sha256 前后一致（`2879009e…`），代码目录改动 **0 行**。
每格 `gofmt -e` 语法闸 + 变异 diff 逐字核对 + `build failed|setup failed` 有效性闸。
**对照组 665 PASS / 0 FAIL。**

| 变异 | `…LimitScalesWithMagnitude` | 其余测试 | 真语料违反 |
|---|---|---|---|
| **M1** `K_rel = 0`（退回纯绝对容差） | **FAIL** ✅ | 全绿（664 PASS） | **7**（假阳全回来） |
| **M2** `K_abs = 0`（无地板） | **FAIL** ✅ | 全绿 | — |
| **M4** `K_rel = 0.001`（不是 0，而是**不够大**） | **FAIL** ✅ | 全绿 | **3**（部分回来） |
| **M5** `K_rel = 0.9`（**过大**，会放过真路由错） | **FAIL** ✅（断言③） | 全绿 | 0，但放过路由错 |

### 🔴 Leader 的担心不成立，且这条守卫比 dev 声称的更宽

Leader 的判据是「若它杀不动，退回纯绝对容差会让 7 条假阳原样回来**且没有测试会红**」。
**实测：它杀得动**（M1）。而且我加测的三个方向也都被它抓住——读它的五条断言可知原因：

```go
① assert.Greater(caliberIdentityLimit(306900), 2000.0)   // 量化锚定真语料最大噪声 → M1/M4 靠它
② assert.Equal(caliberIdentityTolerance, caliberIdentityLimit(-108))  // K_abs 地板 → M2 靠它
③ assert.Greater(|7202-42102|, caliberIdentityLimit(42102))          // 容差上界 → M5 靠它
④ 单调不减    ⑤ 对称
```

⇒ dev 声称它守的是「随量级放大」（即 ④），但**它实际同时守着噪声下界、K_abs 地板与容差上界**。
`dev` 的描述偏保守，不是夸大。

⚠️ 一处**射程边界**（不是缺陷，供后续改动参考）：断言② 用的是**常量本身**作期望值
（`assert.Equal(caliberIdentityTolerance, …)`），所以**改 `K_abs` 的具体值不会红**——
它守的是「小量级时回落到 K_abs」这个关系，不是 5 这个数。

---

## 5. 报告抬头与闸门的一致性（Leader 要核的第三处）

**已修好，而且修法比「改对一次」更强**——抬头印的是**规则**且由**同一对常量**填充：

```go
// backfill_load_report.go:192
fmt.Fprintf(&b, "…容差 max(%g 亿元, %g×|期望值|)…", caliberIdentityTolerance, caliberIdentityRelTolerance, …)
```

⇒ 真 load 输出 `容差 max(5 亿元, 0.02×|期望值|)`，与 `caliberIdentityLimit` **同源**，
改常量时抬头自动跟着变 ⇒ **结构上不可能分叉**。（原先印的是固定 `容差 ±5 亿元`，
而 `expected=30万亿` 的记录实际门限是 6138。）

### 🔴 但 Leader 说的「这类不一致没有测试守着」——我实证了，成立

**M6b**：把抬头两个实参从常量脱钩成字面量 `5.0, 0.02`，**同时**把 `K_abs` 真值改成 `7.0`
（⇒ 报告说门限是 5，实际算的是 7）：

```
顶层 FAIL = 0    顶层 PASS = 665    ⇒ 全绿
```

⇒ **描述与计算脱钩且说谎，没有任何测试会红。** 当前的正确性来自**写法**（同一对常量喂两处），
不来自守卫。若后人改回字面量，这层保护会静默消失。
（⚠️ 我第一次的 M6 把格式串里的 `%g` 删了而留着实参，被 `go vet` 的 printf 检查拦下——
那格作废，M6b 才是干净的验证。）

---

## 6. 其余核实

- **13 对恒 0**：实测 **13/22**，与 dev 报的一致。按 Leader 说明这是**已知开口**
  （`sectorCaliberOf` 每段一个口径 ⇒ 分部门段两侧永不共存），**不据此判红**。
  `interfaces_exposed` 里已把「两条消费判据，缺一不可」与这 13 对写给 TASK-012 ✅。
- **覆盖率 96.3%**（比门槛高 0.2）。dev-m1c4-c 追查跌破根因时补的**两条渲染测试都在**：
  `TestLoadReportRendersRoutingViolation`（`:666`）与 `TestLoadReportRendersFamilyMedianViolation`（`:691`）✅
  ——它们覆盖的正是「违反行的渲染路径」。
- **三处 DoD 订正对应的测试全部存在且通过**：`…CountsArePartition`、`…AcceptsSignCrossingCumulative`、
  `…AcceptEqualMedians`、`…ChecksOppositeSignPairs`、`…AcceptsEqualValues`、`…SkipCountIsNotSilent`。
- 门禁（测自 `07d1113`）：`go test` 退出码 0、`vet` 零输出、`gofmt` 仅两个既有欠账、
  `go.mod`/`go.sum` 零改动。

---

## 7. 验证过程中我自己的两次失误（如实记录）

1. **探针 struct 字段名写错**（`MergedObservation` 无 `Meta`/`Values`，实为 `Obs`）⇒ `build failed`。
   有效性闸捕获，读实际结构后重做。
2. **M6 删掉格式串的 `%g` 却留着实参** ⇒ `go vet` 的 printf 检查报错、编译失败。本格作废，改做 M6b。

两次都被有效性闸拦住。**若没有那道闸，第一次会被记成「探针跑出 0 违反」，第二次会被记成
「抬头改动导致测试红 ⇒ 有守卫」——两个结论都是假的，且方向相反。**

---

## 8. 结论

**11 条 done_criteria 全部 PASS**。核心证据均由验证者独立产生：

- **数字疑问已解**：真 load 得 `13/1674/20/427`（2134=22×97），逐篇 Parse 得 `13/1674/20/2979`
  （4686=22×213），**前三类逐字相同**——这直接证明了「合并掉的 116 个观测全落在 Absent」，
  不是靠算术凑合。CONTRACTS 取 427 正确。
- **7 条假阳**在改前树上逐条复现（数值与 rel 全部吻合），改后归零而四类计数逐字不变。
- **守卫有效且射程比声称的宽**：四个方向（K_rel 过小 / 不够大 / K_abs 无地板 / K_rel 过大）
  全部被 `TestCaliberIdentityLimitScalesWithMagnitude` 抓住。Leader 担心的「没有测试会红」不成立。
- **抬头一致性**已由「同一对常量喂两处」在结构上消除分叉；但 M6b 实证「描述说谎」确实无测试守卫。

**判定：VERIFIED**
