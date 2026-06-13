package game

import (
	"fmt"
	"strconv"
	"strings"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/session"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type CallWarCommandWorld interface {
	FamilyWarWorld
	FamilyWorld
}

func NewCallWarHandler(world CallWarCommandWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			ctx.WriteString("현재 접속자 목록을 확인할 수 없어 패거리 전쟁을 시작할 수 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			ctx.WriteString("현재 접속자에게 알릴 수 없어 패거리 전쟁을 시작할 수 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}

		callerFamily, ok := callWarActorAuthority(world, model.PlayerID(ctx.ActorID))
		if !ok {
			ctx.WriteString("당신은 선전 포고할 권리가 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		targetName := ""
		if len(resolved.Args) == 1 {
			targetName = strings.TrimSpace(resolved.Args[0])
		}
		if targetName == "" {
			ctx.WriteString("어느 패거리와 전쟁을 하시려고요?")
			return enginecmd.StatusDefault, nil
		}
		sessions := active()
		targetFamily, ok := resolveCallWarTargetFamily(world, sessions, targetName)
		if !ok {
			ctx.WriteString("그런 패거리는 없습니다.")
			return enginecmd.StatusDefault, nil
		}
		if callerFamily == targetFamily {
			ctx.WriteString("자기 자신들과 싸우시려고요?")
			return enginecmd.StatusDefault, nil
		}
		if _, _, ok := findOnlineFamilyBoss(world, sessions, targetFamily); !ok {
			ctx.WriteString(fmt.Sprintf("상대편의 두목인 %s님이 이용중이 아닙니다.", callWarBossName(world, targetFamily)))
			return enginecmd.StatusDefault, nil
		}

		result, err := ResolveCallWarState(world, callerFamily, targetFamily)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		message := callWarResultMessage(world, result, callerFamily, targetFamily)
		if message == "" {
			return enginecmd.StatusDefault, nil
		}
		if !callWarResultBroadcasts(result.Transition) {
			ctx.WriteString(message)
			return enginecmd.StatusDefault, nil
		}
		return enginecmd.StatusDefault, broadcastCallWarMessage(ctx, sessions, send, message)
	}
}

func callWarActorAuthority(world FamilyWorld, playerID model.PlayerID) (int, bool) {
	creature, ok := playerCreature(world, playerID)
	if !ok {
		return 0, false
	}
	if !creatureHasNormalizedFlag(creature, "familyFlag", "PFAMIL") {
		return 0, false
	}
	if !creatureHasNormalizedFlag(creature, "PFMBOS", "familyBoss", "familyBossFlag") {
		return 0, false
	}
	familyID, ok := creatureNormalizedInt(creature, "familyID", "dailyExpndMax", "legacyDailyExpndMax")
	return familyID, ok && familyID > 0
}

func resolveCallWarTargetFamily(world FamilyWorld, sessions []ActiveSession, target string) (int, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return 0, false
	}
	if familyID, ok := parseCallWarFamilyName(target); ok {
		return familyID, true
	}
	if familyID, ok := familyIDFromDisplayName(world, target); ok {
		return familyID, true
	}
	if familyID, ok := resolveActiveFamilyDisplayName(world, sessions, target); ok {
		return familyID, true
	}
	active, _, ok := findActivePlayerSession(world, sessions, target)
	if !ok {
		return 0, false
	}
	creature, ok := playerCreature(world, model.PlayerID(active.ActorID))
	if !ok {
		return 0, false
	}
	if !creatureHasNormalizedFlag(creature, "familyFlag", "PFAMIL") {
		return 0, false
	}
	familyID, ok := creatureNormalizedInt(creature, "familyID", "dailyExpndMax", "legacyDailyExpndMax")
	return familyID, ok && familyID > 0
}

func parseCallWarFamilyName(name string) (int, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return 0, false
	}
	if value, err := strconv.Atoi(trimmed); err == nil && value > 0 {
		return value, true
	}
	for _, prefix := range []string{"패거리", "family"} {
		if !strings.HasPrefix(strings.ToLower(trimmed), prefix) {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(trimmed[len(prefix):]))
		if err == nil && value > 0 {
			return value, true
		}
	}
	return 0, false
}

func resolveActiveFamilyDisplayName(world FamilyWorld, sessions []ActiveSession, target string) (int, bool) {
	for _, activeSession := range sessions {
		if activeSession.ActorID == "" {
			continue
		}
		creature, ok := playerCreature(world, model.PlayerID(activeSession.ActorID))
		if !ok || !creatureHasNormalizedFlag(creature, "familyFlag", "PFAMIL") {
			continue
		}
		familyID, ok := creatureNormalizedInt(creature, "familyID", "dailyExpndMax", "legacyDailyExpndMax")
		if !ok || familyID <= 0 {
			continue
		}
		if target == familyDisplayNameFrom(world, familyID) || target == familyDisplayNameFromCreature(creature) {
			return familyID, true
		}
	}
	return 0, false
}

