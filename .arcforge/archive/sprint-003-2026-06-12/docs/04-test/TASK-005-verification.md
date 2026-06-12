# TASK-005 验证报告 — PriceSource 协议 + 入场对齐

- **验证人**: test-agent-1 (Reality Checker)
- **判定**: ✅ **VERIFIED**
- **plan 对应**: Task 6 / commit 7de4730
- **包**: ./scripts/qlib_eval

## 实测命令与输出（hook 同款，仓库根）
```
scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -v
→ 8 passed in 0.14s（test_prices 6 + test_symbols 2）
```

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | 信号 1/2 → 入场 1/3 开盘 10.2（严格次日，规避前视） | test_align_entry_next_open（断言 date==1/3 **且 price==10.2=1/3 的 open**） | PASS |
| functional[1]-保留 | 1/5→1/15 恰 10 日历日 == max_defer*2 → 保留入场 1/15 | test_align_entry_boundary_defer_kept | PASS |
| functional[1]-丢弃 | 1/2→1/15 13 日历日 > 10 → None | test_align_entry_drops_when_defer_exceeds_limit | PASS |
| functional[2] | 最后一根 bar 之后信号 → None | test_align_entry_drops_when_no_data（signal 1/15=末行） | PASS |
| boundary[0] | Entry 携带 positional index | test_align_entry_carries_positional_index（1/2→idx1、1/4→idx3） | PASS |
| non_functional[0] | pytest 全绿且不 import qlib（lazy 验证） | test_no_qlib_at_module_level + 8 passed + impl 全 qlib import 在方法体内 | PASS |

## Reality Check（防 fantasy assertion）
- **次日开盘真实路径**：impl `searchsorted(signal_date, side="right")` → 严格次日；
  test 断言 price==10.2 取的是 **open 列**（1/3 open=10.2, close=10.3），同时挡住「前视」与「误用 close」两类错误。
- **边界比较符 `>` 被双侧夹死**：impl `(entry_date - signal_date).days > max_defer*2`；
  10>10=False（保留）、13>10=True（丢弃）两测试配对——写成 `>=` 则保留用例翻车。非空洞断言。
- **越界 None 真实**：signal=末行 1/15 → searchsorted pos=len → None（test_align_entry_drops_when_no_data）。
- **lazy qlib 机制为真**：QlibPriceSource 的 `import qlib`/`from qlib.data import D` 全部在
  `_ensure_init`/`history`/`benchmark` 方法体内；模块顶层无 qlib import；守门测试 import 后断言
  `"qlib" not in sys.modules`，在未装 qlib 的 .venv 下 8 passed 即铁证。

## 透明备注（非阻塞）
- `QlibPriceSource.history/benchmark/_normalize/_ensure_init` 无单测覆盖——按设计如此：
  这些路径需真实 qlib + 数据包，DoD 明确禁止 pytest 引入 qlib（零依赖机制保证）。
  Python 任务无覆盖率门禁；lazy-import 契约已被守门测试覆盖。真实 qlib 路径将在 Task 8/9 端到端环节由数据就位环境验证。

## 结论
六条 DoD 全部真实断言覆盖，边界算子被双侧用例钉死，lazy-import 机制为真。VERIFIED。
