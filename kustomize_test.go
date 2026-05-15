package chartify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortOptionsValidate(t *testing.T) {
	t.Run("empty order is invalid", func(t *testing.T) {
		err := (&SortOptions{Order: ""}).validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "must not be empty")
	})

	t.Run("invalid order is rejected", func(t *testing.T) {
		err := (&SortOptions{Order: "unknown"}).validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "is not valid")
	})

	t.Run("fifo is valid", func(t *testing.T) {
		require.NoError(t, (&SortOptions{Order: "fifo"}).validate())
	})

	t.Run("legacy is valid", func(t *testing.T) {
		require.NoError(t, (&SortOptions{Order: "legacy"}).validate())
	})
}

func TestMarshalSortOptions(t *testing.T) {
	t.Run("nil returns empty bytes", func(t *testing.T) {
		got, err := marshalSortOptions(nil)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("fifo order", func(t *testing.T) {
		got, err := marshalSortOptions(&SortOptions{Order: "fifo"})
		require.NoError(t, err)
		result := string(got)
		require.Contains(t, result, "sortOptions:")
		require.Contains(t, result, "order: fifo")
	})

	t.Run("invalid order returns error", func(t *testing.T) {
		_, err := marshalSortOptions(&SortOptions{Order: "bogus"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "is not valid")
	})
}

func TestPatch_SortOptions(t *testing.T) {
	t.Run("Patch writes sortOptions to kustomization.yaml", func(t *testing.T) {
		tempDir := t.TempDir()

		templatesDir := filepath.Join(tempDir, "templates")
		require.NoError(t, os.MkdirAll(templatesDir, 0755))

		manifest := filepath.Join(templatesDir, "deployment.yaml")
		require.NoError(t, os.WriteFile(manifest, []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 1
`), 0644))

		patchContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 3
`
		patchFile := filepath.Join(tempDir, "patch.yaml")
		require.NoError(t, os.WriteFile(patchFile, []byte(patchContent), 0644))

		r := New(HelmBin(helm))

		var capturedKustomization string
		origWriteFile := r.WriteFile
		r.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(filename, "kustomization.yaml") {
				capturedKustomization = string(data)
			}
			return origWriteFile(filename, data, perm)
		}

		patchOpts := &PatchOpts{
			StrategicMergePatches: []string{patchFile},
			SortOptions:           &SortOptions{Order: "fifo"},
		}
		err := r.Patch(tempDir, []string{manifest}, patchOpts)
		require.NoError(t, err)
		require.Contains(t, capturedKustomization, "sortOptions:")
		require.Contains(t, capturedKustomization, "order: fifo")
	})

	t.Run("Patch without SortOptions omits sortOptions from kustomization.yaml", func(t *testing.T) {
		tempDir := t.TempDir()

		templatesDir := filepath.Join(tempDir, "templates")
		require.NoError(t, os.MkdirAll(templatesDir, 0755))

		manifest := filepath.Join(templatesDir, "deployment.yaml")
		require.NoError(t, os.WriteFile(manifest, []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 1
`), 0644))

		patchContent := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 3
`
		patchFile := filepath.Join(tempDir, "patch.yaml")
		require.NoError(t, os.WriteFile(patchFile, []byte(patchContent), 0644))

		r := New(HelmBin(helm))

		var capturedKustomization string
		origWriteFile := r.WriteFile
		r.WriteFile = func(filename string, data []byte, perm os.FileMode) error {
			if strings.HasSuffix(filename, "kustomization.yaml") {
				capturedKustomization = string(data)
			}
			return origWriteFile(filename, data, perm)
		}

		patchOpts := &PatchOpts{
			StrategicMergePatches: []string{patchFile},
		}
		err := r.Patch(tempDir, []string{manifest}, patchOpts)
		require.NoError(t, err)
		require.NotContains(t, capturedKustomization, "sortOptions:")
	})
}