func findOnlineFamilyBoss(world FamilyWorld, sessions []ActiveSession, familyID int) (ActiveSession, string, bool) {
	if familyID <= 0 {
		return ActiveSession{}, "", false
	}
	configuredBoss, hasConfiguredBoss := familyBossNameFrom(world, familyID)
	for _, activeSession := range sessions {
		if activeSession.ActorID == "" {
			continue
		}
		displayName := playerDisplayName(world, activeSession.ActorID)
		if hasConfiguredBoss && configuredBoss != displayName && configuredBoss != activeSession.ActorID {
			continue
		}
		creature, ok := playerCreature(world, model.PlayerID(activeSession.ActorID))
		if !ok {
			continue
		}
		if !creatureHasNormalizedFlag(creature, "familyFlag", "PFAMIL") ||
			!creatureHasNormalizedFlag(creature, "PFMBOS", "familyBoss", "familyBossFlag") {
			continue
		}
		activeFamily, ok := creatureNormalizedInt(creature, "familyID", "dailyExpndMax", "legacyDailyExpndMax")
		if ok && activeFamily == familyID {
			return activeSession, displayName, true
		}
	}
	return ActiveSession{}, "", false
}

func callWarBossName(world any, familyID int) string {
	if name, ok := familyBossNameFrom(world, familyID); ok {
		return name
	}
	return familyDisplayNameFrom(world, familyID)
}

func callWarResultMessage(world any, result CallWarResult, callerFamily, targetFamily int) string {
	callerName := familyDisplayNameFrom(world, callerFamily)
	targetName := familyDisplayNameFrom(world, targetFamily)
	switch result.Transition {
	case CallWarRequested:
		return fmt.Sprintf("\n### %s 패거리가 %s에게 선전포고를 합니다.\n\n", callerName, targetName)
	case CallWarCanceled:
		return fmt.Sprintf("\n### %s 패거리에서 선전포고를 취소합니다.\n", callerName)
	case CallWarAccepted:
		return fmt.Sprintf("\n### %s 패거리에서 선전포고를 받아들였습니다.\n", callerName)
	case CallWarRejectedActive:
		return "벌써 전쟁중입니다.\n"
	case CallWarRejectedPending:
		if result.Snapshot.Pending.Second == callerFamily {
			return "다른 패거리에서 전쟁을 신청해두고 있습니다."
		}
		return "다른 패거리에서 먼저 전쟁을 준비중입니다."
	default:
		return ""
	}
}

func familyDisplayNameFromCreature(creature model.Creature) string {
	keys := normalizedWarNames("familyDisplayName", "familyName", "legacyFamilyName", "family_str")
	for key, raw := range creature.Properties {
		if _, ok := keys[normalizeWarName(key)]; ok {
			if value := strings.TrimSpace(raw); value != "" {
				return value
			}
		}
	}
	for key, raw := range creature.Metadata.RawFields {
		if _, ok := keys[normalizeWarName(key)]; ok {
			if value := strings.TrimSpace(string(raw)); value != "" {
				return value
			}
		}
	}
	for _, note := range creature.Metadata.Notes {
		key, value, ok := strings.Cut(note, "=")
		if ok {
			_, ok = keys[normalizeWarName(key)]
		}
		if ok {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func callWarResultBroadcasts(transition CallWarTransition) bool {
	switch transition {
	case CallWarRequested, CallWarCanceled, CallWarAccepted:
		return true
	default:
		return false
	}
}

func broadcastCallWarMessage(ctx *enginecmd.Context, sessions []ActiveSession, send func(session.ID, session.Command) error, message string) error {
	wroteSelf := false
	for _, activeSession := range sessions {
		if activeSession.ActorID == "" {
			continue
		}
		if string(activeSession.ID) == ctx.SessionID {
			ctx.WriteString(message)
			wroteSelf = true
			continue
		}
		if err := send(activeSession.ID, session.Command{Write: message}); err != nil {
			return err
		}
	}
	if !wroteSelf {
		ctx.WriteString(message)
	}
	return nil
}

func creatureHasNormalizedFlag(creature model.Creature, names ...string) bool {
	return creatureHasAnyFlag(creature, names...)
}

func creatureNormalizedInt(creature model.Creature, names ...string) (int, bool) {
	targets := normalizedWarNames(names...)
	for key, value := range creature.Stats {
		if _, ok := targets[normalizeWarName(key)]; ok {
			return value, true
		}
	}
	for key, raw := range creature.Properties {
		if _, ok := targets[normalizeWarName(key)]; ok {
			return parseSocialInt(raw)
		}
	}
	return 0, false
}

func normalizedWarNames(names ...string) map[string]struct{} {
	targets := make(map[string]struct{}, len(names))
	for _, name := range names {
		if normalized := normalizeWarName(name); normalized != "" {
			targets[normalized] = struct{}{}
		}
	}
	return targets
}

func normalizeWarName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, " ", "")
	return name
}
