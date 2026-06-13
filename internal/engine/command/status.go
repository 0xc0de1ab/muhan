package command

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/textfmt"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type StatusWorld interface {
	InventoryWorld
	Room(model.RoomID) (model.Room, bool)
}

func NewEquipmentHandler(world InventoryWorld) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(renderEquipment(world, player, creature, textOptionsFromContext(ctx)))
		return StatusDefault, nil
	}
}

func NewHealthHandler(world InventoryWorld) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(RenderHealth(player, creature))
		return StatusDefault, nil
	}
}

func NewInfoHandler(world StatusWorld) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(RenderPlayerInfo(world, player, creature))
		if SetPendingLineHandler(ctx, (&infoReadState{world: world, playerID: player.ID}).handleLine) {
			return StatusDoPrompt, nil
		}
		return StatusDefault, nil
	}
}

func NewWhereHandler(world StatusWorld) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(RenderWhereList(ctx, world, player, creature))
		return StatusDefault, nil
	}
}

func NewEffectStatusHandler(world InventoryWorld) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(RenderEffectStatus(player, creature))
		return StatusDefault, nil
	}
}

func NewTimeHandler(now func() time.Time) Handler {
	return NewTimeHandlerWithWorld(nil, now)
}

type TimeWorld interface {
	LegacyTime() int64
}

func NewTimeHandlerWithWorld(world TimeWorld, now func() time.Time) Handler {
	if now == nil {
		now = time.Now
	}
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		current := now()
		if world != nil {
			ctx.WriteString(RenderTimeWithLegacyTime(current, world.LegacyTime()))
			return StatusDefault, nil
		}
		ctx.WriteString(RenderTime(current))
		return StatusDefault, nil
	}
}

func RenderEquipment(world InventoryWorld, creature model.Creature) string {
	return renderEquipment(world, model.Player{}, creature, textfmt.Options{})
}

func renderEquipment(world InventoryWorld, player model.Player, creature model.Creature, opts textfmt.Options) string {
	var b strings.Builder
	slots := orderedEquipmentSlots(creature.Equipment, legacyEquipmentDisplaySlotOrder)
	if len(slots) == 0 {
		b.WriteString(legacyEquipmentEmptyMessage)
		return b.String()
	}

	if statusEffectActive(player, creature, "blind", "blinded", "PBLIND", "MBLIND") {
		b.WriteString(colorText(opts, "31", legacyEquipmentBlindMessage))
		return b.String()
	}

	b.WriteString(colorText(opts, "34", "  <<<  착용 장비  >>>  \n"))
	writeLegacyEquipmentList(&b, world, creature, opts)
	return b.String()
}

func writeLegacyEquipmentList(b *strings.Builder, world InventoryWorld, creature model.Creature, opts textfmt.Options) {
	if b == nil || world == nil {
		return
	}
	slots := orderedEquipmentSlots(creature.Equipment, legacyEquipmentDisplaySlotOrder)
	for _, slot := range slots {
		objectID := creature.Equipment[slot]
		label, ok := legacyEquipmentSlotLabel(slot)
		if ok {
			b.WriteString(colorText(opts, "33", label))
			b.WriteString("  ")
			b.WriteString(inventoryObjectName(world, objectID))
		} else {
			b.WriteString(slot)
			b.WriteString(": ")
			b.WriteString(inventoryObjectName(world, objectID))
		}
		b.WriteByte('\n')
	}
}

const (
	legacyEquipmentEmptyMessage = "당신은 걸치고 있는게 아무것도 없습니다."
	legacyEquipmentBlindMessage = "당신은 아무것도 볼수가 없습니다. 당신은 눈이 멀어 있습니다."
)

func legacyEquipmentSlotLabel(slot string) (string, bool) {
	switch slot {
	case "head":
		return "[ 머리 ]", true
	case "face":
		return "[ 얼굴 ]", true
	case "neck1", "neck2":
		return "[  목  ]", true
	case "body":
		return "[  몸  ]", true
	case "arms":
		return "[  팔  ]", true
	case "hands":
		return "[  손  ]", true
	case "finger1", "finger2", "finger3", "finger4", "finger5", "finger6", "finger7", "finger8":
		return "[손가락]", true
	case "legs":
		return "[ 다리 ]", true
	case "feet":
		return "[  발  ]", true
	case "held":
		return "[쥔물건]", true
	case "shield":
		return "[ 방패 ]", true
	case "wield":
		return "[ 무기 ]", true
	default:
		return "", false
	}
}

