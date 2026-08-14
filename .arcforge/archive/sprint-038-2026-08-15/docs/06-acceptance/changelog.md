# Changelog · Sprint M1c-1

交付树 `abaca10`。面向下游（M1c-2）与未来改这段代码的人。

## 新增能力

**`atlas hestia backfill fetch`** —— 把 PBOC 历史报告快照抓到本地磁盘 + 一份 manifest。**不解析、不入库。**

```bash
atlas hestia backfill fetch --from 2020-01 --out ~/hestia-backfill-2026-08-14
```

| flag | 说明 |
|---|---|
| `--from`（`YYYY-MM`） | ⚠️ 语义是**发布日期**，不是期次。2019 年度报告发布于 2020-01，`--from 2020-01` 会含入它 |
| `--out` | **必填、无默认值** —— 不设默认是为了让「误落进仓库」需要显式打出来才会发生 |
| `--expect-periods` / `--expect-articles` | 可选，**不传即零值**，走推算值告警路径（不是「默认某个数」） |

**抓取策略**：index 翻页为主 + 站内搜索交叉校验，抓取集 = 两侧并集，两个差集都进 manifest。断点续抓（已在 manifest 的不重发请求）。

## manifest 契约

```jsonc
{
  "completed_at": "...",        // 缺失 ⇒ 这趟回填夭折了
  "cutover": "2025-09",         // 产出它时实际用的值，别猜
  "to": "...", "keywords": [...],
  "search_skipped_reason": "",  // 非空 ⇒ 交叉校验未生效，两个差集为空不代表两侧一致
  "articles": [ { "id","title","published","url","file","sha256","fetched_at","source" } ],
  "failed":   [ { "id","url","error" } ]
}
```

⚠️ **`~/hestia-backfill-2026-08-14/manifest.json` 早于本 schema 变更，不含前四个字段** —— 那是预期的，**cutover 按 `2025-09` 理解**。**没有为补字段去重跑**（产物经全量复核合格且不可再生）。

## 🔴 使用者必须知道的三条边界

1. **交叉校验只在 `goutongjiaoliu` 栏目内成立**，是「同栏目不一致」检测，**不是完整性检测**。搜索侧另有 263 条带报告标题的条目（`/diaochatongjisi/` 206 + `/taiyuan/` 57）按设计丢弃 ⇒ **某期若整体缺在本栏目而存在于调统司，两个差集都不会报**。接住它的是逐期结构对账（`MissingPeriods`）。
2. **标题正则只挡住「中缀」省名**（`2020年11月厦门市…`），**前缀省名会穿透**（`山西省2024年8月…`）。真正挡住分行报告的是**栏目前缀筛** ⇒ **若下游改去看调统司栏目，那里没有这层保护。**
3. **`search_skipped_reason` 非空时，`only_in_index` / `only_in_search` 为空不代表两侧一致** —— 那是「校验没做」。stdout 报告会在表头之后、逐期明细之前显式说明这一点（TASK-011）。

## 本次真跑的产物

```
~/hestia-backfill-2026-08-14/    452 文件 / 15M
80 期 / 218 篇 / 0 缺篇 / 0 未归类     rule@v1→v2 切换点 2025-09
两个差集均为 0，且非空集平凡为真（218 条全部 source=both）
manifest sha256  e824866e2f992b221842e432e1e93ce795a1b5765f0a4e16ae72712e24d03a40
抓取窗口 2026-08-14 23:18:58 → 23:27:22 CST，当时栏目总页数 409
```

🔴 **不可再生**：央行站点两天内 p146 掉了 2 条。**重抓拿不到同一份快照，请单独备份。**

## 契约文档

`internal/hestia/CONTRACTS.md` 的 **Sprint M1c-1** 一节，**7 组 20 条**，每条标注来源与证据类型（实测 / 推导 / 已知未修）。改这段代码之前值得先读 **C 组（实现陷阱）**与 **D 组（方法学陷阱）**。
