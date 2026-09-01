# TASK-001 验证报告 —— 抽出 eachParsedArticle 共用管道（纯重构）

- **验证者**：test-m1c3b-b
- **判定对象树**：`2433e5577b38f1d0fc8ba77bff4bd2641dee7421`（= `verify_baseline.head`，全部实测数字采于此树）
- **对照基线树**：`db19e803a08d63cdd42af8a4bac4dfddc1ff3bf5`（本任务任何改动之前的 master）
- **验证时主仓库 HEAD**：`2433e5577b38f1d0fc8ba77bff4bd2641dee7421`（与判定对象同一 commit，零漂移）
- **结论**：✅ **VERIFIED** —— 10 项 DoD 全部 PASS，含 Leader 点名的 DoD 缺口项（`res.Periods--`）

## 0. 基线核对（判定前置）

| 项 | `verify_baseline` 记录 | 实测 | 判定 |
|---|---|---|---|
| `head` | `2433e5577b38f1d0fc8ba77bff4bd2641dee7421` | 同值 | ✅ 零漂移 |
| `discovery_sha256` | `01fa0afb51f31d20026fe235927cd30755e95892555290193980c2bdf5bb6585` | 同值 | ✅ 判定原料未漂移 |

`assignment_epoch=1`（承接时记下）。对照树选取用**内容判据**确认而非拓扑推断：
`git show db19e80:internal/hestia/calibrate.go | grep -c eachParsedArticle` ⇒ **0**，确证它是改动前的树。

---

## 1. done_criteria 覆盖矩阵

| # | 完成标准（摘要） | 证据 | 判定 |
|---|---|---|---|
| functional[0] | `eachParsedArticle` 签名与需求文档 Interfaces 段**逐字一致**；返回 sha256 未校验篇数、由调用方汇总成 warning | 签名与需求文档第 85-86 行逐字一致（**含换行位置**）；`calibrate.go:156` 接收返回值，`:170-174` 汇总成 `res.Warnings` | ✅ PASS |
| functional[1] | `collectSamples` 改为调用管道；搬迁时**逐字保留全部注释** | 机械 diff：75 行旧块 vs 72 行新体，**唯一实质差异是 Records 装配 5 行 → `fn(...)` 1 行**，其余 70 行逐字节一致（§3.1） | ✅ PASS |
| functional[2] | `TestEachParsedArticleYieldsFullObservation` 存在且通过，断言交出整个 Observation | 存在且 PASS；变异 M2（不传 item）下**独家转红，外溢度 1**（§3.4） | ✅ PASS |
| boundary[0] | 🔴 `res.Periods++` 位置不得改变（错则 199 变 161） | 变异 M-B 实测 ⇒ **161**，与 DoD 预期数**精确吻合**（§3.2） | ✅ PASS |
| boundary[0′] | 🔴 `res.Periods--` 也必须一起搬（**DoD 未写**，Leader 点名要验） | 变异 M-A 实测 ⇒ **218**，与声称**精确吻合**（§3.2） | ✅ PASS |
| boundary[1] | 沿用既有 fixture，不新写第二份 | 用 `completedFixture(t)`（`calibrate_test.go:88`）；全文件无新增 fixture 函数 | ✅ PASS |
| error_handling[0] | 🔴 **背对背基线比对是唯一验收判据**，`diff` 必须零输出 | **我独立跑**：diff 零输出、两端各 186 行、sha256 `7fdd927c…0134`（§2） | ✅ PASS |
| error_handling[1] | 语料用主仓库绝对路径 + `--allow-incomplete` | 按此跑通，两端 rc=0 | ✅ PASS |
| non_functional[0] | gofmt / vet / test / 覆盖率 / 无新依赖 | 见 §4 | ✅ PASS |
| non_functional[1] | 交付流程 AD-4（merge 先于 dev_done） | 第一轮 merge `23:57:44Z` < dev_done `23:59:12Z`，早 1 分 28 秒（§5 附观察） | ✅ PASS |

---

## 2. error_handling[0]：背对背基线比对（本任务唯一验收判据）—— 我独立复现

