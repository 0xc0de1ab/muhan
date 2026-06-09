package command

import (
	"errors"
	"fmt"
	"strings"

	"muhan/internal/krtext"
	"muhan/internal/textfmt"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

type LookWorld interface {
	Room(model.RoomID) (model.Room, bool)
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
}

type LookViewer struct {
	PlayerID           model.PlayerID
	CreatureID         model.CreatureID
	ActivePlayerIDs    map[model.PlayerID]struct{}
	ActivePlayerFilter bool
	TextOptions        textfmt.Options
}

func NewLookHandler(world LookWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}

		if target, ordinal, explicitOrdinal := lookTargetForLook(resolved); target != "" {
			text, ok, err := renderTargetLook(ctx, world, room, viewer, target, ordinal, explicitOrdinal)
			if err != nil {
				return StatusDefault, err
			}
			if !ok {
				ctx.WriteString("그런 건 보이지 않습니다.\n")
				return StatusDefault, nil
			}
			ctx.WriteString(text)
			return StatusDefault, nil
		}

		ctx.WriteString(RenderRoomLook(world, room, viewer))
		return StatusDefault, nil
	}
}

func LookViewerFromContext(ctx *Context) LookViewer {
	if ctx == nil || ctx.ActorID == "" {
		return LookViewer{}
	}
	viewer := LookViewer{
		PlayerID:    model.PlayerID(ctx.ActorID),
		CreatureID:  model.CreatureID(ctx.ActorID),
		TextOptions: textOptionsFromContext(ctx),
	}
	if activeIDs, ok := activeActorIDSet(ctx); ok {
		viewer.ActivePlayerFilter = true
		viewer.ActivePlayerIDs = make(map[model.PlayerID]struct{}, len(activeIDs))
		for id := range activeIDs {
			viewer.ActivePlayerIDs[model.PlayerID(id)] = struct{}{}
		}
	}
	return viewer
}

func CurrentRoom(world LookWorld, viewer LookViewer) (LookViewer, model.Room, error) {
	if world == nil {
		return viewer, model.Room{}, errors.New("look: world is nil")
	}

	if !viewer.PlayerID.IsZero() {
		player, ok := world.Player(viewer.PlayerID)
		if ok {
			viewer.CreatureID = player.CreatureID
			if player.RoomID.IsZero() {
				return viewer, model.Room{}, fmt.Errorf("look: player %q has no room", viewer.PlayerID)
			}
			room, ok := world.Room(player.RoomID)
			if !ok {
				return viewer, model.Room{}, fmt.Errorf("look: room %q not found", player.RoomID)
			}
			return viewer, room, nil
		}
	}

	if !viewer.CreatureID.IsZero() {
		creature, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return viewer, model.Room{}, fmt.Errorf("look: actor %q not found", viewer.CreatureID)
		}
		if !creature.PlayerID.IsZero() && viewer.PlayerID.IsZero() {
			viewer.PlayerID = creature.PlayerID
		}
		if creature.RoomID.IsZero() {
			return viewer, model.Room{}, fmt.Errorf("look: creature %q has no room", viewer.CreatureID)
		}
		room, ok := world.Room(creature.RoomID)
		if !ok {
			return viewer, model.Room{}, fmt.Errorf("look: room %q not found", creature.RoomID)
		}
		return viewer, room, nil
	}

	return viewer, model.Room{}, errors.New("look: actor id is required")
}

func RenderCurrentRoom(world LookWorld, viewer LookViewer) (string, error) {
	viewer, room, err := CurrentRoom(world, viewer)
	if err != nil {
		return "", err
	}
	return RenderRoomLook(world, room, viewer), nil
}

func RenderRoomLook(world LookWorld, room model.Room, viewer LookViewer) string {
	var b strings.Builder
	b.WriteByte('\n')
	if lookViewerBlind(world, viewer) {
		b.WriteString(colorText(viewer.TextOptions, "31", "당신은 눈이 멀어 아무것도 볼 수 없습니다.\n"))
		b.WriteString(colorText(viewer.TextOptions, "33", "너무 어두워서 볼 수가 없습니다.\n"))
		return b.String()
	}
	if lookRoomDarkBlocks(world, room, viewer) {
		b.WriteString(colorText(viewer.TextOptions, "33", "너무 어두워서 볼 수가 없습니다.\n"))
		return b.String()
	}
	if !lookViewerHasFlag(world, viewer, "PNORNM", "noRoomName", "noName") {
		b.WriteString(colorText(viewer.TextOptions, "36", roomTitle(room, viewer.TextOptions)))
		b.WriteString("\n\n")
	}

	if !lookViewerHasFlag(world, viewer, "PNOSDS", "noShortDescription", "noShort") {
		writeSection(&b, room.ShortDescription, viewer.TextOptions)
	}
	if !lookViewerHasFlag(world, viewer, "PNOLDS", "noLongDescription", "noLong") {
		writeSection(&b, room.LongDescription, viewer.TextOptions)
	}
	detectInvisible := viewerDetectsInvisible(world, viewer)
	if lookViewerHasFlag(world, viewer, "PNOEXT", "graphExits") {
		b.WriteString(renderExitGraph(room.Exits, viewer.TextOptions, detectInvisible))
	} else {
		b.WriteString(renderExits(room.Exits, viewer.TextOptions, detectInvisible))
	}

	if lookViewerHasFlag(world, viewer, "PDSCRP", "description", "showDescription") {
		for _, text := range roomPlayerDetailTexts(world, room, viewer) {
			b.WriteString(text)
		}
	} else if names := roomPlayerNames(world, room, viewer); len(names) > 0 {
		b.WriteString(colorText(viewer.TextOptions, "36", strings.Join(names, ", ")+"님이 서 있습니다."))
		b.WriteByte('\n')
	}

	for _, group := range roomCreatureLookGroups(world, room, viewer) {
		if group.Count > 1 {
			fmt.Fprintf(&b, "(x%d) ", group.Count)
		}
		b.WriteString(group.Text)
		b.WriteString(group.Aura)
		b.WriteByte('\n')
	}

	objectNames := roomObjectLookTexts(world, room, viewer)
	if len(objectNames) > 0 {
		text := strings.Join(objectNames, ", ")
		b.WriteString(text)
		b.WriteString(krtext.Particle(text, '1'))
		b.WriteString(" 놓여져 있습니다.\n")
	}
	writeRoomCreatureEnemyLines(&b, world, room, viewer)

	return b.String()
}

