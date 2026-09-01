# TASK-006 返工验证报告（第 2 轮）—— C-2 / C-3 / C-4 三条 CRITICAL + 4 组 WARNING

- **验证者**：test-m1c3b-b ｜ **epoch** 2 ｜ **rework_count** 1 ｜ **reason_class** `task_defect`
- **判定对象树**：`d880485137f7ed239dcbe971c2f746687a499f5e`（= `verify_baseline.head`，零漂移）
- **返工前树**：`188eb212` ｜ **执行者**：dev-m1c3b-a（改派）
- **结论**：✅ **VERIFIED**（第三轮，2026-09-01 09:4xZ）—— 两处补漏均已落实并经变异证实

> 📌 本文件含三轮验证记录：**§R3 是最新判定**（第三轮，两处补漏）；§0–§7 是第二轮
> （三条 CRITICAL + 4 组 WARNING，依裁决 9 判 VERIFIED）。三轮的 dev 分别是 b / a / a。

## R3. 第三轮：两处补漏（2026-09-01）

- **判定对象树**：`70eedc4caf3502df9f25f125c7dcc546d06320d3`（= `verify_baseline.head`，双字段零漂移）
- **上一轮树**：`d880485` ｜ `rework_count` 2 ｜ `reason_class` `dod_defect`（两项皆 Leader 自认的 DoD 缺陷）

### R3.1 范围边界：我独立复跑确认

```
git diff --numstat d880485 70eedc4 -- internal/hestia/backfill_load.go   ⇒ 零输出
本轮全部改动: backfill_load_report.go +19/-2   backfill_load_test.go +48/-0
```

⇒ **`backfill_load.go` 一行未改** ⇒ 恒等式四、三条 CRITICAL 的实现、5 条新增测试、W-1 判据
**全部未重开**，第二轮的验证结论继续成立，不需重验。

### R3.2 ✅ 补项一：C-3 守卫 —— **不是空判据，我用失败行号证明**

新增 `TestBackfillLoadDoesNotCreateDBWhenInputIdentitiesFail`。独立跑 V3（= C-3 缺陷原状，
让 `checkInputIdentities` 早退分支永不进入），隔离 worktree 钉死 `70eedc4…`：

```
转红(顶层): TestBackfillLoadDoesNotCreateDBWhenInputIdentitiesFail    外溢度 1
失败位置:   backfill_load_test.go:720
```

**720 行是判据行**，不是 `require.Error` 那一行：

```
716:  require.Error(t, err, "恒等式一不成立 ⇒ 必须失败")        ← 缺陷版本上同样成立，未红
718:  // 🔴 这一行才是本条的判据。上面那个 require.Error 在缺陷版本上同样成立。
719:  _, statErr := os.Stat(db)
720:  assert.True(t, os.IsNotExist(statErr), ...)               ← ★ 死在这里
```

⇒ **我上一轮提的那条约束（判据必须是「库文件不存在」，不能是「返回了 error」）被正确落实，
且我用失败行号而不是读代码证明了它。** 若写成后者，V3 下 716 行照样绿、720 行不存在，
整条测试会是一条长得跟真守卫一模一样的空判据。

📌 dev 把该约束写在**判据那一行的紧邻注释里**（不是函数头），理由是「会被『简化』掉的正是那行
`os.Stat`，它看起来像对 `require.Error` 的重复 —— **理由离得越远越看不到**」。这个位置选择是对的。

### R3.3 ✅ 补项二：假注释订正 —— 到位，且用了订正后的等级

`writeLoadReport` 函数头现在与实现一致（「**先打印、后校验恒等式，顺序不能反**」），并且：

- 记录了它**何时变假**（C-2 之前完全正确，被推翻后没人回来改）与**谁没注意到**（dev、Leader，验证者是第三个看的人）
- **用了我订正后的危害等级**：「不是静默回归（`TestWriteLoadReportPropagatesWriteError` 的注释点名了 C-2、会拦住误改），而是**误导 + 一次无效往返**」
- 补了**正面理由**：恒等式一失败的头号成因是 `Unclassified` 非空，而那批标题原文**只在报告里**
- **正面回应了原注释的担忧**：报告带着 error 一起交出、退出码非 0 ⇒「自洽的表格让人停止追问」不成立

