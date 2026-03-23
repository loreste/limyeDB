.PHONY: build run test bench clean proto fmt lint

# Binary name
BINARY=limyedb

# Build directory
BUILD_DIR=bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build flags
LDFLAGS=-ldflags "-s -w"

# Proto files
PROTO_DIR=api/grpc/proto

all: build

build:
	@echo "Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/limyedb

run: build
	@echo "Running $(BINARY)..."
	./$(BUILD_DIR)/$(BINARY)

test:
	@echo "Running tests..."
	$(GOTEST) -v -race ./...

bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./test/benchmark/

coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

proto:
	@echo "Generating protobuf code..."
	protoc --go_out=. --go-grpc_out=. $(PROTO_DIR)/*.proto

fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

lint:
	@echo "Running linter..."
	golangci-lint run ./...

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

docker-build:
	@echo "Building Docker image..."
	docker build -t limyedb:latest .

docker-run:
	@echo "Running Docker container..."
	docker run -p 8080:8080 -p 50051:50051 limyedb:latest

help:
	@echo "Available targets:"
	@echo "  build      - Build the binary"
	@echo "  run        - Build and run"
	@echo "  test       - Run tests"
	@echo "  bench      - Run benchmarks"
	@echo "  coverage   - Generate test coverage report"
	@echo "  proto      - Generate protobuf code"
	@echo "  fmt        - Format code"
	@echo "  lint       - Run linter"
	@echo "  clean      - Clean build artifacts"
	@echo "  deps       - Download dependencies"
