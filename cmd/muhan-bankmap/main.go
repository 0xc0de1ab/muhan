package main

import (
	"flag"
	"fmt"
	"os"

	"muhan/internal/migrate/bankmap"
)

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	jsonOut := flag.Bool("json", false, "print JSON bank snapshot")
	includeObjects := flag.Bool("objects", false, "include mapped object instances in JSON snapshot")
	maxFindings := flag.Int("max-findings", 30, "maximum warnings/errors to print in text mode")
	flag.Parse()

	snapshot, err := bankmap.Build(bankmap.Options{Root: *root, IncludeObjects: *includeObjects})
	if err != nil {
		fmt.Fprintf(os.Stderr, "bankmap: %v\n", err)
		os.Exit(2)
	}

	if *jsonOut {
		if err := bankmap.EncodeJSON(os.Stdout, snapshot); err != nil {
			fmt.Fprintf(os.Stderr, "encode bankmap snapshot: %v\n", err)
			os.Exit(2)
		}
	} else {
		bankmap.WriteText(os.Stdout, snapshot, *maxFindings)
	}

	if len(snapshot.Errors) > 0 {
		os.Exit(1)
	}
}
