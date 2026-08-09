# Sprint 033 进度 —— M1b-1 `internal/hestia` types + store

**分支**：`feat/hestia-store`（基于 `origin/master` `d7c9c69`，含已合并的 M1a）
**状态**：DoD 定稿，等待人类确认门（`autonomy=dod-gate`）

## 任务

| ID | wave | 任务 | DoD | 依赖 | 状态 |
|---|---|---|---|---|---|
| TASK-001 | 1 | 字段常量与唯一真相源 (fields.go) | 7 | — | pending |
| TASK-002 | 1 | 类型与 Meta 校验 (types.go) | 8 | — | pending |
| TASK-003 | 2 | 由字段清单生成 DDL (schema.go) | 8 | T1 | pending |
| TASK-004 | 3 | NewStore 与建表 (store.go 骨架) | 7 | T3 | pending |
| TASK-005 | 4 | Save 输入校验与 INSERT 路径 | 11 | T4,T2,T1 | pending |
| TASK-006 | 5 | Duplicate UPDATE 与 pending 分流 | 9 | T5 | pending |
| TASK-007 | 6 | 收尾验证（无生产代码） | 6 | T1–T6 | pending |

**并行度低是结构性的**：T4/T5/T6 同写 `store.go`，必须串行。真正能并行的只有 T1‖T2。

## 独立 reviewer 反审结果（已全部采纳）

reviewer 用真实 SQLite 实测而非推断，判定「有缺口，不建议原样放行」。5 条必补 + 3 条含糊已全部修入 DoD：

| | 内容 | 不补会漏掉什么 |
|---|---|---|
| G1 | `published_at`/`period` 形态校验 | M1a `lookup.go` 包注释明文指派给写入方的契约，当前无人履行；少补零→当前行视图长期返回旧版本，无报错 |
| G2 | pending 路径 `Verdict != New` 用例 | `bitemporal.New == 0`，需求文档自带断言在「早早 return `Outcome{}`」下同样通过——证明不了它想证明的事 |
| G3 | Duplicate 携带不同 Values 的行为 | rule@v1→v2 回填时每期都判 Duplicate，34 个新字段静默不写入，Save 返回 nil |
| G4+G5 | 从真实库读 PRAGMA 断言 PK/列类型 | 删掉 `PRIMARY KEY` 整行，需求文档全部测试仍绿；业务列建成 TEXT 时读回测试也全绿而 `MAX()` 按字典序算错 |
| G10 | 拒绝自相矛盾的 ValidationReport | `Passed=true` 却含 failed check → 坏数据进权威表且不留痕迹 |

含糊条目 A1/A2/A3 已改写。**A1 是 Leader 自己写错的**：「`fields.go` 之外任何文件不得出现字段名字面量」按字面执行必然 REJECT——T1 自己的分组测试就得写 `"tsf_"`。

## 待人类裁定

**T5（11 条）与 T6（9 条）超出 Realistic Scope 的 ≤8。** Leader 判断：超出源于 reviewer 补充的关键判据密度，而非范围膨胀（T5 实际只有 `Save`+`insert` 两个函数）。拆开会让两个任务改同一个函数。建议保持不拆，请人类确认。