func RenderHealth(player model.Player, creature model.Creature) string {
	if statusEffectActive(player, creature, "blind", "blinded", "PBLIND", "MBLIND") {
		return "당신은 눈이 멀어 있습니다!"
	}

	var b strings.Builder
	name := statusPlayerName(player, creature)
	level := attackCreatureLevel(creature)
	if level <= 0 {
		level = 1
	}

	fmt.Fprintf(&b, "%s : %s (레벨 %d)", name, statusCreatureTitle(creature), level)
	for _, marker := range legacyHealthMarkers {
		if statusEffectActive(player, creature, marker.aliases...) {
			b.WriteString(marker.text)
		}
	}

	hp := fmt.Sprintf("%d/%d", creatureStat(creature, "hpCurrent"), creatureStat(creature, "hpMax"))
	mp := fmt.Sprintf("%d/%d", creatureStat(creature, "mpCurrent"), creatureStat(creature, "mpMax"))
	fmt.Fprintf(&b, "\n [체  력] %-16s", hp)
	fmt.Fprintf(&b, " [도  력] %-16s", mp)
	fmt.Fprintf(&b, "[방어력] %d\n", 100-creatureStat(creature, "armor"))

	experience := creatureStat(creature, "experience")
	gold := creatureStat(creature, "gold")
	class := creatureClass(creature)
	if class == model.ClassCaretaker || class == model.ClassBulsa {
		upgrade := 0
		if experience >= buyStatesExperienceBase {
			upgrade = experience - buyStatesExperienceBase
		}
		fmt.Fprintf(&b, " [향상치] %-16d [  돈  ] %-16d", upgrade, gold)
	} else {
		expNeeded, ok := trainRequiredExperience(level)
		if !ok {
			expNeeded = 0
		}
		fmt.Fprintf(&b, " [목표치] %-16d [  돈  ] %-16d", maxInt(0, expNeeded-experience), gold)
	}
	fmt.Fprintf(&b, "[용  기] %d\n", 20-creatureStat(creature, "thaco"))
	fmt.Fprintf(&b, "\n 당신은 %s있습니다.", statusCreatureDescription(creature))
	return b.String()
}

type legacyHealthMarker struct {
	text    string
	aliases []string
}

var legacyHealthMarkers = []legacyHealthMarker{
	{text: " *은신* ", aliases: []string{"hidden", "PHIDDN", "MHIDDN"}},
	{text: " *중독* ", aliases: []string{"poison", "poisoned", "PPOISN", "MPOISN"}},
	{text: " *최면* ", aliases: []string{"charm", "charmed", "PCHARM", "MCHARM"}},
	{text: " *벙어리* ", aliases: []string{"silence", "silenced", "PSILNC", "MSILNC"}},
	{text: " *질병* ", aliases: []string{"disease", "diseased", "PDISEA", "MDISEA"}},
}

func statusCreatureTitle(creature model.Creature) string {
	if title := strings.TrimSpace(creature.Properties[legacyTitleProperty]); title != "" {
		return cleanDisplayText(title)
	}
	return defaultCreatureTitle(creature)
}

func statusCreatureDescription(creature model.Creature) string {
	if description := textfmt.RenderLegacyColors(creature.Description, textfmt.Options{}); strings.TrimSpace(description) != "" {
		return description
	}
	if description := textfmt.RenderLegacyColors(creature.Properties["description"], textfmt.Options{}); strings.TrimSpace(description) != "" {
		return description
	}
	return ""
}

func RenderPlayerInfo(world StatusWorld, player model.Player, creature model.Creature) string {
	var b strings.Builder
	level := attackCreatureLevel(creature)
	if level <= 0 {
		level = 1
	}
	expNeeded, ok := trainRequiredExperience(level)
	if !ok {
		expNeeded = 0
	}
	weight, count := statusInventoryWeightAndCount(world, creature)
	fmt.Fprintf(&b, "\n[이  름] %s        [배우자] %s\n", statusPlayerName(player, creature), statusSpouseLabel(player, creature))
	fmt.Fprintf(&b, "[칭  호] %s\n\n", statusCreatureTitle(creature))
	fmt.Fprintf(&b, "[레  벨] %-20d       [종  족] %s\n", level, statusRaceName(creature))
	fmt.Fprintf(&b, "[직  업] %-20s       [성  향] %s %s\n",
		creatureClassName(creatureClass(creature)),
		statusAlignmentKind(creature),
		statusAlignmentDescription(creature),
	)
	b.WriteString("접속시간 : ")
	b.WriteString(statusPlayTime(creature))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "[  힘  ] %-2d      [민  첩] %-2d      [맷  집] %-2d\n",
		creatureStat(creature, "strength"),
		creatureStat(creature, "dexterity"),
		creatureStat(creature, "constitution"),
	)
	fmt.Fprintf(&b, "[지  식] %-2d      [신앙심] %-2d      [용  기] %-2d\n\n",
		creatureStat(creature, "intelligence"),
		creatureStat(creature, "piety"),
		20-creatureStat(creature, "thaco"),
	)
	fmt.Fprintf(&b, "[체  력] %-5d/%-5d          [경험치] %d ( %d의 경험치 필요)\n",
		creatureStat(creature, "hpCurrent"),
		creatureStat(creature, "hpMax"),
		creatureStat(creature, "experience"),
		maxInt(0, expNeeded-creatureStat(creature, "experience")),
	)
	fmt.Fprintf(&b, "[도  력] %-5d/%-5d          [  돈  ] %-7d\n",
		creatureStat(creature, "mpCurrent"),
		creatureStat(creature, "mpMax"),
		creatureStat(creature, "gold"),
	)
	fmt.Fprintf(&b, "[방어력] %-5d                [소지품 무게] %d 근 (총 %d개).\n\n",
		100-creatureStat(creature, "armor"),
		weight,
		count,
	)
	b.WriteString("## 무기사용능력 ##\n")
	fmt.Fprintf(&b, "\n[ 도 ] %2d%%         [ 검 ] %2d%%         [ 봉 ] %2d%%\n",
		statusWeaponProficiency(creature, 0),
		statusWeaponProficiency(creature, 1),
		statusWeaponProficiency(creature, 2),
	)
	fmt.Fprintf(&b, "[ 창 ] %2d%%         [ 궁 ] %2d%%\n\n",
		statusWeaponProficiency(creature, 3),
		statusWeaponProficiency(creature, 4),
	)
	b.WriteString("[엔터]를 누르세요. 그만보시려면 [.]을 치세요: ")
	return b.String()
}

