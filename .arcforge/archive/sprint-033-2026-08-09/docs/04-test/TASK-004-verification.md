# TASK-004 验证报告 —— `NewStore` 与建表

> **本文件覆盖两轮验证，第 2 轮（C1 返工轮）为当前有效判定。** 覆盖同名文件而非另起新文件：
> CLAUDE.md 规定报告文件名以 `TASK-{id}-verification.md` 为准，且实测过重复文件无法删除的代价。
> 第 1 轮结论完整保留在 §R1。

---

# 第 2 轮（C1 返工轮）—— 当前有效判定

- **验证者**：test-agent-20 ｜ **判定：VERIFIED（通过）**
- **verify_baseline**：`head` = `823ca15fbea53c626e986419d8f91ad200f03859`、
  `discovery_sha256` = `eb831275…8b89`，出裁决前实算**均一致，无漂移**
- **epoch**：1 ｜ **rework_count**：1 ｜ **reason_class**：`task_defect`
- **验证环境**：隔离 worktree `.worktrees/wt-rw-46`（`--detach 823ca15…`），验毕已 remove

## 1. ⚠ 判定对象的 commit 归属（重要，会影响任何人复现）

**C1 的产出不在任何 commit message 提到 C1 的提交里。** 按符号名检索：

```
git log -S 'func verifyCurrentView' -- internal/hestia/store.go
  → 572f2ce  fix(hestia): G10 的检查状态改用白名单…   ← C1 在这里
```

`572f2ce` 的 message 讲的是 T5 的 C2（G10 白名单化）——dev-agent-41 提交 T5 时用整文件
`git add`，把 dev-42 的 C1 一并带走了。成因是三个 review_fix 声明了**完全相同**的 `writes`
而两个 dev 并行开工（Leader 已认领为调度错误，两份 discovery 均记录）。

**我的测量口径**：在 `823ca15`（= `verify_baseline.head`，含 C1+C2+C3+C4 全部改动）
的隔离检出上测。这与 dev 报的 61/75 → 62/76 **不是同一棵树**——它那组数字取自
`wt-c1`（基于干净 `ed776a0` 只重放它自己的三处改动），用于证明「本次改动净增 1 条测试」。
两组数字各自有效，**不可互相印证，也不可混用**。Leader 提到的共享树 64/84 是污染值，我未使用。

## 2. 基线（@ `823ca15`，隔离检出，工作区 0 处未提交改动）

| 项 | 实测 |
|---|---|
| `--- PASS` 含子测试 | **86** |
| 顶层 PASS | **66** |
| FAIL / SKIP | **0 / 0** |
| `go test -cover` ／ `-func`（门禁口径） | **89.3%** ／ **89.0%** |
| `go vet` ／ `gofmt -l` | exit 0 ／ 空 |

## 3. C1 修复的判据核验

### 3.1 变异（四条自证：diff 非空 + **两文件首行完整** + `go vet` exit 0 + 计数对基线 86）

| 变异 | 内容 | 结果 | 红的测试 |
|---|---|---|---|
| N1 | 删除 `NewStore` 里的 `verifyCurrentView` 调用 | KILLED 85/86 | **仅** `TestNewStoreRejectsDriftedCurrentView` |
| N3 | 比对放宽成「视图存在即通过」 | KILLED 85/86 | **仅** 同上 |
| **N2** | `verifyCurrentView` 恒返回 nil | **KILLED 85/86** | **仅** 同上 —— **见 §3.2，与 dev 的记录不同** |

### 3.2 一处对 dev 记录的**更正**（方向对交付有利）

dev 报「N2 两次编译不过，判无效变异、不计入分母」。**我这里 N2 编译通过且被杀。**

差别在变异写法：我把 `QueryRow` 块与比对尾部**一并删掉**，函数体只剩 `return nil`，
于是 `db`、`spec` 成为**未使用的参数**——Go 允许未使用参数（只禁止未使用的局部变量与 import）。
dev 那版大概保留了 `QueryRow` 块，留下已赋值未使用的 `deployed` 局部变量，才编译失败。

⇒ **「N2 无效」是变异写法的产物，不是该变异形态本身不可测。** 这不改变判定
（N2 无论哪种写法都不构成存活），但**判据实际被守住的程度比 dev 报告所称的更强**：
「恒返回 nil」这一形态确实被 `TestNewStoreRejectsDriftedCurrentView` 拦下。

### 3.3 fixture 区分力 —— **复核成立，且抓住它的确实是前提自证**

dev 自述初版 fixture 给两个 `period_type` 用了**同一个** `published_at`，导致坏视图照样
返回两行、**危害根本没复现**。我把 fixture 退回那个弱形态实测：

```
--- FAIL: TestNewStoreRejectsDriftedCurrentView
    store_test.go:236  expected: 1  actual: 2
    Messages: 前提自证：漏 period_type 的视图会让同一 period 的两个 period_type 互相吞掉
```

