package command

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

var (
	ErrMoveWorldRequired     = errors.New("move world required")
	ErrMoveActorRequired     = errors.New("move actor required")
	ErrMoveDirectionRequired = errors.New("move direction required")
)

const moveDMClass = legacyClassDM

type MoveWorld interface {
	LookWorld
	MovePlayer(model.PlayerID, string) error
}

type MoveMarriageInviteWorld interface {
	HasMarriageInvite(model.PlayerID, model.SpecialID) bool
}

type MoveRoomTrackWorld interface {
	UpdateRoomProperty(model.RoomID, string, string) error
}

type MoveCreatureToRoomWorld interface {
	MoveCreatureToRoom(model.CreatureID, model.RoomID) error
}

type moveCreatureTagWorld interface {
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
}

type movePlayerTagWorld interface {
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
}

type moveCreatureStatWorld interface {
	SetCreatureStat(model.CreatureID, string, int) error
}

type moveCreatureCooldownWorld interface {
	CreatureCooldownExpires(model.CreatureID, string) (int64, bool, error)
}

func NewMoveHandler(world MoveWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		if world == nil {
			return StatusDefault, ErrMoveWorldRequired
		}

		playerID := MovePlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrMoveActorRequired
		}

		viewer, currentRoom, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}

		exitName, exit, userMessage, err := selectMoveExitCandidate(world, viewer, currentRoom, resolved)
		if err != nil {
			return StatusDefault, err
		}
		if userMessage != "" {
			ctx.WriteString(userMessage)
			return StatusDefault, nil
		}
		if userMessage := movePreExitValidationMessage(world, viewer, currentRoom, resolved.Spec.Handler); userMessage != "" {
			ctx.WriteString(userMessage)
			return StatusDefault, nil
		}
		if userMessage, err := validateSelectedMoveExit(world, viewer, currentRoom, exit, resolved.Spec.Handler); err != nil {
			return StatusDefault, err
		} else if userMessage != "" {
			ctx.WriteString(userMessage)
			return StatusDefault, nil
		}

		if stop, err := handleMoveFall(ctx, world, viewer, currentRoom, exit, resolved.Spec.Handler); err != nil {
			return StatusDefault, err
		} else if stop {
			return StatusDefault, nil
		}
		if stop, err := handleMoveAttackCooldown(ctx, world, viewer, resolved.Spec.Handler); err != nil {
			return StatusDefault, err
		} else if stop {
			return StatusDefault, nil
		}
		if stop, err := handleMoveHiddenSneak(ctx, world, viewer, currentRoom); err != nil {
			return StatusDefault, err
		} else if stop {
			return StatusDefault, nil
		}
		if err := recordMoveTrack(world, currentRoom, exitName); err != nil {
			return StatusDefault, err
		}
		if err := world.MovePlayer(playerID, exitName); err != nil {
			return StatusDefault, err
		}
		moveDMFollowersAfterMove(ctx, world, viewer, currentRoom, exit, resolved.Spec.Handler)

		viewer, room, err := CurrentRoom(world, viewer)
		if err != nil {
			return StatusDefault, fmt.Errorf("move: render destination: %w", err)
		}

		ctx.WriteString(RenderRoomLook(world, room, viewer))
		if err := checkMoveRoomTrap(ctx, world, viewer, room); err != nil {
			return StatusDefault, err
		}
		return StatusDefault, nil
	}
}

func MovePlayerIDFromContext(ctx *Context) model.PlayerID {
	if ctx == nil || ctx.ActorID == "" {
		return ""
	}
	return model.PlayerID(ctx.ActorID)
}

func firstMoveArg(resolved ResolvedCommand) string {
	if len(resolved.Args) == 0 {
		return ""
	}
	return strings.TrimSpace(resolved.Args[0])
}

func selectMoveExit(world MoveWorld, viewer LookViewer, room model.Room, resolved ResolvedCommand) (string, model.Exit, string, error) {
	exitName, exit, userMessage, err := selectMoveExitCandidate(world, viewer, room, resolved)
	if err != nil || userMessage != "" {
		return exitName, exit, userMessage, err
	}
	userMessage, err = validateSelectedMoveExit(world, viewer, room, exit, resolved.Spec.Handler)
	if err != nil || userMessage != "" {
		return "", model.Exit{}, userMessage, err
	}
	return exitName, exit, "", nil
}

