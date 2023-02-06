package chartify

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SetNamespace is a poor-man's `kubectl apply -f DIR --dry-run -o yaml --namespace NAMESPACE`
func (r *Runner) SetNamespace(tempDir, ns string) error {
	for _, d := range ContentDirs {
		a := filepath.Join(tempDir, d)
		if err := filepath.Walk(a, func(path string, info os.FileInfo, err error) error {
			if _, ok := err.(*os.PathError); ok {
				return nil
			}

			if err != nil || info == nil || info.IsDir() {
				return err
			}

			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer func() {
				_ = f.Close()
			}()

			var docs []yaml.Node

			dec := yaml.NewDecoder(f)
			for {
				doc := yaml.Node{}

				if err := dec.Decode(&doc); err != nil {
					if err == io.EOF {
						break
					}
					return fmt.Errorf("parsing yaml from %s: %v", path, err)
				}

				resourceIndex := -1
				metadataIndex := -1
				namespaceIndex := -1

				a := doc.Content[0]
				if a.Kind == yaml.MappingNode {
					resourceIndex = 0
				DOC:
					for j := 0; j < len(a.Content); j += 2 {
						if a.Content[j].Value == "metadata" {
							metadataIndex = j + 1
							metadata := a.Content[metadataIndex]
							for k := 0; k < len(metadata.Content); k += 2 {
								if metadata.Content[k].Value == "namespace" {
									namespaceIndex = k + 1
									break DOC
								}
							}
							break DOC
						}
					}
				}

				if resourceIndex > -1 && metadataIndex > -1 {
					c := doc.Content[resourceIndex].Content[metadataIndex].Content
					if namespaceIndex > -1 {
						// Do not override the namespace when it's already specified,
						// to replicate K8s and Helm behavior.
						//
						//c[namespaceIndex].Value = ns
					} else {
						c = append(c, &yaml.Node{
							Kind:  yaml.ScalarNode,
							Tag:   "!!str",
							Value: "namespace",
						},
							&yaml.Node{
								Kind:  yaml.ScalarNode,
								Tag:   "!!str",
								Value: ns,
							},
						)
					}
					doc.Content[resourceIndex].Content[metadataIndex].Content = c
				} else {
					r.Logf("Skipping %s as it has no resource and metadata. Maybe this is an unconventional chart template file that contains only {{ define}} blocks but not named _helpers.tpl?", f.Name())
				}

				docs = append(docs, doc)
			}

			w, err := os.OpenFile(path, os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("opening file %s: %v", path, err)
			}
			defer func() {
				_ = w.Sync()
			}()
			defer func() {
				_ = w.Close()
			}()

			enc := yaml.NewEncoder(w)
			enc.SetIndent(2)

			for _, doc := range docs {
				if err := enc.Encode(&doc); err != nil {
					return fmt.Errorf("marshaling doc %+v: %v", doc, err)
				}
			}

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}
