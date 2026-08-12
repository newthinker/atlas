# TASK-007 验证报告 — LoadConfig 与 Sprint 036 契约

- **验证者**：test-agent-25（Reality Checker，默认判定 NEEDS WORK）
- **判定对象**：`verify_baseline.head = c101d6125d76ce1a8863342072a703c4c206d002`（== 当前 HEAD）
- **承接时 DoD 条数**：`functional 2 / boundary 1 / error_handling 1 / non_functional 4` = **8 条**
- **结论：VERIFIED（8/8 DoD 全部 PASS）**

## 0. 漂移、范围与承接

**双零漂移**：HEAD 与 `discovery_sha256`（`720ed5c0…`）均与基线逐字相同 ⇒ **未使用任何 `--ack-*`**。
`git show --numstat c101d61` → `CONTRACTS.md 67/0`、`config.go 78/0`、`config_test.go 177/0`、
`store.go 0/1`、`store_test.go 25/5`、`thresholds.go 21/15`，与 `writes` **六项逐项一致，无越界**。

⚠️ **本任务是 idle hook 唤醒 + 重扫发现的**（派验通知未送达）。本 Sprint 第三次靠这条自愈路径兜住。

**关于 DoD 未被扩到六项**：dev-agent-50 把 milestone 前缀约定写进了 CONTRACTS（它自己提的那条），
把「口径解释须能预测数值差」推给下个 Sprint，**没有扩 DoD**。这是 Leader 明确授权的选择。
**我按承接那一刻的四项验，不因缺 ⑥ 而 reject** —— DoD 是唯一依据，拿 DoD 外的标准判不合规。

## 1. DoD 逐条覆盖矩阵

| # | DoD 条目 | 对应测试 | 承重证据 | 判定 |
|---|---|---|---|---|
| F1 | YAML 没写的阈值须保持 `DefaultThresholds()`，**不能是零值** | `TestLoadConfigKeepsDefaultsForOmittedThresholds` | **M1**（去掉预填）、**M14**（去掉 tag）KILLED | **PASS** |
| F2 | 完整配置装载；豁免连 `period_types` 读出；`30s`→`30*time.Second` | `TestLoadConfigFull` | M1 KILLED | **PASS** |
| B1 | **显式写 0 也要被拒**；`MagnitudeRanges` 不检查且例外须写进注释 | `TestLoadConfigRejects/显式把容差写成 0` | **M2** KILLED；例外注释在场（§3.2） | **PASS** |
| E1 | 五种非法配置各含键名 + **文件不存在**返 error | `TestLoadConfigRejects`（10 子测试）+ `TestLoadConfigMissingFile` | M2–M10、M12 KILLED | **PASS** |
| N1 | 守卫登记 `LoadConfig`（追加不放宽）+ F8 清理 + `HasPeriod` 文档注释 | 见 §3.3 / §3.4 | **M13** KILLED + 两条自证 | **PASS** |
| N2 | `CONTRACTS.md` 追加 Sprint 036 一节，至少含四项 + 归属声明 | 见 §4 | — | **PASS** |
| N3 | RED / gofmt / vet / build / 整包绿 / **`-race` 绿** / 覆盖率 ≥92.1% | §2 | — | **PASS** |
| N4 | 消除 `thresholds.go:137-139` 的跨 Sprint 编号歧义（`verify_by: review`） | 见 §5 | — | **PASS** |

## 2. N3 的命令与输出

```
$ GOTOOLCHAIN=local go vet ./internal/hestia/   → 无输出，exit 0
$ gofmt -l internal/hestia/                     → 无输出，exit 0
$ GOTOOLCHAIN=local go build ./...              → 无输出，exit 0
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover
ok  github.com/newthinker/atlas/internal/hestia  0.782s  coverage: 93.2% of statements
$ GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -race
ok  github.com/newthinker/atlas/internal/hestia  3.610s        ← N3 明确要求的 -race
$ go tool cover -func | grep config.go
config.go:29: LoadConfig 100.0%   config.go:47: validate 100.0%
```
覆盖率 **93.2% ≥ 92.1%**；`LoadConfig` 与 `validate` **各 100%**
（dev 在 discovery 里记 `validate` 原本只有 66.7%，补齐四条阈值用例后到 100 —— 与我实测一致）。

**RED 独立复现**（删掉 `config.go`，测试保持交付版）：
```
internal/hestia/config_test.go:37:14: undefined: LoadConfig   （及 4 处同形）
```
因**预期原因**失败，未被 import 污染。

