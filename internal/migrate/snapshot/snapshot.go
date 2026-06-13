package snapshot

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const legacySource = "legacy"

type Record struct {
	Kind        string         `json:"kind"`
	ID          string         `json:"id"`
	DisplayName string         `json:"displayName"`
	Metadata    model.Metadata `json:"metadata"`
	Warnings    []string       `json:"warnings"`
}

type Options struct {
	Root string
}

func WriteJSONL(w io.Writer, opts Options) error {
	root := opts.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}

	records, err := Build(Options{Root: absRoot})
	if err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	for _, record := range records {
		if err := enc.Encode(record); err != nil {
			return fmt.Errorf("encode %s %q: %w", record.Kind, record.ID, err)
		}
	}
	return nil
}

func Build(opts Options) ([]Record, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}

	var records []Record
	records = append(records, snapshotRooms(absRoot)...)
	records = append(records, snapshotPlayers(absRoot)...)
	return records, nil
}

var roomFileRE = regexp.MustCompile(`^r[0-9]{5}$`)

func snapshotRooms(root string) []Record {
	base := filepath.Join(root, "rooms")
	if _, err := os.Stat(base); err != nil {
		return nil
	}

	var records []Record
	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !roomFileRE.MatchString(d.Name()) {
			return nil
		}

		id := model.RoomID(d.Name())
		record := Record{
			Kind:        "room",
			ID:          string(id),
			DisplayName: string(id),
			Metadata: model.Metadata{
				Source:     legacySource,
				LegacyKind: "room",
				LegacyID:   d.Name(),
				LegacyPath: displayRelPath(root, path),
			},
			Warnings: []string{},
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			record.Warnings = append(record.Warnings, fmt.Sprintf("read room file: %v", readErr))
			records = append(records, record)
			return nil
		}
		if _, decodeErr := cbin.DecodeRoomFile(data); decodeErr != nil {
			record.Warnings = append(record.Warnings, fmt.Sprintf("decode room file: %v", decodeErr))
		}
		records = append(records, record)
		return nil
	})
	return records
}

var skippedPlayerDirs = map[string]struct{}{
	"alias":    {},
	"bank":     {},
	"fal":      {},
	"family":   {},
	"invite":   {},
	"marriage": {},
	"simul":    {},
	"temp":     {},
	"vote":     {},
}

func snapshotPlayers(root string) []Record {
	playerRoot := filepath.Join(root, "player")
	shards, err := os.ReadDir(playerRoot)
	if err != nil {
		return nil
	}

	var records []Record
	for _, shard := range shards {
		if !shard.IsDir() {
			continue
		}
		if _, skip := skippedPlayerDirs[shard.Name()]; skip {
			continue
		}
		dir := filepath.Join(playerRoot, shard.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			records = append(records, playerRecord(root, path, shard.Name(), entry.Name()))
		}
	}

	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Metadata.LegacyPath < records[j].Metadata.LegacyPath
	})
	return records
}

func playerRecord(root, path, shard, name string) Record {
	displayName, encoding, warnings := decodePathComponent(name, "player filename")
	shardName, _, shardWarnings := decodePathComponent(shard, "player shard")
	warnings = append(warnings, shardWarnings...)

	id := displayName
	if id == "" {
		id = "raw-filename-" + hex.EncodeToString([]byte(name))
	}

	return Record{
		Kind:        "player",
		ID:          string(model.PlayerID(id)),
		DisplayName: displayName,
		Metadata: model.Metadata{
			Source:         legacySource,
			LegacyKind:     "player",
			LegacyID:       displayName,
			LegacyPath:     strings.Join([]string{"player", shardName, displayName}, "/"),
			LegacyEncoding: encoding,
			RawFields: map[string][]byte{
				"filename": []byte(name),
			},
		},
		Warnings: warnings,
	}
}

func decodePathComponent(component, field string) (string, string, []string) {
	if component == "" {
		return "", "utf-8", nil
	}
	decoded, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Field: field}, []byte(component))
	if err == nil {
		if decoded == component {
			return decoded, "utf-8", nil
		}
		return decoded, "euc-kr/cp949", nil
	}

	fallback := "raw-" + hex.EncodeToString([]byte(component))
	return fallback, "invalid", []string{fmt.Sprintf("%s decode failed: %v", field, err)}
}

func displayRelPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		decoded, _, _ := decodePathComponent(part, "path")
		parts[i] = decoded
	}
	return strings.Join(parts, "/")
}
