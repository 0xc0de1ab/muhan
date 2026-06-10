package table

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"muhan/internal/commandspec/extract"
	"muhan/internal/engine/command"
)

const fixtureSource = `
struct {
	char *cmdstr;
	int cmdno;
	int (*cmdfn)();
} cmdlist[] = {
	{ "봐", 2, look },
	{ "보다", 2, look },
	{ "보아", 100, action },
	{ "조사", 2, look },
	{ "말", 4, say },
	{ "때려", 23, attack },
	{ "*순간이동", 101, dm_teleport },
	{ "눌러", -2, 0 },
	{ "@", 0, 0 }
};
`

func TestLoadLegacyRegistryResolvesFixtureCommands(t *testing.T) {
	registry, entries, err := LoadLegacyRegistry(writeLegacyRoot(t, fixtureSource))
	if err != nil {
		t.Fatalf("LoadLegacyRegistry() error = %v", err)
	}
	if len(entries) != 8 {
		t.Fatalf("entries = %d, want 8", len(entries))
	}

	t.Run("exact", func(t *testing.T) {
		got, err := command.ParseAndResolve("대상 때려", registry)
		if err != nil {
			t.Fatalf("ParseAndResolve() error = %v", err)
		}
		if got.Command() != "때려" || got.Spec.Name != "때려" || !got.Exact {
			t.Fatalf("resolved = %+v, want exact 때려", got)
		}
		if len(got.Args) != 1 || got.Args[0] != "대상" || len(got.Values) != 1 || got.Values[0] != 1 {
			t.Fatalf("args/values = %#v/%#v, want 대상/1", got.Args, got.Values)
		}
	})

	t.Run("look aliases", func(t *testing.T) {
		for _, alias := range []string{"봐", "보다", "조사"} {
			t.Run(alias, func(t *testing.T) {
				got, err := command.ParseAndResolve("대상 2 "+alias, registry)
				if err != nil {
					t.Fatalf("ParseAndResolve() error = %v", err)
				}
				if got.Command() != alias || got.Spec.Name != alias || got.Spec.Number != 2 || got.Spec.Handler != "look" || !got.Exact {
					t.Fatalf("resolved = %+v, want exact %s/2/look", got, alias)
				}
				if !slices.Equal(got.Args, []string{"대상"}) || !slices.Equal(got.Values, []int64{2}) {
					t.Fatalf("args/values = %#v/%#v, want 대상/2", got.Args, got.Values)
				}
			})
		}
	})

	t.Run("look prefix and action exact", func(t *testing.T) {
		got, err := command.ParseAndResolve("대상 보", registry)
		if err != nil {
			t.Fatalf("ParseAndResolve() error = %v", err)
		}
		if got.Spec.Name != "보다" || got.Spec.Number != 2 || got.Spec.Handler != "look" || got.Exact {
			t.Fatalf("resolved = %+v, want prefix 보다/2/look", got)
		}

		got, err = command.ParseAndResolve("대상 보아", registry)
		if err != nil {
			t.Fatalf("ParseAndResolve() error = %v", err)
		}
		if got.Spec.Name != "보아" || got.Spec.Number != 100 || got.Spec.Handler != "action" || !got.Exact {
			t.Fatalf("resolved = %+v, want exact 보아/100/action", got)
		}
	})

	t.Run("prefix", func(t *testing.T) {
		got, err := command.ParseAndResolve("대상 때", registry)
		if err != nil {
			t.Fatalf("ParseAndResolve() error = %v", err)
		}
		if got.Command() != "때" || got.Spec.Name != "때려" || got.Exact {
			t.Fatalf("resolved = %+v, want prefix 때려", got)
		}
	})

	t.Run("default verb", func(t *testing.T) {
		got, err := command.ParseAndResolve("대상.", registry)
		if err != nil {
			t.Fatalf("ParseAndResolve() error = %v", err)
		}
		if got.Command() != "말" || got.Spec.Name != "말" || !got.Exact {
			t.Fatalf("resolved = %+v, want default verb 말", got)
		}
	})

	t.Run("privileged deny", func(t *testing.T) {
		_, err := command.ParseAndResolveWithOptions("*순", registry, command.DenyPrivileged())
		if err == nil {
			t.Fatal("ParseAndResolveWithOptions() error = nil, want privileged denial")
		}
		if !errors.Is(err, command.ErrPrivilegedCommand) {
			t.Fatalf("error = %v, want ErrPrivilegedCommand", err)
		}
	})
}

