package command

import (
	"fmt"
	"log"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	legacyTitleProperty = "legacyTitle"
	legacyTitleMaxBytes = 78
)

type TitleWorld interface {
	InventoryWorld
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

func NewSetTitleHandler(root string, world TitleWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		title := legacyTitleText(resolved)
		if title == "" {
			ctx.WriteString(renderCurrentTitleLine(ctx, player, creature))
			return StatusDefault, nil
		}
		if legacyTitleByteLen(title) > legacyTitleMaxBytes {
			ctx.WriteString("칭호가 너무 깁니다.\n")
			return StatusDefault, nil
		}

		creature, err = world.SetCreatureProperty(creature.ID, legacyTitleProperty, title)
		if err != nil {
			return StatusDefault, err
		}

		store := NewFileAliasStore(root, world)
		record, err := store.read(playerID)
		if err != nil {
			return StatusDefault, err
		}
		record.title = title
		if err := store.write(playerID, record); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString(renderTitleChangedLine(ctx, player, creature))

		// B/C: Queue after title mutation
		if w, ok := world.(interface {
			MarkPlayerDirty(model.PlayerID)
			QueueSave(model.PlayerID, model.BankID)
		}); ok {
			w.MarkPlayerDirty(playerID)
			w.QueueSave(playerID, "")
		} else if saver, ok := world.(interface{ SavePlayer(model.PlayerID) error }); ok {
			if err := saver.SavePlayer(playerID); err != nil {
				log.Printf("[PERSIST] ERROR title SavePlayer %s: %v", playerID, err)
			}
		}
		return StatusDefault, nil
	}
}

func NewClearTitleHandler(root string, world TitleWorld) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		if titleCustomTitle(creature) == "" {
			ctx.WriteString("칭호가 설정되어 있지 않습니다.\n")
			return StatusDefault, nil
		}

		creature, err = world.SetCreatureProperty(creature.ID, legacyTitleProperty, "")
		if err != nil {
			return StatusDefault, err
		}

		store := NewFileAliasStore(root, world)
		record, err := store.read(playerID)
		if err != nil {
			return StatusDefault, err
		}
		record.title = ""
		if err := store.write(playerID, record); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString(renderTitleChangedLine(ctx, player, creature))

		// B/C: Queue after title clear mutation
		if w, ok := world.(interface {
			MarkPlayerDirty(model.PlayerID)
			QueueSave(model.PlayerID, model.BankID)
		}); ok {
			w.MarkPlayerDirty(playerID)
			w.QueueSave(playerID, "")
		} else if saver, ok := world.(interface{ SavePlayer(model.PlayerID) error }); ok {
			if err := saver.SavePlayer(playerID); err != nil {
				log.Printf("[PERSIST] ERROR clear title SavePlayer %s: %v", playerID, err)
			}
		}
		return StatusDefault, nil
	}
}

func legacyTitleText(resolved ResolvedCommand) string {
	input := strings.TrimSpace(resolved.Input)
	if input != "" {
		for _, command := range dmCommandNameCandidates(resolved) {
			if stripped, ok := stripCommandAtTextEdge(input, command); ok {
				return stripped
			}
		}
	}
	return joinArgs(resolved.Args)
}

func legacyTitleByteLen(title string) int {
	encoded, err := legacykr.EncodeEUCKR(title)
	if err != nil {
		return len([]byte(title))
	}
	return len(encoded)
}

func renderCurrentTitleLine(ctx *Context, player model.Player, creature model.Creature) string {
	return fmt.Sprintf("당신은 %s %s입니다.\n", renderCreatureTitle(ctx, creature), titleActorName(player, creature))
}

func renderTitleChangedLine(ctx *Context, player model.Player, creature model.Creature) string {
	return fmt.Sprintf("당신은 이제부터 %s %s입니다.\n", renderCreatureTitle(ctx, creature), titleActorName(player, creature))
}

func titleActorName(player model.Player, creature model.Creature) string {
	if name := cleanDisplayText(creature.DisplayName); name != "" {
		return name
	}
	if name := cleanDisplayText(player.DisplayName); name != "" {
		return name
	}
	if !player.ID.IsZero() {
		return string(player.ID)
	}
	return string(creature.ID)
}

func renderCreatureTitle(ctx *Context, creature model.Creature) string {
	opts := textOptionsFromContext(ctx)
	if title := titleCustomTitle(creature); title != "" {
		return renderDisplayText(title, opts)
	}
	return defaultCreatureTitle(creature)
}

func titleCustomTitle(creature model.Creature) string {
	return strings.TrimSpace(creature.Properties[legacyTitleProperty])
}

func defaultCreatureTitle(creature model.Creature) string {
	class := creatureClass(creature)
	if class < 0 || class >= len(legacyPlayerLevelTitles) {
		class = 0
	}
	level := creature.Level
	if statLevel := creatureStat(creature, "level"); statLevel > level {
		level = statLevel
	}
	titleIndex := (((level + 3) / 4) - 1) / 3
	if titleIndex < 0 {
		titleIndex = 0
	}
	if titleIndex > 7 {
		titleIndex = 7
	}
	return legacyPlayerLevelTitles[class][titleIndex]
}

var legacyPlayerLevelTitles = [][]string{
	{"", "", "", "", "", "", "", ""},
	{"깡패", "강도", "살인자", "도살자", "왕자객", "응징자", "살성", "살신"},
	{"초보", "수련생", "무협", "철권", "권성", "권황", "지존", "무신"},
	{"땡중", "사미승", "소승", "감찰승", "주지", "대사", "국사", "부처"},
	{"백정", "칼잡이", "무인", "용병", "검객", "검성", "검황", "무림파천"},
	{"심부름꾼", "도제자", "마술사", "도객", "마인", "도인", "신선", "마존"},
	{"골목대장", "무객", "협객", "의협", "정전자", "용전사", "수호자", "성전사"},
	{"쫄따구", "순찰병", "감찰원", "도성지기", "포교", "포도대장", "감찰어사", "감찰장군"},
	{"바늘도둑", "개도둑", "좀도둑", "소도둑", "왕도둑", "도성", "도신", "신수"},
	{"무적", "무적", "무적", "무적", "무적", "무적", "무적", "무적"},
	{"초인", "초인", "초인", "초인", "초인", "초인", "초인", "초인"},
	{"불사", "초인", "초인", "초인", "초인", "초인", "초인", "불사"},
	{"도우미", "도우미", "도우미", "도우미", "도우미", "도우미", "도우미", "도우미"},
	{"바보", "멍청이", "또라이", "머저리", "띨띨이", "왕바보", "백치황제", "바보들의신"},
}