**同类假注释全包复查**（逐行看语境，不看计数）：

| 位置 | 语境 | 判定 |
|---|---|---|
| `backfill_load_report.go:17` | 「⚠️ 此处**原本写的是相反**的话（…）」 | 否定语境 ✅ |
| `backfill_load_report.go:267` | 「**原文写的是**「报告不予输出」——**那句话在 C-2 之后就是假的**」 | 否定语境 ✅ |

⇒ **无残留假注释。** 两处都是引用旧文以说明其为假 —— 这正是 `grep -c` 会误报的形态。

### R3.4 门禁与边界（采于 `70eedc4…`）

| 项 | 实测 | 判定 |
|---|---|---|
| `go test ./internal/hestia/... -cover` | ok, **96.1%** | ✅ |
| `go test ./cmd/...` / `go vet` / `gofmt` | ok / 零输出 / 恰两既有欠账 | ✅ |
| go.mod·go.sum / 改动范围 vs `writes` | 0 行 / 零越界 | ✅ |
| AD-4 | merge `09:31:44Z` < dev_done `09:33:58Z`（早 2 分 14 秒），其后无提交 | ✅ |
| 生产库 `runtime/atlas/data/hestia.db` | 前后各验：`478d40c0…c28c` | ✅ 未触碰 |

### R3.5 第三轮结论

**VERIFIED。** 两处补漏均已落实：C-3 守卫经 V3 证实**能红且死在判据行**（非空判据），
假注释订正到位且无同类残留。`backfill_load.go` 零改动保证了第二轮结论不受影响。

---

## 0. 基线、门禁时点、生产库

| 项 | 值 | 判定 |
|---|---|---|
| `head` / `discovery_sha256` | 与 `verify_baseline` 逐字符相同 | ✅ 零漂移 |
| merge `d880485` `08:57:16Z` → dev_done `09:00:26Z` | merge 早 3 分 10 秒，其后无新提交 | ✅ AD-4 |
| 生产库 `/Users/zuowei/workspace/runtime/atlas/data/hestia.db` | 前后各验：`478d40c0…c28c` | ✅ 未触碰 |
| 覆盖率 / vet / gofmt / go.mod / 范围 | 96.1%、零输出、恰两既有欠账、0 行、零越界 | ✅ |
| 恒等式四（`fix_items[7]` 边界：不许动） | `git diff` 确认**一行未动** | ✅ |

（审计行用 `jq 'select(.task=="TASK-006")'` 精确过滤，不用 `grep` —— 上一轮的教训。）

---

## 1. 裁决 7 那道闸：五个变异实测

隔离 worktree 钉死 `d880485…`，对照组全绿，每个变异过语法闸 + 编译失败检查，收尾两文件 sha256 复原。

| # | 变异 | 转红（顶层） | 外溢度 | 判定 |
|---|---|---|---|---|
| V2 | **M17**：`len(g.SourceIDs) > 1` → `>= 1`（裁决 7 指定） | `TestLoadIdentityThreeIsCrossSourced` + FlagsPartialCoverage + RequiresCompletedAt | 3 | ✅ **能红** |
| V4 | C-2 原状：早退路径不渲染报告 | `TestBackfillLoadFailsLoudlyOnUnclassified` | 1 | ✅ **能红** |
| V5 | 恒等式三不检查 | `TestWriteLoadReportRejectsBrokenIdentity`（子格「三：MergedGroups ≠ 库里 merged@v1 行数」） | 1 | ✅ **能红** |
| **V3** | **C-3 原状：跳过 `checkInputIdentities` 早退**（让恒等式一/二落到末尾才校验） | **无 —— SURVIVED** | **0** | 🔴 **转不红** |
| V1 | `writeLoadReport` 顺序改回「先校验后渲染」 | `TestWriteLoadReportPropagatesWriteError` | 1 | 见 §3 |

---

## 2. 🔴 发现一：C-3 的修复**没有守卫**（需 Leader 裁决）

### 2.1 行为是对的 —— 我用真实场景实测过

自造语料（复制真语料、把首篇标题改成「答记者问」使其解析不出期次）跑 `BackfillLoad`：

