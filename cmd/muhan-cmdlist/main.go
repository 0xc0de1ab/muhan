package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/0xc0de1ab/muhan/internal/commandspec/extract"
)

type output struct {
	Root    string          `json:"root"`
	Source  string          `json:"source"`
	Count   int             `json:"count"`
	Entries []extract.Entry `json:"entries"`
}

func main() {
	root := flag.String("root", ".", "legacy Muhan source root")
	jsonOut := flag.Bool("json", false, "print JSON")
	flag.Parse()

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve root: %v\n", err)
		os.Exit(2)
	}

	source := filepath.Join(absRoot, "src", "global.c")
	entries, err := extract.ExtractFile(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract cmdlist: %v\n", err)
		os.Exit(2)
	}

	out := output{
		Root:    absRoot,
		Source:  source,
		Count:   len(entries),
		Entries: entries,
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "encode JSON: %v\n", err)
			os.Exit(2)
		}
		return
	}

	fmt.Printf("%s\n", source)
	fmt.Printf("%d command entries\n", len(entries))
	for _, entry := range entries {
		fmt.Printf("%-20s %4d %-24s", entry.Name, entry.Number, entry.Handler)
		if entry.Privileged {
			fmt.Print(" privileged")
		}
		if entry.Special {
			fmt.Print(" special")
		}
		fmt.Println()
	}
}