## 3. 变异/消融独立复验（harness 自写）

`scratchpad/test25-TASK-007-ablation.sh`，锚点 `ARCFORGE_MUT_REF` 可覆写、默认**全 sha**；隔离 worktree。
四道闸：基线闸（`--- PASS` = 577 全绿）、生效闸、**编译失败闸**、**计数自证 14 == 14 → OK**。
**结果 14 条全部 KILLED，0 SURVIVED。**

| 变异 | 唯一/主要杀手 |
|---|---|
| **M1** 去掉 `Config{Thresholds: DefaultThresholds()}` 预填 | `KeepsDefaultsForOmittedThresholds` + `Full` + `Rejects` |
| **M2** 去掉容差的「显式写 0」防线 | `Rejects/显式把容差写成 0` + `/容差写成负数` |
| M3–M6 分别去掉 drift_max / corp_loan / stock_continuity / yoy_sanity 那四条 | **各自被对应的子测试单独杀死** |
| **M7** 不再 `return t.validate()`（豁免校验断链） | **只被** `Rejects/豁免缺 period_types` 杀 |
| M8 去掉 `index_url` 必填 | `Rejects/缺 index_url` |
| M9 `max_pages` 下界放宽（`<1`→`<0`） | `Rejects/max_pages 为 0` |
| M10 `timeout` 下界放宽 | `Rejects/timeout 为 0` |
| **M11** `Unmarshal` 失败被吞 | **只被** `RejectsMalformedYAML` 杀 |
| **M12** `ReadInConfig` 的 `%w`→`%v` | **只被** `MissingFile` 杀（`assert.ErrorIs(err, fs.ErrNotExist)`） |
| **M13** 守卫期望列表删掉 `LoadConfig`（M9 同形） | `TestPackageExposesNoWriteFunctions` |
| M14 去掉 `yoy_sanity_max` 的 mapstructure tag | `KeepsDefaultsForOmittedThresholds` + `Rejects/yoy_sanity_max 为 0` |

### 3.1 dev **超出 DoD** 补齐了四条无人守的检查

DoD `boundary[0]` 只点名 `deposit_sum_tolerance`。dev 在 discovery 里指出另外四条阈值检查
**「写了却没人测」**（删掉任一条都不会有东西变红，实测 `validate` 覆盖率只有 66.7%），
自行补了四条用例 + 一条负数用例。

**我实测确认了这个判断**：M3/M4/M5/M6 **各自只被对应的那一条新用例杀死** ——
没有那四条用例，这四道检查就是四段无人守的代码。它把「第二道防线有五条」从
**看起来有五条**变成了**五条都被守着**。这一条是 dev 主动扩出来的，不在 DoD 要求内。

### 3.2 B1 的例外注释在场

`config.go` 的 `validate()` 里明写：
> `MagnitudeRanges` **不在此检查** —— 它有意为空（区间要等 M1c 用回填分布标定，
> 表为空时 magnitude_sanity 记 skipped{not_calibrated}）。把「非空」当成合法性要求会让默认配置自己就装载不了。

DoD 要求的「这个例外要写进注释」满足，且给了理由而非仅陈述。

### 3.3 N1 之一：守卫登记（M9 同形，按 DoD 明写的判据）

**M13 KILLED** ⇒ 守卫确实在按精确集合相等工作、`LoadConfig` 确实被它盯着。
逐字核对「追加而非放宽 / 追加而非覆盖」：
```
assert.Equal / 十二项 / "DefaultThresholds","Discover","LoadConfig","NewPBOCFetcher",…,"Validate"
Subset / Contains(t, got, …) 出现次数: 0
```
在 T1/T4/T5 的登记基础上**追加**（十一→十二），字典序正确（`Di` < `Lo` < `Ne`）。
`Config` 是结构体类型，**reflect 版全程绿**（两条守卫实跑均 PASS）。
**未跑 M11 同形**（`Equal`→`Subset`）—— 按 DoD 明写：它一定不红，跑它证明不了本次登记。

### 3.4 N1 之二/之三：两条自证，按**新判据**验

**① `HasPeriod` 文档注释**（`store.go` 的 `0/1` 正是第 236 行那个空行）：
```
$ go doc ./internal/hestia Store.HasPeriod
func (s *Store) HasPeriod(ctx context.Context, period, periodType string) (bool, error)
    HasPeriod 回答某期是否已在权威表里。Discover 用它决定翻页何时停。
    查 v_hestia_current 而**不**查 hestia_pending：pending 里的期次不算已入库。…
```
✅ 注释正文出来了（对照：我验 TASK-004 时同一命令只输出签名）。

