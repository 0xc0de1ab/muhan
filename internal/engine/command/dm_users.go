package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

// DMUsersWorld is the required interface for the dm_users command.
type DMUsersWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
}

// NewDMUsersHandler creates a new Handler for the dm_users command.
func NewDMUsersHandler(world DMUsersWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmUsers(ctx, resolved, world)
	}
}

func dmUsers(ctx *Context, resolved ResolvedCommand, world DMUsersWorld) (Status, error) {
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

	// 1. Enforce SUB_DM (12+) permission.
	class := creatureClass(creature)
	if class < legacyClassSubDM {
		return StatusPrompt, nil
	}

	// 2. Parse argument: 'u' for UserID mode, 'f' for Full/Email mode.
	userIDMode := false
	fullUserMode := false
	if arg := dmUsersOptionArg(resolved); arg != "" {
		if arg[0] == 'u' {
			userIDMode = true
		} else if arg[0] == 'f' {
			fullUserMode = true
		}
	}

	// 3. Build and print the header in bold blue.
	opts := textOptionsFromContext(ctx)
	var colHeader string
	if fullUserMode {
		colHeader = fmt.Sprintf("%-9s %-10s %-52s Idle\n", "Lev  Clas", " Player", " Email address")
	} else {
		headerCol := "Address"
		if userIDMode {
			headerCol = "UserID"
		}
		colHeader = fmt.Sprintf("%-9s %-10s %-20s %-15s %-15s Idle\n", "Lev  Clas", " Player", "Room #: Name", headerCol, "Last command")
	}
	dashes := "-------------------------------------------------------------------------------\n"
	headerText := colHeader + dashes
	ctx.WriteString(colorText(opts, "34", headerText))

	// 4. Loop through all active players, filter invisible DMs from lower caster ranks.
	sessions := getActiveSessions(ctx)

	type activeInfo struct {
		player   model.Player
		creature model.Creature
		session  activeSession
	}
	var list []activeInfo
	seenActors := make(map[string]bool)

	for _, s := range sessions {
		if s.ActorID == "" {
			continue
		}
		if seenActors[s.ActorID] {
			continue
		}
		seenActors[s.ActorID] = true

		pID := model.PlayerID(s.ActorID)
		p, ok := world.Player(pID)
		if !ok {
			continue
		}
		c, ok := world.Creature(p.CreatureID)
		if !ok {
			continue
		}

		// Filter invisible DMs from lower caster ranks.
		targetClass := creatureClass(c)
		casterClass := class
		if targetClass == legacyClassDM && casterClass < legacyClassSubDM && creatureHasAnyFlag(c, "PDMINV", "dmInvisible") {
			continue
		}

		list = append(list, activeInfo{
			player:   p,
			creature: c,
			session:  s,
		})
	}

	now := time.Now().Unix()
	if override, ok := ctx.Values["test.now"].(int64); ok {
		now = override
	}

	// 5. Print columns according to mode with exact C alignments and colors.
	for _, info := range list {
		lvl := info.creature.Level
		if lvl <= 0 {
			if v, ok := creatureStatValue(info.creature, "level"); ok {
				lvl = v
			}
		}

		targetClassVal := creatureClass(info.creature)
		classStr := formatClass(targetClassVal)

		hasInvis := creatureHasAnyFlag(info.creature, "PDMINV", "dmInvisible") || creatureHasAnyFlag(info.creature, "PINVIS", "invisible")
		invisChar := " "
		if hasInvis {
			invisChar = "*"
		}

		name := playerName(info.player, info.creature)

		// Idle time
		ltime := getLTime(info.player, info.creature, now)
		idleSecs := now - ltime
		if idleSecs < 0 {
			idleSecs = 0
		}
		idleMins := idleSecs / 60
		idleSecs = idleSecs % 60

		part1 := fmt.Sprintf("[%2d] ", lvl)
		part2 := alignCP949(classStr, 4, true) + " "
		part3 := colorText(opts, "33", invisChar+alignCP949(name, 10, true)) + " "

		if fullUserMode {
			idstr := getUserID(info.player, info.creature) + "@" + getAddress(info.player, info.creature)
			part4 := alignCP949(idstr, 51, true) + " "
			part5 := fmt.Sprintf("%02d:%02d\n", idleMins, idleSecs)
			ctx.WriteString(part1 + part2 + part3 + part4 + part5)
		} else {
			roomNum := parseRoomNumber(info.creature.RoomID)
			roomNumStr := fmt.Sprintf("%5d: ", roomNum)

			roomName := "unknown"
			if room, ok := world.Room(info.creature.RoomID); ok {
				roomName = room.DisplayName
			}
			part5 := colorText(opts, "34", alignCP949(roomName, 12, true)) + " "

			addrOrUserID := getAddress(info.player, info.creature)
			if userIDMode {
				addrOrUserID = getUserID(info.player, info.creature)
			}
			part6 := colorText(opts, "36", alignCP949(addrOrUserID, 15, true)) + " "

			lastCmd := getLastCommand(info.player, info.creature)
			part7 := colorText(opts, "32", alignCP949(lastCmd, 15, true)) + " "

			part8 := fmt.Sprintf("%02d:%02d\n", idleMins, idleSecs)
			ctx.WriteString(part1 + part2 + part3 + roomNumStr + part5 + part6 + part7 + part8)
		}
	}

	if opts.ANSI {
		ctx.WriteString("\x1b[0m\n")
	} else {
		ctx.WriteString("\n")
	}

	return StatusDefault, nil
}

