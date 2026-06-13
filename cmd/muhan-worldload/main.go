package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/0xc0de1ab/muhan/internal/world/load"
)

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	jsonOut := flag.Bool("json", false, "print JSON summary")
	maxFindings := flag.Int("max-findings", 30, "maximum warnings/errors to print in text mode")
	flag.Parse()

	summary, err := load.LoadRoot(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "worldload: %v\n", err)
		os.Exit(2)
	}
	if err := writeSummary(os.Stdout, summary, *jsonOut, *maxFindings); err != nil {
		fmt.Fprintf(os.Stderr, "worldload: %v\n", err)
		os.Exit(2)
	}
	if len(summary.Errors) > 0 {
		os.Exit(1)
	}
}

func writeSummary(w io.Writer, summary load.Summary, jsonOut bool, maxFindings int) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}

	fmt.Fprintf(w, "root: %s\n", summary.Root)
	fmt.Fprintf(w, "rooms: %d (%d files, %d skipped)\n", summary.Counts.Rooms, summary.Counts.RoomFiles, summary.Counts.SkippedRooms)
	fmt.Fprintf(w, "roomContents: %d exits, %d creatures, %d objects, %d descriptions, %d content errors\n",
		summary.Counts.RoomExits,
		summary.Counts.RoomCreatures,
		summary.Counts.RoomObjects,
		summary.Counts.RoomDescriptions,
		summary.Counts.RoomContentErrors)
	fmt.Fprintf(w, "players: %d (%d files)\n", summary.Counts.Players, summary.Counts.PlayerFiles)
	fmt.Fprintf(w, "playerObjects: %d\n", summary.Counts.PlayerObjects)
	fmt.Fprintf(w, "banks: %d accounts, %d objects, %d trailing bytes\n", summary.Counts.BankAccounts, summary.Counts.BankObjects, summary.Counts.BankTrailingBytes)
	fmt.Fprintf(w, "prototype resolution: resolved=%d synthetic=%d ambiguous=%d\n",
		summary.Counts.PrototypeResolved,
		summary.Counts.PrototypeSynthetic,
		summary.Counts.PrototypeAmbiguous,
	)
	fmt.Fprintf(w, "creatures: %d (%d creature prototypes)\n", summary.Counts.Creatures, summary.Counts.CreaturePrototypes)
	fmt.Fprintf(w, "objectInstances: %d\n", summary.Counts.ObjectInstances)
	fmt.Fprintf(w, "objectPrototypes: %d (%d files, %d synthetic materialized)\n",
		summary.Counts.ObjectPrototypes,
		summary.Counts.ObjectPrototypeFiles,
		summary.Counts.SyntheticObjectPrototypes,
	)
	fmt.Fprintf(w, "warnings: %d\n", len(summary.Warnings))
	writeFindings(w, "warning", summary.Warnings, maxFindings)
	fmt.Fprintf(w, "errors: %d\n", len(summary.Errors))
	writeFindings(w, "error", summary.Errors, maxFindings)
	return nil
}

func writeFindings(w io.Writer, label string, findings []load.Finding, maxFindings int) {
	if len(findings) == 0 {
		return
	}
	limit := maxFindings
	if limit <= 0 || limit > len(findings) {
		limit = len(findings)
	}
	for _, finding := range findings[:limit] {
		fmt.Fprintf(w, "%s: %s %s\n", label, findingLocation(finding), finding.Message)
	}
	if limit < len(findings) {
		fmt.Fprintf(w, "  ... %d more\n", len(findings)-limit)
	}
}

func findingLocation(f load.Finding) string {
	switch {
	case f.Path != "" && f.ID != "" && f.Ref != "":
		return f.Path + " " + f.ID + " -> " + f.Ref
	case f.Path != "" && f.ID != "":
		return f.Path + " " + f.ID
	case f.ID != "" && f.Ref != "":
		return f.ID + " -> " + f.Ref
	case f.Path != "":
		return f.Path
	case f.ID != "":
		return f.ID
	default:
		return "-"
	}
}