type infoReadState struct {
	world    StatusWorld
	playerID model.PlayerID
}

func (s *infoReadState) handleLine(ctx *Context, line string) (Status, error) {
	if strings.HasPrefix(line, ".") {
		ClearPendingLineHandler(ctx)
		ctx.WriteString("중단되었습니다.\n")
		return StatusPrompt, nil
	}
	player, creature, err := CurrentInventoryCreature(s.world, s.playerID)
	if err != nil {
		ClearPendingLineHandler(ctx)
		return StatusDefault, err
	}
	ClearPendingLineHandler(ctx)
	ctx.WriteString(RenderPlayerInfoSpells(player, creature))
	return StatusPrompt, nil
}

func RenderPlayerInfoSpells(player model.Player, creature model.Creature) string {
	var b strings.Builder
	b.WriteString("\n## 주 술  계 열 ##\n\n")
	fmt.Fprintf(&b, "[ 땅 ] %2d%%      [바람] %2d%%    [ 불 ] %2d%%   [ 물 ] %2d%%\n\n",
		mprofic(creature, 1),
		mprofic(creature, 2),
		mprofic(creature, 3),
		mprofic(creature, 4),
	)
	b.WriteString("\n주문: ")
	spells := statusKnownSpellNames(player, creature)
	if len(spells) == 0 {
		b.WriteString("없음.\n")
	} else {
		b.WriteString(strings.Join(spells, ", "))
		b.WriteString(".\n")
	}
	b.WriteString("당신의 현주문: ")
	active := statusCurrentSpellNames(player, creature)
	if len(active) == 0 {
		b.WriteString("없음.\n")
	} else {
		b.WriteString(strings.Join(active, ", "))
		b.WriteString(".\n")
	}
	b.WriteString(statusQuestProgressLine(creature))
	if creatureClass(creature) >= model.ClassInvincible {
		b.WriteString("\n무적수련 : ")
		training := statusInvincibleTrainingNames(player, creature)
		if len(training) == 0 {
			b.WriteString("없음\n\n")
		} else {
			b.WriteString(strings.Join(training, " "))
			b.WriteString(" \n\n")
		}
	}
	return b.String()
}

func statusSpouseLabel(player model.Player, creature model.Creature) string {
	if !statusEffectActive(player, creature, "PMARRI", "married", "marriage", "marriageFlag") {
		return "없음"
	}
	if spouse := whereSpouseName(player, creature); spouse != "" {
		return cleanDisplayText(spouse)
	}
	return "없음"
}

func statusRaceName(creature model.Creature) string {
	return cleanDisplayText(creatureRaceName(creature))
}

func statusAlignmentKind(creature model.Creature) string {
	if creatureHasAnyFlag(creature, "PCHAOS", "chaos") {
		return "악"
	}
	return "선"
}

func statusAlignmentDescription(creature model.Creature) string {
	alignment := creatureAlignment(creature)
	switch {
	case alignment < -100:
		return " (악합니다)"
	case alignment < 101:
		return " (평범합니다)"
	default:
		return " (선합니다) "
	}
}

func statusPlayTime(creature model.Creature) string {
	interval := 0
	for _, key := range []string{"legacyHoursInterval", "LT_HOURS_interval", "lastHoursInterval"} {
		if value, ok := whereCreatureInt(creature, key); ok && value >= 0 {
			interval = value
			break
		}
	}

	parts := make([]string, 0, 3)
	if interval > 86400 {
		parts = append(parts, fmt.Sprintf("%d일", interval/86400))
	}
	if interval > 3600 {
		parts = append(parts, fmt.Sprintf("%d시간", (interval%86400)/3600))
	}
	parts = append(parts, fmt.Sprintf("%d분", (interval%3600)/60))
	return strings.Join(parts, " ")
}

