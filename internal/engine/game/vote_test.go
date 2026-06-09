package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	enginecmd "muhan/internal/engine/command"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestVoteHandlerReportsNoIssueInElectionRoom(t *testing.T) {
	dispatcher := voteDispatcher(t, state.NewWorld(voteWorld(t, true, 21)), NewVoteMemory())
	ctx := voteTestContext(nil)

	status, err := dispatcher.DispatchLine(ctx, "투표")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != enginecmd.StatusDefault || ctx.OutputString() != "투표할 안건이 없네요.\n" {
		t.Fatalf("status/output = %d/%q, want no issue", status, ctx.OutputString())
	}
}

func TestVoteHandlerReadsLegacyIssueFileWithPendingChoice(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "post"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "post", "ISSUE"), []byte("1 문주\n갑\n을\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dispatcher := voteDispatcher(t, state.NewWorld(voteWorld(t, true, 21)), NewVoteMemory(), root)

	ctx, pending, status := startVote(t, dispatcher, "투표 b")
	if status != enginecmd.StatusDoPrompt || ctx.OutputString() != "\n갑\n당신의 선택은? : " {
		t.Fatalf("start status/output = %d/%q, want first pending question", status, ctx.OutputString())
	}

	status = answerVote(t, ctx, pending, "b")
	if status != enginecmd.StatusDefault || ctx.OutputString() != "투표를 하였습니다.\n" {
		t.Fatalf("answer status/output = %d/%q, want vote recorded", status, ctx.OutputString())
	}
	content, err := os.ReadFile(filepath.Join(root, "player", "vote", "Alice_v"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "B"; got != want {
		t.Fatalf("vote file content = %q, want %q", got, want)
	}
}

func TestVoteHandlerReportsEmptyLegacyIssueFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "post"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "post", "ISSUE"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	dispatcher := voteDispatcher(t, state.NewWorld(voteWorld(t, true, 21)), NewVoteMemory(), root)
	ctx := voteTestContext(nil)

	status, err := dispatcher.DispatchLine(ctx, "투표")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != enginecmd.StatusDefault || ctx.OutputString() != "현재 투표할 안건이 없습니다.\n" {
		t.Fatalf("status/output = %d/%q, want empty issue", status, ctx.OutputString())
	}
}

func TestVoteHandlerRejectsNonElectionRoomAndUnderage(t *testing.T) {
	tests := []struct {
		name     string
		election bool
		age      int
		want     string
	}{
		{name: "not election room", election: false, age: 21, want: "투표소가 아닙니다."},
		{name: "underage", election: true, age: 20, want: "당신은 투표할 나이가 아닙니다."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memory := NewVoteMemory()
			memory.SetIssue("문주 선출", []string{"갑"})
			dispatcher := voteDispatcher(t, state.NewWorld(voteWorld(t, tt.election, tt.age)), memory)
			ctx := voteTestContext(nil)

			status, err := dispatcher.DispatchLine(ctx, "투표 A")
			if err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if status != enginecmd.StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			if _, ok := memory.Choice("player:alice"); ok {
				t.Fatalf("choice recorded despite rejection")
			}
		})
	}
}

