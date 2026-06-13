package command

import (
	"fmt"
	"log"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// study/teach formulas verified against C src/magic1.c (study, teach):
// - Deterministic (no random success rate, no MP cost, no proficiency gain on learn).
//   C study/teach are instant set if preconds pass (level via ndice, alignment OGOODO/OEVILO,
//   class OCLSEL bits, room RCAST for teach, spell level restrictions for teach >basic).
// - Go matches: readScrollLevelRestricted, magicObjectAlignmentRejected, magicObjectClassRestricted,
//   teach class/spllv checks, RCAST room flag, already-knows via tags.
// - "배워/연마/가르쳐" success/prof/MP confirmed N/A for these paths (combat prof gains elsewhere).
// (historical P0-2/P1-5 markers; resolved in magic + advancement packages)

type StudyWorld interface {
	InventoryWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	DestroyCreatureInventoryObject(model.ObjectInstanceID, model.CreatureID) (bool, error)
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
}

type studySpell struct {
	power int
	name  string
	tag   string
}

func NewStudyHandler(world StudyWorld) Handler {
	// study ("배워") is deterministic success if level/align/class checks pass.
	// Matches C magic1.c study() exactly: no rand, S_SET on success, consume scroll.
	// Success formula: always succeed (no mrand) when ndice <= level && !align reject && class ok.
	// (historical P0-2 marker cleaned post-delivery)
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		roomID := studyActorRoomID(player, creature)

		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("\n무엇을 연마하실려고요?\n")
			return StatusDefault, nil
		}
		if studyActorIsBlind(player, creature) {
			ctx.WriteString("\n당신의 능력으로는 이 비법서를 연마할 수 없습니다.\n")
			return StatusDefault, nil
		}

		object, name, ok := findStudyScrollObject(world, creature, target, firstGetOrdinal(resolved), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("\n그런 것을 소지하고 있지 않습니다.\n")
			return StatusDefault, nil
		}
		if !readScrollObjectIsScroll(world, object) {
			ctx.WriteString("\n이것은 비법서가 아닙니다.\n")
			return StatusDefault, nil
		}

		spell, ok := studySpellByMagicPower(readScrollMagicPower(world, object))
		if !ok {
			ctx.WriteString("\n이 비법서에는 배울 주문이 없습니다.\n")
			return StatusDefault, nil
		}
		if studyActorKnowsSpell(player, creature, spell) {
			ctx.WriteString("\n당신이 이미 터득한 주문서입니다.\n")
			return StatusDefault, nil
		}
		if readScrollLevelRestricted(world, creature, object) {
			ctx.WriteString("\n당신의 능력으로는 " + name + "의 내용을 파악하지 못해 연마할 수 없습니다.")
			return StatusDefault, nil
		}
		if magicObjectAlignmentRejected(world, creature, object) {
			if err := world.MoveObject(object.ID, model.ObjectLocation{RoomID: roomID}); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(studyAlignmentBurnMessage(name))
			return StatusDefault, nil
		}
		if magicObjectClassRestricted(world, creature, object) {
			ctx.WriteString("\n당신의 직업상 " + name + "의 비법을 연마할 수 없습니다.\n")
			return StatusDefault, nil
		}

		player, creature, err = clearCommandActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}
		if _, err := world.UpdateCreatureTags(creature.ID, []string{spell.tag}, nil); err != nil {
			return StatusDefault, err
		}
		if _, err := world.UpdatePlayerTags(player.ID, []string{spell.tag}, nil); err != nil {
			return StatusDefault, err
		}

		if err := destroyStudiedScroll(world, creature.ID, object); err != nil {
			return StatusDefault, err
		}

		// B/C: Mark+Queue after study success (UpdatePlayerTags mutates)
		if w, ok := world.(interface {
			MarkPlayerDirty(model.PlayerID)
			QueueSave(model.PlayerID, model.BankID)
		}); ok {
			w.MarkPlayerDirty(playerID)
			w.QueueSave(playerID, "")
		} else if saver, ok := world.(interface{ SavePlayer(model.PlayerID) error }); ok {
			if err := saver.SavePlayer(playerID); err != nil {
				log.Printf("[PERSIST] ERROR study SavePlayer %s: %v", playerID, err)
			}
		}

		ctx.WriteString("당신은 " + spell.displayName() + "의 내용을 알아내고 연마하기 시작합니다.\n")
		ctx.WriteString("연마를 해 나감에 따라 몸안에서 이상한 기운이 퍼져 나가는\n")
		ctx.WriteString("것이 느껴집니다.\n")
		ctx.WriteString("이야야~~~~~얍 그 기운이 안정되면서 완전히 당신의 것이 \n되었습니다.\n")
		ctx.WriteString("\n연마를 마치자 " + name + "의 형체에 화염이 휩싸이며 어디론가 사라져 버렸습니다.\n")
		actorName := attackCreatureName(creature)
		return StatusDefault, roomBroadcast(ctx, roomID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+name+"의 내용을 읽고 연마합니다.")
	}
}

