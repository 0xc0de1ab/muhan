package command

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	moveTrapPit   = 1
	moveTrapDart  = 2
	moveTrapBlock = 3
	moveTrapMPDam = 4
	moveTrapRMSpl = 5
	moveTrapNaked = 6
	moveTrapAlarm = 7

	moveTrapLegacyPermMonOffset = 216
	moveTrapLegacyLasttimeSize  = 12
	moveTrapLegacyPermMonSlots  = 10
)

var moveTrapSpellExpirationTags = []string{
	"PPROTE",
	"PBLESS",
	"PRFIRE",
	"PRCOLD",
	"PBRWAT",
	"PSSHLD",
	"PRMAGI",
	"PLIGHT",
	"PDINVI",
	"PINVIS",
	"PKNOWA",
	"PDMAGI",
}

type moveTrapCreatureTagWorld interface {
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
}

type moveTrapPlayerTagWorld interface {
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
}

type moveTrapStatWorld interface {
	SetCreatureStat(model.CreatureID, string, int) error
}

type moveTrapDamageWorld interface {
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
}

type moveTrapPlayerDeathWorld interface {
	PlayerDeath(model.PlayerID, model.CreatureID) error
}

type moveTrapDirectMoveWorld interface {
	MovePlayerToRoom(model.PlayerID, model.RoomID) error
}

type moveTrapCreatureMoveWorld interface {
	MoveCreatureToRoom(model.CreatureID, model.RoomID) error
}

type moveTrapEnemyWorld interface {
	AddEnemy(attacker, defender model.CreatureID) (bool, error)
}

type moveTrapPermanentCreatureTriggerWorld interface {
	ActivatePermanentCreatureForTrap(model.PlayerID, model.CreatureID) error
}

type moveTrapPermanentCreaturePopulateWorld interface {
	AddPermanentCreaturesToRoom(model.RoomID) error
}

type moveTrapDBRootWorld interface {
	DBRoot() string
}

type moveTrapCreatureSpawnWorld interface {
	SpawnCreature(model.CreatureID, model.RoomID, bool) (model.CreatureID, error)
}

type moveTrapCreaturePrototypeWorld interface {
	CreaturePrototype(model.CreatureID) (model.Creature, bool)
}

