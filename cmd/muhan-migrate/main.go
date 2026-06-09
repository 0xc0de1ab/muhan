package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/persist/cbin"
	"muhan/internal/persist/legacykr"
)

type Manifest struct {
	Root        string    `json:"root"`
	GeneratedAt time.Time `json:"generatedAt"`
	Counts      Counts    `json:"counts"`
	Warnings    []Finding `json:"warnings"`
	Errors      []Finding `json:"errors"`
}

type Counts struct {
	LegacyRooms   int `json:"legacyRooms"`
	LegacyPlayers int `json:"legacyPlayers"`
	LegacyBanks   int `json:"legacyBanks"`
	LegacyBoards  int `json:"legacyBoards"`

	RoomFiles                int `json:"roomFiles"`
	PlayerFiles              int `json:"playerFiles"`
	BankFiles                int `json:"bankFiles"`
	BoardIndexFiles          int `json:"boardIndexFiles"`
	BoardIndexRecords        int `json:"boardIndexRecords"`
	BoardPostFiles           int `json:"boardPostFiles"`
	ObjectPrototypeFiles     int `json:"objectPrototypeFiles"`
	ObjectPrototypeRecords   int `json:"objectPrototypeRecords"`
	CreaturePrototypeFiles   int `json:"creaturePrototypeFiles"`
	CreaturePrototypeRecords int `json:"creaturePrototypeRecords"`
	SkippedFiles             int `json:"skippedFiles"`

	Names   NameCounts `json:"names"`
	Decoded cbin.Stats `json:"decoded"`
}

type NameCounts struct {
	PlayerNameCandidates   int `json:"playerNameCandidates"`
	UTF8PlayerNames        int `json:"utf8PlayerNames"`
	LegacyKRPlayerNames    int `json:"legacyKrPlayerNames"`
	InvalidPlayerNames     int `json:"invalidPlayerNames"`
	PlayerBucketMismatches int `json:"playerBucketMismatches"`
}

type Finding struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func (m *Manifest) addWarning(path, message string) {
	m.Warnings = append(m.Warnings, Finding{Path: path, Message: message})
}

func (m *Manifest) addError(path string, err error) {
	m.Errors = append(m.Errors, Finding{Path: path, Message: err.Error()})
}

func (c *Counts) addStats(st cbin.Stats) {
	c.Decoded.Objects += st.Objects
	c.Decoded.Creatures += st.Creatures
	c.Decoded.Rooms += st.Rooms
	c.Decoded.Exits += st.Exits
	c.Decoded.Descriptions += st.Descriptions
	c.Decoded.DescriptionBytes += st.DescriptionBytes
	c.Decoded.TrailingBytes += st.TrailingBytes
	if st.MaxDepth > c.Decoded.MaxDepth {
		c.Decoded.MaxDepth = st.MaxDepth
	}
}

func main() {
	root := flag.String("root", ".", "legacy Muhan data root")
	out := flag.String("out", "", "manifest output path; stdout when empty")
	flag.Parse()

	manifest, err := BuildManifest(*root, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(os.Stderr, "build manifest: %v\n", err)
		os.Exit(2)
	}

	if err := writeManifest(manifest, *out); err != nil {
		fmt.Fprintf(os.Stderr, "write manifest: %v\n", err)
		os.Exit(2)
	}

	if len(manifest.Errors) > 0 {
		os.Exit(1)
	}
}

func BuildManifest(root string, generatedAt time.Time) (Manifest, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve root: %w", err)
	}

	manifest := Manifest{
		Root:        absRoot,
		GeneratedAt: generatedAt.UTC(),
		Warnings:    []Finding{},
		Errors:      []Finding{},
	}

	inspectObjmon(absRoot, &manifest)
	inspectRooms(absRoot, &manifest)
	inspectPlayers(absRoot, &manifest)
	inspectBoards(absRoot, &manifest)

	if manifest.Counts.Names.InvalidPlayerNames > 0 {
		manifest.addWarning(
			filepath.Join(absRoot, "player"),
			fmt.Sprintf("%d player filename(s) do not satisfy UTF-8 legacy name policy", manifest.Counts.Names.InvalidPlayerNames),
		)
	}
	if manifest.Counts.Names.PlayerBucketMismatches > 0 {
		manifest.addWarning(
			filepath.Join(absRoot, "player"),
			fmt.Sprintf("%d player filename(s) are stored under a shard that does not match the UTF-8 initial bucket", manifest.Counts.Names.PlayerBucketMismatches),
		)
	}

	return manifest, nil
}

func writeManifest(manifest Manifest, out string) error {
	var w io.Writer = os.Stdout
	var file *os.File
	if out != "" {
		var err error
		file, err = os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		w = file
	}

	return encodeManifest(w, manifest)
}

func encodeManifest(w io.Writer, manifest Manifest) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(manifest)
}

var (
	objectProtoRE   = regexp.MustCompile(`^o[0-9][0-9]$`)
	creatureProtoRE = regexp.MustCompile(`^m[0-9][0-9]$`)
	roomFileRE      = regexp.MustCompile(`^r[0-9]{5}$`)
	boardPostRE     = regexp.MustCompile(`^board\.[0-9]+$`)
)

