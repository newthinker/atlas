# 需求 <-> DoD 双向追溯矩阵 — Sprint 035

**需求真相源**：上游 spec `2026-08-11-hestia-validate-design.md` 第 10 节的 DoD 表（17 条）。
**不以上游计划的自审表为准**——那是计划**对自己**的追溯，本矩阵的价值正在于独立复核它。

---

## 方向一：spec 的每条 DoD 是否都有任务覆盖（查孤儿需求）

| spec DoD | 内容 | 覆盖任务 | 状态 |
|---|---|---|---|
| `functional[0]` | 三道有信号的闸门在两份 golden 上全 passed | TASK-004 f0 | 覆盖（且更强：断言**无任何**闸门 failed） |
| `functional[1]` | 每道闸门各有构造的失败样本 | TASK-004 f1 + TASK-005 f0 + TASK-006 f0/f1 | 覆盖 |
| `functional[2]` | 派生必填集 v2=54 / v1=27，与 golden 键集完全一致 | TASK-002 f1, f2 | 覆盖（且明确要求**双向**） |
| `functional[3]` | 无历史时 stock_continuity 与漂移检测 = `skipped{no_prior_period}` | TASK-006 b0 / TASK-005 f0 | **部分冲突，见发现 1** |
| `functional[4]` | 注入 fake 历史后两者真正生效（含超阈值判 failed） | TASK-005 f0 + TASK-006 f0 | 覆盖 |
| `functional[5]` | MagnitudeRanges 空 ⇒ skipped；填表则生效 | TASK-004 b1 | 覆盖 |
| `functional[6]` | Preceding：降序、限 n、同 period_type、只取当前行 | TASK-003 f0, f1, b1 | 覆盖 |
| `functional[7]` | deposit_sum 五行映射逐行验证 | TASK-005 f0（六行，多一行 insufficient_history） | 覆盖 |
| `boundary[0]` | v1 期次 stock_continuity = `absent_field`，不是 `no_prior_period` | TASK-006 b1 | 覆盖 |
| `boundary[1]` | 修订后 Preceding 取修订值 | TASK-003 f1 | 覆盖 |
| `boundary[2]` | **恰好在容差边界（±12.0%）的判定方向明确** | TASK-005 b0 | **补入，见发现 2** |
| `boundary[3]` | 命中豁免记 `skipped{caliber_exemption}`，不是 passed | TASK-007 f2 | 覆盖 |
| `error_handling[0]` | 豁免未登记版本 / 空 Reason / 未知闸门 ID 分别报错 | TASK-001 e0（前两项）+ TASK-007 e0（未知 ID） | 覆盖（拆两任务：ID 校验需 `gates` 存在，T1 时尚无） |
| `error_handling[1]` | Preceding 查库失败返回 error，而非记成闸门失败 | TASK-003 e0 + TASK-004 e0 | 覆盖（存储层与闸门层各一次） |
| `non_functional[0]` | 每个 skipped 带非空 Reason | TASK-004 n0 | 覆盖 |
| `non_functional[1]` | ULP 契约被钉住 | TASK-007 n1 | 覆盖 |
| `non_functional[2]` | Validate 产出能被 Save 接受 | TASK-007 n0 | 覆盖 |

**孤儿需求：0 条**（`boundary[2]` 修正后）。

---

## 方向二：任务 DoD 是否都对应某条需求（查凭空 DoD）

56 条任务 DoD 全部可回溯，来源分三类：

| 来源 | 条数 | 说明 |
|---|---|---|
| spec 第 10 节 DoD 表 | 17 条对应关系（见方向一） | 直接需求 |
| spec 正文的设计约束（D1-D4、7.1、7.2、8.1-8.3、9、口径豁免三条约束） | 约 20 条 | 如「completeness 必须走模板表不用前缀」（7.2）、「豁免不外溢」（4.6.3）、「零分母三处保护」（9） |
| **Arcforge 流程要求**（非 spec，Leader 追加） | 每任务 1-2 条 | RED 阶段真实性（AD-035-3）、`gofmt`/`vet`/整包绿、覆盖率不下降 |

**凭空 DoD：0 条**。第三类虽不在 spec 内，但来自 `arcforge.config.json`（`tdd.require_failing_test_first=true`、
`coverage.dev_minimum=80`）与本仓库 CLAUDE.md 的既有纪律，**不是我凭空发明的验收标准**。

---

## 发现 1：spec 自身矛盾（`functional[3]` vs `functional[7]` / 7.1）

- `functional[3]`：「无历史时 `stock_continuity` **与漂移检测** = `skipped{no_prior_period}`」
- `functional[7]` 与 7.1 映射表：「无历史时 deposit_sum = **`passed`** + `drift_skipped:no_prior_period`」

**同一件事，两个不相容的说法。** 矛盾只在「漂移检测」那半——对 `stock_continuity` 而言
`skipped{no_prior_period}` 两处一致，没有问题。

**裁定：以 7.1 为准**（`passed` + `drift_skipped:no_prior_period`）。理由：7.1 是详细规格，
且给出了论证——「绝对值**确实查了并通过了**，所以是 passed 而非 skipped；但漂移没查这件事不能丢，
记进 Reason」。`functional[3]` 是把两道闸门写在一行的粗略表述，漏掉了 deposit_sum 的双判据结构。

上游计划采纳的也是 7.1。**本裁定已写入 TASK-005 的 f0**，避免 dev 或验证者照 `functional[3]` 判红。

---

## 发现 2：上游计划遗漏了 spec `boundary[2]`，而其自审声称「无遗漏」

spec `boundary[2]` 写的是「恰好在容差边界（**±12.0%**）的判定方向明确」。
12.0% 是 `DepositSumTolerance`（7.1 映射表逐行写的就是 12%），**明确指向 `deposit_sum`**。

而上游计划的自审表把 `boundary[0..3]` 映射为「T6 Step 1、T3 Step 9、**T6 Step 6**、T7 Step 1」——
第三项 T6 Step 6 是 `stock_continuity` 的 400->408（**2%**）边界测试。**张冠李戴。**

复核计划 T5 的全部用例：残差取值为 10% / 20% / 5%，**没有任何一个落在 12% 上**。
⇒ `boundary[2]` 实际**未被覆盖**，而自审结论写着「无遗漏」。

**处置**：已在 TASK-005 补入 `boundary[0]`，要求构造残差恰好 = `DepositSumTolerance` 的观测，
断言其判定方向（走 passed，因实现用 `r > tolerance` 判失败），并补一个略超阈值的对照。
同时沿用计划自己在 T6 定下的选数纪律：**边界测试必须选精确可表示的比例**，否则测的是浮点舍入。

> 这条正是独立追溯矩阵存在的理由：计划的自审是**它对自己的复核**，
> 覆盖表填满了就会显得「无遗漏」，而填错一格与没填在观感上完全一样。

---

## 待独立 reviewer 复核

Leader 已 spawn 一个**只读上游计划与 spec、不看本矩阵与任务 DoD** 的独立 reviewer，
要求它独立列出验收标准并核查计划自身的漏洞（含 `FieldXxx` 常量逐个 grep 核对、
派生等式实测、计划代码与既有签名是否相符）。其结论回来后并入本文件。
