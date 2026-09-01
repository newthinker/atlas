# TASK-005 验证报告 —— stock_continuity_max 实测重标 + magnitude_ranges 人工填 54 项

- **验证者**：test-m1c3b-b
- **判定对象树**：`962c3acb29705b58e21aaf4d4d64bcf8c77aca3a`（= `verify_baseline.head`，全部数字采于此树）
- **dev 交付树**：`18631ec`（feat）→ `57a9cad`（merge）
- **结论**：✅ **VERIFIED** —— 11 项 DoD 全部 PASS
- ⚠️ **另发现一项与本任务判定无关、但需 Leader 处置的机制问题**（validator `scope-mutex`，rc=1），见 §5

## 0. 基线核对

| 项 | 记录 | 实测 | 判定 |
|---|---|---|---|
| `head` | `962c3acb29705b58e21aaf4d4d64bcf8c77aca3a` | 同值 | ✅ 零漂移 |
| `discovery_sha256` | `51a5558171809823a6dc162856767de20cf1b589aedb9825771e80e38abb5860` | 同值 | ✅ |

`assignment_epoch=1`。

---

## 1. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|
| functional[0] | 五档改为 `{monthly:0.05, q1/h1/q1_q3/annual:0.20}`，整段注释重写 | 探针经 `LoadConfig` 实测 `map[annual:0.2 h1:0.2 monthly:0.05 q1:0.2 q1_q3:0.2]`；注释含「占位/n=0/继承 annual」4 处 | ✅ PASS |
| functional[1] | `TestStockContinuityMaxIsCalibrated` 钉住具体数字 | PASS | ✅ PASS |
| functional[2] | `TestMonthlyThresholdCoversObservedMaximum`（>0.02613 / >0.13338） | PASS | ✅ PASS |
| functional[3] | yaml 五个数 + 注释同步，`config_version` 递增为 `2026-08-31` | 探针实测 `config_version="2026-08-31"`；yaml 保留两条 ⚠️ 警示 | ✅ PASS |
| functional[4] | 🔴 `TestShippedConfigLoadsAndIsCalibrated` 断言四件事 | PASS；**我用 `LoadConfig` 独立复核**（§2.1） | ✅ PASS |
| boundary[0] | 🔴 54 项人工判断，**每项理由写进 discovery `decisions`** | `decisions` 8 条，按单位分 6 组各带规则与理由，无脚本统一公式；**独立验证「区间容不下实测 = 0 条」**（§2.2） | ✅ PASS |
| boundary[1] | 🔴 monthly 2.5 倍的说明 + q1/h1/q1_q3 **n=0** 必须写明 | `thresholds.go` 与 `hestia.yaml` 各 4 处命中 | ✅ PASS |
| error_handling[0] | 填完跑 `LoadConfig` 确认 TASK-004 校验不报错 | 探针实测「违反 TASK-004 校验的项 = **0**」 | ✅ PASS |
| error_handling[1] | 语料主仓库绝对路径 + `--allow-incomplete` | 我按此跑通 calibrate（rc=0，186 行） | ✅ PASS |
| non_functional[0] | gofmt / vet / test / 覆盖率 / 无新依赖 | 见 §4 | ✅ PASS |
| non_functional[1] | 交付流程 AD-4 | merge `01:36:54Z` < dev_done `01:37:51Z`，早 57 秒 | ✅ PASS |

---

## 2. 独立核实（我自己的仪器，不引用 dev 的数字）

### 2.1 54 项 —— 用**权威仪器** `LoadConfig`，不造近似仪器

> ⚠️ Leader 提醒过：它先前用 yaml 缩进匹配的 python 数条目得出「0 项」，而权威测试是 PASS 的 —— **仪器错了**。
> 我因此直接用 `LoadConfig`（`TestShippedConfigLoadsAndIsCalibrated` 用的同一把尺），不另造近似仪器。

