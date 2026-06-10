package command

import (
	"fmt"
	"strconv"
	"strings"

	"muhan/internal/world/model"
)

// DMSetWorld defines a small mockable interface for the World dependencies of the dm_set command.
type DMSetWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
	FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool)
	FindObjectInRoom(roomID model.RoomID, name string) (model.ObjectInstance, bool)
	FindObjectOnCreature(creatureID model.CreatureID, name string) (model.ObjectInstance, bool)
	UpdateRoomProperty(model.RoomID, string, string) error
	UpdateRoomRandomCreature(model.RoomID, int, int) error
	UpdateRoomFlag(model.RoomID, int, bool) error
	UpdateCreatureStat(model.CreatureID, string, int) error
	UpdateCreatureProperty(model.CreatureID, string, string) error
	UpdateObjectProperty(model.ObjectInstanceID, string, string) error
	LinkExits(fromRoomID, toRoomID model.RoomID, exitName, oppositeName string, doubleWay bool) error
	DeleteRoomExit(roomID model.RoomID, exitName string) error
	SetExitFlag(roomID model.RoomID, exitName string, flag string, enabled bool) (model.Exit, error)
}

var roomFlagNamesList = []string{
	"shoppe",          // 1
	"dump",            // 2
	"pawnShop",        // 3
	"train",           // 4
	"trainingBit4",    // 5
	"trainingBit5",    // 6
	"trainingBit6",    // 7
	"repair",          // 8
	"darkAlways",      // 9
	"darkNight",       // 10
	"postOffice",      // 11
	"noPlayerKill",    // 12
	"noTeleport",      // 13
	"healFast",        // 14
	"onePlayer",       // 15
	"twoPlayers",      // 16
	"threePlayers",    // 17
	"noMagic",         // 18
	"permanentTracks", // 19
	"earth",           // 20
	"wind",            // 21
	"fire",            // 22
	"water",           // 23
	"playerWander",    // 24
	"playerHarm",      // 25
	"playerPoison",    // 26
	"playerMPDrain",   // 27
	"playerBefuddle",  // 28
	"noSummonOut",     // 29
	"pledge",          // 30
	"rescind",         // 31
	"noPotion",        // 32
	"magicExtend",     // 33
	"noLog",           // 34
	"election",        // 35
	"forge",           // 36
	"survival",        // 37
	"family",          // 38
	"onlyFamily",      // 39
	"bank",            // 40
	"marriage",        // 41
	"onlyMarried",     // 42
	"cast",            // 43
	"depot",           // 44
}

var exitFlagNamesList = []string{
	"secret",          // 1
	"invisible",       // 2
	"locked",          // 3
	"closed",          // 4
	"lockable",        // 5
	"closable",        // 6
	"unpickable",      // 7
	"naked",           // 8
	"climb",           // 9
	"repel",           // 10
	"hardClimb",       // 11
	"fly",             // 12
	"femaleOnly",      // 13
	"maleOnly",        // 14
	"pledgeOnly",      // 15
	"kingdomSelector", // 16
	"nightOnly",       // 17
	"dayOnly",         // 18
	"guarded",         // 19
	"noSee",           // 20
	"kingdom1",        // 21
	"kingdom2",        // 22
}

var creatureFlagNamesList = []string{
	"permanent", "hidden", "invisible", "manToMenPlural", "noPluralSuffix", "noPrefix", "aggressive", "guardTreasure",
	"blocksExits", "followsAttacker", "flees", "scavenger", "male", "poisoner", "undead", "cannotSteal",
	"poisoned", "magicUser", "hasScavenged", "breathWeapon", "magicOnly", "detectInvisible", "magicOrEnchantedOnly", "talks",
	"unkillable", "fixedGold", "talkAggressive", "resistMagic", "breathWeaponType1", "breathWeaponType2", "energyDrain", "kingdom",
	"pledgeKingdom", "rescindKingdom", "disease", "dissolveItems", "purchaseItems", "tradeItems", "passiveExitGuard", "goodAggressive",
	"evilAggressive", "deathDescription", "magicPercent", "resistStunOnly", "cannotCircle", "blind", "followDM", "fearful",
	"silenced", "blinded", "charmed", "befuddled", "kingdom1", "kingdom2", "kingdom3", "kingdom4", "king1",
	"king2", "king3", "king4", "sayTalk", "summoner", "noCharm",
}

