package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

var timeNow = time.Now

type CastWorld interface {
	StatusWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

type CastEffectFunc func(*Context, CastWorld, model.Creature, ResolvedCommand, int) (bool, error)

type castSpell struct {
	power int
	name  string
	cost  int
}

func NewCastHandler(world CastWorld, effect CastEffectFunc) Handler {
	if effect == nil {
		effect = defaultCastMagicEffect
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		if len(resolved.Args) == 0 && !castInputHasLeadingNumber(resolved) {
			ctx.WriteString("어떤 주술을 펼치실겁니까?")
			return StatusDefault, nil
		}
		if creatureHasAnyFlag(creature, "blind", "pblind") {
			ctx.WriteString("아무것도 보이지 않습니다!")
			return StatusDefault, nil
		}
		if creatureHasAnyFlag(creature, "silenced", "silence", "psilnc") {
			ctx.WriteString("한마디도 할수 없습니다!")
			return StatusDefault, nil
		}

		spell, effectResolved, ok, ambiguous := resolveCastSpell(resolved)
		if !ok {
			if ambiguous {
				ctx.WriteString("펼치실 주문의 이름이 이상하군요.")
				return StatusDefault, nil
			}
			ctx.WriteString("그런 주문은 존재하지 않습니다.")
			return StatusDefault, nil
		}
		room, ok := world.Room(player.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("cast: room %q not found", player.RoomID)
		}
		if roomHasAnyFlag(room, "noMagic", "rnomag") {
			ctx.WriteString("주술을 출수 하는데 실패 하셨습니다.")
			return StatusDefault, nil
		}
		class := creatureClass(creature)
		now := timeNow().Unix()
		if class != model.ClassDM && class != model.ClassSubDM {
			remaining, used, err := world.UseCreatureCooldown(creature.ID, "spell", now, 0)
			if err != nil {
				return StatusDefault, err
			}
			if !used {
				ctx.WriteString(renderPleaseWait(remaining))
				return StatusDefault, nil
			}
		}

		// C cast() clears PHIDDN here (magic1.c:88): after the please_wait cooldown
		// gate but before the offensive-class / MP / learned checks, so a cast that
		// fails any of those still reveals a hidden caster. C skips the clear only
		// when the cooldown gate itself blocks the cast (returns before F_CLR).
		player, creature, err = clearCommandActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}

		if current := creatureStat(creature, "mpCurrent"); current < spell.cost && !castEffectHandlesMPCheck(spell.power) {
			ctx.WriteString(castNotEnoughMPMessage(spell.power))
			return StatusDefault, nil
		}

		if _, isOffensive := magicEffectDamageDiceForPower(spell.power); isOffensive {
			// C cast() (magic1.c:92) bars only the unclassed ZONEMAKER (class 0) from
			// offensive spells; FIGHTER (검사, class 4) is a real caster class with its
			// own spell_fail case (magic8.c) and may cast them (at low success).
			if class == model.ClassZoneMaker {
				ctx.WriteString("당신은 공격주문을 쓸 수 없는 직업을 갖고 있습니다.")
				return StatusDefault, nil
			}
		}

		// Enforce spell learning (S_ISSET equivalent) for CAST path to match C magic*.c checks.
		// DMs bypass; potions/scrolls/zap bypass as in C (guarded by how==CAST).
		// Uses studySpells mapping for exact tag fidelity.
		if tag := spellTagForPower(spell.power); tag != "" && class < model.ClassDM && !castEffectHandlesLearnedCheck(spell.power) {
			if !castActorKnowsSpell(player, creature, studySpell{power: spell.power, tag: tag}) {
				ctx.WriteString(castUnlearnedMessage(spell.power))
				return StatusDefault, nil
			}
		}

		success, err := effect(ctx, world, creature, effectResolved, spell.power)
		if err != nil {
			return StatusDefault, err
		}
		if !success {
			return StatusDefault, nil
		}

		costCreature := creature
		if refreshed, ok := world.Creature(creature.ID); ok {
			costCreature = refreshed
		}
		if !castEffectHandlesCostDeduction(spell.power) {
			if err := world.SetCreatureStat(creature.ID, "mpCurrent", creatureStat(costCreature, "mpCurrent")-spell.cost); err != nil {
				return StatusDefault, err
			}
		}

		var cooldownSec int64
		switch {
		case class == model.ClassCleric || class == model.ClassMage || class == model.ClassCaretaker:
			cooldownSec = 3
		case class == model.ClassBulsa:
			cooldownSec = 2
		case class == model.ClassDM || class == model.ClassSubDM:
			cooldownSec = 1
		default:
			cooldownSec = 5
		}
		if err := world.SetCreatureCooldown(creature.ID, "spell", now, cooldownSec); err != nil {
			return StatusDefault, err
		}

		return StatusDefault, nil
	}
}