func renderTargetLook(ctx *Context, world LookWorld, room model.Room, viewer LookViewer, target string, ordinal int64, explicitOrdinal bool) (string, bool, error) {
	if lookViewerBlind(world, viewer) {
		opts := viewer.TextOptions
		opts.Bright = true
		return colorText(opts, "31", "당신은 눈이 멀어 있습니다!\n"), true, nil
	}
	if text, ok := renderExitTargetLook(world, room, viewer, target, ordinal); ok {
		return text, true, nil
	}
	object, ok := findLookTargetObject(world, room, viewer, target, ordinal, explicitOrdinal)
	if ok {
		return renderObjectLook(world, object, viewerDetectsMagic(world, viewer), viewerDetectsInvisible(world, viewer), viewerKnowsAlignment(world, viewer), viewer.TextOptions), true, nil
	}
	if creature, ok := findLookTargetCreature(world, room, viewer, target, ordinal); ok {
		viewerPlayer, viewerCreature, _ := lookViewerActor(world, viewer)
		text, err := renderCreatureTargetLook(ctx, world, room, viewer, viewerPlayer, viewerCreature, creature)
		return text, true, err
	}
	if player, ok := findLookTargetPlayer(world, room, viewer, target, ordinal); ok {
		viewerPlayer, viewerCreature, _ := lookViewerActor(world, viewer)
		text, err := renderPlayerTargetLook(ctx, world, room, viewer, viewerPlayer, viewerCreature, player)
		return text, true, err
	}
	return "", false, nil
}

func lookViewerBlind(world LookWorld, viewer LookViewer) bool {
	player, creature, ok := lookViewerActor(world, viewer)
	return ok && statusEffectActive(player, creature, "blind", "blinded", "PBLIND", "MBLIND")
}

func lookViewerHasFlag(world LookWorld, viewer LookViewer, names ...string) bool {
	_, creature, ok := lookViewerActor(world, viewer)
	return ok && creatureHasAnyFlag(creature, names...)
}

func lookRoomDarkBlocks(world LookWorld, room model.Room, viewer LookViewer) bool {
	if !lookRoomIsDark(world, room) {
		return false
	}
	_, viewerCreature, ok := lookViewerActor(world, viewer)
	if !ok {
		return true
	}
	if lookCreatureSeesInDark(viewerCreature) || lookCreatureHasLight(world, viewerCreature) {
		return false
	}
	return !lookRoomHasPlayerLight(world, room, viewer)
}

func lookRoomIsDark(world LookWorld, room model.Room) bool {
	if roomHasAnyFlag(room, "RDARKR", "darkAlways") {
		return true
	}
	if !roomHasAnyFlag(room, "RDARKN", "darkNight") {
		return false
	}
	hour := lookLegacyHour(world)
	return hour < 6 || hour > 20
}

func lookLegacyHour(world LookWorld) int {
	legacyTime := int64(0)
	if timeWorld, ok := world.(interface{ LegacyTime() int64 }); ok {
		legacyTime = timeWorld.LegacyTime()
	}
	hour := legacyTime % 24
	if hour < 0 {
		hour += 24
	}
	return int(hour)
}

func lookCreatureSeesInDark(creature model.Creature) bool {
	switch creatureStat(creature, "race") {
	case legacyRaceElf, legacyRaceDwarf:
		return true
	}
	return creatureClass(creature) >= legacyClassCaretaker
}

func lookRoomHasPlayerLight(world LookWorld, room model.Room, viewer LookViewer) bool {
	for _, playerID := range room.PlayerIDs {
		if playerID.IsZero() || !viewerAllowsPlayer(viewer, playerID) {
			continue
		}
		player, ok := world.Player(playerID)
		if !ok || player.RoomID != room.ID || player.CreatureID.IsZero() {
			continue
		}
		creature, ok := world.Creature(player.CreatureID)
		if ok && creature.RoomID == room.ID && lookCreatureHasLight(world, creature) {
			return true
		}
	}
	return false
}

func lookCreatureHasLight(world LookWorld, creature model.Creature) bool {
	if creatureHasAnyFlag(creature, "PLIGHT", "light") {
		return true
	}
	for _, objectID := range lookEquipmentObjectIDs(creature) {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectHasAnyFlagOrProperty(world, object, "OLIGHT", "light") {
			continue
		}
		if objectLegacyTypeOrKind(world, object) != legacyObjectLightSource {
			return true
		}
		charges, ok := objectFirstIntProperty(world, object, "shotsCurrent", "shotscur", "shotsCur", "charges")
		if !ok || charges > 0 {
			return true
		}
	}
	return false
}

func lookViewerActor(world LookWorld, viewer LookViewer) (model.Player, model.Creature, bool) {
	if world == nil {
		return model.Player{}, model.Creature{}, false
	}
	if !viewer.PlayerID.IsZero() {
		player, ok := world.Player(viewer.PlayerID)
		if ok && !player.CreatureID.IsZero() {
			if creature, ok := world.Creature(player.CreatureID); ok {
				return player, creature, true
			}
		}
	}
	if !viewer.CreatureID.IsZero() {
		creature, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return model.Player{}, model.Creature{}, false
		}
		if !creature.PlayerID.IsZero() {
			if player, ok := world.Player(creature.PlayerID); ok {
				return player, creature, true
			}
		}
		return model.Player{}, creature, true
	}
	return model.Player{}, model.Creature{}, false
}

func renderExitTargetLook(
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	target string,
	ordinal int64,
) (string, bool) {
	exit, ok := findLookTargetExitForViewer(world, viewer, room.Exits, target, ordinal)
	if !ok {
		return "", false
	}
	if exitHasAnyFlag(exit, "closed", "xclosd", "xclosed") {
		return "그 출구는 닫혀 있습니다.", true
	}
	destination, ok := world.Room(exit.ToRoomID)
	if !ok || destination.ID == room.ID {
		return "지도가 없습니다.", true
	}
	if roomHasAnyFlag(destination, "onlyMarried", "marriedOnly", "ronmar", "onlyFamily", "familyOnly", "ronfml") {
		return "그 방은 볼 수가 없습니다.", true
	}
	return RenderRoomLook(world, destination, viewer), true
}

func findLookTargetExitForViewer(
	world LookWorld,
	viewer LookViewer,
	exits []model.Exit,
	prefix string,
	ordinal int64,
) (model.Exit, bool) {
	return findLookTargetExit(exits, prefix, ordinal, viewerDetectsInvisible(world, viewer))
}

func findLookTargetExit(
	exits []model.Exit,
	prefix string,
	ordinal int64,
	detectInvisible bool,
) (model.Exit, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.Exit{}, false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, exit := range exits {
		name := strings.TrimSpace(exit.Name)
		if name == "" || !strings.HasPrefix(name, prefix) || !exitTargetVisible(exit, detectInvisible) {
			continue
		}
		seen++
		if seen == ordinal {
			return exit, true
		}
	}
	return model.Exit{}, false
}

func RenderObjectLook(world LookWorld, object model.ObjectInstance) string {
	return renderObjectLook(world, object, false, false, false, textfmt.Options{})
}

