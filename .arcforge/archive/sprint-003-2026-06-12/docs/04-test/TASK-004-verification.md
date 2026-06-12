# TASK-004 验证报告 — Python 脚手架 + 符号映射

- **验证人**: test-agent-1 (Reality Checker)
- **判定**: ✅ **VERIFIED**
- **plan 对应**: Task 5
- **包**: ./scripts/qlib_eval

## 实测命令与输出（hook 同款，从仓库根执行）
```
scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -v
→ 2 passed in 0.00s (rootdir=仓库根, Python 3.11.2)
```

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试/证据 | 判定 |
|---|---|---|---|
| functional[0] | to_qlib_instrument 三正例（SH/SZ 前缀换位） | test_to_qlib_instrument（600519.SH→SH600519、000300.SH→SH000300、399001.SZ→SZ399001 三断言） | PASS |
| functional[1] | 五类非 A 股符号 raise ValueError | test_to_qlib_instrument_rejects_non_ashare（AAPL/^GSPC/GC=F/BTC-USDT/0700.HK 循环 pytest.raises(ValueError)） | PASS |
| non_functional[0] (test) | conftest.py 存在（sys.path 声明）；hook 同款命令从根全绿；不 import qlib | conftest.py 存在并注入 sys.path；从根 2 passed；grep 全源无 `import qlib` 语句（命中仅 docstring/约束文本） | PASS |
| non_functional[1] (review) | README 含数据包下载命令与 pyqlib 安装两方式 | README L16-31：pyqlib 两方式（pip install pyqlib / pip install -e 本地副本）+ qlib_data 下载命令（SunsetWolf/qlib_dataset，region cn） | PASS |

## Reality Check（防 fantasy assertion）
- **真实路径**：测试 `from qlib_eval.symbols import to_qlib_instrument` 调真函数断真实返回值，无硬编码绕过。
- **拒绝测试覆盖多样化非 A 股**（美股/指数/期货/加密/港股 5 类），非共用单一路径。
- **零 qlib 依赖机制保证为真**：源码（conftest/qlib_eval/tests）无任何 `import qlib` 语句，仅 docstring 文本提及约束；pytest 在不装 qlib 的 .venv 内 2 passed。
- **六个必需文件全部落盘**：README/requirements.txt/conftest.py/__init__.py/symbols.py/tests/test_symbols.py。

## 结论
两条 functional + test/review non_functional 全覆盖，pytest 从仓库根全绿，零 qlib 依赖机制为真。VERIFIED。