func defaultCastMagicEffect(
	ctx *Context,
	world CastWorld,
	creature model.Creature,
	resolved ResolvedCommand,
	magicPower int,
) (bool, error) {
	return applyMagicPowerEffect(ctx, world, creature, model.ObjectInstance{}, resolved, magicPower, true)
}

func castActorKnowsSpell(player model.Player, creature model.Creature, spell studySpell) bool {
	if studyActorKnowsSpell(player, creature, spell) {
		return true
	}
	for _, tag := range castLearnedSpellAliases[spell.power] {
		aliasSpell := studySpell{power: spell.power, tag: tag}
		if studyActorKnowsSpell(player, creature, aliasSpell) {
			return true
		}
	}
	return false
}

func resolveCastSpell(resolved ResolvedCommand) (castSpell, ResolvedCommand, bool, bool) {
	if target, ok := castLeadingNumericSpell(resolved); ok {
		spell, found := castSpellByPower(target.power)
		if !found {
			return castSpell{}, ResolvedCommand{}, false, false
		}
		return spell, castEffectResolved(resolved, spell.name, target.argStart), true, false
	}

	token := getArg(resolved, 0)
	if token == "" {
		return castSpell{}, ResolvedCommand{}, false, false
	}
	if power, ok := parseCastSpellNumber(token); ok {
		spell, found := castSpellByPower(power)
		if !found {
			return castSpell{}, ResolvedCommand{}, false, false
		}
		return spell, castEffectResolved(resolved, spell.name, 1), true, false
	}

	spell, ok, ambiguous := castSpellByName(token)
	if !ok {
		return castSpell{}, ResolvedCommand{}, false, ambiguous
	}
	return spell, castEffectResolved(resolved, spell.name, 1), true, false
}

type castNumericSpellTarget struct {
	power    int
	argStart int
}

func castLeadingNumericSpell(resolved ResolvedCommand) (castNumericSpellTarget, bool) {
	token, ok := castLeadingNumberToken(resolved)
	if !ok {
		return castNumericSpellTarget{}, false
	}
	power, ok := parseCastSpellNumber(token)
	if !ok {
		return castNumericSpellTarget{}, false
	}
	if first := getArg(resolved, 0); first != "" {
		if _, ok, _ := castSpellByName(first); ok {
			return castNumericSpellTarget{}, false
		}
	}
	argStart := 0
	if getArg(resolved, 0) == token {
		argStart = 1
	}
	return castNumericSpellTarget{power: power, argStart: argStart}, true
}

func castNotEnoughMPMessage(power int) string {
	switch power {
	case magicPowerRecall:
		return "\n당신이 도력이 부족합니다.\n"
	case magicPowerBless, magicPowerEnchant, magicPowerRemoveCurse, magicPowerCurse, magicPowerSummon, magicPowerMagicTrack:
		return "\n당신의 도력이 부족합니다.\n"
	default:
		return "당신의 도력이 부족합니다.\n"
	}
}

func castUnlearnedMessage(power int) string {
	switch power {
	case magicPowerBless:
		return "\n당신은 아직 그런 주술을 터득하지 못했습니다.\n"
	case magicPowerProtection:
		return "\n당신은 아직 그 주술을 터득하지 못했습니다.\n"
	case magicPowerEnchant, magicPowerSummon, magicPowerMagicTrack, magicPowerFullHeal:
		return "\n당신은 아직 그런 주술을 터득하지 못했습니다.\n"
	case magicPowerRecall, magicPowerRemoveCurse, magicPowerCurse:
		return "\n당신은 아직 그런 주문을 터득하지 못했습니다.\n"
	default:
		return "\n당신은 아직 그 주문을 터득하지 못했습니다.\n"
	}
}

func castEffectHandlesLearnedCheck(power int) bool {
	switch power {
	case magicPowerRecall, magicPowerMagicTrack, magicPowerFullHeal, magicPowerRemoveDisease, magicPowerRemoveBlindness,
		magicPowerLocatePlayer, magicPowerDrainExp, magicPowerRoomVigor, magicPowerObjectSend, magicPowerRmGong:
		return true
	default:
		return false
	}
}

func castEffectHandlesMPCheck(power int) bool {
	switch power {
	case magicPowerLocatePlayer, magicPowerDrainExp, magicPowerRoomVigor, magicPowerObjectSend, magicPowerRmGong:
		return true
	default:
		return false
	}
}

