package chartify

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/otiai10/copy"

	"gopkg.in/yaml.v3"
)

var (
	ContentDirs = []string{"templates", "charts", "crds"}
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

	AdhocChartDependencies           []ChartDependency
	DeprecatedAdhocChartDependencies []string

	JsonPatches           []string
	StrategicMergePatches []string

	// Transformers is the list of YAML files each defines a Kustomize transformer
	// See https://github.com/kubernetes-sigs/kustomize/blob/master/examples/configureBuiltinPlugin.md#configuring-the-builtin-plugins-instead for more information.
	Transformers []string

	// WorkaroundOutputDirIssue prevents chartify from using `helm template --output-dir` and let it use `helm template > some.yaml` instead to
	// workaround the potential helm issue
	// See https://github.com/roboll/helmfile/issues/1279#issuecomment-636839395
	WorkaroundOutputDirIssue bool

	// OverrideNamespace modifies namespace of every resource after rendering and patching,
	// as a workaround to fix a broken chart.
	// For kustomization, `Namespace` should just work and this won't be needed.
	// For helm chart, as long as the chart has "correct" resource templates with `namespace: {{ .Namespace }}`s this isn't needed.
	OverrideNamespace string

	// SkipDeps skips running `helm dep up` on the chart.
	// Useful for cases when the chart has a broken dependencies definition like seen in
	// https://github.com/roboll/helmfile/issues/1547
	SkipDeps bool

	// IncludeCRDs is a Helm 3 only option. When it is true, chartify passes a `--include-crds` flag
	// to helm-template.
	IncludeCRDs bool

	// TemplateFuncs is the FuncMap used while rendering .gotmpl files in the target directory
	TemplateFuncs template.FuncMap
	// TemplateData is the data available via {{ . }} within .gotmpl files
	TemplateData interface{}
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

	isLocal, _ := r.Exists(dirOrChart)

	isKustomization, err := r.Exists(filepath.Join(dirOrChart, "kustomization.yaml"))
	if err != nil {
		return "", err
	}

	var tempDir string
	if !isKustomization {
		tempDir = r.MakeTempDir(release, dirOrChart, u)

		tempDir, err = r.copyToTempDir(dirOrChart, tempDir, u.ChartVersion)
		if err != nil {
			return "", err
		}
	} else {
		tempDir = r.MakeTempDir(release, dirOrChart, u)
	}

	chartYamlPath := filepath.Join(tempDir, "Chart.yaml")

	isChart, err := r.Exists(chartYamlPath)
	if err != nil {
		return "", err
	}

	templatesDir := filepath.Join(tempDir, "templates")
	dirExists, err := r.Exists(templatesDir)
	if err != nil {
		return "", err
	}
	if !dirExists {
		if err := os.Mkdir(templatesDir, 0755); err != nil {
			return "", err
		}
	}

	overrideNamespace := u.OverrideNamespace

	if !isChart && len(u.TemplateFuncs) > 0 {
		templateFiles, err := r.SearchFiles(SearchFileOpts{
			basePath: tempDir,
			fileType: "gotmpl",
		})
		if err != nil {
			return "", err
		}

		for _, absPath := range templateFiles {
			tmpl := template.New(filepath.Base(absPath))
			body, err := r.ReadFile(absPath)
			if err != nil {
				return "", err
			}

			tmpl, err = tmpl.Funcs(u.TemplateFuncs).Parse(string(body))
			if err != nil {
				return "", err
			}

			var buf bytes.Buffer

			if err := tmpl.Execute(&buf, u.TemplateData); err != nil {
				return "", err
			}

			if err := r.WriteFile(strings.TrimSuffix(absPath, filepath.Ext(absPath)), buf.Bytes(), 0644); err != nil {
				return "", err
			}
		}
	}

	generatedManifestsUnderTemplatesDir := []string{}

	if isKustomization {
		kustomOpts := &KustomizeBuildOpts{
			ValuesFiles:        u.ValuesFiles,
			SetValues:          u.SetValues,
			EnableAlphaPlugins: u.EnableKustomizeAlphaPlugins,
			Namespace:          u.Namespace,
		}
		kustomizeFile, err := r.KustomizeBuild(dirOrChart, tempDir, kustomOpts)
		if err != nil {
			return "", err
		}

		generatedManifestsUnderTemplatesDir = append(generatedManifestsUnderTemplatesDir, kustomizeFile)
	} else if !isChart {
		manifestFileOptions := SearchFileOpts{
			basePath: tempDir,
			fileType: "yaml",
		}
		manifestFiles, err := r.SearchFiles(manifestFileOptions)
		if err != nil {
			return "", err
		}

		var usedDirs []string

		for _, absPath := range manifestFiles {
			relPath, err := filepath.Rel(tempDir, absPath)
			if err != nil {
				return "", err
			}

			dst := filepath.Join(templatesDir, relPath)

			dstDir := filepath.Dir(dst)
			if _, err := os.Lstat(dstDir); err != nil && os.IsNotExist(err) {
				if err := os.MkdirAll(dstDir, 0755); err != nil {
					return "", err
				}

				usedDirs = append(usedDirs, filepath.Dir(absPath))
			}

			if err := os.Rename(absPath, dst); err != nil {
				return "", err
			}

			generatedManifestsUnderTemplatesDir = append(generatedManifestsUnderTemplatesDir, dst)
		}

		for _, d := range usedDirs {
			if err := os.RemoveAll(d); err != nil {
				return "", err
			}
		}

		// Do set namespace if and only if the manifest has no `metadata.namespace` set
		if overrideNamespace == "" && u.Namespace != "" {
			overrideNamespace = u.Namespace
		}
	}

	chartName := filepath.Base(dirOrChart)
	if !isChart {
		ver := u.ChartVersion
		if u.ChartVersion == "" {
			ver = "1.0.0"
			r.Logf("using the default chart version 1.0.0 due to that no ChartVersion is specified")
		}
		chartConfigTemplate := "name: \"%s\"\nversion: %s\nappVersion: %s\napiVersion: v2\n"
		chartyaml := fmt.Sprintf(chartConfigTemplate, chartName, ver, ver)

		r.Logf("Writing %s", chartYamlPath)

		if err := r.WriteFile(chartYamlPath, []byte(chartyaml), 0644); err != nil {
			return "", err
		}

		filesDir, err := r.EnsureFilesDir(tempDir)
		if err != nil {
			return "", err
		}

		if err := r.RewriteChartToPreventDoubleRendering(tempDir, filesDir); err != nil {
			return "", err
		}
	}

	// We need to remove the original Chart.yaml's `dependencies` field to
	// avoid failing due to unnecesarily trying to fetch chart dependencies.
	// Chart dependencies should already be rendered, patched, and included in the temporary chart
	// we've generated so far. So we don't need to tell Helm to fetch chart dependencies, as they are already included.
	if isChart {
		type ChartMeta struct {
			Dependencies []map[string]interface{} `yaml:"dependencies,omitempty"`
			Data         map[string]interface{}   `yaml:",inline"`
		}
		var chartMeta ChartMeta

		bytes, err := r.ReadFile(filepath.Join(tempDir, "Chart.yaml"))
		if os.IsNotExist(err) {

		} else if err != nil {
			return "", err
		} else {
			if err := yaml.Unmarshal(bytes, &chartMeta); err != nil {
				return "", err
			}
		}

		if len(chartMeta.Dependencies) > 0 {
			chartMeta.Dependencies = nil

			chartYamlContent, err := yaml.Marshal(&chartMeta)
			if err != nil {
				return "", fmt.Errorf("marshalling-back %s's Chart.yaml: %w", release, err)
			}

			r.Logf("Removing the dependencies field from the original Chart.yaml.")

			if err := r.WriteFile(filepath.Join(tempDir, "Chart.yaml"), chartYamlContent, 0644); err != nil {
				return "", err
			}
		}
	}

	var reqs Requirements

	bytes, err := r.ReadFile(filepath.Join(tempDir, "requirements.yaml"))
	if os.IsNotExist(err) {

	} else if err != nil {
		return "", err
	} else {
		if err := yaml.Unmarshal(bytes, &reqs); err != nil {
			return "", err
		}
	}

	var adhocChartDependencies []ChartDependency

	for _, d := range u.DeprecatedAdhocChartDependencies {
		aliasChartVer := strings.Split(d, "=")
		chartAndVer := strings.Split(aliasChartVer[len(aliasChartVer)-1], ":")
		var ver string
		if len(chartAndVer) == 1 {
			ver = "*"
		} else {
			ver = chartAndVer[1]
		}
		var alias string
		if len(aliasChartVer) > 1 {
			alias = aliasChartVer[0]
		}

		adhocChartDependencies = append(adhocChartDependencies, ChartDependency{
			Alias:   alias,
			Chart:   chartAndVer[0],
			Version: ver,
		})
	}

	for _, d := range u.AdhocChartDependencies {
		adhocChartDependencies = append(adhocChartDependencies, d)
	}

	for _, d := range adhocChartDependencies {
		isLocalChart, _ := r.Exists(d.Chart)

		var name, repoUrl string

		if isLocalChart {
			name = filepath.Base(d.Chart)
			repoUrl = fmt.Sprintf("file://%s", d.Chart)
		} else {
			repoAndChart := strings.Split(d.Chart, "/")
			repo := repoAndChart[0]
			name = repoAndChart[1]

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
		}

		condName := d.Alias
		if condName == "" {
			condName = name
		}

		reqs.Dependencies = append(reqs.Dependencies, Dependency{
			Name:       name,
			Repository: repoUrl,
			Condition:  fmt.Sprintf("%s.enabled", condName),
			Alias:      d.Alias,
			Version:    d.Version,
		})
	}

	requirementsYamlContent, err := yaml.Marshal(&reqs)
	if err != nil {
		return "", fmt.Errorf("marshalling %s's requirements as YAML: %w", release, err)
	}

	if err := r.WriteFile(filepath.Join(tempDir, "requirements.yaml"), requirementsYamlContent, 0644); err != nil {
		return "", err
	}

	{
		debugOut, err := r.ReadFile(filepath.Join(tempDir, "requirements.yaml"))
		if err != nil {
			return "", err
		}
		r.Logf("using requirements.yaml:\n%s", debugOut)
	}

	var generatedManifestFiles []string

	if isLocal {
		// Note on `len(u.AdhocChartDependencies) == 0`:
		// This special handling is required because adding adhoc chart dependencies
		// means that you MUST run `helm dep up` and `helm dep build` to download the dependencies into the ./charts directory.
		// Otherwise you end up getting:
		//   Error: found in Chart.yaml, but missing in charts/ directory: $DEP_CHART_1, $DEP_CHART_2, ...`
		// ...which effectively making this useless when used in e.g. helmfile
		if u.SkipDeps && len(u.AdhocChartDependencies) == 0 {
			r.Logf("Skipping `helm dependency up` on release %s's chart due to that you've set SkipDeps=true.\n"+
				"This may result in outdated chart dependencies.", release)
		} else {
			// Flatten the chart by fetching dependent chart archives and merging their K8s manifests into the temporary local chart
			// So that we can uniformly patch them with JSON patch, Strategic-Merge patch, or with injectors
			_, err := r.run(r.helmBin(), "dependency", "up", tempDir)
			if err != nil {
				return "", err
			}
		}
	}

	templateOptions := ReplaceWithRenderedOpts{
		Debug:        u.Debug,
		Namespace:    u.Namespace,
		SetValues:    u.SetValues,
		ValuesFiles:  u.ValuesFiles,
		ChartVersion: u.ChartVersion,
		IncludeCRDs:  u.IncludeCRDs,

		WorkaroundOutputDirIssue: u.WorkaroundOutputDirIssue,
	}

	generated, err := r.ReplaceWithRendered(release, chartName, tempDir, templateOptions)
	if err != nil {
		return "", err
	}

	generatedManifestFiles = generated

	// We've already rendered resources from the chart and its subcharts to the helmx.1.rendered directory
	// No need to double-render them by leaving requirements.yaml/lock and downloaded sub-charts
	_ = os.Remove(filepath.Join(tempDir, "requirements.yaml"))
	_ = os.Remove(filepath.Join(tempDir, "requirements.lock"))

	var chartNames []string

	chartNames = append(chartNames, filepath.Base(dirOrChart))
	for _, r := range reqs.Dependencies {
		chartNames = append(chartNames, r.Alias)
	}

	if overrideNamespace != "" {
		if err := r.SetNamespace(tempDir, overrideNamespace); err != nil {
			return "", err
		}
	}

	if len(u.JsonPatches) > 0 || len(u.StrategicMergePatches) > 0 || len(u.Transformers) > 0 {
		patchOpts := &PatchOpts{
			JsonPatches:           u.JsonPatches,
			StrategicMergePatches: u.StrategicMergePatches,
			Transformers:          u.Transformers,
		}
		if err := r.Patch(tempDir, generatedManifestFiles, patchOpts); err != nil {
			return "", err
		}
	}

	//
	// Apply injectors to all the files rendered under `templates` and `crds`
	//

	injectOptions := InjectOpts{
		injectors: u.Injectors,
		injects:   u.Injects,
	}
	if err := r.Inject(generatedManifestFiles, injectOptions); err != nil {
		return "", err
	}

	//
	// Move all the resulting files under `templates` and `crds` to `files/templates` and `files/crds` and
	// create replacement template files in their original locations to avoid double rendering.
	//

	filesDir, err := r.EnsureFilesDir(tempDir)
	if err != nil {
		return "", err
	}

	if err := r.RewriteChartToPreventDoubleRendering(tempDir, filesDir); err != nil {
		return "", err
	}

	return tempDir, nil
}

