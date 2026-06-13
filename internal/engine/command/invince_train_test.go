package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestInvinceTrainHandlerPromptsAndCompletesRoomTrainingLikeLegacy(t *testing.T) {
	runtime := state.NewWorld(trainWorld(t, []string{"train", "trainingBit5", "trainingBit6"}, model.ClassInvincible, 100, 1000000, 0))

	var pending PendingLineHandler
	ctx := invinceTrainPendingContext(&pending)
	status, err := NewInvinceTrainHandler(runtime)(ctx, ResolvedCommand{Args: []string{"도술사"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	wantPrompt := "무적수련을 하려면 경험치 100만이 필요합니다.\n무적수련을 하시겠습니까?(예/아니오): "
	if status != StatusDoPrompt || ctx.OutputString() != wantPrompt {
		t.Fatalf("status/output = %d/%q, want prompt %q", status, ctx.OutputString(), wantPrompt)
	}
	if pending == nil {
		t.Fatal("pending handler not set")
	}
	creature, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "SFIGHTER") {
		t.Fatalf("creature tags = %+v, want unchanged before confirmation", creature.Metadata.Tags)
	}

	answerCtx := invinceTrainPendingContext(&pending)
	status, err = pending(answerCtx, "예")
	if err != nil {
		t.Fatalf("pending() error = %v", err)
	}
	if status != StatusDefault || answerCtx.OutputString() != "\n무적수련이 완료되었습니다." {
		t.Fatalf("confirm status/output = %d/%q", status, answerCtx.OutputString())
	}
	if pending != nil {
		t.Fatal("pending handler still set after confirmation")
	}
	creature, _ = runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "SFIGHTER") {
		t.Fatalf("creature tags = %+v, want SFIGHTER from room bits", creature.Metadata.Tags)
	}
	if creature.Stats["experience"] != 0 {
		t.Fatalf("experience = %d, want 0", creature.Stats["experience"])
	}
	if creature.Stats["pDice"] != 1 {
		t.Fatalf("pDice = %d, want 1", creature.Stats["pDice"])
	}
	player, _ := runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "SFIGHTER") {
		t.Fatalf("player tags = %+v, want SFIGHTER", player.Metadata.Tags)
	}
}

func TestInvinceTrainHandlerCancelDoesNotMutate(t *testing.T) {
	runtime := state.NewWorld(trainWorld(t, []string{"train", "trainingBit4"}, model.ClassInvincible, 100, 1000000, 0))

	var pending PendingLineHandler
	ctx := invinceTrainPendingContext(&pending)
	status, err := NewInvinceTrainHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("status/pending = %d/%v, want prompt", status, pending)
	}

	answerCtx := invinceTrainPendingContext(&pending)
	status, err = pending(answerCtx, "아니오")
	if err != nil {
		t.Fatalf("pending() error = %v", err)
	}
	if status != StatusDefault || answerCtx.OutputString() != "무적수련이 되지 않았습니다" {
		t.Fatalf("cancel status/output = %d/%q", status, answerCtx.OutputString())
	}
	creature, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "SMAGE") {
		t.Fatalf("creature tags = %+v, want unchanged on cancel", creature.Metadata.Tags)
	}
	if creature.Stats["experience"] != 1000000 {
		t.Fatalf("experience = %d, want unchanged 1000000", creature.Stats["experience"])
	}
}

