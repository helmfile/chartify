package chartify

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCreateFlagChain(t *testing.T) {
	testcases := []struct {
		flag   string
		values []string
		expect string
	}{
		{
			flag:   "foo",
			values: []string{"1"},
			expect: " --foo 1",
		},
		{
			flag:   "foo",
			values: []string{"1", "2"},
			expect: " --foo 1 --foo 2",
		},
		{
			flag:   "f",
			values: []string{"a"},
			expect: " -f a",
		},
		{
			flag:   "f",
			values: []string{"a", "b"},
			expect: " -f a -f b",
		},
	}

	for i, tc := range testcases {
		actual := createFlagChain(tc.flag, tc.values)

		if diff := cmp.Diff(tc.expect, actual); diff != "" {
			t.Errorf("case %d:\n%s", i, diff)
		}
	}
}
