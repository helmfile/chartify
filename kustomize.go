package chartify

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

type KustomizeOpts struct {
	Images     []KustomizeImage `yaml:"images"`
	NamePrefix string           `yaml:"namePrefix"`
	NameSuffix string           `yaml:"nameSuffix"`
	Namespace  string           `yaml:"namespace"`
}

// KustomizationFile represents the structure of a kustomization.yaml file
type KustomizationFile struct {
	Resources  []string         `yaml:"resources,omitempty"`
	Bases      []string         `yaml:"bases,omitempty"`
	Images     []KustomizeImage `yaml:"images,omitempty"`
	NamePrefix string           `yaml:"namePrefix,omitempty"`
	NameSuffix string           `yaml:"nameSuffix,omitempty"`
	Namespace  string           `yaml:"namespace,omitempty"`
}

type KustomizeImage struct {
	Name    string `yaml:"name"`
	NewName string `yaml:"newName"`
	NewTag  string `yaml:"newTag"`
	Digest  string `yaml:"digest"`
}

func (img KustomizeImage) String() string {
	res := img.Name
	if img.NewName != "" {
		res = res + "=" + img.NewName
	}
	if img.NewTag != "" {
		res = res + ":" + img.NewTag
	}
	if img.Digest != "" {
		res = res + "@" + img.Digest
	}
	return res
}

type KustomizeBuildOpts struct {
	ValuesFiles        []string
	SetValues          []string
	SetFlags           []string
	EnableAlphaPlugins bool
	Namespace          string
	HelmBinary         string
}

func (o *KustomizeBuildOpts) SetKustomizeBuildOption(opts *KustomizeBuildOpts) error {
	*opts = *o
	return nil
}

type KustomizeBuildOption interface {
	SetKustomizeBuildOption(opts *KustomizeBuildOpts) error
}

// generateKustomizationFile creates a complete kustomization.yaml content
func (r *Runner) generateKustomizationFile(relPath string, opts KustomizeOpts) ([]byte, error) {
	kustomization := KustomizationFile{
		Resources: []string{relPath}, // Use resources instead of deprecated bases
	}

	if len(opts.Images) > 0 {
		kustomization.Images = opts.Images
	}
	if opts.NamePrefix != "" {
		kustomization.NamePrefix = opts.NamePrefix
	}
	if opts.NameSuffix != "" {
		kustomization.NameSuffix = opts.NameSuffix
	}
	if opts.Namespace != "" {
		kustomization.Namespace = opts.Namespace
	}

	return yaml.Marshal(&kustomization)
}

