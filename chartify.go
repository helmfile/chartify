package chartify

import (
	"fmt"
	"github.com/otiai10/copy"
	"k8s.io/klog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type ChartifyOpts struct {
	// Debug when set to true passes `--debug` flag to `helm` in order to enable debug logging
	Debug bool

	//ReleaseName string

	// ValuesFiles are a list of Helm chart values files
	ValuesFiles []string

	// SetValues is a list of adhoc Helm chart values being passed via helm's `--set` flags
	SetValues []string

	// Namespace is the default namespace in which the K8s manifests rendered by the chart are associated
	Namespace string

	// ChartVersion is the semver of the Helm chart being used to render the original K8s manifests before various tweaks applied by helm-x
	ChartVersion string

	// TillerNamespace is the namespace Tiller or Helm v3 creates "release" objects(configmaps or secrets depending on the storage backend chosen)
	TillerNamespace string

	// EnableKustomizAlphaPlugins will add the `--enable_alpha_plugins` flag when running `kustomize build`
	EnableKustomizeAlphaPlugins bool

	Injectors []string
	Injects   []string

	AdhocChartDependencies []string

	JsonPatches           []string
	StrategicMergePatches []string

	// WorkaroundOutputDirIssue prevents chartify from using `helm template --output-dir` and let it use `helm template > some.yaml` instead to
	// workaround the potential helm issue
	// See https://github.com/roboll/helmfile/issues/1279#issuecomment-636839395
	WorkaroundOutputDirIssue bool
}

type ChartifyOption interface {
	SetChartifyOption(opts *ChartifyOpts) error
}

type chartifyOptsSetter struct {
	o *ChartifyOpts
}

func (s *chartifyOptsSetter) SetChartifyOption(opts *ChartifyOpts) error {
	*opts = *s.o
	return nil
}

func (s *ChartifyOpts) SetChartifyOption(opts *ChartifyOpts) error {
	*opts = *s
	return nil
}

func WithChartifyOpts(opts *ChartifyOpts) ChartifyOption {
	return &chartifyOptsSetter{
		o: opts,
	}
}

