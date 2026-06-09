package invitemap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/persist/legacykr"
)

func TestMapInviteFileDecodesEUCKRNames(t *testing.T) {
	rawName, err := legacykr.EncodeEUCKR("무한")
	if err != nil {
		t.Fatal(err)
	}
	data := append([]byte{}, rawName...)
	data = append(data, '\n')
	data = append(data, []byte("guest\n0\nignored\n")...)

	got, err := MapInviteFile(filepath.Join("player", "invite", "invite_7"), data)
	if err != nil {
		t.Fatal(err)
	}

	if got.Number != 7 || got.ID != "invite:7" {
		t.Fatalf("identity = number %d id %q", got.Number, got.ID)
	}
	if strings.Join(got.Names, ",") != "무한,guest" {
		t.Fatalf("names = %q", got.Names)
	}
	if len(got.RawNames) != 2 || string(got.RawNames[0]) != string(rawName) {
		t.Fatalf("raw names = % X, want first % X", got.RawNames, rawName)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %+v", got.Warnings)
	}
}

func TestMapInviteFileUsesFirstTenNames(t *testing.T) {
	data := []byte("a b c d e f g h i j k 0")

	got, err := MapInviteFile("invite_99", data)
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Names) != MaxInviteNames {
		t.Fatalf("names len = %d, want %d: %q", len(got.Names), MaxInviteNames, got.Names)
	}
	if got.Names[0] != "a" || got.Names[9] != "j" {
		t.Fatalf("names = %q", got.Names)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0].Message, "more than 10 names") {
		t.Fatalf("warnings = %+v", got.Warnings)
	}
}

func TestScanDirMapsInviteFiles(t *testing.T) {
	dir := t.TempDir()
	rawName, err := legacykr.EncodeEUCKR("무한")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "invite_42"), append(rawName, []byte("\n0\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "memo"), []byte("skip"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "invite_43"), 0700); err != nil {
		t.Fatal(err)
	}

	report, err := ScanDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if report.Counts.Files != 1 || report.Counts.MappedFiles != 1 || report.Counts.Names != 1 || report.Counts.SkippedFiles != 2 {
		t.Fatalf("counts = %+v", report.Counts)
	}
	if report.Counts.Errors != 0 || len(report.Errors) != 0 {
		t.Fatalf("errors = %+v", report.Errors)
	}
	if len(report.Invites) != 1 {
		t.Fatalf("invites len = %d", len(report.Invites))
	}
	if report.Invites[0].Number != 42 || strings.Join(report.Invites[0].Names, ",") != "무한" {
		t.Fatalf("invite = %+v", report.Invites[0])
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0].Message, "directory") {
		t.Fatalf("warnings = %+v", report.Warnings)
	}
}
