# xpm Makefile
# Universal Package Manager build and development commands

# Build variables
BINARY_NAME=xpm
# Version can be set via: make build VERSION=1.0.0
# Defaults to git tag, or 0.0.1 if no tags exist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.1")
LDFLAGS=-trimpath -ldflags "-s -w -X github.com/crenspire/xpm/internal/cli.Version=$(VERSION)"

# Go variables
GO=go
GOFLAGS=-v
GOTEST=$(GO) test
GOBUILD=$(GO) build $(GOFLAGS) $(LDFLAGS)

# Directories
BUILD_DIR=build
CMD_DIR=cmd/xpm

# Default target
.DEFAULT_GOAL := build

# Help target
.PHONY: help
help:
	@echo "xpm - Universal Package Manager"
	@echo ""
	@echo "Usage:"
	@echo "  make build       Build the binary"
	@echo "  make install     Install to GOPATH/bin"
	@echo "  make test        Run tests"
	@echo "  make test-cover  Run tests with coverage"
	@echo "  make lint        Run linters"
	@echo "  make fmt         Format code"
	@echo "  make clean       Remove build artifacts"
	@echo "  make release     Build release binaries for all platforms"
	@echo "  make deps        Download dependencies"
	@echo "  make tidy        Run go mod tidy"
	@echo ""

# Build the binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) -o $(BINARY_NAME) ./$(CMD_DIR)

# Install to GOPATH/bin
.PHONY: install
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GO) install $(LDFLAGS) ./$(CMD_DIR)

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
.PHONY: test-cover
test-cover:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run benchmarks
.PHONY: bench
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Check CLI latency/size against roadmap budgets (needs hyperfine + jq + network)
.PHONY: perf
perf:
	./scripts/perf.sh

# Run linters
.PHONY: lint
lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, running go vet..."; \
		$(GO) vet ./...; \
	fi

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	fi

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Download dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	$(GO) mod download

# Tidy dependencies
.PHONY: tidy
tidy:
	@echo "Tidying modules..."
	$(GO) mod tidy

# Build release binaries for all platforms
.PHONY: release
release: clean
	@echo "Building release binaries..."
	@mkdir -p $(BUILD_DIR)
	
	# Linux AMD64
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	
	# Linux ARM64
	GOOS=linux GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./$(CMD_DIR)
	
	# macOS AMD64
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./$(CMD_DIR)
	
	# macOS ARM64 (Apple Silicon)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)
	
	# Windows AMD64
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
	
	@echo "Release binaries built in $(BUILD_DIR)/"
	@ls -la $(BUILD_DIR)/

# Create checksums for release binaries
.PHONY: checksums
checksums:
	@echo "Creating checksums..."
	@cd $(BUILD_DIR) && sha256sum * > checksums.txt
	@cat $(BUILD_DIR)/checksums.txt

# Run the binary
.PHONY: run
run: build
	./$(BINARY_NAME)

# Development: run with verbose logging
.PHONY: dev
dev: build
	./$(BINARY_NAME) -v $(ARGS)

# Check for updates to dependencies
.PHONY: check-updates
check-updates:
	@echo "Checking for dependency updates..."
	$(GO) list -u -m all

# Generate (if needed)
.PHONY: generate
generate:
	@echo "Running go generate..."
	$(GO) generate ./...

# Verify (build and test)
.PHONY: verify
verify: fmt lint test build
	@echo "Verification complete!"

# Docker build (optional)
.PHONY: docker
docker:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) .

# All: clean, format, lint, test, and build
.PHONY: all
all: clean fmt lint test build

