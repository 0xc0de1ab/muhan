package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"muhan/internal/migrate/protomap"
)

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	jsonOut := flag.Bool("json", false, "write counts and findings as JSON")
	flag.Parse()

	report, err := buildReport(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "protomap: %v\n", err)
		os.Exit(2)
	}
	if err := writeReport(os.Stdout, report, *jsonOut); err != nil {
		fmt.Fprintf(os.Stderr, "protomap: %v\n", err)
		os.Exit(2)
	}
	if len(report.Errors) > 0 {
		os.Exit(1)
	}
}

func buildReport(root string) (protomap.Report, error) {
	snapshot, err := protomap.Build(protomap.Options{Root: root})
	if err != nil {
		return protomap.Report{}, err
	}
	return snapshot.Report(), nil
}

func writeReport(w io.Writer, report protomap.Report, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	fmt.Fprintf(w, "root: %s\n", report.Root)
	fmt.Fprintf(w, "objectPrototypeFiles: %d\n", report.Counts.ObjectPrototypeFiles)
	fmt.Fprintf(w, "objectPrototypes: %d\n", report.Counts.ObjectPrototypes)
	fmt.Fprintf(w, "creaturePrototypeFiles: %d\n", report.Counts.CreaturePrototypeFiles)
	fmt.Fprintf(w, "creaturePrototypes: %d\n", report.Counts.CreaturePrototypes)
	fmt.Fprintf(w, "totalPrototypes: %d\n", report.Counts.TotalPrototypes)
	fmt.Fprintf(w, "skippedFiles: %d\n", report.Counts.SkippedFiles)
	fmt.Fprintf(w, "warnings: %d\n", len(report.Warnings))
	for _, warning := range report.Warnings {
		fmt.Fprintf(w, "warning: %s %s\n", findingLocation(warning), warning.Message)
	}
	fmt.Fprintf(w, "errors: %d\n", len(report.Errors))
	for _, err := range report.Errors {
		fmt.Fprintf(w, "error: %s %s\n", findingLocation(err), err.Message)
	}
	return nil
}

func findingLocation(f protomap.Finding) string {
	switch {
	case f.Path != "" && f.ID != "":
		return f.Path + " " + f.ID
	case f.Path != "":
		return f.Path
	case f.ID != "":
		return f.ID
	default:
		return "-"
	}
}
