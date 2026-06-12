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
  │     --symbols  默认 = watchlist 内 A 股标的（.SH/.SZ 后缀判定）+ 000300.SH 基准（去重）
  │     --from/--to/--out-dir（日期由 Makefile 传 SIGNAL_FROM/SIGNAL_TO，见下）
  │     每标的一个 CSV（列：symbol,date,open,high,low,close,volume,factor）
  │     文件名 = 小写 qlib instrument（600519.SH → sh600519.csv）
  │     collector 选择复用 SelectForSymbol（eastmoney 指数 secid 路径）
  └─ ② build_data.py（scripts/qlib_eval/ 新增，薄封装）
        --csv-dir / --qlib-scripts（默认 /Users/zuowei/workspace/python/qlib/scripts）
        --target-dir（默认 ~/.qlib/qlib_data/atlas_cn）
        subprocess 调官方 dump_bin.py（DumpDataAll 全量模式，--exclude_fields symbol,date）
        校验 instruments/all.txt 与 calendars/day.txt（dump_bin 自动生成，build_data 只核对不重写）
```

**符号三形式约定**（贯穿全文，勿混用）：atlas 符号 = `000300.SH`（CLI 入参/采集器/错误消息）；CSV 文件名 = 小写 instrument `sh000300.csv`；qlib 库内 instrument = `SH000300`（instruments/all.txt 中的形式）。Go 侧文件名映射的**权威定义是 `scripts/qlib_eval/qlib_eval/symbols.py` 的 `to_qlib_instrument`**（小写化），双侧各加同样本契约测试（`000300.SH → sh000300`）。

**日期区间**：Makefile 的 `qlib-data` 与 `export-signals` 共用 `SIGNAL_FROM`/`SIGNAL_TO` 变量，且数据包结束日取 `max(SIGNAL_TO, today)` 语义（实现为 qlib-data 默认 `--to` 用当天）——保证 horizon 60 日窗口有数据，杜绝「数据区间 < 信号区间」重蹈本需求要解决的问题。

**非 A 股符号语义**：export-ohlcv 镜像 `to_qlib_instrument` 规则——非 `.SH/.SZ` 符号直接拒绝并计入失败摘要（不落盘、不静默跳过），与 Python 评估侧 Phase-1 A 股 only 边界一致。

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
- **基准 000300.SH 失败 = 硬错误**（无基准则评估无意义，不降级）。
- 限流：当前默认集仅 2 个 instrument（watchlist 现实只有 600519.SH 一只 A 股 + 基准），无需限流；逐符号加固定 300ms 间隔作为礼貌延迟，为「几十标的」规模预留（一行实现，不做更复杂的限流器）。
- 空 CSV 不写盘（防 dump_bin 吞入产生空 instrument）。
- dump_bin 子进程失败：透传 stderr，非 0 退出。

## 已钉死的口径结论（评审核实，不再是开放核对项）

1. **复权口径已确认**：eastmoney kline 为 **fqt=1 前复权**（eastmoney.go:433），信号生成侧用同一 FetchHistory，两侧天然同源；evaluate 侧只读 $open/$close 不乘 $factor（prices.py），故 **CSV factor 列恒写 1**，README「评估口径」注明「价格为前复权，factor=1」。
2. **dump_bin 参数已确认**：不传字段过滤时 dump_bin 对全部列做 astype(float32)，字符串列必崩——build_data 必须传 **`--exclude_fields symbol,date`**（date 经 --date_field_name 处理、symbol 从文件名取），该参数列入 mock subprocess 命令构造断言的预期值。其余约定（--date_field_name 默认 date、文件名即 instrument）已读源码核实（dump_bin.py:269,328-337）。

## 实现首日核对项

- 实跑 dump_bin 一次后用 qlib `D.features(["SH600519"],...)` 抽查首尾两天数值与源 CSV 一致（端到端数值 sanity，防字段错位类低级事故）。

## 测试策略

- Go（cmd/atlas，沿用 coverage_minimum=35 先例）：
  - golden CSV 逐字节（列名/小写文件名/日期格式/factor=1 列）
  - 单符号失败 → 降级摘要 + 非 0 退出；基准失败 → 硬错误；**非 A 股符号拒绝进摘要**
  - 默认符号集 = watchlist A 股（.SH/.SZ 判定，config 经既有 --config 旗标 + config.Load 加载）+ 000300.SH 基准去重——注意现实：当前 config 仅 1 只 A 股，默认集 = {600519.SH, 000300.SH}，与 SIGNAL_SYMBOLS 恰好重合
  - 文件名契约测试（与 symbols.py 同样本）；--help 冒烟
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
