package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"muhan/internal/persist/cbin"
)

type Summary struct {
	Root string `json:"root"`

	ObjectPrototypeFiles     int `json:"objectPrototypeFiles"`
	ObjectPrototypeRecords   int `json:"objectPrototypeRecords"`
	CreaturePrototypeFiles   int `json:"creaturePrototypeFiles"`
	CreaturePrototypeRecords int `json:"creaturePrototypeRecords"`

	RoomFiles         int `json:"roomFiles"`
	PlayerFiles       int `json:"playerFiles"`
	BankFiles         int `json:"bankFiles"`
	BoardIndexFiles   int `json:"boardIndexFiles"`
	BoardIndexRecords int `json:"boardIndexRecords"`

	Decoded      cbin.Stats `json:"decoded"`
	SkippedFiles int        `json:"skippedFiles"`
	Errors       []string   `json:"errors,omitempty"`
}

func (s *Summary) addStats(st cbin.Stats) {
	s.Decoded.Objects += st.Objects
	s.Decoded.Creatures += st.Creatures
	s.Decoded.Rooms += st.Rooms
	s.Decoded.Exits += st.Exits
	s.Decoded.Descriptions += st.Descriptions
	s.Decoded.DescriptionBytes += st.DescriptionBytes
	s.Decoded.TrailingBytes += st.TrailingBytes
	if st.MaxDepth > s.Decoded.MaxDepth {
		s.Decoded.MaxDepth = st.MaxDepth
	}
}

func (s *Summary) addErr(path string, err error) {
	s.Errors = append(s.Errors, fmt.Sprintf("%s: %v", path, err))
}

func main() {
	root := flag.String("root", ".", "muhan data/source root")
	jsonOut := flag.Bool("json", false, "print JSON summary")
	maxErrors := flag.Int("max-errors", 30, "maximum errors to print in text mode")
	flag.Parse()

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve root: %v\n", err)
		os.Exit(2)
	}

	var sum Summary
	sum.Root = absRoot

	inspectObjmon(absRoot, &sum)
	inspectRooms(absRoot, &sum)
	inspectPlayers(absRoot, &sum)
	inspectBoards(absRoot, &sum)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(sum); err != nil {
			fmt.Fprintf(os.Stderr, "encode summary: %v\n", err)
			os.Exit(2)
		}
	} else {
		printText(sum, *maxErrors)
	}

	if len(sum.Errors) > 0 {
		os.Exit(1)
	}
}

var (
	objectProtoRE   = regexp.MustCompile(`^o[0-9][0-9]$`)
	creatureProtoRE = regexp.MustCompile(`^m[0-9][0-9]$`)
	roomFileRE      = regexp.MustCompile(`^r[0-9]{5}$`)
)

func inspectObjmon(root string, sum *Summary) {
	dir := filepath.Join(root, "objmon")
	entries, err := os.ReadDir(dir)
	if err != nil {
		sum.addErr(dir, err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			sum.addErr(path, err)
			continue
		}
		switch {
		case objectProtoRE.MatchString(name):
			n, err := cbin.ValidateObjectPrototypeFile(data)
			if err != nil {
				sum.addErr(path, err)
				continue
			}
			sum.ObjectPrototypeFiles++
			sum.ObjectPrototypeRecords += n
			sum.Decoded.Objects += n
		case creatureProtoRE.MatchString(name):
			n, err := cbin.ValidateCreaturePrototypeFile(data)
			if err != nil {
				sum.addErr(path, err)
				continue
			}
			sum.CreaturePrototypeFiles++
			sum.CreaturePrototypeRecords += n
			sum.Decoded.Creatures += n
		default:
			sum.SkippedFiles++
		}
	}
}

func inspectRooms(root string, sum *Summary) {
	base := filepath.Join(root, "rooms")
	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			sum.addErr(path, err)
			return nil
		}
		if d.IsDir() || !roomFileRE.MatchString(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			sum.addErr(path, err)
			return nil
		}
		st, err := cbin.DecodeRoomFile(data)
		if err != nil {
			sum.addErr(path, err)
			return nil
		}
		sum.RoomFiles++
		sum.addStats(st)
		return nil
	})
}

