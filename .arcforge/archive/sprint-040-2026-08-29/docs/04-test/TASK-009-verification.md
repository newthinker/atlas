# TASK-009 验证报告 — 分部门口径守卫：拒绝把当月值装进 `*_ytd`

- **验证者**：test-m1c3a-v2
- **判定**：**VERIFIED**（8/8 条 done_criteria 通过）
- **判定对象**：`verify_baseline.head` = `80976e417392c725de3558948eb115d393d5462a`
  （dev 分支交付 commit `81409c98fa47851185f602378edfe685e08ea7be`）
- **discovery sha256** `cbda3bb5…` 与 `verify_baseline.discovery_sha256` **逐字相同**
  ⇒ dev 在 `verifying` 窗口内未改 discovery（它来信说明了为什么不改，判断正确）。
- **HEAD 漂移**：收尾时 master 已到 `e21be041104300ee984ff3767944e8e36a238bdd`。
  我实测 `git diff --stat 80976e4..e21be04`（全量 + 限定本任务四文件）**两次输出都为空**
  ⇒ 区间内容层面零变化，判定对象未变。当前树复测覆盖率 `95.8449%` 与判定对象上**逐字相同**。
- **交付物指纹**：隔离副本四文件 sha256 与主工作区逐字相同；两份新快照 **CRLF 行数均为 0**。

---

## 0. 我自己重采的数字

| 指标 | 我实测 @ `80976e4`（判定对象） | dev 报（@ 其分支 `81409c9`） |
|---|---|---|
| `go test -count=1` | rc=0，**0 FAIL** | ok，0 FAIL |
| 覆盖率 | **95.8449%（1730/1805）** | 95.8449%（1730/1805） |
| RUN 计数（缩进口径） | **1127 = 531 顶层 + 584 一级子 + 12 二级子** | 1122 = 530 + 580 + 12 |
| `gofmt -l` / `go vet` | 空 / 空 | 空 / 空 |
| numstat | +1349/−0（4 文件，纯新增） | 同 |

### 两个数字的差异各有确定成因，我查实了，**不是 dev 报错**

dev 在 discovery 的 `coverage_anchor_warning` 里**预警**「你在 merge 后的 master 上量会得到与 95.8449% 不同的值」。
⚠️ **那个预警没有成真——两个值逐字相同**，而 RUN 计数反而不同。两者都有机制解释：

`81409c9..80976e4` 之间合入了 TASK-005-fix（`983da72` + merge `b87de0c`）：

| | 改了什么 | 后果 |
|---|---|---|
| **覆盖率相同** | `amount.go` 的 15/5 **全部是注释**（我逐行过滤非注释改动，得**零行**）；`profiles.go` 只改了**一行已有语句**（正则加 ` ?`） | 包内语句总数不变（1805），覆盖状态不变（1730）⇒ 分子分母都不动 |
| **RUN +5** | `amount_test.go` / `profiles_test.go` 新增测试 | 测试文件不进 coverprofile ⇒ 只影响 RUN 数 |

⇒ **dev 保守预警是对的做法**（它替我省掉的不是查证时间，是一次「dev 报错了」的错误起点），
只是这次实际差异落在了另一个指标上。

---

## 1. done_criteria 覆盖矩阵（8 条）

