package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"muhan/internal/migrate/snapshot"
)

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	out := flag.String("out", "", "snapshot JSONL output path; stdout when empty")
	flag.Parse()

	if err := writeSnapshot(*root, *out); err != nil {
		fmt.Fprintf(os.Stderr, "snapshot: %v\n", err)
		os.Exit(2)
	}
}

func writeSnapshot(root, out string) error {
	var w io.Writer = os.Stdout
	var file *os.File
	if out != "" {
		var err error
		file, err = os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		w = file
	}

	return snapshot.WriteJSONL(w, snapshot.Options{Root: root})
}
