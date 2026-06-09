package command

import (
	"fmt"
	"strconv"
	"strings"

	"muhan/internal/world/model"
)

type DMStatWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
}

const dmStatLegacyRMAX = 9000

func NewDMStatHandler(world DMStatWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmStat(ctx, resolved, world)
	}
}

func dmStat(ctx *Context, resolved ResolvedCommand, world DMStatWorld) (Status, error) {
	playerID := InventoryPlayerIDFromContext(ctx)
	if playerID.IsZero() {
		return StatusDefault, ErrInventoryActorRequired
	}
	_, creature, err := CurrentInventoryCreature(world, playerID)
	if err != nil {
		return StatusDefault, err
	}

	classVal := creatureClass(creature)
	if classVal != legacyClassZoneMaker && classVal < legacyClassSubDM {
		return StatusPrompt, nil
	}

	if targetRoomID, ok, silent := dmStatRoomTarget(creature, resolved); ok {
		if silent {
			return StatusDefault, nil
		}
		room, ok := world.Room(targetRoomID)
		if !ok {
			cleanID := strings.TrimPrefix(string(targetRoomID), "room:")
			ctx.WriteString(fmt.Sprintf("에러 (%s)\n", cleanID))
			return StatusDefault, nil
		}
		ctx.WriteString(renderStatRom(room))
		return StatusDefault, nil
	}

	// 2. Target name is specified
	targetName := dmStatArg(resolved, 0)
	targetOrdinal := int(dmStatOrdinal(resolved, 0))

	// Determine secondary target (the actor whose inventory/equipment is searched). Defaults to DM.
	var ply_ptr2 model.Creature
	ply_ptr2_ok := false

	if dmStatArgCount(resolved) < 2 {
		ply_ptr2 = creature
		ply_ptr2_ok = true
	} else {
		targetActorName := dmStatArg(resolved, 1)
		actorOrdinal := int(dmStatOrdinal(resolved, 1))

		if c, found := dmStatFindRoomCreature(world, creature, creature.RoomID, targetActorName, int64(actorOrdinal)); found {
			ply_ptr2 = c
			ply_ptr2_ok = true
		}

		if !ply_ptr2_ok {
			if _, c, found := dmStatFindOnlinePlayer(ctx, world, legacyLowercizeASCII(targetActorName, true)); found {
				ply_ptr2 = c
				ply_ptr2_ok = true
			}
		}

		if !ply_ptr2_ok {
			ply_ptr2 = creature
			ply_ptr2_ok = true
		}

		if ply_ptr2_ok {
			if checkFlag(ply_ptr2, "PDMINV") && classVal < legacyClassDM {
				ply_ptr2 = creature
			}
		}
	}

	// Search for object in player inventory, ready worn slots, room floor
	if obj, found := dmStatFindObject(world, ply_ptr2, targetName, int64(targetOrdinal)); found {
		ctx.WriteString(renderStatObj(world, obj))
		return StatusDefault, nil
	}

	// Search for creature or player
	var targetCrt model.Creature
	targetCrtFound := false

	if c, found := dmStatFindRoomCreature(world, creature, ply_ptr2.RoomID, targetName, int64(targetOrdinal)); found {
		targetCrt = c
		targetCrtFound = true
	}

	if !targetCrtFound {
		if _, c, found := dmStatFindOnlinePlayer(ctx, world, legacyLowercizeASCII(targetName, true)); found {
			targetCrt = c
			targetCrtFound = true
		}
	}

	if targetCrtFound {
		if checkFlag(targetCrt, "PDMINV") && classVal < legacyClassSubDM {
			ctx.WriteString("그런건 없습니다.\n")
			return StatusDefault, nil
		}

		var targetPlayer model.Player
		if !targetCrt.PlayerID.IsZero() {
			if p, ok := world.Player(targetCrt.PlayerID); ok {
				targetPlayer = p
			}
		}

		ctx.WriteString(renderStatCrt(ctx, targetPlayer, targetCrt))
		return StatusDefault, nil
	}

	ctx.WriteString("그런건 없습니다.\n")
	return StatusDefault, nil
}

func dmStatRoomTarget(creature model.Creature, resolved ResolvedCommand) (model.RoomID, bool, bool) {
	if resolved.Parsed.Num > 0 {
		if resolved.Parsed.Num >= 2 {
			return "", false, false
		}
		roomNum := resolved.Parsed.Val[0]
		if roomNum >= dmStatLegacyRMAX {
			return "", true, true
		}
		if roomNum == 1 {
			return creature.RoomID, true, false
		}
		return model.RoomID("room:" + strconv.FormatInt(roomNum, 10)), true, false
	}

	if len(resolved.Args) == 0 {
		return creature.RoomID, true, false
	}
	if len(resolved.Args) == 1 {
		arg := strings.TrimSpace(resolved.Args[0])
		if strings.HasPrefix(arg, "room:") {
			return model.RoomID(arg), true, false
		}
		if _, err := strconv.Atoi(arg); err == nil {
			return model.RoomID("room:" + arg), true, false
		}
	}

	return "", false, false
}

func dmStatArgCount(resolved ResolvedCommand) int {
	if resolved.Parsed.Num > 0 {
		return resolved.Parsed.Num - 1
	}
	return len(resolved.Args)
}

func dmStatArg(resolved ResolvedCommand, index int) string {
	slot := index + 1
	if resolved.Parsed.Num > slot {
		if arg := strings.TrimSpace(resolved.Parsed.Str[slot]); arg != "" {
			return arg
		}
	}
	return getArg(resolved, index)
}

func dmStatOrdinal(resolved ResolvedCommand, index int) int64 {
	slot := index + 1
	if resolved.Parsed.Num > slot && resolved.Parsed.Val[slot] > 0 {
		return resolved.Parsed.Val[slot]
	}
	return getOrdinal(resolved, index)
}

func dmStatFindObject(world DMStatWorld, viewer model.Creature, name string, ordinal int64) (model.ObjectInstance, bool) {
	var player model.Player
	if !viewer.PlayerID.IsZero() {
		player, _ = world.Player(viewer.PlayerID)
	}
	detectInvisible := inventoryViewerDetectsInvisible(player, viewer)

	if obj, found := dmFindObjInCreatureInventory(world, viewer, name, ordinal, detectInvisible); found {
		return obj, true
	}
	if obj, found := dmFindReadyObjOnCreature(world, viewer, name, ordinal); found {
		return obj, true
	}
	if room, ok := world.Room(viewer.RoomID); ok {
		if obj, found := dmFindObjInRoom(world, room, name, ordinal, detectInvisible); found {
			return obj, true
		}
	}
	return model.ObjectInstance{}, false
}

