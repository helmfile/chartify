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
		want:    "foo-5b569f58ff",
	})

	run(testcase{
		release: "foo",
		chart:   "stable/envoy",
		opts:    ChartifyOpts{},
		want:    "foo-7b5895898b",
	})

	run(testcase{
		release: "bar",
		chart:   "incubator/raw",
		opts:    ChartifyOpts{},
		want:    "bar-6c86897994",
	})

	run(testcase{
		release: "foo",
		opts: ChartifyOpts{
			Namespace: "myns",
		},
		want: "myns-foo-564b5d8874",
	})

	for id, n := range ids {
		if n > 1 {
			t.Fatalf("too many occurrences of %s: %d", id, n)
		}
	}
}
