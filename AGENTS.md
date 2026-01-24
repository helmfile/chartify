# Chartify - Agents Guide

This guide helps agentic coding agents work effectively with the chartify codebase.

## Build, Lint, and Test Commands

### Build
- **Build CLI**: `go build -o chartify ./cmd/chartify` (25s, set timeout 60s+)
- **Build chart repo server**: `go build -o chartreposerver ./cmd/chartreposerver`
- **Download dependencies**: `go mod download` (15s, set timeout 120s+)

### Common Build Times and Timeouts
- `go mod download`: 15 seconds (set timeout: 120+ seconds)
- `go build`: 25 seconds (set timeout: 60+ seconds)
- `go test ./...`: 60 seconds (set timeout: 180+ seconds)
- `go vet ./...`: 42 seconds (set timeout: 120+ seconds)
- `go fmt ./...`: 1 second (set timeout: 30+ seconds)
- **NEVER CANCEL** build commands - they complete predictably

### Testing
- **Run all tests**: `CGO_ENABLED=0 go test ./...` (60s, set timeout 180s+)
- **Run with verbose output**: `RETAIN_TEMP_DIR=1 go test -v ./...`
- **Run single test**: `go test -run TestFunctionName ./...`
- **Run single test file**: `go test -v chartify_test.go`
- **Use Makefile**: `make test` or `make test/verbose`

### Linting and Formatting
- **Format code**: `go fmt ./...`
- **Vet code**: `go vet ./...` (42s, set timeout 120s+)
- **NOTE**: golangci-lint config (.golangci.yaml) has compatibility issues with recent versions. Use `go fmt` and `go vet` for validation.

### Running the CLI
- Basic usage: `./chartify -o OUTPUT_DIR RELEASE_NAME CHART_OR_MANIFEST_PATH`
- Example with Helm chart: `./chartify -o /tmp/output test-release testdata/charts/log`
- Example with K8s manifests: `./chartify -o /tmp/output test-release testdata/kube_manifest_yml`
- Help: `./chartify -h`

### Running the Chart Repo Server
- Usage: `./chartreposerver CHARTS_DIR`
- Example: `./chartreposerver testdata/charts` (serves charts on port 18080)
- No help flag available - expects exactly 1 argument (charts directory)

## Code Style Guidelines

### Imports
- Order: standard library → third-party → local (github.com/helmfile/chartify prefix)
- Use `goimports` or `gci` formatter for consistent ordering
- Example:
  ```go
  import (
      "fmt"
      "os"

      "github.com/Masterminds/semver/v3"
      "helm.sh/helm/v3/pkg/registry"

      "github.com/helmfile/chartify"
  )
  ```

### Formatting
- Use `go fmt ./...` before committing
- Line length: 120 characters (per golangci.yaml)
- Use `gofmt` with simplify: true

### Types and Naming
- Use PascalCase for exported types, functions, constants
- Use camelCase for unexported variables and fields
- Type names should be descriptive (e.g., `ChartifyOpts`, `PatchOpts`)
- Interface names typically end with "Option" when used for functional options pattern
- Use `func(t *testing.T)` and `t.Helper()` for test helpers

### Error Handling
- Always check and return errors explicitly
- Use early returns for error conditions
- Pattern:
  ```go
  if err != nil {
      return "", err
  }
  ```
- In tests, use `require.NoError(t, err)` or `require.Error(t, err)` from testify
- Use `require.Containsf(t, err.Error(), "msg")` to check error messages

### Testing Conventions
- Use `github.com/stretchr/testify/require` for assertions
- Use `github.com/google/go-cmp/cmp` for comparing complex structs
- Test functions: `func TestFeatureName(t *testing.T)`
- Integration tests in `integration_test.go`
- Use `RETAIN_TEMP_DIR=1` env var to keep temp directories for debugging
- Setup functions use `t.Helper()` and are called at start of tests

### Function Options Pattern
- The codebase uses functional options extensively
- Pattern: `type Option func(*Runner) error`
- Implementation: `func HelmBin(b string) Option { return func(r *Runner) error { r.HelmBinary = b; return nil } }`
- Usage: `New(HelmBin("helm"), UseHelm3(true))`

### Comments and Documentation
- Exported types and functions should have godoc comments
- Comment explains "what" and "why", not "how"
- Use `// nolint` sparingly when intentionally bypassing linters

### File Organization
- `chartify.go` - Core library functionality
- `kustomize.go` - Kustomize integration
- `patch.go` - Strategic merge and JSON patch logic
- `replace.go` - Text replacement functionality
- `requirements.go` - Chart dependency management
- `runner.go` - Command execution utilities
- `cmd/chartify/main.go` - CLI entry point
- `cmd/chartreposerver/main.go` - HTTP server for charts

### Validation After Changes
**Always manually test CLI functionality after making changes to core library code:**
- Test with Helm chart: `./chartify -o /tmp/test1 test-release testdata/charts/log && ls /tmp/test1/`
- Test with K8s manifests: `./chartify -o /tmp/test2 test-release testdata/kube_manifest_yml && find /tmp/test2/ -type f`
- Verify help: `./chartify -h`

### Before Committing
Run these commands to ensure CI passes:
```bash
go fmt ./...
go vet ./...
CGO_ENABLED=0 go test ./...
```

### Prerequisites
- Go 1.25.4+
- Helm v4.1.0 (helm command)
- Kustomize v5.8.0+ (kustomize command)

### External Dependencies
- **Helm**: Required for chart operations. Usually pre-installed as `helm` command.
- **Kustomize**: Required for kustomize integration. Usually pre-installed as `kustomize` command.
- **Note**: Helm repo add commands may fail in restricted network environments but tests can still pass.

### Common Issues
- Helm repo operations may fail in restricted network environments - core functionality still works
- Some complex kustomize examples may fail helm template validation - this is expected
- Use environment variable `HELM_BIN` to override helm binary path for testing
- Never cancel build commands - they take predictable time (see timeouts above)

### Debugging
- Use `RETAIN_TEMP_DIR=1` environment variable to keep temporary directories for debugging
- Build output goes to temporary directories that are automatically cleaned up
- The CLI expects exactly 2 positional arguments: RELEASE_NAME and CHART_PATH, with -o flag for output directory