func TestVoteHandlerUsesLegacyHoursIntervalForAge(t *testing.T) {
	t.Run("legacy interval underage overrides direct adult age", func(t *testing.T) {
		memory := NewVoteMemory()
		memory.SetIssue("문주 선출", []string{"갑"})
		world := state.NewWorld(voteWorld(t, true, 21))
		if err := world.SetCreatureStat("creature:alice", "legacyHoursInterval", 2*86400); err != nil {
			t.Fatal(err)
		}
		dispatcher := voteDispatcher(t, world, memory)
		ctx := voteTestContext(nil)

		status, err := dispatcher.DispatchLine(ctx, "투표 A")
		if err != nil {
			t.Fatalf("DispatchLine() error = %v", err)
		}
		if status != enginecmd.StatusDefault || ctx.OutputString() != "당신은 투표할 나이가 아닙니다.\n" {
			t.Fatalf("status/output = %d/%q, want underage rejection", status, ctx.OutputString())
		}
	})

	t.Run("legacy interval adult overrides direct underage age", func(t *testing.T) {
		memory := NewVoteMemory()
		memory.SetIssue("문주 선출", []string{"갑"})
		world := state.NewWorld(voteWorld(t, true, 20))
		if err := world.SetCreatureStat("creature:alice", "legacyHoursInterval", 3*86400); err != nil {
			t.Fatal(err)
		}
		dispatcher := voteDispatcher(t, world, memory)

		ctx, pending, status := startVote(t, dispatcher, "투표 A")
		if status != enginecmd.StatusDoPrompt || ctx.OutputString() != "\n갑\n당신의 선택은? : " {
			t.Fatalf("start status/output = %d/%q, want prompt", status, ctx.OutputString())
		}
		status = answerVote(t, ctx, pending, "A")
		if status != enginecmd.StatusDefault || ctx.OutputString() != "투표를 하였습니다.\n" {
			t.Fatalf("answer status/output = %d/%q, want recorded", status, ctx.OutputString())
		}
	})

	t.Run("normalized property interval underage overrides direct adult age", func(t *testing.T) {
		memory := NewVoteMemory()
		memory.SetIssue("문주 선출", []string{"갑"})
		world := state.NewWorld(voteWorld(t, true, 21))
		if _, err := world.SetCreatureProperty("creature:alice", "LT-HOURS interval", "172800"); err != nil {
			t.Fatal(err)
		}
		dispatcher := voteDispatcher(t, world, memory)
		ctx := voteTestContext(nil)

		status, err := dispatcher.DispatchLine(ctx, "투표 A")
		if err != nil {
			t.Fatalf("DispatchLine() error = %v", err)
		}
		if status != enginecmd.StatusDefault || ctx.OutputString() != "당신은 투표할 나이가 아닙니다.\n" {
			t.Fatalf("status/output = %d/%q, want underage rejection", status, ctx.OutputString())
		}
	})
}

func TestVoteHandlerRecordsMultiQuestionChoicesAndConfirmsChange(t *testing.T) {
	memory := NewVoteMemory()
	memory.SetIssue("문주 선출", []string{"첫 번째 질문", "두 번째 질문"})
	dispatcher := voteDispatcher(t, state.NewWorld(voteWorld(t, true, 21)), memory)

	ctx, pending, status := startVote(t, dispatcher, "투표 b")
	if status != enginecmd.StatusDoPrompt || ctx.OutputString() != "\n첫 번째 질문\n당신의 선택은? : " {
		t.Fatalf("first prompt status/output = %d/%q", status, ctx.OutputString())
	}
	status = answerVote(t, ctx, pending, "b")
	if status != enginecmd.StatusDoPrompt || ctx.OutputString() != "\n두 번째 질문\n당신의 선택은? : " {
		t.Fatalf("second prompt status/output = %d/%q", status, ctx.OutputString())
	}
	status = answerVote(t, ctx, pending, "c")
	if status != enginecmd.StatusDefault || ctx.OutputString() != "투표를 하였습니다.\n" {
		t.Fatalf("finish status/output = %d/%q", status, ctx.OutputString())
	}
	if got, ok := memory.Choice("player:alice"); !ok || got != "BC" {
		t.Fatalf("choice = %q/%v, want BC/true", got, ok)
	}

	ctx, pending, status = startVote(t, dispatcher, "투표")
	if status != enginecmd.StatusDoPrompt || ctx.OutputString() != "당신은 이미 투표를 했습니다.\n당신의 선택을 바꾸시겠습니까? (y/n): " {
		t.Fatalf("change prompt status/output = %d/%q", status, ctx.OutputString())
	}
	status = answerVote(t, ctx, pending, "n")
	if status != enginecmd.StatusDefault || ctx.OutputString() != "중단합니다.\n" {
		t.Fatalf("cancel status/output = %d/%q", status, ctx.OutputString())
	}
	if got, ok := memory.Choice("player:alice"); !ok || got != "BC" {
		t.Fatalf("choice after cancel = %q/%v, want BC/true", got, ok)
	}

	ctx, pending, status = startVote(t, dispatcher, "투표")
	if status != enginecmd.StatusDoPrompt {
		t.Fatalf("restart status = %d, want do-prompt", status)
	}
	status = answerVote(t, ctx, pending, "Y")
	if status != enginecmd.StatusDoPrompt || ctx.OutputString() != "\n첫 번째 질문\n당신의 선택은? : " {
		t.Fatalf("change accepted prompt status/output = %d/%q", status, ctx.OutputString())
	}
	status = answerVote(t, ctx, pending, "a")
	if status != enginecmd.StatusDoPrompt || ctx.OutputString() != "\n두 번째 질문\n당신의 선택은? : " {
		t.Fatalf("changed second prompt status/output = %d/%q", status, ctx.OutputString())
	}
	status = answerVote(t, ctx, pending, "g")
	if status != enginecmd.StatusDefault || ctx.OutputString() != "투표를 하였습니다.\n" {
		t.Fatalf("changed finish status/output = %d/%q", status, ctx.OutputString())
	}
	if got, ok := memory.Choice("player:alice"); !ok || got != "AG" {
		t.Fatalf("changed choice = %q/%v, want AG/true", got, ok)
	}
}

