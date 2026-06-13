package command

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	picklockThiefClass      = model.ClassThief
	picklockInvincibleClass = model.ClassInvincible
)

type PicklockRollFunc func(min, max int) int

type ExitControlWorld interface {
	LookWorld
	SetExitFlag(model.RoomID, string, string, bool) (model.Exit, error)
	UnlockExitWithKey(model.RoomID, string, model.ObjectInstanceID) (model.Exit, model.ObjectInstance, error)
	LockExitWithKey(model.RoomID, string, model.ObjectInstanceID) (model.Exit, model.ObjectInstance, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
}

func NewOpenExitHandler(world ExitControlWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		target, ordinal := lookTarget(resolved)
		if target == "" {
			ctx.WriteString("무엇을 열고 싶으세요?")
			return StatusDefault, nil
		}
		exit, ok := findLookTargetExitForViewer(world, viewer, room.Exits, target, ordinal)
		if !ok {
			ctx.WriteString("그런 출구는 없습니다.")
			return StatusDefault, nil
		}
		if exitHasAnyFlag(exit, "locked", "xlockd", "xlocked") {
			ctx.WriteString("그것은 잠겨져 있습니다.")
			return StatusDefault, nil
		}
		if !exitHasAnyFlag(exit, "closed", "xclosd", "xclosed") {
			ctx.WriteString("벌써 열려져 있습니다.")
			return StatusDefault, nil
		}
		player, creature, ok := exitControlActor(world, viewer)
		if !ok {
			return StatusDefault, ErrInventoryActorRequired
		}
		if err := revealExitControlActor(world, player, creature); err != nil {
			return StatusDefault, err
		}
		if _, err := world.SetExitFlag(room.ID, exit.Name, "closed", false); err != nil {
			return StatusDefault, err
		}
		if _, _, err := touchExitTimerIfSupported(world, room.ID, exit.Name); err != nil {
			return StatusDefault, err
		}
		_ = roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s이 %s쪽 출구를 열었습니다.", commandActorDisplayName(player, creature), exit.Name))
		ctx.WriteString(fmt.Sprintf("당신은 %s쪽 출구를 열었습니다.", exit.Name))
		return StatusDefault, nil
	}
}

func NewCloseExitHandler(world ExitControlWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		target, ordinal := lookTarget(resolved)
		if target == "" {
			ctx.WriteString("무엇을 닫고 싶으세요?")
			return StatusDefault, nil
		}
		exit, ok := findLookTargetExitForViewer(world, viewer, room.Exits, target, ordinal)
		if !ok {
			ctx.WriteString("그런 출구는 없습니다.")
			return StatusDefault, nil
		}
		if exitHasAnyFlag(exit, "closed", "xclosd", "xclosed") {
			ctx.WriteString("벌써 닫혀져 있습니다.")
			return StatusDefault, nil
		}
		if !exitHasAnyFlag(exit, "closable", "xcloss") {
			ctx.WriteString("당신은 그 출구를 닫을 수 없습니다.")
			return StatusDefault, nil
		}
		player, creature, ok := exitControlActor(world, viewer)
		if !ok {
			return StatusDefault, ErrInventoryActorRequired
		}
		if err := revealExitControlActor(world, player, creature); err != nil {
			return StatusDefault, err
		}
		if _, err := world.SetExitFlag(room.ID, exit.Name, "closed", true); err != nil {
			return StatusDefault, err
		}
		_ = roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s이 %s쪽 출구를 닫습니다.", commandActorDisplayName(player, creature), exit.Name))
		ctx.WriteString(fmt.Sprintf("당신은 %s쪽 출구를 닫습니다.", exit.Name))
		return StatusDefault, nil
	}
}

func NewUnlockExitHandler(world ExitControlWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}

		exitName := getArg(resolved, 0)
		if exitName == "" {
			ctx.WriteString("무엇을 풀고 싶으세요?")
			return StatusDefault, nil
		}
		exit, ok := findLookTargetExitForViewer(world, viewer, room.Exits, exitName, getOrdinal(resolved, 0))
		if !ok {
			ctx.WriteString("무엇을 풀고 싶으세요?")
			return StatusDefault, nil
		}
		if !exitHasAnyFlag(exit, "locked", "xlockd", "xlocked") {
			ctx.WriteString("그것은 잠궈져 있지 않습니다.")
			return StatusDefault, nil
		}
		keyName := getArg(resolved, 1)
		if keyName == "" {
			ctx.WriteString("뭘 가지고 열려구요?")
			return StatusDefault, nil
		}
		key, keyDisplayName, ok := findExitControlInventoryObject(world, creature, keyName, getOrdinal(resolved, 1), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
			return StatusDefault, nil
		}
		if !objectKindIs(world, key, model.ObjectKindKey) {
			ctx.WriteString("그것은 열쇠가 아닙니다.")
			return StatusDefault, nil
		}
		if shots, ok := objectIntProperty(world, key, "shotsCurrent"); !ok || shots < 1 {
			ctx.WriteString(keyDisplayName + "이 부숴져 버렸습니다.")
			return StatusDefault, nil
		}
		if !exitControlKeyMatches(world, exit, key) {
			ctx.WriteString("열쇠가 맞지 않습니다.")
			return StatusDefault, nil
		}
		if err := revealExitControlActor(world, player, creature); err != nil {
			return StatusDefault, err
		}
		updatedExit, updatedKey, err := world.UnlockExitWithKey(room.ID, exit.Name, key.ID)
		if err != nil {
			return StatusDefault, err
		}
		_ = updatedExit
		if text := exitControlUseOutput(world, updatedKey); text != "" {
			ctx.WriteString(text)
		} else {
			ctx.WriteString("## 찰칵 ##")
		}
		_ = roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s이 %s쪽 출구를 풀었습니다.", commandActorDisplayName(player, creature), exit.Name))
		return StatusDefault, nil
	}
}

