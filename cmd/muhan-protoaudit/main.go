package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/0xc0de1ab/muhan/internal/migrate/protoaudit"
)

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	outdir := flag.String("outdir", "", "audit artifact output directory")
	jsonOut := flag.Bool("json", false, "print manifest JSON to stdout")
	flag.Parse()

	snapshot, err := protoaudit.Build(protoaudit.Options{Root: *root})
	if err != nil {
		fmt.Fprintf(os.Stderr, "protoaudit: %v\n", err)
		os.Exit(2)
	}

	var manifest protoaudit.Manifest
	if *outdir != "" {
		manifest, err = protoaudit.Write(*outdir, snapshot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "protoaudit: %v\n", err)
			os.Exit(2)
		}
	} else {
		manifest = protoaudit.Manifest{
			SchemaVersion:   protoaudit.SchemaVersion,
			ResolverVersion: protoaudit.ResolverVersion,
			Root:            snapshot.Root,
			GeneratedAt:     snapshot.GeneratedAt,
			Counts:          snapshot.Counts,
			WorldCounts:     snapshot.WorldCounts,
			Warnings:        snapshot.Warnings,
			Errors:          snapshot.Errors,
		}
	}

	if *jsonOut {
		if err := writeJSON(os.Stdout, manifest); err != nil {
			fmt.Fprintf(os.Stderr, "protoaudit: %v\n", err)
			os.Exit(2)
		}
		return
	}
	writeText(os.Stdout, manifest)
}

func writeJSON(w io.Writer, manifest protoaudit.Manifest) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(manifest)
}

func writeText(w io.Writer, manifest protoaudit.Manifest) {
	fmt.Fprintf(w, "root: %s\n", manifest.Root)
	fmt.Fprintf(w, "evidenceRecords: %d\n", manifest.Counts.EvidenceRecords)
	fmt.Fprintf(w, "findings: %d\n", manifest.Counts.FindingRecords)
	fmt.Fprintf(w, "resolution: resolved=%d unresolved=%d ambiguous=%d synthetic=%d missing=%d\n",
		manifest.Counts.Resolved,
		manifest.Counts.Unresolved,
		manifest.Counts.Ambiguous,
		manifest.Counts.Synthetic,
		manifest.Counts.MissingPrototypeResolution,
	)
	fmt.Fprintf(w, "sourceFiles: %d hashErrors=%d\n", manifest.Counts.SourceFiles, manifest.Counts.SourceHashErrors)
	for _, file := range manifest.Files {
		fmt.Fprintf(w, "artifact: %s records=%d bytes=%d sha256=%s\n", file.Path, file.Records, file.Bytes, file.SHA256)
	}
}
