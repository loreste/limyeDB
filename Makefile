.PHONY: build run test bench clean proto fmt lint docker docker-cluster helm git-commit git-push

# Binary name
BINARY=limyedb

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
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build flags
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Proto files
PROTO_DIR=api/grpc/proto

# Docker
DOCKER_IMAGE=limyedb/limyedb
DOCKER_TAG ?= latest

all: build

build:
	@echo "Building $(BINARY) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/limyedb

build-linux:
	@echo "Building $(BINARY) for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/limyedb

build-darwin:
	@echo "Building $(BINARY) for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 ./cmd/limyedb
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./cmd/limyedb

build-all: build-linux build-darwin
	@echo "Built all platforms"

run: build
	@echo "Running $(BINARY)..."
	./$(BUILD_DIR)/$(BINARY)

run-cluster: build
	@echo "Starting 3-node cluster..."
	@mkdir -p data/node1 data/node2 data/node3
	./$(BUILD_DIR)/$(BINARY) --raft-node-id=node1 --raft-bind=127.0.0.1:7201 --raft-data=./data/node1 --raft-bootstrap=true --rest-addr=127.0.0.1:8081 &
	@sleep 2
	./$(BUILD_DIR)/$(BINARY) --raft-node-id=node2 --raft-bind=127.0.0.1:7202 --raft-data=./data/node2 --raft-join=http://127.0.0.1:8081 --rest-addr=127.0.0.1:8082 &
	@sleep 1
	./$(BUILD_DIR)/$(BINARY) --raft-node-id=node3 --raft-bind=127.0.0.1:7203 --raft-data=./data/node3 --raft-join=http://127.0.0.1:8081 --rest-addr=127.0.0.1:8083 &
	@echo "Cluster started on ports 8081, 8082, 8083"

test:
	@echo "Running tests..."
	$(GOTEST) -v -race ./...

test-short:
	@echo "Running short tests..."
	$(GOTEST) -v -short ./...

test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -tags=integration ./test/integration/...

bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./test/benchmark/

bench-cpu:
	@echo "Running benchmarks with CPU profile..."
	$(GOTEST) -bench=. -benchmem -cpuprofile=cpu.out ./test/benchmark/
	$(GOCMD) tool pprof -http=:8090 cpu.out

bench-mem:
	@echo "Running benchmarks with memory profile..."
	$(GOTEST) -bench=. -benchmem -memprofile=mem.out ./test/benchmark/
	$(GOCMD) tool pprof -http=:8090 mem.out

coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

proto:
	@echo "Generating protobuf code..."
	protoc --go_out=. --go-grpc_out=. $(PROTO_DIR)/*.proto

fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

lint:
	@echo "Running linter..."
	golangci-lint run ./...

vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html cpu.out mem.out
	@rm -rf data/

deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

update-deps:
	@echo "Updating dependencies..."
	$(GOMOD) get -u ./...
	$(GOMOD) tidy

# Docker targets
docker:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

docker-push: docker
	@echo "Pushing Docker image..."
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)

docker-run:
	@echo "Running Docker container..."
	docker run -d --name limyedb -p 8080:8080 -p 6334:6334 -v limyedb_data:/data $(DOCKER_IMAGE):$(DOCKER_TAG)

docker-cluster:
	@echo "Starting Docker cluster..."
	docker-compose -f docker-compose.cluster.yml up -d

docker-cluster-down:
	@echo "Stopping Docker cluster..."
	docker-compose -f docker-compose.cluster.yml down -v

docker-logs:
	@echo "Showing Docker logs..."
	docker-compose logs -f

# Kubernetes/Helm targets
helm-lint:
	@echo "Linting Helm chart..."
	helm lint deploy/helm/limyedb

helm-template:
	@echo "Templating Helm chart..."
	helm template limyedb deploy/helm/limyedb

helm-install:
	@echo "Installing Helm chart..."
	helm install limyedb deploy/helm/limyedb --namespace limyedb --create-namespace

helm-upgrade:
	@echo "Upgrading Helm chart..."
	helm upgrade limyedb deploy/helm/limyedb --namespace limyedb

helm-uninstall:
	@echo "Uninstalling Helm chart..."
	helm uninstall limyedb --namespace limyedb

# Development tools
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Release
release: clean build-all
	@echo "Creating release..."
	@mkdir -p release
	@cp $(BUILD_DIR)/$(BINARY)-linux-amd64 release/
	@cp $(BUILD_DIR)/$(BINARY)-darwin-amd64 release/
	@cp $(BUILD_DIR)/$(BINARY)-darwin-arm64 release/
	@cd release && sha256sum * > checksums.txt
	@echo "Release artifacts in release/"

help:
	@echo "LimyeDB - Enterprise Distributed Vector Database"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build targets:"
	@echo "  build         - Build the binary"
	@echo "  build-linux   - Build for Linux"
	@echo "  build-darwin  - Build for macOS"
	@echo "  build-all     - Build for all platforms"
	@echo ""
	@echo "Run targets:"
	@echo "  run           - Build and run single node"
	@echo "  run-cluster   - Start a 3-node local cluster"
	@echo ""
	@echo "Test targets:"
	@echo "  test          - Run all tests"
	@echo "  test-short    - Run short tests"
	@echo "  test-integration - Run integration tests"
	@echo "  bench         - Run benchmarks"
	@echo "  coverage      - Generate coverage report"
	@echo ""
	@echo "Code quality:"
	@echo "  fmt           - Format code"
	@echo "  lint          - Run linter"
	@echo "  vet           - Run go vet"
	@echo ""
	@echo "Docker targets:"
	@echo "  docker        - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo "  docker-cluster - Start Docker cluster"
	@echo ""
	@echo "Helm targets:"
	@echo "  helm-lint     - Lint Helm chart"
	@echo "  helm-install  - Install Helm chart"
	@echo "  helm-upgrade  - Upgrade Helm chart"
	@echo ""
	@echo "Other:"
	@echo "  deps          - Download dependencies"
	@echo "  proto         - Generate protobuf code"
	@echo "  clean         - Clean build artifacts"
	@echo "  release       - Create release artifacts"
	@echo ""
	@echo "Git targets:"
	@echo "  git-commit    - Stage and commit all changes"
	@echo "  git-push      - Push to origin main"

# Git targets
git-commit:
	@echo "Staging all changes..."
	git add -A
	@echo "Committing..."
	git commit -m "Add comprehensive features: SDKs, observability, caching, rate limiting, webhooks, backup, admin UI, SIMD, quantization, multi-model"

git-push: git-commit
	@echo "Pushing to origin..."
	git push origin main
