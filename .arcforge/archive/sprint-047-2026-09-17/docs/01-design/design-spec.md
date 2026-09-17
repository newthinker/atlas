# M2b Sheets 投影 —— 设计规格（Arcforge 承载层）

**上游设计**：`superpowers/specs/2026-09-16-hestia-m2b-sheets-design.md`（子包边界、列定位、写入语义、失败语义均在那里，本文件不复述）。
**本文件回答的问题**：这 11 个任务在 Arcforge 里怎么承载、交付物长什么样、验证者拿什么判。

---

## 1. 交付物形态

| 类别 | 落点 | 门禁 |
| --- | --- | --- |
| Go 代码与测试 | `internal/hestia/`、`internal/hestia/sheets/`、`cmd/atlas/` | `dev_done` 门禁真实触发：声明包 `go test` + 覆盖率 |
| 依赖变更 | `go.mod` / `go.sum`（仅 TASK-006） | 门禁的漂移检查会看到；DoD 单列版本字面行 |
| 配置样例 | `configs/config.example.yaml` | 无 Go 门禁；DoD 用 `verify_by: review` |
| 契约文档 | `internal/hestia/CONTRACTS.md` 新开 `## Sprint M2b-2` | 同上 |
| vault 内容 | `docs/hestia-m2b/TASK-011-vault-content.md`（仓库内，供人粘贴） | 同上；实际写入 vault 是人执行（AD-M2b-2） |

## 2. 每个任务的 DoD 怎么从需求文档推导

需求文档每个任务是「Files + Interfaces + N 个 Step」。转换规则：

- **`functional`** ← 每个 `require.Equal` / `require.Contains` 里的**具体值**（不是"测试通过"，是"`Columns{"社融存量": 2, "月份": 0}` 这个值"）。验证者可以不看 dev 的测试、自己写断言对照
- **`boundary`** ← 需求文档里带 🔴/⚠️ 的段落和"预期：FAIL"的 RED 步骤（RED 是否真的红过，验证者可用 `git log -p` 核）
- **`error_handling`** ← `require.Error` + `require.Contains(err.Error(), ...)` 的错误文案
- **`non_functional`** ← 全局约束 C1–C11 中该任务直接触碰的条目（如 006 的 C1/C5/C7/C11，010 的 C8/C9）

## 3. 验证者的判定原料

每个任务 `verified` 前验证者至少要有：

1. 隔离 worktree 里 `go test ./<声明包>/... -count=1` 的原始输出（非缓存，判据一）
2. `git show --numstat <交付 sha>` 与 `writes` 声明的逐文件比对
3. 对 `functional` 每一条：自己构造输入、跑出输出、与 DoD 里的具体值比
4. 对写口守卫相关任务（001/002/003/004）：`TestPackageExposesNoWriteFunctions` 与 reflect 那条**都**绿，且白名单是精确集合相等（判据一）

## 4. 阶段边界

| 阶段 | 门 | 产物 |
| --- | --- | --- |
| DoD 定稿 | `dod-gate`：人类确认 | 11 个 `tasks/*.json` + 追溯矩阵 + reviewer 反审 + validator 绿 |
| wave 1 | 001 verified | `exportedFuncs` 递归 + 既有守卫清单未变 |
| wave 4 | 007 verified | dry-run 零写请求由 httptest 他证（判据三） |
| 全部 verified | QA 两轮 | 常规 + 跨视角对抗 |
| 交付 | `accepted` ×11 | final-report 含「待同步 hooks 清单」+ 人执行清单（TASK-012 六步 + vault 回写） |

## 5. 本轮刻意不做的

- 不建 TASK-012 任务文件（AD-M2b-3）
- 不把 §10 判据四的基线数字写进 DoD（AD-M2b-8）
- dev 不直接写 vault（AD-M2b-2）
- Leader 不手工并行放行有 scope 重叠的任务（AD-M2b-6）