**不引用 dev 的数字，自己建两棵树、编两个二进制、同一时刻并排跑。**

```
采样时刻: 2026-09-01T00:47:43Z      采样时主仓库 HEAD: 2433e5577b38f1d0fc8ba77bff4bd2641dee7421
baseline 二进制 ← db19e803a08d63cdd42af8a4bac4dfddc1ff3bf5   (rc=0)
after    二进制 ← 2433e5577b38f1d0fc8ba77bff4bd2641dee7421   (rc=0)
语料: /Users/zuowei/.../data/hestia-backfill-2026-08-14  --allow-incomplete
```

| 项 | 我的实测 | dev 自报 | 一致 |
|---|---|---|---|
| `diff` | **零输出** | 零输出 | ✅ |
| 行数（两端） | **各 186 行** | 各 186 行 | ✅ |
| 输出 sha256 | **`7fdd927c60eacd63808949824fae31a66e99fecebea749bc53b3813abaa0e134`** | 同值 | ✅ |
| 「待解析（受支持期次）」 | **199 篇** | 199 篇 | ✅ |

⇒ **纯重构成立，外部行为逐字节零变化。** 我这次是该 sha256 的第七次独立重现（dev 报了六棵树）。

A/B 两端**跨越本任务的全部改动**（含第二轮 `06c8e98`），不是只测增量 —— 这一点很重要，见 §5。

---

## 3. 变异实测（隔离 worktree，主仓库零改动）

全部变异在我自建的 `wt-t001-after-b`（钉死 `2433e557…`）上进行，每个过唯一性闸 + `gofmt -e` 语法闸 + diff 逐字核对，收尾核实还原（工作树 `git status` 干净、文件 sha256 复原）。

### 3.1 搬迁保真度（Leader 建议独立跑，已跑）

`git show db19e80:internal/hestia/calibrate.go | sed -n '157,231p' | sed 's/d\.Dir/dir/'` vs 新函数体 `calibrate.go:218-289`：

```
旧块 75 行   新体 72 行
70,74c70
<   // 连 Meta 一起留下。此前这里只取 obs.Values，period_type 在采集的这一刻
<   // 就在手边、却被丢掉了 —— 而下游报告恰恰需要它（见 CalibrateResult.Records）。
<   res.Records = append(res.Records, SampleRecord{
<     Period: obs.Meta.Period, PeriodType: obs.Meta.PeriodType, Values: obs.Values,
<   })
---
>   fn(parsedArticle{item: it, obs: obs})
75a72
>   return shaUnverified
```

⇒ **其余 70 行逐字节一致**，含 `res.Periods++` 位置、`fail` 闭包措辞、sha256 在 Parse 之前查、Unsupported 分类判据与全部注释。这是观察不是自述 —— 任何一处注释被改都会显示出来。

> 口径说明：dev 报「只剩 6 行差异」，我数到 5 行旧独有 + 2 行新独有。差的那一行是 `return shaUnverified`，它是函数结构必需的返回语句、不属于搬迁差异。**口径不同，结论相同**，非缺陷。

### 3.2 boundary[0] 与 boundary[0′]：199 的双向自证（**变异实测，不停在算术上**）

对照组先自证：未变异时待解析 = **199**（不是 199 则后续结论全部作废）。

| 变异 | 改动 | 待解析实测 | DoD/dev 声称 | 一致 |
|---|---|---|---|---|
| M-A | 删掉 `res.Periods--` | **218** | 218 | ✅ |
| M-B | `res.Periods++` 挪到成功分流之后（含随之必须删的 `--`） | **161** | 161 | ✅ |

⇒ 三件事同时坐实：① `++` 位置正确；② `--` **也搬了且位置正确**；③ 199 确实是**双向自证**的判据 —— 两个方向的错都破坏同一个数。

⚠️ **`boundary[0]` 只点了 `++`，`--` 是 DoD 缺口**（Leader 已认领为自己的疏漏）。dev 自己发现并上报，且把 199/161/218 三个数写进了**函数头注释**（`calibrate.go:205-214`）而不只写在 discovery 里 —— 载体强度：代码注释 > discovery，下一个改这段的人先看到的是注释。我核实该注释三个数**全部准确**。

