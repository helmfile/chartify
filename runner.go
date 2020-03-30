package chartify

import (
	"fmt"
	"github.com/variantdev/chartify/pkg/cmdsite"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/otiai10/copy"
)

type Runner struct {
	helmBin   string
	isHelm3   bool
	commander *cmdsite.CommandSite
}

type Option func(*Runner) error

func Commander(c cmdsite.RunCommand) Option {
	return func(r *Runner) error {
		r.commander.RunCmd = c
		return nil
	}
}

func HelmBin(b string) Option {
	return func(r *Runner) error {
		r.helmBin = b
		return nil
	}
}

func UseHelm3(u bool) Option {
	return func(r *Runner) error {
		r.isHelm3 = u
		return nil
	}
}

func New(opts ...Option) *Runner {
	cs := cmdsite.New()
	cs.RunCmd = DefaultRunCommand
	r := &Runner{
		commander: cs,
	}
	for i := range opts {
		if err := opts[i](r); err != nil {
			panic(err)
		}
	}
	return r
}

func (r *Runner) HelmBin() string {
	if r.helmBin != "" {
		return r.helmBin
	}
	return os.Getenv("HELM_BIN")
}

func (r *Runner) Run(name string, args ...string) (string, error) {
	bytes, _, err := r.CaptureBytes(name, args)
	if err != nil {
		var out string
		if bytes != nil {
			out = string(bytes)
		}
		return out, err
	}
	return string(bytes), nil
}

func DefaultRunCommand(cmd string, args []string, stdout, stderr io.Writer, env map[string]string) error {
	command := exec.Command(cmd, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = mergeEnv(os.Environ(), env)
	return command.Run()
}

func mergeEnv(orig []string, new map[string]string) []string {
	wanted := env2map(orig)
	for k, v := range new {
		wanted[k] = v
	}
	return map2env(wanted)
}

func map2env(wanted map[string]string) []string {
	result := []string{}
	for k, v := range wanted {
		result = append(result, k+"="+v)
	}
	return result
}

func env2map(env []string) map[string]string {
	wanted := map[string]string{}
	for _, cur := range env {
		pair := strings.SplitN(cur, "=", 2)
		wanted[pair[0]] = pair[1]
	}
	return wanted
}

func (r *Runner) IsHelm3() bool {
	if r.isHelm3 {
		return true
	}

	// Support explicit opt-in via environment variable
	if os.Getenv("HELM_X_HELM3") != "" {
		return true
	}

	// Autodetect from `helm verison`
	bytes, err := r.Run(r.HelmBin(), "version", "--client", "--short")
	if err != nil {
		panic(err)
	}

	return strings.HasPrefix(string(bytes), "v3.")
}

// copyToTempDir checks if the path is local or a repo (in this order) and copies it to a temp directory
// It will perform a `helm fetch` if required
func (r *Runner) copyToTempDir(path string) (string, error) {
	tempDir := mkRandomDir(os.TempDir())
	exists, err := exists(path)
	if err != nil {
		return "", err
	}
	if !exists {
		return r.fetchAndUntarUnderDir(path, tempDir)
	}
	err = copy.Copy(path, tempDir)
	if err != nil {
		return "", err
	}
	return tempDir, nil
}

// exists returns whether the given file or directory exists or not
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func (r *Runner) fetchAndUntarUnderDir(path, tempDir string) (string, error) {
	command := fmt.Sprintf("helm fetch %s --untar -d %s", path, tempDir)
	_, stderr, err := r.DeprecatedCaptureBytes(command)
	if err != nil || len(stderr) != 0 {
		return "", fmt.Errorf(string(stderr))
	}
	files, err := ioutil.ReadDir(tempDir)
	if err != nil {
		return "", err
	}
	if len(files) != 1 {
		return "", fmt.Errorf("%d additional files found in temp direcotry. This is very strange", len(files)-1)
	}
	return filepath.Join(tempDir, files[0].Name()), nil
}

// MkRandomDir creates a new directory with a random name made of numbers
func mkRandomDir(basepath string) string {
	r := strconv.Itoa((rand.New(rand.NewSource(time.Now().UnixNano()))).Int())
	path := filepath.Join(basepath, r)
	os.Mkdir(path, 0755)

	return path
}

func (r *Runner) untarUnderDir(path, tempDir string) (string, error) {
	command := fmt.Sprintf("tar -zxvf %s -C %s", path, tempDir)
	_, stderr, err := r.DeprecatedCaptureBytes(command)
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, string(stderr))
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	files, err := ioutil.ReadDir(tempDir)
	if err != nil {
		return "", err
	}
	if len(files) != 1 {
		fs := []string{}
		for _, f := range files {
			fs = append(fs, f.Name())
		}
		return "", fmt.Errorf("%d additional files found in temp direcotry. This is very strange:\n%s", len(files)-1, strings.Join(fs, "\n"))
	}
	return filepath.Join(tempDir, files[0].Name()), nil
}

// DeprecatedCaptureBytes takes a command as a string and executes it, and returns the captured stdout and stderr
func (r *Runner) DeprecatedCaptureBytes(cmd string) ([]byte, []byte, error) {
	args := strings.Split(cmd, " ")
	binary := args[0]
	_, err := exec.LookPath(binary)
	if err != nil {
		return nil, nil, err
	}

	return r.CaptureBytes(binary, args[1:])
}

func (r *Runner) CaptureBytes(binary string, args []string) ([]byte, []byte, error) {
	return r.commander.CaptureBytes(binary, args)
}