func renderObjectLook(world LookWorld, object model.ObjectInstance, detectMagic bool, detectInvisible bool, knowAlignment bool, opts textfmt.Options) string {
	var b strings.Builder

	text := lookObjectDescription(world, object)
	if text == "" {
		text = "특별한 점이 없습니다."
	}
	writeSection(&b, text, textfmt.Options{})
	writeObjectAlignmentDetails(&b, world, object, knowAlignment, opts)
	writeObjectLegacyTypeDetails(&b, world, object)
	writeObjectLegacyDurabilityDetails(&b, world, object)

	if objectIsContainer(world, object) {
		names := containerObjectNames(world, object, detectMagic, detectInvisible)
		if len(names) > 0 {
			b.WriteString("내용물: ")
			b.WriteString(strings.Join(names, ", "))
			b.WriteString(".\n")
		}
	}

	return b.String()
}

func renderCreatureTargetLook(
	ctx *Context,
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	viewerPlayer model.Player,
	viewerCreature model.Creature,
	creature model.Creature,
) (string, error) {
	var b strings.Builder
	actorName := lookActorName(viewerPlayer, viewerCreature)
	if creature.ID == viewerCreature.ID {
		b.WriteString("당신은 거울을 들고 자신을 봅니다.\n")
		if actorName != "" {
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 거울을 들고 자신을 바라 봅니다."); err != nil {
				return "", err
			}
		}
		if attackCreatureIsPlayer(creature) {
			b.WriteString(renderSelfPlayerCreatureLook(world, viewerPlayer, creature, viewerKnowsAlignment(world, viewer), viewer.TextOptions))
			return b.String(), nil
		}
	} else {
		targetName := attackCreatureName(creature)
		fmt.Fprintf(&b, "당신은 %s%s 봅니다.\n", targetName, krtext.Particle(targetName, '3'))
		if actorName != "" && targetName != "" {
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+krtext.Particle(targetName, '3')+" 봅니다."); err != nil {
				return "", err
			}
		}
	}
	b.WriteString(renderCreatureLook(world, viewerPlayer, viewerCreature, creature, viewerKnowsAlignment(world, viewer), viewer.TextOptions))
	return b.String(), nil
}

func renderSelfPlayerCreatureLook(
	world InventoryWorld,
	viewerPlayer model.Player,
	creature model.Creature,
	knowAlignment bool,
	opts textfmt.Options,
) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s는 %s있습니다.\n", lookCreaturePronoun(creature), playerLookDescriptionText(creature, opts))
	writeCreatureAlignmentDetails(&b, creature, knowAlignment, opts)
	writeCreatureWoundDetails(&b, creature)
	writeCreatureEnemyDetails(&b, world, viewerPlayer, creature, creature)
	writeCreatureConsiderDetails(&b, creature, creature)
	writeLegacyEquipmentList(&b, world, creature, opts)
	return b.String()
}

func renderPlayerTargetLook(
	ctx *Context,
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	viewerPlayer model.Player,
	viewerCreature model.Creature,
	player model.Player,
) (string, error) {
	var b strings.Builder
	actorName := lookActorName(viewerPlayer, viewerCreature)
	targetName := lookPlayerTargetName(player)
	fmt.Fprintf(&b, "당신은 %s%s 봅니다.\n", targetName, krtext.Particle(targetName, '3'))
	if actorName != "" && targetName != "" {
		if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+krtext.Particle(targetName, '3')+" 봅니다."); err != nil {
			return "", err
		}
	}
	b.WriteString(renderPlayerLook(world, player, viewerKnowsAlignment(world, viewer), viewer.TextOptions))
	return b.String(), nil
}

func lookActorName(player model.Player, creature model.Creature) string {
	if name := cleanDisplayText(creature.DisplayName); name != "" {
		return name
	}
	if name := cleanDisplayText(player.DisplayName); name != "" {
		return name
	}
	if !creature.ID.IsZero() {
		return string(creature.ID)
	}
	if !player.ID.IsZero() {
		return string(player.ID)
	}
	return ""
}

func lookPlayerTargetName(player model.Player) string {
	if name := cleanDisplayText(player.DisplayName); name != "" {
		return name
	}
	if name := cleanDisplayText(player.AccountName); name != "" {
		return name
	}
	return string(player.ID)
}

func RenderCreatureLook(creature model.Creature) string {
	return renderCreatureLook(nil, model.Player{}, model.Creature{}, creature, false, textfmt.Options{})
}

func renderCreatureLook(
	world InventoryWorld,
	viewerPlayer model.Player,
	viewerCreature model.Creature,
	creature model.Creature,
	knowAlignment bool,
	opts textfmt.Options,
) string {
	text := creatureLookText(creature)
	if text == "" {
		text = "특별한 것은 보이지 않습니다."
	}
	var b strings.Builder
	b.WriteString(text)
	b.WriteByte('\n')
	writeCreatureAlignmentDetails(&b, creature, knowAlignment, opts)
	writeCreatureWoundDetails(&b, creature)
	writeCreatureEnemyDetails(&b, world, viewerPlayer, viewerCreature, creature)
	writeCreatureConsiderDetails(&b, viewerCreature, creature)
	writeLegacyEquipmentList(&b, world, creature, opts)
	return b.String()
}

func RenderPlayerLook(world LookWorld, player model.Player) string {
	return renderPlayerLook(world, player, false, textfmt.Options{})
}

func renderPlayerLook(world LookWorld, player model.Player, knowAlignment bool, opts textfmt.Options) string {
	if !player.CreatureID.IsZero() {
		if creature, ok := world.Creature(player.CreatureID); ok {
			var b strings.Builder
			fmt.Fprintf(&b, "%s는 %s있습니다.\n", lookCreaturePronoun(creature), playerLookDescriptionText(creature, opts))
			writePlayerAlignmentDetails(&b, creature, knowAlignment, opts)
			writePlayerWoundDetails(&b, creature)
			writeLegacyEquipmentList(&b, world, creature, opts)
			return b.String()
		}
	}
	name := cleanDisplayText(player.DisplayName)
	if name == "" {
		name = string(player.ID)
	}
	return name + "님이 있습니다.\n"
}

func writeObjectAlignmentDetails(b *strings.Builder, world LookWorld, object model.ObjectInstance, knowAlignment bool, opts textfmt.Options) {
	if !knowAlignment {
		return
	}
	if objectHasAnyFlagOrProperty(world, object, "goodOnly", "good", "ogoodo", "OGOODO") {
		writeLookAura(b, "", "푸른 광채", "34", opts)
	}
	if objectHasAnyFlagOrProperty(world, object, "evilOnly", "evil", "oevilo", "OEVILO") {
		writeLookAura(b, "", "붉은 광채", "31", opts)
	}
}

func writeCreatureAlignmentDetails(b *strings.Builder, creature model.Creature, knowAlignment bool, opts textfmt.Options) {
	if !knowAlignment {
		return
	}
	alignment := creatureStat(creature, "alignment")
	if alignment == 0 {
		return
	}
	aura := "푸른 광채"
	color := "34"
	if alignment < 0 {
		aura = "붉은 광채"
		color = "31"
	}
	writeLookAura(b, lookCreaturePronoun(creature)+"에게서 ", aura, color, opts)
}

