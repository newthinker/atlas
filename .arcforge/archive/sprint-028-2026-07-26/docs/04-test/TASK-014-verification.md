# TASK-014 验证报告 — 分部模板批量落地（两阶段合并）

- **验证者**: test-agent-7 (Reality Checker)
- **被验对象**: `4bc36be`（阶段一 fixture）+ `b3542af`（4 家模板）+ `5da74db`（NVDA 结构变更记录）
- **packages**: `configs/prism`、`./internal/collector/edgar`
- **assignment_epoch**: 1
- **判定**: ✅ **PASS (verified)**
- **纪律**: 结论只锚定我本人实跑的输出与文件内容；未采信 dev-agent-15 自述。
  探针在隔离 worktree 内进行，**全程未触网、未触碰任何 runtime 库**，验完 `git worktree remove`。

---

## 1. 阶段一：已提前验证，且此后零改动（结论可直接沿用）

我在 `4bc36be` 提交后做过一次非正式提前验证（结论已报 leader）。本次确认其**仍然有效**：

```
internal/collector/edgar/testdata/instance_mini.xml : 4bc36be..HEAD 改动 0 次
internal/collector/edgar/segments.go                : 0 次
internal/collector/edgar/segments_test.go           : 0 次
git diff --stat 4bc36be HEAD -- internal/collector/edgar/  → 空
AD-18 golden 9 份基线                                : 0 次
```

即 `b3542af`/`5da74db` **只动 `configs/prism/templates/*.yaml`**，未触碰阶段一产物。

阶段一结论（`-count=10`，基线先验全绿）：

| 变异 | 修复前 `ee65b69` | 修复后 `4bc36be` | 浮出的错误值 |
|---|---|---|---|
| A 交叉维 `!=1`→`<1` | 存活 0/10 | **被杀 10/10** | `1.2e+10` 交叉维值 |
| B `<divide>` 天真实现 | 存活 0/10 | **被杀 10/10** | `3.24` 每股值 |
| C tag 优先级（哨兵） | 被杀 10/10 | **被杀 10/10** 无回归 | — |

且均**因其本应保护的那条规则而红**（失败信息分别指向「交叉维 context 必须排除」与
「U_UnitedStatesOfAmericaDollarsShare 是 USD/share,不是营收单位」），非连带误伤。

**修复前两处的遮蔽因子不同**（此点我曾纠正 leader 表述，已被采纳）：
A 是**文档顺序**（同 tag 同优先级、合法 fact 在前）；
B 是**tag 优先级**（合法 fact 反而更靠后，靠优先级压制）。
故两处修法不同：A 前移、B 统一 tag；**B 的修复同时依赖「统一 tag」与「divide 在 USD 之前」，缺一不可**。

---

## 2. 阶段二：模板正确性

### 2.1 真实加载器实测（不是"YAML 看着合法"）

用真实 `sankey.LoadTemplates` 加载真实 `configs/prism/templates/`：

```
AAPL   cik=320193   axis=ProductOrServiceAxis        n=5
       members = [IPhoneMember MacMember IPadMember WearablesHomeandAccessoriesMember ServiceMember]
AMZN   cik=1018724  axis=StatementBusinessSegmentsAxis n=3
GOOGL  cik=1652044  axis=StatementBusinessSegmentsAxis n=3
MSFT   cik=789019   axis=StatementBusinessSegmentsAxis n=3
NVDA   cik=1045810  axis=ProductOrServiceAxis        n=5
AAPL wearables name_en="Wearables, Home and Accessories"   ← 含逗号的值完整保留
```
5 家全部通过加载校验；无空 member、无重复 member、segment key 全小写
（§3.1.1 viper 小写化契约）；模板注释里专门警告过的「含逗号 `name_en` 需加引号」确已生效。

### 2.2 汇总项排除：我独立复算，不采信

| 公司 | Σ 映射项 | 若并入汇总项 | 倍数 |
|---|---|---|---|
| AAPL | **416.161B**（5 项） | +`ProductMember` 307.003B → 723.164B | **1.74×** |
| NVDA | **215.938B**（5 项） | +Compute&Networking 193.737B → 409.675B | **1.90×** |
| MSFT | 281.724B（3 项） | — | — |
| GOOGL | 402.963B（3 项） | — | — |
| AMZN | 716.924B（3 项） | — | — |

子项校验：`iphone+mac+ipad+wearables = 307.003B` **恰等于** `ProductMember` —— 父子关系成立，排除父项正确。

