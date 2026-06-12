# QA Code Review — Round 1 (工具门禁 + E2E 独立复证)
sprint: sprint-004 | reviewer: qa-agent-1 | date: 2026-06-12
scope: 41c5e75..HEAD (4 commits)

## 工具门禁结果

| 检查项 | 结果 |
|--------|------|
| `go build ./...` | OK |
| `go vet ./...` | OK |
| `go test ./...` | OK |
| `go test -race ./cmd/atlas` | OK（macOS LC_DYSYMTAB 链接告警，非问题） |
| `pytest scripts/qlib_eval/tests/ -q` | 44 passed |
| `gitnexus analyze` | OK（6040 nodes）|

## 契约核实

- dump_bin.py 签名：`data_path` / `qlib_dir` / `exclude_fields` / `symbol` / `date` 全匹配
  C2-1 结论（--data_path 非 --csv_path）正确。
- Go `toQlibInstrument` vs Python `to_qlib_instrument`：同样本（000300.SH/600519.SH/399001.SZ + 5 非 A 股）同结果。

## E2E 独立复证 (ADR-S4-3 必跑)

| 步骤 | 结果 |
|------|------|
| `make qlib-data` | atlas_cn 产出，2 instruments，区间 [2021-01-04, 2026-06-12] |
| D.features 首尾核对 | SH600519 首尾两天 open/close/volume/factor 逐值匹配 qlib_csv/sh600519.csv |
| `make signal-eval`（2021-2026） | 1457 信号，data_gaps=0，ma_crossover + price_percentile 结果表非空 |

**E2E: PASS** — 需求存在理由（vs 社区包全丢）已实证。

## 结论
Round 1 全绿，进入 Round 2 深度 Code Review。