var objectFlagNamesList = []string{
	"permanent",          // 1
	"hidden",             // 2
	"invisible",          // 3
	"somePrefix",         // 4
	"noPluralSuffix",     // 5
	"noPrefix",           // 6
	"container",          // 7
	"weightless",         // 8
	"temporaryPermanent", // 9
	"inventoryPermanent", // 10
	"noMage",             // 11
	"lightSource",        // 12
	"goodOnly",           // 13
	"evilOnly",           // 14
	"enchanted",          // 15
	"noRepair",           // 16
	"climbGear",          // 17
	"noTake",             // 18
	"scenery",            // 19
	"sizeSmall",          // 20
	"sizeLarge",          // 21
	"randomEnchantment",  // 22
	"cursed",             // 23
	"worn",               // 24
	"useFromFloor",       // 25
	"containerDevours",   // 26
	"femaleOnly",         // 27
	"maleOnly",           // 28
	"damageDice",         // 29
	"pledgeOnly",         // 30
	"kingdomBound",       // 31
	"classSelective",     // 32
	"classAssassin",      // 33
	"classBarbarian",     // 34
	"classCleric",        // 35
	"classFighter",       // 36
	"classMage",          // 37
	"classPaladin",       // 38
	"classRanger",        // 39
	"classThief",         // 40
	"stunLengthDice",     // 41
	"neverShatter",       // 42
	"alwaysCritical",     // 43
	"customName",         // 44
	"specialItem",        // 45
	"marriageOnly",       // 46
	"eventItem",          // 47
	"named",              // 48
	"noBurn",             // 49
	"held",               // 50
}

// NewDMSetHandler creates a new Handler for the dm_set command.
func NewDMSetHandler(world DMSetWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmSet(ctx, resolved, world)
	}
}

func dmSet(ctx *Context, resolved ResolvedCommand, world DMSetWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	playerID := model.PlayerID(ctx.ActorID)
	var creatureID model.CreatureID
	var player model.Player
	var ok bool
	if player, ok = world.Player(playerID); ok {
		creatureID = player.CreatureID
	} else {
		creatureID = model.CreatureID(ctx.ActorID)
	}

	creature, ok := world.Creature(creatureID)
	if !ok {
		return StatusDefault, nil
	}

	class := creatureClass(creature)
	if class < model.ClassDM {
		return StatusPrompt, nil
	}

	if resolved.Parsed.Num < 2 || dmSetArgCount(resolved) < 1 {
		ctx.WriteString("Set what?\n")
		return StatusDefault, nil
	}

	category := strings.ToLower(dmSetArg(resolved, 0))
	if category != "" {
		switch category[0] {
		case 'x':
			return dmSetExt(ctx, resolved, world, creature)
		case 'r':
			return dmSetRom(ctx, resolved, world, creature)
		case 'c', 'p', 'm':
			return dmSetCrt(ctx, resolved, world, creature)
		case 'o', 'i':
			return dmSetObj(ctx, resolved, world, creature, inventoryViewerDetectsInvisible(player, creature))
		}
	}
	ctx.WriteString("Invalid option.  *set <x|r|c|o> <options>\n")
	return StatusDefault, nil
}

func dmSetArgCount(resolved ResolvedCommand) int {
	if resolved.Parsed.Num > 0 {
		return resolved.Parsed.Num - 1
	}
	return len(resolved.Args)
}

func dmSetArg(resolved ResolvedCommand, index int) string {
	slot := index + 1
	if resolved.Parsed.Num > slot {
		if arg := strings.TrimSpace(resolved.Parsed.Str[slot]); arg != "" {
			return arg
		}
	}
	return getArg(resolved, index)
}

