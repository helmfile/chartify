package chartify

import (
	"os"
	"path/filepath"
	"strings"
)

type SearchFileOpts struct {
	basePath     string
	matchSubPath string
	fileType     []string
}

// SearchFiles returns a slice of files that are within the base path, has a matching sub path and file type
func (r *Runner) SearchFiles(o SearchFileOpts) ([]string, error) {
	var files []string

	err := r.Walk(o.basePath, func(path string, info os.FileInfo, err error) error {
		if !o.matches(path, string(filepath.Separator)) {
			return nil
		}
		files = append(files, path)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// matches tells whether path, as Walk gives it with the separator sep of the OS, has the sub path and one of the
// file types. The sub path is matched on forward slashes: on Windows Walk gives backslashes, and without the
// conversion no file would ever match (manifest directories would produce empty charts).
func (o SearchFileOpts) matches(path, sep string) bool {
	if !strings.Contains(strings.ReplaceAll(path, sep, "/"), o.matchSubPath+"/") {
		return false
	}
	for _, t := range o.fileType {
		if strings.HasSuffix(path, t) {
			return true
		}
	}
	return false
}
