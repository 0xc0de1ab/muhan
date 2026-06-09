package command

import (
	"errors"
	"slices"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
)

func TestParseAndResolveExactUTF8Command(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "때려", Number: 23},
	})

	got, err := ParseAndResolve("고블린 때려", registry)
	if err != nil {
		t.Fatalf("ParseAndResolve() error = %v", err)
	}

	if got.Input != "고블린 때려" {
		t.Fatalf("Input = %q, want original input", got.Input)
	}
	if got.Command() != "때려" || got.Spec.Name != "때려" || !got.Exact {
		t.Fatalf("resolved command = %+v, want exact 때려", got)
	}
	if !slices.Equal(got.Args, []string{"고블린"}) {
		t.Fatalf("Args = %#v, want 고블린", got.Args)
	}
	if !slices.Equal(got.Values, []int64{1}) {
		t.Fatalf("Values = %#v, want 1", got.Values)
	}
}

func TestParseAndResolveLowercasesASCIISlotsLikeLegacy(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "look", Number: 2, Handler: "look"},
	})

	tests := []struct {
		input string
		args  []string
	}{
		{input: "TARGET LOOK", args: []string{"target"}},
		{input: "LOOK TARGET", args: []string{"target"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseAndResolve(tt.input, registry)
			if err != nil {
				t.Fatalf("ParseAndResolve(%q) error = %v", tt.input, err)
			}
			if got.Input != tt.input {
				t.Fatalf("Input = %q, want original %q", got.Input, tt.input)
			}
			if got.Command() != "look" || got.Spec.Name != "look" {
				t.Fatalf("resolved = %+v, want lowercase look command slot", got)
			}
			if !slices.Equal(got.Args, tt.args) {
				t.Fatalf("Args = %#v, want %#v", got.Args, tt.args)
			}
		})
	}
}

func TestParseAndResolveDefaultVerbByPrefix(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "말", Number: 4},
	})

	got, err := ParseAndResolve("고블린.", registry)
	if err != nil {
		t.Fatalf("ParseAndResolve() error = %v", err)
	}

	if got.Command() != commandparse.DefaultVerb {
		t.Fatalf("Command() = %q, want default verb %q", got.Command(), commandparse.DefaultVerb)
	}
	if got.Spec.Name != "말" || !got.Exact {
		t.Fatalf("resolved command = %+v, want exact 말", got)
	}
	if !slices.Equal(got.Args, []string{"고블린."}) {
		t.Fatalf("Args = %#v, want punctuation-preserved object", got.Args)
	}
}

func TestParseAndResolveLookAliasesPreserveTargetOrdinals(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "봐", Number: 2, Handler: "look"},
		{Name: "보다", Number: 2, Handler: "look"},
		{Name: "조사", Number: 2, Handler: "look"},
	})

	tests := []struct {
		input  string
		alias  string
		args   []string
		values []int64
	}{
		{input: "대상 봐", alias: "봐", args: []string{"대상"}, values: []int64{1}},
		{input: "문 3 보다", alias: "보다", args: []string{"문"}, values: []int64{3}},
		{input: "상자#4#조사", alias: "조사", args: []string{"상자"}, values: []int64{4}},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			got, err := ParseAndResolve(tt.input, registry)
			if err != nil {
				t.Fatalf("ParseAndResolve(%q) error = %v", tt.input, err)
			}

			if got.Command() != tt.alias || got.Spec.Name != tt.alias || got.Spec.Number != 2 || got.Spec.Handler != "look" || !got.Exact {
				t.Fatalf("resolved command = %+v, want exact %s/2/look", got, tt.alias)
			}
			if !slices.Equal(got.Args, tt.args) {
				t.Fatalf("Args = %#v, want %#v", got.Args, tt.args)
			}
			if !slices.Equal(got.Values, tt.values) {
				t.Fatalf("Values = %#v, want %#v", got.Values, tt.values)
			}
		})
	}
}

func TestParseAndResolveLookAtExitTarget(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "동", Number: 1, Handler: "move"},
		{Name: "봐", Number: 2, Handler: "look"},
		{Name: "보다", Number: 2, Handler: "look"},
	})

	tests := []struct {
		input  string
		alias  string
		values []int64
	}{
		{input: "동 봐", alias: "봐", values: []int64{1}},
		{input: "동 2 봐", alias: "봐", values: []int64{2}},
		{input: "동 보다", alias: "보다", values: []int64{1}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseAndResolve(tt.input, registry)
			if err != nil {
				t.Fatalf("ParseAndResolve(%q) error = %v", tt.input, err)
			}

			if got.Command() != tt.alias || got.Spec.Name != tt.alias || got.Spec.Number != 2 || got.Spec.Handler != "look" || !got.Exact {
				t.Fatalf("resolved command = %+v, want exact %s/2/look", got, tt.alias)
			}
			if !slices.Equal(got.Args, []string{"동"}) {
				t.Fatalf("Args = %#v, want 동 target", got.Args)
			}
			if !slices.Equal(got.Values, tt.values) {
				t.Fatalf("Values = %#v, want %#v", got.Values, tt.values)
			}
		})
	}
}