func selectMoveExitCandidate(world MoveWorld, viewer LookViewer, room model.Room, resolved ResolvedCommand) (string, model.Exit, string, error) {
	detectInvisible := viewerDetectsInvisible(world, viewer)
	if resolved.Spec.Handler == "move" {
		exitName := normalizeMoveCommand(resolved.Command())
		exit, ok := findExactMoveExit(room.Exits, exitName, detectInvisible)
		if !ok {
			return "", model.Exit{}, "길이 막혀 있습니다.\n", nil
		}
		return exit.Name, exit, "", nil
	}

	prefix := firstMoveArg(resolved)
	if prefix == "" {
		return "", model.Exit{}, "어디로 가고 싶으세요?\n", nil
	}
	exit, ok := findPrefixMoveExit(room.Exits, prefix, firstMoveOrdinal(resolved), detectInvisible)
	if !ok {
		return "", model.Exit{}, "그런 출구는 없습니다.\n", nil
	}
	return exit.Name, exit, "", nil
}

func validateSelectedMoveExit(world MoveWorld, viewer LookViewer, room model.Room, exit model.Exit, handler string) (string, error) {
	if message := blockedMoveExitMessage(world, viewer, room, exit, handler); message != "" {
		return message, nil
	}
	destination, ok := world.Room(exit.ToRoomID)
	if !ok {
		if handler == "move" {
			return "그쪽으로 지도가 없습니다. 신에게 연락해 주세요.\n", nil
		}
		return "그 방향의 지도가 없습니다.\n", nil
	}
	if message := blockedDestinationRoomMessage(world, viewer, destination, handler); message != "" {
		return message, nil
	}
	return "", nil
}

func movePreExitValidationMessage(world MoveWorld, viewer LookViewer, room model.Room, handler string) string {
	if moveViewerHasAnyFlag(world, viewer, "silence", "silenced", "PSILNC") {
		if handler == "go" {
			return "당신은 움직일 수가 없습니다."
		}
		return "당신은 움직일수 없습니다."
	}
	actor, ok := moveViewerCreature(world, viewer)
	if !ok {
		return ""
	}
	if attackActorAlreadyFighting(world, room, viewer, actor) {
		return "싸우는 중에는 이동할 수 없습니다."
	}
	return ""
}

func normalizeMoveCommand(command string) string {
	switch strings.TrimSpace(command) {
	case "8", "ㅂ":
		return "북"
	case "2", "ㄴ":
		return "남"
	case "6", "ㄷ":
		return "동"
	case "4", "ㅅ":
		return "서"
	case "9", "ㅇ":
		return "위"
	case "3", "ㅁ":
		return "밑"
	case "나가":
		return "밖"
	default:
		return strings.TrimSpace(command)
	}
}

func firstMoveOrdinal(resolved ResolvedCommand) int64 {
	if len(resolved.Values) == 0 || resolved.Values[0] < 1 {
		return 1
	}
	return resolved.Values[0]
}

func findExactMoveExit(exits []model.Exit, name string, detectInvisible bool) (model.Exit, bool) {
	for _, exit := range exits {
		if exit.Name == name && exactMoveExitSelectable(exit, detectInvisible) {
			return exit, true
		}
	}
	return model.Exit{}, false
}

func findPrefixMoveExit(exits []model.Exit, prefix string, ordinal int64, detectInvisible bool) (model.Exit, bool) {
	var seen int64
	for _, exit := range exits {
		if strings.HasPrefix(exit.Name, prefix) && moveExitSelectable(exit, detectInvisible) {
			seen++
			if seen == ordinal {
				return exit, true
			}
		}
	}
	return model.Exit{}, false
}

func exactMoveExitSelectable(exit model.Exit, detectInvisible bool) bool {
	return !exitHasAnyFlag(exit, "noSee", "xnosee")
}

func moveExitSelectable(exit model.Exit, detectInvisible bool) bool {
	return exitTargetVisible(exit, detectInvisible)
}