func NewPicklockHandler(world ExitControlWorld, roll PicklockRollFunc) Handler {
	if roll == nil {
		roll = func(min, max int) int {
			if max <= min {
				return min
			}
			return rand.Intn(max-min+1) + min
		}
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
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}

		class := picklockCreatureStat(creature, "class")
		if class != picklockThiefClass && class < picklockInvincibleClass {
			ctx.WriteString("도둑만 자물쇠를 딸 수 있습니다.")
			return StatusDefault, nil
		}

		target, ordinal := lookTarget(resolved)
		if target == "" {
			ctx.WriteString("무엇을 따시려구요?")
			return StatusDefault, nil
		}
		exit, ok := findLookTargetExitForViewer(world, viewer, room.Exits, target, ordinal)
		if !ok {
			ctx.WriteString("그런건 여기 없습니다.")
			return StatusDefault, nil
		}
		if picklockCreatureHasFlag(creature, "blind", "pblind", "PBLIND") {
			ctx.WriteString("당신은 눈이 멀어 있어 딸 수 없습니다.")
			return StatusDefault, nil
		}
		if !exitHasAnyFlag(exit, "locked", "xlockd", "xlocked") {
			ctx.WriteString("그것은 잠궈져 있지 않습니다.")
			return StatusDefault, nil
		}
		if err := revealExitControlActor(world, player, creature); err != nil {
			return StatusDefault, err
		}
		actorName := commandActorDisplayName(player, creature)

		chance := picklockChance(creature, class)
		if exitHasAnyFlag(exit, "unpickable", "xunpck", "XUNPCK") {
			chance = 0
		}
		_ = roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s이 %s쪽 출구를 따려고 합니다.", actorName, exit.Name))
		if roll(1, 100) > chance {
			ctx.WriteString("실패하였습니다!")
			return StatusDefault, nil
		}
		if _, err := world.SetExitFlag(room.ID, exit.Name, "locked", false); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("당신은 문을 따는데 성공했습니다.")
		_ = roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s이 문을 땄습니다.", actorName))
		return StatusDefault, nil
	}
}

func NewLockExitHandler(world ExitControlWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}

		exitName := getArg(resolved, 0)
		if exitName == "" {
			ctx.WriteString("무엇을 잠굴려구요?")
			return StatusDefault, nil
		}
		exit, ok := findLookTargetExitForViewer(world, viewer, room.Exits, exitName, getOrdinal(resolved, 0))
		if !ok {
			ctx.WriteString("무엇을 잠굴려구요?")
			return StatusDefault, nil
		}
		if exitHasAnyFlag(exit, "locked", "xlockd", "xlocked") {
			ctx.WriteString("그것은 이미 잠궈져 있습니다.")
			return StatusDefault, nil
		}
		keyName := getArg(resolved, 1)
		if keyName == "" {
			ctx.WriteString("뭘 가지고 잠구려구요?")
			return StatusDefault, nil
		}
		key, keyDisplayName, ok := findExitControlInventoryObject(world, creature, keyName, getOrdinal(resolved, 1), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("당신은 그런것을 가지고 있지 않습니다.")
			return StatusDefault, nil
		}
		if !objectKindIs(world, key, model.ObjectKindKey) {
			ctx.WriteString(keyDisplayName + "은 열쇠가 아닙니다.")
			return StatusDefault, nil
		}
		if !exitHasAnyFlag(exit, "lockable", "xlocks") {
			ctx.WriteString("당신은 그것을 잠굴수 없습니다.")
			return StatusDefault, nil
		}
		if !exitHasAnyFlag(exit, "closed", "xclosd", "xclosed") {
			ctx.WriteString("먼저 문을 닫아야 될것 같군요.")
			return StatusDefault, nil
		}
		if shots, ok := objectIntProperty(world, key, "shotsCurrent"); !ok || shots < 1 {
			ctx.WriteString(keyDisplayName + "이 부서져 버렸습니다.")
			return StatusDefault, nil
		}
		if !exitControlKeyMatches(world, exit, key) {
			ctx.WriteString("열쇠가 맞지 않습니다.")
			return StatusDefault, nil
		}
		if err := revealExitControlActor(world, player, creature); err != nil {
			return StatusDefault, err
		}
		if _, _, err := world.LockExitWithKey(room.ID, exit.Name, key.ID); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("## 찰칵 ##")
		_ = roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s이 %s쪽 출구를 잠궜습니다.", commandActorDisplayName(player, creature), exit.Name))
		return StatusDefault, nil
	}
}