func studyAlignmentBurnMessage(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "비법서"
	}
	return "\n연마를 끝마치자 " + name + "의 형체가 화염에 휩싸이며 어디론가 사라져\n 버렸습니다.\n"
}

func studyActorRoomID(player model.Player, creature model.Creature) model.RoomID {
	if !player.RoomID.IsZero() {
		return player.RoomID
	}
	return creature.RoomID
}

func studyActorIsBlind(player model.Player, creature model.Creature) bool {
	return settingsPlayerFlag(player, "blind", "pblind") ||
		creatureHasAnyFlag(creature, "blind", "pblind")
}

func findStudyScrollObject(
	world InventoryWorld,
	creature model.Creature,
	target string,
	ordinal int64,
	detectInvisible bool,
) (model.ObjectInstance, string, bool) {
	if object, name, ok := findEquipInventoryObjectWithVisibility(world, creature, target, ordinal, detectInvisible); ok {
		return object, name, true
	}
	return findEquippedObject(world, creature, target, ordinal)
}

func studyActorKnowsSpell(player model.Player, creature model.Creature, spell studySpell) bool {
	if spell.tag == "" {
		return false
	}
	if hasAnyNormalizedFlag(player.Metadata.Tags, spell.tag) {
		return true
	}
	if creatureHasAnyFlag(creature, spell.tag) {
		return true
	}
	targets := normalizedFlagSet(spell.tag)
	for key, value := range creature.Stats {
		if _, ok := targets[normalizeFlagName(key)]; ok && value != 0 {
			return true
		}
	}
	return false
}

// spellTagForPower returns the legacy SXXX tag for a magicPower, used by cast/study/teach
// for learned spell checks. Matches studySpells table which covers all from C spllist.
func spellTagForPower(power int) string {
	for _, s := range studySpells {
		if s.power == power {
			return s.tag
		}
	}
	return ""
}

func destroyStudiedScroll(world StudyWorld, creatureID model.CreatureID, object model.ObjectInstance) error {
	if object.Location.CreatureID == creatureID && object.Location.Slot != "" && object.Location.Slot != "inventory" {
		if err := world.MoveObject(object.ID, model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"}); err != nil {
			return err
		}
	}
	destroyed, err := world.DestroyCreatureInventoryObject(object.ID, creatureID)
	if err != nil {
		return err
	}
	if !destroyed {
		return fmt.Errorf("study destroy object %q: object is not carried by creature %q", object.ID, creatureID)
	}
	return nil
}

func studySpellByMagicPower(power int) (studySpell, bool) {
	for _, spell := range studySpells {
		if spell.power == power {
			return spell, true
		}
	}
	return studySpell{}, false
}

func (s studySpell) displayName() string {
	if s.name != "" {
		return s.name
	}
	return "이름 없는 주문"
}