func blockedMoveExitMessage(world MoveWorld, viewer LookViewer, room model.Room, exit model.Exit, handler string) string {
	if exitHasAnyFlag(exit, "locked", "xlockd", "xlocked") {
		if handler == "move" {
			return "문이 잠겨 있습니다.\n"
		}
		if handler == "sneak" {
			return "그 출구는 잠겨져 있습니다.\n"
		}
		return "그 출구는 잠겨 있습니다.\n"
	}
	if exitHasAnyFlag(exit, "closed", "xclosd", "xclosed") {
		if handler == "move" {
			return "문이 닫혀 있습니다.\n"
		}
		if handler == "sneak" {
			return "먼저 문을 열어야 겠군요.\n"
		}
		return "그 출구는 닫혀 있습니다.\n"
	}
	if exitHasAnyFlag(exit, "fly", "xflysp") && !moveViewerHasAnyFlag(world, viewer, "fly", "flying", "PFLYSP") {
		return blockedFlyMoveMessage(handler)
	}
	if guard, ok := guardedMoveExitBlocker(world, viewer, room, exit, handler); ok {
		return blockedGuardMoveMessage(guard, handler)
	}
	if exitHasAnyFlag(exit, "nightOnly", "xnghto") && moveLegacyHour(world) > 6 && moveLegacyHour(world) < 20 {
		return blockedNightOnlyMoveMessage(handler)
	}
	if exitHasAnyFlag(exit, "dayOnly", "xdayon") && (moveLegacyHour(world) < 6 || moveLegacyHour(world) > 20) {
		return blockedDayOnlyMoveMessage(handler)
	}
	if exitHasAnyFlag(exit, "femaleOnly", "xfemal") && moveViewerHasAnyFlag(world, viewer, "male", "PMALES") {
		return blockedFemaleOnlyMoveMessage(handler)
	}
	if exitHasAnyFlag(exit, "maleOnly", "xmales") && !moveViewerHasAnyFlag(world, viewer, "male", "PMALES") {
		return blockedMaleOnlyMoveMessage(handler)
	}
	if exitHasAnyFlag(exit, "naked", "xnaked") && moveViewerCarriedWeight(world, viewer) != 0 {
		return blockedNakedMoveMessage(handler)
	}
	return ""
}

func blockedFlyMoveMessage(handler string) string {
	if handler == "move" {
		return "그 쪽으로는 날아서 가야 될것 같군요.\n"
	}
	if handler == "sneak" {
		return "그 쪽에는 날아서만 갈 수 있습니다.\n"
	}
	return "그 쪽으로는 날아서 가야 될것 같군요.\n"
}

func blockedGuardMoveMessage(guard model.Creature, handler string) string {
	name := attackCreatureName(guard)
	if handler == "move" {
		return name + krtext.Particle(name, '1') + " 당신의 길을 막습니다.\n"
	}
	if handler == "sneak" {
		return name + krtext.Particle(name, '1') + " 당신의 길을 가로막습니다.\n"
	}
	return name + krtext.Particle(name, '1') + " 당신의 길을 막습니다.\n"
}

func blockedNightOnlyMoveMessage(handler string) string {
	if handler == "move" {
		return "그 출구는 밤에만 열려 있습니다.\n"
	}
	if handler == "sneak" {
		return "그 출구는 밤에만 갈 수 있습니다.\n"
	}
	return "그 출구는 밤에만 열려 있습니다.\n"
}

func blockedDayOnlyMoveMessage(handler string) string {
	if handler == "move" {
		return "그 출구는 밤에는 닫혀 있습니다.\n"
	}
	if handler == "sneak" {
		return "그 출구는 낮에만 갈 수 있습니다.\n"
	}
	return "그 출구는 밤에는 닫혀 있습니다.\n"
}

func blockedFemaleOnlyMoveMessage(handler string) string {
	if handler == "move" {
		return "여성만 들어갈수 있습니다. 여탕인가~~\n"
	}
	if handler == "sneak" {
		return "그 쪽으로는 여성만 갈 수 있습니다.\n"
	}
	return "여성만 들어갈수 있습니다.\n"
}

func blockedMaleOnlyMoveMessage(handler string) string {
	if handler == "move" {
		return "남성만 들어갈수 있습니다.\n"
	}
	if handler == "sneak" {
		return "그 쪽으로는 남성만 갈 수 있습니다.\n"
	}
	return "남성만 들어갈수 있습니다.\n"
}

func blockedNakedMoveMessage(handler string) string {
	if handler == "move" {
		return "뭘 가지고는 들어갈수 없습니다.\n"
	}
	if handler == "sneak" {
		return "그 쪽으로는 뭘 들고는 갈 수 없습니다.\n"
	}
	return "뭘 가지고는 들어갈 수 없습니다.\n"
}

func moveViewerCarriedWeight(world MoveWorld, viewer LookViewer) int {
	if world == nil || viewer.CreatureID.IsZero() {
		return 0
	}
	creature, ok := world.Creature(viewer.CreatureID)
	if !ok {
		return 0
	}
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		weight += moveCarriedObjectWeight(world, objectID, true, seen)
	}
	for _, objectID := range creature.Equipment {
		weight += moveCarriedObjectWeight(world, objectID, false, seen)
	}
	return weight
}

