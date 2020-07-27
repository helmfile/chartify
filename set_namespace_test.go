package chartify

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func removeAll(d string) {
	if !strings.Contains(d, "chartify_setnstest") {
		panic(fmt.Errorf("unexpected directory to be removed: %v", d))
	}

	if err := os.RemoveAll(d); err != nil {
		panic(err)
	}
}

func TestSetNamespace(t *testing.T) {
	d, err := ioutil.TempDir("", "chartify_setnstest")
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer removeAll(d)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}

	input := filepath.Join(wd, "testdata/setns/input")
	output := filepath.Join(wd, "testdata/setns/output")

	if err := filepath.Walk(input, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fatalf("%v", err)
		}

		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(input, path)
		if err != nil {
			t.Fatalf("%v", err)
		}

		data, err := ioutil.ReadFile(path)
		if err != nil {
			t.Fatalf("%v", err)
		}

		dstFile := filepath.Join(d, rel)

		if err := os.MkdirAll(filepath.Dir(dstFile), 0755); err != nil {
			t.Fatalf("%v", err)
		}

		if err := ioutil.WriteFile(dstFile, data, 0644); err != nil {
			t.Fatalf("%v", err)
		}

		return nil
	}); err != nil {
		t.Fatalf("%v", err)
	}

	r := &Runner{}

	if err := r.SetNamespace(d, "foo"); err != nil {
		t.Errorf("%v", err)
	}

	if err := filepath.Walk(output, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fatalf("%v", err)
		}

		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(output, path)
		if err != nil {
			t.Fatalf("%v", err)
		}

		want, err := ioutil.ReadFile(path)
		if err != nil {
			t.Fatalf("%v", err)
		}

		got, err := ioutil.ReadFile(filepath.Join(d, rel))
		if err != nil {
			t.Fatalf("%v", err)
		}

		if diff := cmp.Diff(string(want), string(got)); diff != "" {
			t.Fatalf("%s: %s", rel, diff)
		}

		return nil
	}); err != nil {
		t.Fatalf("%v", err)
	}
}
