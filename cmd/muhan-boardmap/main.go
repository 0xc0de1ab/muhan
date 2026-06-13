package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/0xc0de1ab/muhan/internal/migrate/boardmap"
)

type summary struct {
	Root         string `json:"root"`
	BoardDirs    int    `json:"boardDirs"`
	IndexFiles   int    `json:"indexFiles"`
	IndexRecords int    `json:"indexRecords"`
	PostFiles    int    `json:"postFiles"`
	Warnings     int    `json:"warnings"`
	Errors       int    `json:"errors"`
}

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	jsonOut := flag.Bool("json", false, "print JSON report")
	maxFindings := flag.Int("max-findings", 30, "maximum warnings/errors to print in text mode")
	flag.Parse()

	report, err := boardmap.ScanRoot(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "boardmap: %v\n", err)
		os.Exit(2)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summarize(report)); err != nil {
			fmt.Fprintf(os.Stderr, "encode boardmap report: %v\n", err)
			os.Exit(2)
		}
	} else {
		boardmap.WriteText(os.Stdout, report, *maxFindings)
	}

	if len(report.Errors) > 0 {
		os.Exit(1)
	}
}

func summarize(report boardmap.Report) summary {
	return summary{
		Root:         report.Root,
		BoardDirs:    report.Counts.BoardDirs,
		IndexFiles:   report.Counts.IndexFiles,
		IndexRecords: report.Counts.IndexRecords,
		PostFiles:    report.Counts.PostFiles,
		Warnings:     report.Counts.Warnings,
		Errors:       report.Counts.Errors,
	}
}
