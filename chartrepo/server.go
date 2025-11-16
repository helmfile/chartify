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

	"github.com/Masterminds/semver/v3"
	loaderv3 "helm.sh/helm/v3/pkg/chart/loader"
	provenancev3 "helm.sh/helm/v3/pkg/provenance"
	repov3 "helm.sh/helm/v3/pkg/repo"
	loaderv4 "helm.sh/helm/v4/pkg/chart/loader"
	v2 "helm.sh/helm/v4/pkg/chart/v2"
	provenancev4 "helm.sh/helm/v4/pkg/provenance"
	repov4 "helm.sh/helm/v4/pkg/repo/v1"
)

type Server struct {
	Port      int
	Host      string
	ChartsDir string
	HelmBin   string
	isHelm3   *bool
	isHelm4   *bool
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

func (s *Server) getHelmBin() string {
	if s.HelmBin == "" {
		return "helm"
	}
	return s.HelmBin
}

func (s *Server) detectHelmVersion() (*semver.Version, error) {
	helmBin := s.getHelmBin()
	cmd := exec.Command(helmBin, "version", "--template={{.Version}}+g{{.GitCommit}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("detecting helm version: %w: %s", err, string(out))
	}

	verStr := strings.TrimSpace(string(out))
	sv, err := semver.NewVersion(verStr)
	if err != nil {
		return nil, fmt.Errorf("parsing helm version %q: %w", verStr, err)
	}

	return sv, nil
}

func (s *Server) IsHelm3() bool {
	if s.isHelm3 != nil {
		return *s.isHelm3
	}

	if os.Getenv("HELM_X_HELM3") != "" {
		v := true
		s.isHelm3 = &v
		return true
	}

	sv, err := s.detectHelmVersion()
	if err != nil {
		panic(err)
	}

	v := sv.Major() == 3
	s.isHelm3 = &v
	return v
}

func (s *Server) IsHelm4() bool {
	if s.isHelm4 != nil {
		return *s.isHelm4
	}

	if os.Getenv("HELM_X_HELM4") != "" {
		v := true
		s.isHelm4 = &v
		return true
	}

	sv, err := s.detectHelmVersion()
	if err != nil {
		panic(err)
	}

	v := sv.Major() == 4
	s.isHelm4 = &v
	return v
}

func (s *Server) Run(ctx context.Context) error {
	serverURL := s.ServerURL()
	chartsDir := s.ChartsDir
	if chartsDir == "" {
		return fmt.Errorf("ChartsDir is required")
	}

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

		cmd := exec.CommandContext(ctx, s.getHelmBin(), "package", abs)
		cmd.Dir = worktree
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Println(string(out))
			return fmt.Errorf("unable to package %s: %w", chart, err)
		}
	}

	indexYamlPath := filepath.Join(worktree, "index.yaml")

	if s.IsHelm3() {
		return s.runHelm3(ctx, worktree, indexYamlPath, serverURL)
	} else if s.IsHelm4() {
		return s.runHelm4(ctx, worktree, indexYamlPath, serverURL)
	}

	return fmt.Errorf("unsupported helm version")
}

func (s *Server) runHelm3(ctx context.Context, worktree, indexYamlPath, serverURL string) error {
	var indexFile *repov3.IndexFile
	_, err := os.Stat(indexYamlPath)
	if err == nil {
		indexFile, err = repov3.LoadIndexFile(indexYamlPath)
		if err != nil {
			return err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		indexFile = repov3.NewIndexFile()
	} else {
		return err
	}

	chartPackages, err := filepath.Glob(filepath.Join(worktree, "*.tgz"))
	if err != nil {
		return err
	}

	var update bool
	for _, chartPackage := range chartPackages {
		_, err := loaderv3.LoadFile(chartPackage)
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
			if err := addToIndexFileHelm3(worktree, indexFile, downloadUrl.String()); err != nil {
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

	return s.startHTTPServer(ctx, worktree, indexYamlPath)
}

func (s *Server) runHelm4(ctx context.Context, worktree, indexYamlPath, serverURL string) error {
	var indexFile *repov4.IndexFile
	_, err := os.Stat(indexYamlPath)
	if err == nil {
		indexFile, err = repov4.LoadIndexFile(indexYamlPath)
		if err != nil {
			return err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		indexFile = repov4.NewIndexFile()
	} else {
		return err
	}

	chartPackages, err := filepath.Glob(filepath.Join(worktree, "*.tgz"))
	if err != nil {
		return err
	}

	var update bool
	for _, chartPackage := range chartPackages {
		_, err := loaderv4.LoadFile(chartPackage)
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
			if err := addToIndexFileHelm4(worktree, indexFile, downloadUrl.String()); err != nil {
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

	return s.startHTTPServer(ctx, worktree, indexYamlPath)
}

func (s *Server) startHTTPServer(ctx context.Context, worktree, indexYamlPath string) error {
	port := s.getPort()

	var serveMux http.ServeMux

	serveMux.HandleFunc("/index.yaml", func(w http.ResponseWriter, _ *http.Request) {
		f, err := os.Open(indexYamlPath)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		if _, err := io.Copy(w, f); err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}
	})

	serveMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".tgz") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		base := filepath.Base(r.URL.Path)

		pkgPath := filepath.Join(worktree, base)

		f, err := os.Open(pkgPath)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		if _, err := io.Copy(w, f); err != nil {
			_, _ = w.Write([]byte(err.Error()))
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

func addToIndexFileHelm3(worktree string, indexFile *repov3.IndexFile, url string) error {
	arch := filepath.Join(worktree, filepath.Base(url))

	// extract chart metadata
	fmt.Printf("Extracting chart metadata from %s\n", arch)
	c, err := loaderv3.LoadFile(arch)
	if err != nil {
		return fmt.Errorf("%s is not a helm chart package: %w", arch, err)
	}

	// calculate hash
	fmt.Printf("Calculating Hash for %s\n", arch)
	hash, err := provenancev3.DigestFile(arch)
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

func addToIndexFileHelm4(worktree string, indexFile *repov4.IndexFile, url string) error {
	arch := filepath.Join(worktree, filepath.Base(url))

	// extract chart metadata
	fmt.Printf("Extracting chart metadata from %s\n", arch)
	charter, err := loaderv4.LoadFile(arch)
	if err != nil {
		return fmt.Errorf("%s is not a helm chart package: %w", arch, err)
	}

	// Convert Charter to v2.Chart to access metadata
	c, ok := charter.(*v2.Chart)
	if !ok {
		return fmt.Errorf("chart is not a v2 chart")
	}

	// calculate hash
	fmt.Printf("Calculating Hash for %s\n", arch)
	hash, err := provenancev4.DigestFile(arch)
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
