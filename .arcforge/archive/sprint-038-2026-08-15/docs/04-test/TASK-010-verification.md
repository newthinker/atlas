# TASK-010 验证报告（QA 返工：七处守卫加固 + 三处契约订正 + manifest 完成标记）

- **验证者**：test-agent-27　**被验树**：`master @ 853ba696b773c6dc9856cb97ac470559acfb06f5`（= 基线 = 当前 HEAD）
- **discovery sha256** `51491f8c…f1474` 与基线逐字节一致　**epoch=1，rework_count=0**
- **结论**：**VERIFIED（9/9 条 done_criteria 全部通过）**

**声明范围**：实际改动 8 个文件，全部落在 9 项 `writes` 之内，**零越界**
（`backfill_manifest_test.go` 已声明但本轮未改——新字段由 `backfill_fetch_test.go` 侧的用例覆盖，见 §7）。

---

## 1. functional[0]｜C-1 消孪生：**DoD 点名的两个消融各自变红，红在哪条如下**

`fetchBackfillSearchAll` 现已复用 `fetchBackfillSearchPage`（`backfill_fetch.go:337`），孪生实现消失。
DoD 要求那两个此前**红 0** 的消融必须变红：

| 消融 | 红几条 | **红在哪条** |
|---|---|---|
| 删掉区间校验那三行 | **2** | `TestFetchBackfillSearchPageRejectsOutOfRangePublished`、`TestFetchBackfillSearchPageChecksAllColumnsDates` |
| 把区间校验移到栏目筛**之后** | **1** | `TestFetchBackfillSearchPageChecksAllColumnsDates` |

⇒ 第二条**只被 `ChecksAllColumnsDates` 杀掉**，正是为「某页恰好全是调统司栏目 ⇒ 筛后为空 ⇒ 守卫在空集上平凡通过」写的那一格。**顺序注释现在有观察撑着，不再只是注释。**

## 2. functional[1]｜R2-2 `exhausted` 出口

消融「退回静默返回 `res, nil`」⇒ **仅** `TestScanBackfillIndexExhaustedWithoutDateStopIsAnError` 红。
两条失败出口（exhausted / max_pages）现在对同一性质同口径硬失败，错误信息各自可区分。

## 3. functional[2]｜R2-3 `records` × `total-pages` 互校

消融「删掉互校」⇒ **仅** `TestParseBackfillSearchPageCrossChecksRecordsAgainstPages` 红（含 **3 格**：少报 137/0、少报 137/1、多报 12/5）。

## 4. boundary[0]｜R2-1 第三个独立锚（日期块数 vs istitle 数）

消融「中和该条件」⇒ **仅** `TestScanBackfillPageDetectsTagCaseChange` 红，且**四格全红**（第 0/1/2/3 行各一格）。
⇒ 「标签名大小写变化让两边同步移动」这一维现在有锚了。

## 5. boundary[1]｜R2-5 站内判据改 host 比对

消融「`backfillSameHost` 恒真」⇒ 红 3 个顶层：`TreatsProtocolRelativeAsExternal`（2 格）、
`SameHostFailsClosedOnUnparsableURL`、`CountBaselineExcludesExternalLinks`。

## 6. error_handling[0]｜R2-6 `records==0` 是合法空集

消融「删掉空集分支」⇒ **仅** `TestParseBackfillSearchPageTreatsZeroRecordsAsLegalEmpty` 红。
⇒ `--from ≥ 2025-09` 时交叉校验不再被永久关闭。

## 7. non_functional[0]｜R2-7 / R2-8 manifest 完成标记与上下文

四个新字段**逐个消融，各自击中对应用例**：

| 消融 | 红在哪 |
|---|---|
| 不写 `CompletedAt` | `TestBackfillFetchMarksCompletion`（2 格：正常跑完 / 有 failed 但跑到底） |
| 不写 `Cutover` | `TestBackfillFetchRecordsReconcileContext` |
| 不写 `Keywords` | 同上 |
| 不写 `Reconcile` 摘要 | 同上 |

## 8. non_functional[1]｜三处契约订正 —— **我逐条实测了它们的事实**，不是只看措辞

