# Chartify

Chartify is a Go library and CLI tool that converts Kubernetes resource YAMLs, Kustomize configurations, or existing Helm charts into new Helm charts with modifications. It is primarily used by Helmfile but can also be used as a standalone CLI.

**Always reference these instructions first and fallback to search or bash commands only when you encounter unexpected information that does not match the info here.**

## Working Effectively

### Bootstrap and Build
- **Prerequisites**: Go 1.24+, Helm v3.18.6, Kustomize v5.6.0+ are required
- Build the CLI: `go build -o chartify ./cmd/chartify` -- takes 25 seconds. **NEVER CANCEL**. Set timeout to 60+ seconds.
- Build dependencies: `go mod download` -- takes 15 seconds. **NEVER CANCEL**. Set timeout to 120+ seconds.
- Alternative build with chart repo server: `go build -o chartreposerver ./cmd/chartreposerver`

### Testing
- Run tests: `CGO_ENABLED=0 go test ./...` -- takes 60 seconds. **NEVER CANCEL**. Set timeout to 180+ seconds.
- Run verbose tests: `RETAIN_TEMP_DIR=1 go test -v ./...` -- takes 60 seconds. **NEVER CANCEL**. Set timeout to 180+ seconds.
- Use Makefile: `make test` or `make test/verbose`

### Linting and Formatting  
- Format code: `go fmt ./...` -- takes 1 second
- Vet code: `go vet ./...` -- takes 42 seconds. **NEVER CANCEL**. Set timeout to 120+ seconds.
- **WARNING**: golangci-lint config (.golangci.yaml) has compatibility issues with recent versions. Use `go fmt` and `go vet` for validation instead.

### Running the CLI
- Basic usage: `./chartify -o OUTPUT_DIR RELEASE_NAME CHART_OR_MANIFEST_PATH`
- Example with Helm chart: `./chartify -o /tmp/output test-release testdata/charts/log`
- Example with K8s manifests: `./chartify -o /tmp/output test-release testdata/kube_manifest_yml`
- Help: `./chartify -h`

### Running the Chart Repo Server
- Usage: `./chartreposerver CHARTS_DIR`
- Example: `./chartreposerver testdata/charts` (serves charts on port 18080)
- No help flag available - expects exactly 1 argument (charts directory)

## Validation
- **ALWAYS** manually test CLI functionality after making changes to core library code.
- **SCENARIO 1**: Test with existing Helm chart: `./chartify -o /tmp/test1 test-release testdata/charts/log && ls /tmp/test1/`
- **SCENARIO 2**: Test with Kubernetes manifests: `./chartify -o /tmp/test2 test-release testdata/kube_manifest_yml && find /tmp/test2/ -type f`
- **SCENARIO 3**: Verify help output: `./chartify -h`
- **SCENARIO 4**: Test chart repo server: `./chartreposerver testdata/charts` (serves on localhost:18080)
- **Note**: Some complex kustomize examples may fail helm template validation - this is expected behavior.
- Always run `go fmt ./...` and `go vet ./...` before committing or the CI (.github/workflows/lint.yaml and .github/workflows/go.yml) will fail.

## Key Projects and Files
- **Main library**: `chartify.go` - Core chartify functionality
- **CLI**: `cmd/chartify/main.go` - Command-line interface
- **Chart repo server**: `cmd/chartreposerver/main.go` - HTTP server for serving charts
- **Core modules**:
  - `kustomize.go` - Kustomize integration
  - `patch.go` - Patch application logic  
  - `replace.go` - Content replacement
  - `requirements.go` - Chart dependency management
- **Test data**: `testdata/` - Contains charts, manifests, and test cases for validation

## Common Build Times and Timeouts
- `go mod download`: 15 seconds (set timeout: 120+ seconds)
- `go build`: 25 seconds (set timeout: 60+ seconds)  
- `go test ./...`: 60 seconds (set timeout: 180+ seconds)
- `go vet ./...`: 42 seconds (set timeout: 120+ seconds)
- `go fmt ./...`: 1 second (set timeout: 30+ seconds)

## External Dependencies
- **Helm**: Required for chart operations. Usually pre-installed as `helm` command.
- **Kustomize**: Required for kustomize integration. Usually pre-installed as `kustomize` command.
- **Note**: Helm repo add commands may fail in restricted network environments but tests can still pass.

## Troubleshooting
- If helm repo operations fail, continue with testing - the core functionality does not require remote repositories.
- Build output goes to temporary directories that are automatically cleaned up.
- Use `RETAIN_TEMP_DIR=1` environment variable for debugging to keep temporary files.
- The CLI expects exactly 2 positional arguments: RELEASE_NAME and CHART_PATH, with -o flag for output directory.

## CI/CD Integration
- GitHub Actions run on Go 1.24+ with Helm v3.18.6 and Kustomize v5.6.0
- CI runs `CGO_ENABLED=0 go test ./...` after attempting to add helm stable repo
- Linting workflow uses golangci-lint v2.1.6 but has configuration compatibility issues

## Common Tasks
The following are outputs from frequently run commands. Reference them instead of viewing, searching, or running bash commands to save time.

### Repository Root
```
ls -la /home/runner/work/chartify/chartify/
chartify.go           # Main library code
chartify_test.go      # Primary tests  
go.mod go.sum         # Go modules
Makefile              # Build shortcuts
README.md             # Basic documentation
cmd/                  # CLI applications
  chartify/main.go    # Main CLI
  chartreposerver/main.go # Chart repo server
testdata/             # Test charts and manifests
  charts/log/         # Sample Helm chart
  kube_manifest_yml/  # Sample K8s manifests
  kustomize/          # Sample kustomize config
```

### Main Source Files
- `chartify.go` (680 lines): Core library with Chartify() function
- `kustomize.go` (142 lines): Kustomize integration 
- `patch.go` (208 lines): Strategic merge and JSON patch logic
- `replace.go` (259 lines): Text replacement functionality
- `requirements.go` (128 lines): Chart dependency management
- `runner.go` (122 lines): Command execution utilities