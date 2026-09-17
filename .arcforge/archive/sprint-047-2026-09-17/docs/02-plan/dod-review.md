# DoD 反审（独立 reviewer，只读需求文档与 tasks/*.json）· M2b

> 反审时点：11 个任务初稿定稿后、dod-gate 之前。结论 🔴 21 / 🟡 28。Leader 已按下表处置，处置后 validator ✓。

## 总评（reviewer 原话）
不建议直接进 dod-gate，先修三处：① TASK-004 选行规则写反（需求 D1「有 monthly ⇒ 用它」，DoD 写成选累计期次）；② `store_test.go` 是全 sprint 登记汇聚点，005–008 的 `writes` 漏列，列全又撞 scope-mutex——任务图结构问题；③ TASK-005 接口与需求不一致（`Current/Want`、`[][]any`、凭空 error、`AbsentInDB` 条件写窄破坏三类守恒）。

## Leader 处置表

| reviewer 发现 | 处置 |
|---|---|
| 004 选行反了 / `PublishedAt` 作用错 / 漏 0 值 / 35 列锚点不可验 / 漏 testdata 文件 | **全部按需求 line 771–1056 重写**；testdata `period-keys-2026-09-16.json` 进 `writes` |
| 005 接口不符 / `AbsentInDB` 窄 / 凭空 error / 漏真实差异夹具 | 接口逐字按需求；三类之和 == 行×列 写成不变量；error 改为「前置由 push 第 3 步保证」 |
| `store_test.go` 汇聚点 vs scope-mutex | **裁决：串行链** 001→002→003→004→005→006→007→008；005–008 `writes` 补全 |
| 006 漏依赖 005 / 漏 fake-sa.json / 漏 bodies==1、IsType float64 / 引号条件式 | 依赖加 005；`writes` 补两文件；断言补全；引号改无条件 |
| 007 签名少 `wantLabels` / 漏 RejectsUnknownLabel / 凭空 AJ 拒绝 | 补签名与测试；删 AJ 条 |
| 008 模板名与 index 规则未定 | 裁决：模板固定 `2024年`，index = 升序中第一个 > year 的位置 |
| 009 凭空「退出非零」与需求测试冲突 / 无测试缝 / 漏 flag 顺序与固定文案 / 投影范围未定 | 删退出非零（改退出 0）；加 `newSheetsClient` 变量缝 + `formatResult` 纯函数；补文案；裁决只推该期 |
| 010 漏 `cmd/atlas/hestia.go` 且与 009 撞 / 漏精确日志文案 / panic recover 伪 DoD | 接线挪给 009（010→009 串行）；文案逐字进 DoD；删 recover |
| 011 CONTRACTS 六项被换成对照表 | 六项逐条恢复，对照表标为 Leader 追加 |
| 003 漏 TrimSpace / Row 多了 PeriodType / 漏 53→BB / 注释 C6→C7 误引 | 全部修正；注释误引在 DoD 里明写「照抄时改 C7」 |
| 001/002 🟡（`.` 前缀、import 块不动、隐藏接收者、视图名、注释段） | 全部补入 |

## 需求文档自身的不一致（reviewer 列出，Leader 已裁决，验证者勿判假红）
1. 依赖表 vs Interfaces：006←005、009←004、005←004(row.go) —— 按 Interfaces 取全
2. Files 段漏 `store_test.go`/`testdata` 等 —— 按各 Step 的 `git add` 为准
3. 003/006 注释把 transport 误写为 C6（实为 C7）
4. 006 夹具 `Change{…Value(462.06)}` 非法 Go 且无 `Value` 字段 —— 按 005 的 `Want`
5. `Diff` 对未知 Label 未定 —— 前置由 push 保证
6. 009 测试需 client 注入 —— `newSheetsClient` 变量缝
7. 008 模板名/index、010 `CreateSheets`、009 投影范围 —— 见处置表
8. 011 Step 4 提交信息为空 —— `docs(TASK-011):`

## 统计
🔴 21 / 🟡 28，涉及 001–011 全部。处置后 DoD 5–8 条/任务，validator 21 规则 ✓。