```
Unclassified 条数 = 1
🔴 DB 是否被建出来 = false        ← C-3 修复有效
🔴 报告字节数      = 1088         ← C-2 修复有效
   报告含被改的标题「答记者问」 = true
错误串首行: 回填的输入侧恒等式不成立，账对不上；**尚未建库、尚未写入任何数据**：
```

⇒ **C-2 与 C-3 的行为都真修好了**，且错误串补上了 QA 指出缺失的那句「尚未建库」。

### 2.2 但把它改回缺陷原状，**没有任何测试转红**

变异 V3 让早退分支永不进入（`if idErr := checkInputIdentities(res); idErr != nil` → 条件恒假），
于是恒等式一/二重新落到末尾 `writeLoadReport` 才校验 —— **这正是 C-3 的缺陷原状**
（建库 → 灌数据 → 才发现账不对 → 库留下挡住重跑）。

**全套测试无一转红，外溢度 0。**

> 既有的 `TestBackfillLoadDoesNotCreateDBBeforeChecking` 守的是 **D-1**（`os.Stat` 早于 `NewStore`），
> 不是 C-3（恒等式检查早于 `NewStore`）。返工新增的 5 条测试里也没有守 C-3 的。

### 2.3 两条明文规则对 C-3 给出**不同结论** —— 这是规则冲突，不是我扩大解释

| 规则 | 对 C-3 的要求 | 是否满足 |
|---|---|---|
| `fix_items[1]` | 「拆出 `checkInputIdentities`（恒等式一、二）**在 `NewStore` 之前调用**」 | ✅ 满足（`:330` 早于 `:342`） |
| **裁决 7** | 「C-1/C-2/C-3 的对应变异……**转不红即说明补的仍是空判据，不予验收**」 | ❌ **不满足**（V3 SURVIVED） |

**我不自行解释哪条优先**（wisdom 20：字面与实质分歧时由 Leader 裁决）。我的建议与理由：

- 倾向**不因此判 REJECT**：`fix_items[1]` 只要求改顺序、未要求补判据；行为经实测正确；裁决 7 的措辞
  （「补的**仍是**空判据」）预设了 dev 补了判据，而 C-3 本就没被要求补。
- 但**缺守卫是真实风险**：任何人把 `checkInputIdentities` 移回末尾，测试全绿、C-3 静默复现，
  而它的症状（失败后留下半成品库挡住重跑）**只在真跑时才暴露**。
- ⇒ 建议：判 VERIFIED 的同时，把「补一条 C-3 守卫」列为必须项。**形状是现成的**——我验证时那个探针
  就是它，改成测试不到 15 行，且**不需要新夹具**（复用真语料 + 改一篇标题）：

```go
// TestBackfillLoadDoesNotCreateDBWhenInputIdentitiesFail：输入侧恒等式失败时
// 不得留下半成品库（M1c-3b 的 TASK-006，C-3）。
// 判据是**库文件不存在**而不是「返回了 error」——后者在缺陷版本上同样成立。
func TestBackfillLoadDoesNotCreateDBWhenInputIdentitiesFail(t *testing.T) {
    dir := <真语料副本，把一篇标题改成解析不出期次的串>   // ⇒ Unclassified 非空 ⇒ 恒等式一必不成立
    db := filepath.Join(t.TempDir(), "c3.db")
    _, err := BackfillLoad(context.Background(), BackfillLoadDeps{
        Dir: dir, DBPath: db, Cfg: DefaultThresholds(), Out: io.Discard, AllowIncomplete: true})
    require.Error(t, err)
    _, statErr := os.Stat(db)
    require.True(t, os.IsNotExist(statErr),
        "输入侧恒等式失败时不得建库——留下的半成品库会被下次跑自己的『--db 必须不存在』拒掉，"+
            "而错误串不会提到它刚建了库")
}
```

  ⚠️ **判据必须是「库文件不存在」，不能是「返回了 error」** —— 缺陷版本同样返回 error
  （只是晚在末尾才返回，且库已经建好了）。这正是第 29 条那个形状：**不会变的数不是验收数**。
  我已实测该断言在当前实现上通过（`DB 是否被建出来 = false`），在 V3 变异下会失败。

---

### 2.4 ✅ Leader 裁决（`plan.md` 裁决 9，2026-09-01 09:20Z）

**裁决：判 VERIFIED，不因变异 SURVIVED 判 REJECT。** 理由（Leader 采纳我的表述）：

