# 部署说明 — Qlib 数据仓库 第二期（Part B PIT）

## 特性开关（复用第一期 qlib 开关，完全可降级）
本期不新增配置项。沿用第一期 `qlib` 块；PIT EPS 源仅在 `qlib.enabled=true` 且库可读时自动启用，否则 EPS 源维持纯 yahoo（零回归）。

## 启用步骤
1. **准备基本面 CSV**（best-effort，可选）：按 `scripts/qlib_warehouse/ADAPTERS.md` 契约产出 `fundamentals_csv_us/<symbol>.csv`（表头 `symbol,report_period,observe_date,eps_ttm,...`）。无此目录则仓库只含 OHLCV，EPS 自动回落 yahoo。
2. **构建仓库**（离线，无 live DB 迁移）：
   ```bash
   make warehouse-dump   # 有 fundamentals_csv_us/ 则一并写 fundamentals_pit；无则只写 OHLCV
   ```
   `$(wildcard)` 守卫：目录不存在自动省略 `--fundamentals-dir`，dump 不报错。
3. **配置 + 重启 serve**（同第一期）：`qlib.enabled: true` + `db_path`。启动日志出现 `qlib PIT EPS source enabled (yahoo fallback)` 即 PIT EPS 生效。

## 无需的操作
- ❌ 无新增配置项（复用第一期 qlib 块）。
- ❌ 无 live DB schema 迁移（fundamentals_pit 表第一期已建；仓库离线重建）。
- ❌ 无新增依赖（modernc.org/sqlite 第一期已引入）。

## 回滚
- 删除/不提供 `fundamentals_csv_us/` 重跑 dump → 仓库无 fundamentals → EPS 自动回落 yahoo。
- 或 `qlib.enabled: false` → EPS 源回到纯 yahoo（与第一期前现状一致）。

## 运维提示
- PIT 语义：某符号一旦入库 fundamentals，EPS 即走仓库 PIT（不再退报告期末对齐的 yahoo）；空结果由下游委托 lixinger 兜底。
- 仓库损坏/observe_date 非法格式当前为静默降级（QA follow-up 建议加 warn 日志，见 final-report §8）。
- 定期重跑 `make warehouse-dump` 刷新（原子覆盖，serve 不读半成品）。