func moveViewerHasAnyFlag(world MoveWorld, viewer LookViewer, names ...string) bool {
	if creature, ok := moveViewerCreature(world, viewer); ok && creatureHasAnyFlag(creature, names...) {
		return true
	}
	if viewer.PlayerID.IsZero() {
		return false
	}
	player, ok := world.Player(viewer.PlayerID)
	return ok && hasAnyNormalizedFlag(player.Metadata.Tags, names...)
}

func moveLegacyHour(world MoveWorld) int {
	return lookLegacyHour(world)
}

func handleMoveFall(ctx *Context, world MoveWorld, viewer LookViewer, room model.Room, exit model.Exit, handler string) (bool, error) {
	if !exitHasAnyFlag(exit, "climb", "XCLIMB", "repel", "XREPEL") {
		return false, nil
	}
	actor, ok := moveViewerCreature(world, viewer)
	if !ok || moveViewerHasAnyFlag(world, viewer, "levitate", "levitation", "PLEVIT") {
		return false, nil
	}
	fall := 50 - moveFallBonus(world, actor)
	if exitHasAnyFlag(exit, "difficultClimb", "XDCLIM") {
		fall += 50
	}
	if mrand(1, 100) >= fall {
		return false, nil
	}
	damage := mrand(5, 15+fall/10)
	deadBefore := creatureStat(actor, "hpCurrent") <= damage
	updated, _, dead, err := moveTrapApplyDamage(world, actor, damage)
	if err != nil {
		return false, err
	}
	if deadBefore || dead {
		writeMoveFallFatal(ctx, room, actor, handler)
		if err := moveTrapFinalizePlayerDeath(ctx, world, viewer, updated); err != nil {
			return false, err
		}
		return true, nil
	}
	writeMoveFallDamage(ctx, room, actor, handler, damage)
	if exitHasAnyFlag(exit, "climb", "XCLIMB") {
		return true, nil
	}
	return false, nil
}

func handleMoveAttackCooldown(ctx *Context, world MoveWorld, viewer LookViewer, handler string) (bool, error) {
	if handler != "go" {
		return false, nil
	}
	reader, ok := world.(moveCreatureCooldownWorld)
	if !ok || viewer.CreatureID.IsZero() {
		return false, nil
	}
	expires, found, err := reader.CreatureCooldownExpires(viewer.CreatureID, "attack")
	if err != nil || !found {
		return false, err
	}
	now := timeNow().Unix()
	if now >= expires {
		return false, nil
	}
	ctx.WriteString(renderPleaseWait(expires - now))
	return true, nil
}

func handleMoveHiddenSneak(ctx *Context, world MoveWorld, viewer LookViewer, room model.Room) (bool, error) {
	actor, ok := moveViewerCreature(world, viewer)
	if !ok {
		return false, nil
	}
	class := creatureClass(actor)
	hidden := attackCreatureHasFlag(actor, "hidden", "phiddn", "PHIDDN")
	if (class == legacyClassAssassin || class == legacyClassThief || class > legacyClassInvincible) && hidden {
		if mrand(1, 100) <= sneakChance(actor) {
			return false, nil
		}
		ctx.WriteString("당신은 은신술을 사용하는데 실패하였습니다.\n")
		updated, err := clearMoveHiddenState(world, viewer.PlayerID, actor)
		if err != nil {
			return false, err
		}
		if !updated.ID.IsZero() {
			actor = updated
		}
		for _, cid := range room.CreatureIDs {
			monster, ok := world.Creature(cid)
			if !ok || monster.Kind != model.CreatureKindMonster {
				continue
			}
			if creatureHasAnyFlag(monster, "blocksExits", "MBLOCK", "mblock") &&
				sneakMonsterTargetsActor(world, monster.ID, viewer.PlayerID, actor) &&
				!attackCreatureHasFlag(actor, "invisible", "pinvis", "PINVIS") &&
				class < legacyClassSubDM {
				monsterName := attackCreatureName(monster)
				ctx.WriteString(monsterName + "가 당신의 길을 가로막습니다.\n")
				return true, nil
			}
		}
		return false, nil
	}
	if hidden {
		if _, err := clearMoveHiddenState(world, viewer.PlayerID, actor); err != nil {
			return false, err
		}
	}
	return false, nil
}