type moveTrapCreaturePropertyWorld interface {
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

type moveTrapCreatureCooldownWorld interface {
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

type moveTrapEffectExpirationWorld interface {
	SetEffectExpiration(model.CreatureID, string, int64)
}

type moveTrapObjectDestroyWorld interface {
	DestroyObject(model.ObjectInstanceID) error
}

type moveTrapCombatStatWorld interface {
	RecalculateAC(model.CreatureID) error
	RecalculateTHACO(model.CreatureID) error
}

type moveTrapRoomLoader interface {
	LoadRoom(model.RoomID) error
}

func checkMoveRoomTrap(ctx *Context, world MoveWorld, viewer LookViewer, room model.Room) error {
	trapType, hasTrap := moveRoomTrapType(room)
	if !hasTrap || trapType == 0 {
		return moveTrapClearPrepared(world, viewer)
	}
	if !moveTrapKnown(trapType) {
		return nil
	}

	actor, ok := world.Creature(viewer.CreatureID)
	if !ok {
		return nil
	}

	prepared := moveTrapActorHasAnyFlag(world, viewer, actor, "prepared", "prepare", "PPREPA")
	if prepared && moveTrapPreparedAvoided(trapType, actor) {
		return moveTrapClearPrepared(world, viewer)
	}
	if err := moveTrapClearPrepared(world, viewer); err != nil {
		return err
	}
	if moveTrapNaturallyAvoided(trapType, actor) {
		return nil
	}

	switch trapType {
	case moveTrapPit:
		return moveTrapPitEffect(ctx, world, viewer, room, actor)
	case moveTrapDart:
		return moveTrapDartEffect(ctx, world, viewer, room, actor)
	case moveTrapBlock:
		return moveTrapBlockEffect(ctx, world, viewer, room, actor)
	case moveTrapMPDam:
		return moveTrapMPDamageEffect(ctx, world, viewer, room, actor)
	case moveTrapRMSpl:
		return moveTrapRemoveSpellEffect(ctx, world, viewer, room, actor)
	case moveTrapNaked:
		return moveTrapNakedEffect(ctx, world, room, actor)
	case moveTrapAlarm:
		return moveTrapAlarmEffect(ctx, world, viewer, room, actor)
	default:
		return nil
	}
}

func moveRoomTrapType(room model.Room) (int, bool) {
	value, ok := roomPropertyValue(room, "trap")
	if !ok {
		return 0, false
	}
	trapType, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return trapType, true
}

func moveRoomTrapExitRaw(room model.Room) (string, bool) {
	for _, key := range []string{"trapExit", "trapexit", "trap_exit"} {
		if value, ok := roomPropertyValue(room, key); ok {
			return value, true
		}
	}
	return "", false
}

func moveTrapKnown(trapType int) bool {
	switch trapType {
	case moveTrapPit, moveTrapDart, moveTrapBlock, moveTrapMPDam, moveTrapRMSpl, moveTrapNaked, moveTrapAlarm:
		return true
	default:
		return false
	}
}

func moveTrapPreparedAvoided(trapType int, actor model.Creature) bool {
	switch trapType {
	case moveTrapMPDam, moveTrapRMSpl:
		return mrand(1, 25) < creatureStat(actor, "intelligence")
	default:
		return mrand(1, 20) < creatureStat(actor, "dexterity")
	}
}

func moveTrapNaturallyAvoided(trapType int, actor model.Creature) bool {
	switch trapType {
	case moveTrapMPDam, moveTrapRMSpl:
		return mrand(1, 100) < creatureStat(actor, "intelligence")
	default:
		return mrand(1, 100) < creatureStat(actor, "dexterity")
	}
}

func moveTrapClearPrepared(world MoveWorld, viewer LookViewer) error {
	remove := []string{"prepared", "prepare", "PPREPA"}
	if !viewer.CreatureID.IsZero() {
		if updater, ok := world.(moveTrapCreatureTagWorld); ok {
			if _, err := updater.UpdateCreatureTags(viewer.CreatureID, nil, remove); err != nil {
				return err
			}
		}
	}
	if !viewer.PlayerID.IsZero() {
		if updater, ok := world.(moveTrapPlayerTagWorld); ok {
			if _, err := updater.UpdatePlayerTags(viewer.PlayerID, nil, remove); err != nil {
				return err
			}
		}
	}
	return nil
}

func moveTrapPitEffect(ctx *Context, world MoveWorld, viewer LookViewer, room model.Room, actor model.Creature) error {
	if moveTrapActorHasAnyFlag(world, viewer, actor, "levitate", "levitation", "PLEVIT", "SLEVIT") {
		return nil
	}

	ctx.WriteString("당신은 구덩이에 빠졌습니다!\n")
	actorName := attackCreatureName(actor)
	if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 구덩이에 빠졌습니다!\n"); err != nil {
		return err
	}

	if trapExit, ok := moveTrapExitRoomID(world, room); ok && !viewer.PlayerID.IsZero() {
		if mover, ok := world.(moveTrapDirectMoveWorld); ok {
			if err := mover.MovePlayerToRoom(viewer.PlayerID, trapExit); err != nil {
				return err
			}
			if movedViewer, movedRoom, err := CurrentRoom(world, viewer); err == nil {
				ctx.WriteString(RenderRoomLook(world, movedRoom, movedViewer))
			}
		}
	}

	damage := mrand(1, 15)
	ctx.WriteString(fmt.Sprintf("당신은 %d점의 피해를 입었습니다.\n", damage))
	_, _, dead, err := moveTrapApplyDamage(world, actor, damage)
	if err != nil {
		return err
	}
	if dead {
		return moveTrapFinalizePlayerDeath(ctx, world, viewer, actor)
	}
	return nil
}

func moveTrapDartEffect(ctx *Context, world MoveWorld, viewer LookViewer, room model.Room, actor model.Creature) error {
	ctx.WriteString("당신은 숨겨진 독화살에 맞았습니다!\n")
	actorName := attackCreatureName(actor)
	if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 숨겨진 독화살에 맞았습니다.\n"); err != nil {
		return err
	}

	if updater, ok := world.(moveTrapCreatureTagWorld); ok {
		if _, err := updater.UpdateCreatureTags(actor.ID, []string{"poison", "PPOISN"}, nil); err != nil {
			return err
		}
	}
	if !viewer.PlayerID.IsZero() {
		if updater, ok := world.(moveTrapPlayerTagWorld); ok {
			if _, err := updater.UpdatePlayerTags(viewer.PlayerID, []string{"poison", "PPOISN"}, nil); err != nil {
				return err
			}
		}
	}

	damage := mrand(1, 10)
	ctx.WriteString(fmt.Sprintf("당신은 %d점의 피해를 입었습니다.\n", damage))
	_, _, dead, err := moveTrapApplyDamage(world, actor, damage)
	if err != nil {
		return err
	}
	if dead {
		return moveTrapFinalizePlayerDeath(ctx, world, viewer, actor)
	}
	return nil
}

