package chartify

import (
	"os"
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
