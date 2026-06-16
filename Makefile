.PHONY: build run test clean export-signals signal-eval signal-eval-hk qlib-data qlib-data-hk signal-eval-us qlib-data-us warehouse-dump

BINARY=atlas
BUILD_DIR=bin

SIGNAL_SYMBOLS ?= 600519.SH,000300.SH
SIGNAL_FROM    ?= 2021-01-01
SIGNAL_TO      ?= 2026-06-01
# 全史 dump 起点：warehouse-dump 前若需重建全史 qlib_csv，先运行：
#   ./bin/atlas export-ohlcv --from $(WAREHOUSE_FROM) --market us --out-dir $(QLIB_CSV_US_DIR)
# 再执行 make warehouse-dump。SIGNAL_FROM 保持不变，避免影响既有 signal-eval 数据包。
WAREHOUSE_FROM ?= 1970-01-01
SIGNAL_BENCHMARK ?= 000300.SH

# Python 评估链：系统 python3 已损坏，统一走预置 venv（3.11 + pandas + pytest）
QLIB_PY        ?= scripts/qlib_eval/.venv/bin/python
SIGNAL_OUT     ?= reports/

# qlib 自建数据包：导出的 per-instrument CSV 目录 + dump_bin 产出的 qlib 数据目录
QLIB_CSV_DIR   ?= qlib_csv
QLIB_DATA_DIR  ?= $(HOME)/.qlib/qlib_data/atlas_cn
QLIB_CSV_HK_DIR  ?= qlib_csv_hk
QLIB_DATA_HK_DIR ?= $(HOME)/.qlib/qlib_data/atlas_hk
# 港股 watchlist 标的（atlas 形式）：用于 build_data 的 stale-CSV 防呆 --expected-symbols。
# 须与 configs/config.yaml watchlist 的港股集（.HK + ^HSI/^HSCE）保持一致。
SIGNAL_SYMBOLS_HK ?= 3288.HK,0700.HK,9988.HK,0883.HK,6886.HK,2800.HK,2828.HK,3033.HK,3181.HK,^HSI,^HSCE
QLIB_CSV_US_DIR  ?= qlib_csv_us
QLIB_DATA_US_DIR ?= $(HOME)/.qlib/qlib_data/atlas_us
# 美股 watchlist 标的（atlas 形式）：须与 configs/config.yaml 的美股集（个股 + ETF 裸 ticker + ^GSPC/^IXIC/^DJI 指数）一致。
SIGNAL_SYMBOLS_US ?= AAPL,MSFT,NVDA,GOOGL,AMZN,META,JNJ,JPM,SPY,QQQ,VOO,VTI,^GSPC,^IXIC,^DJI

# signal-eval 默认读自建包 atlas_cn（单一真相源）：社区包 cn_data 截止 2020-09，
# 默认 2021-2026 区间产不出结果——本需求的存在理由。覆盖 QLIB_DIR 可回退社区包。
QLIB_DIR       ?= $(QLIB_DATA_DIR)

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/atlas

export-signals: build
	./bin/atlas export-signals --strategies ma_crossover,price_percentile \
	  --symbols $(SIGNAL_SYMBOLS) --from $(SIGNAL_FROM) --to $(SIGNAL_TO) --out signals.csv

# 端到端：导出真实信号 → Python/qlib 事件研究评估，产出 markdown 报告。
# qlib 数据目录缺失时 evaluate.py 打印 get_data 下载指引并以非 0 退出（不 panic）。
signal-eval: export-signals
	$(QLIB_PY) scripts/qlib_eval/evaluate.py --signals signals.csv \
	  --qlib-dir $(QLIB_DIR) --benchmark $(SIGNAL_BENCHMARK) --out $(SIGNAL_OUT)

# 港股事件研究：港股集信号 → 对 atlas_hk 评估，基准恒生指数 ^HSI。
# 港股行情走 yahoo；离线仅 price_percentile/ma_crossover 可回放。
signal-eval-hk: build
	./bin/atlas export-signals --config configs/config.yaml --symbols $(SIGNAL_SYMBOLS_HK) \
	  --strategies price_percentile,ma_crossover --from $(SIGNAL_FROM) --to $(SIGNAL_TO) --out signals_hk.csv
	$(QLIB_PY) scripts/qlib_eval/evaluate.py --signals signals_hk.csv \
	  --qlib-dir $(QLIB_DATA_HK_DIR) --benchmark ^HSI --out $(SIGNAL_OUT)

