package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/0xc0de1ab/muhan/internal/migrate/playermap"
)

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	jsonOut := flag.Bool("json", false, "print JSON report")
	maxFindings := flag.Int("max-findings", 30, "maximum warnings/errors to print in text mode")
	flag.Parse()

	report, err := playermap.ScanRoot(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "playermap: %v\n", err)
		os.Exit(2)
	}

	if *jsonOut {
		if err := playermap.EncodeJSON(os.Stdout, report); err != nil {
			fmt.Fprintf(os.Stderr, "encode playermap report: %v\n", err)
			os.Exit(2)
		}
	} else {
		playermap.WriteText(os.Stdout, report, *maxFindings)
	}

	if len(report.Errors) > 0 {
		os.Exit(1)
	}
}
