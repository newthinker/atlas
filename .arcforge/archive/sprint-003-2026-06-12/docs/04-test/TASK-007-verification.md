# TASK-007 验证报告 — report.py + evaluate.py CLI（CSV 读取/markdown 报告/qlib 检测）

- **Verifier**: test-agent-2 (Reality Checker)
- **判定**: ✅ VERIFIED
- **Commit**: 592bfbf `feat(qlib_eval): CSV ingestion, markdown report and CLI entry`
- **Package**: `scripts/qlib_eval`（Python，无覆盖率门禁 → DoD↔测试逐条矩阵）
- **依赖**: TASK-006 已 verified
- **验证时间**: 2026-06-12 sprint-003

## 实际运行证据（hook 同款命令，仓库根目录）
```
$ scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -v
24 passed in 0.16s   # 含 test_report.py 8 项全 PASS
$ scripts/qlib_eval/.venv/bin/python -c "find_spec('qlib')"
qlib NOT installed   # 全绿且 qlib 未安装 → 真实证明零 qlib 依赖（任何触达 prices.py/qlib 的路径都会 ImportError 硬失败）
```

## 验证要点逐条（lead 指定 ①–⑤）
1. **read_signals 严格 schema**：
   - 合法 CSV 三断言：`test_read_signals_valid` 断言 date==`Timestamp("2024-01-03")`、`isinstance(confidence,float)` 且 ==0.70、metadata==`'{"k":1}'`（**保留原串不反序列化**——CSV `"{""k"":1}"` 解析回 `{"k":1}` 文本）。
   - **缺列含行号**：`test_read_signals_invalid_missing_column_has_line_number`（第 3 物理行少 metadata 列→`match="line 3"`）。
   - **坏行含行号**：`test_read_signals_invalid_confidence_has_line_number`（confidence="notafloat"→`match="line 2"`）。
   - 两类（缺列 + 坏行）**均有测试**，外加 bonus `test_read_signals_invalid_header`（表头不匹配）。✅
2. **render_report 三类小节**：`test_render_report_sections` 断言含「评估口径」「数据缺口」「每策略小节(ma/pe)」，并校验非 A 股(AAPL/0700.HK)落入数据缺口节、表头列 win_rate 出现。✅
3. **qlib 数据目录缺失 → 打印下载命令 + exit(1)，用真实不存在目录路径**：
   `test_main_exits_when_qlib_dir_missing` 用 `missing = tmp_path/"no_such_qlib_dir"`（**真实不存在目录，非 mock**），调 `evaluate.main(...)` 断言 `rc==1`、stderr 含 `qlib_data`（下载指引）、且 `"qlib" not in sys.modules`（缺目录时提前返回，未触发惰性 import）。源码 evaluate.py L113-115 check_qlib_dir 失败即写 hint+return 1，qlib 仅在 L120 main 内成功路径惰性 import。✅
4. **非 A 股进数据缺口节**：`test_non_ashare_collected_into_data_gaps`——AAPL 经 to_qlib_instrument raise ValueError 被收进 `stats["non_ashare"]`，600519.SH 仍产出 outcome(len==1)，评估不中断。✅
5. **pytest 全程不 import qlib（hook 同款命令跑）**：24 passed 且 qlib 未安装；`test_import_evaluate_no_qlib` + `test_no_qlib_at_module_level`(prices) + test_main_exits 三处断言 `"qlib" not in sys.modules`。✅

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | read_signals 合法 CSV→DataFrame（date=Timestamp/conf=float/metadata 原串） | test_read_signals_valid | ✅ PASS |
| functional[1] | render_report 含评估口径/数据缺口/每策略小节 | test_render_report_sections | ✅ PASS |
| boundary[0] | 非 A 股不中断评估，收集进数据缺口节 | test_non_ashare_collected_into_data_gaps | ✅ PASS |
| error_handling[0a] | 缺列 ValueError 含行号 | test_read_signals_invalid_missing_column_has_line_number (line 3) | ✅ PASS |
| error_handling[0b] | 坏行 ValueError 含行号 | test_read_signals_invalid_confidence_has_line_number (line 2) | ✅ PASS |
| error_handling[0c] | qlib 目录缺失打印下载命令 exit(1) | test_main_exits_when_qlib_dir_missing（真实不存在目录） | ✅ PASS |
| non_functional[0] | pytest 全绿不依赖 qlib；qlib 检测路径有测试 | 24 passed w/o qlib + test_main_exits + test_import_evaluate_no_qlib | ✅ PASS |

## Fantasy Assertion 排查
- ③ qlib 缺目录测试用**真实不存在路径** `tmp_path/"no_such_qlib_dir"`，非 mock/硬编码绕过——真实穿透 check_qlib_dir。
- ⑤ 零 qlib 依赖为**强证明**：qlib 未安装，若任何测试路径触达 prices.py 会 ImportError；24 passed 即证明惰性 import 设计真实成立，非空洞断言。
- ① metadata 原串断言真实区分「保留文本」vs「反序列化为 dict」（断言 == 字符串 `'{"k":1}'`）。
- 缺列/坏行走**不同错误分支**（len 校验 vs float 解析），各自断言不同行号（line 3 / line 2），未共用代码路径掩盖。

## 结论
压倒性证据：24 测试全绿（hook 同款命令，仓库根，qlib 未安装）、忠实 plan T8、5 要点逐条真实测试守护、
qlib 检测用真实不存在目录、零 qlib 依赖为强证明、既有测试零修改（commit 仅新增 3 文件）。判定 **VERIFIED**。