func dmStatFindRoomCreature(world DMStatWorld, actor model.Creature, roomID model.RoomID, name string, ordinal int64) (model.Creature, bool) {
	room, ok := world.Room(roomID)
	if !ok {
		return model.Creature{}, false
	}
	if creature, found := dmFindMonsterInRoomList(world, actor, room, name, ordinal); found {
		return creature, true
	}
	if creature, found := dmStatFindPlayerInRoom(world, actor, room, name, ordinal); found {
		return creature, true
	}
	return model.Creature{}, false
}

func dmStatFindPlayerInRoom(world DMStatWorld, actor model.Creature, room model.Room, name string, ordinal int64) (model.Creature, bool) {
	nameLower := strings.ToLower(strings.TrimSpace(name))
	if len(nameLower) < 2 {
		return model.Creature{}, false
	}
	count := int(ordinal)
	if count < 1 {
		count = 1
	}

	detectInvisible := creatureHasAnyFlag(actor, "PDINVI", "detectInvisible", "detectInvis")
	seen := 0
	for _, playerID := range room.PlayerIDs {
		player, ok := world.Player(playerID)
		if !ok || player.CreatureID.IsZero() {
			continue
		}
		creature, ok := world.Creature(player.CreatureID)
		if !ok || !dmCreatureIsPlayer(creature) {
			continue
		}
		if !legacyFindCrtVisible(creature, detectInvisible) || !dmStatPlayerInRoomMatches(player, creature, nameLower) {
			continue
		}
		seen++
		if seen == count {
			return creature, true
		}
	}
	return model.Creature{}, false
}

func dmStatPlayerInRoomMatches(player model.Player, creature model.Creature, nameLower string) bool {
	terms := []string{
		creature.DisplayName,
		player.DisplayName,
	}
	for _, key := range []string{"name", "key[0]", "key[1]", "key[2]", "key/1", "key/2", "key/3"} {
		if val := strings.TrimSpace(creature.Properties[key]); val != "" {
			terms = append(terms, val)
		}
	}
	for _, term := range terms {
		termLower := strings.ToLower(strings.TrimSpace(term))
		if termLower == "" {
			continue
		}
		if strings.HasPrefix(termLower, nameLower) {
			return true
		}
		for _, word := range strings.Fields(termLower) {
			if strings.HasPrefix(word, nameLower) {
				return true
			}
		}
	}
	return false
}

func dmStatFindOnlinePlayer(ctx *Context, world DMStatWorld, name string) (model.Player, model.Creature, bool) {
	player, creature, _, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return player, creature, ok
}

