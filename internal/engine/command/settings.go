package command

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type SettingsWorld interface {
	InventoryWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	SetCreatureStat(model.CreatureID, string, int) error
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

type settingFlag struct {
	Name string
	Tag  string
}

var settableFlags = []settingFlag{
	{Name: "이야기듣기", Tag: "PIGNOR"},
	{Name: "잡담듣기", Tag: "PNOBRD"},
	{Name: "환호듣기", Tag: "PNOBR2"},
	{Name: "묘사보기", Tag: "PDSCRP"},
	{Name: "소환", Tag: "PNOSUM"},
	{Name: "행삽입", Tag: "PNOCMP"},
	{Name: "상태", Tag: "PPROMP"},
	{Name: "반향", Tag: "PLECHO"},
	{Name: "색", Tag: "PANSIC"},
	{Name: "밝은색", Tag: "PBRIGH"},
	{Name: "방이름", Tag: "PNORNM"},
	{Name: "짧은설명", Tag: "PNOSDS"},
	{Name: "긴설명", Tag: "PNOLDS"},
	{Name: "출구", Tag: "PNOEXT"},
}

func NewSetHandler(world SettingsWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString(RenderSettings(player, creature))
			return StatusDefault, nil
		}
		if target == "도망수치" {
			value := rawSettingValue(resolved, 0)
			if value == 1 {
				value = 10
			}
			if value < 2 {
				value = 2
			}
			if err := world.SetCreatureStat(creature.ID, "wimpyValue", value); err != nil {
				return StatusDefault, err
			}
			if _, err := setSettingsCreatureFlag(world, creature.ID, "PWIMPY", true); err != nil {
				return StatusDefault, err
			}
			creature, _ = world.Creature(creature.ID)
			player, _ = world.Player(player.ID)
			ctx.WriteString(RenderSettings(player, creature))
			return StatusDefault, nil
		}
		if target == "패거리귀환" {
			if settingsCreatureFlag(creature, "PFAMIL", "familyFlag") {
				if _, err := setSettingsCreatureFlag(world, creature.ID, "PFRTUN", true); err != nil {
					return StatusDefault, err
				}
				ctx.WriteString("패거리 존으로 귀환을 합니다.\n")
			} else {
				ctx.WriteString("당신은 패거리에 가입되어 있지 않습니다.\n")
			}
			creature, _ = world.Creature(creature.ID)
			ctx.WriteString(RenderSettings(player, creature))
			return StatusDefault, nil
		}
		if target == "수동공격" {
			if _, err := setSettingsCreatureFlag(world, creature.ID, "PNOAAT", true); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("수동으로 공격합니다.\n")
			creature, _ = world.Creature(creature.ID)
			ctx.WriteString(RenderSettings(player, creature))
			return StatusDefault, nil
		}
		if status, done, err := setPrivilegedCreatureSetting(ctx, world, creature, target); done {
			if err != nil {
				return StatusDefault, err
			}
			creature, _ = world.Creature(creature.ID)
			ctx.WriteString(RenderSettings(player, creature))
			return status, nil
		}
		for _, flag := range settableFlags {
			if flag.Name != target {
				continue
			}
			creature, err = setSettingsCreatureFlag(world, creature.ID, flag.Tag, !settingsCreatureFlag(creature, flag.Tag))
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(RenderSettings(player, creature))
			return StatusDefault, nil
		}
		ctx.WriteString(RenderSettings(player, creature))
		return StatusDefault, nil
	}
}

