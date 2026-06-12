# TASK-006 验证报告 — 事件研究计算核心

- **验证人**: test-agent-1 (Reality Checker)
- **判定**: ✅ **VERIFIED**
- **plan 对应**: Task 7 / commit d92994d
- **包**: ./scripts/qlib_eval

## 实测命令与输出（hook 同款，仓库根）
```
scripts/qlib_eval/.venv/bin/python -m pytest scripts/qlib_eval/tests/ -v
→ 16 passed in 0.16s（event_study 8 + prices 6 + symbols 2）
```

## Done Criteria 覆盖矩阵
| # | 完成标准 | 对应测试 | 判定 |
|---|---|---|---|
| functional[0] | horizon 收益+超额：入场10.0→5日close11.0(+10%)、基准+2%→超额+8% | test_horizon_return_and_excess（断言 returns[5]≈0.10, excess[5]≈0.08） | PASS |
| functional[1] | sell 规避：标的-10%、基准+2%→规避+12% | test_sell_avoidance_return（returns[5]≈-0.10, excess[5]≈0.12） | PASS |
| functional[2] | aggregate strategy×action n/mean/median/win_rate（buy超额>0/sell规避>0） | test_aggregate_by_strategy_action（4buy mean0/wr0.5、2sell wr0.5） | PASS |
| functional[3] | 置信度三桶累积 conf0.5/0.65/0.85→n=3/2/1 | test_confidence_buckets_are_cumulative | PASS |
| functional[4] | 基准最近前值对齐（最易写错处） | test_benchmark_aligns_to_last_available_before_entry | PASS |
| boundary[0] | horizon 越界→该 horizon None 不污染聚合 | test_horizon_exceeds_data_returns_none（h20/h60 None，h5 有效） | PASS |
| boundary[1] | entry 早于 bench 首行→显式 None（防 -1 取末行） | test_entry_before_benchmark_returns_none | PASS |
| non_functional | pytest 全绿，纯 pandas 无 qlib | 16 passed；event_study 无 qlib import，运行时 sys.modules 无 qlib | PASS |

## Reality Check（防 fantasy assertion — 数值任务最高风险）
- **数值手工复算无误**（亲自重算，非信测试自述）：
  - 超额：ret=11/10-1=+0.10，bench=3060/3000-1=+0.02，buy excess=+0.08 ✓
  - sell：raw=-0.10-0.02=-0.12，规避=-raw=+0.12 ✓
  - 聚合：buy 超额{+5,+1,-2,-4}% mean=0、win=2/4=0.5；sell{+3,-1}% win=1/2=0.5 ✓
  - 累积桶：0.0桶含全3、0.6桶含{0.65,0.85}=2、0.8桶含{0.85}=1 ✓
- **基准对齐陷阱亲验**（独立 Python 重算 searchsorted）：entry 1/3 on bench[1/2,1/4,1/8]→
  side="right"-1=0→取 **3000 而非 3030**；test 注释明确「若误取 3030 则 excess≈+9.01% 翻车」——
  即此断言能真实抓住误实现，非空洞。
- **负索引守门为真**：entry 1/2 早于 bench[2/1,2/2]→_last_le 返回 -1→start_pos<0→显式 return None；
  独立重算确认 -1，挡住 Python `iloc[-1]` 静默取末行的经典 bug。
- **越界不污染聚合**：exit_idx=entry.index+h>=n_bars→returns[h]/excess[h]=None；aggregate
  跳过 None（`if ret is None or exc is None: continue`），n 只数有效样本。
- **零 qlib**：event_study.py 仅 import dataclasses/statistics/pandas/.prices；
  grep 无 qlib；PYTHONPATH 运行时 import 后 sys.modules 无 qlib。

## 结论
七条 functional/boundary + non_functional 全部真实断言覆盖，数值经独立手工复算一致，
基准对齐与负索引两大「最易写错处」均被能抓误实现的真实用例钉死。VERIFIED。