func TestVoteHandlerRejectsInvalidPendingChoice(t *testing.T) {
	memory := NewVoteMemory()
	memory.SetIssue("문주 선출", []string{"갑"})
	dispatcher := voteDispatcher(t, state.NewWorld(voteWorld(t, true, 21)), memory)

	ctx, pending, status := startVote(t, dispatcher, "vote")
	if status != enginecmd.StatusDoPrompt || pending == nil || *pending == nil {
		t.Fatalf("start status/pending = %d/%v, want do-prompt pending", status, pending != nil && *pending != nil)
	}
	status = answerVote(t, ctx, pending, "x")
	if status != enginecmd.StatusDefault || ctx.OutputString() != "잘못된 선택입니다. 중단합니다.\n" {
		t.Fatalf("invalid status/output = %d/%q, want invalid choice", status, ctx.OutputString())
	}
	if pending != nil && *pending != nil {
		t.Fatal("pending handler remained after invalid choice")
	}
}

func TestVoteHandlerCanBeRegisteredByDispatcherAliases(t *testing.T) {
	for _, line := range []string{"투표 A", "vote A"} {
		t.Run(line, func(t *testing.T) {
			memory := NewVoteMemory()
			memory.SetIssue("문주 선출", []string{"갑"})
			dispatcher := voteDispatcher(t, state.NewWorld(voteWorld(t, true, 21)), memory)

			ctx, pending, status := startVote(t, dispatcher, line)
			if status != enginecmd.StatusDoPrompt || ctx.OutputString() != "\n갑\n당신의 선택은? : " {
				t.Fatalf("start status/output = %d/%q, want prompt", status, ctx.OutputString())
			}
			status = answerVote(t, ctx, pending, "A")
			if status != enginecmd.StatusDefault || ctx.OutputString() != "투표를 하였습니다.\n" {
				t.Fatalf("answer status/output = %d/%q, want dispatch success", status, ctx.OutputString())
			}
			if got, ok := memory.Choice("player:alice"); !ok || got != "A" {
				t.Fatalf("choice = %q/%v, want A/true", got, ok)
			}
		})
	}
}

func voteDispatcher(t *testing.T, world *state.World, memory *VoteMemory, roots ...string) enginecmd.Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "투표", Number: 79, Handler: "vote"},
		{Name: "vote", Number: 79, Handler: "vote"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return enginecmd.Dispatcher{
		Registry: registry,
		Handlers: map[string]enginecmd.Handler{
			"vote": NewVoteHandler(world, memory, roots...),
		},
	}
}

