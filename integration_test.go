package chartify

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegration(t *testing.T) {
	r := New(UseHelm3(true), HelmBin("helm"))

	testcases := []struct {
		release string
		chart   string
		opts    ChartifyOpts
		expectedOutputFilename string
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
			expectedOutputFilename: "templates/0-kustomized.yaml",
		},
		{
			release: "testrelease",
			chart:   "testdata/helm-chart",
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
				JsonPatches:                 []string{
					"testdata/helm-chart/patch.myapp.yaml",
					"testdata/helm-chart/patch.mysql.yaml",
				},
				StrategicMergePatches:       nil,
			},
			expectedOutputFilename: "templates/helmx.all.yaml",
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
			t.Error(err)
		}

		if _, err := ioutil.ReadFile(filepath.Join(tmpDir, tc.expectedOutputFilename)); err != nil {
			t.Error(err)
		}
	}
}