func dmSetExt(ctx *Context, resolved ResolvedCommand, world DMSetWorld, creature model.Creature) (Status, error) {
	isFlag := false
	var exitName string
	var flagNum int

	category := strings.ToLower(dmSetArg(resolved, 0))
	if len(category) > 1 && category[1] == 'f' {
		isFlag = true
		if dmSetArgCount(resolved) < 2 {
			ctx.WriteString("Syntax: *set xf <exit> <value>\n")
			return StatusDefault, nil
		}
		exitName = dmSetArg(resolved, 1)
		flagNum = dmSetValueAt(resolved, 1)
	}

	if isFlag {
		if flagNum < 1 || flagNum > 32 {
			return StatusDefault, nil
		}

		roomID := creature.RoomID
		room, ok := world.Room(roomID)
		if !ok {
			ctx.WriteString("Room not found.\n")
			return StatusDefault, nil
		}

		var foundExit *model.Exit
		for _, ext := range room.Exits {
			if ext.Name == exitName {
				hasNoSee := exitHasAnyFlag(ext, "noSee")
				if !hasNoSee {
					foundExit = &ext
					break
				}
			}
		}

		if foundExit == nil {
			ctx.WriteString("Exit not found.\n")
			return StatusDefault, nil
		}

		flagStr := ""
		if flagNum-1 >= 0 && flagNum-1 < len(exitFlagNamesList) {
			flagStr = exitFlagNamesList[flagNum-1]
		} else {
			flagStr = strconv.Itoa(flagNum)
		}

		hasFlag := exitHasAnyFlag(*foundExit, flagStr)

		_, err := world.SetExitFlag(room.ID, foundExit.Name, flagStr, !hasFlag)
		if err != nil {
			return StatusDefault, err
		}

		if hasFlag {
			ctx.WriteString(fmt.Sprintf("%s exit flag #%d off.\n", foundExit.Name, flagNum))
		} else {
			ctx.WriteString(fmt.Sprintf("%s exit flag #%d on.\n", foundExit.Name, flagNum))
		}
		return StatusDefault, nil
	}

	// Normal exit setup
	if resolved.Parsed.Num < 3 {
		ctx.WriteString("Syntax: *set [#] x <name> <#> [. or name]\n")
		return StatusDefault, nil
	}

	var fromRoomID model.RoomID
	fromRoomVal := resolved.Parsed.Val[0]
	if fromRoomVal == 1 {
		fromRoomID = creature.RoomID
	} else {
		fromRoomID = roomIDFromNumber(int(fromRoomVal))
	}

	fromRoom, ok := world.Room(fromRoomID)
	if !ok {
		ctx.WriteString(fmt.Sprintf("Room %d does not exist.\n", fromRoomVal))
		return StatusDefault, nil
	}

	exitName = expandExitName(resolved.Parsed.Str[2])
	toRoomVal := resolved.Parsed.Val[2]

	if toRoomVal == 1 {
		ctx.WriteString("Link exit to which room?\n")
		return StatusDefault, nil
	} else if toRoomVal == 0 {
		// Delete room exit
		found := false
		for _, ext := range fromRoom.Exits {
			if strings.EqualFold(ext.Name, exitName) {
				found = true
				break
			}
		}
		if !found {
			ctx.WriteString(fmt.Sprintf("Exit %s not found.\n", exitName))
			return StatusDefault, nil
		}

		err := world.DeleteRoomExit(fromRoomID, exitName)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("Exit %s deleted.\n", exitName))
	} else {
		// Link exits
		toRoomID := roomIDFromNumber(int(toRoomVal))

		doubleWay := false
		oppositeName := ""
		if resolved.Parsed.Num > 3 && resolved.Parsed.Str[3] != "" {
			doubleWay = true
			oppositeName = resolved.Parsed.Str[3]
			if strings.HasPrefix(oppositeName, ".") {
				if opp, ok := oppositeExitName(exitName); ok {
					oppositeName = opp
				} else {
					oppositeName = exitName
				}
			}
		}

		if err := world.LinkExits(fromRoomID, toRoomID, exitName, "", false); err != nil {
			return StatusDefault, err
		}

		if doubleWay {
			if _, ok := world.Room(toRoomID); !ok {
				ctx.WriteString(fmt.Sprintf("Room %d does not exist.\n", fromRoomVal))
			} else if err := world.LinkExits(toRoomID, fromRoomID, oppositeName, "", false); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(fmt.Sprintf("Room #%d linked to room #%d in %s direction, both ways.\n", legacyRoomNumber(fromRoom), toRoomVal, exitName))
		} else {
			ctx.WriteString(fmt.Sprintf("Room #%d linked to room #%d in %s direction.\n", legacyRoomNumber(fromRoom), toRoomVal, exitName))
		}
	}

	return StatusDefault, nil
}

func dmSetRom(ctx *Context, resolved ResolvedCommand, world DMSetWorld, creature model.Creature) (Status, error) {
	if dmSetArgCount(resolved) < 2 {
		ctx.WriteString("Syntax: *set r [trf] [<value>]\n")
		return StatusDefault, nil
	}

	option := dmSetArg(resolved, 1)
	val := dmSetValueAt(resolved, 1)
	roomID := creature.RoomID

	room, ok := world.Room(roomID)
	if !ok {
		ctx.WriteString("Room not found.\n")
		return StatusDefault, nil
	}

	switch strings.ToLower(option[:1]) {
	case "t":
		err := world.UpdateRoomProperty(roomID, "traffic", strconv.Itoa(val))
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("Traffic is now %d%%.\n", val))
	case "s":
		err := world.UpdateRoomProperty(roomID, "special", strconv.Itoa(val))
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("Special is now %d.\n", val))
	case "r":
		if len(option) < 2 {
			return StatusDefault, nil
		}
		num, err := strconv.Atoi(option[1:])
		if err != nil || num < 1 || num > 10 {
			return StatusDefault, nil
		}
		err = world.UpdateRoomRandomCreature(roomID, num-1, val)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("Random #%d is now %d.\n", num, val))
	case "f":
		if val < 1 || val > 64 {
			return StatusDefault, nil
		}
		var flagName string
		if val-1 < len(roomFlagNamesList) {
			flagName = roomFlagNamesList[val-1]
		} else {
			flagName = strconv.Itoa(val)
		}

		hasFlag := roomHasAnyFlag(room, flagName)

		err := world.UpdateRoomFlag(roomID, val, !hasFlag)
		if err != nil {
			return StatusDefault, err
		}

		if hasFlag {
			ctx.WriteString(fmt.Sprintf("Room flag #%d off.\n", val))
		} else {
			ctx.WriteString(fmt.Sprintf("Room flag #%d on.\n", val))
		}
	case "b":
		if len(option) < 2 {
			return StatusDefault, nil
		}
		optChar := strings.ToLower(option[1:2])
		if optChar == "l" {
			err := world.UpdateRoomProperty(roomID, "minLevel", strconv.Itoa(val))
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(fmt.Sprintf("Low level boundary %d\n", val))
		} else if optChar == "h" {
			err := world.UpdateRoomProperty(roomID, "maxLevel", strconv.Itoa(val))
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(fmt.Sprintf("Upper level boundary %d\n", val))
		}
	case "x":
		err := world.UpdateRoomProperty(roomID, "trap", strconv.Itoa(val))
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("Room has trap #%d set.", val))
	default:
		ctx.WriteString("Invalid option.\n")
	}

	return StatusDefault, nil
}