| # | 完成标准 | 对应测试 | 我实际跑的证据 | 判定 |
|---|---|---|---|---|
| functional[0] | 取分部门段前最近的期次前缀查 `cumulativePeriods`，不在表里⇒报错；**两侧都守** | `TestSectorCaliberGuardsBothSides` | 我的独立探针逐样本打印「最近前缀 / 在表内 / 本守卫结论」，两侧同一切换点；**新变异 N-d**（两侧锚点对调）⇒ KILLED，精确红在 `…/2023-08/存款侧`（`:1270`）⇒ 两侧**各自**守自己的锚点 | PASS |
| functional[1] | 判据是**结构性**的，不是数值启发式、不是期次黑名单 | `TestSectorCaliberIsStructuralNotNumeric` | **新变异 N-a**（查表换成形状匹配 `HasSuffix("月份")`）⇒ KILLED，**只红 `/1月报当月即累计_通过`（`:1326`）** ⇒ 形状匹配会被抓到，且 1 月报确实不需特例分支 | PASS |
| functional[2] | 既有 22 篇季报/年报抽取结果**一字不变** | `TestSectorCaliberKeepsCumulativeSamplesIntact` | 探针实打三篇回归底线：年报 `全年`→54 字段、半年报 `上半年`→27、季报 `前三季度`→27，全部放行；**新变异 N-c**（`cumulativePeriods` 删掉「全年」）⇒ KILLED（红 69 个，外溢大但方向明确） | PASS |
| boundary[0] | A 类通过、B 类被拒，**两侧都要有用例** | `TestSectorCaliberGuardsBothSides` | 探针：A（2025-08，`前八个月`）⇒ 产出 25 字段；B（2023-08，`8月份`）⇒ **拒绝且 n=0**，两侧都由**本守卫**拒 | PASS |
| boundary[1] | 🔴 **消融后必须看到那个具体错值 2320** | `TestSectorCaliberGuardsBothSides` + 消融 | **我亲自拿掉两侧守卫后直接打印字段值**（见 §2），看到 `loan_hh_short_ytd = 2320` 与 `loan_flow_ytd = 174400` 并存、`deposit_household_ytd = 7877` | PASS |
| boundary[2] | C / C' / E 类同样被拒且不产出数据 | `TestSectorCaliberRejectsNonCumulativeSamples`（拆两半）+ `…StaysSilentWhenNoSectorSegment` | 探针确认 C（2020-04）与 C'（2022-08）端到端 `n=0` 被拒，**但拒它们的是既有 `mustMatch` 不是本守卫**——而本守卫直调同样判它们非累计（`4月份`/`8月份`）⇒ dev「拆两半」的设计**必要且正确**；E 类由 **R-D4** 复跑确认（见 §4） | PASS |
| error_handling[0] | 错误信息含**实际读到的前缀**；与另两类措辞**可区分** | `TestSectorCaliberErrorIsDistinguishable` | **新变异 N-b**（把「非累计口径」措辞换成 `mustMatch` 那类 `not found among`）⇒ KILLED，红中含 `…ErrorIsDistinguishable/口径不对` | PASS |
| non_functional[0] | 门禁全绿、覆盖率 ≥95.5%；无新增依赖；milestone 前缀；🟡 契约升级 | — | 95.8449% ≥ 95.5%；`go.mod`/`go.sum` 未出现；改动 4 文件与 `writes` `diff` 逐条一致；新增行的 5 处 `TASK-009` 全带 `M1c-3a` 前缀；两份快照的 manifest 原名已记进 discovery；🟡 见 §3 | PASS |

---

## 2. 🔴 boundary[1] 的核心：**我亲眼看到了那个错值**

DoD 明说「判据是**看到 2320 这个值**，不是『测试红了』」。我在隔离副本上拿掉两侧守卫，
写自己的探针**直接打印字段**（不经任何断言）：

```
PROBE pboc-2023-08-monthly.html detectExtractor="rule-monthly@v1" err=<nil>
PROBE   extractFields err=<nil> n=25
PROBE   loan_flow_ytd            = 174400              ← 累计（前八个月）
PROBE   loan_hh_short_ytd        = 2320                ← **当月**（8 月单月短期）
PROBE   deposit_flow_ytd         = 202399.99999999997  ← 累计
PROBE   deposit_household_ytd    = 7877                ← **当月**
```

⇒ **口径混装现象逐字复现**，四个值与 DoD、与 dev 报的一致，本任务的前提成立。

### ⚠️ 一处必须精确记下的差异：`deposit_flow_ytd` 实测是 `202399.99999999997`

DoD 与 dev 的 discovery 都写 **`202400`**。那是四舍五入的写法，**实际是浮点尾差**。
记在这里是因为：**后人若照 DoD 机械比对 `== 202400` 会失败**，然后可能把一个正确的结果
判成「对不上」。另三个值（174400 / 2320 / 7877）是精确的。