func TestInvinceTrainHandlerRejectsLegacyGates(t *testing.T) {
	tests := []struct {
		name     string
		roomTags []string
		class    int
		exp      int
		tag      string
		want     string
	}{
		{name: "not training room", class: model.ClassInvincible, exp: 1000000, want: "이 곳은 수련장이 아닙니다!"},
		{name: "low class", roomTags: []string{"train", "trainingBit5", "trainingBit6"}, class: model.ClassFighter, exp: 1000000, want: "무적 이상만 가능합니다."},
		{name: "duplicate room training", roomTags: []string{"train", "trainingBit5", "trainingBit6"}, class: model.ClassInvincible, exp: 1000000, tag: "SFIGHTER", want: "이미 이 직업의 무적수련을 했습니다."},
		{name: "insufficient experience", roomTags: []string{"train", "trainingBit5", "trainingBit6"}, class: model.ClassInvincible, exp: 999999, want: "무적수련을 하려면 경험치 100만이 필요합니다."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := trainWorld(t, tt.roomTags, tt.class, 100, tt.exp, 0)
			if tt.tag != "" {
				creature := loaded.Creatures["creature:alice"]
				creature.Metadata.Tags = []string{tt.tag}
				loaded.Creatures[creature.ID] = creature
			}
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewInvinceTrainHandler(runtime)(ctx, ResolvedCommand{Args: []string{"검사"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestInvinceTrainHandlerCaretakerPromptAndDowngradeLikeLegacy(t *testing.T) {
	runtime := state.NewWorld(trainWorld(t, []string{"train", "trainingBit4"}, model.ClassCaretaker, 100, 100500000, 0))

	var pending PendingLineHandler
	ctx := invinceTrainPendingContext(&pending)
	status, err := NewInvinceTrainHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	wantPrompt := "초인이 무적수련을 하려면 경험치 200만이 필요합니다.\n무적수련 이후 경험치가 1억이 안되면 무적으로 직업이 바뀝니다.\n무적수련을 하시겠습니까?(예/아니오): "
	if status != StatusDoPrompt || ctx.OutputString() != wantPrompt {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), wantPrompt)
	}

	answerCtx := invinceTrainPendingContext(&pending)
	status, err = pending(answerCtx, "예")
	if err != nil {
		t.Fatalf("pending() error = %v", err)
	}
	if status != StatusDefault || answerCtx.OutputString() != "\n무적수련이 완료되었습니다." {
		t.Fatalf("confirm status/output = %d/%q", status, answerCtx.OutputString())
	}
	creature, _ := runtime.Creature("creature:alice")
	if creature.Stats["experience"] != 99500000 {
		t.Fatalf("experience = %d, want 99500000", creature.Stats["experience"])
	}
	if creature.Stats["class"] != model.ClassInvincible {
		t.Fatalf("class = %d, want invincible", creature.Stats["class"])
	}
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "SMAGE") {
		t.Fatalf("creature tags = %+v, want SMAGE", creature.Metadata.Tags)
	}
}

func TestInvinceTrainHandlerDownLevelsInvincibleAfterExperienceLoss(t *testing.T) {
	loaded := trainWorld(t, []string{"train", "trainingBit5", "trainingBit6"}, model.ClassInvincible, 2, 1000000, 0)
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "PUPDMG")
	creature.Stats["hpMax"] = 500
	creature.Stats["mpMax"] = 300
	creature.Stats["hpCurrent"] = 500
	creature.Stats["mpCurrent"] = 300
	creature.Stats["pDice"] = 5
	creature.Stats["strength"] = 10
	creature.Stats["dexterity"] = 10
	creature.Stats["constitution"] = 10
	creature.Stats["intelligence"] = 10
	creature.Stats["piety"] = 10
	loaded.Creatures[creature.ID] = creature
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = append(player.Metadata.Tags, "PUPDMG")
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	var pending PendingLineHandler
	ctx := invinceTrainPendingContext(&pending)
	status, err := NewInvinceTrainHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("status/pending = %d/%v, want prompt", status, pending != nil)
	}

	answerCtx := invinceTrainPendingContext(&pending)
	status, err = pending(answerCtx, "예")
	if err != nil {
		t.Fatalf("pending() error = %v", err)
	}
	if status != StatusDefault || answerCtx.OutputString() != "\n무적수련이 완료되었습니다." {
		t.Fatalf("confirm status/output = %d/%q", status, answerCtx.OutputString())
	}
	creature, _ = runtime.Creature("creature:alice")
	if creature.Level != 1 || creature.Stats["level"] != 1 {
		t.Fatalf("level = canonical:%d stat:%d, want 1/1", creature.Level, creature.Stats["level"])
	}
	if creature.Stats["hpMax"] != 400 || creature.Stats["hpCurrent"] != 400 {
		t.Fatalf("hp max/current = %d/%d, want 400/400", creature.Stats["hpMax"], creature.Stats["hpCurrent"])
	}
	if creature.Stats["mpMax"] != 196 || creature.Stats["mpCurrent"] != 196 {
		t.Fatalf("mp max/current = %d/%d, want 196/196", creature.Stats["mpMax"], creature.Stats["mpCurrent"])
	}
	if creature.Stats["pDice"] != 2 {
		t.Fatalf("pDice = %d, want 2", creature.Stats["pDice"])
	}
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "PUPDMG", "upDamage", "upDmg") {
		t.Fatalf("creature tags = %+v, want PUPDMG cleared", creature.Metadata.Tags)
	}
	player, _ = runtime.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "PUPDMG", "upDamage", "upDmg") {
		t.Fatalf("player tags = %+v, want PUPDMG cleared", player.Metadata.Tags)
	}
}

