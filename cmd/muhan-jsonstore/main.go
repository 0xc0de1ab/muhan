package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"muhan/internal/persist/jsonstore"
)

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	outdir := flag.String("outdir", "", "artifact output directory")
	flag.Parse()

	if err := writeIndex(*root, *outdir, time.Now().UTC()); err != nil {
		fmt.Fprintf(os.Stderr, "jsonstore: %v\n", err)
		os.Exit(2)
	}
}

func writeIndex(root, outdir string, generatedAt time.Time) error {
	if outdir == "" {
		return fmt.Errorf("missing -outdir")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}

	index := jsonstore.NewArtifactIndex(absRoot, generatedAt, nil)
	return jsonstore.WriteJSON(filepath.Join(outdir, "index.json"), index)
}