**对照组**（同一次消融，A 类 2025-08）：`loan_flow_ytd=134600`、`loan_hh_short_ytd=-3725`
——它是 A 类，加守卫后照常产出 25 字段（负值是住户短期贷款累计减少，正常）。

---

## 3. 🟡 契约升级（我在 TASK-006 报的 N2 缺口）：**独立复跑 dev 的四个变异，闭合成立**

我在 TASK-006 提过一个警告：交叉断言最容易出的假绿是「只补一组、另一组恒绿」。
dev 据此补跑了四个变异（一个断言半边一个）并来信报告。⚠️ **该结论对它有利，我一律自己跑。**

我的独立复跑（隔离副本，harness 自写）：

| 变异 | 锚定单跑 FAIL | 锚定单跑 PASS | 失败行 |
|---|---|---|---|
| **M-a** `default` 丢掉 `unknown extractor` | B 组 2 格 | **A 组 3 格** | `:1500`（`Containsf`） |
| **M-b** `default` **混入** A 的措辞 | B 组 2 格 | **A 组 3 格** | **`:1501`（`NotContainsf`）** |
| **M-c** `wrongpath` 丢掉自己的措辞 | A 组 3 格 | **B 组 2 格** | `:1500`（`Containsf`） |
| **M-d** `wrongpath` **混入** `unknown extractor` | A 组 3 格 | **B 组 2 格** | **`:1501`（`NotContainsf`）** |

⇒ 四个各自**只**杀预期那一组（另一组仍 PASS ⇒ 定点，非连锁），
且 **M-b / M-d 只红在 `NotContainsf`、没碰 `Containsf`**
⇒ **两组的 `NotContains` 半边各有独家杀手，都不是恒真。我警告的假绿不成立。**

**为什么我的警告在这里不成立**（值得记下的机制）：我担心的是「只改 A 分支时，B 组的
『不含 A 的措辞』天然成立」。dev 改的是**两条分支各自的文案**，而且 M-b/M-d 用的是
**往对方分支里塞入自己的标志串**——只有这个方向逼得动 `NotContains`。⇒ 检验交叉断言的
`NotContains` 半边，变异必须是**注入**而不是**删除**。

⚠️ 同时确认它认下的那一半：它原先只跑一个变异（N2，杀的全是 A 组），**B 组一次都没验**
就写进 discovery 说闭合了。预警促成了补验，只是结论方向相反。

---

## 4. 消融证据汇总（9 个，harness 独立实现）

**方法**：`git archive 80976e417392c725de3558948eb115d393d5462a | tar -x` 到 `mktemp -d`
（判定对象锚，**钉全 sha**，可覆写 `ARCFORGE_MUT_REF`）。harness 我独立实现，
**未复用** dev 的 `crossassert-TASK-009-m1c3a-b.py`。每个变异：逐字替换（锚点次数必须为 1）
→ 打印 diff 逐字核对 → `go build` 语法闸 → 全套 `-v` → 关键项锚定单跑并**分列 FAIL / PASS + 核 RUN 行数**。

| ID | 变异 | 结果 | 关键证据 |
|---|---|---|---|
| 消融-D1 同形 | 拿掉两侧守卫 | — | **直接观察到 2320 / 7877**（见 §2） |
| M-a / M-b / M-c / M-d | 交叉断言四个半边 | 全 KILLED | 见 §3 |
| **R-D4** | 锚点不存在改成**报错** | KILLED | 红 `…StaysSilentWhenNoSectorSegment` 两个子测试（`:1545` `:1551`）⇒ **dev 自己发现缺口后补的那条测试确实在守** |
| **N-a** | 查表→形状匹配 `HasSuffix("月份")` | KILLED | **只红 `/1月报当月即累计_通过`（`:1326`）** |
| **N-b** | 「非累计口径」措辞换成 `not found among` | KILLED | 红中含 `…ErrorIsDistinguishable/口径不对` |
| **N-c** | `cumulativePeriods` 删掉「全年」 | KILLED | 红 69 个 ⇒ 回归底线不可能被静默破坏；⚠️ **外溢大，因果不精确**，只能读作「方向明确」 |
| **N-d** | 两侧锚点常量**对调** | KILLED | **精确红在 `…GuardsBothSides/2023-08/存款侧`（`:1270`）** |