var studySpells = []studySpell{
	{power: 1, name: "회복", tag: "SVIGOR"},
	{power: 2, name: "삭풍", tag: "SHURTS"},
	{power: 3, name: "발광", tag: "SLIGHT"},
	{power: 4, name: "해독", tag: "SCUREP"},
	{power: 5, name: "성현진", tag: "SBLESS"},
	{power: 6, name: "수호진", tag: "SPROTE"},
	{power: 7, name: "화궁", tag: "SFIREB"},
	{power: 8, name: "은둔법", tag: "SINVIS"},
	{power: 9, name: "도력반", tag: "SRESTO"},
	{power: 10, name: "은둔감지술", tag: "SDINVI"},
	{power: 11, name: "주문감지술", tag: "SDMAGI"},
	{power: 12, tag: "STELEP"},
	{power: 13, name: "혼동", tag: "SBEFUD"},
	{power: 14, name: "뇌전", tag: "SLGHTN"},
	{power: 15, name: "동설주", tag: "SICEBL"},
	{power: 16, name: "빙의", tag: "SENCHA"},
	{power: 17, name: "귀환", tag: "SRECAL"},
	{power: 18, name: "소환", tag: "SSUMMO"},
	{power: 19, name: "원기회복", tag: "SMENDW"},
	{power: 20, name: "완치", tag: "SFHEAL"},
	{power: 21, name: "추적", tag: "STRACK"},
	{power: 22, name: "부양술", tag: "SLEVIT"},
	{power: 23, name: "방열진", tag: "SRFIRE"},
	{power: 24, name: "비상술", tag: "SFLYSP"},
	{power: 25, name: "보마진", tag: "SRMAGI"},
	{power: 26, name: "권풍술", tag: "SSHOCK"},
	{power: 27, name: "지동술", tag: "SRUMBL"},
	{power: 28, name: "화선도", tag: "SBURNS"},
	{power: 29, name: "탄수공", tag: "SBLIST"},
	{power: 30, name: "풍마현", tag: "SDUSTG"},
	{power: 31, name: "파초식", tag: "SWBOLT"},
	{power: 32, name: "폭진", tag: "SCRUSH"},
	{power: 33, name: "낙석", tag: "SENGUL"},
	{power: 34, name: "화풍술", tag: "SBURST"},
	{power: 35, name: "화룡대천", tag: "SSTEAM"},
	{power: 36, name: "토합술", tag: "SSHATT"},
	{power: 37, name: "주작현", tag: "SIMMOL"},
	{power: 38, name: "열사천", tag: "SBLOOD"},
	{power: 39, name: "파천풍", tag: "STHUND"},
	{power: 40, name: "지옥패", tag: "SEQUAK"},
	{power: 41, name: "태양안", tag: "SFLFIL"},
	{power: 42, name: "선악감지", tag: "SKNOWA"},
	{power: 43, name: "저주해소", tag: "SREMOV"},
	{power: 44, name: "방한진", tag: "SRCOLD"},
	{power: 45, name: "수생술", tag: "SBRWAT"},
	{power: 46, name: "지방호", tag: "SSSHLD"},
	{power: 47, name: "천리안", tag: "SLOCAT"},
	{power: 48, name: "백치술", tag: "SDREXP"},
	{power: 49, name: "치료", tag: "SRMDIS"},
	{power: 50, name: "개안술", tag: "SRMBLD"},
	{power: 51, name: "공포", tag: "SFEARS"},
	{power: 52, name: "전회복", tag: "SRVIGO"},
	{power: 53, name: "전송", tag: "STRANO"},
	{power: 54, name: "실명", tag: "SBLIND"},
	{power: 55, name: "봉합구", tag: "SSILNC"},
	{power: 56, name: "이혼대법", tag: "SCHARM"},
	{power: 57, name: "저주", tag: "SCURSE"},
	{power: 58, name: "천지진동", tag: "SISIX1"},
	{power: 59, name: "천상풍", tag: "SISIX2"},
	{power: 60, name: "천마강기", tag: "SISIX3"},
	{power: 61, name: "빙천파", tag: "SISIX4"},
	{power: 62, name: "공포해소", tag: "SRMGONG"},
	{power: 63, name: "혈사천", tag: "XIXIX1"},
	{power: 64, name: "빙설검", tag: "XIXIX2"},
	{power: 65, name: "멸겁화궁", tag: "XIXIX3"},
	{power: 66, name: "탄지수통", tag: "XIXIX4"},
}