func dmSetCrt(ctx *Context, resolved ResolvedCommand, world DMSetWorld, creature model.Creature) (Status, error) {
	if dmSetArgCount(resolved) < 3 {
		ctx.WriteString("Syntax: *set c <name> <a|con|c|dex|e|f|g|hm|h|int|l|mm|m|\n                       pie|p#|r#|str> [<value>]\n")
		return StatusDefault, nil
	}

	crtName := dmSetArg(resolved, 1)
	var targetCreature model.Creature
	var foundCrt bool
	actorClass := creatureClass(creature)

	if targetCrt, ok := dmSetFindOnlinePlayer(ctx, world, legacyLowercizeASCII(crtName, true)); ok {
		isPdmInv := creatureHasAnyFlag(targetCrt, "PDMINV", "dmInvisible")
		if !(actorClass < model.ClassCaretaker && isPdmInv) {
			targetCreature = targetCrt
			foundCrt = true
		}
	}

	if !foundCrt {
		if crt, ok := dmSetFindRoomCreature(world, creature, crtName, dmSetCreatureOrdinal(resolved)); ok {
			targetCreature = crt
			foundCrt = true
		}
	}

	if !foundCrt {
		ctx.WriteString("Creature not found.\n")
		return StatusDefault, nil
	}

	stat := strings.ToLower(dmSetArg(resolved, 2))
	val := dmSetValueAt(resolved, 2)

	switch stat[0] {
	case 'a':
		if stat == "ar" && targetCreature.Kind == model.CreatureKindMonster {
			err := world.UpdateCreatureStat(targetCreature.ID, "armor", 100-val)
			if err != nil {
				return StatusDefault, err
			}
		} else if stat == "age" {
			err := world.UpdateCreatureStat(targetCreature.ID, "age", val)
			if err != nil {
				return StatusDefault, err
			}
		} else {
			err := world.UpdateCreatureStat(targetCreature.ID, "alignment", val)
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Alignment set.\n")
		}
	case 'c':
		if stat == "con" {
			err := world.UpdateCreatureStat(targetCreature.ID, "constitution", val)
			if err != nil {
				return StatusDefault, err
			}
		} else {
			actorClass := creatureClass(creature)
			if actorClass < model.ClassDM {
				return StatusDefault, nil
			}

			targetClass := val
			isPlayer := targetCreature.Kind == model.CreatureKindPlayer || !targetCreature.PlayerID.IsZero()
			if isPlayer && val == model.ClassDM {
				if !isAllowedDMName(targetCreature.DisplayName) {
					targetClass = model.ClassSubDM
				}
			}
			err := world.UpdateCreatureStat(targetCreature.ID, "class", targetClass)
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Class set.\n")
		}
	case 'd':
		if stat == "dex" {
			err := world.UpdateCreatureStat(targetCreature.ID, "dexterity", val)
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Dexterity set.\n")
		} else if stat == "dn" && targetCreature.Kind == model.CreatureKindMonster {
			err := world.UpdateCreatureStat(targetCreature.ID, "nDice", val)
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Number of dice set.\n")
		} else if stat == "ds" && targetCreature.Kind == model.CreatureKindMonster {
			err := world.UpdateCreatureStat(targetCreature.ID, "sDice", val)
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Sides of dice set.\n")
		} else if stat == "dp" && targetCreature.Kind == model.CreatureKindMonster {
			err := world.UpdateCreatureStat(targetCreature.ID, "pDice", val)
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Plus on dice set.\n")
		}
	case 'e':
		err := world.UpdateCreatureStat(targetCreature.ID, "experience", val)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("%s has %d experience.\n", targetCreature.DisplayName, val))
	case 'f':
		if val < 1 || val > 64 {
			return StatusDefault, nil
		}
		var flagName string
		if val-1 < len(creatureFlagNamesList) {
			flagName = creatureFlagNamesList[val-1]
		} else {
			flagName = strconv.Itoa(val)
		}

		hasFlag := creatureHasAnyFlag(targetCreature, flagName)
		var err error
		if tagger, ok := world.(interface {
			UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
		}); ok {
			if hasFlag {
				if _, tagErr := tagger.UpdateCreatureTags(targetCreature.ID, nil, []string{flagName}); tagErr != nil {
					return StatusDefault, tagErr
				}
			} else {
				if _, tagErr := tagger.UpdateCreatureTags(targetCreature.ID, []string{flagName}, nil); tagErr != nil {
					return StatusDefault, tagErr
				}
			}
		}
		if hasFlag {
			if err = world.UpdateCreatureStat(targetCreature.ID, flagName, 0); err != nil {
				return StatusDefault, err
			}
			err = world.UpdateCreatureProperty(targetCreature.ID, flagName, "false")
		} else {
			if err = world.UpdateCreatureStat(targetCreature.ID, flagName, 1); err != nil {
				return StatusDefault, err
			}
			err = world.UpdateCreatureProperty(targetCreature.ID, flagName, "true")
		}
		if err != nil {
			return StatusDefault, err
		}

		if hasFlag {
			ctx.WriteString(fmt.Sprintf("%s's flag #%d off.\n", targetCreature.DisplayName, val))
		} else {
			ctx.WriteString(fmt.Sprintf("%s's flag #%d on.\n", targetCreature.DisplayName, val))
		}
	case 'g':
		err := world.UpdateCreatureStat(targetCreature.ID, "gold", val)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("%s has %d gold.\n", targetCreature.DisplayName, val))
	case 'h':
		if stat == "hm" {
			err := world.UpdateCreatureStat(targetCreature.ID, "hpMax", val)
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(fmt.Sprintf("%s's max hp is now %d.\n", targetCreature.DisplayName, val))
		} else {
			err := world.UpdateCreatureStat(targetCreature.ID, "hpCurrent", val)
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(fmt.Sprintf("%s's current hp is now %d.\n", targetCreature.DisplayName, val))
		}
	case 'i':
		err := world.UpdateCreatureStat(targetCreature.ID, "intelligence", val)
		if err != nil {
			return StatusDefault, err
		}
	case 'l':
		err := world.UpdateCreatureStat(targetCreature.ID, "level", val)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("%s's level set to %d.\n", targetCreature.DisplayName, val))
	case 'm':
		if stat == "mm" {
			err := world.UpdateCreatureStat(targetCreature.ID, "mpMax", val)
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(fmt.Sprintf("%s's max mp is now %d.\n", targetCreature.DisplayName, val))
		} else {
			err := world.UpdateCreatureStat(targetCreature.ID, "mpCurrent", val)
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(fmt.Sprintf("%s's current mp is now %d.\n", targetCreature.DisplayName, val))
		}
	case 'p':
		if stat == "pi" || stat == "piety" {
			err := world.UpdateCreatureStat(targetCreature.ID, "piety", val)
			if err != nil {
				return StatusDefault, err
			}
		} else if strings.HasPrefix(stat, "p#") {
			num, err := strconv.Atoi(stat[2:])
			if err == nil && num >= 1 && num <= 5 {
				err = world.UpdateCreatureStat(targetCreature.ID, fmt.Sprintf("proficiency%d", num), val)
				if err != nil {
					return StatusDefault, err
				}
				ctx.WriteString("Proficiency set.\n")
			}
		}
	case 'r':
		if strings.HasPrefix(stat, "r#") {
			num, err := strconv.Atoi(stat[2:])
			if err == nil && num >= 1 && num <= 4 {
				err = world.UpdateCreatureStat(targetCreature.ID, fmt.Sprintf("realm%d", num), val)
				if err != nil {
					return StatusDefault, err
				}
				ctx.WriteString("Realm set.\n")
			}
		} else {
			err := world.UpdateCreatureStat(targetCreature.ID, "race", val)
			if err != nil {
				return StatusDefault, err
			}
		}
	case 's':
		if stat == "str" {
			err := world.UpdateCreatureStat(targetCreature.ID, "strength", val)
			if err != nil {
				return StatusDefault, err
			}
		} else if strings.HasPrefix(stat, "s#") {
			num, err := strconv.Atoi(stat[2:])
			if err == nil && num >= 1 && num <= 64 {
				spellName := fmt.Sprintf("spell%d", num)
				hasSpell := creatureHasAnyFlag(targetCreature, spellName)
				var err error
				if hasSpell {
					err = world.UpdateCreatureProperty(targetCreature.ID, spellName, "false")
				} else {
					err = world.UpdateCreatureProperty(targetCreature.ID, spellName, "true")
				}
				if err != nil {
					return StatusDefault, err
				}

				if hasSpell {
					ctx.WriteString(fmt.Sprintf("Spell #%d off.\n", num))
				} else {
					ctx.WriteString(fmt.Sprintf("Spell #%d on.\n", num))
				}
			}
		}
	case 't':
		err := world.UpdateCreatureStat(targetCreature.ID, "thaco", val)
		if err != nil {
			return StatusDefault, err
		}
	}

	return StatusDefault, nil
}

