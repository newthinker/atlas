# 进度 · Sprint M1c-4（hestia 回填收尾与 M1c 关闭）

**状态**：🟢 wave 1 开发中（两 dev 已到 GREEN，等提交 → Leader merge）
**采样锚**：`4670ccbe0abd703f86b1e0c53aef8d3c86cc512d`（master）

## 阶段

- [x] Step 1–3 环境 / 需求分析 / 任务拆分 + DoD + 追溯矩阵 + validator
- [x] Step 4 **dod-gate 已通过**
- [ ] **Step 5 并行开发** ← wave 1 在途
- [ ] Step 6 QA ｜ [ ] Step 7 交付归档

## 任务状态

| TASK | wave | 状态 | owner | 备注 |
|---|---|---|---|---|
| TASK-001 | 1 | 🟡 `in_progress` | dev-m1c4-c | 代码完成、测试绿；**分支 0 commit**，待补 writes/discovery 后提交 |
| TASK-004 | 1 | 🟡 `in_progress` | dev-m1c4-b | 已到 GREEN；**分支 0 commit**，scope 越界待申报（12 文件 vs 声明 8） |
| 002/003/005/006/007/008/009/010/011/012/013 | 2–10 | `pending` | — | DoD 已按两轮独立反审加固 |

## 🔴 两轮独立反审的产出（两个 reviewer 独立跑，结论交叉验证）

**共同结论（不同证据、不同例子，r2 还主动推翻了自己最初的例子）**：TASK-011 的 `|ytd| ≥ |mom|` **只覆盖双侧**，而路由错误最典型的产物是**单侧** ⇒ 防线在最需要它的地方失明。

已落盘的 15 处 DoD 加固：

| TASK | 加固内容 |
|---|---|
| 001 | Step 6 判据订正（`2023-05` 移动必然带动汇总计数行）；NBSP 必须真的进测试输入 |
| 002 | 补**全量 diff**（`sectorPeriodRE` 由 `periodPat` 编译、`selectRMBCumulativeFlow` 查 `cumulativePeriods` ⇒ 加前缀影响全语料） |
| 005 | 🔴 **合计句也必须路由**（孤儿字段）；🔴 **形态②会整篇失败**（`tsfFlowScopeEnd` 语义）；`selectRMBCumulativeFlow` 切法与新失败模式；golden 扩到形态覆盖（testdata 已有现成 fixture）；逐篇差集必须为空 |
| 006 | 期望数组顺序订正（`FieldM2` 在 `fields.go:132` < `DepositHouseholdYTD` 在 `:135`）；`Reason` 实际格式 + `Value: &n` + `firstN` 必须保留 |
| 007 | `corp_loan_reconcile` 一条测试都没有；它的 mom 容差**无标定依据**；单测绕过抽取 ⇒ 需端到端验证 |
| 008 | 🔴 **mom 族取不到同族 drift 基线 ⇒ 恒 skip**（设计必然后果，需在交付报告单独点出）；`n` 截断分支从未执行；`PrecedingAll` 确会撞红导出面守卫（已核实） |
| 009 | `renderCalibrateReport` 实际签名（文档代码编译不过）；`ytd\s+2\s` 正则与样例不相容 ⇒ **永不命中**；导出字段不被守卫覆盖 |
| 010 | 🔴 `LoadConfig` 全覆盖校验从条件从句改为**无条件义务**（`TestMagnitudeRangesCoverEveryFieldWhenCalibrated` **恒 skip**，`thresholds_test.go:98` 钉死默认表为空） |
| 011 | 🔴 报告印**四个数** + 判据加「单侧数 ≠ 总数」；印出实际比较过的字段对；**跨语料族内不等式**（唯一能抓单侧盲区的自动判据）；`ytd == mom` 合法性断言 |
| 012 | `grep -c "缺"` 是坏仪器；「显著下降」需可判决的裁决方法 |
| 013 | 🔴 抽样**恰好避开唯一无闸区**（住户贷款 `totalField: ""`）⇒ 须含 mom 期次 + 贷款分部门锚字段；**口径本身是核对项**；回滚漏 `-wal`/`-shm` |

## 🔴 Leader 自查出的两个失误

1. **把需求文档一个错误的「为什么」原样抄进 TASK-001 DoD**（`(?m)^ +` 的 `+` 是贪婪的，文档举的例子是它自己的反例）。真实理由是字符集不同（NBSP **与 tab** 都会泄漏）。由 dev 的 code-simplifier 用**换序变异实测**证伪。已记 `wisdom/learnings-leader.md`。
2. **拆分时低估了 `nameField` 的消费面**：改其字段名会编译级波及 `extract.go` / `required.go` / `schema_test.go` ⇒ TASK-004 实际改 12 文件而声明 8。已要求 dev 越界申报。

## 待办（Leader）

- [ ] 两 dev 提交后**串行 merge** 进 master（merge 必须先于 `dev_done`）
- [ ] 收 r2 的 §4 风险排序，与 r1 的比对分歧
- [ ] wave 2 放行 TASK-002
- [ ] TASK-010 / TASK-013 的 `questions[]` 呈人类裁决

## 环境问题

两 dev 各三次 `API Error: Connection lost mid-response`（`reason_class=env_infra`，**不计 `rework_count`**）。已要求缩短回复长度。