// Chartify creates a temporary Helm chart from a directory or a remote chart, and applies various transformations.
// Returns the full path to the temporary directory containing the generated chart if succeeded.
//
// Parameters:
// * `release` is the name of Helm release being installed
func (r *Runner) Chartify(release, dirOrChart string, opts ...ChartifyOption) (string, error) {
	u := &ChartifyOpts{}

	for i := range opts {
		if err := opts[i].SetChartifyOption(u); err != nil {
			return "", err
		}
	}

	isKustomization, err := r.Exists(filepath.Join(dirOrChart, "kustomization.yaml"))
	if err != nil {
		return "", err
	}

	var tempDir string
	if !isKustomization {
		tempDir, err = r.copyToTempDir(dirOrChart, u.ChartVersion)
		if err != nil {
			return "", err
		}
	} else {
		tempDir = r.MakeTempDir()
	}

	isChart, err := r.Exists(filepath.Join(tempDir, "Chart.yaml"))
	if err != nil {
		return "", err
	}

	generatedManifestFiles := []string{}

	dstTemplatesDir := filepath.Join(tempDir, "templates")
	dirExists, err := r.Exists(dstTemplatesDir)
	if err != nil {
		return "", err
	}
	if !dirExists {
		if err := os.Mkdir(dstTemplatesDir, 0755); err != nil {
			return "", err
		}
	}

	if isKustomization {
		kustomOpts := &KustomizeBuildOpts{
			ValuesFiles:        u.ValuesFiles,
			SetValues:          u.SetValues,
			EnableAlphaPlugins: u.EnableKustomizeAlphaPlugins,
		}
		kustomizeFile, err := r.KustomizeBuild(dirOrChart, tempDir, kustomOpts)
		if err != nil {
			return "", err
		}

		generatedManifestFiles = append(generatedManifestFiles, kustomizeFile)
	}

	if !isChart && !isKustomization {
		manifestFileOptions := SearchFileOpts{
			basePath: tempDir,
			fileType: "yaml",
		}
		manifestFiles, err := r.SearchFiles(manifestFileOptions)
		if err != nil {
			return "", err
		}

		for i, path := range manifestFiles {
			dst := filepath.Join(tempDir, fmt.Sprintf("%d-%s", i, filepath.Base(path)))

			content, err := r.ReadFile(path)
			if err != nil {
				return "", err
			}

			if err := r.WriteFile(dst, content, 0644); err != nil {
				return "", err
			}

			generatedManifestFiles = append(generatedManifestFiles, dst)
		}
	}

	var requirementsYamlContent string
	if !isChart {
		ver := u.ChartVersion
		if u.ChartVersion == "" {
			ver = "1.0.0"
			klog.Infof("using the default chart version 1.0.0 due to that no ChartVersion is specified")
		}
		chartyaml := fmt.Sprintf("name: \"%s\"\nversion: %s\nappVersion: %s\n", release, ver, ver)
		if err := r.WriteFile(filepath.Join(tempDir, "Chart.yaml"), []byte(chartyaml), 0644); err != nil {
			return "", err
		}
	} else {
		bytes, err := r.ReadFile(filepath.Join(tempDir, "requirements.yaml"))
		if os.IsNotExist(err) {
			requirementsYamlContent = `dependencies:`
		} else if err != nil {
			return "", err
		} else {
			parsed := map[string]interface{}{}
			if err := yaml.Unmarshal(bytes, &parsed); err != nil {
				return "", err
			}
			if _, ok := parsed["dependencies"]; !ok {
				bytes = []byte(`dependencies:`)
			}
			requirementsYamlContent = string(bytes)
		}
	}

	for _, d := range u.AdhocChartDependencies {
		aliasChartVer := strings.Split(d, "=")
		chartAndVer := strings.Split(aliasChartVer[len(aliasChartVer)-1], ":")
		repoAndChart := strings.Split(chartAndVer[0], "/")
		repo := repoAndChart[0]
		chart := repoAndChart[1]
		var ver string
		if len(chartAndVer) == 1 {
			ver = "*"
		} else {
			ver = chartAndVer[1]
		}
		var alias string
		if len(aliasChartVer) == 1 {
			alias = chart
		} else {
			alias = aliasChartVer[0]
		}

		var repoUrl string
		out, err := r.run(r.helmBin(), "repo", "list")
		if err != nil {
			return "", err
		}
		lines := strings.Split(out, "\n")
		re := regexp.MustCompile(`\s+`)
		for lineNum, line := range lines {
			if lineNum == 0 {
				continue
			}
			tokens := re.Split(line, -1)
			if len(tokens) < 2 {
				return "", fmt.Errorf("unexpected format of `helm repo list` at line %d \"%s\" in:\n%s", lineNum, line, out)
			}
			if tokens[0] == repo {
				repoUrl = tokens[1]
				break
			}
		}
		if repoUrl == "" {
			return "", fmt.Errorf("no helm list entry found for repository \"%s\". please `helm repo add` it!", repo)
		}

		requirementsYamlContent = requirementsYamlContent + fmt.Sprintf(`
- name: %s
  repository: %s
  condition: %s.enabled
  alias: %s
`, chart, repoUrl, alias, alias)
		requirementsYamlContent = requirementsYamlContent + fmt.Sprintf(`  version: "%s"
`, ver)
	}

	if err := r.WriteFile(filepath.Join(tempDir, "requirements.yaml"), []byte(requirementsYamlContent), 0644); err != nil {
		return "", err
	}

	{
		debugOut, err := r.ReadFile(filepath.Join(tempDir, "requirements.yaml"))
		if err != nil {
			return "", err
		}
		klog.Infof("using requirements.yaml:\n%s", debugOut)
	}

	{
		// Flatten the chart by fetching dependent chart archives and merging their K8s manifests into the temporary local chart
		// So that we can uniformly patch them with JSON patch, Strategic-Merge patch, or with injectors
		_, err := r.run(r.helmBin(), "dependency", "build", tempDir)
		if err != nil {
			return "", err
		}

		matches, err := filepath.Glob(filepath.Join(tempDir, "charts", "*-*.tgz"))
		if err != nil {
			return "", err
		}

		if isChart || len(matches) > 0 {
			templateFileOptions := SearchFileOpts{
				basePath:     tempDir,
				matchSubPath: "templates",
				fileType:     "yaml",
			}
			templateFiles, err := r.SearchFiles(templateFileOptions)
			if err != nil {
				return "", err
			}

			templateOptions := ReplaceWithRenderedOpts{
				Debug:        u.Debug,
				Namespace:    u.Namespace,
				SetValues:    u.SetValues,
				ValuesFiles:  u.ValuesFiles,
				ChartVersion: u.ChartVersion,

				WorkaroundOutputDirIssue: u.WorkaroundOutputDirIssue,
			}
			generated, err := r.ReplaceWithRendered(release, tempDir, templateFiles, templateOptions)
			if err != nil {
				return "", err
			}

			generatedManifestFiles = generated
		}
	}

	// We've already rendered resources from the chart and its subcharts to the helmx.1.rendered directory
	// No need to double-render them by leaving requirements.yaml/lock
	_ = os.Remove(filepath.Join(tempDir, "requirements.yaml"))
	_ = os.Remove(filepath.Join(tempDir, "requirements.lock"))

	{
		dstFilesDir := filepath.Join(tempDir, "files")
		if err := os.MkdirAll(dstFilesDir, 0755); err != nil {
			return "", err
		}

		if isChart && (len(u.JsonPatches) > 0 || len(u.StrategicMergePatches) > 0) {
			patchOpts := &PatchOpts{
				JsonPatches:           u.JsonPatches,
				StrategicMergePatches: u.StrategicMergePatches,
			}
			patchedAndConcatenated, err := r.Patch(tempDir, generatedManifestFiles, patchOpts)
			if err != nil {
				return "", err
			}

			generatedManifestFiles = []string{patchedAndConcatenated}

			final := filepath.Join(dstFilesDir, "helmx.all.yaml")
			klog.Infof("copying %s to %s", patchedAndConcatenated, final)
			if err := r.CopyFile(patchedAndConcatenated, final); err != nil {
				return "", err
			}
		} else {
			dsts := []string{}
			for i, f := range generatedManifestFiles {
				dst := filepath.Join(dstFilesDir, fmt.Sprintf("%d-%s", i, filepath.Base(f)))
				if err := os.Rename(f, dst); err != nil {
					return "", err
				}
				dsts = append(dsts, dst)
			}
			generatedManifestFiles = dsts
		}


		content := []byte(`{{ $files := .Files -}}
{{ range $path, $content :=  .Files.Glob  "files/**.yaml" -}}
---
{{ $files.Get $path }}
{{ end }}
`)

		if err := r.WriteFile(filepath.Join(dstTemplatesDir, "helmx.all.yaml"), content, 0644); err != nil {
			return "", err
		}
	}

	injectOptions := InjectOpts{
		injectors: u.Injectors,
		injects:   u.Injects,
	}
	if err := r.Inject(generatedManifestFiles, injectOptions); err != nil {
		return "", err
	}

	return tempDir, nil
}

// copyToTempDir checks if the path is local or a repo (in this order) and copies it to a temp directory
// It will perform a `helm fetch` if required
func (r *Runner) copyToTempDir(path, chartVersion string) (string, error) {
	tempDir := r.MakeTempDir()
	exists, err := r.Exists(path)
	if err != nil {
		return "", err
	}
	if !exists {
		return r.fetchAndUntarUnderDir(path, tempDir, chartVersion)
	}
	err = copy.Copy(path, tempDir)
	if err != nil {
		return "", err
	}
	return tempDir, nil
}

func (r *Runner) fetchAndUntarUnderDir(chart, tempDir, chartVersion string) (string, error) {
	command := fmt.Sprintf("helm fetch %s --untar -d %s", chart, tempDir)

	if chartVersion != "" {
		command += fmt.Sprintf(" --version %s", chartVersion)
	}

	if _, err := r.run(command); err != nil {
		return "", err
	}

	files, err := r.ReadDir(tempDir)
	if err != nil {
		return "", err
	}

	if len(files) != 1 {
		return "", fmt.Errorf("%d additional files found in temp direcotry. This is very strange", len(files)-1)
	}

	return filepath.Join(tempDir, files[0].Name()), nil
}