func dmSetFindOnlinePlayer(ctx *Context, world DMSetWorld, name string) (model.Creature, bool) {
	_, creature, _, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return creature, ok
}

func dmSetCreatureOrdinal(resolved ResolvedCommand) int64 {
	if val := dmSetValueAt(resolved, 1); val > 0 {
		return int64(val)
	}
	return 1
}

func dmSetFindRoomCreature(world DMSetWorld, actor model.Creature, name string, ordinal int64) (model.Creature, bool) {
	if lister, ok := any(world).(dmRoomCreatureLister); ok {
		if room, found := lister.Room(actor.RoomID); found && len(room.CreatureIDs) > 0 {
			if creature, ok := dmSetFindRoomCreatureList(lister, actor, room, name, ordinal, true); ok {
				return creature, true
			}
			return dmSetFindRoomCreatureList(lister, actor, room, name, ordinal, false)
		}
	}
	return dmFindCreatureInRoom(world, actor.RoomID, name, ordinal)
}

func dmSetFindRoomCreatureList(world dmRoomCreatureLister, actor model.Creature, room model.Room, name string, ordinal int64, players bool) (model.Creature, bool) {
	nameLower := strings.ToLower(strings.TrimSpace(name))
	if len(nameLower) < 2 {
		return model.Creature{}, false
	}
	count := int(ordinal)
	if count < 1 {
		count = 1
	}
	seen := 0
	for _, creatureID := range room.CreatureIDs {
		creature, ok := world.Creature(creatureID)
		if !ok || creature.RoomID != room.ID || dmCreatureIsPlayer(creature) != players {
			continue
		}
		if !dmFindCrtVisibleForActor(actor, creature) || !dmCreatureLookupNameMatches(creature, nameLower) {
			continue
		}
		seen++
		if seen == count {
			return creature, true
		}
	}
	return model.Creature{}, false
}

