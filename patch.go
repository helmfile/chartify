package chartify

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
)

type PatchOpts struct {
	JsonPatches []string

	StrategicMergePatches []string
}

func (o *PatchOpts) SetPatchOption(opts *PatchOpts) error {
	*opts = *o
	return nil
}

type PatchOption interface {
	SetPatchOption(*PatchOpts) error
}

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
		f = strings.Replace(f, tempDir+"/", "", 1)
		kustomizationYamlContent += `- ` + f + "\n"
	}

	if len(u.JsonPatches) > 0 {
		kustomizationYamlContent += `patchesJson6902:
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
		kustomizationYamlContent += `patchesStrategicMerge:
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
			kustomizationYamlContent += `- ` + path + "\n"
		}
	}

	if err := r.WriteFile(filepath.Join(tempDir, "kustomization.yaml"), []byte(kustomizationYamlContent), 0644); err != nil {
		return err
	}

	r.Logf("generated and using kustomization.yaml:\n%s", kustomizationYamlContent)

	renderedFile := filepath.Join(tempDir, "helmx.2.patched.yaml")
	r.Logf("generating %s", renderedFile)
	_, err := r.run(r.kustomizeBin(), "build", tempDir, "--output", renderedFile)
	if err != nil {
		return err
	}

	removedPathList := append(append([]string{}, ContentDirs...), "strategicmergepatches", "kustomization.yaml")

	for _, f := range removedPathList {
		d := filepath.Join(tempDir, f)
		r.Logf("removing %s", d)
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}

	templatesDir := filepath.Join(tempDir, "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		return err
	}

	final := filepath.Join(templatesDir, "patched.yaml")
	if err := os.Rename(renderedFile, final); err != nil {
		return err
	}

	return nil
}