func writePlayerAlignmentDetails(b *strings.Builder, creature model.Creature, knowAlignment bool, opts textfmt.Options) {
	if !knowAlignment {
		return
	}
	alignment := creatureStat(creature, "alignment")
	if alignment < -100 || alignment >= 101 {
		return
	}
	writeLookAura(b, lookCreaturePronoun(creature)+"에게서 ", "푸른 광채", "34", opts)
}

func writeLookAura(b *strings.Builder, prefix string, aura string, color string, opts textfmt.Options) {
	b.WriteString(prefix)
	b.WriteString(colorText(opts, color, aura))
	b.WriteString("가 뻗어 나오고 있습니다.\n")
}

func writeCreatureWoundDetails(b *strings.Builder, creature model.Creature) {
	hpCurrent, hpMax, ok := lookCreatureHP(creature)
	if !ok {
		return
	}
	pronoun := lookCreaturePronoun(creature)
	switch {
	case hpCurrent < (hpMax*9)/10 && hpCurrent > (hpMax*8)/10:
		fmt.Fprintf(b, "%s는 가벼운 상처를 입었습니다.\n", pronoun)
	case hpCurrent < (hpMax*8)/10 && hpCurrent > (hpMax*6)/10:
		fmt.Fprintf(b, "%s는 여러군데 상처를 입었습니다.\n", pronoun)
	case hpCurrent < (hpMax*6)/10 && hpCurrent > (hpMax*4)/10:
		fmt.Fprintf(b, "%s는 많은 상처를 입었습니다.\n", pronoun)
	case hpCurrent < (hpMax*4)/10 && hpCurrent > (hpMax*2)/10:
		fmt.Fprintf(b, "%s는 심각한 상처를 입었습니다.\n", pronoun)
	case hpCurrent < (hpMax * 2 / 10):
		fmt.Fprintf(b, "%s는 죽기 직전입니다.\n", pronoun)
	}
}

func writePlayerWoundDetails(b *strings.Builder, creature model.Creature) {
	hpCurrent, hpMax, ok := lookCreatureHP(creature)
	if !ok {
		return
	}
	if hpCurrent < (hpMax * 3 / 10) {
		fmt.Fprintf(b, "%s는 가벼운 상처를 입었습니다.\n", lookCreaturePronoun(creature))
	}
}

func writeCreatureEnemyDetails(
	b *strings.Builder,
	world InventoryWorld,
	viewerPlayer model.Player,
	viewerCreature model.Creature,
	creature model.Creature,
) {
	if b == nil || world == nil {
		return
	}
	enemyWorld, ok := world.(interface {
		CreatureEnemies(model.CreatureID) ([]string, error)
	})
	if !ok {
		return
	}
	enemies, err := enemyWorld.CreatureEnemies(creature.ID)
	if err != nil || len(enemies) == 0 {
		return
	}

	viewerName := lookViewerEnemyName(viewerPlayer, viewerCreature)
	pronoun := lookCreaturePronoun(creature)
	if viewerName != "" && lookEnemyListContains(enemies, viewerName) {
		fmt.Fprintf(b, "%s는 당신에게 매우 화가 난것 같습니다.\n", pronoun)
	}
	firstEnemy := strings.TrimSpace(enemies[0])
	if firstEnemy == "" {
		return
	}
	if firstEnemy == viewerName {
		fmt.Fprintf(b, "%s는 당신과 싸우고 있습니다.\n", pronoun)
		return
	}
	fmt.Fprintf(b, "%s는 %s%s 싸우고 있습니다.\n", pronoun, firstEnemy, krtext.Particle(firstEnemy, '2'))
}

func writeRoomCreatureEnemyLines(b *strings.Builder, world LookWorld, room model.Room, viewer LookViewer) {
	if b == nil || world == nil {
		return
	}
	enemyWorld, ok := world.(interface {
		CreatureEnemies(model.CreatureID) ([]string, error)
	})
	if !ok {
		return
	}
	viewerPlayer, viewerCreature, viewerOK := lookViewerActor(world, viewer)
	viewerName := ""
	if viewerOK {
		viewerName = lookViewerEnemyName(viewerPlayer, viewerCreature)
	}
	for _, creatureID := range room.CreatureIDs {
		if creatureID.IsZero() {
			continue
		}
		creature, ok := world.Creature(creatureID)
		if !ok || creature.RoomID != room.ID || !creature.PlayerID.IsZero() {
			continue
		}
		enemies, err := enemyWorld.CreatureEnemies(creature.ID)
		if err != nil || len(enemies) == 0 {
			continue
		}
		firstEnemy := strings.TrimSpace(enemies[0])
		if firstEnemy == "" {
			continue
		}
		name := attackCreatureName(creature)
		if firstEnemy == viewerName {
			fmt.Fprintf(b, "%s%s 당신과 싸우고 있습니다.\n", name, krtext.Particle(name, '1'))
			continue
		}
		if roomPlayerEnemyName(world, room, viewer, firstEnemy) != "" {
			fmt.Fprintf(b, "%s%s %s%s 싸우고 있습니다.\n", name, krtext.Particle(name, '1'), firstEnemy, krtext.Particle(firstEnemy, '2'))
		}
	}
}

func roomPlayerEnemyName(world LookWorld, room model.Room, viewer LookViewer, enemyName string) string {
	enemyName = strings.TrimSpace(enemyName)
	if enemyName == "" {
		return ""
	}
	for _, playerID := range room.PlayerIDs {
		if playerID.IsZero() || !viewerAllowsPlayer(viewer, playerID) {
			continue
		}
		player, ok := world.Player(playerID)
		if !ok || player.RoomID != room.ID {
			continue
		}
		name := roomPlayerDisplayName(world, player)
		if name == enemyName {
			return name
		}
	}
	return ""
}

func lookViewerEnemyName(player model.Player, creature model.Creature) string {
	if name := cleanDisplayText(creature.DisplayName); name != "" {
		return name
	}
	if name := cleanDisplayText(player.DisplayName); name != "" {
		return name
	}
	if !creature.ID.IsZero() {
		return string(creature.ID)
	}
	if !player.ID.IsZero() {
		return string(player.ID)
	}
	return ""
}

func lookEnemyListContains(enemies []string, name string) bool {
	for _, enemy := range enemies {
		if strings.TrimSpace(enemy) == name {
			return true
		}
	}
	return false
}

