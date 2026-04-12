package chartify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubKubectlScript is a minimal sh script that acts as a kubectl stub for tests.
// It handles `kubectl kustomize <dir> [-o|-o FILE|--output FILE]` by writing
// a minimal valid Kubernetes Deployment YAML to the specified output file.
const stubKubectlScript = `#!/bin/sh
if [ "$1" = "kustomize" ]; then
  shift
  OUTPUT=""
  while [ $# -gt 0 ]; do
    case "$1" in
      -o) OUTPUT="$2"; shift 2;;
      --output) OUTPUT="$2"; shift 2;;
      *) shift;;
    esac
  done
  if [ -n "$OUTPUT" ]; then
    printf 'apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: stub\n' > "$OUTPUT"
  fi
fi
`

// writeStubKubectl creates a stub kubectl script in dir.
func writeStubKubectl(t *testing.T, dir string) {
	t.Helper()
	p := filepath.Join(dir, "kubectl")
	require.NoError(t, os.WriteFile(p, []byte(stubKubectlScript), 0755))
}

// TestKubectlKustomize tests behavior when kubectl kustomize is explicitly configured
// via KustomizeBin("kubectl kustomize"). The automatic fallback selection is tested
// in TestKustomizeBin.
func TestKubectlKustomize(t *testing.T) {
	t.Run("KustomizeBuild succeeds with kubectl kustomize option", func(t *testing.T) {
		// Create a stub kubectl so the test is self-contained and always runs in CI.
		stubDir := t.TempDir()
		writeStubKubectl(t, stubDir)

		origPath := os.Getenv("PATH")
		defer os.Setenv("PATH", origPath)
		os.Setenv("PATH", stubDir+string(os.PathListSeparator)+origPath)

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

	t.Run("Patch succeeds with kubectl kustomize option", func(t *testing.T) {
		// Create a stub kubectl so the test is self-contained and always runs in CI.
		stubDir := t.TempDir()
		writeStubKubectl(t, stubDir)

		origPath := os.Getenv("PATH")
		defer os.Setenv("PATH", origPath)
		os.Setenv("PATH", stubDir+string(os.PathListSeparator)+origPath)

		tempDir := t.TempDir()

		// Write a minimal manifest file that Patch() will reference.
		manifestPath := filepath.Join(tempDir, "templates", "deploy.yaml")
		require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0755))
		require.NoError(t, os.WriteFile(manifestPath, []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
`), 0644))

		r := New(KustomizeBin("kubectl kustomize"))
		err := r.Patch(tempDir, []string{manifestPath}, &PatchOpts{})
		require.NoError(t, err)
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
		require.Contains(t, err.Error(), "setting images via Chartify values files or kustomize build options is not supported when using 'kubectl kustomize'")
	})
}
