PROJECT_NAME := anime-syncer

GO := go

BIN_DIR := ./bin
BINARY := $(BIN_DIR)/$(PROJECT_NAME)

.PHONY: help
help:
	@echo "Available targets:"
	@echo ""
	@echo "  build              Build the project"
	@echo "  run_full			Run the project in full mode"
	@echo "  run_partial		Run the project in partial mode"
	@echo "  test               Run unit tests"
	@echo "  test-race          Run tests with race detector"
	@echo "  fmt                Format Go source files"
	@echo "  vet                Run go vet"
	@echo "  lint               Run staticcheck"
	@echo "  check              Run formatting, vet, lint and tests"
	@echo "  tidy               Tidy Go dependencies"
	@echo "  clean              Remove build artifacts"

# Build
.PHONY: build
build:
	@mkdir -p $(BIN_DIR)
	@$(GO) build -o $(BINARY) ./src
	@echo "Executable saved to $(BINARY)"

.PHONY: run_full
run_full:
	@$(GO) run ./src --config config.ini --mode full

.PHONY: run_partial
run_partial:
	@$(GO) run ./src --config config.ini --mode partial

.PHONY: test
test:
	@$(GO) test ./... -v

.PHONY: test-race
test-race:
	@$(GO) test ./... -race -v

.PHONY: fmt
fmt:
	@$(GO) fmt ./...

.PHONY: vet
vet:
	@$(GO) vet ./...

.PHONY: lint
lint:
	@staticcheck ./...

.PHONY: check
check: fmt vet lint test

.PHONY: tidy
tidy:
	@$(GO) mod tidy

.PHONY: clean
clean:
	@rm -rf $(BIN_DIR)

.DEFAULT_GOAL := help