func writeCreatureConsiderDetails(b *strings.Builder, viewerCreature model.Creature, creature model.Creature) {
	if b == nil || viewerCreature.ID.IsZero() {
		return
	}
	pronoun := lookCreaturePronoun(creature)
	diff := (attackCreatureLevel(viewerCreature) / 4) - (attackCreatureLevel(creature) / 4)
	if diff < -4 {
		diff = -4
	}
	if diff > 4 {
		diff = 4
	}

	switch diff {
	case 0:
		fmt.Fprintf(b, "%s는 당신과 꼭 맞는 상대입니다!\n", pronoun)
	case 1:
		fmt.Fprintf(b, "%s는 별 무리없이 이길 수 있습니다.\n", pronoun)
	case -1:
		fmt.Fprintf(b, "%s는 운이 좋으면 이길 수 있습니다..\n", pronoun)
	case 2:
		fmt.Fprintf(b, "%s는 별로 힘 안들이고 이길수 있습니다.\n", pronoun)
	case -2:
		fmt.Fprintf(b, "%s는 상대하기 힘들겠는데요?\n", pronoun)
	case 3:
		fmt.Fprintf(b, "%s는 손쉽게 상대할수 있습니다.\n", pronoun)
	case -3:
		fmt.Fprintf(b, "당신은 %s에게 쨉도 안됩니다.\n", pronoun)
	case 4:
		fmt.Fprintf(b, "%s는 한방에 보낼수 있습니다.\n", pronoun)
	case -4:
		fmt.Fprintf(b, "%s는 보자마자 도망가는것이 좋을겁니다.\n", pronoun)
	}
}

func lookCreatureHP(creature model.Creature) (int, int, bool) {
	hpCurrent := creatureStat(creature, "hpCurrent")
	hpMax := creatureStat(creature, "hpMax")
	return hpCurrent, hpMax, hpMax > 0
}

func lookCreaturePronoun(creature model.Creature) string {
	if creatureHasAnyFlag(creature, "MMALES", "PMALES", "male") {
		return "그"
	}
	return "그녀"
}

func lookTarget(resolved ResolvedCommand) (string, int64) {
	target := joinArgs(resolved.Args)
	if target == "" {
		return "", 1
	}
	ordinal := int64(1)
	if n := len(resolved.Args); n > 0 && len(resolved.Values) >= n && resolved.Values[n-1] > 0 {
		ordinal = resolved.Values[n-1]
	}
	return target, ordinal
}

func lookTargetForLook(resolved ResolvedCommand) (string, int64, bool) {
	target, ordinal := lookTarget(resolved)
	return target, ordinal, lookInputHasExplicitOrdinal(resolved.Input)
}

func lookInputHasExplicitOrdinal(input string) bool {
	for _, token := range strings.FieldsFunc(input, func(r rune) bool {
		return r == ' ' || r == '#'
	}) {
		if isPositiveIntegerToken(token) {
			return true
		}
	}
	return false
}

func isPositiveIntegerToken(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if r < '0' || r > '9' {
			return false
		}
	}
	return token != "0"
}

func findLookTargetObject(
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	prefix string,
	ordinal int64,
	explicitOrdinal bool,
) (model.ObjectInstance, bool) {
	detectInvisible := viewerDetectsInvisible(world, viewer)
	if !viewer.CreatureID.IsZero() {
		if creature, ok := world.Creature(viewer.CreatureID); ok {
			if !explicitOrdinal {
				if object, ok := findLookObjectInList(world, lookEquipmentObjectIDs(creature), prefix, ordinal, func(object model.ObjectInstance) bool {
					return objectLocatedInCreatureEquipment(object, creature.ID)
				}); ok {
					return object, true
				}
			}
			if object, ok := findLookObjectInList(world, creature.Inventory.ObjectIDs, prefix, ordinal, func(object model.ObjectInstance) bool {
				return objectLocatedInCreatureInventory(object, creature.ID) &&
					objectVisibleForFindObj(world, object, detectInvisible)
			}); ok {
				return object, true
			}
			if explicitOrdinal {
				if object, ok := findLookObjectInList(world, lookEquipmentObjectIDs(creature), prefix, ordinal, func(object model.ObjectInstance) bool {
					return objectLocatedInCreatureEquipment(object, creature.ID)
				}); ok {
					return object, true
				}
			}
		}
	}

	return findLookObjectInList(world, room.Objects.ObjectIDs, prefix, ordinal, func(object model.ObjectInstance) bool {
		return objectLocatedInRoom(object, room.ID) && objectVisibleForFindObj(world, object, detectInvisible)
	})
}

func lookEquipmentObjectIDs(creature model.Creature) []model.ObjectInstanceID {
	slots := orderedEquipmentSlots(creature.Equipment, legacyReadySlotOrder)
	ids := make([]model.ObjectInstanceID, 0, len(slots))
	for _, slot := range slots {
		if id := creature.Equipment[slot]; !id.IsZero() {
			ids = append(ids, id)
		}
	}
	return ids
}

func findLookTargetCreature(
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	prefix string,
	ordinal int64,
) (model.Creature, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.Creature{}, false
	}
	if prefix == "나" && !viewer.CreatureID.IsZero() {
		if creature, ok := world.Creature(viewer.CreatureID); ok {
			return creature, true
		}
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, id := range room.CreatureIDs {
		if id.IsZero() || id == viewer.CreatureID {
			continue
		}
		creature, ok := world.Creature(id)
		if !ok || creature.RoomID != room.ID {
			continue
		}
		if !creature.PlayerID.IsZero() {
			continue
		}
		if !lookCreatureMatches(creature, prefix) {
			continue
		}
		seen++
		if seen == ordinal {
			return creature, true
		}
	}
	return model.Creature{}, false
}

func findLookTargetPlayer(
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	prefix string,
	ordinal int64,
) (model.Player, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.Player{}, false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, id := range room.PlayerIDs {
		if id.IsZero() || id == viewer.PlayerID {
			continue
		}
		if !viewerAllowsPlayer(viewer, id) {
			continue
		}
		player, ok := world.Player(id)
		if !ok || player.RoomID != room.ID {
			continue
		}
		if !lookPlayerMatches(world, player, prefix) {
			continue
		}
		seen++
		if seen == ordinal {
			return player, true
		}
	}
	return model.Player{}, false
}

func lookCreatureMatches(creature model.Creature, prefix string) bool {
	if lookFindCrtTermMatches(creature.DisplayName, prefix) {
		return true
	}
	for _, key := range []string{"name", "key[0]", "key[1]", "key[2]"} {
		if lookFindCrtTermMatches(creature.Properties[key], prefix) {
			return true
		}
	}
	return false
}

func lookPlayerMatches(world LookWorld, player model.Player, prefix string) bool {
	prefix = legacyUpperFirstASCII(prefix)
	if lookFindCrtTermMatches(player.DisplayName, prefix) {
		return true
	}
	if world == nil || player.CreatureID.IsZero() {
		return false
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return false
	}
	if lookFindCrtTermMatches(creature.DisplayName, prefix) {
		return true
	}
	for _, key := range []string{"name", "key[0]", "key[1]", "key[2]"} {
		if lookFindCrtTermMatches(creature.Properties[key], prefix) {
			return true
		}
	}
	return false
}

func lookFindCrtTermMatches(term, prefix string) bool {
	term = cleanDisplayText(term)
	prefix = strings.TrimSpace(prefix)
	return len([]byte(prefix)) >= 2 && term != "" && strings.HasPrefix(term, prefix)
}

func legacyUpperFirstASCII(text string) string {
	if text == "" {
		return ""
	}
	if text[0] >= 'a' && text[0] <= 'z' {
		return string(text[0]-('a'-'A')) + text[1:]
	}
	return text
}