func NewClearHandler(world SettingsWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		_, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("[해제 도움말]이라고 치시면 모든 설정사항들을 볼 수 있습니다.\n")
			return StatusDefault, nil
		}
		switch target {
		case "잡담듣기거부":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PNOBRD", false)
			ctx.WriteString("이제부터 잡담을 듣습니다.\n")
		case "행삽입":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PNOCMP", false)
			ctx.WriteString("이제부터 메세지를 출력할때 한행을 삽입하지 않습니다.\n")
		case "반향":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PLECHO", false)
			ctx.WriteString("당신의 메세지가 반향되지 않습니다.\n")
		case "방이름":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PNORNM", true)
			ctx.WriteString("이제부터 방이름을 출력하지 않습니다.\n")
		case "간단":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PNOSDS", true)
			ctx.WriteString("방의 간단한 설명을 보지 않습니다.\n")
		case "일반":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PNOLDS", true)
			ctx.WriteString("방의 자세한 설명을 보지 않습니다.\n")
		case "hexline":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PHEXLN", false)
			ctx.WriteString("Hex line disabled.\n")
		case "도망수치":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PWIMPY", false)
			ctx.WriteString("도망수치 설정이 해제되었습니다.\n")
		case "eavesdropper":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PEAVES", false)
			ctx.WriteString("Eavesdropper mode disabled.\n")
		case "상태":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PPROMP", false)
			ctx.WriteString("당신의 상태를 보여주지 않습니다.\n")
		case "~robot~":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PROBOT", false)
			ctx.WriteString("Robot mode off.\n")
		case "색":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PANSIC", false)
			ctx.WriteString("이제부터 메세지가 모두 흑백으로 출력됩니다.\n")
		case "소환거부":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PNOSUM", false)
			ctx.WriteString("이제부터 다른사람이 당신을 소환할 수 있습니다.\n")
		case "이야기듣기거부":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PIGNOR", false)
			ctx.WriteString("이제부터 개인적인 이야기를 듣습니다.\n")
		case "수동공격":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PNOAAT", false)
			ctx.WriteString("이제부터 자동으로 공격합니다.\n")
		case "밝은색":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PBRIGH", false)
			ctx.WriteString("어두운 색으로 출력합니다.\n")
		case "패거리귀환":
			creature, err = setSettingsCreatureFlag(world, creature.ID, "PFRTUN", false)
			ctx.WriteString("광장으로 귀환합니다.\n")
		default:
			ctx.WriteString("잘못 지정되었습니다.\n")
			return StatusDefault, nil
		}
		if err != nil {
			return StatusDefault, err
		}
		_ = creature
		return StatusDefault, nil
	}
}

func setSettingsCreatureFlag(world SettingsWorld, creatureID model.CreatureID, tag string, enabled bool) (model.Creature, error) {
	value := 0
	if enabled {
		value = 1
	}
	if err := world.SetCreatureStat(creatureID, tag, value); err != nil {
		return model.Creature{}, err
	}
	if enabled {
		return world.UpdateCreatureTags(creatureID, []string{tag}, nil)
	}
	creature, err := world.UpdateCreatureTags(creatureID, nil, []string{tag})
	if err != nil {
		return model.Creature{}, err
	}
	return clearSettingsCreaturePropertyFlag(world, creature, tag)
}

func RenderSettings(player model.Player, creature model.Creature) string {
	wimpy := "미설정"
	if settingsCreatureFlag(creature, "PWIMPY") {
		wimpyValue := creature.Stats["wimpyValue"]
		if wimpyValue == 0 {
			wimpyValue = 10
		}
		wimpy = fmt.Sprintf("%-6d", wimpyValue)
	}
	return fmt.Sprintf(
		"  %-13s%-15s%-13s%-15s\n"+
			"-------------------------------------------------------\n"+
			"  이야기듣기: %-14s  잡담듣기  : %s\n"+
			"  환호듣기  : %-14s  묘사보기  : %s\n"+
			"  소환      : %-14s  도망수치  : %s\n"+
			"  행삽입    : %-14s  상태      : %s\n"+
			"  반향      : %-14s  색        : %s\n"+
			"  밝은색    : %-14s  방이름    : %s\n"+
			"  짧은설명  : %-14s  긴설명    : %s\n"+
			"  출구      : %-14s  패거리귀환: %s\n\n"+
			settingsActiveEffects(creature)+
			"\n[설정 도움말]이라고 치시면 자세한 설정사항을 볼 수 있습니다.\n",
		"설  정", "상태", "설  정", "상태",
		settingUnsetStatus(settingsCreatureFlag(creature, "PIGNOR"), " 설정 ", "미설정"),
		settingUnsetStatus(settingsCreatureFlag(creature, "PNOBRD"), " 설정 ", "미설정"),
		settingUnsetStatus(settingsCreatureFlag(creature, "PNOBR2"), " 설정 ", "미설정"),
		settingOnStatus(settingsCreatureFlag(creature, "PDSCRP"), " 설정 ", "미설정"),
		settingOnStatus(settingsCreatureFlag(creature, "PNOSUM"), " 불가 ", " 가능 "),
		wimpy,
		settingOnStatus(settingsCreatureFlag(creature, "PNOCMP"), " 설정 ", "미설정"),
		settingOnStatus(settingsCreatureFlag(creature, "PPROMP"), " 출력 ", "미설정"),
		settingOnStatus(settingsCreatureFlag(creature, "PLECHO"), " 설정 ", "미설정"),
		settingOnStatus(settingsCreatureFlag(creature, "PANSIC"), " 사용 ", "미사용"),
		settingOnStatus(settingsCreatureFlag(creature, "PBRIGH"), " 사용 ", "미사용"),
		settingUnsetStatus(settingsCreatureFlag(creature, "PNORNM"), " 출력 ", "미설정"),
		settingUnsetStatus(settingsCreatureFlag(creature, "PNOSDS"), " 출력 ", "미설정"),
		settingUnsetStatus(settingsCreatureFlag(creature, "PNOLDS"), " 출력 ", "미설정"),
		settingOnStatus(settingsCreatureFlag(creature, "PNOEXT"), "그래프", "텍스트"),
		settingOnStatus(settingsCreatureFlag(creature, "PFRTUN"), " 설정 ", "미설정"),
	)
}

