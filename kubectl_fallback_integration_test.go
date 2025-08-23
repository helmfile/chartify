package chartify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKubectlKustomizeFallbackIntegration(t *testing.T) {
	// Create a temporary directory for our test
	tempDir, err := os.MkdirTemp("", "kubectl-fallback-integration")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a simple kubernetes manifest
	manifestContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: app
        image: nginx:1.20
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: test-app-service
  namespace: default
spec:
  selector:
    app: test-app
  ports:
  - protocol: TCP
    port: 80
    targetPort: 80
  type: ClusterIP
`
	
	manifestPath := filepath.Join(tempDir, "manifest.yaml")
	err = os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	require.NoError(t, err)

	// Test with non-existent kustomize binary (should use kubectl fallback)
	t.Run("kubectl fallback scenario", func(t *testing.T) {
		runner := New(KustomizeBin("non-existent-kustomize"))
		
		// Test kustomize build with image replacement
		kustomizeOpts := KustomizeOpts{
			Images: []KustomizeImage{
				{Name: "nginx", NewTag: "1.21"},
			},
			NamePrefix: "prefix-",
			Namespace:  "test-namespace",
		}
		
		// Generate kustomization content
		relPath := "."
		kustomizationContent, err := runner.generateKustomizationFile(relPath, kustomizeOpts)
		require.NoError(t, err)
		
		// Verify the generated kustomization content
		kustomizationStr := string(kustomizationContent)
		require.Contains(t, kustomizationStr, "resources:")
		require.NotContains(t, kustomizationStr, "bases:") // Should not use deprecated bases
		require.Contains(t, kustomizationStr, "images:")
		require.Contains(t, kustomizationStr, "namePrefix: prefix-")
		require.Contains(t, kustomizationStr, "namespace: test-namespace")
		require.Contains(t, kustomizationStr, "nginx")
		require.Contains(t, kustomizationStr, "1.21")
		
		// Write kustomization.yaml
		kustomizationPath := filepath.Join(tempDir, "kustomization.yaml")
		err = os.WriteFile(kustomizationPath, kustomizationContent, 0644)
		require.NoError(t, err)
		
		// Test the build command generation for kubectl fallback
		buildArgs := []string{"-o", "/tmp/output.yaml", "build", "--enable-helm"}
		cmd, args, err := runner.kustomizeBuildCommand(buildArgs, tempDir)
		require.NoError(t, err)
		require.Equal(t, "kubectl", cmd)
		require.Contains(t, args, "kustomize")
		require.Contains(t, args, tempDir)
		require.Contains(t, args, "--enable-helm")
		
		t.Logf("kubectl fallback command: %s %s", cmd, strings.Join(args, " "))
	})
	
	// Test with real kustomize binary (should use kustomize directly)
	t.Run("normal kustomize scenario", func(t *testing.T) {
		runner := New(KustomizeBin("kustomize"))
		
		buildArgs := []string{"-o", "/tmp/output.yaml", "build", "--enable-helm"}
		cmd, args, err := runner.kustomizeBuildCommand(buildArgs, tempDir)
		require.NoError(t, err)
		require.Equal(t, "kustomize", cmd)
		require.Equal(t, append(buildArgs, tempDir), args)
		
		t.Logf("normal kustomize command: %s %s", cmd, strings.Join(args, " "))
	})
}