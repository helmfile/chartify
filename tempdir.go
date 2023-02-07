package chartify

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"

	"github.com/davecgh/go-spew/spew"
)

func makeTempDir(release, chart string, opts *ChartifyOpts) string {
	var err error

	var id string
	if opts.ID != "" {
		id = strings.ReplaceAll(opts.ID, "/", string(filepath.Separator))
	} else {
		id, err = GenerateID(release, chart, opts)
		if err != nil {
			panic(err)
		}
	}

	workDir := os.Getenv(EnvVarTempDir)
	if workDir == "" {
		workDir, err = os.MkdirTemp(os.TempDir(), "chartify")
		if err != nil {
			panic(err)
		}
	} else if !filepath.IsAbs(workDir) {
		workDir, err = filepath.Abs(workDir)
		if err != nil {
			panic(err)
		}
	}

	d := filepath.Join(workDir, id)

	if os.Getenv(EnvVarDebug) != "" {
		bs, _ := json.Marshal(map[string]interface{}{
			"Release": release,
			"Chart":   chart,
			"Options": opts,
		})

		if len(bs) > 0 {
			_ = os.WriteFile(d+".json", bs, 0666)
		}
	}

	info, err := os.Stat(d)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		panic(err)
	} else if info == nil {
		if err := os.MkdirAll(d, 0777); err != nil {
			panic(err)
		}
	}

	return d
}

func GenerateID(release, chart string, opts *ChartifyOpts) (string, error) {
	var id []string

	if opts.Namespace != "" {
		id = append(id, opts.Namespace)
	}

	id = append(id, release)

	hash, err := HashObject([]interface{}{release, chart, opts})
	if err != nil {
		return "", err
	}

	id = append(id, hash)

	return strings.Join(id, "-"), nil
}

func HashObject(obj interface{}) (string, error) {
	hash := fnv.New32a()

	hash.Reset()

	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	_, _ = printer.Fprintf(hash, "%#v", obj)

	sum := fmt.Sprint(hash.Sum32())

	return SafeEncodeString(sum), nil
}
