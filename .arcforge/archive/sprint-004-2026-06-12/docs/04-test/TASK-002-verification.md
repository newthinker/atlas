# TASK-002 验证报告 — cobra 接线 + Makefile qlib-data + test_makefile 泛化

- **验证者**: test-agent-1 (Reality Checker)
- **commit**: 24d67fc / **packages**: ./cmd/atlas + ./scripts/qlib_eval（双 scope）/ **coverage_minimum**: 35
- **判定**: ✅ **VERIFIED**（6/6 DoD 逐条有真实测试/命令输出佐证，双语言门禁亲自跑通）
- 依赖前置：TASK-003 已 verified（test-agent-2），qlib-data recipe 第二行 build_data.py 契约成立。

## 机器证据（实跑输出）
1. `go test ./cmd/atlas -race -cover -count=1` → **ok, coverage 59.8%**（≥35，race 全绿）
2. CLI 专项 -v：TestExportOHLCVCommand_UsageListsAllFlags PASS / TestRunExportOHLCV_BenchmarkMissingIsFatal PASS
3. `scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -q`（仓库根）→ **43 passed**
4. test_makefile.py -v → **5 passed**（4 既有 signal-eval 零行为变化 + 新 test_qlib_data_target_flags）
5. `go build -o ./bin/atlas ./cmd/atlas` + `./bin/atlas export-ohlcv --help` → 列出 --symbols/--from/--to/--out-dir
6. `make -n qlib-data` 展开 → `./bin/atlas export-ohlcv --symbols 600519.SH,000300.SH --from 2021-01-01 --out-dir qlib_csv`（**含基准 000300.SH、无 --to**）+ `...venv/bin/python ...build_data.py --csv-dir qlib_csv --target-dir ~/.qlib/.../atlas_cn`

## Done Criteria 覆盖矩阵

| # | 完成标准 | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | UsageListsAllFlags：--symbols/--from/--to/--out-dir 齐全 | TestExportOHLCVCommand_UsageListsAllFlags（UsageString 四 flag 断言）+ 真实 --help 实证 | PASS |
| functional[1] | CLI 层 BenchmarkMissingIsFatal：清单不含 000300.SH → 报错含 benchmark | TestRunExportOHLCV_BenchmarkMissingIsFatal | PASS（**走 CLI 路径** runExportOHLCV→requireBenchmark，requireBenchmark 是独立 CLI 层函数，非核心层重复；缺基准在建 registry/任何网络前即 return，err 含 "benchmark"） |
| functional[2] | test_makefile：_target_block 泛化 + qlib-data 四断言 | test_qlib_data_target_flags + 4 个 signal-eval 测试 | PASS（5 passed；四断言：--symbols $(SIGNAL_SYMBOLS) 在 / --from $(SIGNAL_FROM) 在 / "--to" 不在 / $(QLIB_PY) build_data.py 在） |
| boundary[0] | go build 后 export-ohlcv --help 列全 flags（冒烟） | 真实二进制 ./bin/atlas export-ohlcv --help | PASS（四 flag 全列） |
| —（error_handling）| 空 | — | N/A |
| non_functional[0] | 双语言门禁：go test ./cmd/atlas(35) + hook 同款 pytest 全绿 | go 59.8%≥35/race绿 + pytest 43 passed | PASS（两语言均亲自跑） |

## team-lead 标注 6 点 — 逐一核实
1. **①UsageListsAllFlags 四 flags 齐**：✅ 测试 + 真实 --help 双证。
2. **②CLI 层 BenchmarkMissingIsFatal 真实且走 CLI 路径（非核心层重复）**：✅。requireBenchmark 是 export_ohlcv.go:214 的独立 CLI 层纯函数（slices.Contains），置于 runExportOHLCV 内 resolve 之后、collector.NewRegistry() 之前；测试直调 runExportOHLCV，缺基准在任何网络前 return。与 TASK-001 核心层（resolver 透传 flag、不校验 presence）严格分层，非重复。
3. **③Makefile --symbols 必在 / --to 必不在（C1-1/C1-3）**：✅。make -n 展开实证 `--symbols 600519.SH,000300.SH`（含基准，避开 C1-1 静默退化）、全 recipe 无 `--to`（C1-3）。test_qlib_data_target_flags 用 `"--to" not in block` 机制锁死（`--target-dir` 不含子串 `--to`，断言成立）。
4. **④_target_block 泛化后既有 test_makefile 零（行为）修改通过**：✅。4 个 signal-eval 测试调用点由 `_signal_eval_block()` 改为泛化 helper `_target_block("signal-eval")`——此即泛化本身；断言内容与所验证行为逐字不变，4 个全 PASS（零行为变化，符合 plan/DoD 意图）。
5. **⑤--help 冒烟**：✅ 真实二进制列全四 flag。
6. **⑥双语言门禁亲自跑**：✅ go（59.8%≥35/race）+ pytest（43 passed）均本机实跑，非引用 dev 自述。

## 次要观察（不阻断）
- TestRunExportOHLCV_BenchmarkMissingIsFatal 改写全局 exportOHLCVSymbols（有 t.Cleanup 复原）与 cfgFile=""（未复原）。cfgFile 默认即 ""，包内测试顺序执行无串扰，且全包 -race 绿——属测试卫生小事，非缺陷。
- runExportOHLCV 的网络分支（registry 组装 + executeExportOHLCV 实拉）按设计不在单测覆盖内（留 TASK-004 e2e 承接），coverage 59.8% 仍达标。

## 结论
压倒性证据满足 PASS：6/6 DoD 有意义覆盖且实跑通过，C1-1/C1-3 两坑经 make -n 展开实证避开，CLI 层基准校验确为独立路径非核心重复，双语言门禁双绿。**判定 VERIFIED。**