func TestInvinceTrainHandlerDownLevelAppliesLegacyStatCycle(t *testing.T) {
	loaded := trainWorld(t, []string{"train", "trainingBit4"}, model.ClassInvincible, 4, 1000300, 0)
	creature := loaded.Creatures["creature:alice"]
	creature.Stats["hpMax"] = 500
	creature.Stats["mpMax"] = 300
	creature.Stats["hpCurrent"] = 500
	creature.Stats["mpCurrent"] = 300
	creature.Stats["pDice"] = 6
	creature.Stats["strength"] = 10
	creature.Stats["dexterity"] = 10
	creature.Stats["constitution"] = 10
	creature.Stats["intelligence"] = 10
	creature.Stats["piety"] = 10
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	var pending PendingLineHandler
	ctx := invinceTrainPendingContext(&pending)
	status, err := NewInvinceTrainHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("status/pending = %d/%v, want prompt", status, pending != nil)
	}

	answerCtx := invinceTrainPendingContext(&pending)
	status, err = pending(answerCtx, "예")
	if err != nil {
		t.Fatalf("pending() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("confirm status = %d, want default", status)
	}
	creature, _ = runtime.Creature("creature:alice")
	if creature.Level != 3 || creature.Stats["level"] != 3 {
		t.Fatalf("level = canonical:%d stat:%d, want 3/3", creature.Level, creature.Stats["level"])
	}
	if creature.Stats["experience"] != 300 {
		t.Fatalf("experience = %d, want 300", creature.Stats["experience"])
	}
	if creature.Stats["mpMax"] != 296 || creature.Stats["mpCurrent"] != 296 {
		t.Fatalf("mp max/current = %d/%d, want 296/296", creature.Stats["mpMax"], creature.Stats["mpCurrent"])
	}
	if creature.Stats["intelligence"] != 9 {
		t.Fatalf("intelligence = %d, want 9", creature.Stats["intelligence"])
	}
	if creature.Stats["pDice"] != 6 {
		t.Fatalf("pDice = %d, want unchanged 6", creature.Stats["pDice"])
	}
}

func TestInvinceTrainDispatcherAliasesUseRoomTrainingLikeLegacy(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "무적수련", Number: 156, Handler: "invince_train"},
		{Name: "invince_train", Number: 156, Handler: "invince_train"},
	})

	runtime := state.NewWorld(trainWorld(t, []string{"train", "trainingBit4", "trainingBit5"}, model.ClassInvincible, 100, 1000000, 0))
	dispatcher := Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"invince_train": NewInvinceTrainHandler(runtime),
		},
	}

	var pending PendingLineHandler
	ctx := invinceTrainPendingContext(&pending)
	status, err := dispatcher.DispatchLine(ctx, "무적수련 검사")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDoPrompt || !strings.Contains(ctx.OutputString(), "무적수련을 하시겠습니까?") {
		t.Fatalf("status/output = %d/%q, want prompt", status, ctx.OutputString())
	}

	answerCtx := invinceTrainPendingContext(&pending)
	status, err = pending(answerCtx, "예")
	if err != nil {
		t.Fatalf("pending() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(answerCtx.OutputString(), "무적수련이 완료되었습니다.") {
		t.Fatalf("confirm status/output = %d/%q, want success", status, answerCtx.OutputString())
	}
	creature, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "SRANGER") {
		t.Fatalf("creature tags = %+v, want SRANGER from room bits", creature.Metadata.Tags)
	}
}

func invinceTrainPendingContext(pending *PendingLineHandler) *Context {
	return &Context{
		ActorID: "player:alice",
		Values: map[string]interface{}{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				*pending = handler
			},
		},
	}
}
