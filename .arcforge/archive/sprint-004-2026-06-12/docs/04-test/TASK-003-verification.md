# TASK-003 验证报告 — build_data.py（dump_bin 编排 + 产物校验）

- **判定: ✅ VERIFIED**
- 验证者: test-agent-2 | 日期: 2026-06-12 | commit: a82956d | epoch: 1
- 包: scripts/qlib_eval | 心智: Reality Checker（默认 NEEDS WORK）

## 运行证据（从仓库根，零重跑取巧）
```
$ scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/test_build_data.py -v
10 passed in 0.02s
$ scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -q
42 passed in 0.23s        # 全量回归无破坏
$ grep -nE "import qlib|from qlib" scripts/qlib_eval/build_data.py
(无)  # 零 qlib import；唯一外部调用 subprocess 跑官方 dump_bin.py，单测全 mock
```

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | dump_bin 命令构造：dump_bin.py + dump_all + **--data_path** + --qlib_dir + --exclude_fields symbol,date（紧邻值） | test_dump_bin_command_construction | PASS |
| functional[1] | date_span_from_csvs 多 CSV 推导 (min,max) | test_date_span_from_csvs（2 文件，全局 min 2020-12-31 / max 2021-03-10） | PASS |
| functional[2] | verify_bundle：tab 三字段大写 fixture 通过 / 缺 instrument(含项) / calendar 不足 → ValueError / 只读 | test_verify_bundle_passes, _missing_instrument, _calendar_too_narrow, _is_read_only | PASS |
| boundary[0] | 空文件 / 仅 header CSV → 进 dump 前 raise | test_csv_dir_empty_file_rejected, _header_only_rejected (+_empty_dir) | PASS |
| error_handling[0] | dump_bin returncode!=0 → raise 含 stderr 摘要 | test_dump_bin_failure_propagates | PASS |
| non_functional[0] | hook 同款命令全绿（仓库根，零 qlib import） | 全量 42 passed + grep 无 qlib import | PASS |

## plan 评审拦截坑核查（重点）
- ① **--data_path 非 csv_path**：源码 line 63 `"--data_path"`；test line 85-86 `cmd.index("--data_path"); cmd[i+1]==str(csv_dir)` 真实紧邻断言。**未踩回 C2-1 BLOCKER**。`--exclude_fields symbol,date` 紧邻值断言（line 89-90）✓。
- ② **verify_bundle 只读 + 真实格式**：fixture `_make_bundle` 用 `f"{ins}\t{b}\t{e}"` tab 三字段、instrument 大写（SH600519）；只读由 `_tree_digest`(sha256 路径+内容) 前后比对断言（line 212-216），比 mtime 更强。✓
- ③ **多文件 min/max**：双 CSV 跨文件全局 min/max，真实推导（非硬编码单文件）。✓
- ④ **空 CSV 进 dump 前 raise**：三个 boundary 测试均 `run.assert_not_called()`，证明 subprocess 之前拒绝。✓
- ⑤ **returncode!=0 透传**：mock returncode=1 + stderr="boom"，断言 RuntimeError 含 "boom"。✓
- ⑥ **零 qlib import**：build_data 仅 import argparse/subprocess/sys/pathlib；subprocess 全 mock，无真实 dump_bin/qlib 运行路径。✓

## fantasy assertion 排查（无）
- 命令断言走真实 run_dump_bin 代码路径（仅 mock subprocess.run），非硬编码绕路。
- 失败/边界测试各自独立路径，无共用代码路径掩盖。
- 紧邻值用 cmd.index() 定位真实相邻元素，非位置巧合。

## 非阻断备注（不影响判定）
1. test_verify_bundle_calendar_too_narrow 仅断言 ValueError 类型，未断消息子串；DoD「含缺失项」语义由 missing_instrument 用例覆盖（已断 SH000300）。建议二期补 calendar 区间消息断言。
2. discovery 称「venv 未装 qlib，全绿即证」已过时——qlib 实际已装于 /Users/zuowei/workspace/python/qlib。但「零 qlib import」结论不受影响：由源码 grep（无 import）+ subprocess 全 mock 双重证明，强于「venv 未装」的间接论据。
