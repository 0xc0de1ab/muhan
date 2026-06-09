package command

import (
	"strings"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

type ChangeNameWorld interface {
	InventoryWorld
	SetObjectDisplayName(model.ObjectInstanceID, string) (model.ObjectInstance, error)
	SetObjectProperty(model.ObjectInstanceID, string, string) (model.ObjectInstance, error)
	UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
}

func NewChangeNameHandler(world ChangeNameWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, actor, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		if changeNameLegacyArgCount(resolved) < 3 {
			ctx.WriteString("\n어떤 물건을 무슨 이름으로 바꾸고 싶으세요? <물건> [#] <이름> 명명\n")
			return StatusPrompt, nil
		}

		target, ordinal, name := changeNameTarget(resolved)
		if strings.TrimSpace(name) == "" {
			return StatusPrompt, nil
		}
		if ordinal < 1 {
			ctx.WriteString("그런 물건은 없어요.")
			return StatusPrompt, nil
		}
		object, _, ok := findEquipInventoryObjectWithVisibility(world, actor, target, ordinal, inventoryViewerDetectsInvisible(player, actor))
		if !ok {
			ctx.WriteString("그런 물건은 없어요.")
			return StatusPrompt, nil
		}

		eventOwner, _ := objectStringProperty(world, object, "key[2]")
		if objectHasAnyPropertyFlag(world, object, "event", "oevent", "OEVENT") && eventOwner == "이벤트" {
			ownerName := player.DisplayName
			if ownerName == "" {
				ownerName = actor.DisplayName
			}
			if _, err := world.SetObjectProperty(object.ID, "key[2]", ownerName); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("이름을 기록하였습니다.")
			return StatusPrompt, nil
		}

		if !objectCanChangeName(world, object) {
			ctx.WriteString("명명 할수 있는 물건이 아닙니다.")
			return StatusPrompt, nil
		}
		name = legacyTruncateBytes(name, 79)
		if _, err := world.SetObjectDisplayName(object.ID, name); err != nil {
			return StatusDefault, err
		}
		if _, err := world.SetObjectProperty(object.ID, "name", name); err != nil {
			return StatusDefault, err
		}
		for _, key := range []string{"OCNAME", "customName", "cname"} {
			if _, err := world.SetObjectProperty(object.ID, key, ""); err != nil {
				return StatusDefault, err
			}
		}
		if _, err := world.UpdateObjectTags(object.ID, []string{"named", "ONAMED"}, []string{"customName", "OCNAME"}); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString("\n이름 명명 되었습니다.\n")
		actorName := attackCreatureName(actor)
		return StatusPrompt, roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 자신의 물건에 이름을 명명합니다.")
	}
}

func changeNameLegacyArgCount(resolved ResolvedCommand) int {
	if resolved.Parsed.Num > 0 {
		return resolved.Parsed.Num
	}
	return len(resolved.Args) + 1
}

func changeNameTarget(resolved ResolvedCommand) (string, int64, string) {
	if text, ok := changeNameArgumentText(resolved); ok {
		if target, ordinal, name, ok := parseChangeNameArgumentText(text); ok {
			return target, ordinal, name
		}
	}
	target := resolved.Args[0]
	ordinal := int64(1)
	nameStart := 1
	if len(resolved.Args) >= 3 {
		if value, ok := parseOrdinalToken(resolved.Args[1]); ok {
			ordinal = value
			nameStart = 2
		}
	}
	return target, ordinal, strings.Join(resolved.Args[nameStart:], " ")
}

func changeNameArgumentText(resolved ResolvedCommand) (string, bool) {
	input := strings.TrimSpace(resolved.Input)
	if input == "" {
		return "", false
	}
	for _, command := range dmCommandNameCandidates(resolved) {
		if stripped, ok := stripCommandAtTextEdge(input, command); ok {
			return stripped, true
		}
	}
	return "", false
}

func parseChangeNameArgumentText(input string) (target string, ordinal int64, name string, ok bool) {
	i := 0
	for i < len(input) && isSpaceRune(rune(input[i])) {
		i++
	}
	if i >= len(input) {
		return "", 0, "", false
	}
	start := i
	for i < len(input) && !isSpaceRune(rune(input[i])) {
		i++
	}
	target = input[start:i]
	for i < len(input) && isSpaceRune(rune(input[i])) {
		i++
	}
	ordinal = 1
	if i < len(input) && input[i] >= '0' && input[i] <= '9' {
		ordinal = int64(legacyAtoiPrefix(input[i:]))
		for i < len(input) && input[i] >= '0' && input[i] <= '9' {
			i++
		}
		for i < len(input) && isSpaceRune(rune(input[i])) {
			i++
		}
	}
	if i < len(input) {
		name = input[i:]
	}
	return target, ordinal, name, true
}

func parseOrdinalToken(token string) (int64, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, false
	}
	var value int64
	for _, r := range token {
		if r < '0' || r > '9' {
			return 0, false
		}
		value = value*10 + int64(r-'0')
	}
	return value, true
}

func objectCanChangeName(world InventoryWorld, object model.ObjectInstance) bool {
	if objectHasAnyTag(world, object, "named", "ONAMED") ||
		objectHasAnyPropertyFlag(world, object, "named", "ONAMED", "onamed") {
		return false
	}
	return objectHasAnyTag(world, object, "customName", "OCNAME") ||
		objectHasAnyPropertyFlag(world, object, "customName", "OCNAME", "cname")
}