func renderStatRom(room model.Room) string {
	var b strings.Builder

	roomNum := 0
	cleanID := strings.TrimPrefix(string(room.ID), "room:")
	if val, err := strconv.Atoi(cleanID); err == nil {
		roomNum = val
	}
	fmt.Fprintf(&b, "방번호 #: %d\n", roomNum)
	fmt.Fprintf(&b, "이름: %s\n", room.DisplayName)

	special := 0
	if valStr, ok := roomPropertyValue(room, "special"); ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			special = val
		}
	}
	fmt.Fprintf(&b, "Special: %d\n", special)

	traffic := 0
	if valStr, ok := room.Properties["traffic"]; ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			traffic = val
		}
	}
	fmt.Fprintf(&b, "Traffic: %d%%\n", traffic)

	b.WriteString("Random monsters:")
	for i := 0; i < 10; i++ {
		randMonster := 0
		if valStr, ok := room.Properties[fmt.Sprintf("random[%d]", i)]; ok {
			if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
				randMonster = val
			}
		} else if valStr, ok := room.Properties[fmt.Sprintf("random%d", i)]; ok {
			if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
				randMonster = val
			}
		}
		fmt.Fprintf(&b, " %3d", randMonster)
	}
	b.WriteString("\n")

	lolevel := 0
	hilevel := 0
	if valStr, ok := room.Properties["lolevel"]; ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			lolevel = val
		}
	}
	if valStr, ok := room.Properties["hilevel"]; ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			hilevel = val
		}
	}
	if lolevel != 0 || hilevel != 0 {
		b.WriteString("Level Boundary: ")
		if lolevel != 0 {
			fmt.Fprintf(&b, "%d+ level  ", lolevel)
		}
		if hilevel != 0 {
			fmt.Fprintf(&b, "%d- level  ", hilevel)
		}
		b.WriteString("\n")
	}

	trap, trapExit := getRoomTrapInfo(room)
	if trap != 0 {
		switch trap {
		case 1:
			fmt.Fprintf(&b, "Trap type: 구덩이 함정 (연결된 방%d)\n", trapExit)
		case 2:
			b.WriteString("Trap type: 독화살 함정\n")
		case 3:
			b.WriteString("Trap type: 바위 함정\n")
		case 4:
			b.WriteString("Trap type: 도력 함정\n")
		case 5:
			b.WriteString("Trap type: 주술 함정\n")
		case 6:
			b.WriteString("Trap type: 장비 함정\n")
		case 7:
			fmt.Fprintf(&b, "Trap type:  경보 함정( 보초 방 %d)\n", trapExit)
		default:
			b.WriteString("Trap type: 잘못된 함정 #\n")
		}
	}

	var flags []string
	if checkRoomFlag(room, "RSHOPP") {
		flags = append(flags, "Shoppe")
	}
	if checkRoomFlag(room, "RDUMPR") {
		flags = append(flags, "DumpRoom")
	}
	if checkRoomFlag(room, "RPAWNS") {
		flags = append(flags, "PawnShop")
	}
	if checkRoomFlag(room, "RTRAIN") {
		flags = append(flags, "train")
	}
	if checkRoomFlag(room, "RREPAI") {
		flags = append(flags, "Repair")
	}
	if checkRoomFlag(room, "RDARKR") {
		flags = append(flags, "DarkAlways")
	}
	if checkRoomFlag(room, "RDARKN") {
		flags = append(flags, "DarkNight")
	}
	if checkRoomFlag(room, "RPOSTO") {
		flags = append(flags, "PostOffice")
	}
	if checkRoomFlag(room, "RNOKIL") {
		flags = append(flags, "NoPlyKill")
	}
	if checkRoomFlag(room, "RNOTEL") {
		flags = append(flags, "NoTeleport")
	}
	if checkRoomFlag(room, "RHEALR") {
		flags = append(flags, "HealFast")
	}
	if checkRoomFlag(room, "RONEPL") {
		flags = append(flags, "OnePlayer")
	}
	if checkRoomFlag(room, "RTWOPL") {
		flags = append(flags, "TwoPlayer")
	}
	if checkRoomFlag(room, "RTHREE") {
		flags = append(flags, "ThreePlyr")
	}
	if checkRoomFlag(room, "RNOMAG") {
		flags = append(flags, "NoMagic")
	}
	if checkRoomFlag(room, "RPTRAK") {
		flags = append(flags, "PermTrack")
	}
	if checkRoomFlag(room, "REARTH") {
		flags = append(flags, "Earth")
	}
	if checkRoomFlag(room, "RWINDR") {
		flags = append(flags, "Wind")
	}
	if checkRoomFlag(room, "RFIRER") {
		flags = append(flags, "Fire")
	}
	if checkRoomFlag(room, "RWATER") {
		flags = append(flags, "Water")
	}
	if checkRoomFlag(room, "RPLWAN") {
		flags = append(flags, "Groupwander")
	}
	if checkRoomFlag(room, "RPHARM") {
		flags = append(flags, "PHarm")
	}
	if checkRoomFlag(room, "RPPOIS") {
		flags = append(flags, "P-Poision")
	}
	if checkRoomFlag(room, "RPMPDR") {
		flags = append(flags, "P-mp Drain")
	}
	if checkRoomFlag(room, "RPBEFU") {
		flags = append(flags, "Confusion")
	}
	if checkRoomFlag(room, "RNOLEA") {
		flags = append(flags, "No Summon")
	}
	if checkRoomFlag(room, "RPLDGK") {
		flags = append(flags, "Pledge")
	}
	if checkRoomFlag(room, "RRSCND") {
		flags = append(flags, "Rescind")
	}
	if checkRoomFlag(room, "RNOPOT") {
		flags = append(flags, "No Potion")
	}
	if checkRoomFlag(room, "RPMEXT") {
		flags = append(flags, "Pmagic")
	}
	if checkRoomFlag(room, "RNOLOG") {
		flags = append(flags, "NoLog")
	}
	if checkRoomFlag(room, "RELECT") {
		flags = append(flags, "Elect")
	}
	if checkRoomFlag(room, "RSUVIV") {
		flags = append(flags, "Survival")
	}
	if checkRoomFlag(room, "RFAMIL") {
		flags = append(flags, "Family")
	}
	if checkRoomFlag(room, "RONFML") {
		flags = append(flags, "Only family")
	}
	if checkRoomFlag(room, "RBANK") {
		flags = append(flags, "Bank")
	}
	if checkRoomFlag(room, "RONMAR") {
		flags = append(flags, "Only Married")
	}
	if checkRoomFlag(room, "RCAST") {
		flags = append(flags, "Cast")
	}
	if checkRoomFlag(room, "RDEPOT") {
		flags = append(flags, "Depot")
	}

	flagStr := "Flags set: "
	if len(flags) > 0 {
		flagStr += strings.Join(flags, ", ") + "."
	} else {
		flagStr += "None."
	}
	fmt.Fprintf(&b, "%s\n", flagStr)

	b.WriteString("Exits:\n")
	for _, ext := range room.Exits {
		toRoomNum := 0
		cleanToID := strings.TrimPrefix(string(ext.ToRoomID), "room:")
		if val, err := strconv.Atoi(cleanToID); err == nil {
			toRoomNum = val
		}
		fmt.Fprintf(&b, "  %s: %d", ext.Name, toRoomNum)

		var extFlags []string
		if checkExitFlag(ext, "XSECRT") {
			extFlags = append(extFlags, "Secret")
		}
		if checkExitFlag(ext, "XINVIS") {
			extFlags = append(extFlags, "Invisible")
		}
		if checkExitFlag(ext, "XLOCKD") {
			extFlags = append(extFlags, "Locked")
		}
		if checkExitFlag(ext, "XCLOSD") {
			extFlags = append(extFlags, "Closed")
		}
		if checkExitFlag(ext, "XLOCKS") {
			extFlags = append(extFlags, "Lockable")
		}
		if checkExitFlag(ext, "XCLOSS") {
			extFlags = append(extFlags, "Closable")
		}
		if checkExitFlag(ext, "XUNPCK") {
			extFlags = append(extFlags, "Un-pick")
		}
		if checkExitFlag(ext, "XNAKED") {
			extFlags = append(extFlags, "Naked")
		}
		if checkExitFlag(ext, "XCLIMB") {
			extFlags = append(extFlags, "ClimbUp")
		}
		if checkExitFlag(ext, "XREPEL") {
			extFlags = append(extFlags, "ClimbRepel")
		}
		if checkExitFlag(ext, "XDCLIM") {
			extFlags = append(extFlags, "HardClimb")
		}
		if checkExitFlag(ext, "XFLYSP") {
			extFlags = append(extFlags, "Fly")
		}
		if checkExitFlag(ext, "XFEMAL") {
			extFlags = append(extFlags, "Female")
		}
		if checkExitFlag(ext, "XMALES") {
			extFlags = append(extFlags, "Male")
		}
		if checkExitFlag(ext, "XNGHTO") {
			extFlags = append(extFlags, "Night")
		}
		if checkExitFlag(ext, "XDAYON") {
			extFlags = append(extFlags, "Day")
		}
		if checkExitFlag(ext, "XNOSEE") {
			extFlags = append(extFlags, "No-See")
		}
		if checkExitFlag(ext, "XPGUAR") {
			extFlags = append(extFlags, "P-Guard")
		}
		if checkExitFlag(ext, "XPLDGK") {
			if checkExitFlag(ext, "XKNGDM") {
				extFlags = append(extFlags, "Organization 1")
			} else {
				extFlags = append(extFlags, "Organization 0")
			}
		}

		if len(extFlags) > 0 {
			fmt.Fprintf(&b, ", Flags: %s\n", strings.Join(extFlags, ", ")+".")
		} else {
			b.WriteString(".\n")
		}
	}

	return b.String()
}