func inspectPlayers(root string, sum *Summary) {
	playerRoot := filepath.Join(root, "player")
	entries, err := os.ReadDir(playerRoot)
	if err != nil {
		sum.addErr(playerRoot, err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			sum.SkippedFiles++
			continue
		}
		name := entry.Name()
		switch name {
		case "alias", "fal", "family", "invite", "marriage", "simul", "temp", "vote":
			sum.SkippedFiles += countRegularFiles(filepath.Join(playerRoot, name))
		case "bank":
			inspectObjectTreeDir(filepath.Join(playerRoot, name), sum, true)
		default:
			inspectCreatureTreeDir(filepath.Join(playerRoot, name), sum)
		}
	}
	inspectObjectTreeDir(filepath.Join(playerRoot, "family", "bank"), sum, true)
}

func inspectCreatureTreeDir(dir string, sum *Summary) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		sum.addErr(dir, err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			sum.SkippedFiles += countRegularFiles(filepath.Join(dir, entry.Name()))
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			sum.addErr(path, err)
			continue
		}
		st, err := cbin.DecodeCreatureFile(data)
		if err != nil {
			sum.addErr(path, err)
			continue
		}
		sum.PlayerFiles++
		sum.addStats(st)
	}
}

func inspectObjectTreeDir(dir string, sum *Summary, bank bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		sum.addErr(dir, err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			sum.SkippedFiles += countRegularFiles(filepath.Join(dir, entry.Name()))
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			sum.addErr(path, err)
			continue
		}
		st, err := cbin.DecodeObjectFile(data)
		if bank {
			st, err = cbin.DecodeObjectFileAllowTrailing(data)
		}
		if err != nil {
			sum.addErr(path, err)
			continue
		}
		if bank {
			sum.BankFiles++
		}
		sum.addStats(st)
	}
}

func inspectBoards(root string, sum *Summary) {
	base := filepath.Join(root, "board")
	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			sum.addErr(path, err)
			return nil
		}
		if d.IsDir() || d.Name() != "board_index" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			sum.addErr(path, err)
			return nil
		}
		n, err := cbin.ValidateBoardIndexFile(data)
		if err != nil {
			sum.addErr(path, err)
			return nil
		}
		sum.BoardIndexFiles++
		sum.BoardIndexRecords += n
		return nil
	})
}

func countRegularFiles(dir string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func printText(sum Summary, maxErrors int) {
	fmt.Printf("root: %s\n", sum.Root)
	fmt.Printf("object prototypes: %d files, %d records\n", sum.ObjectPrototypeFiles, sum.ObjectPrototypeRecords)
	fmt.Printf("creature prototypes: %d files, %d records\n", sum.CreaturePrototypeFiles, sum.CreaturePrototypeRecords)
	fmt.Printf("rooms: %d files\n", sum.RoomFiles)
	fmt.Printf("players: %d files\n", sum.PlayerFiles)
	fmt.Printf("banks: %d files\n", sum.BankFiles)
	fmt.Printf("board indexes: %d files, %d records\n", sum.BoardIndexFiles, sum.BoardIndexRecords)
	fmt.Printf("decoded totals: rooms=%d creatures=%d objects=%d exits=%d descriptions=%d descriptionBytes=%d trailingBytes=%d maxDepth=%d\n",
		sum.Decoded.Rooms, sum.Decoded.Creatures, sum.Decoded.Objects, sum.Decoded.Exits,
		sum.Decoded.Descriptions, sum.Decoded.DescriptionBytes, sum.Decoded.TrailingBytes, sum.Decoded.MaxDepth)
	fmt.Printf("skipped non-core files: %d\n", sum.SkippedFiles)
	if len(sum.Errors) == 0 {
		fmt.Println("errors: 0")
		return
	}
	fmt.Printf("errors: %d\n", len(sum.Errors))
	limit := maxErrors
	if limit <= 0 || limit > len(sum.Errors) {
		limit = len(sum.Errors)
	}
	for _, item := range sum.Errors[:limit] {
		fmt.Printf("  - %s\n", strings.TrimSpace(item))
	}
	if limit < len(sum.Errors) {
		fmt.Printf("  ... %d more\n", len(sum.Errors)-limit)
	}
}
