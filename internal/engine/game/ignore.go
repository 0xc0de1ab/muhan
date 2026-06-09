package game

import (
	"fmt"
	"strings"
	"sync"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/world/model"
)

type IgnoreWorld interface {
	PlayerLookup
	Creature(model.CreatureID) (model.Creature, bool)
}

type IgnoreMemory struct {
	mu     sync.Mutex
	byUser map[string][]string
}

func NewIgnoreMemory() *IgnoreMemory {
	return &IgnoreMemory{byUser: map[string][]string{}}
}

func (m *IgnoreMemory) Add(ownerID, name string) bool {
	if m == nil {
		return false
	}
	ownerID = strings.TrimSpace(ownerID)
	name = legacyIgnoreLookupName(name)
	if ownerID == "" || name == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byUser == nil {
		m.byUser = map[string][]string{}
	}
	if ignoreListContains(m.byUser[ownerID], name) {
		return false
	}
	m.byUser[ownerID] = append([]string{name}, m.byUser[ownerID]...)
	return true
}

func (m *IgnoreMemory) Remove(ownerID, name string) bool {
	if m == nil {
		return false
	}
	ownerID = strings.TrimSpace(ownerID)
	name = legacyIgnoreLookupName(name)
	if ownerID == "" || name == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	names := m.byUser[ownerID]
	for i, existing := range names {
		if ignoreNameMatches(existing, name) {
			names = append(names[:i], names[i+1:]...)
			if len(names) == 0 {
				delete(m.byUser, ownerID)
			} else {
				m.byUser[ownerID] = names
			}
			return true
		}
	}
	return false
}

func (m *IgnoreMemory) List(ownerID string) []string {
	if m == nil {
		return nil
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	names := m.byUser[ownerID]
	out := make([]string, len(names))
	copy(out, names)
	return out
}

func (m *IgnoreMemory) Ignored(ownerID, name string) bool {
	if m == nil {
		return false
	}
	ownerID = strings.TrimSpace(ownerID)
	name = legacyIgnoreLookupName(name)
	if ownerID == "" || name == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return ignoreListContains(m.byUser[ownerID], name)
}

func NewIgnoreHandler(world IgnoreWorld, memory *IgnoreMemory) enginecmd.Handler {
	if memory == nil {
		memory = NewIgnoreMemory()
	}
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}

		ownerID := ignoreMemoryKey(ctx)
		targetName := ignoreTargetArg(resolved)
		if targetName == "" {
			writeIgnoreList(ctx, memory.List(ownerID))
			return enginecmd.StatusDefault, nil
		}

		if memory.Remove(ownerID, targetName) {
			ctx.WriteString(fmt.Sprintf("%s님을 이야기 듣기 거부 대상에서 삭제합니다.", targetName))
			return enginecmd.StatusDefault, nil
		}

		target, ok := findIgnoreTarget(ctx, world, targetName)
		if !ok {
			ctx.WriteString("그 사용자는 접속중이 아닙니다.\n")
			return enginecmd.StatusDefault, nil
		}

		memory.Add(ownerID, target.Name)
		ctx.WriteString(fmt.Sprintf("%s님을 이야기 듣기 거부 대상에 추가합니다.\n", target.Name))
		return enginecmd.StatusDefault, nil
	}
}

func ignoreMemoryKey(ctx *enginecmd.Context) string {
	if ctx == nil {
		return ""
	}
	if sessionID := strings.TrimSpace(ctx.SessionID); sessionID != "" {
		return sessionID
	}
	return strings.TrimSpace(ctx.ActorID)
}

type ignoreTarget struct {
	Name string
}

func ignoreTargetArg(resolved enginecmd.ResolvedCommand) string {
	if len(resolved.Args) == 0 {
		return ""
	}
	return legacyIgnoreLookupName(resolved.Args[0])
}

func writeIgnoreList(ctx *enginecmd.Context, names []string) {
	ctx.WriteString("듣기 거부된 사용자: ")
	if len(names) == 0 {
		ctx.WriteString("없음.\n")
		return
	}
	ctx.WriteString(strings.Join(names, ", "))
	ctx.WriteString(".\n")
}

func findIgnoreTarget(ctx *enginecmd.Context, world IgnoreWorld, name string) (ignoreTarget, bool) {
	name = legacyIgnoreLookupName(name)
	if name == "" || world == nil {
		return ignoreTarget{}, false
	}

	if active, ok := activeSessionsFunc(ctx); ok {
		if target, found := findIgnoreActiveTarget(world, active(), name); found {
			return target, true
		}
	}
	return ignoreTarget{}, false
}

func findIgnoreActiveTarget(world IgnoreWorld, sessions []ActiveSession, name string) (ignoreTarget, bool) {
	for _, activeSession := range sessions {
		if strings.TrimSpace(activeSession.ActorID) == "" {
			continue
		}
		player, ok := world.Player(model.PlayerID(activeSession.ActorID))
		if !ok {
			continue
		}
		displayName := ignorePlayerName(world, player)
		if !ignoreNameMatches(displayName, name) {
			continue
		}
		if ignorePlayerHidden(world, player) {
			return ignoreTarget{}, false
		}
		return ignoreTarget{Name: legacyIgnoreLookupName(displayName)}, true
	}
	return ignoreTarget{}, false
}

func ignorePlayerName(world IgnoreWorld, player model.Player) string {
	if name, ok := legacyLookupName(player.DisplayName, string(player.ID)); ok {
		return name
	}
	if world != nil && !player.CreatureID.IsZero() {
		if creature, ok := world.Creature(player.CreatureID); ok {
			if name, ok := legacyLookupName(creature.DisplayName, string(player.ID)); ok {
				return name
			}
		}
	}
	return ""
}

func ignorePlayerHidden(world IgnoreWorld, player model.Player) bool {
	if world == nil || player.CreatureID.IsZero() {
		return false
	}
	creature, ok := world.Creature(player.CreatureID)
	return ok && creatureFlagEnabled(creature, "PDMINV", "dmInvisible")
}

func ignoreListContains(names []string, target string) bool {
	for _, name := range names {
		if ignoreNameMatches(name, target) {
			return true
		}
	}
	return false
}

func ignoreNameMatches(candidate, target string) bool {
	return legacyIgnoreLookupName(candidate) == legacyIgnoreLookupName(target)
}

func legacyIgnoreLookupName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	bytes := []byte(name)
	if bytes[0] >= 'a' && bytes[0] <= 'z' {
		bytes[0] -= 'a' - 'A'
	}
	return string(bytes)
}