func renderStatCrt(ctx *Context, player model.Player, c model.Creature) string {
	var b strings.Builder

	isPlayer := c.Kind == model.CreatureKindPlayer || !c.PlayerID.IsZero()
	if isPlayer {
		titleStr := renderCreatureTitle(ctx, c)
		userID := getUserID(player, c)
		addr := getAddress(player, c)
		fmt.Fprintf(&b, "\n%s %s:\n", c.DisplayName, titleStr)
		fmt.Fprintf(&b, "주소: %s@%s\n\n", userID, addr)
	} else {
		fmt.Fprintf(&b, "이름: %s\n", c.DisplayName)
		fmt.Fprintf(&b, "설명: %s\n", c.Description)
		talk := c.Properties["talk"]
		if talk == "" {
			talk = c.Properties["talks"]
		}
		fmt.Fprintf(&b, "이야기: %s\n", talk)
		k0, k1, k2 := getCreatureKeys(c)
		fmt.Fprintf(&b, "단어: %s %+20s%+20s\n\n", k0, k1, k2)
	}

	classVal := creatureClass(c)
	alignVal := creatureAlignment(c)
	isChaos := creatureIsChaos(c)
	alignStr := "선"
	if isChaos {
		alignStr = "악"
	}
	raceName := creatureRaceName(c)
	className := creatureClassName(classVal)

	fmt.Fprintf(&b, "레벨: %-20d       종족: %s\n", c.Level, raceName)
	fmt.Fprintf(&b, "직업: %-20s       성향: %s %d\n\n", className, alignStr, alignVal)

	gold := dmCreatureStat(c, "gold")
	exp := dmCreatureStat(c, "experience")
	fmt.Fprintf(&b, "경험: %d  돈: %d\n", exp, gold)

	hpcur := dmCreatureStat(c, "hpCurrent")
	if hpcur == 0 {
		hpcur = dmCreatureStat(c, "hpcur")
	}
	hpmax := dmCreatureStat(c, "hpMax")
	if hpmax == 0 {
		hpmax = dmCreatureStat(c, "hpmax")
	}
	mpcur := dmCreatureStat(c, "mpCurrent")
	if mpcur == 0 {
		mpcur = dmCreatureStat(c, "mpcur")
	}
	mpmax := dmCreatureStat(c, "mpMax")
	if mpmax == 0 {
		mpmax = dmCreatureStat(c, "mpmax")
	}
	fmt.Fprintf(&b, "체력: %d/%d   도력: %d/%d\n", hpcur, hpmax, mpcur, mpmax)

	armor := dmCreatureStat(c, "armor")
	thaco := dmCreatureStat(c, "thaco")
	fmt.Fprintf(&b, "방어력: %d   용기: %d\n", 100-armor, 20-thaco)

	ndice, sdice, pdice := getDiceStats(c)
	fmt.Fprintf(&b, "타격: %dd%d+%d\n", ndice, sdice, pdice)

	strVal := dmCreatureStat(c, "strength")
	if strVal == 0 {
		strVal = dmCreatureStat(c, "str")
	}
	dexVal := dmCreatureStat(c, "dexterity")
	if dexVal == 0 {
		dexVal = dmCreatureStat(c, "dex")
	}
	conVal := dmCreatureStat(c, "constitution")
	if conVal == 0 {
		conVal = dmCreatureStat(c, "con")
	}
	intVal := dmCreatureStat(c, "intelligence")
	if intVal == 0 {
		intVal = dmCreatureStat(c, "int")
	}
	pieVal := dmCreatureStat(c, "piety")
	if pieVal == 0 {
		pieVal = dmCreatureStat(c, "pie")
	}

	fmt.Fprintf(&b, "힘[%2d]  민첩[%2d]  맷집[%2d]  지능[%2d]  신앙심[%2d]\n",
		strVal, dexVal, conVal, intVal, pieVal)

	var flags []string
	if isPlayer {
		profSharp := creatureProficiency(c, 0)
		profThrust := creatureProficiency(c, 1)
		profBlunt := creatureProficiency(c, 2)
		profPole := creatureProficiency(c, 3)
		profMissile := creatureProficiency(c, 4)
		fmt.Fprintf(&b, "도: %d  검: %d  봉: %d   창: %d  궁: %d\n",
			profSharp, profThrust, profBlunt, profPole, profMissile)

		realmEarth := creatureRealm(c, 0)
		realmWind := creatureRealm(c, 1)
		realmFire := creatureRealm(c, 2)
		realmWater := creatureRealm(c, 3)
		fmt.Fprintf(&b, "땅: %d    바람: %d   불: %d  물: %d\n",
			realmEarth, realmWind, realmFire, realmWater)

		if checkFlag(c, "PBLESS") {
			flags = append(flags, "Bless")
		}
		if checkFlag(c, "PHIDDN") {
			flags = append(flags, "Hidden")
		}
		if checkFlag(c, "PINVIS") {
			flags = append(flags, "Invis")
		}
		if checkFlag(c, "PNOBRD") {
			flags = append(flags, "NoBroad")
		}
		if checkFlag(c, "PNOLDS") {
			flags = append(flags, "NoLong")
		}
		if checkFlag(c, "PNOSDS") {
			flags = append(flags, "NoShort")
		}
		if checkFlag(c, "PNORNM") {
			flags = append(flags, "NoName")
		}
		if checkFlag(c, "PNOEXT") {
			flags = append(flags, "NoExits")
		}
		if checkFlag(c, "PNOAAT") {
			flags = append(flags, "NoAutoAttk")
		}
		if checkFlag(c, "PNOEXT") {
			flags = append(flags, "NoWaitMsg")
		}
		if checkFlag(c, "PPROTE") {
			flags = append(flags, "Protect")
		}
		if checkFlag(c, "PDMINV") {
			flags = append(flags, "DMInvis")
		}
		if checkFlag(c, "PNOCMP") {
			flags = append(flags, "Noncompact")
		}
		if checkFlag(c, "PMALES") {
			flags = append(flags, "Male")
		}
		if checkFlag(c, "PWIMPY") {
			wimpyVal := dmCreatureStat(c, "wimpyValue")
			if wimpyVal == 0 {
				wimpyVal = dmCreatureStat(c, "wimpy")
			}
			flags = append(flags, fmt.Sprintf("Wimpy%d", wimpyVal))
		}
		if checkFlag(c, "PEAVES") {
			flags = append(flags, "Eaves")
		}
		if checkFlag(c, "PBLIND") {
			flags = append(flags, "Blind")
		}
		if checkFlag(c, "PCHARM") {
			flags = append(flags, "Charmed")
		}
		if checkFlag(c, "PLECHO") {
			flags = append(flags, "Echo")
		}
		if checkFlag(c, "PPOISN") {
			flags = append(flags, "Poisoned")
		}
		if checkFlag(c, "PDISEA") {
			flags = append(flags, "Diseased")
		}
		if checkFlag(c, "PLIGHT") {
			flags = append(flags, "Light")
		}
		if checkFlag(c, "PPROMP") {
			flags = append(flags, "Prompt")
		}
		if checkFlag(c, "PHASTE") {
			flags = append(flags, "Haste")
		}
		if checkFlag(c, "PPOWER") {
			flags = append(flags, "Power")
		}
		if checkFlag(c, "PSLAYE") {
			flags = append(flags, "Slayer")
		}
		if checkFlag(c, "PMEDIT") {
			flags = append(flags, "Meditate")
		}
		if checkFlag(c, "PUPDMG") {
			flags = append(flags, "Up-dmg")
		}
		if checkFlag(c, "PDMAGI") {
			flags = append(flags, "D-magic")
		}
		if checkFlag(c, "PDINVI") {
			flags = append(flags, "D-invis")
		}
		if checkFlag(c, "PPRAYD") {
			flags = append(flags, "Pray")
		}
		if checkFlag(c, "PPREPA") {
			flags = append(flags, "Prepared")
		}
		if checkFlag(c, "PLEVIT") {
			flags = append(flags, "Levitate")
		}
		if checkFlag(c, "PANSIC") {
			flags = append(flags, "Ansi")
		}
		if checkFlag(c, "PRFIRE") {
			flags = append(flags, "R-fire")
		}
		if checkFlag(c, "PFLYSP") {
			flags = append(flags, "Fly")
		}
		if checkFlag(c, "PRMAGI") {
			flags = append(flags, "R-magic")
		}
		if checkFlag(c, "PKNOWA") {
			flags = append(flags, "Know-a")
		}
		if checkFlag(c, "PNOSUM") {
			flags = append(flags, "Nosummon")
		}
		if checkFlag(c, "PIGNOR") {
			flags = append(flags, "Ignore-a")
		}
		if checkFlag(c, "PRCOLD") {
			flags = append(flags, "R-cold")
		}
		if checkFlag(c, "PBRWAT") {
			flags = append(flags, "Breath-wtr")
		}
		if checkFlag(c, "PSSHLD") {
			flags = append(flags, "Earth-shld")
		}
		if checkFlag(c, "PSILNC") {
			flags = append(flags, "Mute")
		}
		if checkFlag(c, "PFEARS") {
			flags = append(flags, "Fear")
		}
		if checkFlag(c, "PFMBOS") {
			flags = append(flags, "Family Boss")
		}
		if checkFlag(c, "PFAMIL") {
			flags = append(flags, "Family")
		}
		if checkFlag(c, "PMARRI") {
			flags = append(flags, "Married")
		}
		if checkFlag(c, "PRDFML") {
			flags = append(flags, "Family Wait")
		}
		if checkFlag(c, "PDSCRP") {
			flags = append(flags, "Description")
		}
		if checkFlag(c, "PPLDGK") {
			if checkFlag(c, "PKNGDM") {
				flags = append(flags, "Organization 1")
			} else {
				flags = append(flags, "Organization 0")
			}
		}
	} else {
		if checkFlag(c, "MPERMT") {
			flags = append(flags, "Perm")
		}
		if checkFlag(c, "MINVIS") {
			flags = append(flags, "Invis")
		}
		if checkFlag(c, "MAGGRE") {
			flags = append(flags, "Aggr")
		}
		if checkFlag(c, "MGAGGR") {
			flags = append(flags, "Good-Aggr")
		}
		if checkFlag(c, "MEAGGR") {
			flags = append(flags, "Evil-Aggr")
		}
		if checkFlag(c, "MGUARD") {
			flags = append(flags, "Guard")
		}
		if checkFlag(c, "MBLOCK") {
			flags = append(flags, "Block")
		}
		if checkFlag(c, "MFOLLO") {
			flags = append(flags, "Follow")
		}
		if checkFlag(c, "MFLEER") {
			flags = append(flags, "Flee")
		}
		if checkFlag(c, "MSCAVE") {
			flags = append(flags, "Scav")
		}
		if checkFlag(c, "MMALES") {
			flags = append(flags, "Male")
		}
		if checkFlag(c, "MPOISS") {
			flags = append(flags, "Poison")
		}
		if checkFlag(c, "MUNDED") {
			flags = append(flags, "Undead")
		}
		if checkFlag(c, "MUNSTL") {
			flags = append(flags, "No-steal")
		}
		if checkFlag(c, "MPOISN") {
			flags = append(flags, "Poisoned")
		}
		if checkFlag(c, "MMAGIC") {
			flags = append(flags, "Magic")
		}
		if checkFlag(c, "MHASSC") {
			flags = append(flags, "Scavenged")
		}
		if checkFlag(c, "MBRETH") {
			p1 := checkFlag(c, "MBRWP1")
			p2 := checkFlag(c, "MBRWP2")
			if !p1 && !p2 {
				flags = append(flags, "BR-fire")
			} else if p1 && !p2 {
				flags = append(flags, "BR-acid")
			} else if !p1 && p2 {
				flags = append(flags, "BR-frost")
			} else {
				flags = append(flags, "BR-gas")
			}
		}
		if checkFlag(c, "MMGONL") {
			flags = append(flags, "Magic-only")
		}
		if checkFlag(c, "MBLNDR") {
			flags = append(flags, "Blinder")
		}
		if checkFlag(c, "MBLIND") {
			flags = append(flags, "Blind")
		}
		if checkFlag(c, "MCHARM") {
			flags = append(flags, "Charmed")
		}
		if checkFlag(c, "MSILNC") {
			flags = append(flags, "Mute")
		}
		if checkFlag(c, "MMAGIO") {
			flags = append(flags, "Cast-percent")
		}
		if checkFlag(c, "MRBEFD") {
			flags = append(flags, "Resist-stun")
		}
		if checkFlag(c, "MNOCIR") {
			flags = append(flags, "No-circle")
		}
		if checkFlag(c, "MDINVI") {
			flags = append(flags, "Detect-invis")
		}
		if checkFlag(c, "MENONL") {
			flags = append(flags, "Enchant-only")
		}
		if checkFlag(c, "MRMAGI") {
			flags = append(flags, "Resist-magic")
		}
		if checkFlag(c, "MTALKS") {
			flags = append(flags, "Talks")
		}
		if checkFlag(c, "MUNKIL") {
			flags = append(flags, "Unkillable")
		}
		if checkFlag(c, "MNRGLD") {
			flags = append(flags, "NonrandGold")
		}
		if checkFlag(c, "MTLKAG") {
			flags = append(flags, "Talk-aggr")
		}
		if checkFlag(c, "MENEDR") {
			flags = append(flags, "Energy Drain")
		}
		if checkFlag(c, "MDISEA") {
			flags = append(flags, "Disease")
		}
		if checkFlag(c, "MDISIT") {
			flags = append(flags, "Dissolve")
		}
		if checkFlag(c, "MNOCHA") {
			flags = append(flags, "No-charmed")
		}
		if checkFlag(c, "MPURIT") {
			flags = append(flags, "Purchase")
		}
		if checkFlag(c, "MTRADE") {
			flags = append(flags, "Trade")
		}
		if checkFlag(c, "MFEARS") {
			flags = append(flags, "Fear")
		}
		if checkFlag(c, "MPGUAR") {
			flags = append(flags, "P-Guard")
		}
		if checkFlag(c, "MDEATH") {
			flags = append(flags, "Death scene")
		}
		if checkFlag(c, "MDMFOL") {
			flags = append(flags, "DM Follow")
		}
		if checkFlag(c, "MPLDGK") {
			if checkFlag(c, "MKNGDM") {
				flags = append(flags, "Pledge 1")
			} else {
				flags = append(flags, "Pledge 0")
			}
		}
		if checkFlag(c, "MRSCND") {
			if checkFlag(c, "MKNGDM") {
				flags = append(flags, "Rescind 1")
			} else {
				flags = append(flags, "Rescind 0")
			}
		}
		if checkFlag(c, "MKNDM1") {
			flags = append(flags, "King-1")
		}
		if checkFlag(c, "MKNDM2") {
			flags = append(flags, "King-2")
		}
		if checkFlag(c, "MSAYTLK") {
			flags = append(flags, "continue talk")
		}
		if checkFlag(c, "MSUMMO") {
			flags = append(flags, "summoner")
		}
	}

	flagStr := "Flags set: "
	if len(flags) > 0 {
		flagStr += strings.Join(flags, ", ") + "."
	} else {
		flagStr += "None."
	}
	fmt.Fprintf(&b, "%s\n", flagStr)

	return b.String()
}

