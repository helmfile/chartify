# chartify

`chartify` converts anything to a Helm chart and modifies a chart in-place so that you don't need to fork an upstream helm chart only for a few custom requirements.

`chartify` is a Go library that is primarily used by [helmfile](https://github.com/helmfile/helmfile) to let it convert Kuberntes resource YAMLs or [kustomize](https://github.com/kubernetes-sigs/kustomize) into a [helm](https://github.com/helm/helm) chart, and apply various modifications to the resulting chart.

`chartify` is intended to be run immediately before running `helm upgrade --install`. For example, instead of forking a helm chart, you should be able to prepend a `chartify` step into your deployment job in your CD pipeline. `chartify` isn't intended to create a fork of a chart. The output of `chartify` is a helm chart that is pre-rendered with all the helm values provided to `chartify`.

## CLI

Beyond it's usage with helmfile, it also provides a basic CLI application that can be run independently.

The simplest usage of the command is:

```
$ chartify $RELEASE $CHART -o $OUTPUT_DIR
```

See `chartify -h` or `go run ./cmd/chartify -h` for more information.
