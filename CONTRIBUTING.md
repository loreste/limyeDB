# Contributing to LimyeDB

Thank you for your interest in contributing to LimyeDB! This document provides guidelines and information for contributors.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Documentation](#documentation)

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment for everyone. Be kind, constructive, and professional in all interactions.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/limyeDB.git
   cd limyeDB
   ```
3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/loreste/limyeDB.git
   ```

## Development Setup

### Prerequisites

- Go 1.26 or later
- Make
- Docker (optional, for container builds)
- protoc (for Protocol Buffer changes)

### Building

```bash
# Install dependencies
make deps

# Build the binary
make build

# Run tests
make test

# Run linter
make lint
```

### Running Locally

```bash
# Single node
make run

# Three-node cluster
make run-cluster
```

## Making Changes

### Branch Naming

Use descriptive branch names:
- `feature/add-new-index-type`
- `fix/search-performance-issue`
- `docs/update-api-reference`
- `refactor/cleanup-storage-layer`

### Commit Messages

Follow conventional commit format:

```
type(scope): brief description

Longer description if needed.

Fixes #123
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `perf`: Performance improvements
- `chore`: Maintenance tasks

### Before Submitting

1. **Update your branch**:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Run all checks**:
   ```bash
   make fmt
   make lint
   make test
   ```

3. **Ensure tests pass**:
   ```bash
   make test
   ```

## Pull Request Process

1. **Create a pull request** against `main` branch
2. **Fill out the PR template** with:
   - Description of changes
   - Related issues
   - Testing performed
   - Screenshots (if UI changes)

3. **Wait for review**:
   - At least one maintainer approval required
   - All CI checks must pass
   - Address review feedback promptly

4. **After approval**:
   - Squash commits if requested
   - Maintainer will merge

## Coding Standards

### Go Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Use meaningful variable and function names
- Add comments for exported functions and types

### File Organization

```
pkg/
  component/
    component.go       # Main implementation
    component_test.go  # Unit tests
    types.go          # Type definitions
```

### Error Handling

```go
// Good
if err != nil {
    return fmt.Errorf("failed to create index: %w", err)
}

// Avoid
if err != nil {
    return err
}
```

### Testing

```go
func TestFeatureName(t *testing.T) {
    // Arrange
    input := ...

    // Act
    result, err := Function(input)

    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result != expected {
        t.Errorf("got %v, want %v", result, expected)
    }
}
```

## Testing

### Unit Tests

```bash
# Run all tests
make test

# Run specific package
go test -v ./pkg/index/hnsw/...

# Run with coverage
make coverage
```

### Integration Tests

```bash
make test-integration
```

### Benchmarks

```bash
make bench
```

## Documentation

### Code Documentation

- Document all exported types and functions
- Use godoc format:

```go
// Search performs a k-NN search on the HNSW index.
// It returns the k nearest neighbors to the query vector.
//
// Parameters:
//   - query: The query vector
//   - k: Number of neighbors to return
//   - ef: Search quality parameter (higher = better recall)
//
// Returns the search results sorted by distance.
func (h *HNSW) Search(query []float32, k, ef int) ([]Result, error) {
    ...
}
```

### README Updates

- Keep README.md up to date with new features
- Add examples for new functionality
- Update API reference as needed

## Questions?

- Open a GitHub issue for bugs or feature requests
- Join our Discord for discussions
- Check existing issues before creating new ones

Thank you for contributing to LimyeDB!
