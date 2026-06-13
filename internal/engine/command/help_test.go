package command

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
)

func TestHelpHandlerReadsDefaultHelpFileAsUTF8(t *testing.T) {
	root := t.TempDir()
	writeLegacyHelpFixture(t, root, "helpfile", "명령어 목록\n")
	registry := helpTestRegistry(t)
	resolved := mustResolveHelpCommand(t, registry, "도움")

	var ctx Context
	status, err := NewHelpHandler(root, registry)(&ctx, resolved)
	if err != nil {
		t.Fatalf("help handler error = %v", err)
	}
	if status != StatusDoPrompt {
		t.Fatalf("status = %d, want StatusDoPrompt", status)
	}
	if got := ctx.OutputString(); got != "명령어 목록\n" {
		t.Fatalf("output = %q, want default help", got)
	}
}

func TestHelpHandlerReadsCommandSpecificHelp(t *testing.T) {
	root := t.TempDir()
	writeLegacyHelpFixture(t, root, "help.8", "누구 도움말\n")
	registry := helpTestRegistry(t)
	resolved := mustResolveHelpCommand(t, registry, "누구 도움")

	var ctx Context
	_, err := NewHelpHandler(root, registry)(&ctx, resolved)
	if err != nil {
		t.Fatalf("help handler error = %v", err)
	}
	if got := ctx.OutputString(); got != "누구 도움말\n" {
		t.Fatalf("output = %q, want command help", got)
	}
}

