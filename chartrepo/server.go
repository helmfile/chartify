// Copyright The Helm Authors
// https://github.com/helm/chart-releaser/blob/main/pkg/releaser/releaser.go
// Part of this source has been derived from the upstream source linked above.

package chartrepo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/provenance"
	"helm.sh/helm/v3/pkg/repo"
)

type Server struct {
	Port int
	Host string
}

func (s *Server) getPort() int {
	port := s.Port
	if port == 0 {
		port = 18080
	}
	return port
}

func (s *Server) getHostport() string {
	port := s.getPort()
	hostport := s.Host
	if hostport == "" {
		hostport = fmt.Sprintf("localhost:%d", port)
	}
	return hostport
}

func (s *Server) ServerURL() string {
	hostport := s.getHostport()
	serverURL := fmt.Sprintf("http://%s/", hostport)
	return serverURL
}

func (s *Server) Run(ctx context.Context, chartsDir string) error {
	port := s.getPort()
	serverURL := s.ServerURL()

	worktree, err := os.MkdirTemp(os.TempDir(), "chartrepo")
	if err != nil {
		return err
	}

	defer func() {
		if err := os.RemoveAll(worktree); err != nil {
			log.Printf("unable to remove worktree %s: %v", worktree, err)
		}
	}()

	dirEntries, err := os.ReadDir(chartsDir)
	if err != nil {
		return err
	}

	for _, ent := range dirEntries {
		if !ent.IsDir() {
			continue
		}

		chart := filepath.Join(chartsDir, ent.Name())

		log.Println("Packaging", chart)

		abs, err := filepath.Abs(chart)
		if err != nil {
			return fmt.Errorf("unable to get abs path to %s: %w", chart, err)
		}

		cmd := exec.CommandContext(ctx, "helm", "package", abs)
		cmd.Dir = worktree
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Println(string(out))
			return fmt.Errorf("unable to package %s: %w", chart, err)
		}
	}

	indexYamlPath := filepath.Join(worktree, "index.yaml")

	var indexFile *repo.IndexFile
	_, err = os.Stat(indexYamlPath)
	if err == nil {
		indexFile, err = repo.LoadIndexFile(indexYamlPath)
		if err != nil {
			return err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		indexFile = repo.NewIndexFile()
	} else {
		return err
	}

	chartPackages, err := filepath.Glob(filepath.Join(worktree, "*.tgz"))
	if err != nil {
		return err
	}

	var update bool
	for _, chartPackage := range chartPackages {
		_, err := loader.LoadFile(chartPackage)
		if err != nil {
			return err
		}
		pkgURL := fmt.Sprintf("%s%s", serverURL, filepath.Base(chartPackage))

		downloadUrl, _ := url.Parse(pkgURL)
		name := filepath.Base(downloadUrl.Path)
		baseName := strings.TrimSuffix(name, filepath.Ext(name))
		tagParts := splitPackageNameAndVersion(baseName)
		packageName, packageVersion := tagParts[0], tagParts[1]
		fmt.Printf("Found %s-%s.tgz\n", packageName, packageVersion)
		if _, err := indexFile.Get(packageName, packageVersion); err != nil {
			if err := addToIndexFile(worktree, indexFile, downloadUrl.String()); err != nil {
				return err
			}
			update = true
		}
	}

	if !update {
		fmt.Printf("Index %s did not change\n", indexYamlPath)
		return nil
	}

	fmt.Printf("Updating index %s\n", indexYamlPath)
	indexFile.SortEntries()

	indexFile.Generated = time.Now()

	if err := indexFile.WriteFile(indexYamlPath, 0644); err != nil {
		return err
	}

	var serveMux http.ServeMux

	serveMux.HandleFunc("/index.yaml", func(w http.ResponseWriter, _ *http.Request) {
		f, err := os.Open(indexYamlPath)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}

		if _, err := io.Copy(w, f); err != nil {
			w.Write([]byte(err.Error()))
			return
		}
	})

	serveMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".tgz") {
			w.WriteHeader(404)
			return
		}

		base := filepath.Base(r.URL.Path)

		pkgPath := filepath.Join(worktree, base)

		f, err := os.Open(pkgPath)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}

		if _, err := io.Copy(w, f); err != nil {
			w.Write([]byte(err.Error()))
			return
		}
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: &serveMux,
	}

	go func() {
		<-ctx.Done()

		_ = server.Close()
	}()

	if err := server.ListenAndServe(); err != nil {
		return fmt.Errorf("starting server: %w", err)
	}

	return nil
}

func splitPackageNameAndVersion(pkg string) []string {
	delimIndex := strings.LastIndex(pkg, "-")
	return []string{pkg[0:delimIndex], pkg[delimIndex+1:]}
}

func addToIndexFile(worktree string, indexFile *repo.IndexFile, url string) error {
	arch := filepath.Join(worktree, filepath.Base(url))

	// extract chart metadata
	fmt.Printf("Extracting chart metadata from %s\n", arch)
	c, err := loader.LoadFile(arch)
	if err != nil {
		return fmt.Errorf("%s is not a helm chart package: %w", arch, err)
	}
	// calculate hash
	fmt.Printf("Calculating Hash for %s\n", arch)
	hash, err := provenance.DigestFile(arch)
	if err != nil {
		return err
	}

	// remove url name from url as helm's index library
	// adds it in during .Add
	// there should be a better way to handle this :(
	s := strings.Split(url, "/")
	s = s[:len(s)-1]

	// Add to index
	if err := indexFile.MustAdd(c.Metadata, filepath.Base(arch), strings.Join(s, "/"), hash); err != nil {
		return err
	}
	return nil
}