func clearMoveHiddenState(world MoveWorld, playerID model.PlayerID, actor model.Creature) (model.Creature, error) {
	updated := actor
	if updater, ok := world.(moveCreatureTagWorld); ok {
		next, err := updater.UpdateCreatureTags(actor.ID, nil, []string{"hidden", "phiddn", "PHIDDN"})
		if err != nil {
			return model.Creature{}, err
		}
		updated = next
	}
	if setter, ok := world.(moveCreatureStatWorld); ok {
		if err := setter.SetCreatureStat(actor.ID, "PHIDDN", 0); err != nil {
			return model.Creature{}, err
		}
	}
	if updater, ok := world.(movePlayerTagWorld); ok && !playerID.IsZero() {
		if _, err := updater.UpdatePlayerTags(playerID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
			return model.Creature{}, err
		}
	}
	if current, ok := world.Creature(actor.ID); ok {
		updated = current
	}
	return updated, nil
}

func moveFallBonus(world MoveWorld, actor model.Creature) int {
	fall := legacyStatBonus(creatureStat(actor, "dexterity")) * 5
	for _, objectID := range actor.Equipment {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectHasAnyFlagOrProperty(world, object, "OCLIMB", "climbGear", "climbing") {
			continue
		}
		pDice, _ := objectIntProperty(world, object, "pDice")
		fall += pDice * 3
	}
	return fall
}

func writeMoveFallDamage(ctx *Context, room model.Room, actor model.Creature, handler string, damage int) {
	actorName := attackCreatureName(actor)
	if handler == "sneak" {
		ctx.WriteString(fmt.Sprintf("당신은 떨어져서 %d만큼의 상처를 입었습니다.\n", damage))
		_ = roomBroadcast(ctx, room.ID, actorName+krtext.Particle(actorName, '1')+" 구덩이에 빠졌습니다.")
		return
	}
	ctx.WriteString(fmt.Sprintf("당신은 구덩이에 떨어져서 %d 만큼의 상처를 입었습니다", damage))
	_ = roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 구덩이에 떨어졌습니다.")
}

func writeMoveFallFatal(ctx *Context, room model.Room, actor model.Creature, handler string) {
	actorName := attackCreatureName(actor)
	if handler == "sneak" {
		ctx.WriteString("당신은 깊은 구덩이에 빠졌습니다..\n")
		_ = roomBroadcast(ctx, room.ID, actorName+krtext.Particle(actorName, '1')+" 구덩이에 빠져서 죽었습니다.\n")
		return
	}
	ctx.WriteString("당신은 죽음이 다가오는것같은 느낌이 듭니다.")
	_ = roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 구덩이에 떨어져서 죽었습니다.")
}

func moveDMFollowersAfterMove(ctx *Context, world MoveWorld, viewer LookViewer, origin model.Room, exit model.Exit, handler string) {
	mover, ok := world.(MoveCreatureToRoomWorld)
	if !ok || origin.ID.IsZero() || exit.ToRoomID.IsZero() {
		return
	}
	for _, creatureID := range origin.CreatureIDs {
		follower, ok := world.Creature(creatureID)
		if !ok || follower.Kind != model.CreatureKindMonster || follower.RoomID != origin.ID {
			continue
		}
		if !creatureHasAnyFlag(follower, "MDMFOL", "dmFollow", "followDM") {
			continue
		}
		if !moveDMFollowerBelongsToActor(ctx, world, viewer, follower) {
			continue
		}
		_ = roomBroadcast(ctx, origin.ID, moveDMFollowerDepartMessage(follower, exit.Name, handler))
		_ = mover.MoveCreatureToRoom(follower.ID, exit.ToRoomID)
	}
}

func moveDMFollowerBelongsToActor(ctx *Context, world MoveWorld, viewer LookViewer, follower model.Creature) bool {
	leaderID := strings.TrimSpace(follower.Properties[dmFollowLeaderProperty])
	leaderCreatureID := strings.TrimSpace(follower.Properties[dmFollowLeaderCreatureProperty])
	if leaderID == "" && leaderCreatureID == "" {
		return true
	}
	if ctx != nil && strings.TrimSpace(ctx.ActorID) != "" {
		actorID := strings.TrimSpace(ctx.ActorID)
		if leaderID == actorID || leaderCreatureID == actorID {
			return true
		}
	}
	if !viewer.PlayerID.IsZero() && leaderID == string(viewer.PlayerID) {
		return true
	}
	if !viewer.CreatureID.IsZero() && leaderCreatureID == string(viewer.CreatureID) {
		return true
	}
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" {
		return false
	}
	if player, ok := world.Player(model.PlayerID(strings.TrimSpace(ctx.ActorID))); ok && !player.CreatureID.IsZero() {
		return leaderCreatureID == string(player.CreatureID)
	}
	return false
}

