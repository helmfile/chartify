package chartify

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshalSortOptions(t *testing.T) {
	t.Run("nil returns no sortOptions content", func(t *testing.T) {
		got, err := marshalSortOptions(nil)
		require.NoError(t, err)
		result := string(got)
		require.Contains(t, result, "sortOptions: null\n")
	})

	t.Run("fifo order", func(t *testing.T) {
		got, err := marshalSortOptions(&SortOptions{Order: "fifo"})
		require.NoError(t, err)
		result := string(got)
		require.Contains(t, result, "sortOptions:")
		require.Contains(t, result, "order: fifo")
	})

	t.Run("legacy order", func(t *testing.T) {
		got, err := marshalSortOptions(&SortOptions{Order: "legacy"})
		require.NoError(t, err)
		result := string(got)
		require.Contains(t, result, "sortOptions:")
		require.Contains(t, result, "order: legacy")
	})
}

func TestKustomizationYamlWithSortOptions(t *testing.T) {
	t.Run("Patch generates sortOptions in kustomization.yaml", func(t *testing.T) {
		opts := &SortOptions{Order: "fifo"}
		sortOptsBytes, err := marshalSortOptions(opts)
		require.NoError(t, err)

		kustomizationYamlContent := `kind: ""
apiversion: ""
resources:
- templates/deployment.yaml
patches:
- path: strategicmergepatches/patch.0.yaml
`
		kustomizationYamlContent += string(sortOptsBytes)

		require.True(t, strings.Contains(kustomizationYamlContent, "sortOptions:"))
		require.True(t, strings.Contains(kustomizationYamlContent, "order: fifo"))
	})

	t.Run("Patch without SortOptions has no sortOptions in kustomization.yaml", func(t *testing.T) {
		kustomizationYamlContent := `kind: ""
apiversion: ""
resources:
- templates/deployment.yaml
patches:
- path: strategicmergepatches/patch.0.yaml
`

		require.False(t, strings.Contains(kustomizationYamlContent, "sortOptions:"))
	})

	t.Run("KustomizeBuild merges SortOptions from KustomizeBuildOpts", func(t *testing.T) {
		u := &KustomizeBuildOpts{
			SortOptions: &SortOptions{Order: "fifo"},
		}
		kustomizeOpts := KustomizeOpts{}
		if u.SortOptions != nil {
			kustomizeOpts.SortOptions = u.SortOptions
		}
		require.NotNil(t, kustomizeOpts.SortOptions)
		require.Equal(t, "fifo", kustomizeOpts.SortOptions.Order)
	})

	t.Run("KustomizeBuild without SortOptions leaves kustomizeOpts.SortOptions nil", func(t *testing.T) {
		u := &KustomizeBuildOpts{}
		kustomizeOpts := KustomizeOpts{}
		if u.SortOptions != nil {
			kustomizeOpts.SortOptions = u.SortOptions
		}
		require.Nil(t, kustomizeOpts.SortOptions)
	})
}
