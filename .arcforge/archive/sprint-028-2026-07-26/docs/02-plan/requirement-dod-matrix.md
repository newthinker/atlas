# 需求 ↔ DoD 双向追溯矩阵（Prism M3）

需求源：`docs/superpowers/plans/2026-07-25-prism-m3.md`（12 个 plan task）
任务源：`.arcforge/tasks/TASK-001..015.json`（15 个 arcforge task）

## 1. 正向追溯：plan task → arcforge task（有无孤儿需求）

| plan Task | 需求要点 | 承接 arcforge task | 覆盖 |
|---|---|---|---|
| T1 | EPS/shares/equity tag 回退链 + EPS 推算回填 | TASK-001 | ✅ |
| T1 | refresh Degraded/Failed 明细始终打 stdout | TASK-004 | ✅（拆出，跨包） |
| T1 Step5 | 真实 EDGAR 手动验证（COST/V/CRM/WMT/AVGO） | TASK-015 boundary（验收记录） | ⚠ 见备注 A |
| T2 | fundamental_q 扩 5 列 + 存量库迁移 | TASK-002 | ✅ |
| T2 | QuarterlyFact 扩 5 字段 + 主干流 tag 链 + 推导 | TASK-001 | ✅（与 T1 合并同包） |
| T2 | refreshEdgar 落库映射补新字段 | TASK-006 | ✅ |
| T3 | segment_revenue 表 + 3 方法 | TASK-002 | ✅ |
| T3 | price_daily 表 + UpsertPrices/PriceSeries | TASK-002 | ✅ |
| T4 | Template/Segment schema + LoadTemplates/LoadManualSegments | TASK-003 | ✅ |
| T4 | configs/prism/templates/msft.yaml | TASK-003 | ✅ |
| T5 | submissions + 实例文档分部解析 ⚠ | TASK-005 | ✅ |
| T5 Step5 | live 校验打印 member 清单 | TASK-014（并 Leader 已预先 live 验证 MSFT） | ✅ |
| T6 | RefreshSegments（增量/映射/Q4 推导/manual 覆盖） | TASK-008 | ✅ |
| T6 | closes 落 price_daily | TASK-006 | ✅ |
| T6 Step5 | cmd 接线 + Report 合并 | TASK-010 | ✅ |
| T7 | BuildPeriods/DefaultSelection/BuildMatrix/BuildSankey | TASK-007 | ✅ |
| T8 | sankey Service（Analyze/Fundamental） | TASK-009 | ✅ |
| T8 | 两个 JSON API + 路由 + serve 装配 | TASK-011 | ✅ |
| T9 | /prism/sankey 页面（网格/矩阵/切换/导出） | TASK-012 | ✅ |
| T10 | 首批模板 5~10 家 + live 校验 + manual 兜底 | TASK-014 | ✅ |
| T11 | /prism/fundamental 页面 | TASK-013 | ✅ |
| T12 | 设计文档 D4/§7/§9 修正 + deployment.md | TASK-015 | ✅ |
| T12 Step2 | M3 七项验收 | TASK-015 boundary（分自动/人工两类） | ✅（AD-7） |

**孤儿需求检查：0 个**。plan 全部 12 个 task 的交付物均有承接任务。

> 备注 A：T1 Step5「config 临时含 COST/V/CRM/WMT/AVGO 跑真实 refresh，预期 degraded 数下降」
> 涉及修改用户运行时配置 + 真实网络刷新，属生产验证。归入 TASK-015 的验收记录条目，
> 按 AD-7 标注为「待人工验收」——agent 不应改用户 config 跑生产刷新。

## 2. 反向追溯：arcforge task → 需求（有无凭空 DoD）

| arcforge task | 需求依据 | 凭空? |
|---|---|---|
| TASK-001 | plan T1 回退链定义 + T2 tag 链/推导规则 | 否 |
| TASK-002 | plan T2 迁移代码 + T3 schema 与接口 | 否 |
| TASK-003 | plan T4 接口与 msft.yaml 全文 | 否 |
| TASK-004 | plan T1 `runPrismRefreshWith` 改法 + 测试代码 | 否 |
| TASK-005 | plan T5 解析规则 4 条（经 AD-2/AD-3 live 修正） | 否 |
| TASK-006 | plan T2 Step3 refreshEdgar 映射 + T6 Files 的 closes 落库 | 否 |
| TASK-007 | plan T7 五条测试清单 | 否 |
| TASK-008 | plan T6 RefreshSegments 五步语义 | 否 |
| TASK-009 | plan T8 service.go 接口 | 否 |
| TASK-010 | plan T6 Step5 | 否 |
| TASK-011 | plan T8 API 路由与 404 约定 | 否 |
| TASK-012 | plan T9 交互清单 | 否 |
| TASK-013 | plan T11 页面清单 | 否 |
| TASK-014 | plan T10 逐家流程与特殊结构处理 | 否 |
| TASK-015 | plan T12 Step1/2/3 | 否 |