func moveDMFollowerDepartMessage(follower model.Creature, exitName string, handler string) string {
	name := attackCreatureName(follower)
	if handler == "go" {
		return "\n" + name + krtext.Particle(name, '1') + " 방황하다 " + exitName + "쪽으로 갔습니다."
	}
	return "\n" + name + "이 " + exitName + "쪽으로 갔습니다."
}

func guardedMoveExitBlocker(world MoveWorld, viewer LookViewer, room model.Room, exit model.Exit, handler string) (model.Creature, bool) {
	if !exitHasAnyFlag(exit, "guarded", "XPGUAR") {
		return model.Creature{}, false
	}
	actor, actorOK := moveViewerCreature(world, viewer)
	class := moveCreatureClass(actor, actorOK)
	threshold := legacyClassSubDM
	if handler == "sneak" {
		threshold = legacyClassCaretaker
	}
	actorInvisible := moveViewerHasAnyFlag(world, viewer, "invisible", "PINVIS")
	for _, creatureID := range room.CreatureIDs {
		guard, ok := world.Creature(creatureID)
		if !ok || guard.Kind != model.CreatureKindMonster || !creatureHasAnyFlag(guard, "MPGUAR", "passiveExitGuard") {
			continue
		}
		if (!actorInvisible || creatureHasAnyFlag(guard, "MDINVI", "detectInvisible")) && class < threshold {
			return guard, true
		}
	}
	return model.Creature{}, false
}

func moveCarriedObjectWeight(world MoveWorld, objectID model.ObjectInstanceID, skipWeightless bool, seen map[model.ObjectInstanceID]struct{}) int {
	if objectID.IsZero() {
		return 0
	}
	if _, ok := seen[objectID]; ok {
		return 0
	}
	seen[objectID] = struct{}{}

	object, ok := world.Object(objectID)
	if !ok {
		return 0
	}
	if skipWeightless && moveObjectWeightless(world, object) {
		return 0
	}

	weight := moveObjectOwnWeight(world, object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += moveCarriedObjectWeight(world, childID, true, seen)
	}
	return weight
}

func moveObjectOwnWeight(world MoveWorld, object model.ObjectInstance) int {
	if weight, ok := parseMoveObjectWeight(object.Properties["weight"]); ok {
		return weight
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if weight, ok := parseMoveObjectWeight(proto.Properties["weight"]); ok {
				return weight
			}
		}
	}
	return 0
}

func parseMoveObjectWeight(value string) (int, bool) {
	weight, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return weight, true
}

func moveObjectWeightless(world MoveWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "weightless", "owtles") ||
		objectHasAnyPropertyFlag(world, object, "weightless", "owtles")
}

func objectHasAnyFlag(flags []string, names ...string) bool {
	return hasAnyNormalizedFlag(flags, names...)
}

func blockedDestinationRoomMessage(world MoveWorld, viewer LookViewer, room model.Room, handler string) string {
	level := moveViewerLevel(world, viewer)
	if minLevel, ok := roomMinLevel(room); ok && level < minLevel {
		return blockedDestinationMoveMessage(handler)
	}
	if maxLevel, ok := roomMaxLevel(room); ok && level > maxLevel {
		return blockedDestinationMoveMessage(handler)
	}
	if playerLimit := roomPlayerLimit(room); playerLimit > 0 && moveVisiblePlayerCount(world, room) >= playerLimit {
		return blockedDestinationMoveMessage(handler)
	}

	creature, hasCreature := moveViewerCreature(world, viewer)
	if roomHasAnyFlag(room, "family") && !moveCreatureHasFamilyFlag(creature, hasCreature) {
		return blockedDestinationMoveMessage(handler)
	}
	if roomHasAnyFlag(room, "onlyFamily", "familyOnly", "ronfml") &&
		moveCreatureClass(creature, hasCreature) < moveDMClass &&
		!moveCreatureMatchesRoomSpecial(room, creature, hasCreature, "familyID", "dailyExpndMax", "legacyDailyExpndMax") {
		return blockedDestinationMoveMessage(handler)
	}
	if roomHasAnyFlag(room, "onlyMarried", "marriedOnly", "ronmar") &&
		moveCreatureClass(creature, hasCreature) < moveDMClass &&
		!moveCreatureMatchesRoomSpecial(room, creature, hasCreature, "marriageID", "dailyMarriageMax", "legacyDailyMarriageMax") &&
		!moveMarriageInviteAllowed(world, viewer, room) {
		return blockedDestinationMoveMessage(handler)
	}
	return ""
}