# 自建 qlib 数据包：导出 per-instrument OHLCV CSV → dump_bin 编译为 qlib 数据目录。
# 必须显式 --symbols $(SIGNAL_SYMBOLS)（C1-1 BLOCKER）：recipe 不带 --config，CLI
# 拿不到 watchlist（config.Defaults() 无该字段），默认集会退化为只剩基准、静默缺
# 600519.SH。共用 SIGNAL_SYMBOLS 还天然保证「评估符号 ⊆ 数据包」。spec 钉死只传
# --from 不传 --to（数据包覆盖至当天）。
qlib-data: build
	./bin/atlas export-ohlcv --symbols $(SIGNAL_SYMBOLS) \
	  --from $(SIGNAL_FROM) --out-dir $(QLIB_CSV_DIR)
	$(QLIB_PY) scripts/qlib_eval/build_data.py --csv-dir $(QLIB_CSV_DIR) \
	  --target-dir $(QLIB_DATA_DIR) --expected-symbols $(SIGNAL_SYMBOLS)

# 港股自建 qlib 数据包：watchlist 港股集（.HK + ^HSI/^HSCE）→ atlas_hk（独立日历）。
# 需 --config 提供 watchlist；港股行情走 yahoo，基准 ^HSI。
qlib-data-hk: build
	./bin/atlas export-ohlcv --config configs/config.yaml --market hk \
	  --from $(SIGNAL_FROM) --out-dir $(QLIB_CSV_HK_DIR)
	$(QLIB_PY) scripts/qlib_eval/build_data.py --csv-dir $(QLIB_CSV_HK_DIR) \
	  --target-dir $(QLIB_DATA_HK_DIR) --expected-symbols $(SIGNAL_SYMBOLS_HK)

# 美股自建 qlib 数据包：watchlist 美股集（裸 ticker + ^GSPC）→ atlas_us（独立日历）。
qlib-data-us: build
	./bin/atlas export-ohlcv --config configs/config.yaml --market us \
	  --from $(SIGNAL_FROM) --out-dir $(QLIB_CSV_US_DIR)
	$(QLIB_PY) scripts/qlib_eval/build_data.py --csv-dir $(QLIB_CSV_US_DIR) \
	  --target-dir $(QLIB_DATA_US_DIR) --expected-symbols $(SIGNAL_SYMBOLS_US)

# 美股事件研究：美股集信号 → 对 atlas_us 评估，基准 ^GSPC，region us。
signal-eval-us: build
	./bin/atlas export-signals --config configs/config.yaml --symbols $(SIGNAL_SYMBOLS_US) \
	  --strategies price_percentile,ma_crossover --from $(SIGNAL_FROM) --to $(SIGNAL_TO) --out signals_us.csv
	$(QLIB_PY) scripts/qlib_eval/evaluate.py --signals signals_us.csv \
	  --qlib-dir $(QLIB_DATA_US_DIR) --benchmark ^GSPC --region us --out $(SIGNAL_OUT)

run: build
	./$(BUILD_DIR)/$(BINARY)

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)

# 历史行情仓库：从 per-instrument CSV 目录构建本地 SQLite 仓库（仅 stdlib）。
WAREHOUSE_DB ?= data/qlib_warehouse.db
# 可选：基本面 CSV 目录（归一化契约，见 scripts/qlib_warehouse/ADAPTERS.md）。
# 目录存在时透传 --fundamentals-dir；目录不存在时守卫自动省略该参数，dump 仍正常。
FUNDAMENTALS_US_DIR ?= fundamentals_csv_us
warehouse-dump:
	@mkdir -p $(dir $(WAREHOUSE_DB))
	$(QLIB_PY) -m scripts.qlib_warehouse.build_warehouse \
	  --csv-dir $(QLIB_CSV_US_DIR) --market US --source yahoo --db $(WAREHOUSE_DB) \
	  $(if $(wildcard $(FUNDAMENTALS_US_DIR)),--fundamentals-dir $(FUNDAMENTALS_US_DIR),)
