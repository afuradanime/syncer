PROJECT_NAME ?= anime-syncer

GO ?= go
PYTHON ?= python3
STATICCHECK ?= staticcheck
CONFIG ?= config.ini
TEST_FLAGS ?= -v
SYSTEM_TEST_FLAGS ?= -v

BIN_DIR ?= ./bin
BINARY := $(BIN_DIR)/$(PROJECT_NAME)

.PHONY: help
help:
	@echo "Available targets:"
	@echo ""
	@echo "  build              Build the project"
	@echo "  run-full           Sync all anime and manga"
	@echo "  run-partial        Refresh active entries and scan for new items"
	@echo "  test               Run Go tests"
	@echo "  test-system        Build the binary and run Python system tests"
	@echo "  test-race          Run tests with race detector"
	@echo "  fmt                Format Go source files"
	@echo "  vet                Run go vet"
	@echo "  lint               Run staticcheck"
	@echo "  check              Run formatting, vet, lint and tests"
	@echo "  tidy               Tidy Go dependencies"
	@echo "  clean              Remove build artifacts"
	@echo "  trace              Trace the codebase with open fast trace"
	@echo "  trace-report       Generate an HTML report from the trace"
	@echo ""
	@echo "Run targets require DATABASE_PATH; CONFIG defaults to config.ini."
	@echo "Override test options with TEST_FLAGS (default: -v)."

# Build
.PHONY: build
build:
	@mkdir -p "$(BIN_DIR)"
	@$(GO) build -o "$(BINARY)" ./src
	@echo "Executable saved to $(BINARY)"

.PHONY: run-full run_full
run-full run_full:
	@$(GO) run ./src --config "$(CONFIG)" --mode full

.PHONY: run-partial run_partial
run-partial run_partial:
	@$(GO) run ./src --config "$(CONFIG)" --mode partial

.PHONY: test
test:
	@$(GO) test $(TEST_FLAGS) ./...

.PHONY: test-race
test-race:
	@$(GO) test -race $(TEST_FLAGS) ./...

.PHONY: test-system
test-system: build
	@SYNCER_BINARY="$(abspath $(BINARY))" $(PYTHON) -m pytest tests/system $(SYSTEM_TEST_FLAGS)

.PHONY: fmt
fmt:
	@$(GO) fmt ./...

.PHONY: vet
vet:
	@$(GO) vet ./...

.PHONY: lint
lint:
	@$(STATICCHECK) ./...

.PHONY: check
check: fmt vet lint test test-race test-system

.PHONY: tidy
tidy:
	@$(GO) mod tidy

.PHONY: clean
clean:
	@rm -rf "$(BIN_DIR)"

# Open Fast Trace
JAVA ?= java
OFT ?= ./tools/openfasttrace-4.11.0.jar
OFT_REPORT ?= trace-report.html

.PHONY: trace
trace:
	@$(JAVA) -jar $(OFT) trace .

.PHONY: trace-report
trace-report:
	@$(JAVA) -jar $(OFT) trace -o html -f $(OFT_REPORT) --report-verbosity all .

.DEFAULT_GOAL := help