func statusInventoryWeightAndCount(world InventoryWorld, creature model.Creature) (int, int) {
	seen := map[model.ObjectInstanceID]struct{}{}
	weight, count := 0, 0
	var visit func(model.ObjectInstanceID)
	visit = func(objectID model.ObjectInstanceID) {
		if objectID.IsZero() {
			return
		}
		if _, ok := seen[objectID]; ok {
			return
		}
		seen[objectID] = struct{}{}
		object, ok := world.Object(objectID)
		if !ok {
			return
		}
		count++
		quantity := object.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		if objectWeight, ok := objectIntProperty(world, object, "weight"); ok {
			weight += objectWeight * quantity
		}
		for _, childID := range object.Contents.ObjectIDs {
			visit(childID)
		}
	}

	for _, objectID := range creature.Equipment {
		visit(objectID)
	}
	for _, objectID := range creature.Inventory.ObjectIDs {
		visit(objectID)
	}
	return weight, count
}

func statusWeaponProficiency(creature model.Creature, idx int) int {
	raw := creatureProficiency(creature, idx)
	if raw < 0 {
		raw = 0
	}
	thresholds := statusWeaponProficiencyThresholds(creatureClass(creature))
	bracket := len(thresholds) - 2
	for i := 0; i < len(thresholds)-1; i++ {
		if raw < thresholds[i+1] {
			bracket = i
			break
		}
	}
	base := thresholds[bracket]
	next := thresholds[bracket+1]
	if next <= base {
		return bracket * 10
	}
	return bracket*10 + ((raw-base)*10)/(next-base)
}

func statusWeaponProficiencyThresholds(class int) []int {
	switch class {
	case model.ClassFighter, model.ClassInvincible, model.ClassCaretaker, model.ClassBulsa, model.ClassSubDM, model.ClassDM:
		return []int{0, 768, 1024, 1440, 1910, 16000, 31214, 167000, 268488, 695000, 934808, 500000000}
	case model.ClassBarbarian:
		return []int{0, 1536, 2048, 2880, 3820, 32000, 62428, 334000, 536976, 1390000, 1869616, 500000000}
	case model.ClassThief, model.ClassRanger:
		return []int{0, 2304, 3072, 4320, 5730, 48000, 93642, 501000, 805464, 2085000, 2804424, 500000000}
	case model.ClassCleric, model.ClassPaladin, model.ClassAssassin:
		return []int{0, 3072, 4096, 5076, 7640, 64000, 124856, 668000, 1073952, 2780000, 3939232, 500000000}
	case model.ClassMage:
		return []int{0, 5376, 7168, 10080, 13370, 112000, 218498, 1169000, 1879416, 4865000, 6543656, 500000000}
	default:
		return []int{0, 768, 1024, 1440, 1910, 16000, 31214, 167000, 268488, 695000, 934808, 500000000}
	}
}

func statusKnownSpellNames(player model.Player, creature model.Creature) []string {
	names := make([]string, 0, len(studySpells))
	for _, spell := range studySpells {
		if statusActorHasExactSpellTag(player, creature, spell.tag) {
			names = append(names, spell.displayName())
		}
	}
	sort.Strings(names)
	return names
}

func statusActorHasExactSpellTag(player model.Player, creature model.Creature, tag string) bool {
	tag = normalizeFlagName(tag)
	if tag == "" {
		return false
	}
	for _, candidate := range player.Metadata.Tags {
		if normalizeFlagName(candidate) == tag {
			return true
		}
	}
	for _, candidate := range creature.Metadata.Tags {
		if normalizeFlagName(candidate) == tag {
			return true
		}
	}
	for key, value := range creature.Stats {
		if value != 0 && normalizeFlagName(key) == tag {
			return true
		}
	}
	for key, value := range creature.Properties {
		if normalizeFlagName(key) == tag && statusTruthyString(value) {
			return true
		}
	}
	return false
}

func statusCurrentSpellNames(player model.Player, creature model.Creature) []string {
	specs := []struct {
		name    string
		aliases []string
	}{
		{name: "성현진", aliases: []string{"PBLESS", "bless", "blessed"}},
		{name: "발광", aliases: []string{"PLIGHT", "light"}},
		{name: "수호진", aliases: []string{"PPROTE", "protection", "protect", "protected"}},
		{name: "은둔법", aliases: []string{"PINVIS", "invisible", "invisibility"}},
		{name: "은둔감지", aliases: []string{"PDINVI", "detectInvisible", "detectInvis"}},
		{name: "주문감지", aliases: []string{"PDMAGI", "detectMagic", "dMagic"}},
		{name: "부양술", aliases: []string{"PLEVIT", "levitate", "levitation"}},
		{name: "방열진", aliases: []string{"PRFIRE", "resistFire", "fireResistance"}},
		{name: "비상술", aliases: []string{"PFLYSP", "fly", "flying"}},
		{name: "보마진", aliases: []string{"PRMAGI", "MRMAGI", "resistMagic", "magicResistance"}},
		{name: "선악감지", aliases: []string{"PKNOWA", "knowAlignment", "alignmentSense"}},
		{name: "방한진", aliases: []string{"PRCOLD", "resistCold", "coldResistance"}},
		{name: "수생술", aliases: []string{"PBRWAT", "breatheWater", "waterBreathing"}},
		{name: "지방호", aliases: []string{"PSSHLD", "earthShield", "stoneShield"}},
	}
	active := make([]string, 0, len(specs))
	for _, spec := range specs {
		if statusEffectActive(player, creature, spec.aliases...) {
			active = append(active, spec.name)
		}
	}
	return active
}

