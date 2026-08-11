# TASK-005 返工验证报告 —— `mustMatch` 唯一性守卫（QA WARNING-1）

- **验证者**: test-agent-22 / `assignment_epoch` = 1 / `rework_count` = **1**
- **验证对象**: `internal/hestia/extract.go` + `extract_test.go`，返工提交 `655157b36ff537b32c5461d5ecbd0eb36542e81f`
- **`verify_baseline.head`**: `29adc9e65ba8c30008f20d59c088b51d43fdd0b8`（== 当前 HEAD）
- **判定**: **PASS → verified**

> 本报告只覆盖**本轮返工**。首轮验证见 `TASK-005-verification.md`（含 F1 定性更正附录），判定不变。

---

## 一、转前清单（四条，全过）

| # | 检查 | 结果 |
|---|---|---|
| 1 | `jq 'has("discovery")'` | **true** |
| 2 | discovery sha256 vs 基线 | `0ee62646…669097`，**逐字相同** |
| 3 | 改动范围 vs 声明 `writes` | `655157b` = **2 文件**（`extract.go` +28/−4、`extract_test.go` +144/−0），均在 `writes` 内，**无越界** |
| 4 | 转后 validator 完整输出 | 见第六节 |

**基线漂移**：`verify_baseline.head`（`29adc9e`）晚于返工提交（`655157b`）——因 T6 的返工先行合并（dev-45 把 T6 分支基于 T5 建，以便验证组合态）。按漂移判据（**声明范围内的文件变了**）核实：

```
git diff --stat 29adc9e..HEAD -- internal/hestia/extract.go internal/hestia/extract_test.go
  → （空）
```

⇒ **声明范围内无变更，判定对象未漂移**，转状态无需 `--ack-drift`。

---

## 二、基线（自证）

| 项 | 我的实测 | Leader / dev-45 自报 | 一致 |
|---|---|---|---|
| `go test ./internal/hestia/ -v -count=1` @ `29adc9e` | **372 PASS / 0 FAIL / exit 0** | 372 / 0 | ✓ |
| coverage @ `29adc9e` | **89.6%** | 89.6% | ✓ |
| `go vet` / `gofmt -l` | exit 0 / **0** | 同 | ✓ |
| **@ 返工提交 `655157b`** | **365 PASS / 0 FAIL**，coverage 89.6% | dev-45 自报 365 | ✓ |

> 我**单独建 worktree 复核了返工提交处的 365**，而不是只测最终树——因为那是 dev-45 交付报告里的自证数字，
> 而最终树含 T6 的返工（净增 7 条 → 372）。两个数字各自锚在自己的 commit 上，都对得上。

---

## 三、修复本身：`mustMatch` 由「最左优先」改为「必须恰好命中一次」

```go
all := re.FindAllStringSubmatch(body, -1)
switch len(all) {
case 1:  return all[0], nil
case 0:  return nil, fmt.Errorf("… not found …")
default: return nil, fmt.Errorf("… matched %d sentences … refusing to pick one …")
}
```

### 3.1 危害场景 A/B 对照（我独立构造，不采信 QA 的输出摘要）

取**真实 2020 存款板块正文**，仅把一句同形的单月句插到累计句**之前**（真实报告确有此体例）：

| | `err` | `deposit_household_ytd` | `deposit_nbfi_ytd` |
|---|---|---|---|
| **A 修复态** | **报错**：`存款分部门 住户存款 matched 2 sentences …` | — | — |
| **B 退回最左优先**（模拟修复前） | **`nil`** | **21000**（应 83300） | **+500**（应 **−7446**，**符号翻转**） |

**A 下 `Values` 为 `nil`**——是「拿不准就停」，不是「先抽取再报错」。

> **与 QA 报的数字对照**：QA 报 `household=21000`（应 146400）、`nbfi=+500`（应 −64100）。
> **注入值（21000 / +500）与我完全一致**；「正确值」不同是因为 QA 用 2025 样本、我用 2020 样本。
> **机制与危害完全复现。**

**另一个方向也验了**：把同形句追加在**末尾**时，最左优先取到的恰好是**正确值**——
这正证明了 QA 的论点：**最左优先的正确性是文本顺序的副产品，不是设计**。

### 3.2 变异

| # | 变异 | PASS（基线 372） | 判定 |
|---|---|---|---|
| R2 | `mustMatch` 退回最左优先（`all = all[:1]`） | 370 / FAIL 2 | **KILLED**（`TestMustMatchRequiresUniqueHit`） |
| R3 | 清单模板 `住户存款`→`存款`（人为造成多命中） | 357 / FAIL 15 | **KILLED**（含 `TestListTemplatesHitExactlyOnceOnRealSamples`） |

