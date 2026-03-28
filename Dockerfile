# Build stage
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev protobuf protobuf-dev

# Install protoc Go plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Generate proto files
RUN protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    api/grpc/proto/limyedb.proto

# Build the binary
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')" \
    -o /app/bin/limyedb \
    ./cmd/limyedb

# Final stage
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 limyedb && \
    adduser -u 1000 -G limyedb -s /bin/sh -D limyedb

# Create data directory
RUN mkdir -p /data /config && chown -R limyedb:limyedb /data /config

# Copy binary from builder
COPY --from=builder /app/bin/limyedb /usr/local/bin/limyedb

# Copy default config
COPY --from=builder /app/config.yaml /config/config.yaml 2>/dev/null || true

# Set user
USER limyedb

# Expose ports
# 8080 - REST API
# 50051 - gRPC API
# 7000 - Raft
# 7001 - Gossip
EXPOSE 8080 50051 7000 7001

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Data volume
VOLUME ["/data"]

# Set working directory
WORKDIR /data

# Entry point
ENTRYPOINT ["limyedb"]

# Default command
CMD ["--rest-addr=0.0.0.0:8080", "--grpc-addr=0.0.0.0:50051", "--data-dir=/data"]
