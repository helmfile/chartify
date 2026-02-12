package chartify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
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
		{
			name:    "semver with extra info",
			version: "v1.2.3-alpha+001",
			want:    "v1.2.3-alpha+001",
			wantErr: false,
		},
		{
			name:    "semver not start with v",
			version: "1.2.3",
			want:    "v1.2.3",
			wantErr: false,
		},
		{
			name:    "helm version info with WARNING message",
			version: "WARNING: both 'platformCommand' and 'command' are set in \"<HOMEDIR>/snap/code/194/.local/share/helm/plugins/helm-secrets/plugin.yaml\" (this will become an error in a future Helm version)\nv4.0.0+g99cd196",
			want:    "v4.0.0+g99cd196",
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

func TestKustomizeBin(t *testing.T) {
	t.Run("KustomizeBinary option is set", func(t *testing.T) {
		r := New(KustomizeBin("/custom/kustomize"))
		got := r.kustomizeBin()
		want := "/custom/kustomize"
		if got != want {
			t.Errorf("kustomizeBin() = %v, want %v", got, want)
		}
	})

	t.Run("KUSTOMIZE_BIN environment variable", func(t *testing.T) {
		if _, ok := os.LookupEnv("KUSTOMIZE_BIN"); ok {
			t.Skip("KUSTOMIZE_BIN environment variable is already set")
		}
		os.Setenv("KUSTOMIZE_BIN", "/custom/kustomize")
		defer os.Unsetenv("KUSTOMIZE_BIN")
		r := New()
		got := r.kustomizeBin()
		want := "/custom/kustomize"
		if got != want {
			t.Errorf("kustomizeBin() = %v, want %v", got, want)
		}
	})

	t.Run("fallback to kubectl kustomize when kustomize not found", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0755))

		kubectlPath := filepath.Join(binDir, "kubectl")
		kubectlContent := []byte("#!/bin/sh\necho 'kubectl version'\n")
		require.NoError(t, os.WriteFile(kubectlPath, kubectlContent, 0755))

		origPath := os.Getenv("PATH")
		defer os.Setenv("PATH", origPath)
		os.Setenv("PATH", binDir)

		r := New()
		got := r.kustomizeBin()
		want := "kubectl kustomize"
		if got != want {
			t.Errorf("kustomizeBin() = %v, want %v", got, want)
		}
	})

	t.Run("use kustomize when both kustomize and kubectl exist in PATH", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0755))

		kustomizePath := filepath.Join(binDir, "kustomize")
		kustomizeContent := []byte("#!/bin/sh\necho 'kustomize version'\n")
		require.NoError(t, os.WriteFile(kustomizePath, kustomizeContent, 0755))

		kubectlPath := filepath.Join(binDir, "kubectl")
		kubectlContent := []byte("#!/bin/sh\necho 'kubectl version'\n")
		require.NoError(t, os.WriteFile(kubectlPath, kubectlContent, 0755))

		origPath := os.Getenv("PATH")
		defer os.Setenv("PATH", origPath)
		os.Setenv("PATH", binDir)

		r := New()
		got := r.kustomizeBin()
		want := "kustomize"
		if got != want {
			t.Errorf("kustomizeBin() = %v, want %v", got, want)
		}
	})

	t.Run("return kustomize as fallback when neither kustomize nor kubectl exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0755))

		origPath := os.Getenv("PATH")
		defer os.Setenv("PATH", origPath)
		os.Setenv("PATH", binDir)

		r := New()
		got := r.kustomizeBin()
		want := "kustomize"
		if got != want {
			t.Errorf("kustomizeBin() = %v, want %v", got, want)
		}
	})
}
