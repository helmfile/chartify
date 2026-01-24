package chartify

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKubectlKustomizeFallback(t *testing.T) {
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skip("kubectl binary not found in PATH")
	}

	t.Run("KustomizeBuild with kubectl kustomize", func(t *testing.T) {
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
}
