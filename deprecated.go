package chartify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *Runner) untarUnderDir(path, tempDir string) (string, error) {
	command := fmt.Sprintf("tar -zxvf %s -C %s", path, tempDir)

	if _, err := r.run(command); err != nil {
		return "", err
	}

	if err := os.Remove(path); err != nil {
		return "", err
	}
	files, err := r.ReadDir(tempDir)
	if err != nil {
		return "", err
	}
	if len(files) != 1 {
		fs := []string{}
		for _, f := range files {
			fs = append(fs, f.Name())
		}
		return "", fmt.Errorf("%d additional files found in temp direcotry. This is very strange:\n%s", len(files)-1, strings.Join(fs, "\n"))
	}
	return filepath.Join(tempDir, files[0].Name()), nil
}