func inspectObjmon(root string, manifest *Manifest) {
	dir := filepath.Join(root, "objmon")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			manifest.addWarning(dir, "objmon directory not found")
			return
		}
		manifest.addError(dir, err)
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
			manifest.addError(path, err)
			continue
		}

		switch {
		case objectProtoRE.MatchString(name):
			n, err := cbin.ValidateObjectPrototypeFile(data)
			if err != nil {
				manifest.addError(path, err)
				continue
			}
			manifest.Counts.ObjectPrototypeFiles++
			manifest.Counts.ObjectPrototypeRecords += n
			manifest.Counts.Decoded.Objects += n
		case creatureProtoRE.MatchString(name):
			n, err := cbin.ValidateCreaturePrototypeFile(data)
			if err != nil {
				manifest.addError(path, err)
				continue
			}
			manifest.Counts.CreaturePrototypeFiles++
			manifest.Counts.CreaturePrototypeRecords += n
			manifest.Counts.Decoded.Creatures += n
		default:
			manifest.Counts.SkippedFiles++
		}
	}
}

func inspectRooms(root string, manifest *Manifest) {
	base := filepath.Join(root, "rooms")
	if _, err := os.Stat(base); err != nil {
		if os.IsNotExist(err) {
			manifest.addWarning(base, "rooms directory not found")
			return
		}
		manifest.addError(base, err)
		return
	}

	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			manifest.addError(path, err)
			return nil
		}
		if d.IsDir() || !roomFileRE.MatchString(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			manifest.addError(path, err)
			return nil
		}
		st, err := cbin.DecodeRoomFile(data)
		if err != nil {
			manifest.addError(path, err)
			return nil
		}
		manifest.Counts.RoomFiles++
		manifest.Counts.LegacyRooms++
		manifest.Counts.addStats(st)
		return nil
	})
}

func inspectPlayers(root string, manifest *Manifest) {
	playerRoot := filepath.Join(root, "player")
	entries, err := os.ReadDir(playerRoot)
	if err != nil {
		if os.IsNotExist(err) {
			manifest.addWarning(playerRoot, "player directory not found")
			return
		}
		manifest.addError(playerRoot, err)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			manifest.Counts.SkippedFiles++
			continue
		}

		name := entry.Name()
		dir := filepath.Join(playerRoot, name)
		switch name {
		case "alias", "fal", "family", "invite", "marriage", "simul", "temp", "vote":
			manifest.Counts.SkippedFiles += countRegularFiles(dir)
		case "bank":
			inspectBankDir(dir, manifest)
		default:
			inspectPlayerShardDir(dir, manifest)
		}
	}

	inspectBankDir(filepath.Join(playerRoot, "family", "bank"), manifest)
}

func inspectPlayerShardDir(dir string, manifest *Manifest) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		manifest.addError(dir, err)
		return
	}

	shardName := filepath.Base(dir)

	for _, entry := range entries {
		if entry.IsDir() {
			manifest.Counts.SkippedFiles += countRegularFiles(filepath.Join(dir, entry.Name()))
			continue
		}

		planPlayerName(shardName, entry.Name(), &manifest.Counts.Names)

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			manifest.addError(path, err)
			continue
		}
		st, err := cbin.DecodeCreatureFile(data)
		if err != nil {
			manifest.addError(path, err)
			continue
		}

		manifest.Counts.PlayerFiles++
		manifest.Counts.LegacyPlayers++
		manifest.Counts.addStats(st)
	}
}

func inspectBankDir(dir string, manifest *Manifest) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		manifest.addError(dir, err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			manifest.Counts.SkippedFiles += countRegularFiles(filepath.Join(dir, entry.Name()))
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			manifest.addError(path, err)
			continue
		}
		st, err := cbin.DecodeObjectFileAllowTrailing(data)
		if err != nil {
			manifest.addError(path, err)
			continue
		}

		manifest.Counts.BankFiles++
		manifest.Counts.LegacyBanks++
		manifest.Counts.addStats(st)
	}
}

func inspectBoards(root string, manifest *Manifest) {
	base := filepath.Join(root, "board")
	if _, err := os.Stat(base); err != nil {
		if os.IsNotExist(err) {
			manifest.addWarning(base, "board directory not found")
			return
		}
		manifest.addError(base, err)
		return
	}

	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			manifest.addError(path, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		switch {
		case d.Name() == "board_index":
			data, err := os.ReadFile(path)
			if err != nil {
				manifest.addError(path, err)
				return nil
			}
			n, err := cbin.ValidateBoardIndexFile(data)
			if err != nil {
				manifest.addError(path, err)
				return nil
			}
			manifest.Counts.BoardIndexFiles++
			manifest.Counts.BoardIndexRecords += n
			manifest.Counts.LegacyBoards += n
		case boardPostRE.MatchString(d.Name()):
			manifest.Counts.BoardPostFiles++
		default:
			manifest.Counts.SkippedFiles++
		}
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

func planPlayerName(shard, name string, counts *NameCounts) {
	counts.PlayerNameCandidates++
	decoded, legacy, err := legacyPlayerNameToUTF8(name)
	if err != nil {
		counts.InvalidPlayerNames++
		return
	}
	if legacy {
		counts.LegacyKRPlayerNames++
	} else {
		counts.UTF8PlayerNames++
	}
	if !krtext.IsLegacyName(decoded) {
		counts.InvalidPlayerNames++
		return
	}
	if expected := krtext.FirstHangulBucket(decoded); expected != "temp" {
		decodedShard, _, err := legacyPlayerNameToUTF8(shard)
		if err == nil && decodedShard != expected {
			counts.PlayerBucketMismatches++
		}
	}
}

func legacyPlayerNameToUTF8(name string) (decoded string, legacy bool, err error) {
	if utf8Name, err := legacykr.DecodeEUCKR([]byte(name)); err == nil && krtext.IsAllHangulSyllables(utf8Name) {
		return utf8Name, true, nil
	}
	decoded, err = legacykr.ValidUTF8OrDecode([]byte(name))
	if err != nil {
		return "", false, err
	}
	if decoded == name {
		return decoded, false, nil
	}
	return decoded, true, nil
}
