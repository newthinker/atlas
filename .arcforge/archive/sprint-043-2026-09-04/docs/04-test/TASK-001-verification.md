# TASK-001 验证报告 · 配置 storage.snapshot_dir

- 验证者：test-m1d-a　判定：**VERIFIED（第 2 轮，epoch 2，rework 1）**
- 第 2 轮 verify_baseline.head：`abebb76057c6fb847ac11fd632fdd28b3b316d47`（master）；discovery sha256 与基线一致 `50e6f7de…2d50`
- 返工 commit：`40e81a7305a63a16df1d133eff01f0f97b4c8460`（父 `2f5ad513f7c0e7539c824e2e8f8a0f078baec316`；merge `abebb76`），只改 `internal/hestia/config_test.go` 17+/1-
- 首轮（REJECTED，task_defect，03:16:48Z）：基线 `2f5ad513…`，dev commit `a45ead0e671ec91732c36c43b7b795f703bac923`（父 `ae088eb253b64b36e10558a02587e3fa657f5f3e`）。首轮记录保留在 §5。
- 验证树：`git worktree add --detach ../wt-verify-TASK-001-r2-m1d abebb76057c6fb847ac11fd632fdd28b3b316d47`；变异在该树副本上做，每次后 `git checkout --` 还原并核实 porcelain 为 0；主仓库 `configs/hestia.yaml` sha256 前缀 `d4f72256b40f07a3` 与 `abebb76` 中一致，未动

## 1. 结论

首轮唯一缺陷（R-001 守卫恒真）已修：`TestShippedConfigLoadsAndIsCalibrated` 新增对**原始 yaml 文本**的断言（`os.ReadFile` + `strings.Contains("\n  snapshot_dir: data/hestia-snapshots\n")`），本轮判据「删 yaml 该行 ⇒ 该测试转红」独立复验成立；改值也转红。首轮已 PASS 的七条无回退（三组首轮变异回归仍 KILLED）。门禁全绿、覆盖率 96.4%、范围内无越界、提交锚匹配。

**成因两条并记**（按 DoD 订正）：① DoD functional[2] 的 R-001 补条给的正是那条恒真断言形态（`assert.Equal(cfg.Storage.SnapshotDir)`），Leader 采纳 reviewer 时未注意预填值 == yaml 值；② dev 首轮按字面落地，未对「这条断言在什么输入下会红」做删行核实。dev 的 discovery `key_findings[5]` 已如实并记两条。

## 2. Done Criteria 覆盖矩阵（第 2 轮）

| # | 完成标准 | 对应测试 / 证据 | 判定 |
|---|---|---|---|
| functional[0] | 字段、`defaultSnapshotDir`、`LoadConfig` 预填 | `TestLoadConfigDefaultsSnapshotDir` PASS；回归变异 M2（删预填行）⇒ FAIL | PASS |
| functional[1] | `var/snap` 原样保留 | `TestLoadConfigReadsSnapshotDir` PASS | PASS |
| functional[2] 前半 | yaml 行 + 四行注释、`config_version` 当日、头部记录 | `config.go`/`hestia.yaml` 相对首轮 `a45ead0` 未动（返工 numstat 只有 config_test.go）；首轮已核 | PASS |
| functional[2] R-001 + **本轮判据** | yaml 显式写出有测试守着；删 yaml 行该测试必须转红 | 树 `abebb76`：M3 删 `  snapshot_dir: data/hestia-snapshots` 行（`git diff --numstat` ⇒ `0 1`，文件内 `snapshot_dir:` 0 处）⇒ `--- FAIL: TestShippedConfigLoadsAndIsCalibrated`；M3b 改值为 `data/snap` ⇒ FAIL；原状 ⇒ PASS。新断言 `config_test.go:403-411`，用 `assert.True(strings.Contains(...))` 避免失败时打 20KB；原 cfg 字段断言保留并改写理由为「yaml 值与代码默认值不分叉」（这条理由成立，与新断言分工清楚） | PASS |
| boundary[0] | 空串拒绝 + `errors.Unwrap != nil` | `TestLoadConfigRejectsEmptySnapshotDir` PASS；回归变异 M1（删 validate case）⇒ FAIL | PASS |
| error_handling[0] | TDD 红留痕 | 首轮 discovery `tdd_red` 未变；返工是守卫加固，discovery `rework_1.mutation` 记了「交付态删行仍 ok（复现）→ 新断言删行 FAIL → 原状 ok」的红绿序列 | PASS |
| non_functional[0] 门禁 | gofmt / vet / 两包 / 覆盖率 / 五个不动文件 / 无新依赖 / 注释前缀 | 树 `abebb76`：`gofmt -l` 仅 `backtest_test.go`、`crisis_test.go`；`go vet` rc=0；`go test ./internal/hestia/... ./cmd/atlas/... -count=1` 两包 ok；`-cover` **96.4%**；`git diff --stat 4916106 abebb76 -- {store,validate,parse,extract,fields}.go` 0 行；`go.mod/go.sum/types.go` 相对 `ae088eb` 0 行；新增 import 仅标准库 `strings` | PASS |
| non_functional[1] 交付流程 | worktree / 提交锚 / code-simplifier / merge 先于 dev_done / 重采 / discovery | `fix(TASK-001): M1d …` 匹配 `^[a-z]+\(TASK-001\):`；merge `abebb76` 早于 `dev_done`（03:26:57Z）；discovery `verification.rework_1` 含 my_commit / merge / master_gates（锚 `abebb76`）/ numstat / code_simplifier（dev 自述其回复含糊、以 git diff 核实实际改动，做法正确）；`git worktree list` 无 `wt-TASK-001` 残留 | PASS |