func statusQuestProgressLine(creature model.Creature) string {
	quest := 0
	for n := 1; n <= 128; n++ {
		if !statusQuestCompleted(creature, n) {
			break
		}
		quest = n
	}
	if quest == 0 {
		return "당신은 현재 달성한 임무가 없습니다."
	}
	return fmt.Sprintf("당신은 현재 임무 %d까지 달성하였습니다.", quest)
}

func statusQuestCompleted(creature model.Creature, number int) bool {
	keys := []string{
		fmt.Sprintf("quest_completed_%d", number),
		fmt.Sprintf("questCompleted%d", number),
		fmt.Sprintf("quest.completed.%d", number),
		fmt.Sprintf("quest/%d/completed", number),
	}
	for _, key := range keys {
		if value, ok := creature.Stats[key]; ok {
			return value != 0
		}
		if value := strings.TrimSpace(creature.Properties[key]); statusTruthyString(value) {
			return true
		}
		if raw := creature.Metadata.RawFields[key]; len(raw) != 0 && statusTruthyString(string(raw)) {
			return true
		}
	}
	tagAliases := []string{
		fmt.Sprintf("quest_completed_%d", number),
		fmt.Sprintf("questCompleted:%d", number),
		fmt.Sprintf("Q%d", number),
	}
	return hasAnyNormalizedFlag(creature.Metadata.Tags, tagAliases...)
}

func statusTruthyString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func statusInvincibleTrainingNames(player model.Player, creature model.Creature) []string {
	specs := []struct {
		name string
		tag  string
	}{
		{name: "자객", tag: "SASSASSIN"},
		{name: "권법가", tag: "SBARBARIAN"},
		{name: "불제자", tag: "SCLERIC"},
		{name: "검사", tag: "SFIGHTER"},
		{name: "도술사", tag: "SMAGE"},
		{name: "무사", tag: "SPALADIN"},
		{name: "포졸", tag: "SRANGER"},
		{name: "도둑", tag: "STHIEF"},
	}
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		if statusEffectActive(player, creature, spec.tag) {
			names = append(names, spec.name)
		}
	}
	return names
}

func RenderWhereList(ctx *Context, world StatusWorld, actorPlayer model.Player, actor model.Creature) string {
	if statusEffectActive(actorPlayer, actor, "blind", "blinded", "PBLIND", "MBLIND") {
		return "당신은 눈이 멀어 있습니다!\n"
	}

	entries := whereVisibleEntries(ctx, world, actorPlayer, actor)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s     레벨  %s  %s%s%s\n",
		legacyLeftWidthBytes("사용자", 13),
		legacyLeftWidthBytes("직업", 4),
		legacyLeftWidthBytes("성별", 7),
		legacyLeftWidthBytes("나이", 6),
		legacyLeftWidthBytes("장소", 32),
	))
	b.WriteString("----------------------------------------------------------------------\n")
	for _, entry := range entries {
		b.WriteString(renderWhereEntry(world, actorPlayer, actor, entry.player, entry.creature))
	}
	if len(entries) != 1 {
		b.WriteString(fmt.Sprintf("\n총 %d명의 사용자가 통계무한을 이용하고 있습니다.", len(entries)))
	} else {
		b.WriteString("\n당신 혼자서 외로이 통계무한을 이용하고 있습니다.")
	}
	return b.String()
}

type whereEntry struct {
	player   model.Player
	creature model.Creature
}

func whereVisibleEntries(ctx *Context, world StatusWorld, actorPlayer model.Player, actor model.Creature) []whereEntry {
	sessions := getActiveSessions(ctx)
	if len(sessions) == 0 && !actorPlayer.ID.IsZero() {
		sessions = []activeSession{{ActorID: string(actorPlayer.ID)}}
	}

	entries := make([]whereEntry, 0, len(sessions))
	seen := map[model.PlayerID]struct{}{}
	for _, session := range sessions {
		playerID := model.PlayerID(strings.TrimSpace(session.ActorID))
		if playerID.IsZero() {
			continue
		}
		if _, ok := seen[playerID]; ok {
			continue
		}
		seen[playerID] = struct{}{}

		player, ok := world.Player(playerID)
		if !ok || player.CreatureID.IsZero() {
			continue
		}
		creature, ok := world.Creature(player.CreatureID)
		if !ok || whereHiddenFrom(actorPlayer, actor, player, creature) {
			continue
		}
		entries = append(entries, whereEntry{player: player, creature: creature})
	}
	return entries
}

func whereHiddenFrom(actorPlayer model.Player, actor model.Creature, targetPlayer model.Player, target model.Creature) bool {
	actorClass := creatureClass(actor)
	if creatureHasAnyFlag(target, "PDMINV", "dmInvisible") && actorClass < model.ClassDM {
		return true
	}
	if creatureHasAnyFlag(target, "PINVIS", "invisible") &&
		!creatureHasAnyFlag(actor, "PDINVI", "detectInvisible") &&
		actorClass < model.ClassDM &&
		targetPlayer.ID != actorPlayer.ID {
		return true
	}
	return false
}

