package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/variantdev/chartify"
)

type stringSlice []string

func (i *stringSlice) String() string {
	return "my string representation"
}

func (i *stringSlice) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func main() {
	var file string
	var outDir string

	opts := chartify.ChartifyOpts{
		Debug:                       false,
		ValuesFiles:                 nil,
		SetValues:                   nil,
		Namespace:                   "",
		ChartVersion:                "",
		TillerNamespace:             "",
		EnableKustomizeAlphaPlugins: false,
		Injectors:                   nil,
		Injects:                     nil,
		AdhocChartDependencies:      nil,
		JsonPatches:                 nil,
		StrategicMergePatches:       nil,
		WorkaroundOutputDirIssue:    false,
	}

	deps := stringSlice{}

	flag.StringVar(&file, "f", "-", "The path to the input file or stdout(-)")
	flag.StringVar(&outDir, "o", "", "The path to the output directory")
	flag.Var(&deps, "d", "one or more \"alias=chart:version\" to add adhoc chart dependencies")

	flag.Parse()

	opts.DeprecatedAdhocChartDependencies = deps

	c := chartify.New(chartify.UseHelm3(true), chartify.HelmBin("helm"))

	args := flag.Args()

	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Error: exactly 2 arguments has been expected. Got %d (%+v)\n", len(args), args)

		os.Exit(1)
	}

	if outDir == "" {
		fmt.Fprintf(os.Stderr, "Error: -o OUTPUT_DIR is required but missing\n")

		os.Exit(1)
	}

	generatedDir, err := c.Chartify(args[0], args[1], chartify.WithChartifyOpts(&opts))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := os.Rename(generatedDir, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: moving %s to %s: %v\n", generatedDir, outDir, err)
		os.Exit(1)
	}
}
