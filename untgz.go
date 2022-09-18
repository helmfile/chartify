package chartify

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

func ExtractFilesFromChartTGZ(tgzReader io.Reader, dir string) (string, error) {
	if err := ExtractFilesFromTGZ(tgzReader, dir); err != nil {
		return "", fmt.Errorf("unable to extract files to %s: %w", dir, err)
	}

	d, err := os.Open(dir)
	if err != nil {
		return "", fmt.Errorf("unable to open %s as dir: %w", d, err)
	}

	entries, err := d.ReadDir(0)
	if err != nil {
		return "", fmt.Errorf("unable to readdir %s: %w", dir, err)
	}

	if numSubDirs := len(entries); numSubDirs != 1 {
		return "", fmt.Errorf("unexpected number of sub-directories (%d) contained in the chart archive", numSubDirs)
	}

	onlyEntry := entries[0]
	p := filepath.Join(dir, onlyEntry.Name())
	if !onlyEntry.IsDir() {
		return "", fmt.Errorf("the only entry within a chart archive must be a directory, but it was not: %s", p)
	}

	return p, nil
}

func ExtractFilesFromTGZ(tgzReader io.Reader, dir string) error {
	gzReader, err := gzip.NewReader(tgzReader)
	if err != nil {
		return fmt.Errorf("unable to open tgz archive: %w", err)
	}

	tReader := tar.NewReader(gzReader)

	for {
		header, err := tReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("unable to read the next entry in tar: %w", err)
		}

		name := header.Name
		path := filepath.Join(dir, name)

		switch f := header.Typeflag; f {
		case tar.TypeDir:
			if err := os.Mkdir(path, 0755); err != nil {
				return fmt.Errorf("unable to mkdir %q: %w", name, err)
			}
		case tar.TypeReg:
			d := filepath.Dir(path)
			if stat, _ := os.Stat(d); stat == nil || !stat.IsDir() {
				if err := os.MkdirAll(d, 0755); err != nil {
					return fmt.Errorf("unable to mkdir %q: %w", d, err)
				}
			}
			outFile, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("unable to create %q: %w", name, err)
			}
			defer outFile.Close()
			if _, err := io.Copy(outFile, tReader); err != nil {
				return fmt.Errorf("unable to write %q: %w", name, err)
			}
		default:
			log.Fatalf(
				"encoutered unknown tar header %v in %s",
				f,
				name)
		}
	}
	return nil
}