**残留重叠排查**：`ProductMember` / `ComputeMember` / `NetworkingMember` /
`HyperscaleMember` / `AICloudsIndustrialEnterpriseMember` / `EdgeComputingMember`
**六个汇总项或细分项均未出现在任何模板的 `xbrl_member` 中** ✅

两家处置形式相反（AAPL 排父取子、NVDA 取父排子）但**规则一致：只映射互不重叠的一层**。

### 2.3 交叉轴独立验证（本次最强的一条证据）

discovery 记录了另一条完全独立的切分轴数据，我用它做交叉验证：

```
AAPL 地域轴 Σ = 178.353+111.032+64.377+33.696+28.703 = 416.161B
AAPL 产品轴 Σ =                                        416.161B   → 两轴相等 ✓
NVDA 分部轴 Σ = 193.479 + 22.459                     = 215.938B
NVDA 平台轴 Σ =                                        215.938B   → 两轴相等 ✓
```

**两套互相独立的切分方式合出同一个数**，这比单轴自洽强得多 ——
它同时证明两轴各自都是**完整且不重叠**的划分（任一轴若漏项或重复，两者不可能相等）。

### 2.4 与 leader 提供的 live 基准对账

MSFT FY2025 年报：`productivity 120.810 / intelligent_cloud 106.265 / personal_computing 54.649`
—— 与 leader 独立实拉的基准**逐项完全一致** ✅

---

## 3. 生产库安全（DoD 硬要求）

| 检查 | 结果 |
|---|---|
| 仓库内任何 `prism.db*` | `find` **零命中** —— 生产库根本不存在于运行路径 |
| 工作区是否新增 db/数据文件 | `git status` 无任何 `.db` / `data/` 条目 |
| dev 声明的临时 DBPath | discovery 记录为 scratchpad 下临时路径 |
| 我本人的验证 | 全程只读 + 隔离 worktree，**未运行任何 refresh、未触网** |

**结论：无生产数据风险。** 「生产库不存在于运行路径」这一点由我独立 `find` 确认，
不依赖 dev 的声明。

---

## 4. Done Criteria 逐条

| # | 完成标准（全部 manual/review） | 核实 | 判定 |
|---|---|---|---|
| **F0** | 除 MSFT 外 ≥4 家产出模板，member 映射经真实实例文档确认，清单记入 discovery | 4 家新增（AAPL/GOOGL/AMZN/NVDA），各模板注明 accession 与实测财年数字；discovery 含逐家 member 清单原文 | **PASS** |
| **F1** | MSFT 分部数字与 10-Q 对账 | FY2025 三个数与 leader live 基准逐项一致（§2.4）；FY26Q3 三个数即 TASK-005 fixture 中已逐值断言者 | **PASS** |
| **F2** | AAPL `segment_axis` 按实测填写 | `ProductOrServiceAxis`，且模板与 discovery 均记录了理由：可报告分部挂在 `ConsolidationItemsAxis × StatementBusinessSegmentsAxis` **双 member** context 上（实测 15 个），被「恰好一个 explicitMember」排除，默认轴对 AAPL **零产出** | **PASS** |
| **B0** | 持续失败公司写 manual 兜底；TSM 明确记录跳过并说明理由 | 5/5 全自动成功 → **manual 兜底分支未触发**（见 §6.1）；TSM 记录为「IFRS/20-F，不在本批」 | **PASS**（附注） |
| **E0** | refresh 全程无 panic；单家失败只进 Degraded/Failed 不阻塞其余 | 退出码 0、`5 ok, 0 failed, 46 degraded`；其中含**一次真实的** MSFT yahoo 价格源 EOF → engine 兜底成功且未阻塞其余 4 家 —— 这是**真实故障的实证**，非构造场景 | **PASS** |
| **N0** | discovery 记录 全自动/manual/跳过 家数 + 各家 member 清单 | **5 / 0 / 0** 明确记录；member 清单原文齐备 | **PASS** |

**6 条 done_criteria 全部满足。**

### 4.1 discovery 数字的内部一致性（我逐条复算）

```
落库 281 行 = AAPL 75 + NVDA 71 + MSFT/GOOGL/AMZN 各 45   → 281 ✓
  AAPL 75 = 15 期 × 5 分部 ✓        MSFT/GOOGL/AMZN 45 = 15 期 × 3 ✓
  NVDA 71 = 14 期 × 5 + 1 期 × 1 ✓  ← 与「FY2027Q1 只披露 data_center」的记录自洽
degraded 46 = 1(MSFT yahoo) + 15(AAPL ProductMember) + 30(NVDA) → 46 ✓
```

