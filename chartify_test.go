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

	r := New(HelmBin(helm))

	// Skip this test for Helm 4 as Kustomize 5.8.0 doesn't support Helm 4 yet
	// Kustomize tries to run 'helm version -c --short' which is not supported in Helm 4
	if r.IsHelm4() {
		t.Skip("Skipping test: Kustomize 5.8.0 does not support Helm 4 (uses unsupported 'helm version -c' flag)")
	}

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
