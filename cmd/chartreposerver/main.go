package main

import (
	"context"
	"fmt"
	"os"

	"github.com/variantdev/chartify/chartrepo"
)

func main() {
	ctx := context.Background()

	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "USAGE: chartreposerver CHARTS_DIR\n")
		os.Exit(1)
	}

	dir := os.Args[1]

	srv := &chartrepo.Server{}

	if err := srv.Run(ctx, dir); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v", err)
		os.Exit(1)
	}
}
