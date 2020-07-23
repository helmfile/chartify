package chartify

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func errWithFiles(err error, tmpDir string) error {
	files := []string{}

	globErr := filepath.Walk(tmpDir, func(path string, f os.FileInfo, err error) error {
		files = append(files, path)

		return nil
	})
	if globErr != nil {
		return fmt.Errorf("augumenting original error %v with files under %q: %v", err, tmpDir, globErr)
	}

	return fmt.Errorf("%v\n\nLISTING FILES:\n%s", err, strings.Join(files, "\n"))
}

func TestIntegration(t *testing.T) {
	r := New(UseHelm3(true), HelmBin("helm"))

	testcases := []struct {
		release string
		chart   string
		opts    ChartifyOpts
	}{
		{
			release: "testrelease",
			chart:   "testdata/kustomize",
			opts: ChartifyOpts{
				Debug:                       false,
				ValuesFiles:                 nil,
				SetValues:                   nil,
				Namespace:                   "",
				ChartVersion:                "",
				TillerNamespace:             "",
				EnableKustomizeAlphaPlugins: false,
				Injectors:                   nil,
				Injects:                     nil,
				AdhocChartDependencies:      nil,
				JsonPatches:                 nil,
				StrategicMergePatches:       nil,
			},
		},
	}

	for _, tc := range testcases {
		tmpDir, err := r.Chartify(tc.release, tc.chart, WithChartifyOpts(&tc.opts))

		if tmpDir != "" && strings.HasPrefix(tmpDir, os.TempDir()) {
			if os.Getenv("RETAIN_TEMP_DIR") != "" {
				t.Logf("Retaining %q", tmpDir)
			} else {
				defer os.RemoveAll(tmpDir)
			}
		}

		if err != nil {
			t.Fatalf("Integration test failed: %v\n\nTo debug, re-run with RETAIN_TEMP_DIR=1", err)
		}

		if _, err := ioutil.ReadFile(filepath.Join(tmpDir, "kustomization.yaml")); err != nil {
			t.Error(errWithFiles(err, tmpDir))
		}

		if _, err := ioutil.ReadFile(filepath.Join(tmpDir, "files/0-kustomized.yaml")); err != nil {
			t.Error(errWithFiles(err, tmpDir))
		}

		if _, err := ioutil.ReadFile(filepath.Join(tmpDir, "templates/helmx.all.yaml")); err != nil {
			t.Error(errWithFiles(err, tmpDir))
		}
	}
}
