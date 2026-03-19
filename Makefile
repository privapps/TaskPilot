.PHONY: help build build-all clean test lint install-tools

# Go build variables
GO := go
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
# SQLite driver is pure-Go (modernc.org/sqlite); CGO not required.
# macOS native builds may keep CGO on; all cross-compilation uses CGO_ENABLED=0.
CGO_ENABLED ?= 0
LDFLAGS ?= -s -w
VERSION ?= dev
OUTPUT_DIR := build/bin
BINARY_NAME := taskpilot

# Target platforms and architectures
PLATFORMS := \
	darwin-amd64 \
	darwin-arm64 \
	linux-amd64 \
	linux-arm64 \
	windows-amd64 \
	windows-arm64

help:
	@echo "TaskPilot Go Build System"
	@echo "========================="
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  help              Show this help message"
	@echo "  build             Build for current platform (GOOS=$(GOOS) GOARCH=$(GOARCH))"
	@echo "  build-all         Cross-compile for all platforms"
	@echo "  build-linux       Build Linux binaries (amd64, arm64)"
	@echo "  build-windows     Build Windows binaries (amd64, arm64)"
	@echo "  build-darwin      Build macOS binaries (amd64, arm64)"
	@echo "  clean             Remove build artifacts"
	@echo "  test              Run Go tests"
	@echo "  lint              Run Go linter"
	@echo "  install-tools     Install build tools (if needed)"
	@echo ""
	@echo "  Uses pure-Go SQLite (modernc.org/sqlite): no C toolchain required."
	@echo ""
	@echo "Examples:"
	@echo "  make build                        # Build for current OS/arch"
	@echo "  make build-linux                  # Cross-compile Linux x64 + ARM64"
	@echo "  make build-all                    # Build all 6 platform binaries"
	@echo "  GOOS=linux GOARCH=amd64 make build"

build:
	@echo "[BUILD] $(GOOS)/$(GOARCH) → $(OUTPUT_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)"
	@mkdir -p $(OUTPUT_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH) .
	@echo "[✓] Built successfully"

# Build specific platform combinations
build-linux-amd64:
	@$(MAKE) build GOOS=linux GOARCH=amd64

build-linux-arm64:
	@$(MAKE) build GOOS=linux GOARCH=arm64

build-darwin-amd64:
	@$(MAKE) build GOOS=darwin GOARCH=amd64

build-darwin-arm64:
	@$(MAKE) build GOOS=darwin GOARCH=arm64

build-windows-amd64:
	@$(MAKE) build GOOS=windows GOARCH=amd64

build-windows-arm64:
	@$(MAKE) build GOOS=windows GOARCH=arm64

# Build all platforms
build-linux: build-linux-amd64 build-linux-arm64
	@echo "[✓] Linux binaries built"

build-windows: build-windows-amd64 build-windows-arm64
	@echo "[✓] Windows binaries built"

build-darwin: build-darwin-amd64 build-darwin-arm64
	@echo "[✓] macOS binaries built"

build-all: build-linux build-windows build-darwin
	@echo ""
	@echo "[✓] All platforms built successfully!"
	@echo ""
	@ls -lh $(OUTPUT_DIR)/$(BINARY_NAME)-*
	@echo ""

# Testing and linting
test:
	@echo "[TEST] Running Go tests..."
	$(GO) test -v -race -timeout 30s ./...
	@echo "[✓] Tests passed"

lint:
	@echo "[LINT] Running Go linter..."
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found, installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	}
	golangci-lint run ./...
	@echo "[✓] Linting passed"

install-tools:
	@echo "[INSTALL] Installing build tools..."
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "[✓] Build tools installed"

# Cleanup
clean:
	@echo "[CLEAN] Removing build artifacts..."
	$(GO) clean
	rm -rf $(OUTPUT_DIR)
	@echo "[✓] Cleaned"

# Verbose build for debugging
build-verbose:
	@echo "[BUILD-VERBOSE] $(GOOS)/$(GOARCH)"
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -v -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH) .

# Version info
version:
	@echo "TaskPilot $(VERSION)"
	@$(GO) version

.DEFAULT_GOAL := help
