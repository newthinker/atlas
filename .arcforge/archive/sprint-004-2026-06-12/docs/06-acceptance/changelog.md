# Changelog — sprint-004 自建 qlib 数据包（2026-06-12）

## 新功能

- **`atlas export-ohlcv`**（7f2a080, 24d67fc）：按 qlib dump_bin 约定导出 per-instrument OHLCV CSV（symbol,date,open,high,low,close,volume,factor；价格 fqt=1 前复权、factor=1）；符号三形式契约（000300.SH → SH000300 → sh000300.csv，与 Python 侧 to_qlib_instrument 同样本锁定）；逐符号降级摘要 + 基准硬错误 + 非 A 股拒绝；默认符号集 = watchlist A 股 + 沪深300 基准
- **`build_data.py`**（a82956d, 14cb5e5）：subprocess 编排官方 dump_bin（DumpDataAll，--exclude_fields symbol,date）；instruments/calendar 只读校验；日期区间从 CSV 推导；**残留 CSV 防呆**（--expected-symbols 比对，发现非预期符号 raise 并指引清理）
- **`make qlib-data`**（24d67fc）：export-ohlcv（--symbols $(SIGNAL_SYMBOLS)，--to 默认当天）→ build_data 一键建包到 ~/.qlib/qlib_data/atlas_cn
- **`make signal-eval` 默认数据源切换**（36d476d）：QLIB_DIR 默认 atlas_cn——默认 2021-2026 区间实测产出 1457 信号、data_gaps=0、两策略非空结果表（社区包截止 2020-09 做不到）

## 文档

- README 新增建包章节：用法/复权口径（含**前复权跨日漂移披露**——每日全量重建后历史值随新除权事件平移）/crontab 示例/evaluate.py 直调注意/社区包 vs 自建包对比

## 已知边界

- 数据包覆盖 = SIGNAL_SYMBOLS（默认 600519.SH + 000300.SH），扩符号改变量即可
- 非原子换包（OPS-1）与 qlib scripts 路径硬编码（OPS-2）为 fast-follow 项
- 评估数据与信号生成同源（同一套 atlas 采集器）——这是本管线的口径基石
