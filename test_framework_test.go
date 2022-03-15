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

func TestFramework(t *testing.T) {
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

	repo := "myrepo"

	r := New(UseHelm3(true), HelmBin("helm"))
	tc := integrationTestCase{
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
	}

	helmRepoAdd := exec.CommandContext(ctx, "helm", "repo", "add", repo, srv.ServerURL())
	helmRepoAddOut, err := helmRepoAdd.CombinedOutput()
	t.Logf("helm repo add: %s", string(helmRepoAddOut))
	require.NoError(t, err)

	tmpDir, err := r.Chartify(tc.release, tc.chart, WithChartifyOpts(&tc.opts))
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			panic("unable to remove chartify tmpDir: " + err.Error())
		}
	})
	require.NoError(t, err)

	cmd := exec.CommandContext(ctx, "helm", "template", tmpDir)
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
