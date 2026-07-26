# 独立 reviewer 反审处置台账

**reviewer 设定**：只读需求文档 `docs/superpowers/plans/2026-07-25-prism-m3.md` 与现有代码，
**未读取 `.arcforge/`**（不看 Leader 已写的 DoD），独立提出应有的验收标准。
**产出**：52 条断言建议 + 边界清单 + 7 项高风险 + 6 项人工验收项 + 9 条技术矛盾。

## 交叉验证（reviewer 独立复现了 Leader 已发现的问题）

| reviewer 条目 | Leader 已有决策 | 意义 |
|---|---|---|
| §5.9 YAML 库选型需定死，倾向 viper | AD-1 | 双方独立得出同一结论 |
| 断言 23 / §5.1 `SegmentPeriod` 无法表达一报告多期 | AD-2（Leader 由 live 实测发现） | reviewer 从代码推理、Leader 从实测验证，互相印证 |
| 断言 47 `pages` 列表两处 + disk/embed 双模式陷阱 | AD-4 | 同上 |
| 断言 16 `PriceSeries` 未知 symbol 语义 | 已在 TASK-002 DoD | — |
| 断言 5 `UpsertPrices` 失败降级为 Degraded | 已在 TASK-006 DoD | — |
| 断言 22 仓库真实模板冒烟测试 | 已在 TASK-003 DoD | — |

## 采纳并升级为架构决策（新增，Leader 原方案的真实缺陷）

| reviewer 条目 | 处置 | 落地 |
|---|---|---|
| 断言 14 / §5.5 `segment_revenue` 用不可靠展示标签作 PK | **AD-9**：PK 改 `period_end`，`fiscal_period` 降为可空展示列；`LatestSegmentPeriodEnd` 不再 JOIN | TASK-002 / TASK-008 DoD |
| 断言 8 migrate check-then-act 并发竞态（serve + prism-daily 并发 Open） | **AD-10**：`duplicate column` 容错 + 并发 Open 断言 | TASK-002 DoD |
| 断言 25 「流式两遍」在单 HTTP 响应上不成立 | **AD-11**：改单遍解析，流末关联 | TASK-005 DoD |
| 断言 26 首跑请求量无上限（数十份 × ~10MB） | **AD-11**：回看上限 12 份 + 响应体 64MB 上限 | TASK-005 DoD |
| 断言 24 / 27 / 29 单位过滤、老式 filing 404、archives host UA | **AD-11** | TASK-005 DoD |
| 断言 31 / §5.6 模板迭代与增量锚点互斥（**阻塞 TASK-014**） | **AD-12**：`force` 参数 + `--full-segments` flag | TASK-008 / TASK-010 / TASK-014 |
| 断言 36 / 37 / §5.2 桑基缺残差节点，守恒断言在真实数据必失败 | **AD-13**：新增 `other_segment` / `other_opex` 残差节点 + 负值不画负流 | TASK-007 DoD |
| 断言 40 / 42 除零 → ±Inf → `json.Marshal` 报错 → API 500（**硬故障**） | **AD-14**：引擎侧返回 NaN + `jf` 扩展拦截 Inf（双保险） | TASK-007 / TASK-009 / TASK-011 DoD |
| 断言 43 / §5.4 期数上限三处不一致（10 vs 8 vs 全部） | **AD-15**：统一为 10 | TASK-007 / TASK-009 DoD |
| 断言 19 / 20 / 44 / §5.7 `LoadTemplates` 吞错、坏 YAML 静默、模板串号 | **AD-16**：三种目录情况区分 + 错误含文件名 + cik 校验 + 调用点不得丢 error | TASK-003 / TASK-010 / TASK-011 DoD |
| 断言 32 Q4 推导的 FY 行双重入账 | **AD-17**：FY 期不落库，年度值一律由引擎聚合 | TASK-008 DoD |
| 断言 2 / §3.1 tag 链改动波及拆股检测 → PE/PB/PS 全序列漂移 | **AD-18**：golden 逐字段回归 | TASK-001 DoD |