func dmSetObj(ctx *Context, resolved ResolvedCommand, world DMSetWorld, creature model.Creature, detectInvisible bool) (Status, error) {
	if dmSetArgCount(resolved) < 3 {
		ctx.WriteString("Syntax: *set o <name> [<crt>] <ad|ar|dn|ds|dp|f|m|sm|s|t|v|wg|wr> [<value>]\n")
		return StatusDefault, nil
	}

	objName := dmSetArg(resolved, 1)
	optionIndex := 2
	valueIndex := 2
	var targetObject model.ObjectInstance
	var foundObj bool

	if dmSetArgCount(resolved) >= 4 {
		targetName := dmSetArg(resolved, 2)
		targetCreature, ok := dmSetFindObjectCreatureTarget(ctx, world, creature, targetName, dmSetObjectTargetOrdinal(resolved))
		if !ok {
			ctx.WriteString("Creature not found.\n")
			return StatusDefault, nil
		}
		if obj, ok := dmSetFindObjectOnCreature(world, targetCreature, objName, dmSetObjectOrdinal(resolved), detectInvisible); ok {
			targetObject = obj
			foundObj = true
		}
		optionIndex = 3
		valueIndex = 3
	} else {
		if obj, ok := dmSetFindActorOrRoomObject(world, creature, objName, dmSetObjectOrdinal(resolved), detectInvisible); ok {
			targetObject = obj
			foundObj = true
		}
	}

	if !foundObj {
		ctx.WriteString("Object not found.\n")
		return StatusDefault, nil
	}

	option := dmSetArg(resolved, optionIndex)
	val := dmSetValueAt(resolved, valueIndex)
	optionLower := strings.ToLower(option)

	switch optionLower[:1] {
	case "a":
		switch optionLower {
		case "ad":
			if err := world.UpdateObjectProperty(targetObject.ID, "adjustment", strconv.Itoa(val)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Adjustment set.\n")
		case "ar":
			if err := world.UpdateObjectProperty(targetObject.ID, "armor", strconv.Itoa(val)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Armor set.\n")
		default:
			ctx.WriteString("Invalid option.\n")
		}
	case "v":
		err := world.UpdateObjectProperty(targetObject.ID, "value", strconv.Itoa(val))
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("Value set.\n")
	case "f":
		var num int
		var err error
		if optionLower == "f" {
			num = val
		} else {
			if len(option) < 2 {
				return StatusDefault, nil
			}
			num, err = strconv.Atoi(option[1:])
			if err != nil {
				return StatusDefault, nil
			}
		}
		if num < 1 || num > 64 {
			return StatusDefault, nil
		}
		var flagName string
		if num-1 < len(objectFlagNamesList) {
			flagName = objectFlagNamesList[num-1]
		} else {
			flagName = strconv.Itoa(num)
		}

		hasFlag := hasObjectFlag(targetObject, flagName)
		var err2 error
		if tagger, ok := world.(interface {
			UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
		}); ok {
			if hasFlag {
				if _, tagErr := tagger.UpdateObjectTags(targetObject.ID, nil, objectFlagMatchingTags(targetObject, flagName)); tagErr != nil {
					return StatusDefault, tagErr
				}
			} else {
				if _, tagErr := tagger.UpdateObjectTags(targetObject.ID, []string{flagName}, nil); tagErr != nil {
					return StatusDefault, tagErr
				}
			}
		}
		if hasFlag {
			err2 = world.UpdateObjectProperty(targetObject.ID, flagName, "false")
			if err2 == nil {
				for _, key := range objectFlagMatchingPropertyKeys(targetObject, flagName) {
					if normalizeFlagName(key) == normalizeFlagName(flagName) {
						continue
					}
					if err := world.UpdateObjectProperty(targetObject.ID, key, ""); err != nil {
						return StatusDefault, err
					}
				}
				for key, value := range objectFlagContainerPropertyValuesWithoutFlag(targetObject, flagName) {
					if err := world.UpdateObjectProperty(targetObject.ID, key, value); err != nil {
						return StatusDefault, err
					}
				}
			}
		} else {
			err2 = world.UpdateObjectProperty(targetObject.ID, flagName, "true")
		}
		if err2 != nil {
			return StatusDefault, err2
		}

		objectName := objectDisplayName(world, targetObject)
		if hasFlag {
			ctx.WriteString(fmt.Sprintf("%s's flag #%d off.\n", objectName, num))
		} else {
			ctx.WriteString(fmt.Sprintf("%s's flag #%d on.\n", objectName, num))
		}
	case "d":
		switch optionLower {
		case "dn":
			if err := world.UpdateObjectProperty(targetObject.ID, "nDice", strconv.Itoa(val)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Dice # set.\n")
		case "ds":
			if err := world.UpdateObjectProperty(targetObject.ID, "sDice", strconv.Itoa(val)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Dice sides set.\n")
		case "dp":
			if err := world.UpdateObjectProperty(targetObject.ID, "pDice", strconv.Itoa(val)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Dice plus set.\n")
		default:
			ctx.WriteString("Invalid option.\n")
		}
	case "m":
		if err := world.UpdateObjectProperty(targetObject.ID, "magicPower", strconv.Itoa(val)); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("Magic power set.\n")
	case "s":
		if optionLower == "sm" {
			if err := world.UpdateObjectProperty(targetObject.ID, "shotsMax", strconv.Itoa(val)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Max shots set.\n")
		} else {
			if err := world.UpdateObjectProperty(targetObject.ID, "shotsCurrent", strconv.Itoa(val)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Current shots set.\n")
		}
	case "t":
		if val < 0 || val > 4 {
			ctx.WriteString("Invalid option.\n")
			break
		}
		if err := world.UpdateObjectProperty(targetObject.ID, "type", strconv.Itoa(val)); err != nil {
			return StatusDefault, err
		}
		switch val {
		case 0:
			ctx.WriteString("Object is a sharp weapon.\n")
		case 1:
			ctx.WriteString("Object is a thrust weapon.\n")
		case 2:
			ctx.WriteString("Object is a blunt weapon.\n")
		case 3:
			ctx.WriteString("Object is a pole weapon.\n")
		case 4:
			ctx.WriteString("Object is a missile weapon.\n")
		}
	case "w":
		switch optionLower {
		case "wg":
			if err := world.UpdateObjectProperty(targetObject.ID, "weight", strconv.Itoa(val)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Weight set.\n")
		case "wr":
			if err := world.UpdateObjectProperty(targetObject.ID, "wearFlag", strconv.Itoa(val)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("Wear location set.\n")
		default:
			ctx.WriteString("Invalid option.\n")
		}
	default:
		ctx.WriteString("Invalid option.\n")
	}

	return StatusDefault, nil
}

func dmSetObjectOrdinal(resolved ResolvedCommand) int64 {
	if val := dmSetValueAt(resolved, 1); val > 0 {
		return int64(val)
	}
	return 1
}

func dmSetObjectTargetOrdinal(resolved ResolvedCommand) int64 {
	if val := dmSetValueAt(resolved, 2); val > 0 {
		return int64(val)
	}
	return 1
}

func dmSetValueAt(resolved ResolvedCommand, index int) int {
	slot := index + 1
	if resolved.Parsed.Num > slot {
		return int(resolved.Parsed.Val[slot])
	}
	if index >= 0 && index < len(resolved.Values) {
		return int(resolved.Values[index])
	}
	return 0
}

func dmSetFindActorOrRoomObject(world DMSetWorld, creature model.Creature, name string, ordinal int64, detectInvisible bool) (model.ObjectInstance, bool) {
	if obj, found := dmFindObjInCreatureInventory(world, creature, name, ordinal, detectInvisible); found {
		return obj, true
	}
	if creature.RoomID.IsZero() {
		return model.ObjectInstance{}, false
	}
	room, ok := world.Room(creature.RoomID)
	if !ok {
		return model.ObjectInstance{}, false
	}
	return dmFindObjInRoom(world, room, name, ordinal, detectInvisible)
}

func dmSetFindObjectOnCreature(world DMSetWorld, creature model.Creature, name string, ordinal int64, detectInvisible bool) (model.ObjectInstance, bool) {
	if obj, found := dmFindObjInCreatureInventory(world, creature, name, ordinal, detectInvisible); found {
		return obj, true
	}
	if obj, found := dmFindReadyObjOnCreature(world, creature, name, ordinal); found {
		return obj, true
	}
	return model.ObjectInstance{}, false
}

func dmSetFindObjectCreatureTarget(ctx *Context, world DMSetWorld, actor model.Creature, name string, ordinal int64) (model.Creature, bool) {
	if creature, ok := dmSetFindOnlinePlayer(ctx, world, legacyLowercizeASCII(name, true)); ok {
		return creature, true
	}
	if lister, ok := any(world).(dmRoomCreatureLister); ok {
		if room, found := lister.Room(actor.RoomID); found && len(room.CreatureIDs) > 0 {
			if creature, ok := dmFindMonsterInRoomForActor(world, actor, room.ID, name, ordinal); ok {
				return creature, true
			}
			return dmSetFindRoomCreatureList(lister, actor, room, name, ordinal, true)
		}
	}
	return dmFindCreatureInRoom(world, actor.RoomID, name, ordinal)
}

func hasObjectFlag(obj model.ObjectInstance, flagName string) bool {
	if hasAnyNormalizedFlag(obj.Metadata.Tags, flagName) {
		return true
	}
	targets := normalizedFlagSet(flagName)
	for key, val := range obj.Properties {
		if _, ok := targets[normalizeFlagName(key)]; ok && propertyFlagEnabled(val) {
			return true
		}
		if objectFlagContainerProperty(key) && propertyFlagValueHasAnyToken(val, targets) {
			return true
		}
	}
	return false
}

func objectFlagMatchingTags(obj model.ObjectInstance, flagName string) []string {
	targets := normalizedFlagSet(flagName)
	remove := make([]string, 0, len(obj.Metadata.Tags)+1)
	seen := make(map[string]struct{}, len(obj.Metadata.Tags)+1)
	for _, tag := range obj.Metadata.Tags {
		if _, ok := targets[normalizeFlagName(tag)]; ok {
			remove = append(remove, tag)
			seen[normalizeFlagName(tag)] = struct{}{}
		}
	}
	if _, ok := seen[normalizeFlagName(flagName)]; !ok {
		remove = append(remove, flagName)
	}
	return remove
}

func objectFlagMatchingPropertyKeys(obj model.ObjectInstance, flagName string) []string {
	targets := normalizedFlagSet(flagName)
	keys := make([]string, 0, len(obj.Properties))
	for key := range obj.Properties {
		if _, ok := targets[normalizeFlagName(key)]; ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func objectFlagContainerPropertyValuesWithoutFlag(obj model.ObjectInstance, flagName string) map[string]string {
	targets := normalizedFlagSet(flagName)
	updates := make(map[string]string)
	for key, value := range obj.Properties {
		if !objectFlagContainerProperty(key) {
			continue
		}
		kept := make([]string, 0)
		removed := false
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || r == ' '
		}) {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if _, ok := targets[normalizeFlagName(token)]; ok {
				removed = true
				continue
			}
			kept = append(kept, token)
		}
		if removed {
			updates[key] = strings.Join(kept, ",")
		}
	}
	return updates
}

func isAllowedDMName(name string) bool {
	names := []string{"무한", "미지", "미르", "클론", "", "̽ý", "Ŭ", "Muhan", "muhan"}
	for _, n := range names {
		if strings.EqualFold(name, n) {
			return true
		}
	}
	return false
}

func roomIDFromNumber(number int) model.RoomID {
	if number >= 0 {
		return model.RoomID(fmt.Sprintf("room:%05d", number))
	}
	return model.RoomID(fmt.Sprintf("room:%d", number))
}

func expandExitName(dir string) string {
	switch strings.ToLower(dir) {
	case "n", "north":
		return "북"
	case "s", "south":
		return "남"
	case "e", "east":
		return "동"
	case "w", "west":
		return "서"
	case "u", "up":
		return "위"
	case "d", "down":
		return "밑"
	case "o", "out":
		return "밖"
	case "i", "in":
		return "안"
	case "ne", "northeast":
		return "북동"
	case "nw", "northwest":
		return "북서"
	case "se", "southeast":
		return "남동"
	case "sw", "southwest":
		return "남서"
	default:
		return dir
	}
}

func oppositeExitName(dir string) (string, bool) {
	switch strings.ToLower(dir) {
	case "북", "n", "north":
		return "남", true
	case "남", "s", "south":
		return "북", true
	case "동", "e", "east":
		return "서", true
	case "서", "w", "west":
		return "동", true
	case "위", "u", "up":
		return "밑", true
	case "밑", "d", "down":
		return "위", true
	case "밖", "out":
		return "안", true
	case "안", "in":
		return "밖", true
	}
	return "", false
}
