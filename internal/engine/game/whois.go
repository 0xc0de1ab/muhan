package game

import (
	"fmt"
	"strings"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	defaultWhoisAgeYears = 18
)

type WhoisWorld interface {
	PlayerLookup
	Creature(model.CreatureID) (model.Creature, bool)
}

func NewWhoisHandler(world WhoisWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		if len(resolved.Args) == 0 || strings.TrimSpace(resolved.Args[0]) == "" {
			ctx.WriteString("누구를 검색하시려구요?")
			return enginecmd.StatusDefault, nil
		}

		actorCreature, _ := playerCreature(world, model.PlayerID(ctx.ActorID))
		target, targetName, ok := findWhoisActivePlayer(world, active(), resolved.Args[0])
		if !ok {
			ctx.WriteString("현재 이용중이 아닙니다.")
			return enginecmd.StatusDefault, nil
		}
		player, ok := world.Player(model.PlayerID(target.ActorID))
		if !ok || player.CreatureID.IsZero() {
			ctx.WriteString("현재 이용중이 아닙니다.")
			return enginecmd.StatusDefault, nil
		}
		creature, ok := world.Creature(player.CreatureID)
		if !ok || whoisHiddenFrom(actorCreature, creature) {
			ctx.WriteString("현재 이용중이 아닙니다.")
			return enginecmd.StatusDefault, nil
		}
		if strings.TrimSpace(creature.DisplayName) != "" {
			targetName = strings.TrimSpace(creature.DisplayName)
		}

		ctx.WriteString(renderLegacyColorForContext(ctx, "{노}"+renderWhoisLine(targetName, creature)+"}"))
		return enginecmd.StatusDefault, nil
	}
}

func findWhoisActivePlayer(world PlayerLookup, sessions []ActiveSession, target string) (ActiveSession, string, bool) {
	target = legacyWhoisLookupName(target)
	if target == "" {
		return ActiveSession{}, "", false
	}
	for _, activeSession := range sessions {
		if activeSession.ActorID == "" {
			continue
		}
		name, ok := activePlayerLookupName(world, activeSession.ActorID)
		if ok && name == target {
			return activeSession, name, true
		}
	}
	return ActiveSession{}, "", false
}

func legacyWhoisLookupName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	bytes := []byte(name)
	for i, b := range bytes {
		if b >= 'A' && b <= 'Z' {
			bytes[i] = b + ('a' - 'A')
		}
	}
	if bytes[0] >= 'a' && bytes[0] <= 'z' {
		bytes[0] -= 'a' - 'A'
	}
	return string(bytes)
}

func whoisHiddenFrom(actor model.Creature, target model.Creature) bool {
	if creatureFlagEnabled(target, "PDMINV", "dmInvisible") {
		return true
	}
	if creatureFlagEnabled(actor, "PBLIND", "blind") {
		return true
	}
	if creatureFlagEnabled(target, "PINVIS", "invisible") && !creatureFlagEnabled(actor, "PDINVI", "detectInvisible") {
		return true
	}
	return false
}

func renderWhoisLine(name string, creature model.Creature) string {
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSpace(creature.DisplayName)
	}
	if name == "" {
		name = string(creature.ID)
	}
	level := creature.Level
	if level <= 0 {
		if value, ok := creatureIntValue(creature, "level"); ok {
			level = value
		}
	}
	class := creatureIntValueDefault(creature, "class", 0)
	race := creatureIntValueDefault(creature, "race", 0)
	title := whoisTitle(creature, level)
	return fmt.Sprintf(
		"%s  %s [레벨] %s %s  %s  %s\n"+
			"----------------------------------------------------------------------------\n"+
			"%s  %s [ %02d ] %s %s  %-4d  %s",
		legacyLeftWidthBytes("사용자", 18),
		legacyLeftWidthBytes("성별", 4),
		legacyLeftWidthBytes("직업", 4),
		legacyLeftWidthBytes("칭호", 20),
		legacyLeftWidthBytes("나이", 4),
		legacyLeftWidthBytes("종족", 10),
		legacyLeftWidthBytes(name, 18),
		legacyLeftWidthBytes(whoisGender(creature), 4),
		level,
		legacyFixedByteLabel(shortWhoisClassName(whoisClassName(class)), 4),
		legacyLeftWidthBytes(title, 20),
		whoisAgeYears(creature),
		legacyLeftWidthBytes(whoisRaceName(race), 10),
	)
}

func whoisGender(creature model.Creature) string {
	if creatureFlagEnabled(creature, "PMALES", "male") {
		return " 남"
	}
	return " 여"
}

func whoisAgeYears(creature model.Creature) int {
	if value, ok := creatureIntValue(creature, "legacyAgeYears"); ok && value >= defaultWhoisAgeYears {
		return value
	}
	if value, ok := creatureIntValue(creature, "legacyHoursInterval"); ok && value >= 0 {
		return defaultWhoisAgeYears + value/86400
	}
	return defaultWhoisAgeYears
}

func whoisTitle(creature model.Creature, level int) string {
	if title := strings.TrimSpace(creature.Properties["legacyTitle"]); title != "" {
		return title
	}
	class := creatureIntValueDefault(creature, "class", 0)
	if class < 0 || class >= len(whoisLevelTitles) {
		class = 0
	}
	titleIndex := (((level + 3) / 4) - 1) / 3
	if titleIndex < 0 {
		titleIndex = 0
	}
	if titleIndex > 7 {
		titleIndex = 7
	}
	return whoisLevelTitles[class][titleIndex]
}

func whoisClassName(class int) string {
	if class >= 0 && class < len(whoisClassNames) {
		return whoisClassNames[class]
	}
	return whoisClassNames[0]
}

func shortWhoisClassName(name string) string {
	runes := []rune(strings.TrimSpace(name))
	if len(runes) <= 2 {
		return string(runes)
	}
	return string(runes[:2])
}

func whoisRaceName(race int) string {
	if race >= 0 && race < len(whoisRaceNames) {
		return whoisRaceNames[race]
	}
	return whoisRaceNames[0]
}

func creatureIntValueDefault(creature model.Creature, key string, fallback int) int {
	if value, ok := creatureIntValue(creature, key); ok {
		return value
	}
	return fallback
}

var whoisClassNames = []string{
	"바보", "자객", "권법가", "불제자", "검사", "도술사", "무사", "포졸", "도둑", "무적", "초인", "불사", "운영자", "관리자",
}

var whoisRaceNames = []string{
	"바보족", "난장이족", "용신족", "요괴족", "토신족", "인간족", "도깨비족", "거인족", "땅귀신족", "개구리족",
}

var whoisLevelTitles = [][]string{
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
