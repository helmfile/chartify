# chartify

`chartify` converts anything to a Helm chart and modifies a chart in-place so that you don't need to fork an upstream helm chart only for a few custom requirements.

`chartify` is a Go library that is primarily used by [helmfile](https://github.com/helmfile/helmfile) to let it convert Kuberntes resource YAMLs or [kustomize](https://github.com/kubernetes-sigs/kustomize) into a [helm](https://github.com/helm/helm) chart, and apply various modifications to the resulting chart.

`chartify` is intended to be run immediately before running `helm upgrade --install`. For example, instead of forking a helm chart, you should be able to prepend a `chartify` step into your deployment job in your CD pipeline. `chartify` isn't intended to create a fork of a chart. The output of `chartify` is a helm chart that is pre-rendered with all the helm values provided to `chartify`.

## Prerequisites

- Go 1.24.0 or later
- Helm 3.x (required for chart operations)

## Building and Installation

### Building from Source

To build the binaries locally:

```bash
# Build both chartify and chartreposerver binaries
make build

# Or build them individually
make build-chartify
make build-chartreposerver

# Clean up built binaries
make clean
```

### Installing

To install the binaries to your `$GOPATH/bin`:

```bash
make install
```

Or install directly with Go:

```bash
go install github.com/helmfile/chartify/cmd/chartify@latest
go install github.com/helmfile/chartify/cmd/chartreposerver@latest
```

## CLI

Beyond it's usage with helmfile, it also provides a basic CLI application that can be run independently.

### chartify

The simplest usage of the command is:

```bash
$ chartify $RELEASE $CHART -o $OUTPUT_DIR
```

#### Examples

Convert a local chart:
```bash
$ ./chartify myrelease ./my-chart -o ./output
```

Convert with additional dependencies:
```bash
$ ./chartify myrelease ./my-chart -o ./output -d "redis=redis:6.0.0"
```

Include CRDs in the output:
```bash
$ ./chartify myrelease ./my-chart -o ./output --include-crds
```

Apply a strategic merge patch:
```bash
$ ./chartify myrelease ./my-chart -o ./output --strategic-merge-patch ./patch.yaml
```

#### Full Usage

See `chartify -h` or `go run ./cmd/chartify -h` for more information:

```
Usage of chartify:
  -d value
        one or more "alias=chart:version" to add adhoc chart dependencies
  -f string
        The path to the input file or stdout(-) (default "-")
  -include-crds
        Whether to render CRDs contained in the chart and include the results into the output
  -o string
        The path to the output directory (required)
  -strategic-merge-patch string
        Path to a kustomize strategic merge patch file
```

### chartreposerver

A simple chart repository server for development and testing:

```bash
$ ./chartreposerver /path/to/charts/directory
```

The server will start on port 18080 and serve charts from the specified directory.

## Development

### Running Tests

```bash
# Run all tests
make test

# Run tests with verbose output and retain temp directories
make test/verbose
```

### Linting

This project uses golangci-lint. The configuration is in `.golangci.yaml`.
