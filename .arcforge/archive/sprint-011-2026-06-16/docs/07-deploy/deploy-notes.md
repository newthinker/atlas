# 部署说明 — Qlib 数据仓库 第一期

## 特性开关（默认关闭，完全可降级）
本特性默认不启用——不配置 `qlib` 块时行为与现状完全一致（零回归）。

## 启用步骤
1. **构建仓库**（离线，无 live DB 迁移）：
   ```bash
   make warehouse-dump   # 从 qlib_csv_us 生成 data/qlib_warehouse.db
   ```
   产物约 1.9M，20505 行 / 15 symbol。可通过 `WAREHOUSE_DB` / `QLIB_CSV_US_DIR` 覆盖路径。

2. **配置启用**（config 文件 `qlib` 块）：
   ```yaml
   qlib:
     enabled: true
     db_path: data/qlib_warehouse.db
     max_staleness_days: 7   # 0 → 默认 7 天，仅告警不阻断
   ```

3. **重启 serve**：日志出现 `qlib warehouse collector registered` 即生效。
   - 库缺失/不可读 → 日志 Warn 跳过注册，自动回落外部源（无需回滚）。

## 无需的操作
- ❌ 无 live 数据库 schema 迁移（仓库是独立 SQLite 文件，离线构建）。
- ❌ 无新增服务/端口/外部依赖（`modernc.org/sqlite` 纯 Go 编译进二进制）。

## 回滚
设 `qlib.enabled: false`（或删除 `qlib` 块）重启即可，瞬时回到现状路由。

## 运维提示
- 仓库陈旧（`last_date < now - max_staleness_days`）仅记 Warn，仍返回数据 + 外部补尾。
- 定期重跑 `make warehouse-dump` 刷新仓库（原子覆盖，serve 不读半成品）。
