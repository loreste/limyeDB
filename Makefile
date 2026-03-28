.PHONY: build run test bench clean proto fmt lint docker help

# Binary name
BINARY=limyedb
CLI_BINARY=limyedb-cli

# Build directory
BUILD_DIR=bin

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build flags
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Proto files
PROTO_DIR=api/grpc/proto

# Docker
DOCKER_IMAGE=limyedb/limyedb
DOCKER_TAG ?= latest

# Test packages (matches CI — excludes api/grpc and cmd/limyedb)
TEST_PKGS=./pkg/... ./internal/... ./api/rest/... ./cmd/limyedb-cli/...

all: build

## build: Build limyedb server and CLI into bin/
build:
	@echo "Building $(BINARY) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/limyedb
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY) ./cmd/limyedb-cli

## test: Run tests with race detector (same scope as CI)
test:
	@echo "Running tests..."
	$(GOTEST) -race $(TEST_PKGS)

## bench: Run benchmarks on key packages
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./pkg/... ./internal/...

## lint: Run golangci-lint with project config
lint:
	@echo "Running linter..."
	golangci-lint run ./...

## fmt: Format code with gofmt and goimports
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .
	@command -v goimports >/dev/null 2>&1 && goimports -w . || true

## proto: Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/*.proto

## docker: Build Docker image
docker:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)

## help: Show this help
help:
	@echo "LimyeDB - Distributed Vector Database"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'
