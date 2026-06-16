# 需求分析 — Qlib 数据仓库 第一期

> 来源: `docs/superpowers/plans/2026-06-15-qlib-data-warehouse-phase1.md`
> 关联 spec: `docs/superpowers/specs/2026-06-15-qlib-data-warehouse-design.md`

## 目标
建立本地 SQLite 历史行情仓库，让 atlas 的 `FetchHistory` 以**仓库为权威主源**、外部 API 仅补新鲜尾巴，降低对 Yahoo/eastmoney 的实时依赖。**完全可降级**：缺库/关库时系统行为与现状完全一致（零回归）。

## 功能模块
1. **Python dump 管线**（仅 stdlib）— 读 `qlib_csv_*` per-instrument CSV → 归一化 → 原子写统一 SQLite。
   - schema 模块（DDL：`ohlcv` / `fundamentals_pit` 空表 / `warehouse_meta`）
   - ingest（CSV 解析归一化，`adj_close=close*factor`，symbol 大写）
   - writer（临时库 + `os.replace` 原子覆盖，写 `warehouse_meta`）
   - build CLI + Makefile `warehouse-dump` target
2. **Go qlib collector**（`internal/collector/qlib`）— 只读打开 SQLite，实现 collector 接口。
   - skeleton + `Covers`
   - `FetchHistory` 仓库主源读取（区间 `[start, min(end,last_date)]`）
   - 补新鲜尾巴 + 陈旧度告警 + 外部失败降级
   - `FetchQuote` / 非日频 → 委托外部源
3. **路由集成** — selector 优先 qlib（命中即用），`SelectExternalForSymbol` 永不返回 qlib（避免补尾递归）。
4. **装配** — `QlibConfig` 配置、`App.CollectorRegistry()` 导出、serve.go 可降级装配。

## 技术要点
- Python 3.11 stdlib（`sqlite3`/`csv`），pytest；统一走 `scripts/qlib_eval/.venv/bin/python`。
- Go 1.24，`modernc.org/sqlite` 纯 Go 驱动（无 cgo）；只读打开 `?mode=ro`。
- 原子性：dump 写临时库再 rename，atlas 不会读到半成品。
- 可降级：缺库/外部 API 失败均不报错、回落仓库段或外部源。

## 复杂度评估
整体**中等**。单个任务均为 simple（计划已给出测试与实现代码，确定性高）。风险点：
- T9 selector 重构需保零回归（既有路由测试必须全绿）。
- T12 装配跨多个既有文件，需正确 import 与 registry 暴露。

## 范围边界（本期不做）
- Part B PIT 基本面源（`fundamentals_pit` 建空不填，Go 不读）→ 第二期。
- A 股/港股 dump（Makefile target 仅接 US）。
- 实时/分钟频入库（始终委托外部源）。
