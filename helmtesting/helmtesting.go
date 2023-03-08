package helmtesting

import (
	"context"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/helmfile/chartify/chartrepo"
)

type ChartRepoServerConfig = chartrepo.Server
type ChartRepoServer string

func (r ChartRepoServer) URL() string {
	return string(r)
}

// StartChartRepoServer starts a local helm chart server and returns ChartRepoServer that
// contains various information like the local server's URL.
func StartChartRepoServer(t *testing.T, srv ChartRepoServerConfig) ChartRepoServer {
	t.Helper()

	srvErr := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		srvErr <- srv.Run(ctx)
	}()

	srvStart := make(chan struct{})
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			res, err := http.Get(srv.ServerURL() + "index.yaml")
			if err == nil && res.StatusCode == http.StatusOK {
				_ = res.Body.Close()
				break
			}

			if res != nil {
				_ = res.Body.Close()
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
				t.Log("cleanup: stopping chartrepo server: " + err.Error())
			}
		case <-time.After(3 * time.Second):
			t.Log("cleanup: unable to stop chartrepo server within 3 seconds")
		}
	})

	t.Logf("Started chartrepo server")

	return ChartRepoServer(srv.ServerURL())
}

// AddChartRepo names the specified chart repo server so that it can be used by helm as a chart repo
func AddChartRepo(t *testing.T, helm, name string, srv ChartRepoServer) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	helmRepoAdd := exec.CommandContext(ctx, helm, "repo", "add", name, srv.URL())
	helmRepoAddOut, err := helmRepoAdd.CombinedOutput()
	t.Logf("%s repo add: %s", helm, string(helmRepoAddOut))
	require.NoError(t, err)
}
