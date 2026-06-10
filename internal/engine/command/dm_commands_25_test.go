package command

import (
	"errors"
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type unifiedDMWorld25 struct {
	players         map[model.PlayerID]model.Player
	creatures       map[model.CreatureID]model.Creature
	rooms           map[model.RoomID]model.Room
	objects         map[model.ObjectInstanceID]model.ObjectInstance
	enemies         map[model.CreatureID][]string
	charmed         map[model.PlayerID][]string
	dustedPlayers   []model.PlayerID
	savedAllPlayers bool
}

func (w *unifiedDMWorld25) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *unifiedDMWorld25) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *unifiedDMWorld25) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *unifiedDMWorld25) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	o, ok := w.objects[id]
	return o, ok
}

func (w *unifiedDMWorld25) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	return model.ObjectPrototype{}, false
}

func (w *unifiedDMWorld25) UpdateRoomDescription(roomID model.RoomID, field, val string) error {
	r, ok := w.rooms[roomID]
	if !ok {
		return errors.New("room not found")
	}
	if field == "short" {
		r.ShortDescription = val
	} else if field == "long" {
		r.LongDescription = val
	}
	w.rooms[roomID] = r
	return nil
}

func (w *unifiedDMWorld25) UpdateObjectProperty(id model.ObjectInstanceID, prop string, val string) error {
	o, ok := w.objects[id]
	if !ok {
		return errors.New("object not found")
	}
	if o.Properties == nil {
		o.Properties = make(map[string]string)
	}
	o.Properties[prop] = val
	w.objects[id] = o
	return nil
}

func (w *unifiedDMWorld25) UpdateCreatureProperty(id model.CreatureID, prop string, val string) error {
	c, ok := w.creatures[id]
	if !ok {
		return errors.New("creature not found")
	}
	if c.Properties == nil {
		c.Properties = make(map[string]string)
	}
	c.Properties[prop] = val
	w.creatures[id] = c
	return nil
}

func (w *unifiedDMWorld25) SetCreatureProperty(id model.CreatureID, prop string, val string) (model.Creature, error) {
	c, ok := w.creatures[id]
	if !ok {
		return model.Creature{}, errors.New("creature not found")
	}
	if val == "" {
		delete(c.Properties, prop)
		if len(c.Properties) == 0 {
			c.Properties = nil
		}
	} else {
		if c.Properties == nil {
			c.Properties = make(map[string]string)
		}
		c.Properties[prop] = val
	}
	w.creatures[id] = c
	return c, nil
}

func (w *unifiedDMWorld25) FindObjectInRoom(roomID model.RoomID, name string) (model.ObjectInstance, bool) {
	for _, o := range w.objects {
		if o.Location.RoomID == roomID && strings.EqualFold(o.DisplayNameOverride, name) {
			return o, true
		}
	}
	return model.ObjectInstance{}, false
}

func (w *unifiedDMWorld25) FindObjectOnCreature(creatureID model.CreatureID, name string) (model.ObjectInstance, bool) {
	for _, o := range w.objects {
		if o.Location.CreatureID == creatureID && strings.EqualFold(o.DisplayNameOverride, name) {
			return o, true
		}
	}
	return model.ObjectInstance{}, false
}

