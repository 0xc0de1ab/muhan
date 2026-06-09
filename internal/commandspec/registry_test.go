package commandspec

import "testing"

func TestResolvePrefersExactMatch(t *testing.T) {
	reg := mustRegistry(t, []CommandSpec{
		{Name: "도움말", Number: 14},
		{Name: "도", Number: 37},
	})

	match, ok := reg.Resolve("도")
	if !ok {
		t.Fatal("Resolve did not match")
	}
	if match.Command.Name != "도" || !match.Exact {
		t.Fatalf("Resolve(%q) = %+v, want exact 도", "도", match)
	}
}

func TestResolveUsesTableOrderForPrefixTies(t *testing.T) {
	reg := mustRegistry(t, []CommandSpec{
		{Name: "동굴", Number: 201},
		{Name: "동쪽", Number: 202},
		{Name: "동", Number: 1},
	})

	match, ok := reg.Resolve("동")
	if !ok {
		t.Fatal("Resolve did not match")
	}
	if match.Command.Name != "동" || !match.Exact {
		t.Fatalf("Resolve(%q) = %+v, want exact 동", "동", match)
	}

	reg = mustRegistry(t, []CommandSpec{
		{Name: "동굴", Number: 201},
		{Name: "동쪽", Number: 202},
	})

	match, ok = reg.Resolve("동")
	if !ok {
		t.Fatal("Resolve did not match")
	}
	if match.Command.Name != "동굴" || match.Index != 0 || match.Exact {
		t.Fatalf("Resolve(%q) = %+v, want first prefix 동굴", "동", match)
	}
}

func TestResolveHandlesUTF8Prefixes(t *testing.T) {
	reg := mustRegistry(t, []CommandSpec{
		{Name: "무장", Number: 13},
		{Name: "무적수련", Number: 156},
		{Name: "ㅂ", Number: 1},
	})

	tests := []struct {
		input string
		want  string
	}{
		{input: "무", want: "무장"},
		{input: "무적", want: "무적수련"},
		{input: "ㅂ", want: "ㅂ"},
	}

	for _, tt := range tests {
		match, ok := reg.Resolve(tt.input)
		if !ok {
			t.Fatalf("Resolve(%q) did not match", tt.input)
		}
		if match.Command.Name != tt.want {
			t.Fatalf("Resolve(%q) = %q, want %q", tt.input, match.Command.Name, tt.want)
		}
	}
}

func TestResolveRejectsPartialUTF8Bytes(t *testing.T) {
	reg := mustRegistry(t, []CommandSpec{
		{Name: "무장", Number: 13},
	})

	if _, ok := reg.Resolve(string([]byte{0xeb, 0xac})); ok {
		t.Fatal("Resolve matched an invalid partial UTF-8 prefix")
	}
}

func TestCommandMetadataIsPreserved(t *testing.T) {
	reg := mustRegistry(t, []CommandSpec{
		{Name: "*순간이동", Number: 101, Handler: "dm_teleport", Privileged: true},
		{Name: "눌러", Number: 2, Handler: "0", Special: true},
	})

	match, ok := reg.Resolve("*순")
	if !ok {
		t.Fatal("Resolve did not match privileged command")
	}
	if !match.Command.Privileged || match.Command.Special || match.Command.Handler != "dm_teleport" {
		t.Fatalf("Resolve privileged metadata = %+v", match.Command)
	}

	match, ok = reg.Resolve("눌")
	if !ok {
		t.Fatal("Resolve did not match special command")
	}
	if !match.Command.Special || match.Command.Number != 2 || match.Command.Handler != "0" {
		t.Fatalf("Resolve special metadata = %+v", match.Command)
	}
}

func mustRegistry(t *testing.T, specs []CommandSpec) Registry {
	t.Helper()

	reg, err := NewRegistry(specs)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return reg
}
