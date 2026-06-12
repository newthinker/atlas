.PHONY: build run test clean export-signals

BINARY=atlas
BUILD_DIR=bin

SIGNAL_SYMBOLS ?= 600519.SH,000300.SH
SIGNAL_FROM    ?= 2021-01-01
SIGNAL_TO      ?= 2026-06-01

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/atlas

export-signals: build
	./bin/atlas export-signals --strategies ma_crossover,price_percentile \
	  --symbols $(SIGNAL_SYMBOLS) --from $(SIGNAL_FROM) --to $(SIGNAL_TO) --out signals.csv

run: build
	./$(BUILD_DIR)/$(BINARY)

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)