func (w *unifiedDMWorld25) FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool) {
	for _, c := range w.creatures {
		if c.RoomID == roomID && strings.EqualFold(c.DisplayName, name) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func (w *unifiedDMWorld25) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range w.players {
		if strings.EqualFold(p.DisplayName, name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (w *unifiedDMWorld25) ActiveCreatures() []model.Creature {
	var list []model.Creature
	for _, c := range w.creatures {
		list = append(list, c)
	}
	return list
}

func (w *unifiedDMWorld25) DustPlayer(playerID model.PlayerID) error {
	w.dustedPlayers = append(w.dustedPlayers, playerID)
	return nil
}

func (w *unifiedDMWorld25) UpdateCreatureTags(creatureID model.CreatureID, add []string, remove []string) (model.Creature, error) {
	c, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, errors.New("creature not found")
	}
	tags := make(map[string]bool)
	for _, t := range c.Metadata.Tags {
		tags[t] = true
	}
	for _, t := range add {
		tags[t] = true
	}
	for _, t := range remove {
		delete(tags, t)
	}
	var newTags []string
	for t := range tags {
		newTags = append(newTags, t)
	}
	c.Metadata.Tags = newTags
	w.creatures[creatureID] = c
	return c, nil
}

func (w *unifiedDMWorld25) SetCreatureStat(creatureID model.CreatureID, key string, value int) error {
	c, ok := w.creatures[creatureID]
	if !ok {
		return errors.New("creature not found")
	}
	if c.Stats == nil {
		c.Stats = make(map[string]int)
	}
	c.Stats[key] = value
	w.creatures[creatureID] = c
	return nil
}

func (w *unifiedDMWorld25) AddEnemy(attacker model.CreatureID, defender model.CreatureID) (bool, error) {
	if w.enemies == nil {
		w.enemies = make(map[model.CreatureID][]string)
	}
	w.enemies[attacker] = append(w.enemies[attacker], string(defender))
	return true, nil
}

func (w *unifiedDMWorld25) CreatureEnemies(creatureID model.CreatureID) ([]string, error) {
	return w.enemies[creatureID], nil
}

func (w *unifiedDMWorld25) PlayerCharmedCreatures(playerID model.PlayerID) ([]string, error) {
	return w.charmed[playerID], nil
}

func (w *unifiedDMWorld25) SaveAllPlayers() error {
	w.savedAllPlayers = true
	return nil
}

func TestUnifiedDMCommands25(t *testing.T) {
	setupWorld := func() *unifiedDMWorld25 {
		return &unifiedDMWorld25{
			players: map[model.PlayerID]model.Player{
				"player:dm":    {ID: "player:dm", DisplayName: "DMPlayer", CreatureID: "creature:dm", RoomID: "room:100"},
				"player:subdm": {ID: "player:subdm", DisplayName: "SubDMPlayer", CreatureID: "creature:subdm", RoomID: "room:100"},
				"player:alice": {ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:100"},
				"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:100"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm":    {ID: "creature:dm", DisplayName: "DMPlayer", RoomID: "room:100", PlayerID: "player:dm", Stats: map[string]int{"class": 13}},
				"creature:subdm": {ID: "creature:subdm", DisplayName: "SubDMPlayer", RoomID: "room:100", PlayerID: "player:subdm", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:alice": {ID: "creature:alice", DisplayName: "Alice", RoomID: "room:100", PlayerID: "player:alice", Stats: map[string]int{"class": 1}},
				"creature:bob":   {ID: "creature:bob", DisplayName: "Bob", RoomID: "room:100", PlayerID: "player:bob", Stats: map[string]int{"class": 1}},
			},
			rooms: map[model.RoomID]model.Room{
				"room:100": {
					ID:               "room:100",
					DisplayName:      "Old Room",
					ShortDescription: "Short Desc",
					LongDescription:  "Long Desc",
				},
			},
			objects: make(map[model.ObjectInstanceID]model.ObjectInstance),
			enemies: make(map[model.CreatureID][]string),
			charmed: make(map[model.PlayerID][]string),
		}
	}

	// 1. dm_delete
	t.Run("dm_delete", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMDeleteHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "dm_delete"},
			Input: "*delete Desc",
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"*delete", "Desc"},
			},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
	})

	// 2. dm_obj_name
	t.Run("dm_obj_name", func(t *testing.T) {
		w := setupWorld()
		w.objects["object:sword"] = model.ObjectInstance{
			ID:                  "object:sword",
			DisplayNameOverride: "Sword",
			Location:            model.ObjectLocation{CreatureID: "creature:subdm", Slot: "inventory"},
		}
		subdm := w.creatures["creature:subdm"]
		subdm.Inventory = model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:sword"}}
		w.creatures["creature:subdm"] = subdm
		handler := NewDMObjNameHandler(w)
		ctx := &Context{ActorID: "player:subdm"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "dm_obj_name"},
			Input: "*oname Sword NewSword",
			Parsed: commandparse.Command{
				Num: 3,
				Str: [7]string{"*oname", "Sword", "NewSword"},
			},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
	})

	// 3. dm_crt_name
	t.Run("dm_crt_name", func(t *testing.T) {
		w := setupWorld()
		w.creatures["creature:monster"] = model.Creature{
			ID:          "creature:monster",
			DisplayName: "monster",
			RoomID:      "room:100",
		}
		handler := NewDMCrtNameHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "dm_crt_name"},
			Input: "*cname monster NewMonster",
			Parsed: commandparse.Command{
				Num: 3,
				Str: [7]string{"*cname", "monster", "NewMonster"},
			},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
	})

	// 4. list_act
	t.Run("list_act", func(t *testing.T) {
		w := setupWorld()
		handler := NewListActHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "list_act"},
			Input: "*active",
			Parsed: commandparse.Command{
				Num: 1,
				Str: [7]string{"*active"},
			},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
	})

	// 5. dm_dust
	t.Run("dm_dust", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMDustHandler(w)
		ctx := &Context{ActorID: "player:subdm"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "dm_dust"},
			Input: "*dust Alice",
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"*dust", "Alice"},
			},
			Args: []string{"Alice"},
		}

		ctx.Values = map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				return nil
			}),
			"game.activeSessions": func() []any {
				return []any{
					struct {
						ID      string
						ActorID string
					}{ID: "session-alice", ActorID: "player:alice"},
				}
			},
			"game.sendToSession": func(id string, cmd struct{ Write string }) error {
				return nil
			},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		t.Logf("DEBUG: ctx.OutputString() = %q", ctx.OutputString())
		if len(w.dustedPlayers) != 1 || w.dustedPlayers[0] != "player:alice" {
			t.Errorf("expected Alice to be dusted, got: %v (output: %q)", w.dustedPlayers, ctx.OutputString())
		}
	})

	// 6. dm_follow
	t.Run("dm_follow", func(t *testing.T) {
		w := setupWorld()
		w.creatures["creature:monster"] = model.Creature{
			ID:          "creature:monster",
			DisplayName: "monster",
			RoomID:      "room:100",
		}
		handler := NewDMFollowHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "dm_follow"},
			Input: "*cfollow monster",
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"*cfollow", "monster"},
			},
			Args: []string{"monster"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
	})

	// 7. dm_help
	t.Run("dm_help", func(t *testing.T) {
		w := setupWorld()
		tempDir := t.TempDir()
		handler := NewDMHelpHandler(tempDir, w)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "dm_help"},
			Input: "*dmhelp",
			Parsed: commandparse.Command{
				Num: 1,
				Str: [7]string{"*dmhelp"},
			},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDoPrompt {
			t.Errorf("expected StatusDoPrompt, got %v", status)
		}
	})

	// 8. dm_attack
	t.Run("dm_attack", func(t *testing.T) {
		w := setupWorld()
		w.creatures["creature:monster"] = model.Creature{
			ID:          "creature:monster",
			DisplayName: "monster",
			RoomID:      "room:100",
		}
		handler := NewDMAttackHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "dm_attack"},
			Input: "*attack monster Alice",
			Parsed: commandparse.Command{
				Num: 3,
				Str: [7]string{"*attack", "monster", "Alice"},
			},
			Args: []string{"monster", "Alice"},
		}

		ctx.Values = map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				return nil
			}),
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session:alice", ActorID: "player:alice"},
				}
			},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		enemies := w.enemies["creature:monster"]
		if len(enemies) != 1 || enemies[0] != "creature:alice" {
			t.Errorf("expected monster to target alice, got: %v", enemies)
		}
	})

	// 9. list_enm
	t.Run("list_enm", func(t *testing.T) {
		w := setupWorld()
		w.creatures["creature:monster"] = model.Creature{
			ID:          "creature:monster",
			DisplayName: "monster",
			RoomID:      "room:100",
		}
		w.enemies["creature:monster"] = []string{"creature:alice"}
		handler := NewListEnmHandler(w)
		ctx := &Context{ActorID: "player:subdm"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "list_enm"},
			Input: "*enemy monster",
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"*enemy", "monster"},
			},
			Args: []string{"monster"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
	})

	// 10. list_charm
	t.Run("list_charm", func(t *testing.T) {
		w := setupWorld()
		w.charmed["player:alice"] = []string{"monster"}
		handler := NewListCharmHandler(w)
		ctx := &Context{
			ActorID: "player:subdm",
			Values: map[string]any{
				"game.activeSessions": func() []testActiveSession {
					return []testActiveSession{
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
			},
		}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "list_charm"},
			Input: "*charm Alice",
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"*charm", "Alice"},
			},
			Args: []string{"Alice"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
	})

	// 11. dm_save_all_ply
	t.Run("dm_save_all_ply", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMSaveAllPlyHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "dm_save_all_ply"},
			Input: "*사용자저장",
			Parsed: commandparse.Command{
				Num: 1,
				Str: [7]string{"*사용자저장"},
			},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if !w.savedAllPlayers {
			t.Errorf("expected savedAllPlayers to be true")
		}
	})
}