func moveTrapBlockEffect(ctx *Context, world MoveWorld, viewer LookViewer, room model.Room, actor model.Creature) error {
	ctx.WriteString("당신은 커다란 돌에 맞았습니다!\n")
	actorName := attackCreatureName(actor)
	if err := roomBroadcast(ctx, room.ID, "\n"+actorName+" 위로 커다란 돌이 떨어졌습니다.\n"); err != nil {
		return err
	}

	damage := creatureStat(actor, "hpMax") / 2
	ctx.WriteString(fmt.Sprintf("당신은 %d점의 피해를 입었습니다.\n", damage))
	_, _, dead, err := moveTrapApplyDamage(world, actor, damage)
	if err != nil {
		return err
	}
	if dead {
		return moveTrapFinalizePlayerDeath(ctx, world, viewer, actor)
	}
	return nil
}

func moveTrapMPDamageEffect(ctx *Context, world MoveWorld, viewer LookViewer, room model.Room, actor model.Creature) error {
	ctx.WriteString("당신의 마음이 충격을 받았습니다!\n")
	actorName := attackCreatureName(actor)
	if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 강한 충격을 받았습니다.\n"); err != nil {
		return err
	}

	mpCurrent := creatureStat(actor, "mpCurrent")
	mpLoss := minInt(mpCurrent, creatureStat(actor, "mpMax")/2)
	ctx.WriteString(fmt.Sprintf("당신은 %d점의 마력을 잃었습니다.\n", mpLoss))
	if setter, ok := world.(moveTrapStatWorld); ok {
		if err := setter.SetCreatureStat(actor.ID, "mpCurrent", maxInt(0, mpCurrent-mpLoss)); err != nil {
			return err
		}
	}

	damage := mrand(1, 6)
	ctx.WriteString(fmt.Sprintf("당신은 %d점의 피해를 입었습니다.\n", damage))
	_, _, dead, err := moveTrapApplyDamage(world, actor, damage)
	if err != nil {
		return err
	}
	if dead {
		return moveTrapFinalizePlayerDeath(ctx, world, viewer, actor)
	}
	return nil
}

func moveTrapRemoveSpellEffect(ctx *Context, world MoveWorld, viewer LookViewer, room model.Room, actor model.Creature) error {
	ctx.WriteString("어두운 기운이 당신을 감쌉니다.\n")
	ctx.WriteString("당신의 주문이 사라집니다.\n")
	actorName := attackCreatureName(actor)
	if err := roomBroadcast(ctx, room.ID, "\n어두운 기운이 "+actorName+krtext.Particle(actorName, '3')+" 감쌉니다.\n"); err != nil {
		return err
	}

	if expirer, ok := world.(moveTrapEffectExpirationWorld); ok {
		for _, tag := range moveTrapSpellExpirationTags {
			expirer.SetEffectExpiration(actor.ID, tag, 0)
		}
	}
	return nil
}

