package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMCrtNameWorld struct {
	players       map[model.PlayerID]model.Player
	creatures     map[model.CreatureID]model.Creature
	rooms         map[model.RoomID]model.Room
	creatureOrder []model.CreatureID

	// Spies
	creatureProps map[model.CreatureID]map[string]string
}

func (w *mockDMCrtNameWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMCrtNameWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMCrtNameWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *mockDMCrtNameWorld) orderedCreatures() []model.Creature {
	if len(w.creatureOrder) == 0 {
		creatures := make([]model.Creature, 0, len(w.creatures))
		for _, c := range w.creatures {
			creatures = append(creatures, c)
		}
		return creatures
	}

	creatures := make([]model.Creature, 0, len(w.creatureOrder))
	for _, id := range w.creatureOrder {
		if c, ok := w.creatures[id]; ok {
			creatures = append(creatures, c)
		}
	}
	return creatures
}

func (w *mockDMCrtNameWorld) FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool) {
	for _, c := range w.orderedCreatures() {
		if c.RoomID == roomID && (strings.EqualFold(c.DisplayName, name) || strings.EqualFold(c.Properties["name"], name)) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func (w *mockDMCrtNameWorld) FindCreatureByName(roomID model.RoomID, name string, count int) (model.Creature, bool) {
	if count < 1 {
		count = 1
	}
	seen := 0
	for _, c := range w.orderedCreatures() {
		if c.RoomID == roomID && (strings.EqualFold(c.DisplayName, name) || strings.EqualFold(c.Properties["name"], name)) {
			seen++
			if seen == count {
				return c, true
			}
		}
	}
	return model.Creature{}, false
}

func (w *mockDMCrtNameWorld) UpdateCreatureProperty(creatureID model.CreatureID, key, val string) error {
	if w.creatureProps[creatureID] == nil {
		w.creatureProps[creatureID] = make(map[string]string)
	}
	w.creatureProps[creatureID][key] = val
	return nil
}

func TestDMCrtName(t *testing.T) {
	world := &mockDMCrtNameWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:00001"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				RoomID: "room:00001",
				Stats:  map[string]int{"class": 13}, // DM (13+)
			},
			"creature:orc": {
				ID:          "creature:orc",
				DisplayName: "orc",
				RoomID:      "room:00001",
			},
			"creature:orc2": {
				ID:          "creature:orc2",
				DisplayName: "orc",
				RoomID:      "room:00001",
			},
		},
		creatureOrder: []model.CreatureID{"creature:alice", "creature:orc", "creature:orc2"},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {ID: "room:00001"},
		},
		creatureProps: make(map[model.CreatureID]map[string]string),
	}

	handler := NewDMCrtNameHandler(world)

	// Case 1: Denied permission (class < 13)
	t.Run("Denied permission", func(t *testing.T) {
		world.creatures["creature:alice"].Stats["class"] = 12
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc new-orc",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		if got := ctx.OutputString(); got != "" {
			t.Errorf("output = %q, want no permission output", got)
		}
		world.creatures["creature:alice"].Stats["class"] = 13 // restore
	})

	// Case 2: Insufficient arguments
	t.Run("Insufficient arguments", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		want := "어떤 몹을 무슨 이름으로 바꾸시려구요?<몹이름> [#] [-dtmk] <이름> *cname"
		if got := ctx.OutputString(); got != want {
			t.Errorf("usage message = %q, want %q", got, want)
		}
	})

	t.Run("Existing target without value returns prompt silently like legacy", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		if got := ctx.OutputString(); got != "" {
			t.Errorf("output = %q, want no output", got)
		}
		if got := world.creatureProps["creature:orc"]["name"]; got != "" {
			t.Errorf("creature name property = %q, want no mutation", got)
		}
	})

	t.Run("Missing target without value still reports legacy not found", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname goblin",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		want := "이 방에 그런 것은 없습니다."
		if got := ctx.OutputString(); got != want {
			t.Errorf("not-found message = %q, want %q", got, want)
		}
	})

	// Case 3: Monster not found in room
	t.Run("Monster not found", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname goblin new-goblin",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		want := "이 방에 그런 것은 없습니다."
		if got := ctx.OutputString(); got != want {
			t.Errorf("not-found message = %q, want %q", got, want)
		}
	})

	t.Run("Player creature cannot be renamed as monster", func(t *testing.T) {
		world.players["player:bob"] = model.Player{
			ID:          "player:bob",
			CreatureID:  "creature:bob",
			DisplayName: "Bob",
			RoomID:      "room:00001",
		}
		world.creatures["creature:bob"] = model.Creature{
			ID:          "creature:bob",
			Kind:        model.CreatureKindPlayer,
			PlayerID:    "player:bob",
			DisplayName: "Bob",
			RoomID:      "room:00001",
		}
		room := world.rooms["room:00001"]
		room.CreatureIDs = []model.CreatureID{"creature:bob", "creature:orc", "creature:orc2"}
		world.rooms["room:00001"] = room

		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname Bob NewBob",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		if got, want := ctx.OutputString(), "이 방에 그런 것은 없습니다."; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
		if props := world.creatureProps["creature:bob"]; len(props) != 0 {
			t.Fatalf("player creature properties mutated: %+v", props)
		}
	})

	// Case 4: Rename (none flag)
	t.Run("Rename none flag", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc Thrall",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "\n이름이 바뀌었습니다."
		if got := ctx.OutputString(); got != want {
			t.Errorf("success message = %q, want %q", got, want)
		}
		if world.creatureProps["creature:orc"]["name"] != "Thrall" {
			t.Errorf("name property = %q, want Thrall", world.creatureProps["creature:orc"]["name"])
		}
	})

	t.Run("Verb-final name preserves legacy cut_command trailing spaces", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "orc Thrall   *cname",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := ctx.OutputString(); got != "\n이름이 바뀌었습니다." {
			t.Errorf("success message = %q, want rename success", got)
		}
		if got := world.creatureProps["creature:orc"]["name"]; got != "Thrall  " {
			t.Errorf("name property = %q, want legacy trailing spaces", got)
		}
	})

	t.Run("Ordinal selects second creature", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc 2 Grom",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "\n이름이 바뀌었습니다."
		if got := ctx.OutputString(); got != want {
			t.Errorf("success message = %q, want %q", got, want)
		}
		if world.creatureProps["creature:orc2"]["name"] != "Grom" {
			t.Errorf("second creature name = %q, want Grom", world.creatureProps["creature:orc2"]["name"])
		}
		if world.creatureProps["creature:orc"]["name"] == "Grom" {
			t.Errorf("first creature was renamed by ordinal lookup")
		}
	})

	// Case 5: Change description (-d flag)
	t.Run("Change description -d flag", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc -d a dirty looking orc",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(ctx.OutputString(), "출력문이 바뀌었습니다.") {
			t.Errorf("expected success message, got: %q", ctx.OutputString())
		}
		if world.creatureProps["creature:orc"]["description"] != "a dirty looking orc" {
			t.Errorf("description property = %q, want 'a dirty looking orc'", world.creatureProps["creature:orc"]["description"])
		}
	})

	// Case 6: Change talk (-t flag)
	t.Run("Change talk -t flag", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc -t Zug Zug!",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(ctx.OutputString(), "대화문이 바뀌었습니다.") {
			t.Errorf("expected success message, got: %q", ctx.OutputString())
		}
		if world.creatureProps["creature:orc"]["talk"] != "Zug Zug!" {
			t.Errorf("talk = %q, want 'Zug Zug!'", world.creatureProps["creature:orc"]["talk"])
		}
	})

	// Case 7: Change keyword (-k2 flag)
	t.Run("Change keyword -k2 flag", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc -k2 warchief",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(ctx.OutputString(), "키가 바뀌었습니다.") {
			t.Errorf("expected success message, got: %q", ctx.OutputString())
		}
		if world.creatureProps["creature:orc"]["key[1]"] != "warchief" {
			t.Errorf("key[1] = %q, want 'warchief'", world.creatureProps["creature:orc"]["key[1]"])
		}
	})

	// Case 8: Position check flag (-m1 flag)
	t.Run("Position check -m1 flag without value returns prompt silently", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc -m1",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		if got := ctx.OutputString(); got != "" {
			t.Errorf("output = %q, want no output", got)
		}
	})

	t.Run("Position check -m1 flag with value reaches legacy movement branch", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc -m1 ignored",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if !strings.Contains(ctx.OutputString(), "몹의 위치가 그 방향에는 방이 없습니다.") {
			t.Errorf("expected 'no room in that direction' message, got: %q", ctx.OutputString())
		}
	})

	t.Run("Separated -m number uses legacy atoi but keeps number as value", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc -m 1",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if !strings.Contains(ctx.OutputString(), "몹의 위치가 그 방향에는 방이 없습니다.") {
			t.Errorf("expected 'no room in that direction' message, got: %q", ctx.OutputString())
		}
	})

	// Case 9: Position check flag without number (-m flag)
	t.Run("Position check -m flag without number returns prompt silently", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc -m",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		if got := ctx.OutputString(); got != "" {
			t.Errorf("output = %q, want no output", got)
		}
	})

	t.Run("Position check -m flag with value falls through unchanged", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc -m ignored",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if !strings.Contains(ctx.OutputString(), "바뀌었습니다.") {
			t.Errorf("expected success message, got: %q", ctx.OutputString())
		}
	})

	// Case 10: Unrecognized flag fallback (handled as part of name)
	t.Run("Unrecognized flag fallback", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc -x unrecognized flag test",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(ctx.OutputString(), "이름이 바뀌었습니다.") {
			t.Errorf("expected success message, got: %q", ctx.OutputString())
		}
		if world.creatureProps["creature:orc"]["name"] != "-x unrecognized flag test" {
			t.Errorf("name = %q, want '-x unrecognized flag test'", world.creatureProps["creature:orc"]["name"])
		}
	})

	t.Run("Value limits use legacy bytes", func(t *testing.T) {
		longKoreanName := strings.Repeat("가", 40)
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc " + longKoreanName,
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := world.creatureProps["creature:orc"]["name"], strings.Repeat("가", 39); got != want {
			t.Errorf("name = %q, want legacy 79-byte truncation %q", got, want)
		}

		longKoreanTalk := strings.Repeat("말", 40)
		ctx = &Context{ActorID: "player:alice"}
		resolved = ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc -t " + longKoreanTalk,
		}
		_, err = handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := world.creatureProps["creature:orc"]["talk"], strings.Repeat("말", 39); got != want {
			t.Errorf("talk = %q, want legacy 79-byte truncation %q", got, want)
		}

		longKoreanKey := strings.Repeat("키", 10)
		ctx = &Context{ActorID: "player:alice"}
		resolved = ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*cname"},
			Input: "*cname orc -k2 " + longKoreanKey,
		}
		_, err = handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := world.creatureProps["creature:orc"]["key[1]"], strings.Repeat("키", 9); got != want {
			t.Errorf("key[1] = %q, want legacy 19-byte truncation %q", got, want)
		}
	})
}
