package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/0xc0de1ab/muhan/internal/report/repairplan"
)

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	jsonOut := flag.Bool("json", false, "print JSON repair plan")
	flag.Parse()

	plan, err := repairplan.Generate(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate repair plan: %v\n", err)
		os.Exit(2)
	}

	if *jsonOut {
		if err := repairplan.EncodeJSON(os.Stdout, plan); err != nil {
			fmt.Fprintf(os.Stderr, "encode repair plan: %v\n", err)
			os.Exit(2)
		}
		return
	}

	repairplan.WriteText(os.Stdout, plan)
}
