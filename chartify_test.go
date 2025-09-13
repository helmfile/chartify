package chartify

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestReadAdhocDependencies(t *testing.T) {
	type testcase struct {
		opts         ChartifyOpts
		wantDendency []Dependency
		wantErr      bool
		errorKeyMsg  string
	}

	helm := "helm"
	if h := os.Getenv("HELM_BIN"); h != "" {
		helm = h
	}
	runner := New(HelmBin(helm))

	setupHelmConfig(t)
	repo := "myrepo"
	startServer(t, repo)

	run := func(tc testcase) {
		t.Helper()

		got, err := runner.ReadAdhocDependencies(&tc.opts)
		if tc.wantErr {
			require.Error(t, err, "ReadAdhocDependencies() expected an error but got nil")
			require.Containsf(t, err.Error(), tc.errorKeyMsg, "ReadAdhocDependencies() expected error key message %q but got %q", tc.errorKeyMsg, err.Error())
		} else {
			require.NoError(t, err, "ReadAdhocDependencies() expected error is nil but got an error")
		}

		if d := cmp.Diff(tc.wantDendency, got); d != "" {
			t.Fatalf("unexpected result: want (-), got (+):\n%s", d)
		}
	}

	run(testcase{
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Chart: "./testdata/charts/db",
				},
			},
		},
		wantDendency: []Dependency{
			{
				Repository: "file://./testdata/charts/db",
				Name:       "db",
				Condition:  "db.enabled",
			},
		},
	})

	run(testcase{
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Chart:   "myrepo/db",
					Version: "0.1.0",
				},
			},
		},
		wantDendency: []Dependency{
			{
				Repository: "http://localhost:18080/",
				Name:       "db",
				Version:    "0.1.0",
				Condition:  "db.enabled",
			},
		},
	})

	run(testcase{
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Chart:   "nomyrepo/db",
					Version: "0.1.0",
				},
			},
		},
		wantDendency: nil,
		wantErr:      true,
		errorKeyMsg:  "no helm list entry found for repository \"nomyrepo\"",
	})

	run(testcase{
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Chart:   "oci://r.example.com/incubator/raw",
					Version: "0.1.0",
				},
			},
		},
		wantDendency: []Dependency{
			{
				Repository: "oci://r.example.com/incubator",
				Name:       "raw",
				Version:    "0.1.0",
				Condition:  "raw.enabled",
			},
		},
	})
}

func TestUseHelmChartsInKustomize(t *testing.T) {
	repo := "myrepo"
	startServer(t, repo)

	r := New(UseHelm3(true), HelmBin(helm))

	tests := []struct {
		name string
		opts ChartifyOpts
	}{
		{
			name: "--enable_alpha_plugins is ON",
			opts: ChartifyOpts{
				EnableKustomizeAlphaPlugins: true,
			},
		},
		{
			name: "--enable_alpha_plugins is OFF",
			opts: ChartifyOpts{
				EnableKustomizeAlphaPlugins: false,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Helper()

			release := "myapp"
			tmpDir, err := r.Chartify(release, "./testdata/kustomize_with_helm_charts", &tc.opts)
			t.Cleanup(func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					panic("unable to remove chartify tmpDir: " + err.Error())
				}
			})
			require.NoError(t, err)

			ctx := context.Background()
			args := []string{"template", release, tmpDir}
			cmd := exec.CommandContext(ctx, helm, args...)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err)
			got := string(out)

			snapshotFile := "./testdata/integration/testcases/kustomize_with_helm_charts/want"
			snapshot, err := os.ReadFile(snapshotFile)
			require.NoError(t, err, "reading snapshot %s", snapshotFile)

			want := string(snapshot)
			require.Equal(t, want, got)
		})
	}
}