func castEffectHandlesCostDeduction(power int) bool {
	if _, offensive := magicEffectDamageDiceForPower(power); offensive {
		return true
	}
	switch power {
	case magicPowerRecall, magicPowerSummon, magicPowerFullHeal, magicPowerMagicTrack,
		magicPowerLocatePlayer, magicPowerDrainExp, magicPowerRoomVigor,
		magicPowerCharm, magicPowerRmGong,
		magicPowerLevitate, magicPowerResistFire, magicPowerFly, magicPowerResistMagic, magicPowerKnowAlignment,
		magicPowerResistCold, magicPowerBreatheWater, magicPowerEarthShield, magicPowerRemoveDisease,
		magicPowerRemoveBlindness,
		magicPowerRestore, magicPowerTeleport, magicPowerEnchant, magicPowerRemoveCurse, magicPowerCurse,
		magicPowerFear, magicPowerBlind, magicPowerSilence, magicPowerObjectSend:
		return true
	default:
		return false
	}
}

func castInputHasLeadingNumber(resolved ResolvedCommand) bool {
	_, ok := castLeadingNumberToken(resolved)
	return ok
}

func castLeadingNumberToken(resolved ResolvedCommand) (string, bool) {
	tokens := strings.FieldsFunc(resolved.Input, func(r rune) bool {
		return r == ' ' || r == '#'
	})
	if len(tokens) == 0 {
		return "", false
	}
	index := 0
	if tokens[0] == resolved.Command() {
		index = 1
	}
	if index >= len(tokens) {
		return "", false
	}
	if _, err := strconv.Atoi(tokens[index]); err != nil {
		return "", false
	}
	return tokens[index], true
}

func parseCastSpellNumber(token string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(token))
	if err != nil {
		return 0, false
	}
	if n == 0 {
		return magicPowerVigor, true
	}
	if n < 0 {
		return 0, false
	}
	return n, true
}

func castEffectResolved(resolved ResolvedCommand, spellName string, argStart int) ResolvedCommand {
	next := resolved
	if argStart < 0 {
		argStart = 0
	}
	args := []string{spellName}
	values := []int64{1}
	if argStart < len(resolved.Args) {
		args = append(args, resolved.Args[argStart:]...)
		for i := argStart; i < len(resolved.Args); i++ {
			values = append(values, getOrdinal(resolved, i))
		}
	}
	next.Args = args
	next.Values = values
	return next
}

func castSpellByPower(power int) (castSpell, bool) {
	for _, spell := range supportedCastSpells {
		if spell.power == power {
			return spell, true
		}
	}
	return castSpell{}, false
}

func castSpellByName(name string) (castSpell, bool, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return castSpell{}, false, false
	}

	var matched castSpell
	matches := map[int]struct{}{}
	for _, alias := range supportedCastSpellAliases {
		if alias.name == name {
			return alias.spell, true, false
		}
		if strings.HasPrefix(alias.name, name) {
			matches[alias.spell.power] = struct{}{}
			matched = alias.spell
		}
	}
	switch len(matches) {
	case 0:
		return castSpell{}, false, false
	case 1:
		return matched, true, false
	default:
		return castSpell{}, false, true
	}
}

type castSpellAlias struct {
	name  string
	spell castSpell
}

