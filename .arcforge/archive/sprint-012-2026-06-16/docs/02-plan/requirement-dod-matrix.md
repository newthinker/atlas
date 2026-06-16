# 需求 ↔ DoD 双向追溯矩阵 — 第二期 Part B PIT

## 正向：需求 → DoD 覆盖（无孤儿需求）

| 需求条目（计划 DoD/范围） | 覆盖任务 | 关键 done_criteria |
|---|---|---|
| 基本面 CSV 摄取（必填+可选列，空→None，大写） | T1 | parse_dir 解析 + None 边界 |
| writer 同次原子写 fundamentals_pit，向后兼容 | T2 | 3 行 fundamentals + 不传时 0 行兼容 |
| 修订（同 report_period 不同 observe_date）保留不去重 | T2,T4 | T2 持久化保留 + T4 升序返回 |
| build CLI --fundamentals-dir | T3 | eps_ttm==3.0 + 缺目录 return 3 |
| PIT 点对时间查询防前视 | T4 | observe_date>end 不返回 |
| EPS 升序 + EPSPoint 映射 | T4 | KeepsRevisionsOrdered 4.0/4.2 |
| 仓库有→主源，无→委托 yahoo，nil→空 | T5 | 三契约钉死 |
| 仓库有数据优先于 fallback | T5 | PreemptFallback called=='' |
| serve 装配 qlibpit 包装 yahoo | T6 | db!=nil 注入 + db==nil 纯 yahoo |
| SetValuationSources 在 Start 前（S1 不变量） | T6 | boundary 顺序约束 |
| 第一期装配单测零回归 | T6 | wiring 四分支同步更新全绿 |
| Python 全绿（含第一期） | T1-T3 | 各 non_functional pytest |
| Go 全绿 qlibpit + cmd | T4-T6 | 各 non_functional go test |
| make warehouse-dump 有/无 fundamentals 均成功 | T7 | wildcard 守卫 + 无目录冒烟 |
| 仓库未启用零回归 | T6 | db==nil 维持纯 yahoo |
| 各市场适配器文档化 | T7 | ADAPTERS.md 三市场 |

## 反向：DoD → 需求来源（无凭空 DoD）
逐任务核对：T1-T7 全部 done_criteria 对应计划某 Task 的 Step 期望测试断言或 DoD/范围章节，无凭空新增。T6 的「wireQlibWarehouse 暴露 db + 装配单测零回归」对应第一期现实适配（架构决策 AD-5），非凭空。

## 机器检查结论
- 孤儿需求：**0** ｜ 凭空 DoD：**0**
- best-effort 范围（适配器实际生产、美股近似 observe_date、PB/PS/ROE 入库不消费）已标注本期不做/best-effort，不产生强 DoD（T7 用 verify_by:manual）。