func exitControlActor(world ExitControlWorld, viewer LookViewer) (model.Player, model.Creature, bool) {
	if !viewer.PlayerID.IsZero() {
		player, ok := world.Player(viewer.PlayerID)
		if ok {
			creature, ok := world.Creature(player.CreatureID)
			return player, creature, ok
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

func touchExitTimerIfSupported(world any, roomID model.RoomID, exitName string) (model.Exit, bool, error) {
	timerWorld, ok := world.(interface {
		TouchExitTimer(model.RoomID, string, int64) (model.Exit, error)
	})
	if !ok {
		return model.Exit{}, false, nil
	}
	exit, err := timerWorld.TouchExitTimer(roomID, exitName, time.Now().Unix())
	return exit, true, err
}

func revealExitControlActor(world ExitControlWorld, player model.Player, creature model.Creature) error {
	remove := []string{"hidden", "phiddn", "PHIDDN"}
	if !player.ID.IsZero() {
		if _, err := world.UpdatePlayerTags(player.ID, nil, remove); err != nil {
			return err
		}
	}
	if _, err := world.UpdateCreatureTags(creature.ID, nil, remove); err != nil {
		return err
	}
	if creature.Stats != nil && creature.Stats["PHIDDN"] != 0 {
		if err := world.SetCreatureStat(creature.ID, "PHIDDN", 0); err != nil {
			return err
		}
	}
	return nil
}

func findExitControlInventoryObject(
	world InventoryWorld,
	creature model.Creature,
	target string,
	ordinal int64,
	detectInvisible bool,
) (model.ObjectInstance, string, bool) {
	return findEquipInventoryObjectWithVisibility(world, creature, target, ordinal, detectInvisible)
}

func exitControlKeyMatches(world InventoryWorld, exit model.Exit, key model.ObjectInstance) bool {
	exitKey, ok := exitControlKeyNumber(exit)
	if !ok {
		return false
	}
	keyNumber, ok := objectIntProperty(world, key, "nDice")
	return ok && keyNumber == exitKey
}

func exitControlKeyNumber(exit model.Exit) (int, bool) {
	for _, flag := range exit.Flags {
		name, raw, ok := strings.Cut(strings.TrimSpace(flag), ":")
		if !ok || normalizeFlagName(name) != "key" {
			continue
		}
		value, ok := parseObjectInt(raw)
		if ok && value > 0 {
			return value, true
		}
	}
	if raw := exit.Metadata.RawFields["key"]; len(raw) > 0 && raw[0] > 0 {
		return int(raw[0]), true
	}
	return 0, false
}

func exitControlUseOutput(world InventoryWorld, object model.ObjectInstance) string {
	if text := object.Properties["useOutput"]; strings.TrimSpace(text) != "" {
		return text
	}
	if object.PrototypeID.IsZero() {
		return ""
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return ""
	}
	if text := proto.Properties["useOutput"]; strings.TrimSpace(text) != "" {
		return text
	}
	return ""
}

func picklockChance(creature model.Creature, class int) int {
	level := picklockCreatureStat(creature, "level")
	if level == 0 {
		level = creature.Level
	}
	if level < 0 {
		level = 0
	}
	step := (level + 3) / 4
	chance := 5 * step
	if class == picklockThiefClass {
		chance = 10 * step
	}
	chance += picklockCreatureStat(creature, "dexterity") * 2
	if chance < 0 {
		return 0
	}
	if chance > 100 {
		return 100
	}
	return chance
}

func picklockCreatureStat(creature model.Creature, key string) int {
	if creature.Stats == nil {
		return 0
	}
	return creature.Stats[key]
}

func picklockCreatureHasFlag(creature model.Creature, names ...string) bool {
	if creatureHasAnyFlag(creature, names...) {
		return true
	}
	targets := normalizedFlagSet(names...)
	for key, value := range creature.Stats {
		if _, ok := targets[normalizeFlagName(key)]; ok && value != 0 {
			return true
		}
	}
	return false
}

func ensureTrailingNewline(text string) string {
	if text == "" || strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}