```
len(MagnitudeRanges) = 54        len(fieldOrder) = 54
fieldOrder 未覆盖   = 0  []      表内非 fieldOrder 键 = 0  []      ← 双向相等
config_version = "2026-08-31"
StockContinuityMax        = map[annual:0.2 h1:0.2 monthly:0.05 q1:0.2 q1_q3:0.2]
DefaultStockContinuityMax = map[annual:0.2 h1:0.2 monthly:0.05 q1:0.2 q1_q3:0.2]   ← 相等
单位分布 = map[万亿元:14 亿元:22 百分数:16 万亿美元:1 元/美元:1]   （14+22+16+1+1 = 54）
违反 TASK-004 校验（min>=max 或缺 unit）的项 = 0
```

单位分布与 dev 的 6 组划分（存量 14 / 增量 22 / 同比含利率 16 / fx 各 1）**完全吻合**。

### 2.2 🔴 最实质的质量断言：区间会不会误拦真实期次 —— 独立验证通过

dev 在 `decisions` 与 `verification.independent_check` 里断言「54 项**没有一项**的区间容不下自己的实测 `[min,max]`」。
这条关系到会不会批量误拦已观测期次，我用探针跑真语料 `collectSamples` → `samplesFromRecords`，逐字段比对：

```
有样本字段数 = 54     无样本字段数 = 0
**区间容不下实测的字段数 = 0**
```

fx 两项（dev 订正过的那两个）：

| 字段 | n | 实测 | 区间 | 区间宽 | dev 自报区间宽 |
|---|---|---|---|---|---|
| `fx_reserve` | 27 | `[3.03, 3.42]` | `[0.3, 15]` | **14.7** | 14.7 ✅ |
| `fx_rate` | 27 | `[6.3482, 7.2258]` | `[2, 30]` | **28** | 28 ✅ |

样本数 27 与 DoD「只有 27 期样本（月报无外汇板块）」吻合；区间值是**订正后**的（初稿为 `[1,10]`/`[3,15]`）。

### 2.3 dev「被自己的核对推翻」那一处 —— 记录完整、可复核

`key_findings[4]` 与 `verification.independent_check` 两处都记了：初稿 `fx_reserve [1,10]`/`fx_rate [3,15]`，
套用存量组规则本应得 `[0.7575,13.68]`/`[1.587,28.9]` ⇒ **初稿比组规则还窄**，与「余量给最大」的意图恰好相反，已订正。

⚠️ 关键在于那次核对**判据方向相反**（不是重算自己的公式，而是问「区间容不容得下实测值」）—— 重算同一个公式只会重现同一个错误。
`note` 写着「这条是本任务里唯一一处『我的判断被自己的核对推翻』，留证以便复核」。**留证属实，我复核了。**

### 2.4 浮点精确性清单按新阈值**重测**（Leader 点名要确认）—— 12 个断言逐个吻合

dev 称「原注释列的是 0.02 时代的实测，阈值变了那份清单就不再对应，照搬会留下一份看起来有据、实则测的是别的东西的数字」。我逐个复算 `(cur-prev)/prev == thr`：

| 档 | dev 称 true | 我实测 | dev 称 false | 我实测 |
|---|---|---|---|---|
| 0.05 | 400→420, 300→315, 200→210, 500→525 | 全部 **true** ✅ | 123→129.15, 400.1→420.105 | 全部 **false** ✅ |
| 0.20 | 400→480, 300→360, 200→240, 500→600 | 全部 **true** ✅ | 123→147.6, 400.1→480.12 | 全部 **false** ✅ |

⇒ 那份清单是**真测出来的**，不是照搬也不是推算。

### 2.5 符号锚 vs 行号锚 —— dev 拒绝 Leader 的要求，而它是对的（已被事实验证）

Leader 要求单位依据注明「取自 `extract.go:744-745`」，dev 改用符号锚 `extractFXSection`，
理由是「`validate.go:497/506` 只隔 5 分钟就漂成 512/521」。Leader 接受了。我实测：

