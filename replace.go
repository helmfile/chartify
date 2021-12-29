package chartify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ReplaceWithRenderedOpts struct {
	// Debug when set to true passes `--debug` flag to `helm` in order to enable debug logging
	Debug bool

	// ValuesFiles are a list of Helm chart values files
	ValuesFiles []string

	// SetValues is a list of adhoc Helm chart values being passed via helm's `--set` flags
	SetValues []string

	// SetFlags is the list of set flags like --set k=v, --set-file k=path, --set-string k=str
	// used while rendering the chart.
	SetFlags []string

	// Namespace is the default namespace in which the K8s manifests rendered by the chart are associated
	Namespace string

	// ChartVersion is the semver of the Helm chart being used to render the original K8s manifests before various tweaks applied by helm-x
	ChartVersion string

	// IncludeCRDs is a Helm 3 only option. When it is true, chartify passes a `--include-crds` flag
	// to helm-template.
	IncludeCRDs bool

	// Validate is a Helm 3 only option. When it is true, chartify passes --validate while running helm-template
	// It is required when your chart contains any template that relies on Capabilities.APIVersions
	// for rendering resourecs depending on the API resources and versions available on a live cluster.
	// In other words, setting this to true means that you need access to a Kubernetes cluster,
	// even if you aren't trying to install the generated chart onto the cluster.
	Validate bool

	// WorkaroundOutputDirIssue prevents chartify from using `helm template --output-dir` and let it use `helm template > some.yaml` instead to
	// workaround the potential helm issue
	// See https://github.com/roboll/helmfile/issues/1279#issuecomment-636839395
	WorkaroundOutputDirIssue bool
}

func (r *Runner) ReplaceWithRendered(name, chartName, chartPath string, o ReplaceWithRenderedOpts) ([]string, error) {
	var additionalFlags string
	additionalFlags += createFlagChain("set", o.SetValues)
	if len(o.SetFlags) > 0 {
		additionalFlags += " " + strings.Join(o.SetFlags, " ")
	}
	defaultValuesPath := filepath.Join(chartPath, "values.yaml")
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

	helmOutputDir := filepath.Join(chartPath, "helmx.1.rendered")
	if err := os.Mkdir(helmOutputDir, 0755); err != nil {
		return nil, err
	}

	// This directory contains templates/ and charts/SUBCHART/templates
	chartOutputDir := filepath.Join(helmOutputDir, chartName)

	var command string

	writtenFiles := map[string]bool{}
	if r.isHelm3 {
		args := []string{
			fmt.Sprintf("--debug=%v", o.Debug),
			fmt.Sprintf("--output-dir=%s", helmOutputDir),
		}

		if o.IncludeCRDs {
			args = append(args, "--include-crds")
		}

		if o.Validate {
			args = append(args, "--validate")
		}

		args = append(args, name, chartPath)

		command = fmt.Sprintf("%s template %s%s", r.helmBin(), strings.Join(args, " "), additionalFlags)
	} else {
		command = fmt.Sprintf("%s template --debug=%v %s --name %s%s --output-dir %s", r.helmBin(), o.Debug, chartPath, name, additionalFlags, helmOutputDir)
	}

	stdout, err := r.run(command)
	if err != nil {
		return nil, err
	}

	// - Replace templates/**/*.yaml with rendered templates/**/*.yaml
	// - Replace charts/SUBCHART.tgz with rendered charts/SUBCHART/templates/*.yaml
	// - Replace crds/*.yaml with rendered crds/*.yaml
	for _, d := range ContentDirs {
		origDir := filepath.Join(chartPath, d)
		if err := os.RemoveAll(origDir); err != nil {
			return nil, err
		}

		newDir := filepath.Join(chartOutputDir, d)
		if _, err := os.Stat(newDir); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if err := os.Rename(newDir, origDir); err != nil {
			return nil, err
		}

		usedDir := filepath.Join(chartPath, "files", d)
		if err := os.RemoveAll(usedDir); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "wrote ") {
			file := strings.Split(line, "wrote ")[1]

			for _, d := range ContentDirs {
				origDir := filepath.Join(chartPath, d)
				newDir := filepath.Join(chartOutputDir, d)
				file = strings.ReplaceAll(strings.ReplaceAll(file, "/", string(filepath.Separator)), newDir, origDir)
			}

			writtenFiles[file] = true
		}
	}

	if len(writtenFiles) == 0 {
		return nil, fmt.Errorf("invalid state: no files rendered")
	}

	results := make([]string, 0, len(writtenFiles))
	for f := range writtenFiles {
		results = append(results, f)
	}

	// Avoids errors like the below due to Chart.yaml containing already dependencies that are already rendered
	// and included in the files and the templates directories.
	//   Error: found in Chart.yaml, but missing in charts/ directory: common, kibana
	if r.IsHelm3() {
		type ChartMeta struct {
			Dependencies []Dependency           `yaml:"dependencies,omitempty"`
			Data         map[string]interface{} `yaml:",inline"`
		}
		var chartMeta ChartMeta

		chartYamlPath := filepath.Join(chartPath, "Chart.yaml")

		bytes, err := r.ReadFile(chartYamlPath)
		if os.IsNotExist(err) {

		} else if err != nil {
			return nil, err
		} else {
			if err := yaml.Unmarshal(bytes, &chartMeta); err != nil {
				return nil, err
			}
		}

		chartMeta.Dependencies = nil

		chartYamlContent, err := yaml.Marshal(&chartMeta)
		if err != nil {
			return nil, fmt.Errorf("marshalling-back %s's Chart.yaml: %w", chartName, err)
		}

		r.Logf("Removing the dependencies field from the original Chart.yaml.")

		if err := r.WriteFile(chartYamlPath, chartYamlContent, 0644); err != nil {
			return nil, err
		}
	} else {
		var reqs Requirements

		bytes, err := r.ReadFile(filepath.Join(chartPath, "requirements.yaml"))
		if os.IsNotExist(err) {

		} else if err != nil {
			return nil, err
		} else {
			if err := yaml.Unmarshal(bytes, &reqs); err != nil {
				return nil, err
			}
		}

		reqs.Dependencies = nil

		requirementsYamlContent, err := yaml.Marshal(&reqs)
		if err != nil {
			return nil, fmt.Errorf("marshalling %s's requirements as YAML: %w", chartName, err)
		}

		if err := r.WriteFile(filepath.Join(chartPath, "requirements.yaml"), requirementsYamlContent, 0644); err != nil {
			return nil, err
		}

		{
			debugOut, err := r.ReadFile(filepath.Join(chartPath, "requirements.yaml"))
			if err != nil {
				return nil, err
			}
			r.Logf("using requirements.yaml:\n%s", debugOut)
		}
	}

	return results, nil
}

func (r *Runner) executeHelmTemplate(release, chart string) error {
	var command string

	if r.isHelm3 {
		command = fmt.Sprintf("%s template --include-crds %s %s", r.helmBin(), release, chart)
	} else {
		command = fmt.Sprintf("%s template --name % s%s", r.helmBin(), release, chart)
	}

	_, err := r.run(command)
	if err != nil {
		return err
	}

	return nil
}