**② F8 清理** —— DoD 的判据是**看哪条断言红，不是看红不红**（我在 T7 派单前证否了原判据：
该测试有 7 条断言，任何丢包裹的消融都会让兄弟断言先红，修与不修**四格全红**）：
```
全文已无【活的】NotErrorIs 断言（仅作为反面教材保留在注释里）
消融 %w→%v 后 TestPrecedingWrapsQueryError 的失败输出：
    Error: Target error should be in err chain:      ← 兄弟断言（修不修都有）
    Error: Expected value not to be nil.             ← ✅ 只有改写后的断言才产生这句
```
✅ 按新判据通过。dev 还把那段「为什么是这个写法」的说明**保留在注释里**，理由是
「F8 那种平凡为真不会在代码里留下痕迹，删掉这段下一个人很可能照直觉又写回 NotErrorIs」——
这个判断我认同：**被修掉的缺陷不留痕迹，正是它会复发的原因。**

## 4. N2：CONTRACTS 四项 + 归属声明

`## Sprint 036 · M1b-4a discover + fetch + config` 一节（3208 字符），小标题：
```
### 三处对方案报告 4.1 的修正（都由实测推翻，2026-08-12）     ← DoD ①
### discover 的三条判据                                      ← DoD ②
### 「pending 不可见」在两处的含义相反 —— 同一张表的两面        ← DoD ③
### 留给 M1b-4b 的一个张力（**必须在 4b 设计时明确回答**）      ← DoD ④
### 编码约定：注释里引用任务编号一律带 milestone 前缀           ← 超出 DoD，dev 自提
```
**四项齐全，另多一项。**

DoD 尾句「引用他人实测数据时注明哪些是你复核过的、哪些是转述」**已满足**——节首有：
> **取证归属**：本节标 ✅**亲验**的是我（TASK-002/003/005/007 的 dev）在本 Sprint 自己跑出来的；
> 标 📋**转述**的来自他人（独立 reviewer、TASK-001/004/006 的 dev、test-agent-25），我读过其
> discovery 但**没有重跑**。区分它们不是客套：转述的结论若有误，追查要从原始出处开始，
> 而不是从这份文件开始。

**我的结论在其中被标为「转述」，这是正确的**——它确实没重跑我的消融。

## 5. N4：跨 Sprint 编号歧义已消除（`verify_by: review`）

原文：`// …M1b-3 的 T1 写这个结构时 gates 表尚不存在，故留到 T7。`
改后（`thresholds.go:138-145`）：
```go
// 故留到 **M1b-3 的 TASK-007**（已兑现，就是紧接着这段的 checkEnum）。
//
// ⚠️ 编号带 milestone 前缀不是啰嗦：任务 ID 每个 Sprint 从 001 重开，
// 光写「T7」会让下一个 Sprint 的 TASK-007 以为这是自己的待办，
// 进而去动一段已经正确、且有 TestExemptionRejectsUnknownCheckID 守着的代码。
```
✅ 歧义消除，且**写出了危害机制**（不只是补个前缀），并与 CONTRACTS 那条通用约定呼应。

## 6. ⚠️ 我在验证期间更正了 DoD 里一处**我自己传播的错数字**

`non_functional[0]` 的 `-run` 锚定那一段里写着「实际会跑起 **7 个**顶层测试」——
**那是我在 T5 报告里的口径错误，被 Leader 原样写进了这条 DoD。** 真值是
**7 条 `=== RUN` 行 = 6 个顶层 + 1 个子测试**（锚定后为 2 条 = 1 顶层 + 1 子测试）。

我是 `verifying` 状态的 task 文件 owner，经写通道
`update --expect-epoch 1 --json-field done_criteria=…` 更正，四组条数 `2/1/1/4` 未变，
`jq` 直读核实旧句已消失。**更正只涉及事实数字，DoD 的要求本身一字未动**
（两条建议：锚定 `^Top$/^Sub$`、只读 `--- PASS:` 那行不读退出码，都原样保留）。

记此项是因为：这个错数字**不影响任何人的行动，也不会有人来查** ——
按项目已有判据（`discipline-beats-judgment`），那正是最不能省的一类。

## 6.5 Leader 点名要我独立判的两件