func (r *Runner) KustomizeBuild(srcDir string, tempDir string, opts ...KustomizeBuildOption) (string, error) {
	kustomizeOpts := KustomizeOpts{}
	u := &KustomizeBuildOpts{}

	for i := range opts {
		if err := opts[i].SetKustomizeBuildOption(u); err != nil {
			return "", err
		}
	}

	for _, f := range u.ValuesFiles {
		valsFileContent, err := r.ReadFile(f)
		if err != nil {
			return "", err
		}
		if err := yaml.Unmarshal(valsFileContent, &kustomizeOpts); err != nil {
			return "", err
		}
	}

	if u.Namespace != "" {
		kustomizeOpts.Namespace = u.Namespace
	}

	if len(u.SetValues) > 0 || len(u.SetFlags) > 0 {
		panic("--set is not yet supported for kustomize-based apps! Use -f/--values flag instead.")
	}

	prevDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	evaluatedPath, err := filepath.EvalSymlinks(tempDir)
	if err != nil {
		return "", err
	}
	var absoluteSrcPath string
	if filepath.IsAbs(srcDir) {
		absoluteSrcPath = srcDir
	} else {
		absoluteSrcPath = path.Join(prevDir, srcDir)
	}
	relPath, err := filepath.Rel(evaluatedPath, absoluteSrcPath)
	if err != nil {
		return "", err
	}
	
	// Generate complete kustomization.yaml file directly instead of using edit commands
	kustomizationContent, err := r.generateKustomizationFile(relPath, kustomizeOpts)
	if err != nil {
		return "", fmt.Errorf("generating kustomization.yaml: %v", err)
	}
	
	kustomizationPath := path.Join(tempDir, "kustomization.yaml")
	if err := r.WriteFile(kustomizationPath, kustomizationContent, 0644); err != nil {
		return "", err
	}

	outputFile := filepath.Join(tempDir, "templates", "kustomized.yaml")
	kustomizeArgs := []string{"-o", outputFile, "build"}

	if u.EnableAlphaPlugins {
		f, err := r.kustomizeEnableAlphaPluginsFlag()
		if err != nil {
			return "", err
		}
		kustomizeArgs = append(kustomizeArgs, f)
	}
	f, err := r.kustomizeLoadRestrictionsNoneFlag()
	if err != nil {
		return "", err
	}
	kustomizeArgs = append(kustomizeArgs, f, "--enable-helm")

	if u.HelmBinary != "" {
		kustomizeArgs = append(kustomizeArgs, "--helm-command="+u.HelmBinary)
	}

	// Use kubectl kustomize fallback if standalone kustomize is not available
	buildCmd, buildArgs, err := r.kustomizeBuildCommand(kustomizeArgs, tempDir)
	if err != nil {
		return "", err
	}

	out, err := r.runInDir(tempDir, buildCmd, buildArgs...)
	if err != nil {
		return "", err
	}
	fmt.Println(out)

	if err := os.RemoveAll(kustomizationPath); err != nil {
		return "", fmt.Errorf("removing unnecessary kustomization.yaml after build: %v", err)
	}

	return outputFile, nil
}

// kustomizeVersion returns the kustomize binary version.
// Returns nil if kustomize binary is not available (fallback scenario).
func (r *Runner) kustomizeVersion() (*semver.Version, error) {
	// Skip version detection if using a fallback scenario
	if !r.isKustomizeBinaryAvailable() {
		return nil, nil
	}

	versionInfo, err := r.run(nil, r.kustomizeBin(), "version")
	if err != nil {
		return nil, err
	}

	vi, err := FindSemVerInfo(versionInfo)
	if err != nil {
		return nil, err
	}
	version, err := semver.NewVersion(vi)
	if err != nil {
		return nil, err
	}
	return version, nil
}

// kustomizeEnableAlphaPluginsFlag returns the kustomize binary alpha plugin argument.
// Above Kustomize v3, it is `--enable-alpha-plugins`.
// Below Kustomize v3 (including v3), it is `--enable_alpha_plugins`.
// Uses modern flag format when kustomize binary is not available (kubectl fallback).
func (r *Runner) kustomizeEnableAlphaPluginsFlag() (string, error) {
	version, err := r.kustomizeVersion()
	if err != nil {
		return "", err
	}
	// If version is nil (fallback scenario), use modern flag format
	if version == nil || version.Major() > 3 {
		return "--enable-alpha-plugins", nil
	}
	return "--enable_alpha_plugins", nil
}

// kustomizeLoadRestrictionsNoneFlag returns the kustomize loading files from outside
// the root argument.
// Above Kustomize v3, it is `--load-restrictor=LoadRestrictionsNone`.
// Below Kustomize v3 (including v3), it is `--load_restrictor=none`.
// Uses modern flag format when kustomize binary is not available (kubectl fallback).
func (r *Runner) kustomizeLoadRestrictionsNoneFlag() (string, error) {
	version, err := r.kustomizeVersion()
	if err != nil {
		return "", err
	}
	// If version is nil (fallback scenario), use modern flag format
	if version == nil || version.Major() > 3 {
		return "--load-restrictor=LoadRestrictionsNone", nil
	}
	return "--load_restrictor=none", nil
}