**NVDA 的 71 行是一条有价值的交叉印证**：结构变更记录**预测**了某一期只落 1 个分部，
而独立报出的行数恰好等于 `14×5+1`。叙述与数字互相印证，不是各说各话。

NVDA 结构变更期的父子关系我也复算了：
`2026-04-26: 37.869+37.377 = 75.246 = DataCenter` ✓；`2025-04-27: 21.513+17.599 = 39.112` ✓

---

## 5. 回归

| 项 | 结果 |
|---|---|
| `go test ./internal/collector/edgar/ ./internal/prism/... ./internal/storage/prism/ -count=1` | 全 `ok`（edgar 93.1%） |
| `go build ./...` | OK |
| AD-18 golden 9 份基线 | `4bc36be..HEAD` 零改动 |

---

## 6. 三点如实说明（均不影响判定）

### 6.1 manual 兜底分支**未被触发**，故该路径在本任务中无实证
DoD boundary[0] 前半句是条件式的（「自动解析**持续失败**的公司写 manual 兜底数据」）。
本批 5/5 全自动成功，`configs/prism/segments/` 目录不存在 —— 条件未触发，
按 DoD 字面属**空真满足**。但这意味着 **manual 兜底路径本身在本任务中没有得到任何验证**。
若后续有公司需要兜底，该路径是首次上生产。建议在有真实兜底案例时补验，或由 TASK-015 记为已知覆盖边界。

### 6.2 TSM 的记账口径
DoD 要求「TSM 明确记录为跳过并说明理由」。discovery 记录了理由（IFRS/20-F），
但**计入的是「跳过 0 家」**（因 TSM 从未纳入本批配置）。理由已记录、口径可解释，
故判 PASS；仅提示：「未纳入」与「跳过」在统计口径上是两件事，后续若做覆盖率统计需注意。

### 6.3 `fiscal_period` 标签冲突缺陷
按 leader 明确指示**不因此判 TASK-014 不合格** —— 它不在本任务 DoD 内，
且是 dev 主动发现并逐层验根因后上报的（已开 TASK-016/017 处置）。

我要补充一点**对本任务判定有利**的观察：dev 在做财年对账时
**刻意按 `period_end` 窗口聚合而非依赖 `fiscal_period` 标签**，
正是这个选择让本任务的对账数字不受该缺陷污染。这不是运气，是它识别出缺陷后主动规避的结果——
也说明**分部数据落库本身是正确的，错的是下游按标签聚合**，与 leader 的判断一致。

---

## 7. 一条建议（非缺口，供后续考虑）

NVDA 结构变更这类**外部世界变化**的失效模式是 leader 指出的「不报错，只让某个分部悄悄变空」。
本次的缓解是**模板注释 + degraded 文本**：未映射 member 确实会进 Degraded
（NVDA 30 条里就含 Hyperscale/AIClouds/EdgeComputing），所以**可观测性是有的**，
但需要有人去读 degraded 输出。

模板注释里那句警告写得很到位：
> 若将来要展示 Hyperscale/AIClouds 细分，**必须同时把 DataCenterMember 从本模板移除**，
> 否则又是一次「只映射互不重叠的一层」被破坏、合计翻倍且不报错。

建议后续考虑一条自动信号（如 `Σsegments / Revenue` 比值突变时告警）——
它能同时覆盖「结构变更导致分部变空」与「重叠映射导致合计翻倍」两个方向，
而这两个方向目前都只靠人读注释与 degraded 防守。**属设计范围，非本任务缺口。**

---

## 8. 判定

**verified（PASS）。**

6 条 done_criteria 全部满足；阶段一的变异体结论在 `4bc36be..HEAD` 零改动下继续有效；
5 家模板经**真实加载器**实测可用；汇总项排除经**独立复算**确认（1.74× / 1.90× 与父子关系分毫不差），
六个汇总/细分 member 确认零残留；**两条独立切分轴合出同一合并营收**，
交叉证明各轴均为完整不重叠划分；MSFT 数字与 leader live 基准逐项一致；
discovery 内部数字（281 行 / 46 degraded / NVDA 71）逐条复算自洽，
且 NVDA 行数与结构变更叙述互相印证；生产库安全经我独立 `find` 确认；回归全绿。
达到「压倒性证据」标准。

§6 三点为如实说明与口径提示，§7 为设计建议，均不构成退回理由。