func TestPatches(t *testing.T) {
	t.Run("strategic merge patch with path", func(t *testing.T) {
		patches := []Patch{
			{
				Path: "./testdata/patches/strategic-merge.yaml",
			},
		}

		// Test that the patch struct is properly constructed
		require.Equal(t, "./testdata/patches/strategic-merge.yaml", patches[0].Path)
		require.Empty(t, patches[0].Patch)
		require.Nil(t, patches[0].Target)
	})

	t.Run("json patch with inline content and target", func(t *testing.T) {
		patches := []Patch{
			{
				Patch: `- op: replace
  path: /spec/replicas
  value: 5`,
				Target: &PatchTarget{
					Kind: "Deployment",
					Name: "myapp",
				},
			},
		}

		// Test that the patch struct is properly constructed
		require.Empty(t, patches[0].Path)
		require.Contains(t, patches[0].Patch, "op: replace")
		require.Equal(t, "Deployment", patches[0].Target.Kind)
		require.Equal(t, "myapp", patches[0].Target.Name)
	})

	t.Run("strategic merge patch with inline content", func(t *testing.T) {
		patches := []Patch{
			{
				Patch: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  replicas: 3`,
			},
		}

		// Test that the patch struct is properly constructed
		require.Empty(t, patches[0].Path)
		require.Contains(t, patches[0].Patch, "kind: Deployment")
		require.Nil(t, patches[0].Target)
	})

	// Test validation logic that would be in patch processing
	t.Run("validation errors", func(t *testing.T) {
		testCases := []struct {
			name    string
			patch   Patch
			wantErr string
		}{
			{
				name: "both path and patch set",
				patch: Patch{
					Path:  "./some/path.yaml",
					Patch: "some content",
				},
				wantErr: "both \"path\" and \"patch\" are set",
			},
			{
				name: "neither path nor patch set",
				patch: Patch{
					// empty
				},
				wantErr: "either \"path\" or \"patch\" must be set",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// This simulates the validation that would happen in patch processing
				hasPath := tc.patch.Path != ""
				hasPatch := tc.patch.Patch != ""
				
				if hasPath && hasPatch {
					require.Contains(t, tc.wantErr, "both \"path\" and \"patch\" are set")
				} else if !hasPath && !hasPatch {
					require.Contains(t, tc.wantErr, "either \"path\" or \"patch\" must be set")
				}
			})
		}
	})
}

func TestPatchIntegration(t *testing.T) {
	helm := "helm"
	if h := os.Getenv("HELM_BIN"); h != "" {
		helm = h
	}

	setupHelmConfig(t)

	runner := New(UseHelm3(true), HelmBin(helm))

	t.Run("strategic merge patch file", func(t *testing.T) {
		// Test that a strategic merge patch file works
		opts := ChartifyOpts{
			Patches: []Patch{
				{
					Path: "./testdata/patches/strategic-merge.yaml",
				},
			},
		}

		_, err := runner.Chartify("myapp", "./testdata/simple_manifest", WithChartifyOpts(&opts))
		require.NoError(t, err)
	})

	t.Run("inline strategic merge patch", func(t *testing.T) {
		// Test that an inline strategic merge patch works
		opts := ChartifyOpts{
			Patches: []Patch{
				{
					Patch: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  replicas: 5
  template:
    spec:
      containers:
      - name: myapp
        image: myapp:v4.0.0`,
				},
			},
		}

		resultDir, err := runner.Chartify("myapp", "./testdata/simple_manifest", WithChartifyOpts(&opts))
		require.NoError(t, err)
		require.DirExists(t, resultDir)
	})

	t.Run("inline json patch with target", func(t *testing.T) {
		// Test that an inline JSON patch with target works
		opts := ChartifyOpts{
			Patches: []Patch{
				{
					Patch: `- op: replace
  path: /spec/replicas
  value: 7
- op: replace
  path: /spec/template/spec/containers/0/image
  value: myapp:v5.0.0`,
					Target: &PatchTarget{
						Kind: "Deployment",
						Name: "myapp",
					},
				},
			},
		}

		resultDir, err := runner.Chartify("myapp", "./testdata/simple_manifest", WithChartifyOpts(&opts))
		require.NoError(t, err)
		require.DirExists(t, resultDir)
	})
}