var supportedCastSpells = []castSpell{
	{power: magicPowerVigor, name: "회복", cost: 5},
	{power: magicPowerHurt, name: "삭풍", cost: 3},
	{power: magicPowerLight, name: "발광", cost: 5},
	{power: magicPowerCurePoison, name: "해독", cost: 6},
	{power: magicPowerBless, name: "성현진", cost: 10},
	{power: magicPowerProtection, name: "수호진", cost: 10},
	{power: magicPowerFireball, name: "화궁", cost: 7},
	{power: magicPowerInvisibility, name: "은둔법", cost: 15},
	{power: magicPowerDetectInvisible, name: "은둔감지술", cost: 10},
	{power: magicPowerDetectMagic, name: "주문감지술", cost: 10},
	{power: magicPowerBefuddle, name: "혼동", cost: 10},
	{power: magicPowerRecall, name: "귀환", cost: 30},
	{power: magicPowerFullHeal, name: "완치", cost: 50},
	{power: magicPowerLevitate, name: "부양술", cost: 10},
	{power: magicPowerResistFire, name: "방열진", cost: 12},
	{power: magicPowerFly, name: "비상술", cost: 15},
	{power: magicPowerResistMagic, name: "보마진", cost: 12},
	{power: magicPowerShockbolt, name: "권풍술", cost: 10},
	{power: magicPowerLightning, name: "뇌전", cost: 15},
	{power: magicPowerIceBlade, name: "동설주", cost: 25},
	{power: magicPowerRumble, name: "지동술", cost: 3},
	{power: magicPowerBurn, name: "화선도", cost: 3},
	{power: magicPowerBlister, name: "탄수공", cost: 3},
	{power: magicPowerDustGust, name: "풍마현", cost: 7},
	{power: magicPowerWaterBolt, name: "파초식", cost: 7},
	{power: magicPowerStoneCrush, name: "폭진", cost: 7},
	{power: magicPowerKnowAlignment, name: "선악감지", cost: 6},
	{power: magicPowerResistCold, name: "방한진", cost: 12},
	{power: magicPowerBreatheWater, name: "수생술", cost: 12},
	{power: magicPowerEarthShield, name: "지방호", cost: 12},
	{power: magicPowerRemoveDisease, name: "치료", cost: 12},
	{power: magicPowerRemoveBlindness, name: "개안술", cost: 12},
	{power: magicPowerFear, name: "공포", cost: 15},
	{power: magicPowerBlind, name: "실명", cost: 15},
	{power: magicPowerSilence, name: "봉합구", cost: 12},
	{power: magicPowerRestore, name: "도주천", cost: 20},
	{power: magicPowerTeleport, name: "축지법", cost: 20},
	{power: magicPowerEnchant, name: "빙의", cost: 25},
	{power: magicPowerSummon, name: "소환", cost: 50},
	{power: magicPowerMend, name: "원기회복", cost: 10},
	{power: magicPowerMagicTrack, name: "추적", cost: 13},
	{power: magicPowerRemoveCurse, name: "저주해소", cost: 18},
	{power: magicPowerLocatePlayer, name: "천리안", cost: 15},
	{power: magicPowerDrainExp, name: "백치술", cost: 25},
	{power: magicPowerRoomVigor, name: "전회복", cost: 12},
	{power: magicPowerObjectSend, name: "전송", cost: 25},
	{power: magicPowerCharm, name: "이혼대법", cost: 15},
	{power: magicPowerCurse, name: "저주", cost: 25},
	{power: magicPowerRmGong, name: "공포해소", cost: 100},
	// Tier 3, 4, and 5 offensive spells
	{power: magicPowerEngulf, name: "낙석", cost: 10},
	{power: magicPowerBurstFlame, name: "화풍술", cost: 10},
	{power: magicPowerSteamBlast, name: "화룡대천", cost: 10},
	{power: magicPowerShatterStone, name: "토합술", cost: 15},
	{power: magicPowerImmolate, name: "주작현", cost: 15},
	{power: magicPowerBloodBoil, name: "열사천", cost: 15},
	{power: magicPowerThunderbolt, name: "파천풍", cost: 25},
	{power: magicPowerEarthquake, name: "지옥패", cost: 25},
	{power: magicPowerFlameFill, name: "태양안", cost: 25},
	// High tier offensive spells from C ospell (level 6/7) - exact MP from C magic tables
	{power: magicPowerSisix1, name: "천지진동", cost: 35},
	{power: magicPowerSisix2, name: "천상풍", cost: 35},
	{power: magicPowerSisix3, name: "천마강기", cost: 35},
	{power: magicPowerSisix4, name: "빙천파", cost: 35},
	{power: magicPowerXixix1, name: "혈사천", cost: 60},
	{power: magicPowerXixix2, name: "빙설검", cost: 60},
	{power: magicPowerXixix3, name: "멸겁화궁", cost: 60},
	{power: magicPowerXixix4, name: "탄지수통", cost: 60},
}

var supportedCastSpellAliases = buildCastSpellAliases()

var castLearnedSpellAliases = map[int][]string{
	magicPowerDetectMagic: {"SDETEC"},
}

func buildCastSpellAliases() []castSpellAlias {
	extra := map[int][]string{
		magicPowerBless:         {"성현"},
		magicPowerInvisibility:  {"은둔술"},
		magicPowerDetectMagic:   {"주문감지"},
		magicPowerBefuddle:      {"혼동술"},
		magicPowerResistCold:    {"추위보호"},
		magicPowerRemoveDisease: {"질병치료"},
		magicPowerRestore:       {"도력반"},
		magicPowerEnchant:       {"마법부여"},
		magicPowerLocatePlayer:  {"인물탐지", "위치파악", "투시"},
		magicPowerDrainExp:      {"경험치흡수"},
		magicPowerCharm:         {"최면", "매혹"},
	}

	aliases := make([]castSpellAlias, 0, len(supportedCastSpells))
	seen := map[int]map[string]struct{}{}
	for _, spell := range supportedCastSpells {
		names := append([]string{spell.name}, extra[spell.power]...)
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if seen[spell.power] == nil {
				seen[spell.power] = map[string]struct{}{}
			}
			if _, ok := seen[spell.power][name]; ok {
				continue
			}
			seen[spell.power][name] = struct{}{}
			aliases = append(aliases, castSpellAlias{name: name, spell: spell})
		}
	}
	return aliases
}