func moveTrapNakedEffect(ctx *Context, world MoveWorld, room model.Room, actor model.Creature) error {
	ctx.WriteString("붉은 액체가 당신위로 쏟아집니다.\n")
	ctx.WriteString("으악!!! 당신의 장비가 녹아버립니다.\n")
	actorName := attackCreatureName(actor)
	if err := roomBroadcast(ctx, room.ID, "\n붉은 액체가 "+actorName+"님위로 쏟아집니다.\n"); err != nil {
		return err
	}

	destroyer, ok := world.(moveTrapObjectDestroyWorld)
	if !ok {
		return nil
	}
	for _, objectID := range actor.Inventory.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		if err := destroyer.DestroyObject(objectID); err != nil {
			return err
		}
	}
	for _, objectID := range actor.Equipment {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || moveTrapObjectCursed(world, object) {
			continue
		}
		if err := destroyer.DestroyObject(objectID); err != nil {
			return err
		}
	}
	if recalc, ok := world.(moveTrapCombatStatWorld); ok {
		if err := recalc.RecalculateAC(actor.ID); err != nil {
			return err
		}
		if err := recalc.RecalculateTHACO(actor.ID); err != nil {
			return err
		}
	}
	return nil
}

func moveTrapAlarmEffect(ctx *Context, world MoveWorld, viewer LookViewer, room model.Room, actor model.Creature) error {
	ctx.WriteString("경보장치가 울립니다!\n")
	ctx.WriteString("근처에 경비원들이 없길 바랍니다.\n")
	actorName := attackCreatureName(actor)
	if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 경보장치를 건드렸습니다!\n"); err != nil {
		return err
	}

	trapExit, ok := moveTrapExitRoomID(world, room)
	if !ok {
		return nil
	}
	if populator, ok := world.(moveTrapPermanentCreaturePopulateWorld); ok {
		if err := populator.AddPermanentCreaturesToRoom(trapExit); err != nil {
			return err
		}
	}
	if err := moveTrapLoadLegacyPermanentCreatures(world, trapExit, time.Now().Unix()); err != nil {
		return err
	}
	guardRoom, ok := world.Room(trapExit)
	if !ok {
		return nil
	}
	mover, canMove := world.(moveTrapCreatureMoveWorld)
	if !canMove {
		return nil
	}

	for _, creatureID := range guardRoom.CreatureIDs {
		guard, ok := world.Creature(creatureID)
		if !ok || !creatureHasAnyFlag(guard, "MPERMT", "permanent") {
			continue
		}
		guardName := attackCreatureName(guard)
		_ = roomBroadcast(ctx, guardRoom.ID, "\n"+guardName+krtext.Particle(guardName, '1')+" 경보를 듣고 조사하러 갑니다.\n")
		if updater, ok := world.(moveTrapCreatureTagWorld); ok {
			if _, err := updater.UpdateCreatureTags(guard.ID, []string{"MAGGRE", "aggressive", "was_attacked"}, []string{"MPERMT", "permanent"}); err != nil {
				return err
			}
		}
		if cooldowns, ok := world.(moveTrapCreatureCooldownWorld); ok {
			if err := cooldowns.SetCreatureCooldown(guard.ID, "attack", 0, 1); err != nil {
				return err
			}
		}
		if trigger, ok := world.(moveTrapPermanentCreatureTriggerWorld); ok {
			if err := trigger.ActivatePermanentCreatureForTrap(viewer.PlayerID, guard.ID); err != nil {
				return err
			}
		}
		if enemyWorld, ok := world.(moveTrapEnemyWorld); ok {
			if _, err := enemyWorld.AddEnemy(guard.ID, actor.ID); err != nil {
				return err
			}
		}
		if err := mover.MoveCreatureToRoom(guard.ID, room.ID); err != nil {
			return err
		}
		_ = roomBroadcast(ctx, room.ID, guardName+krtext.Particle(guardName, '1')+" 경보를 듣고 조사하러 왔습니다.\n")
	}
	return nil
}

