package command

// NewSpecialHandler creates a SpecialHandler for legacy special command actions (like '눌러' and '밀어').
func NewSpecialHandler(world UseWorld, root string) SpecialHandler {
	comboMemory := newSpecialComboMemory()
	return func(ctx *Context, special int, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		if special != legacySpecialCombo && special != legacySpecialMapScroll {
			ctx.WriteString("아무런 일도 일어나지 않습니다.\n")
			return StatusDefault, nil
		}

		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("무얼 누릅니까?\n")
			return StatusDefault, nil
		}

		room, ok := world.Room(player.RoomID)
		if !ok {
			return StatusDefault, errUseRoomNotFound(player.RoomID)
		}

		ordinal := firstGetOrdinal(resolved)
		if ordinal < 1 {
			ordinal = 1
		}
		object, _, ok := findLegacySpecialObjectCandidate(world, creature, room, target, ordinal, inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("그같은 물건이 없습니다.\n")
			return StatusDefault, nil
		}

		objSpecial, specialOK := legacyObjectSpecial(world, object)
		if !specialOK || objSpecial != special {
			ctx.WriteString("무얼 하려고 하는데요?.\n")
			return StatusDefault, nil
		}

		if special == legacySpecialCombo {
			return useSpecialCombo(ctx, world, comboMemory, creature, room, object)
		}
		if special == legacySpecialMapScroll {
			return readSpecialMapScroll(ctx, world, root, object)
		}

		ctx.WriteString("아무런 일도 일어나지 않습니다.\n")
		return StatusDefault, nil
	}
}

// NOTE (Package 6/6 special hooks port):
// special1.c/sp.c object specials (MAPSC, COMBO) are ported via use.go + this handler (combo_box, view_file, trap-like unlock).
// SP_WAR has no C special_obj case; war declaration remains the separate call_war command path.
// NPC/room specials (ACTION/GIVE/ATTACK/CAST from talk files) implemented in game/talk.go:executeTalk*Action + talkFileActionFromLine.
// GIVE, ATTACK, and CAST talk actions are covered in game/talk.go; room traps trigger from movement/update paths.
// Overall port of special hooks: high; keep special1.c object-special details covered by special/use tests.