func (r *Runner) EnsureFilesDir(tempDir string) (string, error) {
	// Files are written to somewhere else than "templates/` to avoid double-rendering
	// which will break go templates embedded in YAML(e.g. PrometheusRule)
	filesDir := filepath.Join(tempDir, "files")

	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return "", err
	}

	return filesDir, nil
}

// RewriteChartToPreventDoubleRendering rewrites templates/*.yaml files with
// template files containing:
//   {{ .Files.Get "path/to/the/yaml/file" }}
// So that re-running helm-template on chartify's final output doesn't result in double-rendering.
// Double-rendering accidentally renders e.g. go template expressions embedded in prometheus rules manifests,
// which is not what the user wants.
func (r *Runner) RewriteChartToPreventDoubleRendering(tempDir, filesDir string) error {
	for _, d := range ContentDirs {
		if d == "crds" {
			// Do not rewrite crds/*.yaml, as `helm template --includec-crds` seem to
			// render CRD yaml files as-is, without processing go template.
			// Also see https://github.com/helm/helm/pull/7138/files
			continue
		}

		srcDir := filepath.Join(tempDir, d)
		dstDir := filepath.Join(filesDir, d)

		if _, err := os.Lstat(srcDir); err == nil {
			if err := os.Rename(srcDir, dstDir); err != nil {
				return err
			}
		} else {
			continue
		}

		if err := filepath.Walk(dstDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			rel, err := filepath.Rel(filesDir, path)
			if err != nil {
				return fmt.Errorf("calculating relative path to %s from %s: %v", path, filesDir, err)
			}

			content := []byte(fmt.Sprintf(`{{ .Files.Get "files/%s" }}`, rel))

			f := filepath.Join(tempDir, rel)

			if err := createDirForFile(f); err != nil {
				return err
			}

			if err := r.WriteFile(f, content, 0644); err != nil {
				return err
			}

			return nil
		}); err != nil {
			return err
		}

		// Without this, any sub-sequent helm command on the generated local chart result in
		// an error due to missing Chart.yaml for every `charts/SUBCHART`
		if d == "charts" {
			chartsDir := filepath.Join(tempDir, "charts")
			templatesDir := filepath.Join(tempDir, "templates")
			templateChartsDir := filepath.Join(templatesDir, "charts")

			// Otherwise the below Rename fail due to missing destination `templates` directory when
			// the original chart had no `templates` directory. Yes, that's a valid chart.
			if err := os.MkdirAll(templatesDir, 0755); err != nil {
				return err
			}

			if err := os.Rename(chartsDir, templateChartsDir); err != nil {
				return err
			}
		}
	}

	return nil
}

func createDirForFile(f string) error {
	dstFileDir := filepath.Dir(f)
	if _, err := os.Lstat(dstFileDir); err == nil {

	} else if err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(dstFileDir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %v", dstFileDir, err)
		}
	} else {
		return fmt.Errorf("checking directory %s: %v", dstFileDir, err)
	}

	return nil
}

// copyToTempDir checks if the path is local or a repo (in this order) and copies it to a temp directory
// It will perform a `helm fetch` if required
func (r *Runner) copyToTempDir(path, tempDir, chartVersion string) (string, error) {
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