func renderStatObj(world DMStatWorld, obj model.ObjectInstance) string {
	var b strings.Builder

	fmt.Fprintf(&b, "이름: %s\n", objectName(world, obj))
	fmt.Fprintf(&b, "설명: %s\n", objectDescription(world, obj))
	useOutput := objectProperty(world, obj, "useOutput")
	if useOutput == "" {
		useOutput = objectProperty(world, obj, "use_output")
	}
	fmt.Fprintf(&b, "사용:  %s\n", useOutput)

	k0 := objectProperty(world, obj, "key[0]")
	if k0 == "" {
		k0 = objectProperty(world, obj, "key/1")
	}
	k1 := objectProperty(world, obj, "key[1]")
	if k1 == "" {
		k1 = objectProperty(world, obj, "key/2")
	}
	k2 := objectProperty(world, obj, "key[2]")
	if k2 == "" {
		k2 = objectProperty(world, obj, "key/3")
	}
	fmt.Fprintf(&b, "단어: %s %+20s %+20s\n\n", k0, k1, k2)

	ndice := dmObjectIntProperty(world, obj, "nDice")
	sdice := dmObjectIntProperty(world, obj, "sDice")
	pdice := dmObjectIntProperty(world, obj, "pDice")
	adjust := dmObjectIntProperty(world, obj, "adjustment")

	fmt.Fprintf(&b, "타격: %dd%d + %d", ndice, sdice, pdice)
	if adjust != 0 {
		fmt.Fprintf(&b, " (+%d)\n", adjust)
	} else {
		b.WriteString("\n")
	}

	shotscur := dmObjectIntProperty(world, obj, "shotsCurrent")
	shotsmax := dmObjectIntProperty(world, obj, "shotsMax")
	fmt.Fprintf(&b, "사용회수 %d/%d\n", shotscur, shotsmax)

	b.WriteString("종류: ")
	objType := dmObjectIntProperty(world, obj, "type")
	if objType <= 4 {
		switch objType {
		case 0:
			b.WriteString("도")
		case 1:
			b.WriteString("검")
		case 2:
			b.WriteString("봉")
		case 3:
			b.WriteString("창")
		case 4:
			b.WriteString("궁")
		}
		b.WriteString(" 무기.\n")
	} else {
		fmt.Fprintf(&b, "%d\n", objType)
	}

	armor := dmObjectIntProperty(world, obj, "armor")
	value := dmObjectIntProperty(world, obj, "value")
	weight := dmObjectIntProperty(world, obj, "weight")
	questnum := dmObjectIntProperty(world, obj, "questnum")
	if questnum == 0 {
		questnum = dmObjectIntProperty(world, obj, "questNum")
	}

	fmt.Fprintf(&b, "방어력: %2.2d  가격: %5.5d  무게: %2.2d", armor, value, weight)
	if questnum != 0 {
		fmt.Fprintf(&b, "   임무: %d\n", questnum)
	} else {
		b.WriteString("\n")
	}

	var flags []string
	if dmObjectHasAnyFlagOrProperty(world, obj, "OPERMT", "perm", "permanent") {
		flags = append(flags, "Pperm")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OHIDDN", "hidden") {
		flags = append(flags, "Hidden")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OINVIS", "invis", "invisible") {
		flags = append(flags, "Invis")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OCONTN", "container", "cont") {
		flags = append(flags, "Cont")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OWTLES", "wtless", "weightless") {
		flags = append(flags, "Wtless")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OTEMPP", "tperm", "temporary") {
		flags = append(flags, "Tperm")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OPERM2", "iperm") {
		flags = append(flags, "Iperm")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "ONOMAG", "nomage", "noMagic") {
		flags = append(flags, "Nomage")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OLIGHT", "light") {
		flags = append(flags, "Light")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OGOODO", "good", "goodOnly") {
		flags = append(flags, "Good")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OEVILO", "evil", "evilOnly") {
		flags = append(flags, "Evil")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OENCHA", "ench", "enchanted") {
		flags = append(flags, "Ench")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "ONOFIX", "nofix") {
		flags = append(flags, "Nofix")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OCLIMB", "climbing") {
		flags = append(flags, "Climbing")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "ONOTAK", "notake", "noTake") {
		flags = append(flags, "Notake")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OSCENE", "scenery") {
		flags = append(flags, "Scenery")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OSIZE1", "osize1") || dmObjectHasAnyFlagOrProperty(world, obj, "OSIZE2", "osize2") {
		flags = append(flags, "Sized")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "ORENCH", "randEnch") {
		flags = append(flags, "RandEnch")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OCURSE", "cursed", "curse") {
		flags = append(flags, "Cursed")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OWEARS", "worn") {
		flags = append(flags, "Worn")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OUSEFL", "useFloor", "use-floor") {
		flags = append(flags, "Use-floor")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OCNDES", "devours") {
		flags = append(flags, "Devours")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "ONOMAL", "nomale") {
		flags = append(flags, "Nomale")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "ONOFEM", "nofemale") {
		flags = append(flags, "Nofemale")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "ONSHAT", "shatterproof") {
		flags = append(flags, "Shatterproof")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OALCRT", "alwaysCrit", "always crit") {
		flags = append(flags, "Always crit")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "ODDICE", "ndS damage", "ndSdamage") {
		flags = append(flags, "NdS damage")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OCNAME", "changeName", "Change Name") {
		flags = append(flags, "Change Name")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OSPECI", "specialItem", "Special Item") {
		flags = append(flags, "Special Item")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OMARRI", "marriage") {
		flags = append(flags, "Marriage")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OEVENT", "eventItem", "Event Item") {
		flags = append(flags, "Event Item")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "ONOBUN", "noburn") {
		flags = append(flags, "Noburn")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OWHELD", "held") {
		flags = append(flags, "Held")
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OPLDGK", "org", "organization") {
		if dmObjectHasAnyFlagOrProperty(world, obj, "OKNGDM", "kngdm") {
			flags = append(flags, "Organization 1")
		} else {
			flags = append(flags, "Organization 0")
		}
	}
	if dmObjectHasAnyFlagOrProperty(world, obj, "OCLSEL", "clsSel", "cls-sel") {
		clsStr := "Cls-Sel: "
		var classes []string
		if dmObjectHasAnyFlagOrProperty(world, obj, "OASSNO", "assno") {
			classes = append(classes, "자")
		}
		if dmObjectHasAnyFlagOrProperty(world, obj, "OBARBO", "barbo") {
			classes = append(classes, "권")
		}
		if dmObjectHasAnyFlagOrProperty(world, obj, "OCLERO", "clero") {
			classes = append(classes, "불")
		}
		if dmObjectHasAnyFlagOrProperty(world, obj, "OFIGHO", "figho") {
			classes = append(classes, "검")
		}
		if dmObjectHasAnyFlagOrProperty(world, obj, "OMAGEO", "mageo") {
			classes = append(classes, "도")
		}
		if dmObjectHasAnyFlagOrProperty(world, obj, "OPALAO", "palao") {
			classes = append(classes, "무")
		}
		if dmObjectHasAnyFlagOrProperty(world, obj, "ORNGRO", "rngro") {
			classes = append(classes, "포")
		}
		if dmObjectHasAnyFlagOrProperty(world, obj, "OTHIEO", "thieo") {
			classes = append(classes, "도")
		}
		if len(classes) > 0 {
			clsStr += strings.Join(classes, ", ") + ", "
		}
		flags = append(flags, strings.TrimSuffix(clsStr, ", "))
	}

	flagStr := "Flags set: "
	if len(flags) > 0 {
		flagStr += strings.Join(flags, ", ") + "."
	} else {
		flagStr += "None."
	}
	fmt.Fprintf(&b, "%s\n", flagStr)

	return b.String()
}