### ① dev 的「加守卫会超范围」理由 —— **不成立**，而且不成立的原因是它自己的探针误报

dev 说：`go doc` 修好了但**修复本身没有守卫**，加守卫要两步都超 DoD
（给 `Close` 补注释 + 新增 AST 测试，而**整包范围的守卫会碰不在 `writes` 里的 `validate.go`**）。

**「给 `Close` 补注释」属实**：`store.go` 里恰好只有 `Close`（`:190`）缺文档注释，其余
`NewStore`/`DB`/`HasPeriod`/`Preceding`/`Save` 全部相连。

**但「会碰 `validate.go`」不属实。** 我写了一个与 `store_test.go` 正式守卫**同口径**的 AST 探针
（关键是照 `ast.IsExported(recv)` 跳过接收者未导出的方法），对整包扫描：

```
正确口径（跳过未导出接收者）：
  store.go     缺文档注释的导出物: [Close]
  （validate.go 未被点名）

错误口径（不看接收者导出性 —— 即 dev 那个一次性探针的口径）：
  store.go     ['Close']
  validate.go  ['Preceding']      ← 假阳：那是 func (noHistory) Preceding(...)，接收者未导出
```

⇒ **即使做成整包范围的守卫，需要动的也只有 `store.go`（补 `Close` 的注释）与 `store_test.go`
（新增测试）—— 两者都在 T7 的 `writes` 里。** 更不用说还可以做成只扫 `store.go` 的窄版本。

**这不改变判定**（DoD 没有要求加守卫，不加不违反任何一条），但**它给出的理由是错的**，
而错的来源正是 Leader 已经点出的那次探针误报：
**假阳性不只产出了一个错数字，它产出了一个错决定。**

⇒ 建议 Leader 把这条带进下个 Sprint 时，一并纠正理由——否则下一个人会照着「会碰 validate.go」
这个不存在的障碍继续绕。

### ② `numbers_corrected_after_commit` 字段 —— **在场且与实测一致**

```
$ jq '.verification.numbers_corrected_after_commit' discoveries/TASK-007.json
"⚠️ 首版 discovery 把 config_test.go 记成 143 行 —— 那是补 %w 包裹断言与四条阈值用例之前的行数，
  属于「自证数字采样早于最后一次改动」。提交后以 git show --numstat 复核时发现并订正为 177。
  记在这里而不是悄悄改掉：这正是本项目反复强调的那条纪律，我自己也踩了一次。"

$ git show --numstat c101d61 | grep config_test
177  0  internal/hestia/config_test.go        ← 与订正值逐字一致
```

**它没有悄悄改掉，而是把违反记进了产物。** 这一点值得记频次：

| 谁 | 什么 | 怎么被发现的 |
|---|---|---|
| Leader | F8 自证判据四格全红（修不修都「通过」） | 验证者预演时证否 |
| 验证者（我） | 「7 条 RUN」写成「7 个顶层」 | Leader 核对时两数相等，重跑确认 |
| dev-agent-50 | discovery 记 143 行（采样早于最后一次改动） | 自己用 `git show --numstat` 复核 |

**本 Sprint 三个人各踩一次同族错误，没有一次是被机制挡下的，全靠事后复核。**
三次的共同形状：**自证数字/自证判据本身不受任何自动门禁检查** ——
`gofmt`/`vet`/`go test`/validator 都不看它们。

## 7. 结论

**VERIFIED。** 8 条 done_criteria 逐条有对应测试或产物、逐条有证据；14 条变异**全部 KILLED**，
计数自证 14==14；两条自证按**新判据**（看哪条断言红 / `go doc` 输出正文）通过；
CONTRACTS 四项齐全并多一项、归属声明在场；跨 Sprint 编号歧义已消除且写出了危害机制；
dev 另超出 DoD 补齐了四条无人守的阈值检查（我实测确认各自只被对应新用例杀死）；
RED 独立复现；双零漂移；主工作区零污染。

## 8. 主工作区完整性

变异窗口内 + 收尾双重核实，五文件 sha256 与 `git status --porcelain` 前后逐字相同：
`config.go b10ddf36…`、`config_test.go 85ab3308…`、`thresholds.go dd2db9fd…`、
`store_test.go 13ef1af7…`、`CONTRACTS.md ee323dab…`。
变异树收尾 sha256 一致；`/tmp/mut-036-7`、`/tmp/verify-036-7`、`/tmp/verify-036-7b`、
`/tmp/prep-036-7` 均已 remove + prune。
