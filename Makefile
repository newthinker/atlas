.PHONY: build run test clean

BINARY=atlas
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/atlas

run: build
	./$(BUILD_DIR)/$(BINARY)

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)
