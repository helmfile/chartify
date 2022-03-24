package chartify

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/variantdev/chartify/chartrepo"
)

var helm string = "helm"

func TestIntegration(t *testing.T) {
	if h := os.Getenv("HELM_BIN"); h != "" {
		helm = h
	}

	repo := "myrepo"

	tempDir := filepath.Join(t.TempDir(), "helm")
	helmCacheHome := filepath.Join(tempDir, "cache")
	helmConfigHone := filepath.Join(tempDir, "config")

	os.Setenv("HELM_CACHE_HOME", helmCacheHome)
	os.Setenv("HELM_CONFIG_HOME", helmConfigHone)

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
}

func startServer(t *testing.T, repo string) {
	srvErr := make(chan error)
	port := 18080
	srv := &chartrepo.Server{
		Port: port,
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		srvErr <- srv.Run(ctx, "testdata/charts")
	}()

	srvStart := make(chan struct{})
	go func() {
		for _ = range time.Tick(1 * time.Second) {
			res, err := http.Get(srv.ServerURL() + "index.yaml")
			if err == nil && res.StatusCode == http.StatusOK {
				break
			}

			if res != nil {
				t.Logf("Waiting for chartrepo server to start: code=%d", res.StatusCode)
			} else {
				t.Logf("Waiting for chartrepo server to start: error=%v", err)
			}
		}

		srvStart <- struct{}{}
	}()

	select {
	case <-srvStart:
		t.Logf("chartrepo server started")
	case <-time.After(10 * time.Second):
		t.Fatalf("unable to restart chartrepo server within 10 seconds")
	}

	t.Cleanup(func() {
		t.Logf("Stopping chartrepo server")

		cancel()

		select {
		case err := <-srvErr:
			if err != nil {
				t.Log("cleanup: stopping cahrtrepo server: " + err.Error())
			}
		case <-time.After(3 * time.Second):
			t.Log("cleanup: unable to stop chartrepo server within 3 seconds")
		}
	})

	helmRepoAdd := exec.CommandContext(ctx, helm, "repo", "add", repo, srv.ServerURL())
	helmRepoAddOut, err := helmRepoAdd.CombinedOutput()
	t.Logf("%s repo add: %s", helm, string(helmRepoAddOut))
	require.NoError(t, err)

	t.Logf("Started chartrepo server")
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

	cmd := exec.CommandContext(ctx, helm, "template", tmpDir)
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
