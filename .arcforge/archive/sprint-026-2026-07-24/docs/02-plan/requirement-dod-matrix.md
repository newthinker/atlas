# 需求 ↔ DoD 双向追溯矩阵(Prism M2)

> R 编号见 `01-design/requirements-analysis.md`。机器检查:每个 R 至少一个 TASK 覆盖(无孤儿需求);每个 TASK 的 DoD 均可回指某 R(无凭空 DoD)。

## 正向:需求 → 任务/DoD

| 需求 | 覆盖任务 | 关键 DoD 锚点 |
|---|---|---|
| R1 EDGAR 客户端与季度化解析 | TASK-003 | Request/Quarterization/IFRS 三测试;Q4 推导;tag 优先级(含回退子测试);修正重报去重(fixture 含旧 filed 条目) |
| R2 filing_date 防前视 | TASK-001, TASK-004 | FilingDateEffective 测试;refreshEdgar EPSPoint 携带 FilingDate |
| R3 fundamental_q 与 PB/PS | TASK-002, TASK-004 | UpsertFundamentalsRoundtrip;PB=40/8、PS=40/8 断言 |
| R4 /prism/compare 页 | TASK-006 | TestPrismComparePage;路由注册;≤8 选择上限(review) |
| R5 edgar 分派与 fallback engine | TASK-004 | TestRefreshEdgarFallback(Degraded 含 NVDA);无 fallback_source → Failed(不静默吞错) |
| R6 cmd 接线 + 20 家 CIK 清单 | TASK-005 | 构造注入;UA 缺失报错;CIK 官方映射比对(review);live smoke(manual) |
| R7 文档同步与 M2 验收 | TASK-007 | 数据源矩阵/§9/deployment.md(review);验收 7 条(manual) |
| N1 EDGAR 礼貌要求(UA 邮箱) | TASK-003, TASK-005 | UA header 断言;UA 缺失报错 |
| N2 AKShare 链路零改动 | TASK-001, TASK-004 | 全回归 non_functional 条目(collector/prism 用例不改即过) |
| N3 FilingDate 零值兼容零漂移 | TASK-001 | boundary:既有用例不修改即通过 |
| N5 US-GAAP 限定 | TASK-003 | ErrNotUSGAAP 测试 |

## 反向:任务 → 需求

| 任务 | 回指需求 | 备注 |
|---|---|---|
| TASK-001 | R2, N2, N3 | |
| TASK-002 | R3 | |
| TASK-003 | R1, N1, N5 | live 校验点移交 TASK-005 smoke(review 条目注明) |
| TASK-004 | R2, R3, R5, N2 | |
| TASK-005 | R6, N1 | manual smoke = 计划 Task 5 Step 4 |
| TASK-006 | R4 | 零新 API(AD-5) |
| TASK-007 | R7 | 纯文档任务,DoD 全部 review/manual(符合无代码任务声明) |

**检查结论**:无孤儿需求(R1~R7、N1/N2/N3/N5 均有覆盖;N4 为编码规范由 QA 审查兜底,不设独立 DoD);无凭空 DoD(每条 DoD 可回指)。

## 独立 reviewer 反审处置记录(2026-07-24)

dod-reviewer 结论 NEEDS-FIX 3 项,已全部采纳修正:
1. TASK-003 functional#3:修正重报去重改为要求 fixture 追加同 (fy,fp) 旧 filed 条目(原 fixture 唯一条目走不到去重分支,会成 fantasy assertion)。
2. TASK-003 boundary#2:Revenue tag 优先级补「第一优先 tag 缺失 → 回退 Revenues」子测试要求。
3. TASK-004 error_handling#2:新增「edgar 失败且无 fallback_source → Report.Failed,不静默吞错」。

不阻塞建议 2 条(Q4 qCount!=3 不推导、TTM 窗口 NaN 不产点)已作为「加强项(不阻塞)」标注进对应 DoD 条目文本,供 dev 自评。修正后 validator 重跑通过。

## Sprint 中新增需求(2026-07-24 用户决策)

| 需求 | 覆盖任务 | 关键 DoD 锚点 |
|---|---|---|
| R8 EDGAR 每股值拆股归一化(AD-9→批准入 Sprint) | TASK-008 | 比例跳变检测归一到最新基准;派生 Q4 无符号矛盾;shares 反向同步;无拆股零影响;NVDA smoke 复跑(manual) |

TASK-007 依赖已更新为 [005,006,008](验收基于归一化后数据),wave 5。
