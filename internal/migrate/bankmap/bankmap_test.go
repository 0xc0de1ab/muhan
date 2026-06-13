package bankmap

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

func TestBuildMapsPlayerBankMinimalObject(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "player", "bank")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "owner"), testObjectTree("coin"), 0600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Build(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	if snapshot.Counts.PlayerBanks != 1 || snapshot.Counts.FamilyBanks != 0 ||
		snapshot.Counts.TotalBanks != 1 || snapshot.Counts.Objects != 1 {
		t.Fatalf("counts = %+v", snapshot.Counts)
	}
	if len(snapshot.Errors) != 0 || len(snapshot.Warnings) != 0 {
		t.Fatalf("findings = warnings %+v errors %+v", snapshot.Warnings, snapshot.Errors)
	}
	if len(snapshot.Banks) != 1 {
		t.Fatalf("banks len = %d, want 1", len(snapshot.Banks))
	}

	got := snapshot.Banks[0]
	if got.ID != "bank:player:owner" || got.Kind != KindPlayer || got.OwnerName != "owner" {
		t.Fatalf("bank identity = %+v", got)
	}
	if got.Path != "player/bank/owner" || got.ObjectCount != 1 || got.TrailingBytes != 0 {
		t.Fatalf("bank record = %+v", got)
	}
}

func TestBuildAllowsTrailingBankBytes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "player", "bank")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := append(testObjectTree("safe"), []byte{0xde, 0xad, 0xbe, 0xef}...)
	if err := os.WriteFile(filepath.Join(dir, "owner"), data, 0600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Build(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	if len(snapshot.Errors) != 0 {
		t.Fatalf("errors = %+v", snapshot.Errors)
	}
	if len(snapshot.Banks) != 1 || snapshot.Banks[0].TrailingBytes != 4 {
		t.Fatalf("banks = %+v", snapshot.Banks)
	}
	if snapshot.Counts.TrailingBytes != 4 || snapshot.Counts.Objects != 1 {
		t.Fatalf("counts = %+v", snapshot.Counts)
	}
}

func TestBuildCanIncludeBankObjects(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "player", "bank")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "owner"), testObjectTree("box", testObjectTree("gem")), 0600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Build(Options{Root: root, IncludeObjects: true})
	if err != nil {
		t.Fatal(err)
	}

	if len(snapshot.Objects) != 2 {
		t.Fatalf("objects len = %d", len(snapshot.Objects))
	}
	if len(snapshot.Banks) != 1 || len(snapshot.Banks[0].ObjectIDs) != 1 {
		t.Fatalf("banks = %+v", snapshot.Banks)
	}
	account := snapshot.Banks[0].BankAccount()
	if account.ID != model.BankID("bank:player:owner") || account.OwnerPlayerID != model.PlayerID("owner") {
		t.Fatalf("account = %+v", account)
	}
	rootID := snapshot.Banks[0].ObjectIDs[0]
	if snapshot.Objects[0].ID != rootID || snapshot.Objects[0].Location.BankID != account.ID {
		t.Fatalf("root object = %+v", snapshot.Objects[0])
	}
	if snapshot.Objects[1].Location.ContainerID != rootID {
		t.Fatalf("child object = %+v, want container %q", snapshot.Objects[1], rootID)
	}
}

func TestBuildDecodesEUCKRFamilyBankFilename(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "player", "family", "bank")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	nameBytes, err := legacykr.EncodeEUCKR("무한문파")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, string(nameBytes)), testObjectTree("box"), 0600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Build(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	if snapshot.Counts.PlayerBanks != 0 || snapshot.Counts.FamilyBanks != 1 ||
		snapshot.Counts.TotalBanks != 1 || snapshot.Counts.Objects != 1 {
		t.Fatalf("counts = %+v", snapshot.Counts)
	}
	if len(snapshot.Errors) != 0 || len(snapshot.Warnings) != 0 {
		t.Fatalf("findings = warnings %+v errors %+v", snapshot.Warnings, snapshot.Errors)
	}
	got := snapshot.Banks[0]
	if got.ID != "bank:family:무한문파" || got.Kind != KindFamily || got.OwnerName != "무한문파" {
		t.Fatalf("bank identity = %+v", got)
	}
	if got.Path != "player/family/bank/무한문파" {
		t.Fatalf("path = %q", got.Path)
	}
}

func TestBuildReportsObjectTreeWarning(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "player", "bank")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := testObjectTree("")
	data[0] = 0xff
	if err := os.WriteFile(filepath.Join(dir, "owner"), data, 0600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Build(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	if len(snapshot.Errors) != 0 {
		t.Fatalf("errors = %+v", snapshot.Errors)
	}
	if len(snapshot.Warnings) != 1 || !strings.Contains(snapshot.Warnings[0].Message, "object.name") {
		t.Fatalf("warnings = %+v", snapshot.Warnings)
	}
	if len(snapshot.Banks) != 1 || len(snapshot.Banks[0].Warnings) != 1 {
		t.Fatalf("bank warnings = %+v", snapshot.Banks)
	}
}

func testObjectTree(name string, children ...[]byte) []byte {
	data := make([]byte, cbin.ObjectSize)
	copy(data, []byte(name+"\x00"))
	var count [4]byte
	binary.LittleEndian.PutUint32(count[:], uint32(len(children)))
	data = append(data, count[:]...)
	for _, child := range children {
		data = append(data, child...)
	}
	return data
}