func findLookObjectInList(
	world objectNameWorld,
	ids []model.ObjectInstanceID,
	prefix string,
	ordinal int64,
	visible func(model.ObjectInstance) bool,
) (model.ObjectInstance, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.ObjectInstance{}, false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		object, ok := world.Object(id)
		if !ok || !visible(object) || !legacyObjectPrefixMatches(world, object, prefix) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, true
		}
	}
	return model.ObjectInstance{}, false
}

func lookObjectDescription(world objectNameWorld, object model.ObjectInstance) string {
	if text := cleanDescriptionText(object.Properties["description"]); text != "" {
		return text
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if text := cleanDescriptionText(proto.Description); text != "" {
				return text
			}
			if text := cleanDescriptionText(proto.Properties["description"]); text != "" {
				return text
			}
		}
	}
	return ""
}

func writeObjectLegacyTypeDetails(b *strings.Builder, world LookWorld, object model.ObjectInstance) {
	legacyType := objectLegacyTypeOrKind(world, object)
	var description string
	switch legacyType {
	case legacyObjectSharp:
		description = "매우 날카로운 '도'입니다."
	case legacyObjectThrust:
		description = "매우 공격적인 '검'입니다."
	case legacyObjectPole:
		description = "날이 바짝 선 '창'입니다."
	case legacyObjectBlunt:
		description = "매우 위력적인 '봉'입니다."
	case legacyObjectMissile:
		description = "매우 강력하게 보이는 '궁'입니다."
	}
	if description == "" {
		return
	}
	name := objectDisplayName(world, object)
	if name == "" {
		name = "그것"
	}
	b.WriteString(name)
	b.WriteString(krtext.Particle(name, '0'))
	b.WriteByte(' ')
	b.WriteString(description)
	b.WriteByte('\n')
}

func writeObjectLegacyDurabilityDetails(b *strings.Builder, world LookWorld, object model.ObjectInstance) {
	legacyType := objectLegacyTypeOrKind(world, object)
	if !lookObjectShowsDurability(legacyType) {
		return
	}
	shotsCurrent, ok := objectFirstIntProperty(world, object, "shotsCurrent", "shotscur", "shotsCur", "charges")
	if !ok {
		return
	}
	if shotsCurrent < 1 {
		b.WriteString("그것은 부서져 버렸거나 다 써버렸습니다.\n")
		return
	}
	shotsMax, ok := objectFirstIntProperty(world, object, "shotsMax", "shotsmax", "shotsmaximum")
	if !ok {
		return
	}
	if shotsCurrent <= shotsMax/10 {
		b.WriteString("그것은 곧 부서질것 같습니다.\n")
	}
}

func lookObjectShowsDurability(legacyType int) bool {
	return (legacyType >= legacyObjectSharp && legacyType <= legacyObjectMissile) ||
		legacyType == legacyObjectArmor ||
		legacyType == legacyObjectLightSource ||
		legacyType == legacyObjectWand ||
		legacyType == legacyObjectKey
}

func containerObjectNames(world objectNameWorld, container model.ObjectInstance, detectMagic bool, detectInvisible bool) []string {
	return objectListLookTexts(world, container.Contents.ObjectIDs, func(object model.ObjectInstance) bool {
		return objectLocatedInContainer(object, container.ID)
	}, detectMagic, detectInvisible)
}

func roomTitle(room model.Room, opts textfmt.Options) string {
	if title := renderDisplayText(room.DisplayName, opts); title != "" {
		return title
	}
	return "무명"
}

func writeSection(b *strings.Builder, text string, opts textfmt.Options) {
	text = renderBlockText(text, opts)
	if text == "" {
		return
	}
	b.WriteString(text)
	b.WriteByte('\n')
}

func renderExits(exits []model.Exit, opts textfmt.Options, detectInvisible bool) string {
	names := make([]string, 0, len(exits))
	for _, exit := range exits {
		name := renderDisplayText(exit.Name, opts)
		if name != "" && exitVisibleInRoomLook(exit, detectInvisible) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return colorText(opts, "32", "[ 출구 : 없음 ]") + "\n"
	}
	return colorText(opts, "32", "[ 출구 : "+strings.Join(names, ", ")+" ]") + "\n"
}

func renderExitGraph(exits []model.Exit, opts textfmt.Options, detectInvisible bool) string {
	top := []rune("       ")
	middle := []rune("   O   ")
	bottom := []rune("       ")
	var other []string
	visible := 0

	for _, exit := range exits {
		name := renderDisplayText(exit.Name, opts)
		if name == "" || !exitVisibleInRoomLook(exit, detectInvisible) {
			continue
		}
		visible++
		switch name {
		case "동":
			middle[5], middle[6] = '-', '-'
		case "서":
			middle[0], middle[1] = '-', '-'
		case "남":
			bottom[3] = '|'
		case "북":
			top[3] = '|'
		case "상":
			top[5] = '상'
		case "하":
			bottom[5] = '하'
		default:
			other = append(other, name)
		}
	}

	if visible == 0 {
		return colorText(opts, "32", "[ 출구 : 없음 ]") + "\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[ %s ]\n", string(top))
	fmt.Fprintf(&b, "[ %s ]", string(middle))
	if len(other) > 0 {
		fmt.Fprintf(&b, " [ 출구 : %s ]\n", strings.Join(other, ", "))
	} else {
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "[ %s ]\n", string(bottom))
	return colorText(opts, "32", b.String())
}

func exitVisibleInRoomLook(exit model.Exit, detectInvisible bool) bool {
	return !exitHasAnyFlag(exit, "secret", "xsecrt", "xsecret") && exitTargetVisible(exit, detectInvisible)
}

func exitTargetVisible(exit model.Exit, detectInvisible bool) bool {
	if exitHasAnyFlag(exit, "noSee", "xnosee") {
		return false
	}
	if exitHasAnyFlag(exit, "invisible", "xinvis") && !detectInvisible {
		return false
	}
	return true
}

func exitHasAnyFlag(exit model.Exit, names ...string) bool {
	return hasAnyNormalizedFlag(exit.Flags, names...)
}

func roomHasAnyFlag(room model.Room, names ...string) bool {
	if hasAnyNormalizedFlag(room.Metadata.Tags, names...) {
		return true
	}
	if len(room.Properties) == 0 {
		return false
	}

	targets := normalizedFlagSet(names...)
	for key, value := range room.Properties {
		normalizedKey := normalizeFlagName(key)
		if _, ok := targets[normalizedKey]; ok && propertyFlagEnabled(value) {
			return true
		}
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || r == ' '
		}) {
			if _, ok := targets[normalizeFlagName(token)]; ok {
				return true
			}
		}
	}
	return false
}

func hasAnyNormalizedFlag(flags []string, names ...string) bool {
	targets := normalizedFlagSet(names...)
	for _, flag := range flags {
		if _, ok := targets[normalizeFlagName(flag)]; ok {
			return true
		}
	}
	return false
}

