# 需求 ↔ DoD 追溯矩阵 — Sprint 032（M1a bitemporal）

**需求文档**：`hestia/docs/superpowers/plans/2026-08-07-macro-bitemporal.md`（1173 行）

## 一、文档原有 DoD（写在各测试文件头部的 Context Checkpoint）→ 任务映射

| 文档行号 | 原始 DoD 条目 | 落在 |
|---|---|---|
| :71 | NewSpec 接受合法形状 | 001 functional[0] |
| :72 | 非法标识符被拒 | 001 error_handling[0] |
| :73 | 业务键为空/重复/与 revision 重名 | 001 error_handling[2] |
| :74 | Key 键集不匹配 | 001 error_handling[3] |
| :75 | correlate 只产出 `col = alias.col` | 001 boundary[0] |
| :311 | 四种 Verdict 各自触发 | 002 functional[0] |
| :312 | 1 个 / 3 个版本时判定变化 | 002 boundary[0] |
| :313 | Verdict.String 可读 | 002 functional[1] |
| :658 | CurrentQuery 有修订时只返回最新行 | 004 functional[0] |
| :659 | AsOfQuery 返回时点行 | 004 functional[0] |
| :660 | 两套 Spec 形状行为一致 | 004 functional[1] / 005 functional[1] |
| :661 | 空表 → 零行 | 004 boundary[0] |
| :662 | 乱序插入后仍返回 revision 最大者 | 004 boundary[1] / 005 boundary[1] |
| :663 | 单列业务键可用 | 004 boundary[2] |
| :664 | SQL 不含字面量，只有 `?` | 004 error_handling[0] |
| :875 | Lookup 命中/未命中/多版本 | 005 functional[0] |
| :877 | 空表 → Exists:false 且非 error | 005 boundary[0] |
| :879 | Key 不匹配 | 005 error_handling[0] |
| :880 | 零值 Spec 报错 | 005 boundary[2] |
| :881 | SQL 错误被包装且含表名 | 005 error_handling[1] |
| :882 | `*sql.Tx` 满足 Querier | 005 functional[2] |

**孤儿检查**：文档 Context Checkpoint 共 21 条，**全部有落点**。

## 二、Global Constraints（:13-25）→ 覆盖

| # | 约束 | 覆盖它的 DoD |
|---|---|---|
| C1 | 不新增依赖 | 001 non_functional[1]、**006 functional[1]（终验）** |
| C2 | 标识符正则 | 001 error_handling[0] |
| C3 | 取值走 `?` | 004 non_functional[0]、005 non_functional[0] |
| C4 | 不导出写操作 | **006 functional[0]（`go doc` 核对）** |
| C5 | 真实 SQLite + `t.TempDir()` | 003 functional[1] |
| C6 | Context Checkpoint 注释 | 各任务 non_functional 均有 |
| C7 | 跑真 SQL 不断言字面量 | 004 全部 functional（**唯一例外见 004 error_handling[0]**）|
| C8 | 两套形状并行 | 003 functional[0]、004 functional[1]、005 functional[1] |
| C9 | 中文注释解释「为什么」 | 各任务 non_functional |
| C10 | 不触及既有包 | **006 functional[2] + boundary[0]（`detect_changes`）** |

**10 条全部有落点**，其中 C1/C4/C10 集中在 Task 6 终验。

## 三、Leader 加的五条（design-spec T1-T5，**文档中无对应**）

> 这些**不是凭空 DoD**——依据是 `.arcforge/docs/01-design/design-spec.md`，
> 素材来自 Sprint 031 的教训。列在这里以便反审者单独判断。

| 编号 | 加强项 | 落在 | 为什么加 |
|---|---|---|---|
| **T1** | 纯函数守护须配行为守护 | 004 functional[2]、005 error_handling[2] | `correlate`/`checkKey` 的测试断言的是**返回值**，不证明 Query/Lookup **用了**它们。Sprint 031 的 tushare `callKey` 缺口同形 |
| **T2** | 注入用例不能只断言 `require.Error` | 001 error_handling[1] | 任何原因的 error 都满足它；若 NewSpec 因别的原因先行报错，注入那条照样绿 |
| **T3** | `bothShapes` 失败须能指出哪套形状 | 003 boundary[0] | 两套共用断言时，红了不知道是哪套的问题 |
| **T4** | 「无字面值」是否定断言，须造变异证明它能红 | 004 error_handling[0] | 未实现时天然成立 |
| **T5** | 零值 Spec 在 **Query 侧**也要钉住 | 004 boundary[3] | 文档只在 Lookup 侧有 `TestLookupRejectsZeroSpec`，Query 侧未提 |

**另加的三条同族**（未编号，同一逻辑）：
001 functional[1]（切片复制的变异判据）、002 boundary[0]（`<` vs `<=` 须恰好只红一条）、
002 error_handling[0]（`Exists=false` 时判定与 revision 无关）。

## 四、机器检查结论

- **孤儿需求**：0（文档 21 条 Context Checkpoint + 10 条 Global Constraints 全部有落点）
- **凭空 DoD**：0（加强项均可溯至 design-spec，且已在本矩阵单列）
- **validator**：✓ 6 个任务通过（DAG 无环、wave 序、scope 非空且互斥）

> **独立 reviewer 的反审结果待补**——它只读需求文档、不看本矩阵，结论到达后附在此节之后。
