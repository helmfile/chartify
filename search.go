package chartify

import (
	"os"
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
		if !strings.Contains(path, o.matchSubPath+"/") {
			return nil
		}
		var any bool
		for _, t := range o.fileType {
			any = strings.HasSuffix(path, t)
			if any {
				break
			}
		}
		if !any {
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
