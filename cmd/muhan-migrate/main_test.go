package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"muhan/internal/persist/cbin"
)

func TestEncodeManifestJSON(t *testing.T) {
	generatedAt := time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)
	manifest := Manifest{
		Root:        "/legacy/muhan",
		GeneratedAt: generatedAt,
		Counts: Counts{
			LegacyRooms:              2,
			LegacyPlayers:            3,
			LegacyBanks:              4,
			LegacyBoards:             5,
			RoomFiles:                2,
			PlayerFiles:              3,
			BankFiles:                4,
			BoardIndexFiles:          1,
			BoardIndexRecords:        5,
			BoardPostFiles:           6,
			ObjectPrototypeFiles:     7,
			ObjectPrototypeRecords:   8,
			CreaturePrototypeFiles:   9,
			CreaturePrototypeRecords: 10,
			SkippedFiles:             11,
			Names: NameCounts{
				PlayerNameCandidates:   3,
				UTF8PlayerNames:        2,
				LegacyKRPlayerNames:    1,
				InvalidPlayerNames:     1,
				PlayerBucketMismatches: 1,
			},
			Decoded: cbin.Stats{
				Rooms:         2,
				Creatures:     3,
				Objects:       12,
				Exits:         4,
				TrailingBytes: 5,
				MaxDepth:      6,
			},
		},
		Warnings: []Finding{{Path: "/legacy/muhan/player", Message: "legacykr pending"}},
		Errors:   []Finding{{Path: "/legacy/muhan/rooms/r00/r00000", Message: "decode failed"}},
	}

	var buf bytes.Buffer
	if err := encodeManifest(&buf, manifest); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}

	var got Manifest
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}

	if got.Root != manifest.Root {
		t.Fatalf("root = %q, want %q", got.Root, manifest.Root)
	}
	if !got.GeneratedAt.Equal(generatedAt) {
		t.Fatalf("generatedAt = %s, want %s", got.GeneratedAt, generatedAt)
	}
	if got.Counts.LegacyRooms != 2 || got.Counts.LegacyPlayers != 3 ||
		got.Counts.LegacyBanks != 4 || got.Counts.LegacyBoards != 5 {
		t.Fatalf("legacy counts = %+v", got.Counts)
	}
	if got.Counts.Names.LegacyKRPlayerNames != 1 || got.Counts.Names.InvalidPlayerNames != 1 {
		t.Fatalf("name planning counts = %+v", got.Counts.Names)
	}
	if got.Counts.Decoded.Objects != 12 || got.Counts.Decoded.TrailingBytes != 5 {
		t.Fatalf("decoded stats = %+v", got.Counts.Decoded)
	}
	if len(got.Warnings) != 1 || len(got.Errors) != 1 {
		t.Fatalf("warnings/errors = %d/%d", len(got.Warnings), len(got.Errors))
	}
}

func TestLegacyPlayerNameHook(t *testing.T) {
	if converted, legacy, err := legacyPlayerNameToUTF8("가나다"); err != nil || legacy || converted != "가나다" {
		t.Fatalf("valid UTF-8 conversion = %q legacy=%v err=%v", converted, legacy, err)
	}

	converted, legacy, err := legacyPlayerNameToUTF8(string([]byte{0xb0, 0xa1}))
	if err != nil {
		t.Fatal(err)
	}
	if !legacy {
		t.Fatal("legacy EUC-KR name was not marked legacy")
	}
	if converted != "가" {
		t.Fatalf("legacy conversion = %q, want 가", converted)
	}
}
