# Go project variables
BINARY_NAME=frugal-llm
BUILD_DIR=bin
MAIN_PATH=cmd/proxy-server/main.go

# Versioning flags
VERSION?=0.0.1
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME) -s -w"

.PHONY: all build run clean test lint cross-compile docker-build help

all: build

## build: Builds the Go binary for current OS/Arch into bin/
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)"

## run: Runs the server directly
run:
	go run $(MAIN_PATH)

## dev: Runs the server with hot-reloading (requires 'air' installed: go install github.com/air-verse/air@latest)
dev:
	air

## clean: Cleans build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)

## test: Runs all tests
test:
	go test -v ./...

## lint: Runs golangci-lint if installed
lint:
	golangci-lint run ./...

## build-linux-amd64: Builds Linux x86_64/amd64 static binary
build-linux-amd64:
	@echo "Building Linux amd64 binary..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)

## build-linux-arm64: Builds Linux ARM64 (aarch64) static binary
build-linux-arm64:
	@echo "Building Linux arm64 binary..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)

## cross-compile: Builds lightweight static binaries for Linux, macOS (AMD64 & ARM64), and Windows
cross-compile:
	@echo "Cross-compiling binaries for all platforms..."
	@mkdir -p $(BUILD_DIR)
	# Linux amd64 (x86_64)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	# Linux arm64 (aarch64)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	# Darwin (macOS) amd64 (Intel)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	# Darwin (macOS) arm64 (Apple Silicon)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	# Windows amd64
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "Cross-compilation complete!"

## docker-build: Builds Docker image for current local platform
docker-build:
	docker build -t $(BINARY_NAME):$(VERSION) -t $(BINARY_NAME):latest .

## docker-build-amd64: Builds Docker image specifically for linux/amd64
docker-build-amd64:
	docker build --platform linux/amd64 -t $(BINARY_NAME):$(VERSION)-amd64 .

## docker-build-multiarch: Builds and exports multi-arch image (linux/amd64 and linux/arm64) using buildx
docker-build-multiarch:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(BINARY_NAME):$(VERSION) -t $(BINARY_NAME):latest .

## help: Shows available make commands
help:
	@echo "Available commands:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' |  sed -e 's/^/ /'
