package chartify

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReadAdhocDependencies(t *testing.T) {
	type testcase struct {
		opts ChartifyOpts
		want []Dependency
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
		if err != nil {
			t.Fatalf("uenxpected error: %v", err)
		}

		if d := cmp.Diff(tc.want, got); d != "" {
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
		want: []Dependency{
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
		want: []Dependency{
			{
				Repository: "http://localhost:18080/",
				Name:       "db",
				Version:    "0.1.0",
				Condition:  "db.enabled",
			},
		},
	})
}
