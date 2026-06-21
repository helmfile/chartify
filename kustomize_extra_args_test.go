package chartify

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type spyCall struct {
	name string
	args []string
}

// newSpyRunner returns a Runner whose RunCommand captures every invocation
// instead of running a real process. The spy:
//   - replies to `kustomize version` so kustomizeLoadRestrictionsNoneFlag /
//     kustomizeEnableAlphaPluginsFlag can derive a flag, and
//   - honors `--output <path>` (used by Patch) by writing a minimal resource
//     YAML there, so callers that read the rendered file succeed.
//
// The kustomize binary name still has to resolve via exec.LookPath, so this
// relies on kustomize being on PATH (already a documented test prerequisite).
func newSpyRunner(t *testing.T, versionOut string) (*Runner, *[]spyCall) {
	t.Helper()
	var calls []spyCall
	r := New()
	r.RunCommand = func(name string, args []string, dir string, stdout, stderr io.Writer, env map[string]string) error {
		calls = append(calls, spyCall{name: name, args: append([]string{}, args...)})

		if len(args) > 0 && args[0] == "version" {
			_, _ = io.WriteString(stdout, versionOut)
			return nil
		}
		for i, a := range args {
			if a == "--output" && i+1 < len(args) {
				_ = os.WriteFile(args[i+1], []byte("kind: ConfigMap\nmetadata:\n  name: test\n"), 0644)
			}
		}
		return nil
	}
	return r, &calls
}

// buildCall returns the first recorded invocation that runs a kustomize "build".
func buildCall(calls []spyCall) (spyCall, bool) {
	for _, c := range calls {
		for _, a := range c.args {
			if a == "build" {
				return c, true
			}
		}
	}
	return spyCall{}, false
}

func TestKustomizeBuildExtraArgs(t *testing.T) {
	r, callsPtr := newSpyRunner(t, "v5.0.0")

	srcDir := t.TempDir()
	tempDir := t.TempDir()

	_, err := r.KustomizeBuild(srcDir, tempDir, &KustomizeBuildOpts{
		ExtraArgs: []string{"--enable-exec"},
	})
	require.NoError(t, err)

	build, ok := buildCall(*callsPtr)
	require.True(t, ok, "expected a kustomize build invocation; calls=%v", *callsPtr)

	// ExtraArgs must be appended to the build command.
	require.Contains(t, build.args, "--enable-exec",
		"ExtraArgs should be appended to the kustomize build command")
	// Standard flags must still be present.
	require.Contains(t, build.args, "--enable-helm")
	require.Contains(t, build.args, "--load-restrictor=LoadRestrictionsNone")

	// ExtraArgs must be inserted before the positional target (tempDir, passed
	// last) so a flag is never mistaken for the build target.
	require.NotEmpty(t, build.args)
	require.Equal(t, tempDir, build.args[len(build.args)-1],
		"the kustomize target must remain the last positional arg")
}

func TestKustomizeBuildNoExtraArgsByDefault(t *testing.T) {
	r, callsPtr := newSpyRunner(t, "v5.0.0")

	_, err := r.KustomizeBuild(t.TempDir(), t.TempDir())
	require.NoError(t, err)

	build, ok := buildCall(*callsPtr)
	require.True(t, ok)
	require.NotContains(t, build.args, "--enable-exec",
		"no extra flags should be added when ExtraArgs is unset")
}

func TestPatchExtraArgs(t *testing.T) {
	r, callsPtr := newSpyRunner(t, "v5.0.0")

	tempDir := t.TempDir()
	manifest := filepath.Join(tempDir, "templates", "cm.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(manifest), 0755))
	require.NoError(t, os.WriteFile(manifest, []byte("kind: ConfigMap\nmetadata:\n  name: test\n"), 0644))

	err := r.Patch(tempDir, []string{"templates/cm.yaml"}, &PatchOpts{
		ExtraArgs: []string{"--enable-exec"},
	})
	require.NoError(t, err)

	build, ok := buildCall(*callsPtr)
	require.True(t, ok, "expected a kustomize build invocation; calls=%v", *callsPtr)
	require.Contains(t, build.args, "--enable-exec",
		"PatchOpts.ExtraArgs should be appended to the kustomize build command")
	require.Equal(t, tempDir, build.args[len(build.args)-1],
		"the kustomize target must remain the last positional arg")
}