### 3.3 输出侧交叉验证

`本迭代不解析: 19 篇` ⇒ 199 + 19 = 218，与 M-A 实测吻合，两条独立路径互证。

### 3.4 B 建议的两条变异断言（Leader 建议核，已核）

dev 在测试注释里写「措辞是变异实测定下来的」—— 这是一个**断言**，其证据必须来自它所描述的那一版代码。我在最终态上重跑：

| 变异 | `TestEachParsedArticleYieldsFullObservation` | 全部转红 | 外溢度 |
|---|---|---|---|
| M1 删掉管道内期次交叉校验 guard | **绿** | `TestCollectSamplesCrossChecksPeriodAgainstTitle` | 1 |
| M2 `fn` 调用不传 item | **红** | 仅本条 | 1 |

⇒ **与注释逐字相符。** 该断言守的确实是「item 与产出它的那次迭代的 obs 配对」，而非原 message 写的「分类期次与正文期次一致」。dev 的订正成立 —— 原措辞会让下一个人拿着这条红去查分类逻辑，而真正坏掉的是 item 的传递。

对照组：`go test ./internal/hestia/ -v` ⇒ **1230 PASS，零 FAIL**。

---

## 4. 门禁项（全部采于判定对象树 `2433e557…`）

| 项 | 实测 | DoD 判据 | 判定 |
|---|---|---|---|
| `go test ./internal/hestia/... -count=1 -cover` | ok, **coverage 95.9%** | 不低于 95.9%（基线 @ `32bc1e5`） | ✅ |
| `go test ./cmd/... -count=1` | ok | — | ✅ |
| `go vet ./internal/hestia/... ./cmd/...` | 零输出 | 零输出 | ✅ |
| `gofmt -l internal/hestia cmd/atlas` | 恰 `cmd/atlas/backtest_test.go`、`crisis_test.go` | 这两个之外无新增项 | ✅ |
| go.mod / go.sum 改动 | **0 行** | 不得出现 | ✅ |
| 实际改动文件 | 恰 `calibrate.go`、`calibrate_test.go` | 与 `writes` 声明一致，**无越界** | ✅ |

---

## 5. 交付流程（non_functional[1]）—— PASS，附一条流程观察

| 事件 | UTC |
|---|---|
| `5094fcd` 第一轮提交 | 2026-08-31 23:47:13 |
| **`053d1a9` 第一轮 merge** | **2026-08-31 23:57:44** |
| **`in_progress → dev_done`** | **2026-08-31 23:59:12** |
| `06c8e98` 第二轮提交 | 2026-09-01 00:04:26 |
| `2433e55` 第二轮 merge | 2026-09-01 00:39:31 |
| `dev_done → verifying`（写 `verify_baseline`） | 2026-09-01 00:45:11 |

**AD-4 满足**：第一轮 merge 早于 `dev_done` 1 分 28 秒 ⇒ `task-completed.sh` 的门禁量到了真实代码。

### 📌 流程观察：第二轮改动发生在 `dev_done` 之后，未重新过门禁

第二轮（`053d1a9..2433e55`，`calibrate.go +16/-7`、`calibrate_test.go +20/-6`）落在 `dev_done` 与派验之间，
因此**没有再经过一次 `dev_done` 门禁**（审计行显示这期间无任何 transition）。

**实质风险已排除，故不阻断**：
1. Leader 在第二轮 merge **之后**才派验，`verify_baseline.head` 锚的是最终态 `2433e55` ✅
2. 我的背对背比对**跨越全部改动**（`db19e80` → `2433e55`），diff 零输出 ⇒ 第二轮的行为影响已被覆盖 ✅
3. 我在最终态上独立跑了全部门禁项，全绿（§4）✅
4. DoD `non_functional[1]` 要求的是「merge 在 dev_done 之前」，未规定 `dev_done` 之后不得追加改动；Leader 全程知情

