package main

import (
	"flag"
	"fmt"
	"os"

	"muhan/internal/migrate/bundle"
)

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	out := flag.String("out", "", "bundle manifest output path; stdout when empty")
	flag.Parse()

	manifest, err := bundle.Build(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "muhan-bundle: %v\n", err)
		os.Exit(2)
	}

	if err := writeBundle(manifest, *out); err != nil {
		fmt.Fprintf(os.Stderr, "muhan-bundle: %v\n", err)
		os.Exit(2)
	}
}

func writeBundle(manifest bundle.Bundle, out string) error {
	if out == "" {
		return bundle.EncodeJSON(os.Stdout, manifest)
	}

	file, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	return bundle.EncodeJSON(file, manifest)
}
