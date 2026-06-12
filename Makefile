.PHONY: build run test clean export-signals signal-eval qlib-data

BINARY=atlas
BUILD_DIR=bin

SIGNAL_SYMBOLS ?= 600519.SH,000300.SH
SIGNAL_FROM    ?= 2021-01-01
SIGNAL_TO      ?= 2026-06-01

# Python 评估链：系统 python3 已损坏，统一走预置 venv（3.11 + pandas + pytest）
QLIB_PY        ?= scripts/qlib_eval/.venv/bin/python
SIGNAL_OUT     ?= reports/

# qlib 自建数据包：导出的 per-instrument CSV 目录 + dump_bin 产出的 qlib 数据目录
QLIB_CSV_DIR   ?= qlib_csv
QLIB_DATA_DIR  ?= $(HOME)/.qlib/qlib_data/atlas_cn

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
	  --qlib-dir $(QLIB_DIR) --out $(SIGNAL_OUT)

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

run: build
	./$(BUILD_DIR)/$(BINARY)

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)