func checkFlag(c model.Creature, uppercase string) bool {
	return creatureHasAnyFlag(c, uppercase, strings.ToLower(uppercase))
}

func checkRoomFlag(r model.Room, uppercase string) bool {
	return roomHasAnyFlag(r, uppercase, strings.ToLower(uppercase))
}

func checkExitFlag(e model.Exit, uppercase string) bool {
	return exitHasAnyFlag(e, uppercase, strings.ToLower(uppercase))
}

func getRoomTrapInfo(room model.Room) (int, int) {
	trapVal := 0
	if valStr, ok := room.Properties["trap"]; ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			trapVal = val
		}
	}
	trapExitVal := 0
	for _, key := range []string{"trapExit", "trapexit", "trap_exit"} {
		if valStr, ok := room.Properties[key]; ok {
			if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
				trapExitVal = val
				break
			}
		}
	}
	return trapVal, trapExitVal
}

func getCreatureKeys(creature model.Creature) (string, string, string) {
	k0 := creature.Properties["key[0]"]
	if k0 == "" {
		k0 = creature.Properties["key/1"]
	}
	k1 := creature.Properties["key[1]"]
	if k1 == "" {
		k1 = creature.Properties["key/2"]
	}
	k2 := creature.Properties["key[2]"]
	if k2 == "" {
		k2 = creature.Properties["key/3"]
	}
	return k0, k1, k2
}