func moveMarriageInviteAllowed(world MoveWorld, viewer LookViewer, room model.Room) bool {
	inviteWorld, ok := world.(MoveMarriageInviteWorld)
	if !ok || viewer.PlayerID.IsZero() {
		return false
	}
	roomSpecial, ok := moveRoomSpecialInt(room)
	if !ok {
		return false
	}
	return inviteWorld.HasMarriageInvite(viewer.PlayerID, model.SpecialID(roomSpecial))
}

func blockedDestinationMoveMessage(handler string) string {
	if handler == "move" {
		return "그쪽으로 갈 수 없습니다.\n"
	}
	return "그 방향으로 갈 수 없습니다.\n"
}

func recordMoveTrack(world MoveWorld, room model.Room, exitName string) error {
	if room.ID.IsZero() || exitName == "" || roomHasAnyFlag(room, "RPTRAK", "permanentTracks") {
		return nil
	}
	updater, ok := world.(MoveRoomTrackWorld)
	if !ok {
		return nil
	}
	return updater.UpdateRoomProperty(room.ID, "track", exitName)
}

func moveViewerLevel(world MoveWorld, viewer LookViewer) int {
	creature, ok := moveViewerCreature(world, viewer)
	if !ok {
		return 0
	}
	if level, ok := moveCreatureStatOrPropertyInt(creature, "level"); ok {
		return level
	}
	return creature.Level
}

func moveViewerCreature(world MoveWorld, viewer LookViewer) (model.Creature, bool) {
	if world == nil || viewer.CreatureID.IsZero() {
		return model.Creature{}, false
	}
	return world.Creature(viewer.CreatureID)
}

func moveVisiblePlayerCount(world LookWorld, room model.Room) int {
	if world == nil {
		return len(room.PlayerIDs)
	}
	count := 0
	seen := map[model.PlayerID]struct{}{}
	for _, playerID := range room.PlayerIDs {
		if playerID.IsZero() {
			continue
		}
		seen[playerID] = struct{}{}
		player, ok := world.Player(playerID)
		if !ok {
			count++
			continue
		}
		if movePlayerDMInvisible(world, player) {
			continue
		}
		count++
	}
	for _, creatureID := range room.CreatureIDs {
		creature, ok := world.Creature(creatureID)
		if !ok || creature.PlayerID.IsZero() {
			continue
		}
		if _, ok := seen[creature.PlayerID]; ok {
			continue
		}
		if creatureHasAnyFlag(creature, "PDMINV", "dmInvisible") {
			continue
		}
		count++
	}
	return count
}

func movePlayerDMInvisible(world LookWorld, player model.Player) bool {
	if hasAnyNormalizedFlag(player.Metadata.Tags, "PDMINV", "dmInvisible") {
		return true
	}
	if player.CreatureID.IsZero() {
		return false
	}
	creature, ok := world.Creature(player.CreatureID)
	return ok && creatureHasAnyFlag(creature, "PDMINV", "dmInvisible")
}

func moveCreatureClass(creature model.Creature, hasCreature bool) int {
	if !hasCreature {
		return 0
	}
	class, _ := moveCreatureStatOrPropertyInt(creature, "class")
	return class
}

func moveCreatureHasFamilyFlag(creature model.Creature, hasCreature bool) bool {
	return hasCreature && moveCreatureStatOrPropertyFlag(creature, "familyFlag", "PFAMIL")
}

func moveCreatureMatchesRoomSpecial(room model.Room, creature model.Creature, hasCreature bool, keys ...string) bool {
	if !hasCreature {
		return false
	}
	special, ok := roomPropertyValue(room, "special")
	if !ok {
		return false
	}
	value, ok := moveCreatureStatOrPropertyValue(creature, keys...)
	return ok && moveRestrictionValuesEqual(value, special)
}

func moveCreatureStatOrPropertyInt(creature model.Creature, keys ...string) (int, bool) {
	value, ok := moveCreatureStatOrPropertyValue(creature, keys...)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return n, true
}

func moveCreatureStatOrPropertyFlag(creature model.Creature, keys ...string) bool {
	for _, key := range keys {
		if value, ok := moveCreatureStatValue(creature.Stats, key); ok && value != 0 {
			return true
		}
		if value, ok := moveCreaturePropertyValue(creature.Properties, key); ok && movePropertyFlagEnabled(value) {
			return true
		}
	}
	return false
}

