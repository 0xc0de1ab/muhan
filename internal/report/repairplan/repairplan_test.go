package repairplan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/report/dataissues"
)

func TestGenerateReportsZeroByteRoomAction(t *testing.T) {
	root := t.TempDir()
	roomDir := filepath.Join(root, "rooms", "0")
	if err := os.MkdirAll(roomDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roomDir, "r00000"), nil, 0600); err != nil {
		t.Fatal(err)
	}

	plan, err := Generate(root)
	if err != nil {
		t.Fatal(err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Root != absRoot {
		t.Fatalf("root = %q, want %q", plan.Root, absRoot)
	}
	if plan.GeneratedAt.IsZero() {
		t.Fatal("generatedAt is zero")
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("actions = %d, want 1: %+v", len(plan.Actions), plan.Actions)
	}

	action := plan.Actions[0]
	if action.Path != "rooms/0/r00000" {
		t.Fatalf("path = %q, want rooms/0/r00000", action.Path)
	}
	if action.Kind != ActionRestoreOrDelete {
		t.Fatalf("kind = %q, want %q", action.Kind, ActionRestoreOrDelete)
	}
	if action.Severity != dataissues.SeverityError {
		t.Fatalf("severity = %q, want %q", action.Severity, dataissues.SeverityError)
	}
	if !strings.Contains(action.Problem, "need 480 bytes") {
		t.Fatalf("problem = %q, want room record EOF", action.Problem)
	}
	if !strings.Contains(action.RecommendedAction, "Restore") {
		t.Fatalf("recommendedAction = %q, want restore guidance", action.RecommendedAction)
	}
	if !strings.Contains(action.Rationale, "0 bytes") {
		t.Fatalf("rationale = %q, want 0-byte rationale", action.Rationale)
	}
}