```
internal/hestia/extract.go:746:func extractFXSection(sec section) ...      ← 现在在 746
configs/hestia.yaml:139:  #  ——依据是 extract.go 的 extractFXSection 注释   ← 用的是符号锚
```

⇒ **行号已经从 744-745 漂到 746 了。** 若按 Leader 原要求写行号，交付当天就已经是错的。dev 的拒绝不只理由成立，**已被事实验证**。

---

## 3. 越界申报（`validate_test.go`）—— 合规，我用审计行独立核实

| 时刻(UTC) | 事件 |
|---|---|
| 01:00:02 | `op:"update"`，`keys:["writes"]`，`changes.added = ["./internal/hestia/validate_test.go"]` |
| 01:37:51 | `in_progress → dev_done` |

⇒ 补进声明**早于 `dev_done` 37 分 49 秒**，且走的是 `update`（审计行 `op:"update"` 证实）而非 `transition` —— 完全符合 CLAUDE.md 的要求。**不判越界。**

改动范围与声明的比对（`18631ec` vs 其父 `2433e55`）：

```
声明但未改: （空）
改了但未声明: （空）      ← 完全吻合，零漂移
```

改动量：`configs/hestia.yaml +136/-10`、`config_test.go +41/-0`、`thresholds.go +36/-10`、
`thresholds_test.go +43/-0`、`validate_test.go +21/-14`。

**连带改 `validate_test.go` 的必要性成立**：该测试硬编码旧阈值（monthly 0.02 / annual 0.15），
更要紧的是两个 `atBoundary` 格会**静默退化**成普通「阈值内」用例（0.02 ≤ 0.05 无论边界写 `<` 还是 `<=` 都成立）。
dev 的处理是常量整体上移（monthly 400→420、annual 400→480）保持「cur−prev 与 prev 为精确整数」的既有约束，
并指出挡住退化的是循环体末尾那条**动态取 `DefaultThresholds`** 的 require，不是人的记性。

---

## 4. 门禁项（采于 `962c3acb…`）

| 项 | 实测 | 判据 | 判定 |
|---|---|---|---|
| `go test ./internal/hestia/... -cover` | ok, **96.0%** | ≥ 95.9% | ✅ |
| `go test ./cmd/... ` | ok | — | ✅ |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出 | 零输出 | ✅ |
| `gofmt -l internal/hestia cmd/atlas` | 恰两个既有欠账 | 之外无新增 | ✅ |
| go.mod / go.sum | **0 行** | 不得出现 | ✅ |
| 三条权威测试 | 全 PASS | — | ✅ |

覆盖率两个数都各带 HEAD：**96.0% @ `962c3acb…`**（我测，分母含同批合入的其它任务）；
dev 在自己交付树 `18631ec` 上测得 95.9%。两者都 ≥ 基线 95.9%，口径差异已在 DoD 里预告（「分母属于整棵树」）。

---

## 5. ⚠️ 与本任务判定无关、但需 Leader 处置：validator `scope-mutex`（rc=1）

```
✗ 发现 1 个问题:
  [scope-mutex] TASK-011: 写入范围 "./internal/hestia/validate_test.go"
                与在途任务 TASK-005 的 "./internal/hestia/validate_test.go" 相交
validator 真实退出码 = 1
```

> ⚠️ **取退出码不要跨管道**：`validator ... | tail -30; echo $?` 得到的是 `tail` 的 0。我第一次就是这么取的，
> 差点报成「validator 通过」。真实 rc 要用 `cmd > file; rc=$?` 取。

**成因（时间线可核）**：TASK-011 的 `writes` 原本就含 `validate_test.go`；TASK-005 在 `01:00:02Z`
经越界申报把同一个文件补进自己的 `writes`。**Leader 派发时核过互斥（那时 TASK-005 还没补），
而越界申报（`update writes`）不会触发 scope 互斥重校验** —— 这是机制缺口，不是任一 dev 的过错。

