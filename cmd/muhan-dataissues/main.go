package main

import (
	"flag"
	"fmt"
	"os"

	"muhan/internal/report/dataissues"
)

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	jsonOut := flag.Bool("json", false, "print JSON report")
	flag.Parse()

	report, err := dataissues.Scan(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan data issues: %v\n", err)
		os.Exit(2)
	}

	if *jsonOut {
		if err := dataissues.EncodeJSON(os.Stdout, report); err != nil {
			fmt.Fprintf(os.Stderr, "encode report: %v\n", err)
			os.Exit(2)
		}
	} else {
		dataissues.WriteText(os.Stdout, report)
	}

	if len(report.Issues) > 0 {
		os.Exit(1)
	}
}