func dmUsersOptionArg(resolved ResolvedCommand) string {
	if resolved.Parsed.Num > 0 {
		if resolved.Parsed.Num > 1 {
			return resolved.Parsed.Str[1]
		}
		return ""
	}
	if len(resolved.Args) > 0 {
		return resolved.Args[0]
	}
	return ""
}

func formatClass(classVal int) string {
	classes := []string{
		"바보", "자객", "권법가", "불제자",
		"검사", "도술사", "무사", "포졸",
		"도둑", "무적", "초인", "불사",
		"운영자", "관리자",
	}
	if classVal < 0 || classVal >= len(classes) {
		return "    "
	}
	className := classes[classVal]
	b, err := legacykr.EncodeEUCKR(className)
	if err != nil {
		runes := []rune(className)
		if len(runes) > 2 {
			runes = runes[:2]
		}
		return fmt.Sprintf("%-4s", string(runes))
	}
	if len(b) > 4 {
		b = b[:4]
	}
	decoded, err := legacykr.DecodeEUCKR(b)
	if err != nil {
		runes := []rune(className)
		if len(runes) > 2 {
			runes = runes[:2]
		}
		return fmt.Sprintf("%-4s", string(runes))
	}
	return decoded
}

func playerName(player model.Player, creature model.Creature) string {
	if name := strings.TrimSpace(creature.DisplayName); name != "" {
		return name
	}
	if name := strings.TrimSpace(player.DisplayName); name != "" {
		return name
	}
	return string(player.ID)
}

func getUserID(player model.Player, creature model.Creature) string {
	if val, ok := creature.Properties["userid"]; ok {
		return val
	}
	if raw, ok := player.Metadata.RawFields["userid"]; ok {
		return string(raw)
	}
	if player.AccountName != "" {
		return player.AccountName
	}
	return "unknown"
}

func getAddress(player model.Player, creature model.Creature) string {
	if val, ok := creature.Properties["address"]; ok {
		return val
	}
	if raw, ok := player.Metadata.RawFields["address"]; ok {
		return string(raw)
	}
	return "127.0.0.1"
}

func getLastCommand(player model.Player, creature model.Creature) string {
	if val, ok := creature.Properties["lastCommand"]; ok {
		return val
	}
	if val, ok := creature.Properties["last_command"]; ok {
		return val
	}
	if raw, ok := player.Metadata.RawFields["lastCommand"]; ok {
		return string(raw)
	}
	return "N/A"
}

func getLTime(player model.Player, creature model.Creature, now int64) int64 {
	if val, ok := creature.Properties["ltime"]; ok {
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
			return parsed
		}
	}
	if val, ok := creature.Stats["ltime"]; ok {
		return int64(val)
	}
	if raw, ok := player.Metadata.RawFields["ltime"]; ok {
		if parsed, err := strconv.ParseInt(string(raw), 10, 64); err == nil {
			return parsed
		}
	}
	return now
}

func alignCP949(s string, width int, leftAlign bool) string {
	b, err := legacykr.EncodeEUCKR(s)
	if err != nil {
		// Fallback
		padding := width - len(s)
		if padding > 0 {
			padStr := strings.Repeat(" ", padding)
			if leftAlign {
				return s + padStr
			}
			return padStr + s
		}
		return s
	}

	padding := width - len(b)
	if padding > 0 {
		padStr := strings.Repeat(" ", padding)
		if leftAlign {
			b = append(b, []byte(padStr)...)
		} else {
			b = append([]byte(padStr), b...)
		}
	}

	decoded, _ := legacykr.DecodeEUCKR(b)
	return decoded
}