func renderWhereEntry(world StatusWorld, actorPlayer model.Player, actor model.Creature, player model.Player, creature model.Creature) string {
	var b strings.Builder
	name := strings.TrimSpace(player.DisplayName)
	if name == "" {
		name = strings.TrimSpace(creature.DisplayName)
	}
	if name == "" {
		name = string(player.ID)
	}
	invis := creatureHasAnyFlag(creature, "PDMINV", "dmInvisible") || creatureHasAnyFlag(creature, "PINVIS", "invisible")
	marker := "   "
	if invis {
		marker = "(*)"
	}
	level := creature.Level
	if level <= 0 {
		level = creatureStat(creature, "level")
	}
	classLabel := legacyFixedByteLabel(creatureClassName(creatureClass(creature)), 4)
	gender := "여"
	if creatureHasAnyFlag(creature, "PMALES", "male") || creatureStat(creature, "PMALES") != 0 {
		gender = "남"
	}
	fmt.Fprintf(&b, "%s%s [%s%02d ] %s   %s [ %d ] ",
		legacyLeftWidthBytes(name, 13),
		marker,
		whereLevelPrefix(level),
		level,
		classLabel,
		legacyLeftWidthBytes(gender, 4),
		whereAgeYears(creature),
	)
	if whereCanSeeRoom(actorPlayer, actor, player, creature) {
		b.WriteString(statusRoomName(world, wherePlayerRoomID(player, creature)))
	}
	b.WriteString("\n")
	return b.String()
}

func whereLevelPrefix(level int) string {
	if level >= 100 {
		return ""
	}
	return " "
}

func wherePlayerRoomID(player model.Player, creature model.Creature) model.RoomID {
	if !creature.RoomID.IsZero() {
		return creature.RoomID
	}
	return player.RoomID
}

func whereCanSeeRoom(actorPlayer model.Player, actor model.Creature, targetPlayer model.Player, target model.Creature) bool {
	targetClass := creatureClass(target)
	targetMarried := statusEffectActive(targetPlayer, target, "PMARRI", "married", "marriage", "marriageFlag")
	if targetClass <= model.ClassInvincible && !targetMarried {
		return true
	}
	actorClass := creatureClass(actor)
	if actorClass > model.ClassInvincible && !targetMarried {
		return true
	}
	if actorClass >= model.ClassDM {
		return true
	}
	if actorPlayer.ID == targetPlayer.ID {
		return true
	}
	if statusEffectActive(actorPlayer, actor, "PMARRI", "married", "marriage", "marriageFlag") &&
		strings.EqualFold(whereSpouseName(actorPlayer, actor), wherePlayerName(targetPlayer, target)) {
		return true
	}
	return false
}

func whereSpouseName(player model.Player, creature model.Creature) string {
	for _, key := range []string{"spouse", "spouseName", "marriedTo", "marriageSpouse"} {
		if value := strings.TrimSpace(creature.Properties[key]); value != "" {
			return value
		}
		if raw := creature.Metadata.RawFields[key]; len(raw) != 0 {
			if value := strings.TrimSpace(string(raw)); value != "" {
				return value
			}
		}
	}
	return ""
}

func wherePlayerName(player model.Player, creature model.Creature) string {
	if name := strings.TrimSpace(player.DisplayName); name != "" {
		return name
	}
	if name := strings.TrimSpace(creature.DisplayName); name != "" {
		return name
	}
	return string(player.ID)
}

func whereAgeYears(creature model.Creature) int {
	for _, key := range []string{"legacyHoursInterval", "LT_HOURS_interval", "lastHoursInterval"} {
		if value, ok := whereCreatureInt(creature, key); ok && value >= 0 {
			return 18 + value/86400
		}
	}
	if value, ok := whereCreatureInt(creature, "legacyAgeYears"); ok && value >= 18 {
		return value
	}
	return 18
}

func whereCreatureInt(creature model.Creature, key string) (int, bool) {
	if creature.Stats != nil {
		if value, ok := creature.Stats[key]; ok {
			return value, true
		}
	}
	if creature.Properties != nil {
		if raw, ok := creature.Properties[key]; ok {
			value, err := strconv.Atoi(strings.TrimSpace(raw))
			return value, err == nil
		}
	}
	target := normalizeFlagName(key)
	if target == "" {
		return 0, false
	}
	for statKey, value := range creature.Stats {
		if normalizeFlagName(statKey) == target {
			return value, true
		}
	}
	for propertyKey, raw := range creature.Properties {
		if normalizeFlagName(propertyKey) == target {
			value, err := strconv.Atoi(strings.TrimSpace(raw))
			return value, err == nil
		}
	}
	return 0, false
}