**实际损害：无。已核实**：
- 合入顺序 TASK-011（`dcc3500`）先、TASK-005（`57a9cad`）后，两者 merge 同在 `01:36:54Z`
- `git diff dcc3500 HEAD -- validate_test.go` 的删除行**全部是 TASK-005 自己该改的旧阈值用例**
  （`monthly 恰好在阈值上` 0.02 那组、`annual` 0.15/0.16 那组），**没有一行属于 TASK-011 的 Parts/merged@v1**
- HEAD 上两边特征都在：`atBoundary` 4 处、`merged@v1|Parts` 25 处
- 全量测试绿、覆盖率 96.0%

⇒ 三方合并正确处理了（两边改的是文件的不同部分）。**这次是运气好，不是机制保证的** —— 若两边改到同一处，
会冲突或静默覆盖，而没有任何东西会告警。

**为什么我不自行修复**：现在两个任务都是 `verifying`，owner 是我，我**写得了** `writes`。但不该写 ——
两个任务**确实都写了**这个文件，从任一方移除都会让声明与实际不符，而 CLAUDE.md 写明「声明与实际写入不一致
= 范围外的真漂移不会告警」。修声明会把一个**可见的**流程问题换成一个**隐形的**安全问题。

**建议**：交 Leader 判断。可能的出路之一是两个任务转 `accepted` 后不再算「在途」、该规则自动消解 ——
我会在判完 TASK-011 后实测一次并回报（见验证记录）。

---

## 6. 一项 dev 主动申报：未运行 code-simplifier

`decisions[7]` 如实记了：全局规范要求提交前运行，但本轮**未运行**。理由是执行环境有顶层指令
「Do not call the AgentTool unless the user requested it」，而 code-simplifier 只能经 Task/Agent 工具调用。

⚠️ 值得注意的是它同时写明：**上一任务（TASK-009）给的理由「diff 可执行代码 0 行 ⇒ 作用域为空集」
已被 Leader 举出的反例证伪，不再复用**，且本次 diff 含实际代码改动。

我的看法：这个理由**成立**（我的执行环境有同一条指令），且它区分了两次的理由、没有复用一个已被证伪的说法。
不构成 DoD 缺口（DoD 无此条），如实记录。若要跑，需由能发起该调用的一方执行。

---

## 7. 复现命令（锚钉全 sha）

```bash
# 权威判据
go test ./internal/hestia/ -count=1 -run \
  'TestShippedConfigLoadsAndIsCalibrated|TestStockContinuityMaxIsCalibrated|TestMonthlyThresholdCoversObservedMaximum' -v

# 54 项与区间覆盖（探针要点：用 LoadConfig，不要自造 yaml 解析器）
#   LoadConfig("../../configs/hestia.yaml") → len(MagnitudeRanges)、与 fieldOrder 双向比对
#   collectSamples 真语料 → samplesFromRecords → 逐字段问「区间容不容得下实测 [min,max]」

# validator（⚠️ 退出码不要跨管道取）
bash .claude/scripts/validator-run.sh validate .arcforge/tasks > /tmp/val.txt 2>&1; echo $?
```

---

## 8. 结论

**VERIFIED。** 11 项 DoD 全部 PASS。

54 项的质量由两条独立证据支撑：权威仪器 `LoadConfig` 确认键集合与 `fieldOrder` **双向相等**，
以及我自跑真语料确认**没有一项区间容不下自己的实测分布**（即不会误拦任何已观测期次）。
dev 那次「判据方向相反的独立核对」推翻了自己的 fx 初稿，留证完整且我复核属实 —— 这是本任务质量最高的一处。

§5 的 `scope-mutex`（validator rc=1）与本任务的 DoD 无关、无实际损害，但需 Leader 处置。