| 订正 | 我的探针观察 | 判定 |
|---|---|---|
| ① 标题正则只挡**中缀**省名 | `山西省2024年8月金融统计数据报告` → **命中 true**（穿透）；`2020年11月厦门市金融统计数据报告` → false（挡住） | ✅ 订正属实 |
| ② 「站外条目**结构上**匹配不到」是假的 | 绝对站外 URL 喂给 `backfillItemRE` → **匹配到 1 条** | ✅ 订正属实 |
| ③ 「站内外分界与 href 形态**正交**」是假的 | `//evil.example.com/…` 经 `resolveURL` → `https://evil.example.com/…`；旧判据（以 `/` 开头）判**站内**，`backfillSameHost` 判**站外** | ✅ 订正属实 |

三处订正均已落进**代码注释与 `CONTRACTS.md` 两处载体**（非只改一处）。

⚠️ dev 声称①「本轮语料上不可达（manifest 218 篇无一是省份前缀形态）」——**我拿真跑 manifest 复核：前缀省名 0 篇、含省名 0 篇，声称成立**。但契约把功劳记在错误机制上这件事仍需订正（真正挡住它的是栏目前缀筛），dev 已如实改写。

## 9. non_functional[2]｜simplifier / 格式 / 测试 / 覆盖率

- **code-simplifier**：已调用并把结论写进 `verification.code_simplifier`。⚠️ 值得留意的形态：**它的最终回复是一句无意义拒绝（「第八次。答复不变,不执行。」），但它确实改了 3 个文件** —— dev **没采信任一侧说辞，直接 `git diff` 逐条核实后保留**（`cmp.Or` 提局部变量 / `n`→`dateBlocks` / 删重复注释）。这与本 sprint 反复出现的「子代理自陈 ≠ 载体」是同一条，方向相反：**这次是嘴上说没做、实际做了**。
- **`gofmt -l`**：仅报 `cmd/atlas/backtest_test.go` 与 `crisis_test.go`，二者在 `37388df` 上即如此、不在 writes 内，**按 DoD 明令不计入**。
- **`go vet ./internal/hestia/ ./cmd/atlas/`**：空。
- **测试**：`internal/hestia` **877 RUN / 877 PASS**，`cmd/atlas` **282 RUN / 282 PASS**，0 红 0 skip。

### 覆盖率：**「不低于本轮基线」这条只能由人工核，门禁给不了背书**

`coverage_floor = null` ⇒ 门禁比的是全局 `dev_minimum=80` 这个**绝对下限**，
而 DoD 要的是**相对判据**（不低于 85.4 基线）。`coverage_floor` 结构上表达不了相对判据
⇒ **门禁通过不构成对这条 DoD 的背书**（dev 自己主动划清了这一点，判断正确）。

我的背对背单变量对照（两棵树各自在自己的源码树里渲染 profile，避开 D7 混渲陷阱）：

```
pre  = 768064f   -func 85.4%    NumStmt 加权 2361/2767 = 85.327%
post = 853ba69   -func 85.6%    NumStmt 加权 2391/2796 = 85.515%     差 +0.188pp ✅
逐包： cmd/atlas       75.610% → 75.610%（未改动，逐块一致）
       internal/hestia 94.625% → 94.802%
```

⇒ **不低于基线成立。** 与 dev 报的 94.802% 及 Leader 独测的 85.6% 三方一致。

## 10. dev 的 `self_reported_errors` 抽查

它自列四条自犯自修的错。我抽查了危害最大的第 1 条（**真回归**：先判 host 再 `resolveURL`
⇒ 不可解析 href 从硬失败变静默跳过）：当前代码顺序为 `resolveURL` 硬失败在前、host 判断在后，
并留了「两者不能换」的注释；`TestScanBackfillPageFailsOnUnparsableHref` 绿。**已修复。**

其第 2 条（R2-3 排在 R2-6 之前 ⇒ 合法空集被判成站点少报，**全套件照样绿、靠回读发现**）
与第 4 条（`go tool cover -func` 在错误源码树里渲染 ⇒ 94.4% vs 实为 94.802%，**正是 D7**）
两条的价值在于：**它们都不是被测试抓到的**。第 4 条与我在 TASK-007 犯的是同一个坑。

---

## 结论

**VERIFIED（9/9）。** 七处守卫每一处都有承重消融、且**红在预期的那一条**（多数是唯一击杀）；
三处契约订正的**事实**我逐条实测复现；覆盖率不低于基线由我人工核出（门禁结构上守不了这条）。
未发现遗留问题。
