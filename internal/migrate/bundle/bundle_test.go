package bundle

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"muhan/internal/persist/cbin"
)

const testCommandSource = `
struct {
	char *cmdstr;
	int cmdno;
	int (*cmdfn)();
} cmdlist[] = {
	{ "look", 2, look },
	{ "*teleport", 101, dm_teleport },
	{ "push", -2, 0 },
	{ "@", 0, 0 }
};
`

func TestBuildSummarizesComponents(t *testing.T) {
	root := newBundleRoot(t)
	writeFile(t, filepath.Join(root, "objmon", "o00"), make([]byte, cbin.ObjectSize))
	writeFile(t, filepath.Join(root, "rooms", "r00", "r00001"), minimalRoomData(1, 0))

	generatedAt := time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)
	got, err := build(root, generatedAt)
	if err != nil {
		t.Fatal(err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Root != absRoot {
		t.Fatalf("Root = %q, want %q", got.Root, absRoot)
	}
	if !got.GeneratedAt.Equal(generatedAt) {
		t.Fatalf("GeneratedAt = %s, want %s", got.GeneratedAt, generatedAt)
	}
	if got.CommandRegistry.Count != 3 || got.CommandRegistry.RegistryCount != 3 ||
		got.CommandRegistry.Privileged != 1 || got.CommandRegistry.Special != 1 {
		t.Fatalf("CommandRegistry = %+v", got.CommandRegistry)
	}
	if got.PrototypeCounts.ObjectPrototypeFiles != 1 ||
		got.PrototypeCounts.ObjectPrototypes != 1 ||
		got.PrototypeCounts.TotalPrototypes != 1 {
		t.Fatalf("PrototypeCounts = %+v", got.PrototypeCounts)
	}
	if got.RoomMap.Files != 1 || got.RoomMap.MappedRooms != 1 || got.RoomMap.Errors != 0 {
		t.Fatalf("RoomMap = %+v", got.RoomMap)
	}
	if got.PlayerMap.Counts.PlayerFiles != 0 || got.PlayerMap.Counts.Errors != 0 {
		t.Fatalf("PlayerMap = %+v", got.PlayerMap)
	}
	if got.ObjectTotals.ObjectPrototypes != 1 || got.ObjectTotals.LegacyObjectRecords != 1 {
		t.Fatalf("ObjectTotals = %+v", got.ObjectTotals)
	}
	if got.RepairActions.Total != 0 {
		t.Fatalf("RepairActions = %+v", got.RepairActions)
	}
	if got.FindingCounts.Warnings != 0 || got.FindingCounts.Errors != 0 {
		t.Fatalf("FindingCounts = %+v", got.FindingCounts)
	}
}

func TestBuildKeepsDamagedRoomAsSummaryError(t *testing.T) {
	root := newBundleRoot(t)
	writeFile(t, filepath.Join(root, "rooms", "r00", "r00001"), minimalRoomData(1, 0))
	writeFile(t, filepath.Join(root, "rooms", "r00", "r00002"), []byte{1, 2, 3})

	got, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}

	if got.RoomMap.Files != 2 || got.RoomMap.MappedRooms != 1 || got.RoomMap.Errors != 1 {
		t.Fatalf("RoomMap = %+v", got.RoomMap)
	}
	if got.RepairActions.Total == 0 {
		t.Fatalf("RepairActions = %+v, want damaged room action", got.RepairActions)
	}
	if got.FindingCounts.Errors == 0 {
		t.Fatalf("FindingCounts = %+v, want nonfatal error count", got.FindingCounts)
	}
}

func TestEncodeJSON(t *testing.T) {
	b := Bundle{
		Root:        "/tmp/muhan",
		GeneratedAt: time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC),
		CommandRegistry: CommandRegistrySummary{
			Count: 1,
		},
		FindingCounts: FindingCounts{
			Components: []ComponentFindingCount{{Name: "roommap", Errors: 1}},
			Errors:     1,
		},
	}

	var buf bytes.Buffer
	if err := EncodeJSON(&buf, b); err != nil {
		t.Fatal(err)
	}

	var decoded Bundle
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Root != b.Root || decoded.CommandRegistry.Count != 1 ||
		decoded.FindingCounts.Errors != 1 {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func newBundleRoot(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for _, dir := range []string{
		filepath.Join(root, "src"),
		filepath.Join(root, "objmon"),
		filepath.Join(root, "rooms", "r00"),
		filepath.Join(root, "player"),
		filepath.Join(root, "board"),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(root, "src", "global.c"), []byte(testCommandSource))
	return root
}

func minimalRoomData(number int, exitCount int) []byte {
	data := make([]byte, cbin.RoomSize)
	binary.LittleEndian.PutUint16(data, uint16(int16(number)))
	data = appendInt32(data, exitCount)
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	return data
}

func appendInt32(data []byte, n int) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(int32(n)))
	return append(data, buf[:]...)
}

func appendDescription(data []byte, desc []byte) []byte {
	data = appendInt32(data, len(desc))
	return append(data, desc...)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
