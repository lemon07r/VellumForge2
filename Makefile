.PHONY: all build test lint clean install run fmt vet check deps help
.PHONY: build-all build-linux build-darwin build-windows test-coverage test-integration
.PHONY: lint-fix security pre-commit ci

BINARY_NAME=vellumforge2
VERSION?=1.5.3
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# Colors for output
COLOR_RESET=\033[0m
COLOR_BOLD=\033[1m
COLOR_GREEN=\033[32m
COLOR_YELLOW=\033[33m
COLOR_BLUE=\033[34m

define print_step
	@echo -e "$(COLOR_BOLD)$(COLOR_BLUE)==> $(1)$(COLOR_RESET)"
endef

## all: Run all checks and build (optimal order for development)
all: deps fmt vet lint test build
	$(call print_step,"✅ All checks passed! Binary built successfully.")

## check: Run all quality checks without building (faster CI)
check: fmt-check vet lint test
	$(call print_step,"✅ All quality checks passed!")

## pre-commit: Fast checks before committing (skip tests)
pre-commit: fmt vet lint
	$(call print_step,"✅ Pre-commit checks passed!")

## ci: Full CI pipeline (format check + all validations + coverage)
ci: fmt-check vet lint test-coverage build
	$(call print_step,"✅ CI pipeline completed successfully!")

build:
	$(call print_step,"Building $(BINARY_NAME)...")
	@go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/vellumforge2
	$(call print_step,"Built: bin/$(BINARY_NAME)")

build-linux:
	@echo "Building $(BINARY_NAME) for Linux..."
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/vellumforge2

build-darwin:
	@echo "Building $(BINARY_NAME) for macOS..."
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/vellumforge2
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/vellumforge2

build-windows:
	@echo "Building $(BINARY_NAME) for Windows..."
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/vellumforge2

build-all: build-linux build-darwin build-windows

test:
	$(call print_step,"Running unit tests - skips integration tests...")
	@go test -short -race -coverprofile=coverage.out ./...
	@echo -e "$(COLOR_GREEN)Total coverage: $$(go tool cover -func=coverage.out | grep total | awk '{print $$3}')$(COLOR_RESET)"
	$(call print_step,"✓ Tests passed")

test-integration:
	$(call print_step,"Running ALL tests including integration tests - makes real API calls...")
	@go test -race -coverprofile=coverage.out ./...
	@echo -e "$(COLOR_GREEN)Total coverage: $$(go tool cover -func=coverage.out | grep total | awk '{print $$3}')$(COLOR_RESET)"
	$(call print_step,"✓ All tests passed")

test-coverage:
	$(call print_step,"Running ALL tests with coverage report - makes real API calls...")
	@go test -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo -e "$(COLOR_GREEN)Total coverage: $$(go tool cover -func=coverage.out | grep total | awk '{print $$3}')$(COLOR_RESET)"
	$(call print_step,"✓ Coverage report generated: coverage.html")

test-short:
	$(call print_step,"Running short tests - alias for test...")
	@go test -short -race ./...
	$(call print_step,"✓ Short tests passed")

test-verbose:
	$(call print_step,"Running tests with verbose output...")
	@go test -v -race -coverprofile=coverage.out -count=1 ./...
	$(call print_step,"✓ Verbose tests completed")

lint:
	$(call print_step,"Running golangci-lint...")
	@golangci-lint run ./...
	$(call print_step,"✓ Linting passed")

lint-fix:
	$(call print_step,"Running golangci-lint with auto-fix...")
	@golangci-lint run --fix ./...
	$(call print_step,"✓ Linting with auto-fix completed")

security:
	$(call print_step,"Running security checks...")
	@golangci-lint run --disable-all --enable gosec ./...
	$(call print_step,"✓ Security checks passed")

clean:
	$(call print_step,"Cleaning build artifacts...")
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	$(call print_step,"✓ Clean completed")

install:
	$(call print_step,"Installing dependencies...")
	@go mod download
	@go mod tidy
	$(call print_step,"✓ Dependencies installed")

deps:
	$(call print_step,"Verifying dependencies...")
	@go mod download
	@go mod verify
	$(call print_step,"✓ Dependencies verified")

run:
	@go run ./cmd/vellumforge2 run --config configs/config.example.toml

fmt:
	$(call print_step,"Formatting code...")
	@go fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w -local github.com/lamim/vellumforge2 .; \
	elif [ -f "$(shell go env GOPATH)/bin/goimports" ]; then \
		$(shell go env GOPATH)/bin/goimports -w -local github.com/lamim/vellumforge2 .; \
	else \
		echo -e "$(COLOR_YELLOW)Warning: goimports not found, run: go install golang.org/x/tools/cmd/goimports@latest$(COLOR_RESET)"; \
	fi
	$(call print_step,"✓ Code formatted")

fmt-check:
	$(call print_step,"Checking code formatting...")
	@test -z "$$(gofmt -l .)" || (echo -e "$(COLOR_YELLOW)Files not formatted:$(COLOR_RESET)" && gofmt -l . && exit 1)
	$(call print_step,"✓ Code formatting check passed")

vet:
	$(call print_step,"Running go vet...")
	@go vet ./...
	$(call print_step,"✓ Go vet passed")

help:
	@echo -e "$(COLOR_BOLD)VellumForge2 Makefile Commands$(COLOR_RESET)"
	@echo ""
	@echo -e "$(COLOR_BOLD)Primary Commands:$(COLOR_RESET)"
	@echo -e "  $(COLOR_GREEN)make all$(COLOR_RESET)          - Run full pipeline: deps → fmt → vet → lint → test → build"
	@echo -e "  $(COLOR_GREEN)make check$(COLOR_RESET)        - Run quality checks without building (faster)"
	@echo -e "  $(COLOR_GREEN)make pre-commit$(COLOR_RESET)   - Quick checks before committing (fmt → vet → lint)"
	@echo -e "  $(COLOR_GREEN)make ci$(COLOR_RESET)           - Full CI pipeline with coverage"
	@echo ""
	@echo -e "$(COLOR_BOLD)Build Commands:$(COLOR_RESET)"
	@echo "  make build           - Build binary for current platform"
	@echo "  make build-all       - Build for all platforms (Linux, macOS, Windows)"
	@echo "  make build-linux     - Build for Linux amd64"
	@echo "  make build-darwin    - Build for macOS (amd64 + arm64)"
	@echo "  make build-windows   - Build for Windows amd64"
	@echo ""
	@echo -e "$(COLOR_BOLD)Quality Commands:$(COLOR_RESET)"
	@echo "  make fmt             - Format code with gofmt and goimports"
	@echo "  make fmt-check       - Check if code is formatted (CI)"
	@echo "  make vet             - Run go vet"
	@echo "  make lint            - Run golangci-lint"
	@echo "  make lint-fix        - Run golangci-lint with auto-fix"
	@echo "  make security        - Run security checks (gosec)"
	@echo ""
	@echo -e "$(COLOR_BOLD)Test Commands:$(COLOR_RESET)"
	@echo "  make test            - Run unit tests (fast, skips integration tests)"
	@echo "  make test-integration - Run ALL tests including integration tests (slow, makes API calls)"
	@echo "  make test-coverage   - Run ALL tests and generate HTML coverage report (slow)"
	@echo "  make test-short      - Alias for 'make test'"
	@echo "  make test-verbose    - Run tests with verbose output"
	@echo ""
	@echo -e "$(COLOR_BOLD)Utility Commands:$(COLOR_RESET)"
	@echo "  make deps            - Verify and download dependencies"
	@echo "  make install         - Install and tidy dependencies"
	@echo "  make clean           - Remove build artifacts and coverage files"
	@echo "  make run             - Run with example config"
	@echo "  make help            - Show this help message"
	@echo ""
	@echo -e "$(COLOR_BOLD)Examples:$(COLOR_RESET)"
	@echo "  make all             # Run everything before pushing"
	@echo "  make pre-commit      # Quick check before commit"
	@echo "  make test-coverage   # Generate coverage report"
	@echo "  make lint-fix        # Auto-fix linting issues"
