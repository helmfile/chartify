package chartify

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type RunCommandFunc func(name string, args []string, dir string, stdout, stderr io.Writer, env map[string]string) error

type Runner struct {
	// HelmBinary is the name or the path to `helm` command
	HelmBinary string

	// KustomizeBinary is the name or the path to `kustomize` command
	KustomizeBinary string

	isHelm3 bool

	RunCommand RunCommandFunc

	CopyFile    func(src, dst string) error
	WriteFile   func(filename string, data []byte, perm os.FileMode) error
	ReadFile    func(filename string) ([]byte, error)
	ReadDir     func(dirname string) ([]os.DirEntry, error)
	Walk        func(root string, walkFn filepath.WalkFunc) error
	MakeTempDir func(release, chart string, opts *ChartifyOpts) string
	Exists      func(path string) (bool, error)

	// Logf is the alternative log function used by chartify
	Logf func(string, ...interface{})
}

type Option func(*Runner) error

func HelmBin(b string) Option {
	return func(r *Runner) error {
		r.HelmBinary = b
		return nil
	}
}

func UseHelm3(u bool) Option {
	return func(r *Runner) error {
		r.isHelm3 = u
		return nil
	}
}

func WithLogf(logf func(string, ...interface{})) Option {
	return func(r *Runner) error {
		r.Logf = logf
		return nil
	}
}

func KustomizeBin(b string) Option {
	return func(r *Runner) error {
		r.KustomizeBinary = b
		return nil
	}
}

func New(opts ...Option) *Runner {
	r := &Runner{
		RunCommand:  RunCommand,
		CopyFile:    CopyFile,
		WriteFile:   os.WriteFile,
		ReadFile:    os.ReadFile,
		ReadDir:     os.ReadDir,
		Walk:        filepath.Walk,
		Exists:      exists,
		Logf:        printf,
		MakeTempDir: makeTempDir,
	}

	for i := range opts {
		if err := opts[i](r); err != nil {
			panic(err)
		}
	}

	return r
}

func (r *Runner) helmBin() string {
	if r.HelmBinary != "" {
		return r.HelmBinary
	}
	return os.Getenv("HELM_BIN")
}

func (r *Runner) kustomizeBin() string {
	if r.KustomizeBinary != "" {
		return r.KustomizeBinary
	}
	return "kustomize"
}

// isKustomizeBinaryAvailable checks if the kustomize binary is available
func (r *Runner) isKustomizeBinaryAvailable() bool {
	_, _, err := r.captureBytes(r.kustomizeBin(), []string{"version"}, "", nil)
	return err == nil
}

// kustomizeBuildCommand returns the appropriate command and args for kustomize build operation
// Falls back to "kubectl kustomize" if standalone kustomize binary is not available
func (r *Runner) kustomizeBuildCommand(buildArgs []string, targetDir string) (string, []string, error) {
	// First check if the configured kustomize binary is available
	if r.isKustomizeBinaryAvailable() {
		return r.kustomizeBin(), append(buildArgs, targetDir), nil
	}

	// Fallback to kubectl kustomize
	// kubectl kustomize requires different argument order: kubectl kustomize [flags] DIR
	// We need to transform: kustomize [args] build [more-args] DIR
	// Into: kubectl kustomize [more-args] DIR
	
	kubectlArgs := []string{"kustomize"}
	
	// Extract build-specific flags from buildArgs (everything except "build")
	for i, arg := range buildArgs {
		if arg == "build" {
			// Add everything after "build" except the target dir (which gets added at the end)
			kubectlArgs = append(kubectlArgs, buildArgs[i+1:]...)
			break
		}
		// Add everything before "build"
		kubectlArgs = append(kubectlArgs, arg)
	}
	
	// Add target directory at the end
	kubectlArgs = append(kubectlArgs, targetDir)
	
	return "kubectl", kubectlArgs, nil
}

func (r *Runner) run(envs map[string]string, cmd string, args ...string) (string, error) {
	bytes, err := r.runBytes(envs, "", cmd, args...)

	var out string

	if bytes != nil {
		out = string(bytes)
	}

	return out, err
}

func (r *Runner) runInDir(dir, cmd string, args ...string) (string, error) {
	bytes, err := r.runBytes(nil, dir, cmd, args...)

	var out string

	if bytes != nil {
		out = string(bytes)
	}

	return out, err
}

func (r *Runner) runBytes(envs map[string]string, dir, cmd string, args ...string) ([]byte, error) {
	nameArgs := strings.Split(cmd, " ")

	name := nameArgs[0]

	if len(nameArgs) > 2 {
		a := append([]string{}, nameArgs[1:]...)
		a = append(a, args...)

		args = a
	}

	bytes, errBytes, err := r.captureBytes(name, args, dir, envs)
	if err != nil {
		c := strings.Join(append([]string{name}, args...), " ")

		wrappedErr := fmt.Errorf(`%w

COMMAND:
%s

OUTPUT:
%s`,
			err,
			indent(c, "  "),
			indent(string(errBytes), "  "),
		)

		return bytes, wrappedErr
	}

	return bytes, nil
}

func (r *Runner) IsHelm3() bool {
	if r.isHelm3 {
		return true
	}

	// Support explicit opt-in via environment variable
	if os.Getenv("HELM_X_HELM3") != "" {
		return true
	}

	// Autodetect from `helm version`
	sv, err := r.DetectHelmVersion()
	if err != nil {
		panic(err)
	}

	return sv.Major() == 3
}

// DetectHelmVersion detects the version of Helm installed on the system.
// It runs the `helm version` command and parses the output to extract the client version.
// Returns the detected Helm version as a semver.Version object.
// If an error occurs during the detection process, it returns an error.
func (r *Runner) DetectHelmVersion() (*semver.Version, error) {
	// Autodetect from `helm version`
	out, err := r.run(nil, r.helmBin(), "version", "--client", "--short")
	if err != nil {
		return nil, fmt.Errorf("error determining helm version: %w", err)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("error determining helm version: output was empty")
	}
	v, err := FindSemVerInfo(out)

	if err != nil {
		return nil, fmt.Errorf("error find helm srmver version '%s': %w", out, err)
	}

	ver, err := semver.NewVersion(v)
	if err != nil {
		return nil, fmt.Errorf("error parsing helm version '%s'", ver)
	}

	return ver, nil
}

func (r *Runner) captureBytes(binary string, args []string, dir string, envs map[string]string) ([]byte, []byte, error) {
	r.Logf("running %s %s", binary, strings.Join(args, " "))
	_, err := exec.LookPath(binary)
	if err != nil {
		return nil, nil, err
	}

	var stdout, stderr bytes.Buffer
	err = r.RunCommand(binary, args, dir, &stdout, &stderr, envs)
	if err != nil {
		r.Logf(stderr.String())
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

func printf(format string, vars ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", vars...)
}