func normalizedFlagSet(names ...string) map[string]struct{} {
	targets := make(map[string]struct{})
	for _, name := range state.ExpandFlagNames(names...) {
		targets[name] = struct{}{}
	}
	return targets
}

func normalizeFlagName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, " ", "")
	return name
}

func propertyFlagEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func creatureLookText(creature model.Creature) string {
	if text := creatureDescriptionText(creature); text != "" {
		return text
	}
	return cleanDisplayText(creature.DisplayName)
}

func creatureDescriptionText(creature model.Creature) string {
	if creature.Properties != nil {
		if text, ok := creature.Properties[legacyDescriptionProperty]; ok {
			return cleanDescriptionText(text)
		}
	}
	return cleanDescriptionText(creature.Description)
}

func playerLookDescriptionText(creature model.Creature, opts textfmt.Options) string {
	if creature.Properties != nil {
		if text, ok := creature.Properties[legacyDescriptionProperty]; ok {
			return textfmt.RenderLegacyColors(text, opts)
		}
	}
	return textfmt.RenderLegacyColors(creature.Description, opts)
}

func roomPlayerNames(world LookWorld, room model.Room, viewer LookViewer) []string {
	names := make([]string, 0, len(room.PlayerIDs))
	detectInvisible := viewerDetectsInvisible(world, viewer)
	for _, playerID := range room.PlayerIDs {
		if playerID.IsZero() || playerID == viewer.PlayerID {
			continue
		}
		if !viewerAllowsPlayer(viewer, playerID) {
			continue
		}
		player, ok := world.Player(playerID)
		if !ok || player.RoomID != room.ID || !playerVisibleInRoomLook(world, player, viewer, detectInvisible) {
			continue
		}
		name := roomPlayerDisplayName(world, player)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func roomPlayerDetailTexts(world LookWorld, room model.Room, viewer LookViewer) []string {
	texts := make([]string, 0, len(room.PlayerIDs))
	detectInvisible := viewerDetectsInvisible(world, viewer)
	for _, playerID := range room.PlayerIDs {
		if playerID.IsZero() || playerID == viewer.PlayerID {
			continue
		}
		if !viewerAllowsPlayer(viewer, playerID) {
			continue
		}
		player, ok := world.Player(playerID)
		if !ok || player.RoomID != room.ID || !playerVisibleInRoomLook(world, player, viewer, detectInvisible) {
			continue
		}
		name := roomPlayerDisplayName(world, player)
		if name == "" {
			continue
		}
		var description string
		if !player.CreatureID.IsZero() {
			if creature, ok := world.Creature(player.CreatureID); ok {
				description = playerLookDescriptionText(creature, viewer.TextOptions)
			}
		}
		text := colorText(viewer.TextOptions, "36", name+"님이 "+description+"있습니다.") + "\n"
		if !player.CreatureID.IsZero() {
			if creature, ok := world.Creature(player.CreatureID); ok && creatureHasAnyFlag(creature, "PANGEL", "angel") {
				text += colorText(viewer.TextOptions, "35", name+"의 정령이 주위를 맴돕니다.") + "\n"
			}
		}
		texts = append(texts, text)
	}
	return texts
}

func roomPlayerDisplayName(world LookWorld, player model.Player) string {
	if !player.CreatureID.IsZero() {
		if creature, ok := world.Creature(player.CreatureID); ok {
			if name := cleanDisplayText(creature.DisplayName); name != "" {
				return name
			}
		}
	}
	if name := cleanDisplayText(player.DisplayName); name != "" {
		return name
	}
	if name := cleanDisplayText(player.AccountName); name != "" {
		return name
	}
	return string(player.ID)
}

func viewerAllowsPlayer(viewer LookViewer, playerID model.PlayerID) bool {
	if !viewer.ActivePlayerFilter {
		return true
	}
	_, ok := viewer.ActivePlayerIDs[playerID]
	return ok
}

type creatureLookGroup struct {
	Text  string
	Aura  string
	Count int
}

func roomCreatureLookGroups(world LookWorld, room model.Room, viewer LookViewer) []creatureLookGroup {
	groups := make([]creatureLookGroup, 0, len(room.CreatureIDs))
	detectInvisible := viewerDetectsInvisible(world, viewer)
	knowAlignment := viewerKnowsAlignment(world, viewer)
	for _, creatureID := range room.CreatureIDs {
		if creatureID.IsZero() || creatureID == viewer.CreatureID {
			continue
		}
		creature, ok := world.Creature(creatureID)
		if !ok || creature.RoomID != room.ID || creatureHPDead(creature) ||
			!creatureVisibleInRoomLook(creature, viewer, detectInvisible) {
			continue
		}
		if !creature.PlayerID.IsZero() {
			continue
		}
		text := creatureLookText(creature)
		if text == "" {
			continue
		}
		aura := roomCreatureAuraText(creature, knowAlignment)
		last := len(groups) - 1
		if last >= 0 && groups[last].Text == text {
			groups[last].Count++
			groups[last].Aura = aura
			continue
		}
		groups = append(groups, creatureLookGroup{Text: text, Aura: aura, Count: 1})
	}
	return groups
}

func roomCreatureAuraText(creature model.Creature, knowAlignment bool) string {
	if !knowAlignment {
		return ""
	}
	if creatureStat(creature, "alignment") < 0 {
		return " (붉은 광채)"
	}
	return " (푸른 광채)"
}

type objectLookGroup struct {
	Text       string
	BaseName   string
	Adjustment int
	Count      int
}

func roomObjectLookTexts(world LookWorld, room model.Room, viewer LookViewer) []string {
	return objectListLookTexts(world, room.Objects.ObjectIDs, func(object model.ObjectInstance) bool {
		return objectLocatedInRoom(object, room.ID)
	}, viewerDetectsMagic(world, viewer), viewerDetectsInvisible(world, viewer))
}

func objectListLookTexts(
	world objectNameWorld,
	ids []model.ObjectInstanceID,
	located func(model.ObjectInstance) bool,
	detectMagic bool,
	detectInvisible bool,
) []string {
	groups := objectListLookGroups(world, ids, located, detectMagic, detectInvisible)
	texts := make([]string, 0, len(groups))
	for _, group := range groups {
		if group.Count > 1 {
			texts = append(texts, fmt.Sprintf("(x%d) %s", group.Count, group.Text))
			continue
		}
		texts = append(texts, group.Text)
	}
	return texts
}

func objectListLookGroups(
	world objectNameWorld,
	ids []model.ObjectInstanceID,
	located func(model.ObjectInstance) bool,
	detectMagic bool,
	detectInvisible bool,
) []objectLookGroup {
	groups := make([]objectLookGroup, 0, len(ids))
	for _, id := range ids {
		object, ok := world.Object(id)
		if !ok || !located(object) || !objectVisibleInRoomLook(world, object, detectInvisible) {
			continue
		}
		baseName := objectDisplayName(world, object)
		if baseName == "" {
			continue
		}
		name := objectMagicDisplayName(world, object, detectMagic)
		if name == "" {
			continue
		}
		adjustment := objectIntPropertyOrDefault(world, object, "adjustment", "adjust")
		last := len(groups) - 1
		if last >= 0 && groups[last].BaseName == baseName &&
			(!detectMagic || groups[last].Adjustment == adjustment) {
			groups[last].Count++
			groups[last].Text = name
			groups[last].Adjustment = adjustment
			continue
		}
		groups = append(groups, objectLookGroup{
			Text:       name,
			BaseName:   baseName,
			Adjustment: adjustment,
			Count:      1,
		})
	}
	return groups
}

func playerVisibleInRoomLook(world LookWorld, player model.Player, viewer LookViewer, detectInvisible bool) bool {
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn") {
		return false
	}
	seesDMInvisible := viewerSeesDMInvisible(world, viewer)
	if hasAnyNormalizedFlag(player.Metadata.Tags, "dmInvisible", "pdminv") && !seesDMInvisible {
		return false
	}
	if hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis") && !detectInvisible {
		return false
	}
	if player.CreatureID.IsZero() {
		return true
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return true
	}
	return playerCreatureVisibleInRoomLook(creature, viewer, detectInvisible, seesDMInvisible)
}

