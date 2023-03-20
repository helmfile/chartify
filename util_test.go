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

// TestFindSemVerInfo tests the FindSemVerInfo function.
func TestFindSemVerInfo(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
		{
			name:    "valid semver for v4",
			version: "{Version:kustomize/v4.5.7 GitCommit:56d82a8378dfc8dc3b3b1085e5a6e67b82966bd7 BuildDate:2022-08-02T16:35:54Z GoOs:darwin GoArch:arm64}",
			want:    "v4.5.7",
			wantErr: false,
		},
		{
			name:    "valid semver for v3",
			version: "{Version:kustomize/v3.10.0 GitCommit:602ad8aa98e2e17f6c9119e027a09757e63c8bec BuildDate:2021-02-10T00:00:50Z GoOs:linux GoArch:amd64}",
			want:    "v3.10.0",
			wantErr: false,
		},
		{
			name:    "invalid semver",
			version: "version",
			want:    "",
			wantErr: true,
		},
		{
			name:    "valid semver for v5",
			version: "v5.0.0",
			want:    "v5.0.0",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindSemVerInfo(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindSemVerInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FindSemVerInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}
