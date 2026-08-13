# TASK-011 验证报告 —— Discover 判停规则：HasPeriod → HasArticle

- **验证者**：test-agent-26 ｜ **交付**：dev-agent-52，提交 `fd6a24cfcce8df205a09df057613a9314bc8f917`
- **验证基线**：`verify_baseline.head = fd6a24c…` = 承接时 HEAD ⇒ **HEAD 无漂移**
- **assignment_epoch**：1 ｜ **结论**：**VERIFIED**（全部 done_criteria 通过）

> 📌 本任务修的是**我上一轮报告并实证的缺陷**。为避免「自己验自己的发现」变成走过场，
> 下文每一条都用**我自己写的探针/变异**取证，不复用 dev 的任何结论。

---

## 0. 承接核实

| 核项 | 结果 |
|---|---|
| HEAD 漂移 | 无 ✅ |
| **DoD 未被改写** | 指纹 `903906f63cd256b7` == 我的 wave3 基线（`16:49:58Z` 存）✅ **未变** |
| 实际改动 vs `writes` | `discover.go` / `discover_test.go` / `ingest.go` / `store.go` / `store_test.go` —— **与声明逐字一致，无越界** ✅ |
| discovery 指针 | 原本缺失，**由我补上**（本 Sprint 第 7 例） |
| **discovery 内容漂移** | ⚠️ **确实发生**，见 §5 |

---

## 1. 完成标准覆盖矩阵

| # | done_criteria | 证据 | 判定 |
|---|---|---|---|
| functional[0] | 判停换成按 article_id；**必须有一条测试逐字复现「已入库期次排第 1 页、未入库在第 2 页 ⇒ 断言未入库那期被发现」** | `TestDiscoverDoesNotStopAtKnownPeriodAheadOfUnknownOne`（构造与我上一轮实证时用的 p7+p2 一致）。**我做变异确认其鉴别力**，见 §2 | **PASS** |
| functional[1] | 接口调整由你定形态，**必须在 discovery 写明选了哪种及理由**；同步改 `ingest.go` 调用点 | 选 `ArticleChecker{HasArticleInObservations}`；discovery 的 `decisions` 写明理由且**逐条排除了两种折中**（见 §3）；`ingest.go` 已同步（+28/-2） | **PASS** |
| boundary[0] | 正面处理 `discover.go` 那段 M0 反论证；**用测试实证两轮自愈**，贴两轮输出；先钉「最脆一环」 | `TestDiscoverSelfHealsAfterSiteMigration` 真库两轮，第 1 轮 `StopMaxPages`、第 2 轮 `StopSeen`；**最脆一环由 `require.Equal(bitemporal.Duplicate, out.Verdict)` 直接钉住**。**我独立核过该环**，见 §4 | **PASS** |
| non_functional[0] | 闭合「16 vs 十七」缺口（message 由 `len(want)` 生成） | `want := []string{…}` + `assert.Equalf(…, "恰好是这 %d 个…", len(want))` ✅ 手写汉字数字已消除 | **PASS** |
| non_functional[1] | `gofmt`/`vet` 空、整包 `-count=1` 与 `-race` 全绿、覆盖率 ≥93.2% | 隔离树实测：gofmt 空 / vet 空 / build OK / **317 顶层 PASS、677 全部、0 FAIL** / cover **93.5%** / `-race` ok | **PASS** |

**新增导出方法 `Store.HasArticleInObservations` 已登记**进 AST 版守卫（18 项）。

---

## 2. 那条「缺它等于没做」的测试，我验了它的鉴别力

**第一次变异失败，如实记录**：我把 `it.ArticleID` 换成 `it.Period` 传给 `HasArticleInObservations`
⇒ 红了 4 条，但**那条关键测试没红**。原因是该用例的 fake `have` map 是空的，传什么都返回 false。
⇒ **我的变异没有真正模拟「按期次判停」**，不构成对它的检验。

**第二次（正确版）**：直接改回旧行为 —— 判停用 `HasPeriod`、去重键用期次。
第一版因 `neverSeen` 不实现 `HasPeriod` 而**编译失败**（`go vet` 闸挡下，顶层 `PASS=0` 说明根本没跑，
是无效变异不是 KILLED）；改用类型断言绕开接口约束后：

```
--- FAIL: TestDiscoverDoesNotStopAtKnownPeriodAheadOfUnknownOne   ← 红在 discover_test.go:1110
--- FAIL: TestDiscoverSelfHealsAfterSiteMigration
--- FAIL: TestDiscoverDedupKeyMatchesStopKey
--- FAIL: TestDiscoverStopsAtKnownPeriod
--- FAIL: TestDiscoverEmptyStoreExhaustsWhileKnownStopsEarly
顶层 PASS=312 FAIL=5（基线 317，外溢 0）
```

⇒ **鉴别力确认**：把实现改回旧行为，那条测试立刻红在「不该让第 2 页那期跟着消失」。

该用例本身的质量：有三条前置锚点（两页各恰一期、两期不同）；断言 `f.calls == 2`（**翻页确实发生**）；
用 `Contains` 而非 `got[0]` 并注明「顺序不是本用例要守的性质」。