## 采纳为 DoD 条目（未升级为 AD）

| reviewer 条目 | 落地任务 |
|---|---|
| 断言 1 / 3 / 5 回退只在缺失时生效、不覆盖已有 EPS、Q4 不走推算 | TASK-001 boundary |
| 断言 12 / 13 负毛利不钳零、SG&A 单侧不求和 | TASK-001 boundary |
| 断言 9 迁移不损坏存量数据（行数 + 抽样值） | TASK-002 functional |
| 断言 10 新库与迁移后旧库 schema 收敛 | TASK-002 boundary |
| 断言 21 manual YAML 的 fiscal_period 键格式校验 | TASK-003 functional |
| 断言 30 单维但轴不匹配的 context 需排除（fixture 含干扰项） | TASK-005 functional |
| 断言 33 Q4 推导负值不落库 | TASK-008 functional |
| 断言 34 ±3 天容差命中 0 条 / ≥2 条的行为 | TASK-008 functional |
| 断言 18 manual 与 auto 反复刷新的收敛语义须定死 | TASK-008 functional |
| 断言 35 `Report.Refreshed` 合并后不得超过标的数 | TASK-010 boundary |
| 断言 38 `DefaultSelection` 四种边界 | TASK-007 boundary |
| 断言 39 非日历财年（MSFT 6 月结 / NVDA 1 月结）分组 | TASK-007 functional |
| 断言 41 CAGR 起点 ≤0 / 跨期 <2 → NaN | TASK-007 boundary |
| 断言 45 / 46 三种 404 文案不混淆、granularity 非法 → 400 | TASK-011 boundary |
| 断言 7 汇总行格式不变（运维脚本可能依赖） | TASK-010 non_functional |
| 断言 «configs/prism 相对路径在 go test 下不存在» | TASK-010 boundary |
| 断言 «fundamental_q 新列全 NULL 时不报错» | TASK-009 boundary |

## 降级 / 不采纳（含理由）

| reviewer 条目 | 处置 | 理由 |
|---|---|---|
| 断言 11 旧二进制读迁移后的库 | 降为 TASK-002 的 review 论证条目 | 需旧版二进制产物，超出本 sprint 投入；`ADD COLUMN` 对显式列 SELECT 的兼容性是 SQLite 契约 |
| 断言 9（真实 runtime prism.db 副本迁移）/ 52（valuation_daily 逐行 diff） | 移入 TASK-015 review 条目，在**副本**上验证 | Leader 已明令禁止 agent 触碰用户生产库（TASK-014 安全约束） |
| 断言 17 `UpsertPrices` 单次时长断言 | 只做幂等与行数断言，不做时长 | 时长断言在 CI 上不稳定，易成 flaky |
| 断言 48 / 50 部署投递与 ECharts sankey 模块检查 | 部分并入 TASK-012（`/static/echarts.min.js` 引用断言），部署说明进 TASK-015 | reviewer 已实测确认 vendored echarts 含 sankey/toolbox/dataZoom，风险已消解 |
| §5.8 测试同包访问 `s2.db` | 无需处置 | 已确认 `sqlite_test.go` 是同包测试 |
| 断言 51 Task 10 记录可复算证据 | 已在 TASK-014 discovery 要求中 | 已覆盖 |

## 人工验收项（AD-7，agent 不得声称通过）

reviewer 列出的 6 项与 Leader 判断一致，全部写入 TASK-015 的验收记录条目并将进入
`06-acceptance/final-report.md` 的人工验收清单：
分部数字与财报原文核对、中文分部译名、桑基视觉与统一比例尺可读性、PNG 导出/中英切换/dataZoom 交互、
股价叠加双轴量纲、真实 `atlas prism refresh` 的 degraded 数下降。
