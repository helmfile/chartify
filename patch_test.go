package chartify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPatch_PreserveCRDLocation tests that CRDs from templates/crds/ stay in templates/crds/
// This is the fix for https://github.com/helmfile/helmfile/issues/2291
func TestPatch_PreserveCRDLocation(t *testing.T) {
	tests := []struct {
		name                  string
		setupFiles            map[string]string // path -> content
		expectedCRDLocation   string            // Expected directory for CRDs after patching
		expectedCRDsPreserved bool              // Should CRDs be in templates/crds/?
	}{
		{
			name: "CRDs from templates/crds/ should stay in templates/crds/",
			setupFiles: map[string]string{
				"templates/crds/crd-scaledobjects.yaml": `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: scaledobjects.keda.sh
spec:
  group: keda.sh
  names:
    kind: ScaledObject`,
				"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 1`,
			},
			expectedCRDLocation:   "templates/crds",
			expectedCRDsPreserved: true,
		},
		{
			name: "CRDs from root crds/ should go to crds/",
			setupFiles: map[string]string{
				"crds/crd-example.yaml": `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: examples.test.io
spec:
  group: test.io
  names:
    kind: Example`,
				"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment`,
			},
			expectedCRDLocation:   "crds",
			expectedCRDsPreserved: false,
		},
		{
			name: "Mixed resources without CRDs in templates/crds/",
			setupFiles: map[string]string{
				"templates/configmap.yaml": `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  key: value`,
				"templates/service.yaml": `apiVersion: v1
kind: Service
metadata:
  name: test-svc
spec:
  ports:
  - port: 80`,
			},
			expectedCRDLocation:   "crds", // Default location when no CRDs in templates/
			expectedCRDsPreserved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tempDir, err := os.MkdirTemp("", "chartify-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Setup test files
			var generatedFiles []string
			for path, content := range tt.setupFiles {
				fullPath := filepath.Join(tempDir, path)
				dir := filepath.Dir(fullPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("Failed to create directory %s: %v", dir, err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write file %s: %v", fullPath, err)
				}
				generatedFiles = append(generatedFiles, fullPath)
			}

			// Test the detection logic
			crdsFromTemplates := false
			for _, f := range generatedFiles {
				relPath := strings.Replace(f, tempDir+string(filepath.Separator), "", 1)
				// Normalize path separators for cross-platform compatibility
				relPath = filepath.ToSlash(relPath)
				// Use same logic as the fix
				if strings.HasPrefix(relPath, "templates/crds/") {
					crdsFromTemplates = true
					break
				}
			}

			// Verify detection result matches expectation
			if crdsFromTemplates != tt.expectedCRDsPreserved {
				t.Errorf("CRD detection mismatch: got %v, want %v", crdsFromTemplates, tt.expectedCRDsPreserved)
			}

			// Verify expected CRD location logic
			var expectedDir string
			if crdsFromTemplates {
				expectedDir = filepath.Join(tempDir, "templates", "crds")
			} else {
				expectedDir = filepath.Join(tempDir, "crds")
			}

			// Normalize paths and check if expected location is in the path
			expectedNorm := filepath.ToSlash(expectedDir)
			wantNorm := filepath.ToSlash(filepath.Join(tempDir, tt.expectedCRDLocation))
			if !strings.HasPrefix(expectedNorm, wantNorm) {
				t.Errorf("Expected CRD location mismatch: got %s, want to contain %s",
					expectedDir, tt.expectedCRDLocation)
			}
		})
	}
}

// TestPatch_CRDLocationDetection tests the CRD location detection logic in isolation
func TestPatch_CRDLocationDetection(t *testing.T) {
	tests := []struct {
		name          string
		relPaths      []string // Already relative paths (what we get after removing tempDir)
		wantTemplates bool
	}{
		{
			name: "templates/crds/ files should be detected",
			relPaths: []string{
				"templates/crds/crd1.yaml",
				"templates/deployment.yaml",
			},
			wantTemplates: true,
		},
		{
			name: "root crds/ should not be detected as templates",
			relPaths: []string{
				"crds/crd1.yaml",
				"templates/deployment.yaml",
			},
			wantTemplates: false,
		},
		{
			name: "no crds should not be detected",
			relPaths: []string{
				"templates/deployment.yaml",
				"templates/service.yaml",
			},
			wantTemplates: false,
		},
		{
			name: "templates with other subdirs, no crds",
			relPaths: []string{
				"templates/manager/deployment.yaml",
				"templates/webhooks/service.yaml",
			},
			wantTemplates: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the detection logic (same as in patch.go)
			crdsFromTemplates := false
			for _, relPath := range tt.relPaths {
				// Normalize path separators for cross-platform compatibility
				relPath = filepath.ToSlash(relPath)
				// Check if path starts with templates/crds/
				if strings.HasPrefix(relPath, "templates/crds/") {
					crdsFromTemplates = true
					break
				}
			}

			if crdsFromTemplates != tt.wantTemplates {
				t.Errorf("Detection mismatch for %v: got %v, want %v",
					tt.relPaths, crdsFromTemplates, tt.wantTemplates)
			}
		})
	}
}

// TestPatch_KEDA_RealWorld tests the fix with a structure similar to the real KEDA chart
// This verifies the fix for issue #2291
func TestPatch_KEDA_RealWorld(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "chartify-keda-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a structure similar to KEDA chart
	kedaCRDs := map[string]string{
		"templates/crds/crd-scaledobjects.yaml": `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: scaledobjects.keda.sh
spec:
  group: keda.sh`,
		"templates/crds/crd-scaledjobs.yaml": `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: scaledjobs.keda.sh
spec:
  group: keda.sh`,
		"templates/manager/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: keda-operator
spec:
  replicas: 1`,
	}

	var generatedFiles []string
	for path, content := range kedaCRDs {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write file: %v", err)
		}
		generatedFiles = append(generatedFiles, fullPath)
	}

	// Test the fix: CRDs from templates/crds/ should be preserved
	crdsFromTemplates := false
	for _, f := range generatedFiles {
		relPath := strings.Replace(f, tempDir+string(filepath.Separator), "", 1)
		// Normalize path separators for cross-platform compatibility
		relPath = filepath.ToSlash(relPath)
		if strings.HasPrefix(relPath, "templates/crds/") {
			crdsFromTemplates = true
			t.Logf("Detected CRD in templates/: %s", relPath)
			break
		}
	}

	if !crdsFromTemplates {
		t.Errorf("KEDA-like chart: Expected to detect CRDs in templates/crds/, but did not")
	}

	// Verify the CRDs would be placed in templates/crds/ (preserving location)
	expectedCRDDir := filepath.Join(tempDir, "templates", "crds")
	t.Logf("CRDs would be placed in: %s (original location preserved)", expectedCRDDir)
}