func voteWorld(t *testing.T, election bool, age int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	room := model.Room{
		ID:          "room:poll",
		DisplayName: "투표소",
		PlayerIDs:   []model.PlayerID{"player:alice"},
	}
	if election {
		room.Metadata.Tags = []string{"RELECT"}
	}
	if err := loaded.AddRoom(room); err != nil {
		t.Fatalf("AddRoom() error = %v", err)
	}
	if err := loaded.AddPlayer(model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:poll",
	}); err != nil {
		t.Fatalf("AddPlayer() error = %v", err)
	}
	if err := loaded.AddCreature(model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:poll",
		Stats: map[string]int{
			"age":   age,
			"class": 4,
		},
	}); err != nil {
		t.Fatalf("AddCreature() error = %v", err)
	}
	return loaded
}

func TestVoteHandlerSerialization(t *testing.T) {
	tempDir := t.TempDir()
	memory := NewVoteMemory()
	memory.SetIssue("문주 선출", []string{"갑"})
	dispatcher := voteDispatcher(t, state.NewWorld(voteWorld(t, true, 21)), memory, tempDir)

	ctx, pending, status := startVote(t, dispatcher, "투표 A")
	if status != enginecmd.StatusDoPrompt {
		t.Fatalf("start status = %d, want do-prompt", status)
	}
	status = answerVote(t, ctx, pending, "A")
	if status != enginecmd.StatusDefault || ctx.OutputString() != "투표를 하였습니다.\n" {
		t.Fatalf("answer status/output = %d/%q, want vote recorded", status, ctx.OutputString())
	}

	voteFile := filepath.Join(tempDir, "player", "vote", "Alice_v")
	content, err := os.ReadFile(voteFile)
	if err != nil {
		t.Fatalf("failed to read vote file: %v", err)
	}
	if string(content) != "A" {
		t.Fatalf("vote file content = %q, want A", string(content))
	}

	ctx, pending, status = startVote(t, dispatcher, "투표 B")
	if status != enginecmd.StatusDoPrompt || ctx.OutputString() != "당신은 이미 투표를 했습니다.\n당신의 선택을 바꾸시겠습니까? (y/n): " {
		t.Fatalf("change prompt status/output = %d/%q", status, ctx.OutputString())
	}
	status = answerVote(t, ctx, pending, "y")
	if status != enginecmd.StatusDoPrompt || ctx.OutputString() != "\n갑\n당신의 선택은? : " {
		t.Fatalf("accepted change status/output = %d/%q", status, ctx.OutputString())
	}
	status = answerVote(t, ctx, pending, "B")
	if status != enginecmd.StatusDefault || ctx.OutputString() != "투표를 하였습니다.\n" {
		t.Fatalf("changed answer status/output = %d/%q", status, ctx.OutputString())
	}

	content, err = os.ReadFile(voteFile)
	if err != nil {
		t.Fatalf("failed to read vote file after change: %v", err)
	}
	if string(content) != "B" {
		t.Fatalf("changed vote file content = %q, want B", string(content))
	}
}

func startVote(t *testing.T, dispatcher enginecmd.Dispatcher, line string) (*enginecmd.Context, *enginecmd.PendingLineHandler, enginecmd.Status) {
	t.Helper()
	pending := new(enginecmd.PendingLineHandler)
	ctx := voteTestContext(pending)
	status, err := dispatcher.DispatchLine(ctx, line)
	if err != nil {
		t.Fatalf("DispatchLine(%q) error = %v", line, err)
	}
	return ctx, pending, status
}

func answerVote(t *testing.T, ctx *enginecmd.Context, pending *enginecmd.PendingLineHandler, line string) enginecmd.Status {
	t.Helper()
	if pending == nil || *pending == nil {
		t.Fatal("missing pending vote handler")
	}
	ctx.Output = nil
	status, err := (*pending)(ctx, line)
	if err != nil {
		t.Fatalf("pending vote %q error = %v", line, err)
	}
	return status
}

func voteTestContext(pending *enginecmd.PendingLineHandler) *enginecmd.Context {
	ctx := &enginecmd.Context{ActorID: "player:alice"}
	if pending != nil {
		ctx.Values = map[string]any{
			enginecmd.ContextPendingLineKey: func(handler enginecmd.PendingLineHandler) {
				*pending = handler
			},
		}
	}
	return ctx
}