---

## 3. 接口选择：它排除了两种折中，理由我认为是对的

- `hasArticle **||** hasPeriod` ⇒ 修订版仍命中期次 ⇒ **完全抵消本任务的修复**
- `hasArticle **&&** hasPeriod` ⇒ 看似两全，但它是**近似**而非精确的「这篇在不在权威表」——
  同一期原文已在权威表、修订版落 pending 时两者皆真会误停

> **「拿近似判据当精确用正是本 bug 的成因家族」** —— 这句判断我认同，它正是原缺陷的形状
> （「index 按发布时间倒序」被当成「按期次倒序」用）。

**为什么是 `HasArticleInObservations`（只查权威表）而不是 DoD 字面写的 `HasArticle`（查两表）**：
若判停查两表，**落 pending 的期次会挡住其后所有未入库期次的发现** —— 那是把刚修掉的 bug 换个位置重造。
pending 那篇仍被交出候选，由 ingest 层的 `HasArticle` 挡住。⇒ **偏离 DoD 字面，但达成 DoD 目的且更正确。**

**两处超出 DoD 的自主发现**（DoD 未要求，我认为有价值）：
① **去重键也从期次改成 article_id** —— 否则「原文与修订版同时在榜」时，修订版会把期次键占上，
原文在**查库之前**就被 `continue` 掉，判停漏判。**这一层我上一轮没发现。**
② 新增 `StopReason`（`StopSeen`/`StopExhausted`/`StopMaxPages`），使「为什么停」可观测 ——
`TestDiscoverDedupKeyMatchesStopKey` 正是靠它才断得出差异（该用例的注释坦承：消融时发现把去重键
改回期次**没有任何断言变红**，那句注释当时只是「一句没人守的声明」，补测试才钉住）。

---

## 4. 「最脆一环」我独立核过（在它交付前就核了）

DoD 说这一环不成立则整条自愈链不成立。**我读源码独立确认**（读断言与实现，不读用例名）：

- `store.go:58` `NewSpec(TableObservations, []string{"period","period_type"}, "published_at")` ⇒ **业务键不含 `article_id`**
- `lookup.go:54` 生成 `SELECT MAX(published_at) … WHERE period = ? AND period_type = ?` ⇒ **整条 SQL 无 `article_id`**
- `classify.go` `incoming == LatestRevision` ⇒ **`Duplicate`**

⇒ 同期次 + 新 article_id + published_at 不变 ⇒ **必判 `Duplicate`** ⇒ `refreshArticleID` 刷新 ⇒ 下轮命中。

**dev-52 的测试用 `require.Equal(bitemporal.Duplicate, out.Verdict)` 把这一环直接钉住，与我的独立结论一致。**
它还不看 `Save` 的返回值、**直接问库**（`s.HasArticle(found.ArticleID)`）确认 id 真的被刷新 —— 这一步是对的。

---

## 5. ⚠️ discovery 内容漂移：我 ack 了，理由写在这里

`verify_baseline.discovery_sha256` = `e88090c2a9d3904d…`，当前 = `9d51d26d5678f502…` ⇒ **确实漂移**
（dev-52 主动报告：`dev_done` 后 10 秒改写，追加 `key_findings` 一条、`decisions` 一段 rationale、
`open_items_for_leader` 一条；声称 `verification` 整块未动）。

**我无法验证「只改了那三处」这个声称** —— `.arcforge/` 未被 git 跟踪、discovery 是覆盖写、
`verify_baseline` 只记 sha256 不记内容 ⇒ **基线时的内容已不可再生**。dev-52 自己也指出了这一点
（「那是关于我的动作的陈述，不是关于文件的陈述」）。

**我仍然 ack，理由是判定的锚不在 discovery 上**：本报告每一条结论都出自**我自己的探针与变异**
（§2 的鉴别力变异、§4 的源码核实、隔离树的全套实测），**没有一条依赖 discovery 里的自证数字**。
即使那些数字被改过，判定也不受影响。

⇒ 📌 **但这暴露一个机制缺口，应当记下**：`verify_baseline.discovery_sha256` 能**告警**漂移，
却**无法核实**漂移内容 —— 因为没有任何地方保存基线时的那一份。**它是一个只能亮灯、不能查证的守卫。**
要补，得让写通道在覆盖 discovery 前留一份旧值（与我此前建议「给 doc/task 提供读回入口」同源）。

---

## 6. 复现（锚已钉全 sha）

```bash
git worktree add --detach ../wt-v-w3 fd6a24cfcce8df205a09df057613a9314bc8f917
GOTOOLCHAIN=local go test ./internal/hestia/ -count=1 -cover   # 93.5% / 317 顶层 / 0 FAIL
# §2 的鉴别力变异：用类型断言把判停改回 HasPeriod + 去重键改回期次 ⇒ 5 红、外溢 0
```

---

## 7. 验证后补记：dev-52 主动更正了消融行号 —— 一个**通用**的错误形态

出具判定后，dev-52 报告其 discovery 里的消融行号有系统性偏差。**我在提交态 `fd6a24c` 上独立核实，属实：**

