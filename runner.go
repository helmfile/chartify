package chartify

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func (r *Runner) run(cmd string, args ...string) (string, error) {
	bytes, err := r.runBytes("", cmd, args...)

	var out string

	if bytes != nil {
		out = string(bytes)
	}

	return out, err
}

func (r *Runner) runInDir(dir, cmd string, args ...string) (string, error) {
	bytes, err := r.runBytes(dir, cmd, args...)

	var out string

	if bytes != nil {
		out = string(bytes)
	}

	return out, err
}

func (r *Runner) runBytes(dir, cmd string, args ...string) ([]byte, error) {
	nameArgs := strings.Split(cmd, " ")

	name := nameArgs[0]

	if len(nameArgs) > 2 {
		a := append([]string{}, nameArgs[1:]...)
		a = append(a, args...)

		args = a
	}

	bytes, errBytes, err := r.captureBytes(name, args, dir)
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
	out, err := r.run(r.helmBin(), "version", "--client", "--short")
	if err != nil {
		panic(err)
	}

	return strings.HasPrefix(out, "v3.")
}

func (r *Runner) captureBytes(binary string, args []string, dir string) ([]byte, []byte, error) {
	r.Logf("running %s %s", binary, strings.Join(args, " "))
	_, err := exec.LookPath(binary)
	if err != nil {
		return nil, nil, err
	}

	var stdout, stderr bytes.Buffer
	err = r.RunCommand(binary, args, dir, &stdout, &stderr, map[string]string{})
	if err != nil {
		r.Logf(stderr.String())
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

func printf(format string, vars ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", vars...)
}
