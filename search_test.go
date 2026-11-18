package chartify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchFileOptsMatches(t *testing.T) {
	manifests := SearchFileOpts{fileType: []string{"yaml", "yml"}}
	templates := SearchFileOpts{fileType: []string{"gotmpl"}}

	cases := []struct {
		name string
		opts SearchFileOpts
		path string
		sep  string
		want bool
	}{
		{"unix manifest", manifests, "/tmp/chartify123/default/release/00_namespaces.yaml", "/", true},
		{"unix template", templates, "/tmp/chartify123/default/release/00_namespaces.yaml.gotmpl", "/", true},
		{"unix other file", manifests, "/tmp/chartify123/default/release/README.md", "/", false},
		// Walk on Windows gives backslashes: these were never matched, so manifest directories
		// produced charts without templates (an empty render).
		{"windows manifest", manifests, `C:\Users\me\AppData\Local\Temp\chartify123\default\release\00_namespaces.yaml`, `\`, true},
		{"windows template", templates, `C:\Users\me\AppData\Local\Temp\chartify123\default\release\00_namespaces.yaml.gotmpl`, `\`, true},
		{"windows other file", manifests, `C:\Users\me\AppData\Local\Temp\chartify123\default\release\README.md`, `\`, false},
		{"windows sub path", SearchFileOpts{matchSubPath: "templates", fileType: []string{"yaml"}}, `C:\tmp\chart\templates\cm.yaml`, `\`, true},
		{"windows sub path missing", SearchFileOpts{matchSubPath: "templates", fileType: []string{"yaml"}}, `C:\tmp\chart\crds\cm.yaml`, `\`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.opts.matches(c.path, c.sep); got != c.want {
				t.Errorf("matches(%q, %q) = %v, want %v", c.path, c.sep, got, c.want)
			}
		})
	}
}

func TestSearchFilesFindsManifests(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.yaml", "b.yml", "c.yaml.gotmpl", "README.md", filepath.Join("sub", "d.yaml")} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x: 1\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	r := New()
	files, err := r.SearchFiles(SearchFileOpts{basePath: dir, fileType: []string{"yaml", "yml"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Errorf("found %v, want a.yaml, b.yml and sub/d.yaml", files)
	}
}
