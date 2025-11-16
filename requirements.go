package chartify

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Requirements struct {
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

type Dependency struct {
	Name       string `yaml:"name,omitempty"`
	Repository string `yaml:"repository,omitempty"`
	Condition  string `yaml:"condition,omitempty"`
	Alias      string `yaml:"alias,omitempty"`
	Version    string `yaml:"version,omitempty"`
	// ImportValues holds the mapping of source values to parent key to be imported. Each item can be a
	// string or pair of child/parent sublist items.
	ImportValues []interface{} `yaml:"import-values,omitempty"`
}

type ChartDependency struct {
	Alias   string
	Chart   string
	Version string
}

// UpdateRequirements updates either Chart.yaml's dependencies(helm 3) or requirements.yaml(helm 2)
// so that our subsequent run of `helm dep up` can fulfill missing chart dependencies.
// If it's a remote chart, only adhoc dependencies needs to be downloaded by `helm dep up` because the original
// chart dependencies shold have been already fetched by preceding `helm fetch`.
// If it's a local chart, unlike a remote chart there is not preceding step like `helm fetch` to download chart dependencies,
// `helm dep up` needs to download all the original + adhoc dependencies.
//
// It returns an concatenated list of dependencies, including the original and the adhoc dependencies.
// The list is intended to be used as the requirements for `helm template`.
// At `helm template` run time, we already have all the dependencies under the `charts/` directory thanks to this function.
// But requirements are still needed in particular for `condition` fields, which controls whether to render the dependency chart or not in the end.
func (r *Runner) UpdateRequirements(replace bool, chartYamlPath, tempDir string, deps []Dependency) ([]Dependency, error) {
	// requirements.yaml can exist for both helm v2 or helm v3 chart
	// so we try to load it regardless of the helm version

	var reqs Requirements

	bytes, err := r.ReadFile(filepath.Join(tempDir, "requirements.yaml"))
	if os.IsNotExist(err) {

	} else if err != nil {
		return nil, err
	} else {
		if err := yaml.Unmarshal(bytes, &reqs); err != nil {
			return nil, err
		}
	}

	var all []Dependency

	if r.IsHelm3() || r.IsHelm4() {
		type ChartMeta struct {
			Dependencies []Dependency           `yaml:"dependencies,omitempty"`
			Data         map[string]interface{} `yaml:",inline"`
		}
		var chartMeta ChartMeta

		bytes, err := r.ReadFile(chartYamlPath)
		if os.IsNotExist(err) {

		} else if err != nil {
			return nil, err
		} else {
			if err := yaml.Unmarshal(bytes, &chartMeta); err != nil {
				return nil, err
			}
		}

		all = append(all, chartMeta.Dependencies...)
		all = append(all, reqs.Dependencies...)
		all = append(all, deps...)

		if replace {
			// When it's a remote chart, the helm-fetch preceded this chartification step
			// should have been already downloaded all the dependencies into the charts/ directory.
			//
			// In that case, we need to remove the original Chart.yaml's `dependencies` to
			// avoid failing due to unnecessarily trying to fetch chart dependencies.
			//
			// Note that this depends on how `helm package` used to package the chart served by the chart repo server works.
			// We assume that `helm package` to enforce the package to contains `charts/*.tgz` for every dependency declared in Chart.yaml or requirements.yaml.
			// If the package somehow misses the `charts/*.tgz` files even though it has one ore more dependencies in either Chart.yaml or requirements.yaml,
			// this assumption breaks and chartify might not work well.
			chartMeta.Dependencies = deps
		} else {
			chartMeta.Dependencies = all
		}

		chartYamlContent, err := yaml.Marshal(&chartMeta)
		if err != nil {
			return nil, fmt.Errorf("marshaling-back Chart.yaml: %w", err)
		}

		r.Logf("Removing the dependencies field from the original Chart.yaml.")

		if err := r.WriteFile(filepath.Join(tempDir, "Chart.yaml"), chartYamlContent, 0644); err != nil {
			return nil, err
		}

		// We already merged requirements.yaml into Chart.yaml's dependencies field
		// so we don't need requirements anymore.
		reqYaml := filepath.Join(tempDir, "requirements.yaml")
		if _, err := os.Stat(reqYaml); err == nil {
			r.Logf("Removing requirements.yaml as unneeded. charts/ should have already been populated by helm-fetch.")
			if err := os.Remove(reqYaml); err != nil {
				return nil, err
			}
		}
	} else {
		all = append(all, reqs.Dependencies...)
		all = append(all, deps...)

		if replace {
			reqs.Dependencies = all
		} else {
			reqs.Dependencies = deps
		}

		requirementsYamlContent, err := yaml.Marshal(&reqs)
		if err != nil {
			return nil, fmt.Errorf("marshaling requirements as YAML: %w", err)
		}

		if err := r.WriteFile(filepath.Join(tempDir, "requirements.yaml"), requirementsYamlContent, 0644); err != nil {
			return nil, err
		}

		debugOut, err := r.ReadFile(filepath.Join(tempDir, "requirements.yaml"))
		if err != nil {
			return nil, err
		}
		r.Logf("using requirements.yaml:\n%s", debugOut)
	}

	return all, nil
}