> **R1 作废**：我最初把 `case 1` 的 `return all[0]` 改成 `all[len(all)-1]` ——
> 该分支下 `all` 恰有一个元素，两者**恒等**，是等价变异体，SURVIVED 不说明任何事。
> 记录在此以免被读成「守卫有洞」。dev-45 的 M2（取最后一个）是在**同时移除 ≥2 报错分支**的版本上做的，两者不是同一个变异。

---

## 四、新增两条测试的质量

**`TestMustMatchRequiresUniqueHit`**：1 / 0 / 多 三种结果都断言。多命中那条同时钉住
「错误含命中数」「错误指名模板」**「不把候选值当结果带出来」**（`assert.NotContains(err, "2.1")`）——
最后一条与 `selectUnique` 的 label 纪律同源，是我在首轮报告里赞过的那个设计。

**`TestListTemplatesHitExactlyOnceOnRealSamples`**：常驻守卫，两份真实样本上每条清单模板命中数**恰为 1**。

- **枚举从清单表派生**（`tsfStockItems` / `tsfFlowItems` / `moneyItems` / `depositItems` / `loanScopes`），不另写模板列表；
- **非空转自证有两道**：`require.NotEmpty(t, secs, …)`，以及末尾
  `assert.Equalf(t, want, checked, "实际检查了 %d 条模板、清单表里有 %d 条——枚举与表脱节了")`。
  后者比 `NotZero` 强：**同时抓「枚举为空」与「枚举漏项」**。
- **判据方向与 T3 的 `TestSectionKeywordsHitAtMostOneTitle` 相反**（那边 `≤1`、这边**恰为 1**），
  注释写明了为什么——我核过：板块关键词在 2020 期次可以正确地为 0，而清单模板未命中本就该由 `mustMatch` 报错。**区分正确。**

---

## 五、dev-45 自报两处的核实，与一处我判为**不可独立核实**的

**① 「把自己基线跑坏过一次」——记录属实且处置正确。** 它在探针里的 `git checkout` 把复制进去的
修复版还原成未修复版，变异跑在错误载体上；靠 harness 打印的基线（363 PASS / exit=1）发现。
此后它把 harness 从**打印基线**改为**断言基线为绿**。
这与本 Sprint 已记的「自查跑错目录」「`tail -1` 跑 validator」同族：**读数出来了，但没有东西拦住你继续往下走。**

**② 两次 VET-KILL 被有效性闸挡下、未计为测试杀死** —— 处置正确。手抄锚点导致替换后留下悬空代码，
`go vet` 红。按纪律那不算测试杀死。**我本轮的 R1 作废是同一条纪律的另一种形态**（等价变异体）。

**③ 「code-simplifier 回复『不变』，`git diff` 逐行核实属实」—— 我判为不可事后独立核实。**

本轮 `extract.go`/`extract_test.go` 只有 `655157b` **一个提交**，**没有 simplifier 运行前的中间态**。
⇒ 事后无人能把 dev-45 写的行与 simplifier 改的行分开。它当时手持 diff 核实过，我采信它的**过程记录**，
但必须标明：**这一条我无法独立验证**。

> **建议（非阻塞）**：跑 simplifier 前先 commit（哪怕是 WIP），使「哪些行不是我写的」成为可查事实
> 而非自述。这正对着本 Sprint 已记的教训——该子代理累计 6 次中 3 次回复与实际不符，
> 且 `--stat` 粒度不足以发现藏在自己新增行里的改动。

---

## 六、discovery 的处置：**追加，未覆盖**

`key_findings` 11→13，新增 `verification.rework`（8 个键）。**首轮的 `verification.commit`
（`8e4811d`）、`tests`（315 PASS）、`coverage`（89.1%）逐字保留。**

⇒ 与我在**首轮** T5 验证里定下的规则一致：**判定依据只追加、不原地改写**。现在它被用在
dev-45 自己的 discovery 上，且用对了。

---

## 七、结论

QA 的 WARNING-1 已闭合：`mustMatch` 从「多命中静默取最左」改为「必须恰好一次」，
危害场景经我独立 A/B 复现（修复前 `err=nil` 且 `nbfi` 符号翻转，修复后报错且 `Values` 为 nil）。
两条新测试非空转、判据方向正确、枚举从表派生。范围无越界，判定对象未漂移，
自证数字（365 @ `655157b`、372 @ `29adc9e`）**双锚点分别复核，全部一致**。

一处不可独立核实（simplifier 归属）已如实标明，并给出使其可查的建议。

**判定：PASS → `verified`**