func RenderEffectStatus(player model.Player, creature model.Creature) string {
	var b strings.Builder
	b.WriteString("========================================================================\n")
	fmt.Fprintf(&b, "                          현재 %s님의 상태\n", statusPlayerName(player, creature))
	b.WriteString("========================================================================\n")
	for _, row := range legacyEffectStatusRows {
		for _, cell := range row {
			if statusEffectActive(player, creature, cell.aliases...) {
				b.WriteString(cell.label)
				b.WriteString(cell.activeSuffix)
			} else {
				b.WriteString(cell.inactive)
			}
		}
	}
	b.WriteString("========================================================================\n")
	return b.String()
}

type legacyEffectStatusCell struct {
	label        string
	activeSuffix string
	inactive     string
	aliases      []string
}

var legacyEffectStatusRows = [][]legacyEffectStatusCell{
	{
		{label: "중독", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"poison", "poisoned", "PPOISN", "MPOISN"}},
		{label: "질병", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"disease", "diseased", "PDISEA", "MDISEA"}},
		{label: "실명", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"blind", "blinded", "PBLIND", "MBLIND"}},
		{label: "공포", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"fear", "fearful", "PFEARS", "MFEARS"}},
		{label: "이혼", activeSuffix: "\n", inactive: "\n", aliases: []string{"charm", "charmed", "PCHARM", "MCHARM"}},
	},
	{
		{label: "은신", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"hidden", "PHIDDN", "MHIDDN"}},
		{label: "은둔", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"invisible", "invisibility", "PINVIS", "MINVIS"}},
		{label: "은둔감지", activeSuffix: "\t", inactive: "\t\t", aliases: []string{"detectInvisible", "detectInvis", "PDINVI", "MDINVI"}},
		{label: "성현진", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"bless", "blessed", "PBLESS"}},
		{label: "발광", activeSuffix: "\n", inactive: "\n", aliases: []string{"light", "PLIGHT"}},
	},
	{
		{label: "수호진", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"protection", "protect", "protected", "PPROTE"}},
		{label: "방열진", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"resistFire", "fireResistance", "PRFIRE"}},
		{label: "방한진", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"resistCold", "coldResistance", "PRCOLD"}},
		{label: "지방호", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"earthShield", "stoneShield", "PSSHLD"}},
		{label: "보마진", activeSuffix: "\n", inactive: "\n", aliases: []string{"resistMagic", "magicResistance", "PRMAGI", "MRMAGI"}},
	},
	{
		{label: "경계", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"prepared", "prepare", "PPREPA"}},
		{label: "부양술", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"levitate", "levitation", "PLEVIT"}},
		{label: "비상술", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"fly", "flying", "PFLYSP"}},
		{label: "수생술", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"breatheWater", "waterBreathing", "PBRWAT"}},
		{label: "주문감지", activeSuffix: "\n", inactive: "\n", aliases: []string{"detectMagic", "dMagic", "PDMAGI"}},
	},
	{
		{label: "활보법", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"haste", "hasted", "PHASTE"}},
		{label: "신원법", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"prayer", "prayed", "PPRAYD"}},
		{label: "참선", activeSuffix: "\t\t", inactive: "\t\t", aliases: []string{"meditate", "meditation", "PMEDIT"}},
		{label: "기공집결", activeSuffix: "\t", inactive: "\t\t", aliases: []string{"power", "PPOWER"}},
		{label: "선악감지", activeSuffix: "\n", inactive: "\n", aliases: []string{"knowAlignment", "alignmentSense", "PKNOWA"}},
	},
	{
		{label: "잠력격발", activeSuffix: "\t", inactive: "\t\t", aliases: []string{"upDamage", "upDmg", "PUPDMG"}},
		{label: "반탄강기", activeSuffix: "\t", inactive: "\t\t", aliases: []string{"reflect", "reflection", "PREFLECT"}},
		{label: "살기충전", activeSuffix: "\t", inactive: "\t\t", aliases: []string{"slayer", "slay", "PSLAYE"}},
		{label: "정령소환", activeSuffix: "\t", inactive: "\t\t", aliases: []string{"angel", "PANGEL"}},
		{label: "결혼", activeSuffix: "\n", inactive: "\n", aliases: []string{"married", "marriage", "PMARRI"}},
	},
}

func statusPlayerName(player model.Player, creature model.Creature) string {
	if name := strings.TrimSpace(player.DisplayName); name != "" {
		return name
	}
	if name := strings.TrimSpace(creature.DisplayName); name != "" {
		return name
	}
	if !player.ID.IsZero() {
		return string(player.ID)
	}
	return string(creature.ID)
}

func RenderTime(now time.Time) string {
	kst, err := time.LoadLocation("Asia/Seoul")
	if err == nil {
		now = now.In(kst)
	}
	return renderTime(now, now.Hour())
}

func RenderTimeWithLegacyTime(now time.Time, legacyTime int64) string {
	kst, err := time.LoadLocation("Asia/Seoul")
	if err == nil {
		now = now.In(kst)
	}
	hour := int(legacyTime % 24)
	if hour < 0 {
		hour += 24
	}
	return renderTime(now, hour)
}