**红在前提自证那一行**（不是最终的 `require.Error`）。⇒ 现在的 fixture
（monthly@2026-07-15 + h1@2026-08-20，并断言幸存者是 h1）**确有区分力**，
而守住这一点的正是那两条 `require`。若无它们，弱 fixture 会让整条测试变成
「视图定义不一样就报错」，而 DoD 要求的是**先坐实危害**。

## 4. 两处设计判断 —— **都接受**

### ① 视图用全等比对（与表的「只拦缺列」相反）—— 接受

理由成立：表放行多列是因为 INSERT 显式列名、多列无害，且一并失败会把回滚变成硬故障；
**视图没有「多／少」的中间态，定义不符即读语义不符**，两个方向都会让本版代码按错误的
「当前行」语义读数据。这个不对称是真实的，不是图省事。

**但它有一个必须点名的代价，我实测了**：全等比对下，`currentViewDDL` 输出的**纯格式改动**
（我的探针只加了一个空格，SQL 语义完全相同）会让既有库 `NewStore` **直接失败**。

而 `currentViewDDL` 的主体来自 **`bitemporal.CurrentQuery(spec)`——另一个包**。
⇒ **`internal/macro/bitemporal` 一次纯排版重构，会让全部既有 hestia 库打不开。**

这**不构成缺陷**：按 dev 自己的判准（「视图对不上说明这个库出自另一个代码版本」），
这正是它要的信号；本包已把「不支持自动迁移」列为显式非目标；错误信息也明确告知
「drop the view (it holds no data)」，恢复成本约等于零。**但这是一处跨包耦合**，
`bitemporal` 的维护者不会知道自己在改 hestia 的开库条件。已在 §6 提请 Leader 登记。

### ② 选失败而非 DROP + 重建 —— 接受，且理由比「省事」深

重建视图近乎免费（不存数据），但会**丢掉信号**：列检查对「多出来的列」是放行的，
所以对一个来自另一版本的库，**视图不符是唯一还会响的警报**。静默修好视图 =
把最后一个警报也关掉。这个论证成立。

### ③ QA 要求的注释同步 —— 已做，且做得比要求好

QA 指出「改完后 store.go:89-90 那段只说表的注释会成为误导」。现在那段改成了范围声明，
明确列出三个对象各自的归属（观测表 → 本函数；视图 → `verifyCurrentView`；
pending → 刻意不做，论证见 `savePending`），并写了一句
「不要以为这里查了就等于全查了（QA 的 C1 正是这么漏的）」。**误导已消除。**

## 5. 结论

**VERIFIED。** C1 已闭合：`verifyCurrentView` 在 `NewStore` 中被调用，三个变异形态
（删调用／恒返回 nil／放宽比对）**各自只红新增的那一条测试**；fixture 的区分力经
退化实验复核成立，且由前提自证守住。两处设计判断接受，理由均成立。

**本轮无阻断问题。** 一处跨包耦合见 §6。

## 6. 提请 Leader 登记（不阻断）

**`bitemporal.CurrentQuery` 的输出格式现在是 hestia 的开库前置条件。**
该函数在 `internal/macro/bitemporal`（本 sprint 范围之外），它的任何输出变化——
**包括纯排版**——都会让既有 hestia 库 `NewStore` 失败。恢复方式简单且错误信息已写明，
但**改动方无从得知这层依赖**。建议在 `bitemporal.CurrentQuery` 处加一行注释指回
`hestia.verifyCurrentView`，或在 T7 交接清单里登记这条跨包契约。

---

# §R1 第 1 轮（历史记录，判定已被第 2 轮取代）

- 被验 commit `5e4d217873b84ae7c146a2f8b676bb718209c7d8`，判定 **VERIFIED**
- 计数：43 `--- PASS` 行 / 37 顶层 / 0 FAIL / 0 SKIP，覆盖率 86.8%（`-cover`）／85.9%（`-func`）
- 7 条 done_criteria 全 PASS；12 个变异窗口，6 条判据有「只红该条」的精确变异
- **头号复核项**：弱/强 fixture 对照（C0 控制组 = 弱 fixture 单独跑 43/43 全绿 ⇒ 弱 fixture
  合法但不具区分力；A = 强 fixture × M2′ KILLED；B = 弱 fixture × M2′ SURVIVED）
- 另证：M8 两条红均有因果；两个初版变异确因孤立 import 编译不过，排除正确；
  视图断言的完整子句是承重的（裸词 `"period"` 被 `period_type` 蕴含，元变异 SURVIVED）
- **发现 P1（低-中）**：`TestStoreExposesNoWriteMethods` 只反射 `*Store` 方法，
  包级导出写函数（M10）存活 → **已在 T5（`39aa8af`）闭环**，新增
  `TestPackageExposesNoWriteFunctions`（AST 扫导出 `FuncDecl`），与 reflect 那条**并存互补**
- **发现 P2（提示）**：`boundary[1]` 选项②的文本在 `Store` 类型注释而非包注释；因选①故不构成未满足
