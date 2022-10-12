package chartify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/variantdev/chartify/helmtesting"
)

var helm string = "helm"

func TestIntegration(t *testing.T) {
	if h := os.Getenv("HELM_BIN"); h != "" {
		helm = h
	}

	setupHelmConfig(t)

	repo := "myrepo"
	startServer(t, repo)

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/adhoc_dependency_condition$ ./
	runTest(t, integrationTestCase{
		description: "adhoc dependency condition",
		release:     "myapp",
		chart:       repo + "/db",
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Alias:   "log",
					Chart:   repo + "/log",
					Version: "0.1.0",
				},
			},
			SetFlags: []string{
				"--set", "log.enabled=true",
			},
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/adhoc_dependency_condition_disabled$ ./
	runTest(t, integrationTestCase{
		description: "adhoc dependency condition disabled",
		release:     "myapp",
		chart:       repo + "/db",
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Alias:   "log",
					Chart:   repo + "/log",
					Version: "0.1.0",
				},
			},
			SetFlags: []string{
				"--set", "log.enabled=false",
			},
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/adhoc_dependency_condition_default$ ./
	runTest(t, integrationTestCase{
		description: "adhoc dependency condition default",
		release:     "myapp",
		chart:       repo + "/db",
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Alias:   "log",
					Chart:   repo + "/log",
					Version: "0.1.0",
				},
			},
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/force_namespace$ ./
	runTest(t, integrationTestCase{
		description: "force namespace",
		release:     "myapp",
		chart:       repo + "/db",
		opts: ChartifyOpts{
			OverrideNamespace: "force",
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/kube_version_and_api_versions$ ./
	runTest(t, integrationTestCase{
		description: "kube_version_and_api_versions",
		release:     "vers1",
		chart:       repo + "/vers",
		opts: ChartifyOpts{
			KubeVersion: "1.23.0",
			ApiVersions: []string{
				"foo/v1alpha1",
				"bar/v1beta1",
				"baz/v1",
			},
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/disabled_inaccessible_chart_yaml_dep$ ./
	runTest(t, integrationTestCase{
		description: "disabled inaccessible chart yaml dep",
		release:     "inaccessible1",
		chart:       repo + "/inaccessibledep",
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Alias:   "log",
					Chart:   repo + "/log",
					Version: "0.1.0",
				},
			},
			SetFlags: []string{
				"--set", "relns.enabled=false",
				"--set", "log.enabled=true",
			},
		},
	})

	//
	// Local Chart
	//

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/local_chart_with_adhoc_dependency$ ./
	runTest(t, integrationTestCase{
		description: "local chart with adhoc dependency",
		release:     "myapp",
		chart:       "./testdata/charts/db",
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Alias:   "log",
					Chart:   repo + "/log",
					Version: "0.1.0",
				},
			},
			SetFlags: []string{
				"--set", "log.enabled=true",
			},
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/local_chart_with_slash_at_the_end$ ./
	// Related to https://github.com/variantdev/chartify/pull/13
	runTest(t, integrationTestCase{
		description: "local chart with slash at the end",
		release:     "myapp",
		chart:       "./testdata/charts/db/./",
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Alias:   "log",
					Chart:   repo + "/log",
					Version: "0.1.0",
				},
			},
			StrategicMergePatches: []string{
				"./testdata/chart_patch/deploy.db.strategic.yaml",
			},
			SetFlags: []string{
				"--set", "log.enabled=true",
			},
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/local_chart_with_dot_at_the_end$ ./
	// Related to https://github.com/variantdev/chartify/pull/13
	runTest(t, integrationTestCase{
		description: "local chart with dot at the end",
		release:     "myapp",
		chart:       "./testdata/charts/db/./.",
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Alias:   "log",
					Chart:   repo + "/log",
					Version: "0.1.0",
				},
			},
			StrategicMergePatches: []string{
				"./testdata/chart_patch/deploy.db.strategic.yaml",
			},
			SetFlags: []string{
				"--set", "log.enabled=true",
			},
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/local_chart_with_release_namespace$ ./
	// Related to https://github.com/variantdev/chartify/pull/13
	runTest(t, integrationTestCase{
		description: "local chart with release namespace",
		release:     "myapp",
		chart:       "./testdata/charts/relns",
		opts: ChartifyOpts{
			Namespace: "myns",
			StrategicMergePatches: []string{
				"./testdata/chart_patch/configmap.relns.strategic.yaml",
			},
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/local_chart_with_chart_name_unequal_to_dir_name$ ./
	// Related to https://github.com/variantdev/chartify/pull/13#issuecomment-1077431214
	runTest(t, integrationTestCase{
		description: "local chart with chart name unequal to dir name",
		release:     "myapp",
		chart:       "./testdata/localchart",
		opts: ChartifyOpts{
			Namespace: "myns",
			StrategicMergePatches: []string{
				"./testdata/chart_patch/configmap.chartname.strategic.yaml",
			},
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/local_tgz_chart$ ./
	runTest(t, integrationTestCase{
		description: "local tgz chart",
		release:     "myapp",
		chart:       "./testdata/chartname-0.1.0.tgz",
		opts: ChartifyOpts{
			Namespace: "myns",
			StrategicMergePatches: []string{
				"./testdata/chart_patch/configmap.chartname.strategic.yaml",
			},
		},
	})

	//
	// Kubernets Manifests
	//

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/kube_manifest_with_adhoc_dep$ ./
	runTest(t, integrationTestCase{
		description: "kube_manifest_with_adhoc_dep",
		release:     "myapp",
		chart:       "./testdata/kube_manifest",
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Alias:   "log",
					Chart:   repo + "/log",
					Version: "0.1.0",
				},
			},
			SetFlags: []string{
				"--set", "log.enabled=true",
			},
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/kube_manifest_with_patch$ ./
	runTest(t, integrationTestCase{
		description: "kube_manifest_with_patch",
		release:     "myapp",
		chart:       "./testdata/kube_manifest",
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Alias:   "log",
					Chart:   repo + "/log",
					Version: "0.1.0",
				},
			},
			StrategicMergePatches: []string{
				"./testdata/kube_manifest_patch/cm.strategic.yaml",
			},
			SetFlags: []string{
				"--set", "log.enabled=true",
			},
		},
	})

	// SAVE_SNAPSHOT=1 go1.17 test -run ^TestIntegration/kube_manifest_yml_with_patch$ ./
	runTest(t, integrationTestCase{
		description: "kube_manifest_yml_with_patch",
		release:     "myapp",
		chart:       "./testdata/kube_manifest_yml",
		opts: ChartifyOpts{
			AdhocChartDependencies: []ChartDependency{
				{
					Alias:   "log",
					Chart:   repo + "/log",
					Version: "0.1.0",
				},
			},
			StrategicMergePatches: []string{
				"./testdata/kube_manifest_patch/cm.strategic.yaml",
			},
			SetFlags: []string{
				"--set", "log.enabled=true",
			},
		},
	})
}

func setupHelmConfig(t *testing.T) {
	tempDir := filepath.Join(t.TempDir(), "helm")
	helmCacheHome := filepath.Join(tempDir, "cache")
	helmConfigHone := filepath.Join(tempDir, "config")

	os.Setenv("HELM_CACHE_HOME", helmCacheHome)
	os.Setenv("HELM_CONFIG_HOME", helmConfigHone)
}

func startServer(t *testing.T, repo string) {
	t.Helper()

	port := 18080
	srv := helmtesting.ChartRepoServerConfig{
		Port:      port,
		ChartsDir: "testdata/charts",
	}
	s := helmtesting.StartChartRepoServer(t, srv)
	helmtesting.AddChartRepo(t, helm, repo, s)
}

func runTest(t *testing.T, tc integrationTestCase) {
	t.Helper()

	t.Run(tc.description, func(t *testing.T) {
		doTest(t, tc)
	})
}

func doTest(t *testing.T, tc integrationTestCase) {
	t.Helper()

	ctx := context.Background()

	r := New(UseHelm3(true), HelmBin(helm))

	tmpDir, err := r.Chartify(tc.release, tc.chart, WithChartifyOpts(&tc.opts))
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			panic("unable to remove chartify tmpDir: " + err.Error())
		}
	})
	require.NoError(t, err)

	if info, _ := os.Stat(tc.chart); info != nil {
		// Our contract (mainly for Helmfile) is that any local chart can pass
		// subsequent `helm dep build` on it after chartification
		// https://github.com/roboll/helmfile/issues/2074#issuecomment-1068335836
		cmd := exec.CommandContext(ctx, helm, "dependency", "build", tmpDir)
		helmDepBuildOut, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("%s depependency build: %s", helm, string(helmDepBuildOut))
		}
		require.NoError(t, err)
	}

	args := []string{"template", tc.release, tmpDir}
	args = append(args, tc.opts.SetFlags...)
	if tc.opts.KubeVersion != "" {
		args = append(args, "--kube-version", tc.opts.KubeVersion)
	}
	for _, v := range tc.opts.ApiVersions {
		args = append(args, "--api-versions", v)
	}

	cmd := exec.CommandContext(ctx, helm, args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	got := string(out)

	snapshotFile := filepath.Join("testdata", "integration", "testcases", strings.ReplaceAll(tc.description, " ", "_"), "want")

	// You can update the snapshot by running e.g.:
	//   SAVE_SNAPSHOT=1 go1.17 test -run ^TestFramework$ ./
	if os.Getenv("SAVE_SNAPSHOT") != "" {
		testcaseDir := filepath.Dir(snapshotFile)
		err = os.MkdirAll(testcaseDir, 0755)
		require.NoError(t, err, "creating testcase dir %s", testcaseDir)
		err = os.WriteFile(snapshotFile, out, 0644)
		require.NoError(t, err, "saving snapshot to %s", snapshotFile)
	}

	snapshot, err := os.ReadFile(snapshotFile)
	require.NoError(t, err, "reading snapshot %s", snapshotFile)

	want := string(snapshot)
	require.Equal(t, want, got)
}

type integrationTestCase struct {
	description string
	release     string
	chart       string
	opts        ChartifyOpts
}