**N-d 的价值**：dev 的 D5/D6 是「只守一侧」，我这个是「**守错侧**」——两侧都仍在调用守卫、
调用次数不变，只是各自用了对方的锚点。它精确红在存款侧，证明两侧的锚点**各自被钉住**，
而不只是「有两次调用」。

**卫生**：每个变异窗口内 + 收尾各校验主工作区 3 文件 sha256 与 `git status --porcelain`，
**全程未变**。副本已 `rm -rf` 拆除。

---

## 5. 独立探针：各类快照的实际行为（不看 dev 的断言）

```
A   2025-08  前缀=前八个月  在表=true   ⇒ 产出 25 字段（两侧放行）
B   2023-08  前缀=8月份     在表=false  ⇒ 拒绝(★本守卫:非累计口径) n=0（两侧）
C   2020-04  前缀=4月份     在表=false  ⇒ 端到端拒(既有 mustMatch) n=0；本守卫直调**也拒**
C'  2022-08  前缀=8月份     在表=false  ⇒ 端到端拒(既有 mustMatch) n=0；本守卫直调**也拒**
年报 2025-12  前缀=全年      在表=true   ⇒ 产出 54 字段
半年报 2020-06 前缀=上半年    在表=true   ⇒ 产出 27 字段
季报 2020-q1q3 前缀=前三季度   在表=true   ⇒ 产出 27 字段
```

🔴 **C / C' 两行独立印证了 dev 的一个关键设计判断**：它们端到端被拒，但**拒它们的是既有的
`mustMatch`，不是本守卫**——所以「端到端报错了」**证明不了**本守卫覆盖它们（把本守卫整个
删掉照样红）。dev 因此把 `TestSectorCaliberRejectsNonCumulativeSamples` 拆两半
（① 端到端仍被拒 ② `checkSectorCaliber` 直调也判它非累计）。**这个拆分是必要的**，
我的探针从另一侧确认了同一件事。

---

## 6. dev 自述的核实

| 自述 | 我的核实 | 结论 |
|---|---|---|
| 混装现象四个值（174400/2320/202400/7877） | 我自己消融后直接打印 | **属实**（`deposit_flow_ytd` 精确值是 `202399.99999999997`） |
| D4 第一轮 SURVIVED，补测试后 KILLED | 我复跑 R-D4：KILLED，红在它补的那条测试 | 属实 |
| 交叉断言四个变异各自定点、`NotContains` 有独家杀手 | 我独立复跑四个 | **属实**（见 §3） |
| C/C' 端到端被拒是「因为别的理由」，故拆两半 | 我的探针独立确认拒它们的是 `mustMatch` | 属实且设计必要 |
| 两份新快照已转 LF | 我在副本上数：CRLF 行数均为 **0** | 属实 |
| 覆盖率锚点预警 | 预警**未成真**（两值相同），但差异出现在 RUN 计数上，成因我已查实 | 保守预警，做法正确 |
| 结构判据在 9 份快照上成立 | 我的探针在 7 份上独立复核（手上没有 2026-07 与 1 月报快照） | 属实（我覆盖 7/9） |

---

## 7. 结论

**VERIFIED。** 8 条 done_criteria 全部有对应测试、断言非空洞，且经消融证明在守。
boundary[1] 这条最关键的验收我**没有依赖任何断言**——直接拿掉守卫打印字段值，
亲眼看到 `2320` 与 `174400` 并存。

我在 TASK-006 提的交叉断言假绿警告，经四个变异独立复跑**不成立**；
dev 自己发现并补上的 D4 缺口，经复跑确认已闭合。

一处留给下游：`deposit_flow_ytd` 的精确值是 `202399.99999999997`，
DoD 与 discovery 里的 `202400` 是四舍五入写法，机械比对会失败。
