# 需求 ↔ DoD 双向追溯矩阵 · M1c-2

需求来源：`hestia/docs/superpowers/plans/2026-08-24-hestia-calibrate.md`
DoD 来源：`.arcforge/tasks/TASK-00{1..4}.json`（共 28 条）

## 一、需求 → DoD（查孤儿需求）

### Global Constraints（9 条）

| # | 需求 | 覆盖它的 DoD | 备注 |
|---|---|---|---|
| G1 | Go 1.24.4，包 `internal/hestia` | 全部任务的 `packages` 声明 | 结构性，非断言 |
| G2 | **无新增依赖** | ⚠️ **无 DoD 覆盖** | 见「二、孤儿需求」 |
| G3 | 工具只产出依据，**不改 `configs/hestia.yaml`** | ⚠️ **无 DoD 覆盖**（T1 改它，但那是阈值改造不是标定产物） | 见「二」 |
| G4 | `CompletedAt` 缺失 ⇒ 报错，除非 `--allow-incomplete` | T2 `boundary[0]`、T4 `boundary[1]` | ✅ 两侧都有 |
| G5 | **社融两篇不计入失败表** | T2 `functional[1]` | ✅ 含刻意构造（文件不存在） |
| G6 | 缺键 ⇒ `skipped` 不是 `failed`；`DefaultThresholds`/`LoadConfig` 五种齐全 | T1 `functional[1]` + `boundary[1]` + `error_handling[0]` | ✅ 三条互补 |
| G7 | 注释引用任务编号带 milestone 前缀 | T1 `non_functional[1]` | ✅ |
| G8 | 每 task 结束 `gofmt -l`/`go vet`/`go test ./...` 干净 | 四个任务各自的 `non_functional` 末条 | ✅ |
| G9 | 测试文件 `import` 按需增补 | — | 实施细节，不需 DoD |

### 三条「写计划时核实的事实」

| # | 需求 | 覆盖它的 DoD |
|---|---|---|
| F1 | `--manifest` 改 `--dir` | T2 description（理由）+ T4 `functional[1]`（flag 经真实解析验证） ✅ |
| F2 | 改类型波及 9 处，1 处编译期抓不到（`configs/hestia.yaml:56`） | T1 `boundary[0]`（旧格式响亮失败）+ `non_functional[0]`（YAML 能装载） ✅ |
| F3 | `ingest_test.go:485` 注释会过期 | T1 `non_functional[1]` ✅ **并补上计划漏的 `validate.go:343`** |

### 设计决策节（6 条）

| # | 需求 | 覆盖它的 DoD |
|---|---|---|
| D1 | 两档阈值 + 不用「月均×间隔」 | T1 `functional[0]`（含 `monthly < annual` 断言） ✅ |
| D2 | 旧格式静默停用闸门 ⇒ 拒绝空 map、错误点名 `scalar` | T1 `boundary[0]`（含「`want` 用 scalar 不用字段名」+ 消融） ✅ |
| D3 | 装载齐全 vs 运行时容忍，两者不矛盾 | T1 `error_handling[0]`（两条各自单独红） ✅ |
| D4 | 标定只覆盖可解析字段 ⇒ **`n` 列不可省** | T3 `functional[1]`（含消融：删 n 列必须只红一条） ✅ |
| D5 | 建议区间用加性余量；`n<3` 不给建议 | T3 `boundary[0]`（负值样本 + 换乘性必红）、`boundary[1]`（0/1/2/3 四格） ✅ |
| D6 | 留给 M1c-3 的两件事 | — | 范围声明，非本迭代义务 |

## 二、🔴 孤儿需求（有需求、无 DoD）

| # | 需求 | 处置 |
|---|---|---|
| **G2** | **无新增依赖** | **补进 T2/T3/T4 的 `non_functional` 末条**：`go.mod`/`go.sum` 不得出现在任何任务的实际改动中（`git diff --name-only` 核） |
| **G3** | **工具只产出依据，不改 `configs/hestia.yaml`** | **补进 T4**：`Calibrate` 的实际改动文件不得含 `configs/`；T1 改 YAML 是**阈值改造**、与标定产物无关，两者别混 |

⚠️ 这两条都是**否定式约束**（「不要做 X」），最容易在 DoD 里缺席——**因为 DoD 天然是写「要做什么」的**。

## 三、凭空 DoD（有 DoD、无需求来源）

逐条回查 28 条，**发现 2 条**，均为 Leader 主动加，理由记此：

| DoD | 来源 | 是否保留 |
|---|---|---|
| T4 `functional[0]` 的「不得新增其它导出标识符」 | **不在计划里** —— 来自 M1c-1 的 `TestPackageExposesNoWriteFunctions`（AST 断言导出集合恰好相等），而**那个文件不在 T4 的 writes 里** | ✅ 保留：不写它，dev 会撞上一个自己改不了的红 |
| T4 `boundary[1]` 的「`--allow-incomplete` 须打印为什么允许」 | **不在计划里** —— 来自 Leader 需求分析阶段的定案（flag 名与计划注释语义不一致） | ✅ 保留：见 requirements-analysis 偏差 2 |

## 四、统计

```
需求项      21 条（G1-G9 / F1-F3 / D1-D6，其中 G1/G9/D6 非断言型）
DoD 条目    28 条 → 补两条孤儿后 30 条
孤儿需求     2 条（G2 无新增依赖、G3 不改 configs）← 待补
凭空 DoD     2 条（均有明确理由，保留）
```