func creatureClassName(classVal int) string {
	classes := []string{
		"바보", "자객", "권법가", "불제자",
		"검사", "도술사", "무사", "포졸",
		"도둑", "무적", "초인", "불사",
		"운영자", "관리자",
	}
	if classVal >= 0 && classVal < len(classes) {
		return classes[classVal]
	}
	return fmt.Sprintf("%d", classVal)
}

func creatureIsChaos(creature model.Creature) bool {
	if checkFlag(creature, "PCHAOS") {
		return true
	}
	alignVal := 0
	if val, ok := creature.Stats["alignment"]; ok {
		alignVal = val
	} else if valStr, ok := creature.Properties["alignment"]; ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			alignVal = val
		}
	}
	return alignVal < 0
}

func creatureAlignment(creature model.Creature) int {
	if val, ok := creature.Stats["alignment"]; ok {
		return val
	}
	if valStr, ok := creature.Properties["alignment"]; ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			return val
		}
	}
	return 0
}

func dmCreatureStat(creature model.Creature, key string) int {
	if val, ok := creature.Stats[key]; ok {
		return val
	}
	if val, ok := creature.Stats[strings.ToLower(key)]; ok {
		return val
	}
	if valStr, ok := creature.Properties[key]; ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			return val
		}
	}
	if valStr, ok := creature.Properties[strings.ToLower(key)]; ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			return val
		}
	}
	return 0
}

