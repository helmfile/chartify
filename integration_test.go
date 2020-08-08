package chartify

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
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

	return fmt.Errorf("%v\n\nListing files under %s:\n%s", err, tmpDir, strings.Join(files, "\n"))
}

type check struct {
	kind      string
	name      string
	namespace string
	cond      string

	f func(*testing.T, map[string]interface{})
}

func TestIntegration(t *testing.T) {
	r := New(UseHelm3(true), HelmBin("helm"))

	testcases := []struct {
		release  string
		chart    string
		snapshot string
		fileList string
		checks   []check
		opts     ChartifyOpts
	}{
		{
			release:  "testrelease",
			chart:    "testdata/kustomize/input",
			snapshot: "testdata/kustomize/output",
			fileList: "testdata/kustomize/filelist.yaml",
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
		{
			// Ensure that adhoc chart dependencies work with existing requirements.yaml with different
			// arrray item indentation
			release:  "testrelease",
			chart:    "stable/prometheus-operator",
			snapshot: "testdata/prometheus-operator-adhoc-dep/output",
			fileList: "testdata/prometheus-operator-adhoc-dep/filelist.yaml",
			opts: ChartifyOpts{
				ChartVersion:           "9.2.2",
				AdhocChartDependencies: []string{"my=stable/mysql:1.6.6"},
			},
		},
		{
			release:  "testrelease",
			chart:    "stable/prometheus-operator",
			snapshot: "testdata/prometheus-operator-adhoc-dep-with-strategicpatch/output",
			fileList: "testdata/prometheus-operator-adhoc-dep-with-strategicpatch/filelist.yaml",
			checks: []check{
				{
					kind:      "Deployment",
					name:      "testrelease-my",
					namespace: "default",
					cond:      "spec.strategy.type must be RollingUpdate",
					f: func(t *testing.T, m map[string]interface{}) {
						spec, ok := m["spec"].(map[string]interface{})
						if ok {
							strategy, ok := spec["strategy"].(map[string]interface{})
							if ok {
								tpe, ok := strategy["type"]
								if ok && tpe == "RollingUpdate" {
									return
								}
							}
						}
						t.Errorf("unexpected spec.strategy.type: %+v", m)
					},
				},
			},
			opts: ChartifyOpts{
				ValuesFiles:            []string{"testdata/prometheus-operator-adhoc-dep-with-strategicpatch/values.yaml"},
				ChartVersion:           "9.2.1",
				AdhocChartDependencies: []string{"my=stable/mysql:1.6.6"},
				StrategicMergePatches:  []string{"testdata/prometheus-operator-adhoc-dep-with-strategicpatch/strategicpatch.yaml"},
			},
		},
		{
			release:  "testrelease",
			chart:    "stable/envoy",
			snapshot: "testdata/envoy-with-ns/output",
			fileList: "testdata/envoy-with-ns/filelist.yaml",
			opts: ChartifyOpts{
				OverrideNamespace: "foo",
				ChartVersion:      "1.9.1",
				ValuesFiles:       []string{"testdata/envoy-with-ns/values.yaml"},
			},
		},
	}

	for i, tc := range testcases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
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

			if err := filepath.Walk(tc.snapshot, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					t.Errorf("unexpected error: %v", err)

					return err
				}

				if info.IsDir() {
					return nil
				}

				rel, err := filepath.Rel(tc.snapshot, path)
				if err != nil {
					return fmt.Errorf("calculating relative path to %s from %s: %v", path, tc.snapshot, err)
				}

				want, err := ioutil.ReadFile(path)
				if err != nil {
					return fmt.Errorf("reading wanted file %s: %v", path, err)
				}

				gotFile := filepath.Join(tmpDir, rel)
				got, err := ioutil.ReadFile(gotFile)
				if err != nil {
					if os.IsNotExist(err) {
						filesDir := filepath.Dir(gotFile)
						t.Errorf("expected file %s not found: %v", gotFile, errWithFiles(err, filesDir))
						return nil
					}
					return fmt.Errorf("reading expected file %s: %v", gotFile, err)
				}

				if diff := cmp.Diff(string(want), string(got)); diff != "" {
					t.Errorf("unexpected diff on %s:\n%s", path, diff)

					return nil
				}

				return nil
			}); err != nil {
				t.Fatalf("unexpected error while comparing result to snapshot: %v", err)
			}

			if tc.fileList != "" {
				bs, err := ioutil.ReadFile(tc.fileList)
				if err != nil {
					t.Fatalf("%v", err)
				}

				var fileList []string

				if err := yaml.Unmarshal(bs, &fileList); err != nil {
					t.Fatalf("%v", err)
				}

				var paths []string
				if err := filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
					rel, err := filepath.Rel(tmpDir, path)
					if err != nil {
						return err
					}

					paths = append(paths, rel)

					if info.IsDir() || filepath.Ext(path) != ".yaml" {
						return nil
					}

					//
					// Check partial content
					//

					passed := map[int]bool{}

					m := map[string]interface{}{}
					bs, err := ioutil.ReadFile(path)
					if err != nil {
						t.Fatalf("%v", err)
					}

					if strings.HasPrefix(string(bs), "{{ .Files.Get \"") {
						// Looks like chartify-generated helm template
						return nil
					}

					f, err := os.Open(path)
					if err != nil {
						t.Fatalf("opening %s: %v", path, err)
					}

					dec := yaml.NewDecoder(f)

					for ; ; {
						err := dec.Decode(&m)
						if err != nil {
							if err == io.EOF {
								break
							}
							t.Fatalf("%v:\nYAML:\n%s", err, string(bs))
						}

						kind := m["kind"]
						metadata, ok := m["metadata"].(map[string]interface{})
						if !ok {
							// Looks like a non-K8s YAML (like Chart.yaml and values.yaml)
							continue
						}
						name := metadata["name"].(string)
						namespace, ok := metadata["namespace"].(string)
						if !ok {
							namespace = ""
						}
						for i, check := range tc.checks {
							if kind == check.kind && name == check.name && namespace == check.namespace {
								check.f(t, m)
								passed[i] = true
							}
						}
					}

					var checks []check
					for i, check := range tc.checks {
						if !passed[i] {
							checks = append(checks, check)
						}
					}
					tc.checks = checks

					return nil
				}); err != nil {
					t.Fatalf("%v", err)
				}

				if len(tc.checks) > 0 {
					var lines []string
					for _, c := range tc.checks {
						lines = append(lines, fmt.Sprintf("%s/%s (%s) %s", c.namespace, c.name, c.kind, c.cond))
					}
					msg := strings.Join(lines, "\n")
					t.Errorf("%d checks are remaining:\n%s", len(tc.checks), msg)
				}

				sort.Strings(paths)

				if diff := cmp.Diff(fileList, paths); diff != "" {
					t.Errorf("unexpected files:\n%s", diff)
				}
			}

			if err := r.executeHelmTemplate("foo", tmpDir); err != nil {
				t.Errorf("unexpected error while verifying the final chart with helm template: %v", err)
			}
		})
	}
}