> 裁决 7 的措辞是「补的**仍是**空判据」，**预设了 dev 补了判据**；而 `fix_items[1]`
> **只要求改行为、从未要求补守卫**。判它不过 = 拿一条它没被要求满足的规则去卡它。

Leader 同时认领了根因：**「这是我的第 9 处 DoD 缺陷：要求了行为，没要求守卫。」**
并指出裁决 7 本身须修（留 M1c-4），正确表述是
**「每一条 fix_item 修复的行为，都必须有一条能红的守卫；没有守卫的修复不算完成」**。

**后续（Leader 已定，不影响本次判定）**：走 `verified → review_fix` 补两项 ——
① 本报告 §2.3 给出的 C-3 守卫（含「判据必须是库文件不存在」那条约束）
② §3 的假注释订正。`reason_class` 按裁决 8 用 `dod_defect`。

⚠️ 一处流程实证：Leader 此前只发消息未写文件，**那条裁决消息丢了（本 sprint 第 4 次）**，
我读 plan.md 读到的是 5 条无 C-3。**这一条本身就是「消息不是状态」的又一次实证** ——
而我因为坚持读文件而不是等消息，才在它落盘后 1 分钟内拿到。

---

## 3. 🔴 发现二：第三处假注释（dev 与 Leader 都未提及），且我量化了它的危害

`backfill_load_report.go` 第 15-16 行，`writeLoadReport` 的**函数头注释**：

```go
// 🔴 **恒等式先校验、后打印**：报告本身就是验收物，它不能在数字对不上时照样打印
// 一份好看的表格 —— 一份自洽的表格会让人停止追问，而那正是账对不上的时候最不该发生的事。
func writeLoadReport(w io.Writer, dir string, res *BackfillLoadResult) error {
	if err := renderLoadReport(w, dir, res); err != nil {   // ← 先渲染
		return err
	}
	return checkLoadIdentities(res)                          // ← 后校验
}
```

**注释与实现完全相反**，而 C-2 的整个修复就是把顺序反过来。

Leader 点名要核的两处假话，我核了，**都已订正**：
- ① `checkLoadIdentities` 错误串的「报告不予输出」⇒ 现存的那处是**订正说明里的否定语境引用**
  （「原文写的是『报告不予输出』——那句话在 C-2 之后就是假的」），**看行才分得出，`grep -c` 会假阳**
- ② `TestWriteLoadReportRejectsBrokenIdentity` 的注释 ⇒ 已订正

**但这第三处没被订正。** 它的危害我用 V1 量化了：

> 变异 V1（把顺序改回注释描述的样子）⇒ **只红一条 `TestWriteLoadReportPropagatesWriteError`，外溢度 1**。

### ⚠️ 危害等级订正（我先前报高了）

我最初据「外溢度 1 + 测试名看起来无关」判为「C-2 会静默回归」。**读了那条测试的注释后订正**：

```go
// TestWriteLoadReportPropagatesWriteError：写不出去时要原样报出，不要被恒等式错误盖掉
// （M1c-3b 的 TASK-006，C-2 的边界）。
// C-2 把顺序改成「先写再校验」之后，w 不可用是一条新的失败路径：此时若继续查恒等式，
// 返回的会是一个恒等式错误，而真正的成因（写不出去）就消失了。
```

⇒ **它就是那个顺序的守卫**，注释点名 C-2 并解释了为什么不能反。照假注释改的人会撞红它、读一眼就明白。

**「静默回归」不成立。** 假注释仍应修（与实现相反、带 🔴 强语气，会让人**去尝试**改，浪费一次往返），
但等级是「误导 + 一次无效往返」，不是「拆掉修复而无人察觉」。

> 📌 我的方法教训：评估「守卫能不能拦住误改」有**两个维度** ——
> **红不红**（变异回答，决定会不会被拦）与**守卫传达了什么**（读注释/message 回答，决定拦住后他明不明白）。
> 我只量了前一半就下了后一半的结论。而误改的人首先读到的正是后一半。

### 附：Leader 问的「先渲染是不是真的结构性质」—— 部分是

- **是结构性质的部分**：`renderLoadReport` 被拆出来，早退路径（`NewStore` 之前）**有可用的纯渲染函数**。
  不拆就无法同时满足 C-2 与 C-3（一个要提前失败、一个要失败也有输出）。这一层是真的结构。
