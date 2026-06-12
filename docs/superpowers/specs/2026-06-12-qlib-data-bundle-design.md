# 自建 qlib 数据包（atlas 采集数据 → dump_bin）设计

**日期**: 2026-06-12 | **状态**: 设计定稿，待实现
**动机**: 社区 qlib cn_data 截止 2020-09-25，signal-eval 无法评估 2021-2026 信号（sprint-003 验收实测：1457 信号全部丢弃）。
**前置**: sprint-003 qlib 评估管线已交付；eastmoney 指数 FetchHistory 路径已修复（e5e8056）。

## 目标

`make qlib-data` 一键从 atlas 自有采集器拉取 OHLCV、经官方 dump_bin 构建 qlib 数据包，使 `make signal-eval` 在默认 2021-2026 区间产出非空评估结果。

**核心原则：评估数据与信号生成数据同源**（同一套 atlas 采集器），消除评估口径漂移——这是否决「纯 Python 拉取」方案的决定性理由。

## 架构与数据流

```
make qlib-data
  ├─ ① atlas export-ohlcv（Go，新增 cobra 子命令）
  │     --symbols  默认 = watchlist 内 A 股标的 + SH000300 基准；可传任意清单
  │     --from/--to/--out-dir
  │     每标的一个 CSV（dump_bin 标准列：symbol,date,open,high,low,close,volume,factor）
  │     文件名用 qlib 小写 instrument 形式（600519.SH → sh600519.csv）
  │     collector 选择复用 SelectForSymbol（eastmoney 指数 secid 路径）
  └─ ② build_data.py（scripts/qlib_eval/ 新增，薄封装）
        --csv-dir / --qlib-scripts（默认 /Users/zuowei/workspace/python/qlib/scripts）
        --target-dir（默认 ~/.qlib/qlib_data/atlas_cn）
        subprocess 调官方 dump_bin.py（DumpDataAll 全量模式）
        生成 instruments/all.txt；校验 calendars/day.txt 覆盖请求区间
```

- 目标目录 `atlas_cn` 独立于社区 `cn_data`，两包并存可对比。
- `signal-eval` 已有 `--qlib-dir`/`QLIB_DIR` 参数，零改动适配；Makefile 将 `QLIB_DIR` 默认值切到 `atlas_cn`。
- 更新模式：每次全量重建（几十标的分钟级）；定时调度用系统 cron 调 make target（README 给 crontab 示例），不新建 atlas 内调度框架。增量更新（DumpDataUpdate）明确留二期。

## 组件

| 组件 | 职责 | 依赖 |
|------|------|------|
| cmd/atlas/export_ohlcv.go | 拉取+CSV 落地，结构同 export_signals（exportDeps 注入 + golden 测试） | collector registry / SelectForSymbol |
| scripts/qlib_eval/build_data.py | dump_bin 编排 + instruments/calendar 收尾 | 本地 qlib 副本（仅运行时） |
| Makefile qlib-data | ①→② 串联；signal-eval 的 QLIB_DIR 默认切换 | — |
| README 建包章节 | 一次性准备、crontab 示例、复权口径说明 | — |

## 错误处理

- 逐符号降级：单标的拉取失败 → stderr 警告 + 跳过，结尾输出失败摘要；失败数 > 0 时整体非 0 退出（已成功的 CSV 保留，可重跑）。
- **基准 SH000300 失败 = 硬错误**（无基准则评估无意义，不降级）。
- 空 CSV 不写盘（防 dump_bin 吞入产生空 instrument）。
- dump_bin 子进程失败：透传 stderr，非 0 退出。

## 实现首日核对项（钉死，防口径事故）

1. **复权口径**：核对 eastmoney kline 接口当前 fqt 参数取值（不复权/前复权/后复权），必须与信号生成侧（分析循环/backtest 用的同一 FetchHistory）一致；据此决定 factor 列写法——qlib 惯例 factor=复权因子，若导出价已复权则 factor=1 并在 README 注明。两侧口径写进 README「评估口径」。
2. **dump_bin 输入约定**：CSV 列名、日期格式、--date_field_name/--symbol_field_name 等参数以官方脚本实际解析逻辑为准（实现时读 dump_bin.py 源码核对，不凭记忆）。

## 测试策略

- Go（cmd/atlas，沿用 coverage_minimum=35 先例）：
  - golden CSV 逐字节（列名/小写文件名/日期格式/factor 列）
  - 单符号失败 → 降级摘要 + 非 0 退出；基准失败 → 硬错误
  - 默认符号集 = watchlist A 股 + 基准（含去重）；--help 冒烟
- Python（pytest 默认零 qlib 依赖，hook 同款命令）：
  - build_data 的 dump_bin **命令构造**用 mock subprocess 断言（参数/路径/顺序）
  - instruments/calendar 生成逻辑用合成 CSV 目录单测
  - 真实 dump_bin 调用走 pytest marker（integration，本地 qlib 副本存在时运行）
- **e2e 验收（需求存在理由）**：`make qlib-data && make signal-eval`（默认 2021-2026 区间）产出含非空策略结果表的报告——社区包做不到的事。

## 范围外（YAGNI）

增量更新（DumpDataUpdate）、全市场/指数成分股全量、分钟线、qlib 语义池（csi300 等 instruments 池）、atlas 内置调度框架。

## 环境备注

- Python 一律 `scripts/qlib_eval/.venv/bin/python`（系统 python3 损坏，sprint-003 已沉淀）。
- venv 中 qlib 已装（pyqlib 0.9.8.dev31，pandas 2.3.3）；数据包构建是运行时行为，pytest 不依赖。
