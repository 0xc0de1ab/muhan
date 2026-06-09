package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"muhan/internal/migrate/roommap"
	"muhan/internal/persist/cbin"
)

type Summary struct {
	Root          string     `json:"root"`
	RoomFiles     int        `json:"roomFiles"`
	Rooms         int        `json:"rooms"`
	Skipped       int        `json:"skipped"`
	Creatures     int        `json:"creatures"`
	Objects       int        `json:"objects"`
	ContentErrors int        `json:"contentErrors"`
	Decoded       cbin.Stats `json:"decoded"`
	Warnings      int        `json:"warnings"`
	Errors        int        `json:"errors"`
}

var roomFileRE = regexp.MustCompile(`^r[0-9]{5}$`)

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	jsonOut := flag.Bool("json", false, "print JSON summary")
	flag.Parse()

	summary, err := scan(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan room files: %v\n", err)
		os.Exit(2)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summary); err != nil {
			fmt.Fprintf(os.Stderr, "encode summary: %v\n", err)
			os.Exit(2)
		}
	} else {
		fmt.Printf("root: %s\n", summary.Root)
		fmt.Printf("rooms: %d (%d files, %d skipped)\n", summary.Rooms, summary.RoomFiles, summary.Skipped)
		fmt.Printf("creatures: %d\n", summary.Creatures)
		fmt.Printf("objects: %d\n", summary.Objects)
		fmt.Printf("decoded: %d rooms, %d exits, %d creatures, %d objects\n", summary.Decoded.Rooms, summary.Decoded.Exits, summary.Decoded.Creatures, summary.Decoded.Objects)
		fmt.Printf("contentErrors: %d\n", summary.ContentErrors)
		fmt.Printf("warnings: %d\n", summary.Warnings)
		fmt.Printf("errors: %d\n", summary.Errors)
	}

	if summary.Errors > 0 {
		os.Exit(1)
	}
}

func scan(root string) (Summary, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve root: %w", err)
	}

	summary := Summary{Root: absRoot}
	base := filepath.Join(absRoot, "rooms")
	if _, err := os.Stat(base); err != nil {
		return summary, err
	}

	err = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			summary.Errors++
			return nil
		}
		if d.IsDir() || !roomFileRE.MatchString(d.Name()) {
			return nil
		}

		summary.RoomFiles++
		data, err := os.ReadFile(path)
		if err != nil {
			summary.Errors++
			summary.Skipped++
			return nil
		}

		bundle, err := roommap.MapRoomFileBundle(path, data)
		if err != nil {
			summary.Errors++
			summary.Skipped++
		} else {
			summary.Rooms++
			summary.Creatures += len(bundle.Creatures)
			summary.Objects += len(bundle.Objects)
			if bundle.ContentError != "" {
				summary.ContentErrors++
				summary.Skipped++
			} else {
				mergeStats(&summary.Decoded, bundle.Decoded)
			}
			summary.Warnings += len(bundle.Warnings)
		}
		return nil
	})
	if err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func mergeStats(dst *cbin.Stats, src cbin.Stats) {
	dst.Objects += src.Objects
	dst.Creatures += src.Creatures
	dst.Rooms += src.Rooms
	dst.Exits += src.Exits
	dst.Descriptions += src.Descriptions
	dst.DescriptionBytes += src.DescriptionBytes
	dst.TrailingBytes += src.TrailingBytes
	if src.MaxDepth > dst.MaxDepth {
		dst.MaxDepth = src.MaxDepth
	}
}
