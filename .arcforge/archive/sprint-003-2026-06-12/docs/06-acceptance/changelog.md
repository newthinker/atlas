# Changelog — sprint-003 Qlib 回测验证管线（2026-06-12）

## 新功能

- **`atlas export-signals` CLI**（024c195）：按策略×标的逐日重放回测引擎导出原始信号 CSV（七列契约：symbol,date,strategy,action,confidence,price,metadata）。Fundamentals 类策略动态拒绝并列出可用清单；warm-up 自动前移拉数起点（策略窗口需求×365/252+30），仅导出 `--from` 之后的信号；`--out -` 支持 stdout
- **Python 事件研究评估层 `scripts/qlib_eval/`**（3bb35ce..592bfbf）：
  - 次日开盘入场（严格 searchsorted，规避前视），停牌顺延超 max_defer*2 日历日丢弃并计数
  - 5/20/60 日 horizon 绝对收益 + 相对 SH000300 超额；sell 信号规避口径 −(ret−bench)
  - 胜率：buy=超额>0、sell=规避>0；置信度三桶累积（≥0/≥0.6/≥0.8）
  - 基准最近前值对齐 + 负索引防护（entry 早于基准首行显式入数据缺口）
  - markdown 报告（评估口径/数据缺口/每策略小节）；qlib 数据缺失打印下载指引 exit(1)
  - pytest 全程零 qlib 依赖（守门测试锁定 lazy import）
- **`make export-signals` / `make signal-eval`**（038f49b）：一键导出+评估串联

## 缺陷修复

- backtest 引擎统一盖戳 GeneratedAt=bar 时间，机制性覆写策略自报值（302a200）——此前导出日期可能为墙钟
- ma_crossover 信号时间戳墙钟问题（9c4aab5）
- 退化路径加固（35c18c9）：空信号文件不再崩溃（写「无信号」报告 exit 0）、基准缺失降级为报告警示节、CSV BOM 容忍（utf-8-sig）

## 基础设施

- `scripts/qlib_eval/.venv`（Python 3.11.2 + pandas + pytest；系统默认 python3 损坏的绕行方案，见 README）
- `Result.SkippedBars` 新字段；导出端 stderr 输出跳过摘要

## 已知边界（一期）

- 仅 A 股符号评估（非 A 股进数据缺口节）；基准口径为收盘到收盘（入场日内偏置已注明）
- qlib 数据包需按 README 一次性下载；pe_band/dividend_yield/pe_percentile 不可离线重放
