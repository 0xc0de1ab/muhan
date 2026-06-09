package game

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/krtext"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

const (
	legacyClassCaretaker = 10
	legacyClassSubDM     = 12
	legacyClassDM        = 13
)

func NewPfingerHandler(world WhoisWorld, root string) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		if len(resolved.Args) == 0 || strings.TrimSpace(resolved.Args[0]) == "" {
			ctx.WriteString("누구의 정보를 보고 싶으세요?\n")
			return enginecmd.StatusDefault, nil
		}

		actorCreature, _ := playerCreature(world, model.PlayerID(ctx.ActorID))
		targetName := legacyPfingerLookupName(resolved.Args[0])
		target, activeName, activeOK := findPfingerActivePlayer(world, active(), targetName)
		if activeOK {
			player, ok := world.Player(model.PlayerID(target.ActorID))
			if !ok || player.CreatureID.IsZero() {
				ctx.WriteString("그런 사용자는 없습니다.\n")
				return enginecmd.StatusDefault, nil
			}
			creature, ok := world.Creature(player.CreatureID)
			if !ok {
				ctx.WriteString("그런 사용자는 없습니다.\n")
				return enginecmd.StatusDefault, nil
			}
			if !pfingerCanSeeActive(actorCreature, creature) {
				ctx.WriteString("당신은 그 사용자의 정보를 볼 수 없습니다.\n")
				return enginecmd.StatusDefault, nil
			}
			if strings.TrimSpace(creature.DisplayName) != "" {
				activeName = strings.TrimSpace(creature.DisplayName)
			}
			ctx.WriteString(renderPfingerIdentity(activeName, creature))
			ctx.WriteString("현재 접속 중 입니다.\n")
			ctx.WriteString(renderPfingerPostStatus(root, targetName))
			return enginecmd.StatusDefault, nil
		}

		player, creature, ok := pfingerOfflinePlayer(world, targetName)
		if !ok {
			ctx.WriteString("그런 사용자는 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		if !pfingerCanSeeOffline(actorCreature, creature) {
			ctx.WriteString("당신은 그 사용자의 정보를 볼 수 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		playerPath, ok := pfingerPlayerFilePath(root, targetName, player)
		if !ok {
			ctx.WriteString("그런 사용자는 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		fileTimes, err := statLegacyTimes(playerPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				ctx.WriteString("그런 사용자는 없습니다.\n")
				return enginecmd.StatusDefault, nil
			}
			return enginecmd.StatusDefault, err
		}
		name := strings.TrimSpace(creature.DisplayName)
		if name == "" {
			name = targetName
		}
		ctx.WriteString(renderPfingerIdentity(name, creature))
		ctx.WriteString("마지막 접속시간: " + formatLegacyCTime(fileTimes.Change))
		if creatureFlagEnabled(creature, "SUICD", "suicide") {
			ctx.WriteString("그 사용자는 자살신청한 사용자입니다.\n")
		}
		ctx.WriteString(renderPfingerPostStatus(root, targetName))
		return enginecmd.StatusDefault, nil
	}
}

func findPfingerActivePlayer(world PlayerLookup, sessions []ActiveSession, target string) (ActiveSession, string, bool) {
	target = strings.TrimSpace(target)
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

func legacyPfingerLookupName(name string) string {
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

func pfingerOfflinePlayer(world WhoisWorld, name string) (model.Player, model.Creature, bool) {
	if world == nil || strings.TrimSpace(name) == "" {
		return model.Player{}, model.Creature{}, false
	}
	player, ok := world.Player(model.PlayerID(name))
	if !ok || player.CreatureID.IsZero() {
		return model.Player{}, model.Creature{}, false
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return model.Player{}, model.Creature{}, false
	}
	return player, creature, true
}

func pfingerCanSeeOffline(actor model.Creature, target model.Creature) bool {
	return creatureIntValueDefault(actor, "class", 0) >= legacyClassDM ||
		creatureIntValueDefault(target, "class", 0) < legacyClassSubDM
}

func pfingerCanSeeActive(actor model.Creature, target model.Creature) bool {
	if !creatureFlagEnabled(target, "PDMINV", "dmInvisible") {
		return true
	}
	actorClass := creatureIntValueDefault(actor, "class", 0)
	targetClass := creatureIntValueDefault(target, "class", 0)
	if actorClass < legacyClassDM {
		return false
	}
	return !(actorClass == legacyClassCaretaker && targetClass == legacyClassDM)
}

func renderPfingerIdentity(name string, creature model.Creature) string {
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSpace(creature.DisplayName)
	}
	if name == "" {
		name = string(creature.ID)
	}
	return fmt.Sprintf(
		"%s %+25s %s\n",
		name,
		whoisRaceName(creatureIntValueDefault(creature, "race", 0)),
		whoisClassName(creatureIntValueDefault(creature, "class", 0)),
	)
}

func renderPfingerPostStatus(root string, name string) string {
	postPath, ok := pfingerPostFilePath(root, name)
	if !ok {
		return "받은 편지가 없습니다.\n"
	}
	fileTimes, err := statLegacyTimes(postPath)
	if err != nil {
		return "받은 편지가 없습니다.\n"
	}
	if fileTimes.Access.After(fileTimes.Change) {
		return "읽지 않은 편지가 도착한 날짜: " + formatLegacyCTime(fileTimes.Access)
	}
	return "새 편지가 도착한 날짜: " + formatLegacyCTime(fileTimes.Change)
}

func pfingerPlayerFilePath(root string, name string, player model.Player) (string, bool) {
	for _, path := range pfingerPlayerFileCandidates(root, name, player) {
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func pfingerPostFilePath(root string, name string) (string, bool) {
	for _, path := range pfingerPostFileCandidates(root, name) {
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func pfingerPlayerFileCandidates(root string, name string, player model.Player) []string {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	var out []string
	rawName := player.Metadata.RawFields["filename"]
	rawShard := player.Metadata.RawFields["shard"]
	if len(rawName) != 0 && len(rawShard) != 0 {
		out = append(out, filepath.Join(root, "player", string(rawShard), string(rawName)))
	}
	if path := strings.TrimSpace(player.Metadata.LegacyPath); path != "" {
		out = append(out, filepath.Join(root, filepath.FromSlash(path)))
	}
	for _, candidateName := range pfingerEncodedNameCandidates(name, player) {
		out = append(out, filepath.Join(root, "player", krtext.FirstHangulBucket(name), candidateName))
	}
	return uniqueNonEmptyPaths(out)
}

func pfingerPostFileCandidates(root string, name string) []string {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	var out []string
	for _, candidateName := range pfingerLegacyNameCandidates(name) {
		out = append(out, filepath.Join(root, "post", candidateName))
	}
	return uniqueNonEmptyPaths(out)
}

func pfingerLegacyNameCandidates(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	names := []string{}
	if encoded, err := legacykr.EncodeEUCKR(name); err == nil {
		names = append(names, string(encoded))
	}
	names = append(names, name)
	return uniqueNonEmptyPaths(names)
}

func pfingerEncodedNameCandidates(name string, player model.Player) []string {
	names := []string{}
	if raw := player.Metadata.RawFields["filename"]; len(raw) != 0 {
		names = append(names, string(raw))
	}
	name = strings.TrimSpace(name)
	if name != "" {
		if encoded, err := legacykr.EncodeEUCKR(name); err == nil {
			names = append(names, string(encoded))
		}
		names = append(names, name)
	}
	if display := strings.TrimSpace(player.DisplayName); display != "" && display != name {
		if encoded, err := legacykr.EncodeEUCKR(display); err == nil {
			names = append(names, string(encoded))
		}
		names = append(names, display)
	}
	return uniqueNonEmptyPaths(names)
}

func uniqueNonEmptyPaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

type legacyFileTimes struct {
	Access time.Time
	Change time.Time
}

func statLegacyTimes(path string) (legacyFileTimes, error) {
	info, err := os.Stat(path)
	if err != nil {
		return legacyFileTimes{}, err
	}
	times := legacyFileTimes{Access: info.ModTime(), Change: info.ModTime()}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		times.Access = time.Unix(stat.Atim.Sec, stat.Atim.Nsec)
		times.Change = time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)
	}
	return times, nil
}

func formatLegacyCTime(t time.Time) string {
	if t.IsZero() {
		t = time.Unix(0, 0)
	}
	return t.Local().Format("Mon Jan _2 15:04:05 2006\n")
}