func creatureVisibleInRoomLook(creature model.Creature, viewer LookViewer, detectInvisible bool) bool {
	if creature.ID == viewer.CreatureID {
		return false
	}
	if creatureHasAnyFlag(creature, "hidden", "phiddn", "PHIDDN", "mhiddn", "MHIDDN", "dmInvisible", "pdminv", "PDMINV") {
		return false
	}
	if creatureHasAnyFlag(creature, "invisible", "pinvis", "PINVIS", "minvis", "MINVIS") && !detectInvisible {
		return false
	}
	return true
}

func playerCreatureVisibleInRoomLook(creature model.Creature, viewer LookViewer, detectInvisible bool, seesDMInvisible bool) bool {
	if creature.ID == viewer.CreatureID {
		return false
	}
	if creatureHasAnyFlag(creature, "hidden", "phiddn", "PHIDDN", "mhiddn", "MHIDDN") {
		return false
	}
	if creatureHasAnyFlag(creature, "dmInvisible", "pdminv", "PDMINV") && !seesDMInvisible {
		return false
	}
	if creatureHasAnyFlag(creature, "invisible", "pinvis", "PINVIS", "minvis", "MINVIS") && !detectInvisible {
		return false
	}
	return true
}

func viewerSeesDMInvisible(world LookWorld, viewer LookViewer) bool {
	_, creature, ok := lookViewerActor(world, viewer)
	return ok && creatureClass(creature) >= legacyClassDM
}

func creatureHPDead(creature model.Creature) bool {
	hp, ok := creature.Stats["hpCurrent"]
	return ok && hp <= 0
}

func objectVisibleInRoomLook(world objectNameWorld, object model.ObjectInstance, detectInvisible bool) bool {
	if objectHasAnyLookFlag(world, object, "hidden", "ohiddn", "OHIDDN", "scenery", "scene", "oscene", "OSCENE") {
		return false
	}
	if objectHasAnyLookFlag(world, object, "invisible", "oinvis", "OINVIS") && !detectInvisible {
		return false
	}
	return true
}

func objectVisibleForFindObj(world objectNameWorld, object model.ObjectInstance, detectInvisible bool) bool {
	if objectHasAnyLookFlag(world, object, "invisible", "oinvis", "OINVIS") && !detectInvisible {
		return false
	}
	return true
}

func objectHasAnyLookFlag(world objectNameWorld, object model.ObjectInstance, names ...string) bool {
	targets := normalizedFlagSet(names...)
	if hasAnyNormalizedFlag(object.Metadata.Tags, names...) || objectPropertiesHaveAnyFlag(object.Properties, targets) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return false
	}
	return hasAnyNormalizedFlag(proto.Metadata.Tags, names...) || objectPropertiesHaveAnyFlag(proto.Properties, targets)
}

func viewerDetectsInvisible(world LookWorld, viewer LookViewer) bool {
	if viewer.CreatureID.IsZero() {
		return false
	}
	creature, ok := world.Creature(viewer.CreatureID)
	if !ok {
		return false
	}
	return viewerHasDetectInvisibleTag(creature)
}

func viewerHasDetectInvisibleTag(creature model.Creature) bool {
	return creatureHasAnyFlag(creature, "detectInvisible", "pdinvi", "detectInvis", "PDINVI")
}

func viewerDetectsMagic(world LookWorld, viewer LookViewer) bool {
	if !viewer.CreatureID.IsZero() {
		if creature, ok := world.Creature(viewer.CreatureID); ok && viewerHasDetectMagicCreatureTag(creature) {
			return true
		}
	}
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok && viewerHasDetectMagicPlayerTag(player) {
			return true
		}
	}
	return false
}

func viewerKnowsAlignment(world LookWorld, viewer LookViewer) bool {
	player, creature, ok := lookViewerActor(world, viewer)
	return ok && statusEffectActive(player, creature, "PKNOWA", "knowAlignment", "alignmentSense")
}

func viewerHasDetectMagicCreatureTag(creature model.Creature) bool {
	return creatureHasAnyFlag(creature, "detectMagic", "pdmagi", "PDMAGI", "dMagic")
}

func viewerHasDetectMagicPlayerTag(player model.Player) bool {
	return hasAnyNormalizedFlag(player.Metadata.Tags, "detectMagic", "pdmagi", "dMagic")
}

func objectMagicDisplayName(world objectNameWorld, object model.ObjectInstance, detectMagic bool) string {
	name := objectDisplayName(world, object)
	if !detectMagic || name == "" {
		return name
	}
	if adjustment := objectIntPropertyOrDefault(world, object, "adjustment", "adjust"); adjustment != 0 {
		return fmt.Sprintf("%s(%+d)", name, adjustment)
	}
	if objectIntPropertyOrDefault(world, object, "magicPower", "magicpower") != 0 {
		return name + "(주문)"
	}
	return name
}

func objectHasMagicInformation(world objectNameWorld, object model.ObjectInstance) bool {
	return objectIntPropertyOrDefault(world, object, "adjustment", "adjust") != 0 ||
		objectIntPropertyOrDefault(world, object, "magicPower", "magicpower") != 0
}

func objectIntPropertyOrDefault(world objectNameWorld, object model.ObjectInstance, keys ...string) int {
	value, _ := objectFirstIntProperty(world, object, keys...)
	return value
}

func objectFirstIntProperty(world objectNameWorld, object model.ObjectInstance, keys ...string) (int, bool) {
	for _, key := range keys {
		if value, ok := parseObjectInt(object.Properties[key]); ok {
			return value, true
		}
	}
	if object.PrototypeID.IsZero() {
		return 0, false
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return 0, false
	}
	for _, key := range keys {
		if value, ok := parseObjectInt(proto.Properties[key]); ok {
			return value, true
		}
	}
	return 0, false
}
