package chartify

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type KustomizeOpts struct {
	Images     []KustomizeImage `yaml:"images"`
	NamePrefix string           `yaml:"namePrefix"`
	NameSuffix string           `yaml:"nameSuffix"`
	Namespace  string           `yaml:"namespace"`
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
}

func (o *KustomizeBuildOpts) SetKustomizeBuildOption(opts *KustomizeBuildOpts) error {
	*opts = *o
	return nil
}

type KustomizeBuildOption interface {
	SetKustomizeBuildOption(opts *KustomizeBuildOpts) error
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
	baseFile := []byte("bases:\n- " + relPath + "\n")
	kustomizationPath := path.Join(tempDir, "kustomization.yaml")
	if err := r.WriteFile(kustomizationPath, baseFile, 0644); err != nil {
		return "", err
	}

	if len(kustomizeOpts.Images) > 0 {
		args := []string{"edit", "set", "image"}
		for _, image := range kustomizeOpts.Images {
			args = append(args, image.String())
		}
		_, err := r.runInDir(tempDir, r.kustomizeBin(), args...)
		if err != nil {
			return "", err
		}
	}
	if kustomizeOpts.NamePrefix != "" {
		_, err := r.runInDir(tempDir, r.kustomizeBin(), "edit", "set", "nameprefix", kustomizeOpts.NamePrefix)
		if err != nil {
			fmt.Println(err)
			return "", err
		}
	}
	if kustomizeOpts.NameSuffix != "" {
		// "--" is there to avoid `namesuffix -acme` to fail due to `-a` being considered as a flag
		_, err := r.runInDir(tempDir, r.kustomizeBin(), "edit", "set", "namesuffix", "--", kustomizeOpts.NameSuffix)
		if err != nil {
			return "", err
		}
	}
	if kustomizeOpts.Namespace != "" {
		_, err := r.runInDir(tempDir, r.kustomizeBin(), "edit", "set", "namespace", kustomizeOpts.Namespace)
		if err != nil {
			return "", err
		}
	}
	outputFile := filepath.Join(tempDir, "templates", "kustomized.yaml")
	kustomizeArgs := []string{"-o", outputFile, "build"}

	versionInfo, err := r.run(r.kustomizeBin(), "version", "--short")
	if err != nil {
		return "", err
	}
	version := strings.Split(strings.TrimPrefix(versionInfo, "{kustomize/v"), ".")
	major, err := strconv.Atoi(version[0])
	if err != nil {
		r.Logf("Failed to parse `kustomize version` output: %v\nFalling-back to the kustomize v4 mode...", err)
	}

	if major > 3 {
		kustomizeArgs = append(kustomizeArgs, "--load-restrictor=LoadRestrictionsNone")
		if u.EnableAlphaPlugins {
			kustomizeArgs = append(kustomizeArgs, "--enable-alpha-plugins")
		}
	} else {
		kustomizeArgs = append(kustomizeArgs, "--load_restrictor=none")
		if u.EnableAlphaPlugins {
			kustomizeArgs = append(kustomizeArgs, "--enable_alpha_plugins")
		}
	}

	out, err := r.runInDir(tempDir, r.kustomizeBin(), append(kustomizeArgs, tempDir)...)
	if err != nil {
		return "", err
	}
	fmt.Println(out)

	if err := os.RemoveAll(kustomizationPath); err != nil {
		return "", fmt.Errorf("removing unnecessary kustomization.yaml after build: %v", err)
	}

	return outputFile, nil
}
