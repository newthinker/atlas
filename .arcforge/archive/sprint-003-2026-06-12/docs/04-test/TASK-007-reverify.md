# TASK-007 复验报告（review_fix）— 退化路径加固

- **Verifier**: test-agent-2 (Reality Checker)
- **判定**: ✅ VERIFIED（复验，rework_count=1, epoch=2）
- **Commit**: 35c18c9 `fix(TASK-007): harden degraded paths — empty signals, benchmark guard, utf-8-sig, close-to-close note`
- **Package**: `scripts/qlib_eval`
- **验证时间**: 2026-06-12 sprint-003

## 实际运行证据（hook 同款命令，仓库根目录）
```
$ scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -v
32 passed in 0.17s   # 原 DoD 测试全回归通过 + 4 项 review_fix 新测试
```

## fix_items 四项逐条复验
| fix | 源码修复 | 对应新测试 | 判定 |
|---|---|---|---|
| **W1** 空信号文件(仅 header) NaTType strftime 崩溃 | evaluate.py main：`if signals.empty:` 入口短路，写「无信号」报告 exit 0（在惰性 qlib import **之前**） | `test_main_empty_signals_writes_report_exit0`：header-only CSV + 真实存在 qlib_dir → 驱动 main()，断言 rc==0、写 1 份报告、含「信号总数: 0」、"qlib" not in sys.modules | ✅ PASS |
| **W2** benchmark 缺失整跑崩溃 | collect_outcomes：`source.benchmark()` 包 try/except → 降级 `[], {**_empty_stats(), "benchmark_error": str(e)}`；render_report 增 ⚠ 基准缺失提示节 | `test_collect_outcomes_benchmark_failure_is_graceful`（_BenchFailSource raise FileNotFoundError → outcomes==[]、stats 含 benchmark_error 且含 SH000300）+ `test_render_report_surfaces_benchmark_error`（md 含错误文案） | ✅ PASS |
| **S3** README 收盘到收盘口径注明 | README.md L85 新增「基准收益口径：基准为收盘到收盘 vs 个股开盘到收盘，入场日内偏置」 | review/manual（N/A 测试）；grep 确认存在 | ✅ PASS |
| **S7** read_signals utf-8-sig | report.py L22 `open(path, newline="", encoding="utf-8-sig")` | `test_read_signals_tolerates_utf8_bom`：写真实 BOM 字节 + VALID_CSV → 断言解析 2 行且首列名 "symbol" 未被 BOM 污染 | ✅ PASS |

## Reality Checker 校验
- **W1 真实复现**：QA 原反例是 NaTType strftime 崩溃。新测试用 header-only 文件（空 date 列）驱动**真实 main()**——若修复缺失，main 会在 `.min().strftime` 抛 NaTType 异常使测试 error；测试 PASS 即证明短路真实生效，**非空洞断言**。短路点位于惰性 qlib import 之前，`"qlib" not in sys.modules` 断言佐证。
- **W2 真实降级**：_BenchFailSource.benchmark() 真抛异常走 except 分支；history() 标注不应被调用（基准先失败）——验证降级路径而非乐观路径。
- **S7 真实 BOM**：用 `write_bytes("﻿"+VALID_CSV)` 构造真实 UTF-8 BOM，断言首列名不被污染——真实穿透 utf-8-sig。
- **原 DoD 回归**：read_signals schema / render_report 小节 / 非A股 / qlib 缺目录 / 不 import qlib 等原测试全部仍 PASS（32 passed），修复未引入回归。
- 既有测试零删改：fix commit test_report.py 为纯新增（+63），report.py/evaluate.py 为重构+新增（_empty_stats/_meta/_write_report 抽取消除 drift），原测试不变。

## 结论
四项 fix_items 全部有真实测试守护、原 DoD 全回归、hook 同款 pytest 32 全绿、无 fantasy assertion。判定 **VERIFIED**。