- **仍是约定的部分**：`writeLoadReport` **内部**的两行顺序仍可被改回，只红 1 条测试（V1）。
  ⇒ 「先渲染」在**末尾路径**上仍是调用顺序上的约定，不是结构强制的。

---

## 4. Leader 给的四条前置信息，逐条核实

| # | 前置信息 | 我的核实 |
|---|---|---|
| 1 | `79/17` 不是本次造成的，是 TASK-011 的 C-1 | ✅ 已在 TASK-011 验证中独立真跑得 79/17，与本轮无关 |
| 2 | W-1 假阳在它树上是 7 个、QA 报 3 个，属不同的树 | 待补（§5） |
| 3 | 覆盖率中途掉到 95.8%，补测试后回 96.1% | ✅ 实测 96.1%，与基线持平 |
| 4 | 恒等式四未动（`fix_items[7]` 边界） | ✅ `git diff` 确认一行未动 |

---

## 5. W-1 复核：**假阳完全消除**

独立真跑（探针直调 `BackfillLoad`，临时库）：

```
PartialCoverage 条数 = 61      MissingFields 条数 = 61   （一一对应）
🔴 字段数 >= 52 的（W-1 假阳形态）= 0 条
字段数分布: 9→2  18→42  27→2  36→13  45→2      ← 全部 < 52
```

⇒ W-1 要修的正是「54/54 字段全非空却被列为部分覆盖」，**现在这类为 0**，每一条都真的缺字段。
条数 61 与 dev 自报一致。

## 6. 第三类缺口检查：5 条新增测试**全部通过**

按「看到手工构造结构体字段 ⇒ `grep '\.字段名 ='` 找生产赋值点核对可达性」：

| 测试 | 手工构造 | 可达性 |
|---|---|---|
| `TestLoadReportSurfacesAllSignals` | `SHAUnverified`/`FetchFailed`/`PartOverlaps`/`MissingFields` | 生产赋值点各 1/1/2/1 处 ✅ |
| `TestWriteLoadReportPropagatesWriteError` | `res.Total = 99` | 有意为之且注释说明理由 ✅ |
| `TestMergeConflictFailsTheRun` | `&BackfillLoadResult{}` + Conflicts | 测纯函数 `conflictError` ✅ |
| `TestLoadIdentityThreeIsCrossSourced` / `TestPartOverlapsAreEmptyOnRealFamilies` | 无手工构造 | ✅ |

**关键差别在基础夹具**：`okResult()` 本身四道恒等式全自洽（`3=3+0`、`3=3+0`、`1=0+1`、`1=1+0`，
且 `DBMergedRows:1, MergedRowsCounted:true`），测试在其上**加**信号字段；
而被 QA 抓住的旧 `SurfacesUnclassified` 是为了跑到某分支而**拼出一个自相矛盾的状态**。

⚠️ dev 在 `okResult()` 注释里自己写明了这一层：

> 夹具要同时给两边，否则那道闸被 `MergedRowsCounted==false` 跳过，而**跳过时它不会红** —— 那正是它此前的毛病。

**这正是第三类缺口的形态，它独立想到了。** 同一洞见本轮被三个角色各自撞到（QA 从审查、我从学习、dev 从修复）
⇒ 不是某人的巧思，是这个问题域客观存在的一道坎。

## 7. 结论

**VERIFIED。** 三条 CRITICAL 的行为经真实场景实测均已修复；C-2、C-4 通过裁决 7 那道闸
（V4、V2 各自转红）；C-3 的行为正确但**无守卫**，依裁决 9 不因此判 REJECT，已转为后续 `review_fix` 补项。

其余全部通过：W-1 假阳清零（字段数 ≥52 的 0 条）、5 条新增测试无第三类缺口、
门禁全绿（96.1% / vet 零输出 / gofmt 恰两既有欠账 / go.mod·go.sum 0 行 / 零越界）、
恒等式四一行未动、生产库真跑前后 sha256 逐字符不变。

**本次验证发现两项需后续处理**（均已进 Leader 的 `review_fix` 补项）：
C-3 缺守卫（V3 SURVIVED，外溢度 0）与 `writeLoadReport` 函数头假注释。
