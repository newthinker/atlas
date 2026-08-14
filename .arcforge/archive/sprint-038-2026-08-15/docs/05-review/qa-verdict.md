# Sprint M1c-1 · QA 两轮审查裁决

<!-- qa-m1c1 | 2026-08-15 | 审查对象树 768064f5868274687dc3a5d598b49068a6587cd3 -->

## 裁决：REJECT（返工范围窄且已枚举；**交付物本身合格，不需重跑**）

| 轮次 | 判定 | 报告 |
|---|---|---|
| 第一轮 常规审查 | CONTESTED（2 CRITICAL / 6 WARNING / 6 SUGGESTION） | `docs/05-review/code-review-round1.md` |
| 第二轮 跨视角对抗 | **REJECT**（8 CRITICAL / 8 WARNING，三视角无事实分歧） | `docs/05-review/adversarial-review.md` |

## 基线（自采，同一棵树，最后一次改动之后）
1133 条 RUN（两包，含子测试）/ 1133 PASS / 0 FAIL；`internal/hestia` 单包 851/0；
`go vet` exit 0；`gofmt -l` 只报既有 `backtest_test.go` + `crisis_test.go`（`37388df` 上即如此）。

## 必须返工（8 项，代码约 25 行 + 3 处契约订正）
1. 生产路径的搜索侧区间校验**零背书**。同一道 `checkBackfillSearchDateRange`、两个调用点、
   覆盖完全相反（基线 0 红）：**删生产路径 `backfill_fetch.go:316` ⇒ 红 0**；
   **删副本 `backfill_search.go:304` ⇒ 红 2**（`TestFetchBackfillSearchPageRejectsOutOfRangePublished`、
   `TestFetchBackfillSearchPageChecksAllColumnsDates`）。⇒ 守卫**被测得很好，只是测在不上线的那一份上**。
   🔴 **修法必须是消掉复制（让生产复用被测函数），不是补一条用例** —— 后者又要靠人维护同步。
2. 任一行标签大小写变化 ⇒ 计数守卫两边同步移动 ⇒ **静默吞条 + 邻条日期错位**
3. `exhausted` 出口不检查日期判停是否生效 ⇒ 站点自报 totalPages 小即**静默截断整趟回填**，退出码 0
4. 搜索侧 `total-records` / `total-pages` 从不互校 ⇒ pages 少报即**静默只翻第 1 页**
5. 协议相对外链被判为站内 ⇒ **静默抓下外站内容写进 manifest**
6. `--from ≥ 2025-09`（业务上永久成立）⇒ 交叉校验**永久关闭**，成因被记成「检索服务改版」
7. manifest 缺 `completed_at` / `cutover` / `to` / 关键词 / reconcile 摘要（可由 Leader 降级）
8. 三处契约订正：前缀省名穿透标题正则；「站外条目结构上匹配不到」为假；「站内外分界与 href 形态正交」为假

## 登记即可
第一轮 W-1/W-3/W-4/W-5 + S-1~S-6；第二轮 R2-9~R2-16。

## reason_class 建议：`task_defect`
逐条查过：**没有一条**是 DoD 自相矛盾或不可实现，故不是 `dod_defect`。
⚠️ 但值得记进 wisdom：九份 DoD 逐条验证全部通过，而上述八条**一条都没被任何 DoD 问到**
——它们是 **DoD 覆盖不到的空白**。

## 交付物复核结论（不返工的依据）
`~/hestia-backfill-2026-08-14/`：218/218 sha256 逐篇一致、与 151 页 index 快照及 82 页 search
快照**双向闭合**、80 期 0 未归类 0 缺篇、`manifest.title` 与文件内 `<title>` 218/218 逐字相同、
两个差集为 0 **非空集平凡为真**。⇒ 语料无需重抓。
