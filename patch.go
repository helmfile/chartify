package chartify

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type PatchOpts struct {
	JsonPatches []string

	StrategicMergePatches []string

	Transformers []string

	// Kustomize alpha plugin enable flag.
	// Above Kustomize v3, it is `--enable-alpha-plugins`.
	// Below Kustomize v3 (including v3), it is `--enable_alpha_plugins`.
	EnableAlphaPlugins bool
}

func (o *PatchOpts) SetPatchOption(opts *PatchOpts) error {
	*opts = *o
	return nil
}

type PatchOption interface {
	SetPatchOption(*PatchOpts) error
}

// nolint
func (r *Runner) Patch(tempDir string, generatedManifestFiles []string, opts ...PatchOption) error {
	u := &PatchOpts{}

	for i := range opts {
		if err := opts[i].SetPatchOption(u); err != nil {
			return err
		}
	}

	r.Logf("patching files: %v", generatedManifestFiles)

	kustomizationYamlContent := `kind: ""
apiversion: ""
resources:
`
	for _, f := range generatedManifestFiles {
		f = strings.Replace(f, tempDir+string(filepath.Separator), "", 1)
		kustomizationYamlContent += `- ` + f + "\n"
	}

	if len(u.JsonPatches) > 0 {
		kustomizationYamlContent += `patches:
`
		for i, f := range u.JsonPatches {
			fileBytes, err := r.ReadFile(f)
			if err != nil {
				return err
			}

			type jsonPatch struct {
				Target map[string]string        `yaml:"target"`
				Patch  []map[string]interface{} `yaml:"patch"`
				Path   string                   `yaml:"path"`
			}
			patch := jsonPatch{}
			if err := yaml.Unmarshal(fileBytes, &patch); err != nil {
				return err
			}

			buf := &bytes.Buffer{}
			encoder := yaml.NewEncoder(buf)
			encoder.SetIndent(2)
			if err := encoder.Encode(map[string]interface{}{"target": patch.Target}); err != nil {
				return err
			}
			targetBytes := buf.Bytes()

			for i, line := range strings.Split(string(targetBytes), "\n") {
				if i == 0 {
					line = "- " + line
				} else {
					line = "  " + line
				}
				kustomizationYamlContent += line + "\n"
			}

			var path string
			if patch.Path != "" {
				path = patch.Path
			} else if len(patch.Patch) > 0 {
				buf := &bytes.Buffer{}
				encoder := yaml.NewEncoder(buf)
				encoder.SetIndent(2)
				err := encoder.Encode(patch.Patch)
				if err != nil {
					return err
				}
				jsonPatchData := buf.Bytes()
				path = filepath.Join("jsonpatches", fmt.Sprintf("patch.%d.yaml", i))
				abspath := filepath.Join(tempDir, path)
				if err := os.MkdirAll(filepath.Dir(abspath), 0755); err != nil {
					return err
				}
				r.Logf("%s:\n%s", path, jsonPatchData)
				if err := r.WriteFile(abspath, jsonPatchData, 0644); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("either \"path\" or \"patch\" must be set in %s", f)
			}
			kustomizationYamlContent += "  path: " + path + "\n"
		}
	}

	if len(u.StrategicMergePatches) > 0 {
		kustomizationYamlContent += `patches:
`
		for i, f := range u.StrategicMergePatches {
			bytes, err := r.ReadFile(f)
			if err != nil {
				return err
			}
			path := filepath.Join("strategicmergepatches", fmt.Sprintf("patch.%d.yaml", i))
			abspath := filepath.Join(tempDir, path)
			if err := os.MkdirAll(filepath.Dir(abspath), 0755); err != nil {
				return err
			}
			if err := r.WriteFile(abspath, bytes, 0644); err != nil {
				return err
			}
			kustomizationYamlContent += `- path: ` + path + "\n"
		}
	}

	if len(u.Transformers) > 0 {
		kustomizationYamlContent += `transformers:
`
		for i, f := range u.Transformers {
			bytes, err := r.ReadFile(f)
			if err != nil {
				return err
			}
			path := filepath.Join("transformers", fmt.Sprintf("transformer.%d.yaml", i))
			abspath := filepath.Join(tempDir, path)
			if err := os.MkdirAll(filepath.Dir(abspath), 0755); err != nil {
				return err
			}
			if err := r.WriteFile(abspath, bytes, 0644); err != nil {
				return err
			}
			kustomizationYamlContent += `- ` + path + "\n"
		}
	}

	if err := r.WriteFile(filepath.Join(tempDir, "kustomization.yaml"), []byte(kustomizationYamlContent), 0644); err != nil {
		return err
	}

	r.Logf("generated and using kustomization.yaml:\n%s", kustomizationYamlContent)

	renderedFileName := "all.patched.yaml"
	renderedFile := filepath.Join(tempDir, renderedFileName)
	r.Logf("Generating %s", renderedFile)

	kustomizeArgs := []string{"build", tempDir, "--output", renderedFile}

	if u.EnableAlphaPlugins {
		f, err := r.kustomizeEnableAlphaPluginsFlag()
		if err != nil {
			return err
		}
		kustomizeArgs = append(kustomizeArgs, f)
	}

	_, err := r.run(r.kustomizeBin(), kustomizeArgs...)
	if err != nil {
		return err
	}

	var resources, crds []string

	bs, err := os.ReadFile(renderedFile)
	if err != nil {
		return fmt.Errorf("reading %s: %w", renderedFileName, err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(bs))

	// Forget about resource consumption and
	// use the file size as the buffer size, so that we will never
	// mis-parse looong YAML due to buffer isn't large enough to
	// contain the YAML document separator...
	buffer := make([]byte, 0, len(bs))
	scanner.Buffer(buffer, len(bs))

	split := func(d []byte, atEOF bool) (int, []byte, error) {
		if atEOF {
			if len(d) == 0 {
				return 0, nil, nil
			}

			return len(d), d, nil
		}

		if i := bytes.Index(d, []byte("\n---\n")); i >= 0 {
			return i + 5, d[0 : i+1], nil
		}

		// "SplitFunc can return (0, nil, nil) to signal the Scanner to read more data into the slice and try again with a longer slice starting at the same point in the input."
		//https://golang.org/pkg/bufio/#SplitFunc
		return 0, nil, nil
	}

	consume := func(t string) error {
		type res struct {
			Kind string `yaml:"kind"`
		}
		var r res

		if err := yaml.Unmarshal([]byte(t), &r); err != nil {
			return fmt.Errorf("processing %s: parsing yaml doc from %q: %w", renderedFileName, t, err)
		}

		if r.Kind == "CustomResourceDefinition" {
			crds = append(crds, t)
		} else {
			resources = append(resources, t)
		}

		return nil
	}

	var scanned bool

	scanner.Split(split)
	for scanner.Scan() {
		scanned = true

		t := scanner.Text()

		if err := consume(t); err != nil {
			return err
		}
	}

	// When the scanner managed to provide all the buffer on first scan, `split` func ends up
	// returning `0, nil, nil` and stops the scanner.
	// In other words, a single resourced chart can never be patched if we didn't handle that case.
	if !scanned {
		if err := consume(string(bs)); err != nil {
			return err
		}
	}

	r.Logf("Detected %d resources and %d CRDs", len(resources), len(crds))

	resourcesFile := filepath.Join(tempDir, "all.patched.resources.yaml")
	crdsFile := filepath.Join(tempDir, "all.patched.crds.yaml")

	err = func() error {
		f, err := os.Create(resourcesFile)
		if err != nil {
			return err
		}
		defer func() {
			_ = f.Close()
		}()

		w := bufio.NewWriter(f)

		for _, resource := range resources {
			_, _ = w.WriteString(resource)
			_, _ = w.WriteString("---\n")
		}

		if err := w.Flush(); err != nil {
			return err
		}

		return f.Sync()
	}()
	if err != nil {
		return fmt.Errorf("writing %s: %w", resourcesFile, err)
	}

	err = func() error {
		f, err := os.Create(crdsFile)
		if err != nil {
			return err
		}
		defer func() {
			_ = f.Close()
		}()

		w := bufio.NewWriter(f)
		for _, crd := range crds {
			_, _ = w.WriteString(crd)
			_, _ = w.WriteString("---\n")
		}

		if err := w.Flush(); err != nil {
			return err
		}

		return f.Sync()
	}()
	if err != nil {
		return fmt.Errorf("writing %s: %w", crdsFile, err)
	}

	removedPathList := append(append([]string{}, ContentDirs...), "strategicmergepatches", "kustomization.yaml", renderedFileName)

	for _, f := range removedPathList {
		d := filepath.Join(tempDir, f)
		r.Logf("Removing %s", d)
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}

	if len(crds) > 0 {
		var crdsDir string

		if r.isHelm3 {
			crdsDir = filepath.Join(tempDir, "crds")
		} else {
			crdsDir = filepath.Join(tempDir, "templates")
		}

		if err := os.MkdirAll(crdsDir, 0755); err != nil {
			return err
		}

		if err := os.Rename(crdsFile, filepath.Join(crdsDir, "patched_crds.yaml")); err != nil {
			return err
		}
	}

	if len(resources) > 0 {
		templatesDir := filepath.Join(tempDir, "templates")
		if err := os.MkdirAll(templatesDir, 0755); err != nil {
			return err
		}

		if err := os.Rename(resourcesFile, filepath.Join(templatesDir, "patched_resources.yaml")); err != nil {
			return err
		}
	}

	return nil
}
