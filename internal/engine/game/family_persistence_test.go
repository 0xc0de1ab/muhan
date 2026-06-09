package game

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"muhan/internal/persist/legacykr"
)

func TestPersistFamilyMemberClassChangeUpdatesLegacyFileAsUTF8(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "player", "family", "family_member_2")
	raw, err := legacykr.EncodeEUCKR("4 홍길동\n5 Bob\n0 무영문\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	members, err := PersistFamilyMemberClassChange(root, 2, "무영문", "홍길동", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("members len = %d, want 2", len(members))
	}
	if members[0].DisplayName != "홍길동" || members[0].Class != 8 {
		t.Fatalf("updated member = %+v, want 홍길동 class 8", members[0])
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(written) {
		t.Fatalf("family_member file should be rewritten as UTF-8: % X", written)
	}
	if !bytes.Contains(written, []byte("8 홍길동\n")) {
		t.Fatalf("updated class line missing from file:\n%s", string(written))
	}
	if bytes.Contains(written, []byte("4 홍길동\n")) {
		t.Fatalf("old class line still present:\n%s", string(written))
	}
	verifyTestFamilyMemberFile(t, path, []legacyMember{{classID: 8, name: "홍길동"}, {classID: 5, name: "Bob"}}, "무영문")
}

func TestPersistFamilyMemberLeaveRemovesMemberForSuicidePath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "player", "family", "family_member_2")
	if err := writeFamilyMembersFile(path, []legacyMember{
		{classID: 10, name: "Alice"},
		{classID: 4, name: "인제로"},
	}, "무영문"); err != nil {
		t.Fatal(err)
	}

	members, err := PersistFamilyMemberLeave(root, 2, "무영문", "인제로")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].DisplayName != "Alice" {
		t.Fatalf("members after leave = %+v, want only Alice", members)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(written) {
		t.Fatalf("family_member file should stay UTF-8: % X", written)
	}
	if bytes.Contains(written, []byte("인제로")) {
		t.Fatalf("removed member still present:\n%s", string(written))
	}
	if !bytes.Contains(written, []byte("0 무영문\n")) {
		t.Fatalf("family sentinel missing:\n%s", string(written))
	}
	verifyTestFamilyMemberFile(t, path, []legacyMember{{classID: 10, name: "Alice"}}, "무영문")
}

func TestPersistFamilyMemberMissingFileNoopsForLeaveAndClassChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "player", "family", "family_member_2")

	members, err := PersistFamilyMemberLeave(root, 2, "무영문", "Nobody")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("leave missing members = %+v, want empty", members)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("leave on missing file should not create file, stat err=%v", err)
	}

	members, err = PersistFamilyMemberClassChange(root, 2, "무영문", "Nobody", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("class change missing members = %+v, want empty", members)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("class change on missing file should not create file, stat err=%v", err)
	}
}