func TestParseAndResolveFallsBackToCommandFirstInput(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "봐", Number: 2, Handler: "look"},
		{Name: "말", Number: 4, Handler: "say"},
	})

	tests := []struct {
		input   string
		command string
		handler string
		args    []string
		values  []int64
	}{
		{
			input:   "봐 사물함",
			command: "봐",
			handler: "look",
			args:    []string{"사물함"},
			values:  []int64{1},
		},
		{
			input:   "봐 사물함 2",
			command: "봐",
			handler: "look",
			args:    []string{"사물함"},
			values:  []int64{2},
		},
		{
			input:   "말 안녕하세요 여러분",
			command: "말",
			handler: "say",
			args:    []string{"안녕하세요", "여러분"},
			values:  []int64{1, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseAndResolve(tt.input, registry)
			if err != nil {
				t.Fatalf("ParseAndResolve(%q) error = %v", tt.input, err)
			}
			if got.Command() != tt.command || got.Spec.Handler != tt.handler {
				t.Fatalf("resolved command = %+v, want %s/%s", got, tt.command, tt.handler)
			}
			if !slices.Equal(got.Args, tt.args) {
				t.Fatalf("Args = %#v, want %#v", got.Args, tt.args)
			}
			if !slices.Equal(got.Values, tt.values) {
				t.Fatalf("Values = %#v, want %#v", got.Values, tt.values)
			}
		})
	}
}

func TestParseAndResolveUnknownCommandReturnsTypedError(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "때려", Number: 23},
	})

	_, err := ParseAndResolve("고블린 춤춰", registry)
	if err == nil {
		t.Fatal("ParseAndResolve() error = nil, want unknown command")
	}
	if !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("error = %v, want ErrUnknownCommand", err)
	}

	var resolveErr *ResolveError
	if !errors.As(err, &resolveErr) {
		t.Fatalf("error = %T, want *ResolveError", err)
	}
	if resolveErr.Code != CodeUnknownCommand || resolveErr.Command != "춤춰" {
		t.Fatalf("ResolveError = %+v, want unknown 춤춰", resolveErr)
	}
}

func TestParseAndResolveEmptyCommandReturnsTypedError(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "봐", Number: 2},
	})

	_, err := ParseAndResolve("", registry)
	if err == nil {
		t.Fatal("ParseAndResolve() error = nil, want empty command")
	}
	if !errors.Is(err, ErrEmptyCommand) {
		t.Fatalf("error = %v, want ErrEmptyCommand", err)
	}

	var resolveErr *ResolveError
	if !errors.As(err, &resolveErr) {
		t.Fatalf("error = %T, want *ResolveError", err)
	}
	if resolveErr.Code != CodeEmptyCommand || resolveErr.Command != "" {
		t.Fatalf("ResolveError = %+v, want empty command", resolveErr)
	}
}

func TestParseAndResolveCanBlockPrivilegedCommands(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "*순간이동", Number: 101, Privileged: true},
	})

	allowed, err := ParseAndResolve("*순", registry)
	if err != nil {
		t.Fatalf("ParseAndResolve() error = %v", err)
	}
	if !allowed.Privileged() {
		t.Fatalf("Privileged() = false, want true")
	}

	_, err = ParseAndResolveWithOptions("*순", registry, DenyPrivileged())
	if err == nil {
		t.Fatal("ParseAndResolveWithOptions() error = nil, want privileged denial")
	}
	if !errors.Is(err, ErrPrivilegedCommand) {
		t.Fatalf("error = %v, want ErrPrivilegedCommand", err)
	}

	var resolveErr *ResolveError
	if !errors.As(err, &resolveErr) {
		t.Fatalf("error = %T, want *ResolveError", err)
	}
	if resolveErr.Code != CodePrivilegedCommand || resolveErr.Command != "*순" {
		t.Fatalf("ResolveError = %+v, want privileged *순", resolveErr)
	}
}

func TestParseAndResolveUsesPrivilegePolicyCallback(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "*공지", Number: 102, Privileged: true},
	})

	called := false
	got, err := ParseAndResolveWithOptions("*공지", registry, WithPrivilegePolicy(func(cmd ResolvedCommand) bool {
		called = true
		return cmd.Spec.Number == 102
	}))
	if err != nil {
		t.Fatalf("ParseAndResolveWithOptions() error = %v", err)
	}
	if !called {
		t.Fatal("privilege policy was not called")
	}
	if got.Spec.Name != "*공지" {
		t.Fatalf("Spec.Name = %q, want *공지", got.Spec.Name)
	}
}

func mustRegistry(t *testing.T, specs []commandspec.CommandSpec) commandspec.Registry {
	t.Helper()

	registry, err := commandspec.NewRegistry(specs)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}
