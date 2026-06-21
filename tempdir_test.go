package chartify

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGenerateID(t *testing.T) {
	type testcase struct {
		release string
		chart   string
		opts    ChartifyOpts
		want    string
	}

	ids := map[string]int{}

	run := func(tc testcase) {
		t.Helper()

		got, err := GenerateID(tc.release, tc.chart, &tc.opts)
		if err != nil {
			t.Fatalf("uenxpected error: %v", err)
		}

		if d := cmp.Diff(tc.want, got); d != "" {
			t.Fatalf("unexpected result: want (-), got (+):\n%s", d)
		}

		ids[got]++
	}

	run(testcase{
		release: "foo",
		chart:   "incubator/raw",
		opts:    ChartifyOpts{},
		want:    "foo-ff86f54bd",
	})

	run(testcase{
		release: "foo",
		chart:   "stable/envoy",
		opts:    ChartifyOpts{},
		want:    "foo-8b5d8bf5",
	})

	run(testcase{
		release: "bar",
		chart:   "incubator/raw",
		opts:    ChartifyOpts{},
		want:    "bar-5f9576b4b7",
	})

	run(testcase{
		release: "foo",
		opts: ChartifyOpts{
			Namespace: "myns",
		},
		want: "myns-foo-595c9b47bc",
	})

	for id, n := range ids {
		if n > 1 {
			t.Fatalf("too many occurrences of %s: %d", id, n)
		}
	}
}
