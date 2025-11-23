# Patches Support

Chartify now supports Kustomize-style patches through the `Patches` field in `ChartifyOpts` and the `--patch` CLI flag.

## Overview

The patches feature allows you to apply modifications to Kubernetes resources using either:
- **Strategic Merge Patches**: YAML that gets merged with existing resources
- **JSON Patches**: RFC 6902 JSON patch operations

Patches can be:
- **File-based**: Reference patch files using the `Path` field
- **Inline**: Include patch content directly using the `Patch` field
- **Targeted**: Specify which resources to patch using the `Target` field

## Examples

### Strategic Merge Patch (File-based)

```yaml
patches:
- path: "./my-patch.yaml"
```

Where `my-patch.yaml` contains:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: myapp
        image: myapp:v2.0.0
```

### Strategic Merge Patch (Inline)

```yaml
patches:
- patch: |-
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: myapp
    spec:
      replicas: 5
```

### JSON Patch (Inline with Target)

```yaml
patches:
- target:
    kind: Deployment
    name: myapp
  patch: |-
    - op: replace
      path: /spec/replicas
      value: 7
    - op: replace
      path: /spec/template/spec/containers/0/image
      value: myapp:v3.0.0
```

## CLI Usage

Use the `--patch` flag to specify patch files:

```bash
chartify myapp ./my-chart -o ./output --patch ./my-patch.yaml
```

Multiple patches can be specified:

```bash
chartify myapp ./my-chart -o ./output --patch ./patch1.yaml --patch ./patch2.yaml
```

## Auto-detection

Chartify automatically detects whether a patch is a Strategic Merge Patch or JSON Patch based on the content structure:

- **JSON Patches**: Must contain operations with `op` and `path` fields
- **Strategic Merge Patches**: Standard Kubernetes YAML resources

JSON patches require a target specification to identify which resources to patch.

## Backward Compatibility

The new patches feature is fully backward compatible with existing functionality:
- `JsonPatches` field continues to work as before
- `StrategicMergePatches` field continues to work as before
- `--strategic-merge-patch` CLI flag continues to work as before

The new `Patches` field provides a unified interface that can handle both types.