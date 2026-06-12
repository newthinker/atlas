.PHONY: build run test clean export-signals signal-eval

BINARY=atlas
BUILD_DIR=bin

SIGNAL_SYMBOLS ?= 600519.SH,000300.SH
SIGNAL_FROM    ?= 2021-01-01
SIGNAL_TO      ?= 2026-06-01

# Python 评估链：系统 python3 已损坏，统一走预置 venv（3.11 + pandas + pytest）
QLIB_PY        ?= scripts/qlib_eval/.venv/bin/python
QLIB_DIR       ?= ~/.qlib/qlib_data/cn_data
SIGNAL_OUT     ?= reports/

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

run: build
	./$(BUILD_DIR)/$(BINARY)

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)
