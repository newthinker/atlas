# 需求 ↔ DoD 双向追溯矩阵 — qlib 数据仓库 第一期

## 正向：需求 → DoD 覆盖（无孤儿需求）

| 需求条目（计划 DoD / 范围） | 覆盖任务 | 关键 done_criteria |
|---|---|---|
| `make warehouse-dump` 从 `qlib_csv_us` 生成 `data/qlib_warehouse.db`，含 ohlcv+warehouse_meta | T1,T2,T3,T4 | T4: make 实跑打印 wrote N rows(N>0) |
| schema 三表 + ohlcv 主键 | T1 | apply 建三表 + PK(symbol,date) 冲突 |
| CSV 归一化 adj_close=close*factor，symbol 大写，缺 factor 退化 | T2 | 两条 functional + None 边界 |
| 原子写（临时库 + os.replace），warehouse_meta last_date | T3 | 无 tmp 残留 + 覆盖语义 + last_date 最大值 |
| CLI 入口 + Makefile target | T4 | main rc 语义 + make 冒烟 |
| Python 测试全绿 `pytest scripts/qlib_warehouse/` | T1-T4 | 各任务 non_functional pytest passed |
| Go 测试全绿 `go test ./internal/collector/... ./internal/config/... ./cmd/...` | T5-T12 | 各任务 non_functional go test |
| 仓库命中走仓库 | T6 | TestFetchHistoryReadsWarehouseRange |
| 超 last_date 补尾 | T7 | TestFetchHistoryAppendsTail |
| 补尾失败降级 | T7 | TestFetchHistoryTailFailureDegrades |
| 缺符号/缺库完全回落 | T6(缺符号 error),T9(回落路由),T12(缺库跳过注册) | error + SelectExternal + log.Warn skip |
| 陈旧度告警仍返回数据 | T7 | staleness warning boundary |
| FetchQuote/非日频委托外部 | T8 | TestFetchQuoteDelegates / NonDailyDelegates |
| selector 优先 qlib + SelectExternalForSymbol 永不返 qlib | T9 | Prefers/NeverReturnsQlib |
| 配置 QlibConfig | T10 | 字段 + mapstructure tag |
| serve 装配可降级 | T11(registry 暴露),T12(装配) | CollectorRegistry + Ping 失败跳过 |
| 关库/缺库零回归 | T9,T12 | 既有测试零回归 + Enabled=false 不装配 |

## 反向：DoD → 需求来源（无凭空 DoD）
每条 done_criteria 均可追溯到计划某 Task 的 Step 期望输出或「完成标准（DoD）」「范围边界」章节。逐任务核对：T1-T12 全部 done_criteria 对应计划原文测试断言或 DoD 列表项，无凭空新增。

## 机器检查结论
- 孤儿需求（无 DoD 覆盖）：**0**
- 凭空 DoD（不对应任何需求）：**0**
- 范围边界（Part B / A-HK dump / 实时入库）已显式标注为本期不做，不产生 DoD。