func setPrivilegedCreatureSetting(ctx *Context, world SettingsWorld, creature model.Creature, target string) (Status, bool, error) {
	tag := ""
	message := ""
	switch target {
	case "hexline":
		tag = "PHEXLN"
		message = "Hexline enabled.\n"
	case "eavesdropper":
		tag = "PEAVES"
		message = "Eavesdropper mode enabled.\n"
	case "~robot~":
		tag = "PROBOT"
		message = "Robot mode on.\n"
	default:
		return StatusDefault, false, nil
	}
	if creatureStat(creature, "class") >= model.ClassSubDM {
		if _, err := setSettingsCreatureFlag(world, creature.ID, tag, true); err != nil {
			return StatusDefault, true, err
		}
		ctx.WriteString(message)
	} else if _, err := setSettingsCreatureFlag(world, creature.ID, tag, false); err != nil {
		return StatusDefault, true, err
	}
	return StatusDefault, true, nil
}

func rawSettingValue(resolved ResolvedCommand, index int) int {
	if index < 0 || index >= len(resolved.Values) {
		return 1
	}
	return int(resolved.Values[index])
}

func settingsPlayerFlag(player model.Player, names ...string) bool {
	return hasAnyNormalizedFlag(player.Metadata.Tags, names...)
}

func settingsCreatureFlag(creature model.Creature, names ...string) bool {
	return creatureHasAnyFlag(creature, names...)
}

func clearSettingsCreaturePropertyFlag(world SettingsWorld, creature model.Creature, tag string) (model.Creature, error) {
	if len(creature.Properties) == 0 {
		return creature, nil
	}
	targets := normalizedFlagSet(tag)
	updates := map[string]string{}
	for key, value := range creature.Properties {
		if _, ok := targets[normalizeFlagName(key)]; ok {
			updates[key] = ""
			continue
		}
		if !objectFlagContainerProperty(key) {
			continue
		}
		kept := make([]string, 0)
		removed := false
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || r == ' '
		}) {
			if _, ok := targets[normalizeFlagName(token)]; ok {
				removed = true
				continue
			}
			kept = append(kept, token)
		}
		if removed {
			updates[key] = strings.Join(kept, "|")
		}
	}
	updated := creature
	for key, value := range updates {
		next, err := world.SetCreatureProperty(creature.ID, key, value)
		if err != nil {
			return model.Creature{}, err
		}
		updated = next
	}
	return updated, nil
}

func settingOnStatus(on bool, yes, no string) string {
	if on {
		return yes
	}
	return no
}

func settingUnsetStatus(flag bool, yesWhenClear, noWhenSet string) string {
	if flag {
		return noWhenSet
	}
	return yesWhenClear
}

func settingsActiveEffects(creature model.Creature) string {
	messages := []struct {
		flag string
		text string
	}{
		{"PHASTE", "  당신은 활보법으로 기를 운행하고 있습니다\n"},
		{"PPRAYD", "  당신은 신의 보호를 받고있습니다\n"},
		{"PPOWER", "  당신은 기공으로 힘을 모으고 있습니다\n"},
		{"PSLAYE", "  당신의 무기에 살기가 감돕니다\n"},
		{"PMEDIT", "  당신은 참선으로 사물을 꿰뚤어봅니다\n"},
		{"PANGEL", "  당신은 정령소환술을 사용중입니다.\n"},
		{"PREFLECT", "  당신은 반탄강기를 행하고 있습니다.\n"},
	}
	var b strings.Builder
	for _, message := range messages {
		if settingsCreatureFlag(creature, message.flag) {
			b.WriteString(message.text)
		}
	}
	return b.String()
}