func moveCreatureStatOrPropertyValue(creature model.Creature, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := moveCreatureStatValue(creature.Stats, key); ok {
			return strconv.Itoa(value), true
		}
		if value, ok := moveCreaturePropertyValue(creature.Properties, key); ok {
			return value, true
		}
	}
	return "", false
}

func moveCreatureStatValue(stats map[string]int, key string) (int, bool) {
	target := normalizeFlagName(key)
	for statKey, value := range stats {
		if normalizeFlagName(statKey) == target {
			return value, true
		}
	}
	return 0, false
}

func moveCreaturePropertyValue(properties map[string]string, key string) (string, bool) {
	target := normalizeFlagName(key)
	for propertyKey, value := range properties {
		if normalizeFlagName(propertyKey) == target {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func roomPropertyValue(room model.Room, key string) (string, bool) {
	target := normalizeFlagName(key)
	for propertyKey, value := range room.Properties {
		if normalizeFlagName(propertyKey) == target {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func moveRoomSpecialInt(room model.Room) (int, bool) {
	value, ok := roomPropertyValue(room, "special")
	if !ok {
		return 0, false
	}
	roomSpecial, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return roomSpecial, true
}

func movePropertyFlagEnabled(value string) bool {
	if propertyFlagEnabled(value) {
		return true
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	return err == nil && n != 0
}

func moveRestrictionValuesEqual(left string, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if leftInt, leftErr := strconv.Atoi(left); leftErr == nil {
		if rightInt, rightErr := strconv.Atoi(right); rightErr == nil {
			return leftInt == rightInt
		}
	}
	return left == right
}

func roomMinLevel(room model.Room) (int, bool) {
	levels := roomLevelLimits(room, "minLevel")
	if len(levels) == 0 {
		return 0, false
	}
	minLevel := levels[0]
	for _, level := range levels[1:] {
		if level > minLevel {
			minLevel = level
		}
	}
	return minLevel, true
}

func roomMaxLevel(room model.Room) (int, bool) {
	levels := roomLevelLimits(room, "maxLevel")
	if len(levels) == 0 {
		return 0, false
	}
	maxLevel := levels[0]
	for _, level := range levels[1:] {
		if level < maxLevel {
			maxLevel = level
		}
	}
	return maxLevel, true
}

func roomLevelLimits(room model.Room, name string) []int {
	var levels []int
	target := normalizeFlagName(name)

	for key, value := range room.Properties {
		if normalizeFlagName(key) == target {
			if level, ok := parseMoveLevelValue(value); ok {
				levels = append(levels, level)
			}
		}
		levels = appendNamedMoveLevelLimits(levels, value, target)
	}

	for _, tag := range room.Metadata.Tags {
		levels = appendNamedMoveLevelLimits(levels, tag, target)
	}

	return levels
}

func appendNamedMoveLevelLimits(levels []int, value string, target string) []int {
	if level, ok := parseNamedMoveLevelLimit(value, target); ok {
		levels = append(levels, level)
	}
	for _, token := range moveRestrictionTokens(value) {
		if level, ok := parseNamedMoveLevelLimit(token, target); ok {
			levels = append(levels, level)
		}
	}
	return levels
}

func parseNamedMoveLevelLimit(value string, target string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	for _, sep := range []string{":", "="} {
		key, rawLevel, ok := strings.Cut(value, sep)
		if ok && normalizeFlagName(key) == target {
			return parseMoveLevelValue(rawLevel)
		}
	}

	normalized := normalizeFlagName(value)
	if !strings.HasPrefix(normalized, target) || len(normalized) == len(target) {
		return 0, false
	}
	return parseMoveLevelValue(normalized[len(target):])
}

func parseMoveLevelValue(value string) (int, bool) {
	level, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || level < 0 {
		return 0, false
	}
	return level, true
}

func moveRestrictionTokens(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	})
}

func roomPlayerLimit(room model.Room) int {
	limit := 0
	for _, option := range []struct {
		names []string
		limit int
	}{
		{names: []string{"onePlayer"}, limit: 1},
		{names: []string{"twoPlayers"}, limit: 2},
		{names: []string{"threePlayers"}, limit: 3},
	} {
		if roomHasAnyFlag(room, option.names...) && (limit == 0 || option.limit < limit) {
			limit = option.limit
		}
	}
	return limit
}
