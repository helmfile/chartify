package chartify

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
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

	// SortOptions configures kustomize's sortOptions for resource ordering.
	SortOptions *SortOptions

	// ExtraArgs are extra arguments to pass to `kustomize build` command
	// For example, ["--enable-exec"] for plugins like ksops
	ExtraArgs []string
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

	// Resolve the kustomize binary once so PATH lookups are not repeated for every check.
	bin := r.kustomizeBin()
	usingKubectl := bin == "kubectl kustomize"

	r.Logf("patching files: %v", generatedManifestFiles)

	// Detect if CRDs originally came from templates/ directory
	// This is important for preserving the chart author's intent
	// Issue: https://github.com/helmfile/helmfile/issues/2291
	crdsFromTemplates := false
	for _, f := range generatedManifestFiles {
		relPath := strings.Replace(f, tempDir+string(filepath.Separator), "", 1)
		// Normalize path separators for cross-platform compatibility
		relPath = filepath.ToSlash(relPath)
		// Check if any file is in templates/crds/ subdirectory
		if strings.HasPrefix(relPath, "templates/crds/") {
			crdsFromTemplates = true
			r.Logf("Detected CRDs in templates/ directory - will preserve location")
			break
		}
	}

	kustomizationYamlContent := `kind: ""
apiversion: ""
resources:
`
	for _, f := range generatedManifestFiles {
		f = strings.Replace(f, tempDir+string(filepath.Separator), "", 1)
		kustomizationYamlContent += `- ` + f + "\n"
	}

	if len(u.StrategicMergePatches) > 0 || len(u.JsonPatches) > 0 {
		kustomizationYamlContent += `patches:
`
	}

	// handle json patches
	for i, f := range u.JsonPatches {
		fileBytes, err := r.ReadFile(f)
		if err != nil {
			return err
		}

		type jsonPatch struct {
			Target map[string]string `yaml:"target"`
			Patch  []map[string]any  `yaml:"patch"`
			Path   string            `yaml:"path"`
		}
		patch := jsonPatch{}
		if err := yaml.Unmarshal(fileBytes, &patch); err != nil {
			return err
		}

		buf := &bytes.Buffer{}
		encoder := yaml.NewEncoder(buf)
		encoder.SetIndent(2)
		if err := encoder.Encode(map[string]any{"target": patch.Target}); err != nil {
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

	// handle strategic merge patches
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

	if len(u.Transformers) > 0 {
		kustomizationYamlContent += `transformers:
`
		for i, f := range u.Transformers {
			bytes, err := r.ReadFile(f)
			if err != nil {
				return err
			}

			// Resolve any external file references (e.g., "path:" in PatchTransformer) by
			// copying referenced files into tempDir and rewriting paths.
			// See https://github.com/helmfile/chartify/issues/90
			transformerFileDir := filepath.Dir(f)
			bytes, err = r.resolveTransformerFileRefs(bytes, transformerFileDir, tempDir)
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

	if u.SortOptions != nil {
		sortOptsBytes, err := marshalSortOptions(u.SortOptions)
		if err != nil {
			return err
		}
		kustomizationYamlContent += string(sortOptsBytes)
	}

	if err := r.WriteFile(filepath.Join(tempDir, "kustomization.yaml"), []byte(kustomizationYamlContent), 0644); err != nil {
		return err
	}

	r.Logf("generated and using kustomization.yaml:\n%s", kustomizationYamlContent)

	renderedFileName := "all.patched.yaml"
	renderedFile := filepath.Join(tempDir, renderedFileName)
	r.Logf("Generating %s", renderedFileName)

	kustomizeArgs := []string{"--output", renderedFile}

	if !usingKubectl {
		kustomizeArgs = append(kustomizeArgs, "build")
	}

	if u.EnableAlphaPlugins {
		f, err := r.kustomizeEnableAlphaPluginsFlag(usingKubectl)
		if err != nil {
			return err
		}
		kustomizeArgs = append(kustomizeArgs, f)
	}

	// Add any extra arguments provided by the user
	if len(u.ExtraArgs) > 0 {
		kustomizeArgs = append(kustomizeArgs, u.ExtraArgs...)
	}

	// tempDir is the kustomize target, appended last (mirrors KustomizeBuild argument order).
	_, err := r.run(nil, bin, append(kustomizeArgs, tempDir)...)
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

	removedPathList := append(append([]string{}, ContentDirs...), "strategicmergepatches", "transformer-patch-files", "kustomization.yaml", renderedFileName)

	for _, f := range removedPathList {
		d := filepath.Join(tempDir, f)
		r.Logf("Removing %s", d)
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}

	if len(crds) > 0 {
		var crdsDir string

		// Preserve original CRD location to maintain chart author's intent
		// Issue: https://github.com/helmfile/helmfile/issues/2291
		//
		// If CRDs originally came from templates/ directory (e.g., templates/crds/),
		// they were likely placed there intentionally for:
		// - Conditional rendering with {{- if .Values.crds.install }}
		// - Using template features like .Release.Namespace
		// - Requiring CRD updates during helm upgrade
		//
		// Moving them to the special crds/ directory changes Helm's behavior:
		// - templates/crds/: Regular resources, updated on upgrade, deleted on uninstall
		// - crds/: Install-only, immutable on upgrade, not deleted on uninstall
		if crdsFromTemplates {
			// Preserve original location in templates/crds/
			crdsDir = filepath.Join(tempDir, "templates", "crds")
			r.Logf("Preserving CRDs in templates/crds/ (original location)")
		} else if r.IsHelm3() || r.IsHelm4() {
			// Use standard Helm 3/4 crds/ directory for CRDs from root crds/
			crdsDir = filepath.Join(tempDir, "crds")
			r.Logf("Placing CRDs in crds/ (standard Helm 3/4 location)")
		} else {
			// Helm 2 compatibility
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

// resolveTransformerFileRefs scans transformer YAML content for top-level "path" fields
// that reference external files (e.g., PatchTransformer's "path" field). It copies those
// files into tempDir and rewrites the path references to point to the copied locations,
// so that kustomize can access them within its restricted root.
// See https://github.com/helmfile/chartify/issues/90
func (r *Runner) resolveTransformerFileRefs(transformerBytes []byte, transformerFileDir string, tempDir string) ([]byte, error) {
	// Decode all YAML documents from the transformer content.
	// A transformer file can be a single document, a list of documents, or multi-document YAML.
	decoder := yaml.NewDecoder(bytes.NewReader(transformerBytes))

	var docs []interface{}
	for {
		var doc interface{}
		if err := decoder.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			// If we can't parse the YAML, return the original content unchanged and let kustomize handle it.
			return transformerBytes, nil
		}
		docs = append(docs, doc)
	}

	if len(docs) == 0 {
		return transformerBytes, nil
	}

	// Collect all transformer map nodes. This handles both single-document and
	// list-of-documents formats. Maps are reference types in Go, so modifying
	// them below will be reflected when re-encoding docs.
	var allMaps []map[string]interface{}
	for _, doc := range docs {
		switch v := doc.(type) {
		case map[string]interface{}:
			allMaps = append(allMaps, v)
		case []interface{}:
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					allMaps = append(allMaps, m)
				}
			}
		}
	}

	// Look for top-level "path" fields that reference existing files and copy them into tempDir.
	patchFileCounter := 0
	modified := false
	for _, m := range allMaps {
		pathStr, ok := m["path"].(string)
		if !ok || pathStr == "" {
			continue
		}

		// Resolve the referenced file's path. Kustomize resolves paths in transformers
		// relative to the kustomization root (the user's CWD). We also try the transformer
		// file's own directory as a fallback for colocated files.
		resolvedPath, found := r.resolveTransformerPath(pathStr, transformerFileDir)
		if !found {
			continue
		}

		// Skip directories — the "path" field should reference a file.
		// A directory match is likely coincidental (e.g., a "path" field that
		// happens to match a directory name); leave it for kustomize to handle.
		info, err := os.Stat(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("checking file referenced by transformer path %q: %w", pathStr, err)
		}
		if info.IsDir() {
			continue
		}

		fileBytes, err := r.ReadFile(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("reading file referenced by transformer path %q: %w", pathStr, err)
		}

		// Copy the referenced file into tempDir under a known subdirectory.
		// The path in the transformer is rewritten relative to the kustomization root (tempDir),
		// which is how kustomize resolves file references in transformers.
		destRelPath := filepath.Join("transformer-patch-files", fmt.Sprintf("patchfile.%d.yaml", patchFileCounter))
		destAbsPath := filepath.Join(tempDir, destRelPath)
		if err := os.MkdirAll(filepath.Dir(destAbsPath), 0755); err != nil {
			return nil, fmt.Errorf("creating directory for transformer patch file: %w", err)
		}
		if err := r.WriteFile(destAbsPath, fileBytes, 0644); err != nil {
			return nil, fmt.Errorf("writing transformer patch file: %w", err)
		}

		r.Logf("Copied transformer path reference %q to %q", pathStr, destRelPath)

		m["path"] = destRelPath
		modified = true
		patchFileCounter++
	}

	if !modified {
		return transformerBytes, nil
	}

	// Re-encode the YAML with the updated path references.
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	for _, doc := range docs {
		if err := encoder.Encode(doc); err != nil {
			return nil, fmt.Errorf("re-encoding transformer YAML after path resolution: %w", err)
		}
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("closing transformer YAML encoder: %w", err)
	}

	return out.Bytes(), nil
}

// resolveTransformerPath resolves a "path" field from a transformer document to an
// existing file. It tries the CWD first (matching kustomize's behavior where paths
// are relative to the kustomization root), then falls back to the transformer file's
// directory (for colocated files). Returns the resolved path and true if found.
func (r *Runner) resolveTransformerPath(pathStr string, transformerFileDir string) (string, bool) {
	if filepath.IsAbs(pathStr) {
		exists, _ := r.Exists(pathStr)
		return pathStr, exists
	}

	// Try CWD first — this is how kustomize resolves paths in transformers.
	if exists, _ := r.Exists(pathStr); exists {
		return pathStr, true
	}

	// Fall back to the transformer file's directory for colocated files.
	candidate := filepath.Join(transformerFileDir, pathStr)
	if exists, _ := r.Exists(candidate); exists {
		return candidate, true
	}

	return pathStr, false
}