func getDiceStats(creature model.Creature) (int, int, int) {
	ndice := dmCreatureStat(creature, "nDice")
	if ndice == 0 {
		ndice = dmCreatureStat(creature, "ndice")
	}
	if ndice == 0 {
		ndice = dmCreatureStat(creature, "ndiceNo")
	}
	sdice := dmCreatureStat(creature, "sDice")
	if sdice == 0 {
		sdice = dmCreatureStat(creature, "sdice")
	}
	pdice := dmCreatureStat(creature, "pDice")
	if pdice == 0 {
		pdice = dmCreatureStat(creature, "pdice")
	}
	return ndice, sdice, pdice
}

func creatureProficiency(c model.Creature, idx int) int {
	statKeys := []string{"proficiencySharp", "proficiencyThrust", "proficiencyBlunt", "proficiencyPole", "proficiencyMissile"}
	propertyParts := []string{"sharp", "thrust", "blunt", "pole", "missile"}
	valueForKey := func(key string) (int, bool) {
		if val, ok := c.Stats[key]; ok {
			return val, true
		}
		if valStr, ok := c.Properties[key]; ok {
			if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
				return val, true
			}
		}
		return 0, false
	}
	if idx >= 0 && idx < len(statKeys) {
		part := propertyParts[idx]
		for _, key := range []string{
			statKeys[idx],
			fmt.Sprintf("proficiency/%s", part),
			fmt.Sprintf("proficiency.%s", part),
			fmt.Sprintf("proficiency_%s", part),
		} {
			if val, ok := valueForKey(key); ok {
				return val
			}
		}
	}
	indexKeys := []string{
		fmt.Sprintf("proficiency/%d", idx),
		fmt.Sprintf("proficiency.%d", idx),
		fmt.Sprintf("proficiency_%d", idx),
		fmt.Sprintf("proficiency%d", idx),
	}
	for _, key := range indexKeys {
		if val, ok := valueForKey(key); ok {
			return val
		}
	}
	return 0
}

func creatureRealm(c model.Creature, idx int) int {
	keys := []string{"realmEarth", "realmWind", "realmFire", "realmWater"}
	if idx >= 0 && idx < len(keys) {
		if val, ok := c.Stats[keys[idx]]; ok {
			return val
		}
	}
	indexKeys := []string{
		fmt.Sprintf("realm/%d", idx+1),
		fmt.Sprintf("realm.%d", idx+1),
		fmt.Sprintf("realm_%d", idx+1),
		fmt.Sprintf("realm%d", idx+1),
	}
	for _, key := range indexKeys {
		if val, ok := c.Stats[key]; ok {
			return val
		}
		if valStr, ok := c.Properties[key]; ok {
			if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
				return val
			}
		}
	}
	return 0
}

func creatureRaceName(creature model.Creature) string {
	if val := strings.TrimSpace(creature.Properties["raceName"]); val != "" {
		return val
	}
	if val := strings.TrimSpace(creature.Properties["race_name"]); val != "" {
		return val
	}
	raceVal := dmCreatureStat(creature, "race")
	races := []string{
		"바보족", "난장이족", "용신족", "요괴족", "토신족",
		"인간족", "도깨비족", "거인족", "땅귀신족", "개구리족",
	}
	if raceVal >= 0 && raceVal < len(races) {
		return races[raceVal]
	}
	return fmt.Sprintf("%d", raceVal)
}

func objectProperty(world DMStatWorld, obj model.ObjectInstance, key string) string {
	if val, ok := obj.Properties[key]; ok {
		return val
	}
	if !obj.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(obj.PrototypeID); ok {
			if val, ok := proto.Properties[key]; ok {
				return val
			}
		}
	}
	return ""
}

func objectName(world DMStatWorld, obj model.ObjectInstance) string {
	if obj.DisplayNameOverride != "" {
		return obj.DisplayNameOverride
	}
	if !obj.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(obj.PrototypeID); ok {
			return proto.DisplayName
		}
	}
	return string(obj.ID)
}

func objectDescription(world DMStatWorld, obj model.ObjectInstance) string {
	if val, ok := obj.Properties["description"]; ok {
		return val
	}
	if !obj.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(obj.PrototypeID); ok {
			return proto.Description
		}
	}
	return ""
}

func dmObjectIntProperty(world DMStatWorld, obj model.ObjectInstance, key string) int {
	valStr := objectProperty(world, obj, key)
	if valStr == "" {
		valStr = objectProperty(world, obj, strings.ToLower(key))
	}
	if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
		return val
	}
	return 0
}

func dmObjectHasAnyFlagOrProperty(world DMStatWorld, obj model.ObjectInstance, keys ...string) bool {
	targets := normalizedFlagSet(keys...)
	if hasAnyNormalizedFlag(obj.Metadata.Tags, keys...) || dmPropertiesHaveAnyFlag(obj.Properties, targets) {
		return true
	}
	if obj.PrototypeID.IsZero() {
		return false
	}
	proto, ok := world.ObjectPrototype(obj.PrototypeID)
	if !ok {
		return false
	}
	return hasAnyNormalizedFlag(proto.Metadata.Tags, keys...) || dmPropertiesHaveAnyFlag(proto.Properties, targets)
}

func dmPropertiesHaveAnyFlag(properties map[string]string, targets map[string]struct{}) bool {
	for key, value := range properties {
		if _, ok := targets[normalizeFlagName(key)]; ok && propertyFlagEnabled(value) {
			return true
		}
		if objectFlagContainerProperty(key) && propertyFlagValueHasAnyToken(value, targets) {
			return true
		}
	}
	return false
}
