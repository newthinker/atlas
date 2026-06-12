# 设计规格 — sprint-004 自建 qlib 数据包

> **权威施工图 = plan rev4**（docs/plans/2026-06-12-qlib-data-bundle-implementation.md，含完整测试代码/实现骨架/评审修订），
> **权威设计 = spec rev4**（docs/superpowers/specs/2026-06-12-qlib-data-bundle-design.md）。本文件只记任务图重组。

## plan Task → arcforge 任务映射（4 任务）

| arcforge | plan | packages | deps | wave | 要点 |
|----------|------|----------|------|------|------|
| TASK-001 | T1 | ./cmd/atlas | — | 1 | export-ohlcv 核心：makeOHLCVBars+fakeOHLCVProvider 新 helper、golden 互锁、toQlibInstrument 契约（含文件名派生）、非 A 股拒绝/基准失败 fatal/空集报错（resolver 形态）、300ms 注入 |
| TASK-003 | T3 | ./scripts/qlib_eval | — | 1 | build_data.py：mock subprocess 断言 --data_path/--exclude_fields symbol,date、date_span_from_csvs、verify_bundle（tab 三字段只读校验）、空 CSV 拒绝、失败透传 |
| TASK-002 | T2 | ./cmd/atlas + ./scripts/qlib_eval | 001,003 | 2 | cobra 接线（config 参照 serve.go:55-66）+ CLI 层基准校验测试 + UsageListsAllFlags + Makefile qlib-data target（--symbols $(SIGNAL_SYMBOLS)，只传 --from）+ test_makefile 泛化 _target_block |
| TASK-004 | T4 | ./scripts/qlib_eval | 002,003 | 3 | QLIB_DIR 默认切 atlas_cn + README（建包/复权口径/crontab/evaluate 直调注意）+ **e2e 必跑**（make qlib-data → D.features 抽查首尾数值对照 CSV → make signal-eval 非空结果表） |

调度说明：001 与 003 无 scope 交集可并行（wave1）；002 跨两 scope 必须等两者 verified；004 终局收口。

## 与 sprint-003 的验收差异

e2e 真实运行从「可选」升为**必跑 DoD**：qlib 数据包/采集网络/venv 全部已验证可用，且「signal-eval 默认区间非空结果」正是本需求存在理由（plan 验收对照最后一条）。