**越界申报核对**：`git show --stat 40e81a7` ⇒ 仅 `internal/hestia/config_test.go`，在 `writes` 内；声明范围内三文件在 `40e81a7` 与 `abebb76` 逐字节一致。

## 3. 变异汇总（第 2 轮，4/4 KILLED）

| 变异 | 期望 | 实测 |
|---|---|---|
| M3 删 yaml `snapshot_dir` 行（**本轮判据**） | Shipped 红 | KILLED |
| M3b yaml 值改 `data/snap` | Shipped 红 | KILLED |
| M1 删 validate case（回归） | Rejects 红 | KILLED |
| M2 删 LoadConfig 预填（回归） | Defaults 红 | KILLED |

## 4. 复现命令（锚全 sha）

```bash
git worktree add --detach ../wt-verify-TASK-001-r2-m1d abebb76057c6fb847ac11fd632fdd28b3b316d47
cd ../wt-verify-TASK-001-r2-m1d
sed -i '' '/^  snapshot_dir: data\/hestia-snapshots$/d' configs/hestia.yaml
GOTOOLCHAIN=local go test ./internal/hestia/ -run TestShippedConfigLoadsAndIsCalibrated -count=1   # ⇒ FAIL（守卫生效）
git checkout -- configs/hestia.yaml
GOTOOLCHAIN=local go test ./internal/hestia/ -run TestShippedConfigLoadsAndIsCalibrated -count=1   # ⇒ ok
```

## 5. 第 1 轮记录（REJECTED，2026-09-03 03:16:48Z）

- 基线 `2f5ad513f7c0e7539c824e2e8f8a0f078baec316`；dev commit `a45ead0…`。
- 缺陷：R-001 断言 `assert.Equal(t, "data/hestia-snapshots", cfg.Storage.SnapshotDir)` 恒真——预填默认值与 yaml 值相同，删 yaml 行（M3，numstat 0/1）后测试仍 PASS。dev 在断言上方的注释恰好描述了这个失效模式。
- 首轮其余全部 PASS：三条新测试绿；M1/M2/M4（删 case / 删预填 / 改常量值）KILLED；yaml 四行注释、`config_version: "2026-09-03"` = 提交当日、头部记录齐；`ConfigVersion` 断言 `"2026-09-02"→"2026-09-03"` 是 DoD 必然连带，非绕过；A/B 背对背 `a45ead0` 96.3% / `ae088eb` 96.3%。
- 首轮给出的修法（对原始 yaml 文本断言，隔离副本实测变异态转红、原状绿）即本轮 dev 落地的形态。