func TestHelpHandlerReadsSpellAndPolicyHelp(t *testing.T) {
	root := t.TempDir()
	writeLegacyHelpFixture(t, root, "spellfile", "주술 목록\n")
	writeLegacyHelpFixture(t, root, "policy", "운영 정책\n")
	registry := helpTestRegistry(t)
	handler := NewHelpHandler(root, registry)

	for _, tt := range []struct {
		line string
		want string
	}{
		{line: "주술 도움", want: "주술 목록\n"},
		{line: "정책 도움", want: "운영 정책\n"},
	} {
		t.Run(tt.line, func(t *testing.T) {
			var ctx Context
			_, err := handler(&ctx, mustResolveHelpCommand(t, registry, tt.line))
			if err != nil {
				t.Fatalf("help handler error = %v", err)
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWelcomeHandlerReadsWelcomeFile(t *testing.T) {
	root := t.TempDir()
	writeLegacyHelpFixture(t, root, "welcome", "환영합니다\n")

	var ctx Context
	status, err := NewWelcomeHandler(root)(&ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("welcome handler error = %v", err)
	}
	if status != StatusDoPrompt {
		t.Fatalf("status = %d, want StatusDoPrompt", status)
	}
	if got := ctx.OutputString(); got != "환영합니다\n" {
		t.Fatalf("output = %q, want welcome text", got)
	}
}

func TestWelcomeHandlerReportsMissingFileLikeLegacyViewFile(t *testing.T) {
	root := t.TempDir()

	var ctx Context
	status, err := NewWelcomeHandler(root)(&ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("welcome handler error = %v", err)
	}
	if status != StatusDoPrompt {
		t.Fatalf("status = %d, want StatusDoPrompt", status)
	}
	if got := ctx.OutputString(); got != "화일을 읽을 수 없습니다.\n" {
		t.Fatalf("output = %q, want missing file message", got)
	}
}

func TestHelpHandlerReportsMissingResolvedFileLikeLegacyViewFile(t *testing.T) {
	root := t.TempDir()
	registry := helpTestRegistry(t)
	resolved := mustResolveHelpCommand(t, registry, "누구 도움")

	var ctx Context
	status, err := NewHelpHandler(root, registry)(&ctx, resolved)
	if err != nil {
		t.Fatalf("help handler error = %v", err)
	}
	if status != StatusDoPrompt {
		t.Fatalf("status = %d, want StatusDoPrompt", status)
	}
	if got := ctx.OutputString(); got != "화일을 읽을 수 없습니다.\n" {
		t.Fatalf("output = %q, want missing file message", got)
	}
}

func TestHelpHandlerPaginatesLongHelpLikeLegacyViewFile(t *testing.T) {
	root := t.TempDir()
	writeLegacyHelpFixture(t, root, "helpfile", helpLongLines(24))
	registry := helpTestRegistry(t)
	resolved := mustResolveHelpCommand(t, registry, "도움")

	var pending PendingLineHandler
	ctx := &Context{
		Values: map[string]any{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				pending = handler
			},
		},
	}
	status, err := NewHelpHandler(root, registry)(ctx, resolved)
	if err != nil {
		t.Fatalf("help handler error = %v", err)
	}
	if status != StatusDoPrompt {
		t.Fatalf("first page status = %d, want StatusDoPrompt", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "line S\n") {
		t.Fatalf("first page output missing final first-page line:\n%s", out)
	}
	if strings.Contains(out, "line T\n") {
		t.Fatalf("first page output included continuation line:\n%s", out)
	}
	if !strings.Contains(out, postReadContinuePrompt) {
		t.Fatalf("first page output missing continue prompt:\n%s", out)
	}
	if pending == nil {
		t.Fatal("pending continuation handler was not installed")
	}

	ctx.Output = nil
	status, err = pending(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusDefault {
		t.Fatalf("continuation status = %d, want StatusDefault", status)
	}
	out = ctx.OutputString()
	if !strings.Contains(out, "line T\n") || !strings.Contains(out, "line X\n") {
		t.Fatalf("continuation output missing remaining lines:\n%s", out)
	}
	if pending != nil {
		t.Fatal("pending continuation handler was not cleared")
	}
}

func TestHelpHandlerResolvesSpellSpecificHelp(t *testing.T) {
	root := t.TempDir()
	writeLegacyHelpFixture(t, root, "spell.0", "회복 주술 도움말\n")
	writeLegacySourceFixture(t, root, "src/mtype.h", "#define SVIGOR 0\n")
	writeLegacySourceFixture(t, root, "src/global.c", `
} spllist[] = {
	{ "회복", SVIGOR, vigor, 1 },
	{ "@", -1,0,0 }
};
`)
	registry := helpTestRegistry(t)

	var ctx Context
	_, err := NewHelpHandler(root, registry)(&ctx, mustResolveHelpCommand(t, registry, "회복 도움"))
	if err != nil {
		t.Fatalf("help handler error = %v", err)
	}
	if got := ctx.OutputString(); got != "회복 주술 도움말\n" {
		t.Fatalf("output = %q, want spell help", got)
	}
}

func TestHelpHandlerReportsMissingTopic(t *testing.T) {
	root := t.TempDir()
	registry := helpTestRegistry(t)
	resolved := mustResolveHelpCommand(t, registry, "없는항목 도움")

	var ctx Context
	_, err := NewHelpHandler(root, registry)(&ctx, resolved)
	if err != nil {
		t.Fatalf("help handler error = %v", err)
	}
	if got := ctx.OutputString(); got != "그 명령어에 대한 도움말은 없습니다.\n" {
		t.Fatalf("output = %q, want missing topic message", got)
	}
}

func TestHelpHandlerRequiresRoot(t *testing.T) {
	registry := helpTestRegistry(t)
	_, err := NewHelpHandler("", registry)(&Context{}, mustResolveHelpCommand(t, registry, "도움"))
	if !errors.Is(err, ErrHelpRootRequired) {
		t.Fatalf("error = %v, want ErrHelpRootRequired", err)
	}
}

func helpTestRegistry(t *testing.T) commandspec.Registry {
	t.Helper()
	return mustRegistry(t, []commandspec.CommandSpec{
		{Name: "도움말", Number: 14, Handler: "help"},
		{Name: "누구", Number: 8, Handler: "who"},
	})
}

func mustResolveHelpCommand(t *testing.T, registry commandspec.Registry, line string) ResolvedCommand {
	t.Helper()
	resolved, err := ParseAndResolve(line, registry)
	if err != nil {
		t.Fatalf("ParseAndResolve(%q) error = %v", line, err)
	}
	if resolved.Spec.Handler != "help" {
		t.Fatalf("ParseAndResolve(%q) handler = %q, want help", line, resolved.Spec.Handler)
	}
	return resolved
}

func writeLegacyHelpFixture(t *testing.T, root, name, text string) {
	t.Helper()
	writeLegacySourceFixture(t, root, filepath.Join("help", name), text)
}

func writeLegacySourceFixture(t *testing.T, root, name, text string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := legacykr.EncodeEUCKR(text)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func helpLongLines(n int) string {
	var builder strings.Builder
	for i := 1; i <= n; i++ {
		builder.WriteString("line ")
		builder.WriteByte(byte('A' - 1 + i))
		builder.WriteByte('\n')
	}
	return builder.String()
}