func moveTrapLoadLegacyPermanentCreatures(world MoveWorld, roomID model.RoomID, nowUnix int64) error {
	if world == nil || roomID.IsZero() {
		return nil
	}
	dbroot, ok := world.(moveTrapDBRootWorld)
	if !ok {
		return nil
	}
	spawner, ok := world.(moveTrapCreatureSpawnWorld)
	if !ok {
		return nil
	}
	root := strings.TrimSpace(dbroot.DBRoot())
	if root == "" {
		return nil
	}
	path, ok := moveTrapLegacyRoomPath(root, roomID)
	if !ok {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	due := moveTrapLegacyPermanentCreatureCounts(data, nowUnix)
	for protoID, wantCount := range due {
		existing := moveTrapPermanentCreatureCount(world, roomID, protoID)
		for i := existing; i < wantCount; i++ {
			spawnedID, err := spawner.SpawnCreature(protoID, roomID, true)
			if err != nil {
				continue
			}
			if updater, ok := world.(moveTrapCreatureTagWorld); ok {
				if _, err := updater.UpdateCreatureTags(spawnedID, []string{"MPERMT", "permanent"}, nil); err != nil {
					return err
				}
			}
			if setter, ok := world.(moveTrapCreaturePropertyWorld); ok {
				if _, err := setter.SetCreatureProperty(spawnedID, "legacyPermanentPrototypeID", string(protoID)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func moveTrapLegacyRoomPath(root string, roomID model.RoomID) (string, bool) {
	raw := strings.TrimPrefix(strings.TrimSpace(string(roomID)), "room:")
	number, err := strconv.Atoi(raw)
	if err != nil || number < 0 {
		return "", false
	}
	return filepath.Join(root, "rooms", fmt.Sprintf("r%02d", number/1000), fmt.Sprintf("r%05d", number)), true
}

func moveTrapLegacyPermanentCreatureCounts(data []byte, nowUnix int64) map[model.CreatureID]int {
	counts := map[model.CreatureID]int{}
	for i := 0; i < moveTrapLegacyPermMonSlots; i++ {
		offset := moveTrapLegacyPermMonOffset + i*moveTrapLegacyLasttimeSize
		if offset+moveTrapLegacyLasttimeSize > len(data) {
			break
		}
		interval := int64(int32(binary.LittleEndian.Uint32(data[offset : offset+4])))
		last := int64(int32(binary.LittleEndian.Uint32(data[offset+4 : offset+8])))
		misc := int(int16(binary.LittleEndian.Uint16(data[offset+8 : offset+10])))
		if misc <= 0 {
			continue
		}
		if last+interval > nowUnix {
			continue
		}
		counts[moveTrapLegacyCreaturePrototypeID(misc)]++
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}

func moveTrapLegacyCreaturePrototypeID(number int) model.CreatureID {
	return model.CreatureID(fmt.Sprintf("creature:m%02d:%d", number/100, number%100))
}

func moveTrapPermanentCreatureCount(world MoveWorld, roomID model.RoomID, protoID model.CreatureID) int {
	room, ok := world.Room(roomID)
	if !ok {
		return 0
	}

	var protoName string
	if prototypes, ok := world.(moveTrapCreaturePrototypeWorld); ok {
		if proto, ok := prototypes.CreaturePrototype(protoID); ok {
			protoName = strings.TrimSpace(proto.DisplayName)
		}
	}

	count := 0
	for _, creatureID := range room.CreatureIDs {
		creature, ok := world.Creature(creatureID)
		if !ok || !creatureHasAnyFlag(creature, "MPERMT", "permanent") {
			continue
		}
		if creature.Properties != nil && creature.Properties["legacyPermanentPrototypeID"] == string(protoID) {
			count++
			continue
		}
		if protoName != "" && strings.TrimSpace(creature.DisplayName) == protoName {
			count++
		}
	}
	return count
}

func moveTrapApplyDamage(world MoveWorld, actor model.Creature, damage int) (model.Creature, int, bool, error) {
	if damage <= 0 {
		return actor, 0, false, nil
	}
	if damageWorld, ok := world.(moveTrapDamageWorld); ok {
		return damageWorld.ApplyCreatureDamage(actor.ID, damage)
	}
	if setter, ok := world.(moveTrapStatWorld); ok {
		current := creatureStat(actor, "hpCurrent")
		applied := minInt(current, damage)
		next := maxInt(0, current-damage)
		if err := setter.SetCreatureStat(actor.ID, "hpCurrent", next); err != nil {
			return actor, 0, false, err
		}
		updated := actor
		if updated.Stats == nil {
			updated.Stats = map[string]int{}
		}
		updated.Stats["hpCurrent"] = next
		return updated, applied, next < 1, nil
	}
	return actor, 0, false, nil
}

func moveTrapFinalizePlayerDeath(ctx *Context, world MoveWorld, viewer LookViewer, actor model.Creature) error {
	if viewer.PlayerID.IsZero() {
		return nil
	}
	finalizer, ok := world.(moveTrapPlayerDeathWorld)
	if !ok {
		return nil
	}
	if err := finalizer.PlayerDeath(viewer.PlayerID, actor.ID); err != nil {
		return err
	}
	ctx.WriteString("당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다.\n")
	return nil
}

func moveTrapActorHasAnyFlag(world MoveWorld, viewer LookViewer, actor model.Creature, names ...string) bool {
	if creatureHasAnyFlag(actor, names...) {
		return true
	}
	if viewer.PlayerID.IsZero() {
		return false
	}
	player, ok := world.Player(viewer.PlayerID)
	return ok && hasAnyNormalizedFlag(player.Metadata.Tags, names...)
}

func moveTrapObjectCursed(world MoveWorld, object model.ObjectInstance) bool {
	return objectHasAnyFlagOrProperty(world, object, "cursed", "OCURSE")
}

func moveTrapExitRoomID(world MoveWorld, room model.Room) (model.RoomID, bool) {
	raw, ok := moveRoomTrapExitRaw(room)
	if !ok {
		raw = "0"
	}
	for _, roomID := range moveTrapRoomIDCandidates(raw) {
		if _, ok := world.Room(roomID); ok {
			return roomID, true
		}
		if loader, ok := world.(moveTrapRoomLoader); ok {
			if err := loader.LoadRoom(roomID); err == nil {
				if _, ok := world.Room(roomID); ok {
					return roomID, true
				}
			}
		}
	}
	return "", false
}

func moveTrapRoomIDCandidates(raw string) []model.RoomID {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	added := map[model.RoomID]struct{}{}
	var ids []model.RoomID
	add := func(id model.RoomID) {
		if id.IsZero() {
			return
		}
		if _, ok := added[id]; ok {
			return
		}
		added[id] = struct{}{}
		ids = append(ids, id)
	}

	explicit := model.RoomID("room:" + raw)
	if strings.HasPrefix(raw, "room:") {
		explicit = model.RoomID(raw)
		raw = strings.TrimPrefix(raw, "room:")
	} else if strings.HasPrefix(strings.ToLower(raw), "r") {
		raw = strings.TrimPrefix(strings.TrimPrefix(raw, "r"), "R")
		explicit = model.RoomID("room:" + raw)
	}

	if number, err := strconv.Atoi(raw); err == nil {
		add(model.RoomID(fmt.Sprintf("room:%05d", number)))
		add(model.RoomID(fmt.Sprintf("room:%04d", number)))
		add(model.RoomID(fmt.Sprintf("room:%d", number)))
	}
	add(explicit)
	return ids
}
