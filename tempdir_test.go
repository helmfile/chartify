package chartify

import (
	"github.com/google/go-cmp/cmp"
	"testing"
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
		want:    "foo-79fc666848",
	})

	run(testcase{
		release: "foo",
		chart:   "stable/envoy",
		opts:    ChartifyOpts{},
		want:    "foo-6fc7d45f69",
	})

	run(testcase{
		release: "bar",
		chart:   "incubator/raw",
		opts:    ChartifyOpts{},
		want:    "bar-58858795d9",
	})

	run(testcase{
		release: "foo",
		opts: ChartifyOpts{
			Namespace: "myns",
		},
		want: "myns-foo-6c7f9bd567",
	})

	for id, n := range ids {
		if n > 1 {
			t.Fatalf("too many occurences of %s: %d", id, n)
		}
	}
}