| 消融 | discovery 记的 | 交付态实际 | 差 | `:记的值` 在交付态上是什么 | 那次变异动过测试文件吗 |
|---|---|---|---|---|---|
| A1 | `:1114` | **`:1110`** | +4 | `for _, c := range got {` ❌ **非断言** | 是（补了 4 行 `HasPeriod`） |
| A2 | `:1300` | **`:1292`** | +8 | **空行** ❌ | 是（两个 fake 各补 4 行） |
| A3 | `:1246` | `:1246` ✅ | 0 | `assert.Equal(t, StopSeen, stop,` ✅ | **否**（只改 `discover.go`） |

**偏移量恰好等于变异脚本往测试文件里插入的行数**，而唯一没偏的那格恰是唯一没动测试文件的那格。

### 形态：**消融报出的行号属于变异树，不属于交付物**

变异后跑测试，失败行号自然是**变异态**文件的行号；而报告的读者会拿它去查**交付物**。
只要那次变异动了测试文件，其后所有行号都被推下去 —— 而**一个错的行号看起来和有效行号完全一样**。

⇒ **本报告 §2 引用的 `:1110` 是我自己在交付态实测的，不受影响。**

⇒ 可复用的做法：**消融行号要么在还原后于交付态复核一遍，要么改为引用断言原文**（断言原文不随行号漂移）。
本 Sprint 我在 TASK-006 报告里记过一次形态相同的错（dev-53 记 `:2232`、实际 `:2253`），
⚠️ **但那次我核过两版文件均 2401 行、内容未变 ⇒ 不是同一个机制**，成因未明。
**不拿这次已确认的成因去解释那次未确认的现象。**

### 附：dev-52 对我第一次无效变异的归类，我认同并记下

我第一次的变异（把 `it.ArticleID` 换成 `it.Period`）红了 4 条却没红目标测试，成因是那个用例的 fake
`have` 是空 map、传什么都返回 false ⇒ **变异根本没生效，红的是别的**。
它把这归为「**假 KILLED 的对偶形态**」：不是「杀手另有其人」，是「变异压根没生效而红了别的」。
两者都会产出一个看起来正常的 KILLED 记录。

---

# 复验（QA `review_fix` 第 1 轮）—— **PASS**

- **判定对象** `b6b13a4ac3ab…`（= `verify_baseline.head`，**无漂移**）｜ discovery 无漂移（`090cce68f628`）｜ `rework=1`
- **基线含本任务返工提交** `0ad726996bf1 fix(TASK-011): StopReason 有候选时也要说 + 清两份过期结论` ✅

## 3 条 `fix_items` 逐条核

**① WARNING-1 — `StopReason` 在有候选时被丢掉** ⇒ **PASS**

`fix_items` 要求三件，逐条对上：

| 要求 | 实现 |
|---|---|
| 候选非空时也打停止原因 | `ingest.go:117` `fmt.Fprintf(d.Out, "discover stopped: %s (%d candidate(s))\n", stop, len(cands))` |
| `stop == StopMaxPages` 时写 **stderr** | `ingest.go:105-110`，文案点明「窗口外的期次本轮不可达」 |
| **不要改退出码** | 未改，仍 `return nil` ✅ |
| **补两条断言**（`Ingest` 会输出停止原因 / `StopExhausted`） | `discover_test.go:1341` `assert.Contains(out, "discover stopped:")`、`:1343` `assert.Contains(out, string(StopExhausted))`，**且有前置锚点** `require.NotEmpty(outPeriods(...), "本用例要的是「有候选」那条路径")` |

**② WARNING-2（第 1、3 份过期副本）** ⇒ **PASS**
- `ingest.go` 的「恒为 `New`/当前不可达」**残留 0**
- `store.go:219` / `store_test.go:359,1956` 仍出现 `PeriodChecker`，但**读原文全是「已删」的更新后说明**（「`PeriodChecker` 接口已删」「TASK-011 起判停键是 `HasArticleInObservations`」）⇒ **不是残留**
- **护栏照做了**：`ingest.go:208-211` **保留**「不要为当前的局限写断言」这个**决定**及其实证（`TestForceOnObservedPeriodIsDuplicate` 现在是绿的、且**从未因局限被放开而红过**），**删除**了「恒为 New/不可达」这个**事实陈述**

**③ WARNING-3 的 `ingest.go` 半（`IngestDeps.Force` 字段注释）** ⇒ **PASS**
注释已点名「穿不透第三层：同篇 `published_at` 不变 ⇒ 恒判 `Duplicate` ⇒ `refreshArticleID` 只刷 id、新抽 Values 一个不写」。

## 实跑
`vet`/`build`/`test ./...`/`-race` **全部 exit 0**；覆盖率 `internal/hestia` **93.6%**。

## DoD 之外的观察
`ingest.go:105-110` 那条 **stderr 告警本身没有守卫**（`fix_items` 只要求断言「输出停止原因」与「`StopExhausted`」，两条都补了）。
⇒ 不影响判定；登记形态：**加了输出、没加守卫** —— 与本 Sprint 的主题同族。
