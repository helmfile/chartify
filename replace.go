package chartify

import (
	"fmt"
	"io/ioutil"

	"os"
	"path/filepath"
	"strings"
)

type ReplaceWithRenderedOpts struct {
	// Debug when set to true passes `--debug` flag to `helm` in order to enable debug logging
	Debug bool

	// ValuesFiles are a list of Helm chart values files
	ValuesFiles []string

	// SetValues is a list of adhoc Helm chart values being passed via helm's `--set` flags
	SetValues []string

	// Namespace is the default namespace in which the K8s manifests rendered by the chart are associated
	Namespace string

	// ChartVersion is the semver of the Helm chart being used to render the original K8s manifests before various tweaks applied by helm-x
	ChartVersion string

	// WorkaroundOutputDirIssue prevents chartify from using `helm template --output-dir` and let it use `helm template > some.yaml` instead to
	// workaround the potential helm issue
	// See https://github.com/roboll/helmfile/issues/1279#issuecomment-636839395
	WorkaroundOutputDirIssue bool
}

func (r *Runner) ReplaceWithRendered(name, chart string, files []string, o ReplaceWithRenderedOpts) ([]string, error) {
	var additionalFlags string
	additionalFlags += createFlagChain("set", o.SetValues)
	defaultValuesPath := filepath.Join(chart, "values.yaml")
	exists, err := r.Exists(defaultValuesPath)
	if err != nil {
		return nil, err
	}
	if exists {
		additionalFlags += createFlagChain("f", []string{defaultValuesPath})
	}
	additionalFlags += createFlagChain("f", o.ValuesFiles)
	if o.Namespace != "" {
		additionalFlags += createFlagChain("namespace", []string{o.Namespace})
	}

	r.Logf("options: %v", o)

	dir := filepath.Join(chart, "helmx.1.rendered")
	if err := os.Mkdir(dir, 0755); err != nil {
		return nil, err
	}

	var command string

	writtenFiles := map[string]bool{}

	if r.isHelm3 && o.WorkaroundOutputDirIssue {
		templatePath := filepath.Join(dir, filepath.Base(chart), "templates", "all.yaml")

		if err := os.MkdirAll(filepath.Dir(templatePath), 0755); err != nil {
			return nil, err
		}

		command = fmt.Sprintf("%s template --debug=%v%s %s %s", r.helmBin(), o.Debug, additionalFlags, name, chart)

		stdout, err := r.run(command)
		if err != nil {
			return nil, err
		}

		if err := ioutil.WriteFile(templatePath, []byte(stdout), 0644); err != nil {
			return nil, err
		}

		writtenFiles[templatePath] = true
	} else {
		if r.isHelm3 {
			command = fmt.Sprintf("%s template --debug=%v --output-dir %s%s %s %s", r.helmBin(), o.Debug, dir, additionalFlags, name, chart)
		} else {
			command = fmt.Sprintf("%s template --debug=%v %s --name %s%s --output-dir %s", r.helmBin(), o.Debug, chart, name, additionalFlags, dir)
		}

		stdout, err := r.run(command)
		if err != nil {
			return nil, err
		}

		lines := strings.Split(stdout, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "wrote ") {
				file := strings.Split(line, "wrote ")[1]
				writtenFiles[file] = true
			}
		}
	}

	if len(writtenFiles) == 0 {
		return nil, fmt.Errorf("invalid state: no files rendered")
	}

	for _, f := range files {
		r.Logf("removing %s", f)
		if err := os.Remove(f); err != nil {
			return nil, err
		}
	}

	results := make([]string, 0, len(writtenFiles))
	for f := range writtenFiles {
		results = append(results, f)
	}
	return results, nil
}
