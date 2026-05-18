package chartify

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

func TestDepCommandSelection(t *testing.T) {
	newTestRunner := func(failBuild bool, failMsg string) (*Runner, *[]helmCall) {
		var calls []helmCall
		r := &Runner{
			HelmBinary: "helm",
			isHelm3:    true,
			RunCommand: func(name string, args []string, dir string, stdout, stderr io.Writer, env map[string]string) error {
				calls = append(calls, helmCall{name: name, args: append([]string{}, args...)})
				if failBuild && len(args) >= 2 && args[0] == "dependency" && args[1] == "build" {
					if _, err := stderr.Write([]byte(failMsg)); err != nil {
						return err
					}
					return fmt.Errorf("%s", failMsg)
				}
				return nil
			},
			CopyFile:  CopyFile,
			WriteFile: os.WriteFile,
			ReadFile:  os.ReadFile,
			ReadDir:   os.ReadDir,
			Walk:      filepath.Walk,
			Exists:    exists,
			Logf:      func(string, ...interface{}) {},
			MakeTempDir: func(release, chart string, opts *ChartifyOpts) string {
				return ""
			},
		}
		return r, &calls
	}

	setupChart := func(t *testing.T, withLock bool, lockName string) string {
		t.Helper()
		dir := t.TempDir()
		chartYaml := filepath.Join(dir, "Chart.yaml")
		if err := os.WriteFile(chartYaml, []byte("apiVersion: v2\nname: test\nversion: 0.1.0\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "templates"), 0755); err != nil {
			t.Fatal(err)
		}
		if withLock {
			if err := os.WriteFile(filepath.Join(dir, lockName), []byte("dependencies: []\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}

	t.Run("uses dependency build when Chart.lock exists and no adhoc deps", func(t *testing.T) {
		chartDir := setupChart(t, true, "Chart.lock")
		r, calls := newTestRunner(false, "")
		r.MakeTempDir = func(_, _ string, _ *ChartifyOpts) string { return chartDir }

		_, err := r.Chartify("rel", chartDir, WithChartifyOpts(&ChartifyOpts{}))
		require.NoError(t, err)

		depCalls := filterDepCalls(*calls)
		require.Len(t, depCalls, 1)
		require.Equal(t, "build", depCalls[0].args[1])
	})

	t.Run("uses dependency up when no lock file exists", func(t *testing.T) {
		chartDir := setupChart(t, false, "")
		r, calls := newTestRunner(false, "")
		r.MakeTempDir = func(_, _ string, _ *ChartifyOpts) string { return chartDir }

		_, err := r.Chartify("rel", chartDir, WithChartifyOpts(&ChartifyOpts{}))
		require.NoError(t, err)

		depCalls := filterDepCalls(*calls)
		require.Len(t, depCalls, 1)
		require.Equal(t, "up", depCalls[0].args[1])
	})

	t.Run("uses dependency up when AdhocChartDependencies present despite lock", func(t *testing.T) {
		chartDir := setupChart(t, true, "Chart.lock")
		r, calls := newTestRunner(false, "")
		r.MakeTempDir = func(_, _ string, _ *ChartifyOpts) string { return chartDir }
		r.Exists = func(path string) (bool, error) {
			if _, err := os.Stat(path); err == nil {
				return true, nil
			}
			return false, nil
		}

		_, err := r.Chartify("rel", chartDir, WithChartifyOpts(&ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{{Chart: chartDir, Version: "0.1.0"}},
		}))
		require.NoError(t, err)

		depCalls := filterDepCalls(*calls)
		require.Len(t, depCalls, 1)
		require.Equal(t, "up", depCalls[0].args[1])
	})

	t.Run("uses dependency up when DeprecatedAdhocChartDependencies present despite lock", func(t *testing.T) {
		chartDir := setupChart(t, true, "Chart.lock")
		r, calls := newTestRunner(false, "")
		r.MakeTempDir = func(_, _ string, _ *ChartifyOpts) string { return chartDir }
		r.Exists = func(path string) (bool, error) {
			if _, err := os.Stat(path); err == nil {
				return true, nil
			}
			return false, nil
		}

		_, err := r.Chartify("rel", chartDir, WithChartifyOpts(&ChartifyOpts{
			DeprecatedAdhocChartDependencies: []string{chartDir + ":0.1.0"},
		}))
		require.NoError(t, err)

		depCalls := filterDepCalls(*calls)
		require.Len(t, depCalls, 1)
		require.Equal(t, "up", depCalls[0].args[1])
	})

	t.Run("falls back to up when build fails with lock out of sync", func(t *testing.T) {
		chartDir := setupChart(t, true, "Chart.lock")
		r, calls := newTestRunner(true, "the lock file (Chart.lock) is out of sync with the dependencies listed in Chart.yaml")
		r.MakeTempDir = func(_, _ string, _ *ChartifyOpts) string { return chartDir }

		_, err := r.Chartify("rel", chartDir, WithChartifyOpts(&ChartifyOpts{}))
		require.NoError(t, err)

		depCalls := filterDepCalls(*calls)
		require.Len(t, depCalls, 2)
		require.Equal(t, "build", depCalls[0].args[1])
		require.Equal(t, "up", depCalls[1].args[1])
	})

	t.Run("does not fall back to up when build fails with non-sync error", func(t *testing.T) {
		chartDir := setupChart(t, true, "Chart.lock")
		r, calls := newTestRunner(true, "network timeout fetching dependency")
		r.MakeTempDir = func(_, _ string, _ *ChartifyOpts) string { return chartDir }

		_, err := r.Chartify("rel", chartDir, WithChartifyOpts(&ChartifyOpts{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "network timeout")

		depCalls := filterDepCalls(*calls)
		require.Len(t, depCalls, 1)
		require.Equal(t, "build", depCalls[0].args[1])
	})

	t.Run("uses requirements.lock for legacy charts", func(t *testing.T) {
		chartDir := setupChart(t, true, "requirements.lock")
		r, calls := newTestRunner(false, "")
		r.MakeTempDir = func(_, _ string, _ *ChartifyOpts) string { return chartDir }

		_, err := r.Chartify("rel", chartDir, WithChartifyOpts(&ChartifyOpts{}))
		require.NoError(t, err)

		depCalls := filterDepCalls(*calls)
		require.Len(t, depCalls, 1)
		require.Equal(t, "build", depCalls[0].args[1])
	})
}

type helmCall struct {
	name string
	args []string
}

func filterDepCalls(calls []helmCall) []helmCall {
	var result []helmCall
	for _, c := range calls {
		if len(c.args) >= 2 && c.args[0] == "dependency" {
			result = append(result, c)
		}
	}
	return result
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
