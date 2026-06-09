package extract

import (
	"os"
	"path/filepath"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/persist/legacykr"
)

const fixtureSource = `
struct {
	char *cmdstr;
	int cmdno;
	int (*cmdfn)();
} cmdlist[] = {
	{ "도움말", 14, help },
	/* { "비활성", 99, disabled }, */
	{ "도", 37, go },
	{
		"눌러", -2, 0
	},
	{ "*teleport", 101, dm_teleport },
	{ "\"", 4, say },
	{ "@", 0, 0 }
};
`

func TestExtractSourceSkipsCommentsAndSentinel(t *testing.T) {
	entries, err := ExtractSource(fixtureSource)
	if err != nil {
		t.Fatalf("ExtractSource() error = %v", err)
	}

	want := []Entry{
		{Name: "도움말", Number: 14, Handler: "help"},
		{Name: "도", Number: 37, Handler: "go"},
		{Name: "눌러", Number: 2, Handler: "0", Special: true},
		{Name: "*teleport", Number: 101, Handler: "dm_teleport", Privileged: true},
		{Name: "\"", Number: 4, Handler: "say"},
	}
	if len(entries) != len(want) {
		t.Fatalf("ExtractSource() got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Fatalf("entry %d = %+v, want %+v", i, entries[i], want[i])
		}
	}
}

func TestExtractedCommandSpecsResolveExactAndPrefix(t *testing.T) {
	entries, err := ExtractSource(fixtureSource)
	if err != nil {
		t.Fatalf("ExtractSource() error = %v", err)
	}
	reg, err := commandspec.NewRegistry(CommandSpecs(entries))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	match, ok := reg.Resolve("도")
	if !ok {
		t.Fatal("Resolve exact command did not match")
	}
	if match.Command.Name != "도" || !match.Exact {
		t.Fatalf("Resolve(%q) = %+v, want exact 도", "도", match)
	}
	if match.Command.Handler != "go" {
		t.Fatalf("Resolve(%q) handler = %q, want go", "도", match.Command.Handler)
	}

	match, ok = reg.Resolve("도움")
	if !ok {
		t.Fatal("Resolve prefix command did not match")
	}
	if match.Command.Name != "도움말" || match.Exact {
		t.Fatalf("Resolve(%q) = %+v, want prefix 도움말", "도움", match)
	}
	if match.Command.Handler != "help" {
		t.Fatalf("Resolve(%q) handler = %q, want help", "도움", match.Command.Handler)
	}
}

func TestExtractRootPreservesRealHandlerThroughRegistry(t *testing.T) {
	root := testRoot(t)
	entries, err := ExtractRoot(root)
	if err != nil {
		t.Fatalf("ExtractRoot(%q) error = %v", root, err)
	}

	var help Entry
	for _, entry := range entries {
		if entry.Name == "도움말" {
			help = entry
			break
		}
	}
	if help == (Entry{}) {
		t.Fatal("real cmdlist did not include 도움말")
	}
	if help.Number != 14 || help.Handler != "help" || help.Privileged || help.Special {
		t.Fatalf("real 도움말 entry = %+v, want number 14 handler help", help)
	}

	reg, err := commandspec.NewRegistry(CommandSpecs(entries))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	match, ok := reg.Resolve("도움")
	if !ok {
		t.Fatal("Resolve real 도움 prefix did not match")
	}
	if match.Command.Name != "도움말" || match.Command.Handler != "help" || match.Exact {
		t.Fatalf("Resolve(%q) = %+v, want prefix 도움말 with handler help", "도움", match)
	}
}

func TestExtractBytesDecodesLegacyKorean(t *testing.T) {
	data, err := legacykr.EncodeEUCKR(fixtureSource)
	if err != nil {
		t.Fatalf("EncodeEUCKR() error = %v", err)
	}

	entries, err := ExtractBytes("fixture.c", data)
	if err != nil {
		t.Fatalf("ExtractBytes() error = %v", err)
	}
	if entries[0].Name != "도움말" {
		t.Fatalf("decoded first command = %q, want 도움말", entries[0].Name)
	}
}

func TestExtractBytesPreservesUTF8IslandInLegacySource(t *testing.T) {
	prefix, err := legacykr.EncodeEUCKR(`struct { char *cmdstr; int cmdno; int (*cmdfn)(); } cmdlist[] = { { "`)
	if err != nil {
		t.Fatalf("EncodeEUCKR(prefix) error = %v", err)
	}
	suffix, err := legacykr.EncodeEUCKR(`", 1, move }, { "@", 0, 0 } };`)
	if err != nil {
		t.Fatalf("EncodeEUCKR(suffix) error = %v", err)
	}
	data := append(prefix, []byte("ɪ")...)
	data = append(data, suffix...)

	entries, err := ExtractBytes("mixed.c", data)
	if err != nil {
		t.Fatalf("ExtractBytes() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "ɪ" {
		t.Fatalf("entries = %+v, want one UTF-8 island command", entries)
	}
}

func testRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "src", "global.c")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find test root containing go.mod and src/global.c")
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