func TestAnalyzeReportsNameIssuesAndSummaries(t *testing.T) {
	entries := []extract.Entry{
		{Name: "", Number: 1, Handler: "empty"},
		{Name: string([]byte{0xff}), Number: 2, Handler: "invalid"},
		{Name: "깨짐" + string(runeError()), Number: 3, Handler: "replacement"},
		{Name: "중복", Number: 4, Handler: "first"},
		{Name: "중복", Number: 5, Handler: "second"},
		{Name: "*관리", Number: 101, Handler: "dm", Privileged: true},
		{Name: "눌러", Number: 2, Handler: "0", Special: true},
	}

	got := Analyze(entries)
	if got.Count != len(entries) {
		t.Fatalf("Count = %d, want %d", got.Count, len(entries))
	}
	if len(got.EmptyNames) != 1 || got.EmptyNames[0].Reason != "empty" {
		t.Fatalf("EmptyNames = %+v, want one empty issue", got.EmptyNames)
	}
	if len(got.BrokenNames) != 2 {
		t.Fatalf("BrokenNames = %+v, want invalid UTF-8 and replacement rune", got.BrokenNames)
	}
	if len(got.DuplicateNames) != 1 || got.DuplicateNames[0].Name != "중복" || len(got.DuplicateNames[0].Occurrences) != 2 {
		t.Fatalf("DuplicateNames = %+v, want duplicated 중복", got.DuplicateNames)
	}
	if got.Privileged.Count != 1 || got.Privileged.Entries[0].Name != "*관리" {
		t.Fatalf("Privileged = %+v, want *관리 summary", got.Privileged)
	}
	if got.Special.Count != 1 || got.Special.Entries[0].Name != "눌러" {
		t.Fatalf("Special = %+v, want 눌러 summary", got.Special)
	}
	if !got.HasIssues() {
		t.Fatal("HasIssues() = false, want true")
	}
}

func TestLoadLegacyRegistryFromRepositoryGlobalC(t *testing.T) {
	registry, entries, err := LoadLegacyRegistry(repoRoot(t))
	if err != nil {
		t.Fatalf("LoadLegacyRegistry(repo root) error = %v", err)
	}
	if len(entries) != 373 {
		t.Fatalf("entries = %d, want 373", len(entries))
	}

	analysis := Analyze(entries)
	if len(analysis.EmptyNames) != 0 || len(analysis.BrokenNames) != 0 {
		t.Fatalf("name issues = empty:%+v broken:%+v, want none", analysis.EmptyNames, analysis.BrokenNames)
	}

	got, err := command.ParseAndResolve("대상 때려", registry)
	if err != nil {
		t.Fatalf("ParseAndResolve(real registry) error = %v", err)
	}
	if got.Spec.Name != "때려" || got.Spec.Number != 23 {
		t.Fatalf("resolved = %+v, want command 23 때려", got)
	}

	for _, tt := range []struct {
		input   string
		name    string
		number  int
		handler string
		args    []string
		values  []int64
	}{
		{input: "대상 봐", name: "봐", number: 2, handler: "look", args: []string{"대상"}, values: []int64{1}},
		{input: "대상 보", name: "보다", number: 2, handler: "look", args: []string{"대상"}, values: []int64{1}},
		{input: "대상 보다", name: "보다", number: 2, handler: "look", args: []string{"대상"}, values: []int64{1}},
		{input: "대상 조사", name: "조사", number: 2, handler: "look", args: []string{"대상"}, values: []int64{1}},
		{input: "동 봐", name: "봐", number: 2, handler: "look", args: []string{"동"}, values: []int64{1}},
		{input: "동 2 봐", name: "봐", number: 2, handler: "look", args: []string{"동"}, values: []int64{2}},
		{input: "동 보다", name: "보다", number: 2, handler: "look", args: []string{"동"}, values: []int64{1}},
		{input: "대상 보아", name: "보아", number: 100, handler: "action"},
		{input: "사과 주워", name: "주워", number: 5, handler: "get"},
		{input: "사과 주", name: "주", number: 5, handler: "get"},
		{input: "사과 가져", name: "가져", number: 5, handler: "get"},
		{input: "가방 사과 꺼내", name: "꺼내", number: 5, handler: "get"},
		{input: "사과 버려", name: "버려", number: 7, handler: "drop"},
		{input: "사과 가방 넣어", name: "넣어", number: 7, handler: "drop"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, err := command.ParseAndResolve(tt.input, registry)
			if err != nil {
				t.Fatalf("ParseAndResolve(%q) error = %v", tt.input, err)
			}
			wantExact := got.Command() == tt.name
			if got.Spec.Name != tt.name || got.Spec.Number != tt.number || got.Spec.Handler != tt.handler || got.Exact != wantExact {
				t.Fatalf("resolved = %+v, want %s/%d/%s exact=%v", got, tt.name, tt.number, tt.handler, wantExact)
			}
			if tt.args != nil && (!slices.Equal(got.Args, tt.args) || !slices.Equal(got.Values, tt.values)) {
				t.Fatalf("args/values = %#v/%#v, want %#v/%#v", got.Args, got.Values, tt.args, tt.values)
			}
		})
	}
}

func writeLegacyRoot(t *testing.T, source string) string {
	t.Helper()

	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatalf("Mkdir(src) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "global.c"), []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile(global.c) error = %v", err)
	}
	return root
}

func repoRoot(t *testing.T) string {
	t.Helper()
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("skipping test requiring legacy src/ in CI or short mode")
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}

func runeError() rune {
	return '\uFFFD'
}