**凭空 DoD 检查：0 条**。以下 DoD 虽非 plan 原文，但均可溯源到 Leader 调研发现的**既有代码事实**
（记录于 `01-design/architecture-decisions.md`），属必要补充而非凭空发明：

| 追加 DoD | 来源 | 理由 |
|---|---|---|
| TASK-002「迁移不损坏存量数据」「migrate 幂等」 | 风险登记（生产 prism.db 有数据） | plan 只断言列存在，未断言旧数据可读 |
| TASK-003「用 viper + mapstructure」 | AD-1（仓库无直接 yaml 依赖） | plan 自身规则「用 crisis config 同款库」的落实 |
| TASK-005「AD-2 一份报告多条 SegmentPeriod」「/Archives 大写」 | live 实测 | plan 原假设与真实结构不符 |
| TASK-006「A/H 链路不调 UpsertPrices」负向断言 | Global Constraints「零行为变更」 | plan 未给回归断言 |
| TASK-012/013「模板两处目录 + pages 两处列表」 | AD-4（既有双份目录机制） | plan File Structure 只列了 embed 一处，漏则生产启动失败 |
| TASK-012/013「JS 用 resp.data」 | AD-8（既有 compare 页缺陷） | 防止照抄有缺陷的既有写法 |

## 3. DoD 规模自检（Realistic Scope）

| task | DoD 条数 | 核心包数 | 预计文件数 | 合规 |
|---|---|---|---|---|
| TASK-001 | 7 | 1 | 4 | ✅ |
| TASK-002 | 8 | 1 | 2 | ✅ |
| TASK-003 | 8 | 1（+configs 产物） | 5 | ✅ |
| TASK-004 | 5 | 1 | 2 | ✅ |
| TASK-005 | 7 | 1 | 4 | ✅ |
| TASK-006 | 7 | 1 | 2 | ✅ |
| TASK-007 | 8 | 1 | 2 | ✅ |
| TASK-008 | 7 | 1 | 2 | ✅ |
| TASK-009 | 7 | 1 | 2 | ✅ |
| TASK-010 | 5 | 1 | 2 | ✅ |
| TASK-011 | 8 | 1（+2 接线包，AD-6） | 4 | ✅ |
| TASK-012 | 7 | 1（+2 接线/产物） | 5 | ✅ |
| TASK-013 | 8 | 1（+2 接线/产物） | 5 | ✅ |
| TASK-014 | 6（全 manual/review） | 产物任务 | 5 | ✅ |
| TASK-015 | 5（全 review） | 文档任务 | 3 | ✅ |

全部 ≤8 条 DoD、≤5 文件；核心包均为 1（接线型附带包按 AD-6 显式声明）。

## 4. 任务图与调度（validator 已通过：15 任务，0 错误）

```
wave1: 001(edgar)  002(storage)  003(sankey)  004(cmd)          ← 4 路并行
wave2: 005(edgar)  006(prism)    007(sankey)                     ← 3 路并行
wave3: 008(prism)  009(sankey)                                   ← 2 路并行
wave4: 010(cmd)
wave5: 011(api+server+serve)     014(configs, live) ⚠            ← 2 路并行
wave6: 012(web)
wave7: 013(web)
wave8: 015(docs)
```

**包互斥核验**（同 wave 或 dag 模式下可能并行的任务，packages 无交集）：

| wave | 并行任务的 packages | 交集 |
|---|---|---|
| 1 | edgar / storage-prism / sankey+configs-templates / cmd-atlas | 空 ✅ |
| 2 | edgar / prism / sankey | 空 ✅ |
| 3 | prism / sankey | 空 ✅ |
| 5 | api-handler+api+cmd-atlas / configs-prism | 空 ✅ |
| 跨 wave（dag 模式）014 与 011/012/013 可能重叠在途 | configs/prism vs 代码包 | 空 ✅ |

同包任务全部由 `dependencies` 串行：edgar(001→005)、sankey(003→007→009)、
prism(006→008)、cmd(004→010→011)、web(012→013)。
