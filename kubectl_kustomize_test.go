package chartify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestKubectlKustomize tests behavior when kubectl kustomize is explicitly configured
// via KustomizeBin("kubectl kustomize"). The automatic fallback selection is tested
// in TestKustomizeBin.
func TestKubectlKustomize(t *testing.T) {
	t.Run("KustomizeBuild succeeds with kubectl kustomize option", func(t *testing.T) {
		// Create a stub kubectl that handles the kustomize subcommand,
		// so this test is self-contained and not skipped when kubectl is absent.
		stubDir := t.TempDir()
		stubKubectl := filepath.Join(stubDir, "kubectl")
		// The stub writes minimal valid YAML to the -o output file.
		stubScript := []byte(`#!/bin/sh
# Stub kubectl for testing kubectl kustomize
if [ "$1" = "kustomize" ]; then
  shift
  OUTPUT=""
  while [ $# -gt 0 ]; do
    case "$1" in
      -o) OUTPUT="$2"; shift 2;;
      *) shift;;
    esac
  done
  if [ -n "$OUTPUT" ]; then
    printf 'apiVersion: v1\nkind: List\nitems: []\n' > "$OUTPUT"
  fi
fi
`)
		require.NoError(t, os.WriteFile(stubKubectl, stubScript, 0755))

		origPath := os.Getenv("PATH")
		defer os.Setenv("PATH", origPath)
		os.Setenv("PATH", stubDir+":"+origPath)

		tmpDir := t.TempDir()
		srcDir := t.TempDir()

		kustomizationContent := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- deployment.yaml
`
		deploymentContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: test
        image: test:latest
`

		templatesDir := filepath.Join(tmpDir, "templates")
		require.NoError(t, os.MkdirAll(templatesDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "kustomization.yaml"), []byte(kustomizationContent), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "deployment.yaml"), []byte(deploymentContent), 0644))

		r := New(KustomizeBin("kubectl kustomize"))

		outputFile, err := r.KustomizeBuild(srcDir, tmpDir)
		require.NoError(t, err)
		require.FileExists(t, outputFile)
	})

	t.Run("edit commands not supported with kubectl kustomize", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := t.TempDir()

		kustomizationContent := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- deployment.yaml
`
		deploymentContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: test
        image: test:latest
`

		templatesDir := filepath.Join(tmpDir, "templates")
		valuesDir := t.TempDir()
		valuesFile := filepath.Join(valuesDir, "values.yaml")
		valuesContent := `images:
- name: test
  newName: newtest
  newTag: v2
`

		require.NoError(t, os.MkdirAll(templatesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "kustomization.yaml"), []byte(kustomizationContent), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "deployment.yaml"), []byte(deploymentContent), 0644))
		require.NoError(t, os.WriteFile(valuesFile, []byte(valuesContent), 0644))

		r := New(KustomizeBin("kubectl kustomize"))

		_, err := r.KustomizeBuild(srcDir, tmpDir, &KustomizeBuildOpts{ValuesFiles: []string{valuesFile}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "setting images via kustomizeOpts.Images is not supported when using 'kubectl kustomize'")
	})
}