func renderTime(now time.Time, hour int) string {
	period := "오전"
	if hour > 11 {
		period = "오후"
	}
	displayHour := hour % 12
	if displayHour == 0 {
		displayHour = 12
	}
	var b strings.Builder
	b.WriteString("현재 시간: ")
	b.WriteString(period)
	b.WriteByte(' ')
	b.WriteString(formatInt(displayHour))
	b.WriteString("시.\n")
	b.WriteString("실제 시간: ")
	b.WriteString(now.Format("Mon Jan _2 15:04:05 2006"))
	b.WriteString(" (KST).\n")
	return b.String()
}

func statusRoomName(world StatusWorld, roomID model.RoomID) string {
	if world != nil && !roomID.IsZero() {
		if room, ok := world.Room(roomID); ok {
			if name := cleanDisplayText(room.DisplayName); name != "" {
				return name
			}
		}
	}
	if roomID.IsZero() {
		return "알 수 없음"
	}
	return string(roomID)
}

type statusEffectSpec struct {
	Label   string
	Aliases []string
}

var statusEffectSpecs = []statusEffectSpec{
	{Label: "중독", Aliases: []string{"poison", "poisoned", "PPOISN", "MPOISN"}},
	{Label: "질병", Aliases: []string{"disease", "diseased", "PDISEA", "MDISEA"}},
	{Label: "실명", Aliases: []string{"blind", "blinded", "PBLIND", "MBLIND"}},
	{Label: "공포", Aliases: []string{"fear", "fearful", "PFEARS", "MFEARS"}},
	{Label: "매혹", Aliases: []string{"charm", "charmed", "PCHARM", "MCHARM"}},
	{Label: "침묵", Aliases: []string{"silence", "silenced", "PSILNC", "MSILNC"}},
	{Label: "혼란", Aliases: []string{"befuddle", "befuddled", "MBEFUD"}},
	{Label: "은신", Aliases: []string{"hidden", "PHIDDN", "MHIDDN"}},
	{Label: "은둔", Aliases: []string{"invisible", "invisibility", "PINVIS", "MINVIS"}},
	{Label: "은둔감지", Aliases: []string{"detectInvisible", "detectInvis", "PDINVI", "MDINVI"}},
	{Label: "성현진", Aliases: []string{"bless", "blessed", "PBLESS"}},
	{Label: "발광", Aliases: []string{"light", "PLIGHT"}},
	{Label: "수호진", Aliases: []string{"protection", "protect", "protected", "PPROTE"}},
	{Label: "방열진", Aliases: []string{"resistFire", "fireResistance", "PRFIRE"}},
	{Label: "방한진", Aliases: []string{"resistCold", "coldResistance", "PRCOLD"}},
	{Label: "지방호", Aliases: []string{"earthShield", "stoneShield", "PSSHLD"}},
	{Label: "보마진", Aliases: []string{"resistMagic", "magicResistance", "PRMAGI", "MRMAGI"}},
	{Label: "경계", Aliases: []string{"prepared", "prepare", "PPREPA"}},
	{Label: "부양술", Aliases: []string{"levitate", "levitation", "PLEVIT"}},
	{Label: "비상술", Aliases: []string{"fly", "flying", "PFLYSP"}},
	{Label: "수생술", Aliases: []string{"breatheWater", "waterBreathing", "PBRWAT"}},
	{Label: "주문감지", Aliases: []string{"detectMagic", "dMagic", "PDMAGI"}},
	{Label: "활보법", Aliases: []string{"haste", "hasted", "PHASTE"}},
	{Label: "신원법", Aliases: []string{"prayer", "prayed", "PPRAYD"}},
	{Label: "참선", Aliases: []string{"meditate", "meditation", "PMEDIT"}},
	{Label: "기공집결", Aliases: []string{"power", "PPOWER"}},
	{Label: "선악감지", Aliases: []string{"knowAlignment", "alignmentSense", "PKNOWA"}},
	{Label: "잠력격발", Aliases: []string{"upDamage", "upDmg", "PUPDMG"}},
	{Label: "반탄강기", Aliases: []string{"reflect", "reflection", "PREFLECT"}},
	{Label: "살기충전", Aliases: []string{"slayer", "slay", "PSLAYE"}},
	{Label: "정령소환", Aliases: []string{"angel", "PANGEL"}},
	{Label: "결혼", Aliases: []string{"married", "marriage", "PMARRI"}},
}

func activeStatusEffectLabels(player model.Player, creature model.Creature) []string {
	effects := make([]string, 0, len(statusEffectSpecs))
	for _, spec := range statusEffectSpecs {
		if statusEffectActive(player, creature, spec.Aliases...) {
			effects = append(effects, spec.Label)
		}
	}
	return effects
}

func statusEffectActive(player model.Player, creature model.Creature, aliases ...string) bool {
	if hasAnyNormalizedFlag(player.Metadata.Tags, aliases...) || creatureHasAnyFlag(creature, aliases...) {
		return true
	}
	return false
}

func formatCurrentMax(current, max int) string {
	if max == 0 {
		return formatInt(current)
	}
	return formatInt(current) + "/" + formatInt(max)
}

func formatInt(value int) string {
	return strconv.Itoa(value)
}