⇒ 记录而非扣分。若要机制化，方向是让 `dev_done` 之后的追加改动走一次 `dev_done → ... → dev_done` 的回路，
但那需要状态机新增出边，超出本任务范围。

---

## 6. Leader 点名要顺带核的其余两项

### 6.1 C 不采纳的理由 —— **成立**

被建议删掉的是测试里的 `Samples` map 初始化。核实：

| 位置 | 原文 |
|---|---|
| 生产调用点 `calibrate.go:148` | `res := &CalibrateResult{Samples: map[string][]float64{}}` |
| 测试 `calibrate_test.go:1182` | `res := &CalibrateResult{Samples: map[string][]float64{}}` |

**逐字同形**（连缩进都一致）。且 `eachParsedArticle` 当前确实不写 `Samples`（函数体内 `Samples` 出现 **0** 次），
与 dev「今天不显现」的说法吻合。⇒ 删掉后测试会跑在生产中不存在的 nil-map 形态上。

dev 那句话我认为值得留存：

> 「现在删掉仍绿」证明的是它当下没被用到，不是它不该在。

### 6.2 `provenance` 的归属划分 —— **准确，不判冒领或漏记**

它把「发现归子代理、核实取舍与实现归 dev-m1c3b-a」写得很清楚，且**主动订正了自己先前的错话**
（先前写「本任务未获得简化建议」，实为子代理的结论被 idle hook 的身份误认投递给了 Leader）。
还说明了 A/B/C 各自的核实手法（A 用 grep 验消费者数 + 读 TASK-006 的 DoD；B 在隔离 worktree 重跑 M1/M2 并自补 M3）。
子代理未修改任何文件一节，我独立复核：本任务实际改动恰为 `writes` 声明的两个文件，无第三个文件。

---

## 7. 复现命令（锚一律钉全 sha，不写 HEAD/分支名）

```bash
# 背对背基线比对（本任务唯一验收判据）
git worktree add --detach <dirA> db19e803a08d63cdd42af8a4bac4dfddc1ff3bf5
git worktree add --detach <dirB> 2433e5577b38f1d0fc8ba77bff4bd2641dee7421
(cd <dirA> && go build -o /tmp/atlas-base  ./cmd/atlas)
(cd <dirB> && go build -o /tmp/atlas-after ./cmd/atlas)
DATA=/Users/zuowei/workspace/go/src/github.com/newthinker/atlas/data/hestia-backfill-2026-08-14
for b in base after; do /tmp/atlas-$b hestia backfill calibrate --dir "$DATA" --allow-incomplete > /tmp/c-$b.txt 2>&1; done
diff /tmp/c-base.txt /tmp/c-after.txt      # ⇒ 零输出；各 186 行；sha256 7fdd927c…0134

# 搬迁保真度
git show db19e803a08d63cdd42af8a4bac4dfddc1ff3bf5:internal/hestia/calibrate.go \
  | sed -n '157,231p' | sed 's/d\.Dir/dir/' > /tmp/old.txt
(cd <dirB> && sed -n '218,289p' internal/hestia/calibrate.go) > /tmp/new.txt
diff /tmp/old.txt /tmp/new.txt              # ⇒ 仅 Records 5 行 → fn 1 行，另加 return 1 行

# 门禁项（在 <dirB>）
go test ./internal/hestia/... -count=1 -cover   # ⇒ 95.9%
go vet ./internal/hestia/... ./cmd/...          # ⇒ 零输出
```

---

## 8. 结论

**VERIFIED。** 10 项 DoD 全部 PASS，无阻断项。

本任务是纯重构，唯一验收判据（背对背基线比对）由我独立复现且与 dev 自报**逐位吻合**；
DoD 的缺口项（`res.Periods--`）经变异实测 **218 / 161 两个数精确命中**，dev 主动发现并写进函数头注释的做法是对的。

唯一记录项是 §5 的流程观察（第二轮改动落在 `dev_done` 之后未重新过门禁），实质风险已由
「派验锚最终态 + 背对背跨全部改动 + 我在最终态独立跑全部门禁」三重排除，不影响判定。
