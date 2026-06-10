package game

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/krtext"
	"muhan/internal/session"
	"muhan/internal/textfmt"
	"muhan/internal/world/model"
)

var (
	ErrSocialActorRequired  = errors.New("game: social actor required")
	ErrSocialContextMissing = errors.New("game: social context missing")
)

type PlayerLookup interface {
	Player(model.PlayerID) (model.Player, bool)
}

type ActionWorld interface {
	PlayerLookup
	Room(model.RoomID) (model.Room, bool)
	Creature(model.CreatureID) (model.Creature, bool)
}

type FamilyWorld interface {
	PlayerLookup
	Creature(model.CreatureID) (model.Creature, bool)
}

type TellMemory struct {
	mu                sync.Mutex
	lastSenderByActor map[string]string
}

func NewTellMemory() *TellMemory {
	return &TellMemory{lastSenderByActor: map[string]string{}}
}

func (m *TellMemory) Remember(recipientActorID, senderActorID string) {
	if m == nil || recipientActorID == "" || senderActorID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastSenderByActor == nil {
		m.lastSenderByActor = map[string]string{}
	}
	m.lastSenderByActor[recipientActorID] = senderActorID
}

func (m *TellMemory) LastSender(actorID string) (string, bool) {
	if m == nil || actorID == "" {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	last := strings.TrimSpace(m.lastSenderByActor[actorID])
	return last, last != ""
}

func NewWhoHandler(world PlayerLookup) enginecmd.Handler {
	return func(ctx *enginecmd.Context, _ enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}

		sessions := active()
		ctx.WriteString("접속자:\n")
		if len(sessions) == 0 {
			ctx.WriteString("없음\n")
			return enginecmd.StatusDefault, nil
		}
		for _, active := range sessions {
			if active.ActorID == "" {
				continue
			}
			ctx.WriteString(" - ")
			ctx.WriteString(playerDisplayName(world, active.ActorID))
			ctx.WriteString("\n")
		}
		ctx.WriteString(familyWarStatusLine(world))
		return enginecmd.StatusDefault, nil
	}
}

func NewTellHandler(world PlayerLookup, memory *TellMemory, ignores ...*IgnoreMemory) enginecmd.Handler {
	ignoreMemory := firstIgnoreMemory(ignores...)
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		if len(resolved.Args) == 0 || strings.TrimSpace(resolved.Args[0]) == "" {
			ctx.WriteString("누구에게 말을 전하시려구요?")
			return enginecmd.StatusDefault, nil
		}

		target, targetName, ok := findActivePlayerSession(world, active(), resolved.Args[0])
		if !ok {
			ctx.WriteString("누구에게 말을 전하시려구요?")
			return enginecmd.StatusDefault, nil
		}
		if socialActorBlocksTell(world, model.PlayerID(ctx.ActorID), model.PlayerID(target.ActorID)) {
			ctx.WriteString(fmt.Sprintf("%s님은 이야기 듣기 거부 상태입니다.", targetName))
			return enginecmd.StatusDefault, nil
		}
		if socialIgnoredBy(ignoreMemory, world, string(target.ID), target.ActorID, ctx.ActorID) {
			ctx.WriteString(fmt.Sprintf("%s is ignoring you.", targetName))
			return enginecmd.StatusDefault, nil
		}
		message := tellMessageFromResolved(resolved)
		if message == "" {
			ctx.WriteString("무슨 말을 전하시려구요?")
			return enginecmd.StatusDefault, nil
		}
		if socialActorSilenced(world, model.PlayerID(ctx.ActorID)) {
			ctx.WriteString("당신은 말을 할 수 없습니다.")
			return enginecmd.StatusDefault, nil
		}

		actorName := playerDisplayName(world, ctx.ActorID)
		actorCreature, actorOK := socialActorCreature(world, model.PlayerID(ctx.ActorID))
		if actorOK && creatureFlagEnabled(actorCreature, "PLECHO", "echo", "legacyEcho") {
			ctx.WriteString(fmt.Sprintf("당신은 %s에게 \"%s\"라고 이야기합니다.", targetName, message))
		} else {
			ctx.WriteString(fmt.Sprintf("%s님에게 말을 전달하였습니다.", targetName))
		}
		out := fmt.Sprintf("\n%s 당신에게 \"%s\"라고 이야기합니다.", actorSubject(actorName), message)
		if string(target.ID) == ctx.SessionID {
			ctx.WriteString(out)
		} else if err := send(target.ID, session.Command{Write: out}); err != nil {
			return enginecmd.StatusDefault, err
		}
		if memory != nil {
			memory.Remember(target.ActorID, ctx.ActorID)
		}
		return enginecmd.StatusDefault, nil
	}
}

func NewReplyHandler(world PlayerLookup, memory *TellMemory, ignores ...*IgnoreMemory) enginecmd.Handler {
	ignoreMemory := firstIgnoreMemory(ignores...)
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		if memory == nil {
			ctx.WriteString("누구에게 말을 전하시려구요?")
			return enginecmd.StatusDefault, nil
		}
		targetActorID, ok := memory.LastSender(ctx.ActorID)
		if !ok {
			ctx.WriteString("누구에게 말을 전하시려구요?")
			return enginecmd.StatusDefault, nil
		}
		target, targetName, ok := findActivePlayerSessionByActor(world, active(), targetActorID)
		if !ok {
			ctx.WriteString("누구에게 말을 전하시려구요?")
			return enginecmd.StatusDefault, nil
		}
		if socialReplyTargetHiddenFromActor(world, model.PlayerID(ctx.ActorID), model.PlayerID(target.ActorID)) {
			ctx.WriteString("누구에게 말을 전하시려구요?")
			return enginecmd.StatusDefault, nil
		}
		if socialActorBlocksTell(world, model.PlayerID(ctx.ActorID), model.PlayerID(target.ActorID)) {
			ctx.WriteString(fmt.Sprintf("%s님은 이야기 듣기 거부 상태입니다.", targetName))
			return enginecmd.StatusDefault, nil
		}
		if socialIgnoredBy(ignoreMemory, world, string(target.ID), target.ActorID, ctx.ActorID) {
			ctx.WriteString(fmt.Sprintf("%s님은 이야기 거부중입니다.", targetName))
			return enginecmd.StatusDefault, nil
		}
		message := replyMessageFromResolved(resolved)
		if message == "" {
			ctx.WriteString("무슨 말을 전하시려구요?")
			return enginecmd.StatusDefault, nil
		}
		if socialActorSilenced(world, model.PlayerID(ctx.ActorID)) {
			ctx.WriteString("당신은 말을 할 수 없습니다.")
			return enginecmd.StatusDefault, nil
		}

		actorName := playerDisplayName(world, ctx.ActorID)
		actorCreature, actorOK := socialActorCreature(world, model.PlayerID(ctx.ActorID))
		if actorOK && creatureFlagEnabled(actorCreature, "PLECHO", "echo", "legacyEcho") {
			ctx.WriteString(fmt.Sprintf("당신은 %s에게 \"%s\"라고 대답합니다.", targetName, message))
		} else {
			ctx.WriteString(fmt.Sprintf("%s님에게 말을 전하였습니다.", targetName))
		}
		out := fmt.Sprintf("\n%s 당신에게 \"%s\"라고 대답합니다.", actorSubject(actorName), message)
		if string(target.ID) == ctx.SessionID {
			ctx.WriteString(out)
		} else if err := send(target.ID, session.Command{Write: out}); err != nil {
			return enginecmd.StatusDefault, err
		}
		memory.Remember(target.ActorID, ctx.ActorID)
		return enginecmd.StatusDefault, nil
	}
}

func NewSayHandler(world PlayerLookup) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}

		message := sayMessageFromResolved(resolved)
		if message == "" {
			ctx.WriteString("뭘 말하고 싶으세요?")
			return enginecmd.StatusDefault, nil
		}
		actorCreature, actorOK := socialActorCreature(world, model.PlayerID(ctx.ActorID))
		if actorOK && creatureFlagEnabled(actorCreature, "PSILNC", "silenced", "silence") {
			ctx.WriteString("말을 해 보았지만 이 방 밖의 사람들은 들리지 않는듯 하군요.")
			return enginecmd.StatusDefault, nil
		}
		if err := revealSocialActor(world, model.PlayerID(ctx.ActorID)); err != nil {
			return enginecmd.StatusDefault, err
		}

		speaker := model.PlayerID(ctx.ActorID)
		speakerRoom := playerRoomID(world, speaker)
		speakerName := playerDisplayName(world, ctx.ActorID)
		if actorOK && creatureFlagEnabled(actorCreature, "PLECHO", "echo", "legacyEcho") {
			ctx.WriteString(fmt.Sprintf("당신은 \"%s\"라고 말합니다.", message))
		} else {
			ctx.WriteString("예. 좋습니다.")
		}

		for _, target := range active() {
			if string(target.ID) == ctx.SessionID || target.ActorID == "" {
				continue
			}
			if speakerRoom != "" && playerRoomID(world, model.PlayerID(target.ActorID)) != speakerRoom {
				continue
			}
			if err := send(target.ID, session.Command{Write: fmt.Sprintf("\n%s \"%s\"라고 말합니다.", actorSubject(speakerName), message)}); err != nil {
				return enginecmd.StatusDefault, err
			}
		}
		return enginecmd.StatusDefault, nil
	}
}

func NewBroadcastChatHandler(world PlayerLookup) enginecmd.Handler {
	memory := newBroadcastSocialMemory()
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		return broadcastSocialLine(ctx, resolved, world, memory, broadcastSocialConfig{
			silencedMessage:          "당신의 목소리가 너무 작아 잡담을 할 수 없습니다.",
			lowLevelMessage:          "당신의 레벨로는 잡담을 할 수 없습니다.",
			lowHPMessage:             "당신의 목숨이 위태로워 잡담을 할 수 없습니다.",
			duplicateMessage:         "\n도배하지 마세요.\n",
			duplicateBeforeStatus:    false,
			duplicateLimitSeconds:    5,
			restrictionClock:         time.Now,
			legacyBroadcastFormatter: func(name, message string) string { return fmt.Sprintf("\n{녹%s> %s}", name, message) },
		})
	}
}

func NewCheerHandler(world PlayerLookup) enginecmd.Handler {
	memory := newBroadcastSocialMemory()
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		return broadcastSocialLine(ctx, resolved, world, memory, broadcastSocialConfig{
			silencedMessage:       "당신의 목소리가 너무 작아 환호를 할 수 없습니다.",
			lowLevelMessage:       "당신의 레벨로는 환호를 할 수 없습니다.",
			lowHPMessage:          "당신의 목숨이 위태로워 환호를 할 수 없습니다.",
			duplicateMessage:      "\n도배하지 마세요\n",
			duplicateBeforeStatus: true,
			duplicateLimitSeconds: 5,
			restrictionClock:      time.Now,
			legacyBroadcastFormatter: func(name, message string) string {
				return fmt.Sprintf("\n{녹%s님이 \"%s\"라고 환호를 합니다.}", name, message)
			},
		})
	}
}

func NewFamilyTalkHandler(world FamilyWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}

		actorFamily, ok := playerFamilyMembership(world, model.PlayerID(ctx.ActorID))
		if !ok {
			ctx.WriteString("당신은 패거리에 속해있지 않습니다.")
			return enginecmd.StatusDefault, nil
		}
		actorCreature, _ := playerCreature(world, model.PlayerID(ctx.ActorID))
		if creatureFlagEnabled(actorCreature, "PSILNC", "silenced", "silence") {
			ctx.WriteString("입이 막혀 말이 나오질 않습니다.")
			return enginecmd.StatusDefault, nil
		}

		message := legacyCutCommandMessageFromResolved(resolved)
		if message == "" {
			ctx.WriteString("패거리원들에게 무슨 말을 하시려고요?")
			return enginecmd.StatusDefault, nil
		}

		actorName := playerDisplayName(world, ctx.ActorID)
		out := renderLegacyColorForContext(ctx, fmt.Sprintf("\n{노%s>>> %s}", actorName, message))
		for _, activeSession := range active() {
			if activeSession.ActorID == "" {
				continue
			}
			if targetFamily, ok := playerFamilyMembership(world, model.PlayerID(activeSession.ActorID)); !ok || targetFamily != actorFamily {
				continue
			}
			if string(activeSession.ID) == ctx.SessionID {
				ctx.WriteString(out)
				continue
			}
			if err := send(activeSession.ID, session.Command{Write: out}); err != nil {
				return enginecmd.StatusDefault, err
			}
		}
		return enginecmd.StatusDefault, nil
	}
}

func NewFamilyWhoHandler(world FamilyWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}

		actorCreature, actorOK := playerCreature(world, model.PlayerID(ctx.ActorID))
		if actorOK && creatureHasNormalizedFlag(actorCreature, "PBLIND", "blind") {
			ctx.WriteString("당신은 눈이 멀어 있습니다!")
			return enginecmd.StatusDefault, nil
		}

		if len(resolved.Args) > 0 && strings.TrimSpace(resolved.Args[0]) != "" {
			target, targetName, ok := findActivePlayerSession(world, active(), resolved.Args[0])
			if !ok {
				ctx.WriteString("현재 이용중이 아닙니다.")
				return enginecmd.StatusDefault, nil
			}
			targetCreature, targetOK := playerCreature(world, model.PlayerID(target.ActorID))
			if !targetOK || !familyWhoTargetVisible(actorCreature, actorOK, targetCreature) {
				ctx.WriteString("현재 이용중이 아닙니다.")
				return enginecmd.StatusDefault, nil
			}
			familyID, member, pending := playerFamilyState(world, model.PlayerID(target.ActorID))
			if !member && !pending {
				ctx.WriteString(fmt.Sprintf("%s님은 어떤 패거리에도 소속되어 있지 않습니다.", targetName))
				return enginecmd.StatusDefault, nil
			}
			if pending {
				ctx.WriteString(fmt.Sprintf("%s님은 [%s] 패거리에 가입을 신청중입니다.", targetName, familyDisplayNameFrom(world, familyID)))
				return enginecmd.StatusDefault, nil
			}
			ctx.WriteString(fmt.Sprintf("%s님은 [%s] 패거리에 속해있습니다.", targetName, familyDisplayNameFrom(world, familyID)))
			return enginecmd.StatusDefault, nil
		}

		actorFamily, actorMember, actorPending := playerFamilyState(world, model.PlayerID(ctx.ActorID))
		if !actorMember && !actorPending {
			ctx.WriteString("당신은 어떤 패거리에도 소속되어 있지 않습니다.")
			return enginecmd.StatusDefault, nil
		}

		if actorPending {
			ctx.WriteString(fmt.Sprintf("당신은 [%s] 패거리에 가입을 신청중입니다.\n\n", familyDisplayNameFrom(world, actorFamily)))
		} else {
			ctx.WriteString(fmt.Sprintf("당신은 [%s] 패거리에 소속되어 있습니다.\n\n", familyDisplayNameFrom(world, actorFamily)))
		}
		total := 0
		for _, activeSession := range active() {
			if activeSession.ActorID == "" {
				continue
			}
			targetFamily, member, pending := playerFamilyState(world, model.PlayerID(activeSession.ActorID))
			if targetFamily != actorFamily || (!member && !pending) {
				continue
			}
			total++
			if pending {
				ctx.WriteString(fmt.Sprintf("(-)%-14s", playerDisplayName(world, activeSession.ActorID)))
			} else {
				ctx.WriteString(fmt.Sprintf("   %-14s", playerDisplayName(world, activeSession.ActorID)))
			}
			if total%3 == 0 {
				ctx.WriteString("\n")
			}
		}
		if total%3 != 0 {
			ctx.WriteString("\n")
		}
		ctx.WriteString(fmt.Sprintf("\n총 %d명의 패거리원들이 이용중입니다.", total))
		return enginecmd.StatusDefault, nil
	}
}

func NewActionHandler(world ActionWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}

		actorID := model.PlayerID(ctx.ActorID)
		actorRoom := playerRoomID(world, actorID)
		if actorRoom.IsZero() {
			return enginecmd.StatusDefault, fmt.Errorf("game: action actor %q has no room", actorID)
		}
		room, ok := world.Room(actorRoom)
		if !ok {
			return enginecmd.StatusDefault, fmt.Errorf("game: action room %q not found", actorRoom)
		}

		actorCreature, actorOK := socialActorCreature(world, actorID)
		if err := revealSocialActor(world, actorID); err != nil {
			return enginecmd.StatusDefault, err
		}
		if actorOK && creatureFlagEnabled(actorCreature, "PSILNC", "silenced", "silence") {
			ctx.WriteString("한마디도 할수 없습니다!")
			return enginecmd.StatusDefault, nil
		}

		actorName := playerDisplayName(world, ctx.ActorID)
		target, consumed := actionTarget{}, 0
		if resolved.Spec.Name != "생각" {
			target, consumed = findActionTarget(world, room, actorID, resolved.Args)
		}
		modifier := actionModifierFromArgs(resolved.Args, consumed)
		message := renderActionMessages(resolved.Spec.Name, actorName, target.Name, target.OK, modifier)
		ctx.WriteString(message.Self)

		for _, activeSession := range active() {
			if string(activeSession.ID) == ctx.SessionID || activeSession.ActorID == "" {
				continue
			}
			if playerRoomID(world, model.PlayerID(activeSession.ActorID)) != actorRoom {
				continue
			}
			out := message.Room
			if target.OK && target.PlayerID == model.PlayerID(activeSession.ActorID) && message.Target != "" {
				out = message.Target
			}
			if err := send(activeSession.ID, session.Command{Write: out}); err != nil {
				return enginecmd.StatusDefault, err
			}
		}
		return enginecmd.StatusDefault, nil
	}
}

func NewEmoteHandler(world ActionWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}

		message := emoteMessageFromResolved(resolved)
		if message == "" {
			ctx.WriteString("무슨말을 표현하시려구요?\n")
			return enginecmd.StatusDefault, nil
		}
		actorCreature, actorOK := socialActorCreature(world, model.PlayerID(ctx.ActorID))
		if actorOK && creatureFlagEnabled(actorCreature, "PSILNC", "silenced", "silence") {
			ctx.WriteString("당신은 지금당장 그것을 할 수 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		if err := revealSocialActor(world, model.PlayerID(ctx.ActorID)); err != nil {
			return enginecmd.StatusDefault, err
		}

		actorID := model.PlayerID(ctx.ActorID)
		actorRoom := playerRoomID(world, actorID)
		if actorRoom.IsZero() {
			return enginecmd.StatusDefault, fmt.Errorf("game: emote actor %q has no room", actorID)
		}
		actorName := playerDisplayName(world, ctx.ActorID)
		if actorOK && creatureFlagEnabled(actorCreature, "PLECHO", "echo", "legacyEcho") {
			ctx.WriteString(fmt.Sprintf(":%s %s.", actorSubject(actorName), message))
		} else {
			ctx.WriteString("예. 좋습니다.\n")
		}
		out := fmt.Sprintf("\n:%s %s.", actorSubject(actorName), message)

		for _, activeSession := range active() {
			if string(activeSession.ID) == ctx.SessionID || activeSession.ActorID == "" {
				continue
			}
			if playerRoomID(world, model.PlayerID(activeSession.ActorID)) != actorRoom {
				continue
			}
			if err := send(activeSession.ID, session.Command{Write: out}); err != nil {
				return enginecmd.StatusDefault, err
			}
		}
		return enginecmd.StatusDefault, nil
	}
}

func NewYellHandler(world ActionWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}

		message := yellMessageFromResolved(resolved)
		if message == "" {
			ctx.WriteString("무슨말을 외치려구요?")
			return enginecmd.StatusDefault, nil
		}
		actorCreature, actorOK := socialActorCreature(world, model.PlayerID(ctx.ActorID))
		if actorOK && creatureFlagEnabled(actorCreature, "PSILNC", "silenced", "silence") {
			ctx.WriteString("당신의 목소리가 너무 약해서 외칠수 없습니다.")
			return enginecmd.StatusDefault, nil
		}
		if err := revealSocialActor(world, model.PlayerID(ctx.ActorID)); err != nil {
			return enginecmd.StatusDefault, err
		}

		actorID := model.PlayerID(ctx.ActorID)
		actorRoom := playerRoomID(world, actorID)
		if actorRoom.IsZero() {
			return enginecmd.StatusDefault, fmt.Errorf("game: yell actor %q has no room", actorID)
		}
		room, ok := world.Room(actorRoom)
		if !ok {
			return enginecmd.StatusDefault, fmt.Errorf("game: yell room %q not found", actorRoom)
		}

		actorName := playerDisplayName(world, ctx.ActorID)
		ctx.WriteString("예. 좋습니다.")
		sameRoomOut := fmt.Sprintf("\n%s이 \"%s!\"라고 외칩니다.", actorName, message)
		adjacentOut := fmt.Sprintf("\n누군가가 \"%s!\"라고 외쳤습니다.", message)

		for _, activeSession := range active() {
			if string(activeSession.ID) == ctx.SessionID || activeSession.ActorID == "" {
				continue
			}
			targetRoom := playerRoomID(world, model.PlayerID(activeSession.ActorID))
			switch {
			case targetRoom == actorRoom:
				if err := send(activeSession.ID, session.Command{Write: sameRoomOut}); err != nil {
					return enginecmd.StatusDefault, err
				}
			case roomHasExitTo(room, targetRoom):
				if err := send(activeSession.ID, session.Command{Write: adjacentOut}); err != nil {
					return enginecmd.StatusDefault, err
				}
			}
		}
		return enginecmd.StatusDefault, nil
	}
}

func activeSessionsFunc(ctx *enginecmd.Context) (func() []ActiveSession, bool) {
	if ctx == nil || ctx.Values == nil {
		return nil, false
	}
	fn, ok := ctx.Values[ContextActiveSessionsKey].(func() []ActiveSession)
	return fn, ok
}

func sendToSessionFunc(ctx *enginecmd.Context) (func(session.ID, session.Command) error, bool) {
	if ctx == nil || ctx.Values == nil {
		return nil, false
	}
	fn, ok := ctx.Values[ContextSendToSessionKey].(func(session.ID, session.Command) error)
	return fn, ok
}

type broadcastSocialConfig struct {
	silencedMessage          string
	lowLevelMessage          string
	lowHPMessage             string
	duplicateMessage         string
	duplicateBeforeStatus    bool
	duplicateLimitSeconds    int64
	restrictionClock         func() time.Time
	legacyBroadcastFormatter func(string, string) string
}

type broadcastSocialMemory struct {
	mu           sync.Mutex
	lastMessage  map[session.ID]string
	limitedUntil map[session.ID]int64
	lastTime     map[session.ID]int64
}

func newBroadcastSocialMemory() *broadcastSocialMemory {
	return &broadcastSocialMemory{
		lastMessage:  map[session.ID]string{},
		limitedUntil: map[session.ID]int64{},
		lastTime:     map[session.ID]int64{},
	}
}

func (m *broadcastSocialMemory) duplicateBlocked(sessionID session.ID, message string, now int64, limitSeconds int64) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastMessage[sessionID] == message && m.limitedUntil[sessionID]-now > 0 {
		return true
	}
	return false
}

func (m *broadcastSocialMemory) recordSuccess(sessionID session.ID, message string, now int64, limitSeconds int64) int64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	last := m.lastTime[sessionID]
	m.lastMessage[sessionID] = message
	m.limitedUntil[sessionID] = now + limitSeconds
	m.lastTime[sessionID] = now
	return last
}

func broadcastSocialLine(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand, world PlayerLookup, memory *broadcastSocialMemory, config broadcastSocialConfig) (enginecmd.Status, error) {
	if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
		return enginecmd.StatusDefault, ErrSocialActorRequired
	}
	active, ok := activeSessionsFunc(ctx)
	if !ok {
		return enginecmd.StatusDefault, ErrSocialContextMissing
	}
	send, ok := sendToSessionFunc(ctx)
	if !ok {
		return enginecmd.StatusDefault, ErrSocialContextMissing
	}

	message := broadcastSocialMessage(resolved)
	if message == "" {
		ctx.WriteString("무슨 말을 하시려구요?")
		return enginecmd.StatusDefault, nil
	}
	now := broadcastSocialNow(config)
	sessionID := session.ID(ctx.SessionID)
	if config.duplicateBeforeStatus && memory.duplicateBlocked(sessionID, message, now, config.duplicateLimitSeconds) {
		ctx.WriteString(config.duplicateMessage)
		return enginecmd.StatusDefault, nil
	}
	actorCreature, actorOK := broadcastSocialActorCreature(world, model.PlayerID(ctx.ActorID))
	actorClass := 0
	actorLevel := 0
	if actorOK {
		actorClass = creatureStat(actorCreature, "class")
		actorLevel = broadcastSocialCreatureLevel(actorCreature)
	}
	if actorOK && creatureFlagEnabled(actorCreature, "PSILNC", "silenced", "silence") && actorClass < model.ClassSubDM {
		ctx.WriteString(config.silencedMessage)
		return enginecmd.StatusDefault, nil
	}
	if actorLevel < 20 && actorClass < model.ClassCaretaker {
		ctx.WriteString(config.lowLevelMessage)
		return enginecmd.StatusDefault, nil
	}
	if !config.duplicateBeforeStatus && memory.duplicateBlocked(sessionID, message, now, config.duplicateLimitSeconds) {
		ctx.WriteString(config.duplicateMessage)
		return enginecmd.StatusDefault, nil
	}
	discount := broadcastSocialDiscount(actorCreature, actorOK, memory, sessionID, now)
	if actorClass < model.ClassSubDM {
		hpCurrent := creatureStat(actorCreature, "hpCurrent")
		if hpCurrent <= discount {
			ctx.WriteString(config.lowHPMessage)
			return enginecmd.StatusDefault, nil
		}
		if setter, ok := world.(interface {
			SetCreatureStat(model.CreatureID, string, int) error
		}); ok && actorOK {
			if err := setter.SetCreatureStat(actorCreature.ID, "hpCurrent", hpCurrent-discount); err != nil {
				return enginecmd.StatusDefault, err
			}
		}
	}
	memory.recordSuccess(sessionID, message, now, config.duplicateLimitSeconds)

	actorName := playerDisplayName(world, ctx.ActorID)
	out := renderLegacyColorForContext(ctx, config.legacyBroadcastFormatter(actorName, message))
	for _, activeSession := range active() {
		if activeSession.ActorID == "" {
			continue
		}
		if string(activeSession.ID) == ctx.SessionID {
			ctx.WriteString(out)
			continue
		}
		if err := send(activeSession.ID, session.Command{Write: out}); err != nil {
			return enginecmd.StatusDefault, err
		}
	}
	return enginecmd.StatusDefault, nil
}

func broadcastSocialMessage(resolved enginecmd.ResolvedCommand) string {
	input := strings.TrimSpace(resolved.Input)
	if input != "" {
		for _, command := range socialCommandNameCandidates(resolved) {
			if stripped, ok := stripSocialCommandAtTextEdge(input, command); ok {
				return stripped
			}
		}
	}
	return strings.TrimSpace(strings.Join(resolved.Args, " "))
}

func tellMessageFromResolved(resolved enginecmd.ResolvedCommand) string {
	input := strings.TrimSpace(resolved.Input)
	if input != "" {
		for _, command := range socialCommandNameCandidates(resolved) {
			if stripped, ok := stripSocialCommandAtTextEdge(input, command); ok {
				return legacyTellMessageAfterTarget(stripped)
			}
		}
	}
	if len(resolved.Args) <= 1 {
		return ""
	}
	return strings.TrimSpace(strings.Join(resolved.Args[1:], " "))
}

func replyMessageFromResolved(resolved enginecmd.ResolvedCommand) string {
	input := strings.TrimSpace(resolved.Input)
	if input != "" {
		for _, command := range socialCommandNameCandidates(resolved) {
			if stripped, ok := stripSocialCommandAtTextEdge(input, command); ok {
				return stripped
			}
		}
	}
	return strings.TrimSpace(strings.Join(resolved.Args, " "))
}

func sayMessageFromResolved(resolved enginecmd.ResolvedCommand) string {
	input := resolved.Input
	if input != "" {
		for _, command := range socialCommandNameCandidates(resolved) {
			if stripped, ok := stripSocialCommandAtTextEdge(input, command); ok {
				return stripped
			}
		}
		if legacySayInputUsesDefaultVerb(input) {
			return input
		}
	}
	return strings.TrimSpace(strings.Join(resolved.Args, " "))
}

func emoteMessageFromResolved(resolved enginecmd.ResolvedCommand) string {
	return legacyCutCommandMessageFromResolved(resolved)
}

func legacyCutCommandMessageFromResolved(resolved enginecmd.ResolvedCommand) string {
	input := resolved.Input
	if input != "" {
		for _, command := range socialCommandNameCandidates(resolved) {
			if stripped, ok := stripSocialCommandAtTextEdge(input, command); ok {
				return stripped
			}
		}
	}
	return strings.TrimSpace(strings.Join(resolved.Args, " "))
}

func yellMessageFromResolved(resolved enginecmd.ResolvedCommand) string {
	input := resolved.Input
	if input != "" {
		for _, command := range socialCommandNameCandidates(resolved) {
			if socialCommandAtTextEnd(input, command) {
				return input
			}
			if stripped, ok := stripSocialCommandAtTextEdge(input, command); ok {
				return stripped
			}
		}
		return ""
	}
	return strings.TrimSpace(strings.Join(resolved.Args, " "))
}

func legacyTellMessageAfterTarget(text string) string {
	for i := 0; i < len(text); i++ {
		if text[i] == ' ' && (i+1 >= len(text) || text[i+1] != ' ') {
			if i+1 >= len(text) {
				return ""
			}
			return text[i+1:]
		}
	}
	return ""
}

func socialCommandAtTextEnd(input, command string) bool {
	command = strings.TrimSpace(command)
	if command == "" || len(input) <= len(command) {
		return false
	}
	start := len(input) - len(command)
	return start > 0 && input[start-1] == ' ' && strings.EqualFold(input[start:], command)
}

func legacySayInputUsesDefaultVerb(input string) bool {
	if input == "" {
		return false
	}
	switch input[len(input)-1] {
	case ' ', '.', '!', '?':
		return true
	default:
		return false
	}
}

func socialCommandNameCandidates(resolved enginecmd.ResolvedCommand) []string {
	raw := []string{
		resolved.Command(),
		resolved.CmdName,
		resolved.Spec.Name,
		resolved.Spec.Handler,
	}
	seen := map[string]struct{}{}
	candidates := make([]string, 0, len(raw))
	for _, command := range raw {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		key := strings.ToLower(command)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, command)
	}
	return candidates
}

func stripSocialCommandAtTextEdge(input, command string) (string, bool) {
	command = strings.TrimSpace(command)
	if command == "" || len(input) < len(command) {
		return "", false
	}
	if strings.EqualFold(input, command) {
		return "", true
	}
	if len(input) > len(command) && input[len(command)] == ' ' && strings.EqualFold(input[:len(command)], command) {
		return strings.TrimSpace(input[len(command):]), true
	}
	start := len(input) - len(command)
	if start > 0 && input[start-1] == ' ' && strings.EqualFold(input[start:], command) {
		before := input[:start-1]
		if strings.TrimSpace(before) == "" {
			return "", true
		}
		return before, true
	}
	return "", false
}

func broadcastSocialNow(config broadcastSocialConfig) int64 {
	if config.restrictionClock == nil {
		return time.Now().Unix()
	}
	return config.restrictionClock().Unix()
}

func broadcastSocialActorCreature(world PlayerLookup, playerID model.PlayerID) (model.Creature, bool) {
	return socialActorCreature(world, playerID)
}

func broadcastSocialCreatureLevel(creature model.Creature) int {
	if level := creatureStat(creature, "level"); level != 0 {
		return level
	}
	return creature.Level
}

func broadcastSocialDiscount(creature model.Creature, ok bool, memory *broadcastSocialMemory, sessionID session.ID, now int64) int {
	discountTable := [31]int{60, 57, 54, 51, 48, 45, 42, 40, 38, 36, 34, 32, 30, 28, 26, 24, 22, 20, 18, 16, 14, 12, 10, 8, 7, 6, 5, 4, 3, 2, 2}
	discount := 2
	if memory != nil {
		memory.mu.Lock()
		last := memory.lastTime[sessionID]
		memory.mu.Unlock()
		if last != 0 {
			elapsed := now - last
			if elapsed >= 0 && elapsed <= 30 {
				discount = discountTable[int(elapsed)]
			}
		}
	}
	level := 0
	if ok {
		level = broadcastSocialCreatureLevel(creature)
	}
	if level < 10 {
		discount /= 2
	}
	if level < 20 {
		discount /= 2
	}
	return discount
}

func findActivePlayerSession(world PlayerLookup, sessions []ActiveSession, target string) (ActiveSession, string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return ActiveSession{}, "", false
	}
	var fallback ActiveSession
	var fallbackName string
	for _, activeSession := range sessions {
		if activeSession.ActorID == "" {
			continue
		}
		name, ok := activePlayerLookupName(world, activeSession.ActorID)
		if !ok {
			continue
		}
		if legacyASCIINameMatches(name, target) {
			return activeSession, name, true
		}
		if legacyASCIIPrefixMatches(name, target) {
			fallback = activeSession
			fallbackName = name
		}
	}
	if fallback.ActorID != "" {
		return fallback, fallbackName, true
	}
	return ActiveSession{}, "", false
}

func findActivePlayerSessionByActor(world PlayerLookup, sessions []ActiveSession, actorID string) (ActiveSession, string, bool) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ActiveSession{}, "", false
	}
	for _, activeSession := range sessions {
		if activeSession.ActorID != actorID {
			continue
		}
		name := playerDisplayName(world, activeSession.ActorID)
		if name == "" {
			name = activeSession.ActorID
		}
		return activeSession, name, true
	}
	return ActiveSession{}, "", false
}

func renderLegacyColorForContext(ctx *enginecmd.Context, text string) string {
	return textfmt.RenderLegacyColors(text, textfmt.Options{
		ANSI:   socialBoolContextValue(ctx, enginecmd.ContextANSIKey),
		Bright: socialBoolContextValue(ctx, enginecmd.ContextANSIBrightKey),
	})
}

func socialBoolContextValue(ctx *enginecmd.Context, key string) bool {
	if ctx == nil || ctx.Values == nil {
		return false
	}
	value, ok := ctx.Values[key]
	if !ok {
		return false
	}
	enabled, ok := value.(bool)
	return ok && enabled
}

func playerDisplayName(world PlayerLookup, actorID string) string {
	if world != nil {
		if player, ok := world.Player(model.PlayerID(actorID)); ok && strings.TrimSpace(player.DisplayName) != "" {
			return strings.TrimSpace(player.DisplayName)
		}
	}
	return actorID
}

func activePlayerLookupName(world PlayerLookup, actorID string) (string, bool) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "", false
	}
	if world != nil {
		if player, ok := world.Player(model.PlayerID(actorID)); ok {
			return legacyLookupName(player.DisplayName, actorID)
		}
	}
	return legacyLookupName("", actorID)
}

func playerRoomID(world PlayerLookup, playerID model.PlayerID) model.RoomID {
	if world == nil || playerID.IsZero() {
		return ""
	}
	player, ok := world.Player(playerID)
	if !ok {
		return ""
	}
	return player.RoomID
}

func playerCreature(world FamilyWorld, playerID model.PlayerID) (model.Creature, bool) {
	if world == nil || playerID.IsZero() {
		return model.Creature{}, false
	}
	player, ok := world.Player(playerID)
	if !ok || player.CreatureID.IsZero() {
		return model.Creature{}, false
	}
	return world.Creature(player.CreatureID)
}

func socialActorCreature(world PlayerLookup, playerID model.PlayerID) (model.Creature, bool) {
	familyWorld, ok := world.(FamilyWorld)
	if !ok {
		return model.Creature{}, false
	}
	return playerCreature(familyWorld, playerID)
}

func revealSocialActor(world PlayerLookup, playerID model.PlayerID) error {
	if world == nil || playerID.IsZero() {
		return nil
	}
	player, ok := world.Player(playerID)
	if !ok {
		return nil
	}
	if !player.CreatureID.IsZero() {
		creature, creatureOK := socialActorCreature(world, playerID)
		if updater, ok := world.(interface {
			UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
		}); ok {
			if _, err := updater.UpdateCreatureTags(player.CreatureID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
				return err
			}
		}
		if creatureOK && creature.Stats != nil && creature.Stats["PHIDDN"] != 0 {
			if setter, ok := world.(interface {
				SetCreatureStat(model.CreatureID, string, int) error
			}); ok {
				if err := setter.SetCreatureStat(player.CreatureID, "PHIDDN", 0); err != nil {
					return err
				}
			}
		}
	}
	if !player.ID.IsZero() {
		if updater, ok := world.(interface {
			UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
		}); ok {
			if _, err := updater.UpdatePlayerTags(player.ID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
				return err
			}
		}
	}
	return nil
}

func playerFamilyMembership(world FamilyWorld, playerID model.PlayerID) (int, bool) {
	creature, ok := playerCreature(world, playerID)
	if !ok {
		return 0, false
	}
	if !creatureFlagEnabled(creature, "familyFlag", "PFAMIL") {
		return 0, false
	}
	for _, key := range []string{"familyID", "dailyExpndMax", "legacyDailyExpndMax"} {
		if value, ok := creatureIntValue(creature, key); ok && value >= 0 {
			return value, true
		}
	}
	return 0, true
}

func playerFamilyState(world FamilyWorld, playerID model.PlayerID) (familyID int, member bool, pending bool) {
	creature, ok := playerCreature(world, playerID)
	if !ok {
		return 0, false, false
	}
	member = creatureHasNormalizedFlag(creature, "familyFlag", "PFAMIL")
	pending = creatureHasNormalizedFlag(creature, "PRDFML", "familyRequest", "familyPending")
	if !member && !pending {
		return 0, false, false
	}
	if familyID, ok = creatureNormalizedInt(creature, "familyID", "dailyExpndMax", "legacyDailyExpndMax"); ok && familyID >= 0 {
		return familyID, member, pending
	}
	return 0, member, pending
}

func familyWhoTargetVisible(actor model.Creature, actorOK bool, target model.Creature) bool {
	if creatureHasNormalizedFlag(target, "PDMINV", "dmInvisible") {
		return false
	}
	if actorOK && creatureHasNormalizedFlag(actor, "PBLIND", "blind") {
		return false
	}
	if creatureHasNormalizedFlag(target, "PINVIS", "invisible") &&
		(!actorOK || !creatureHasNormalizedFlag(actor, "PDINVI", "detectInvisible")) {
		return false
	}
	return true
}

func creatureFlagEnabled(creature model.Creature, keys ...string) bool {
	return creatureHasAnyFlag(creature, keys...)
}

func socialActorSilenced(world PlayerLookup, playerID model.PlayerID) bool {
	familyWorld, ok := world.(FamilyWorld)
	if !ok {
		return false
	}
	creature, ok := playerCreature(familyWorld, playerID)
	if !ok {
		return false
	}
	return creatureFlagEnabled(creature, "PSILNC", "silenced", "silence")
}

func socialReplyTargetHiddenFromActor(world PlayerLookup, actorID model.PlayerID, targetID model.PlayerID) bool {
	familyWorld, ok := world.(FamilyWorld)
	if !ok {
		return false
	}
	actor, actorOK := playerCreature(familyWorld, actorID)
	target, targetOK := playerCreature(familyWorld, targetID)
	if !actorOK || !targetOK {
		return false
	}
	if creatureStat(actor, "class") >= model.ClassDM {
		return false
	}
	return creatureFlagEnabled(target, "PINVIS", "invisible", "invisibility") &&
		!creatureFlagEnabled(actor, "PDINVI", "detectInvisible", "detectInvis")
}

func socialActorBlocksTell(world PlayerLookup, actorID model.PlayerID, targetID model.PlayerID) bool {
	familyWorld, ok := world.(FamilyWorld)
	if !ok {
		return false
	}
	actor, ok := playerCreature(familyWorld, actorID)
	if ok && creatureStat(actor, "class") >= model.ClassDM {
		return false
	}
	creature, ok := playerCreature(familyWorld, targetID)
	if !ok {
		return false
	}
	return creatureFlagEnabled(creature, "PIGNOR", "ignoreTalk", "talkIgnore", "ignoreAllTalk")
}

func firstIgnoreMemory(memories ...*IgnoreMemory) *IgnoreMemory {
	for _, memory := range memories {
		if memory != nil {
			return memory
		}
	}
	return nil
}

func socialIgnoredBy(memory *IgnoreMemory, world PlayerLookup, listenerOwnerID, listenerActorID, speakerActorID string) bool {
	if memory == nil || strings.TrimSpace(speakerActorID) == "" {
		return false
	}
	for _, ownerID := range ignoreOwnerCandidates(listenerOwnerID, listenerActorID) {
		if memory.Ignored(ownerID, playerDisplayName(world, speakerActorID)) {
			return true
		}
		if memory.Ignored(ownerID, speakerActorID) {
			return true
		}
	}
	return false
}

func ignoreOwnerCandidates(primary, fallback string) []string {
	candidates := []string{strings.TrimSpace(primary), strings.TrimSpace(fallback)}
	seen := map[string]bool{}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	return out
}

func creatureIntValue(creature model.Creature, key string) (int, bool) {
	if value, ok := creature.Stats[key]; ok {
		return value, true
	}
	if raw, ok := creature.Properties[key]; ok {
		value, parsed := parseSocialInt(raw)
		return value, parsed
	}
	target := normalizeFlagName(key)
	if target == "" {
		return 0, false
	}
	for statKey, value := range creature.Stats {
		if normalizeFlagName(statKey) == target {
			return value, true
		}
	}
	for propertyKey, raw := range creature.Properties {
		if normalizeFlagName(propertyKey) == target {
			value, parsed := parseSocialInt(raw)
			return value, parsed
		}
	}
	return 0, false
}

func parseSocialInt(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
		return 0, false
	}
	return parsed, true
}

func propertyFlagEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func familyDisplayName(familyID int) string {
	return familyDisplayNameFrom(nil, familyID)
}

func roomHasExitTo(room model.Room, target model.RoomID) bool {
	if target.IsZero() {
		return false
	}
	for _, exit := range room.Exits {
		if exit.ToRoomID == target {
			return true
		}
	}
	return false
}

type actionTarget struct {
	OK       bool
	Name     string
	PlayerID model.PlayerID
}

type actionMessages struct {
	Self   string
	Target string
	Room   string
}

func findActionTarget(world ActionWorld, room model.Room, actorID model.PlayerID, args []string) (actionTarget, int) {
	if world == nil || len(args) == 0 {
		return actionTarget{}, 0
	}
	target := strings.TrimSpace(args[0])
	if target == "" {
		return actionTarget{}, 0
	}
	if found, ok := findActionCreatureTarget(world, room, actorID, target); ok {
		return found, 1
	}
	if found, ok := findActionPlayerTarget(world, room, actorID, target); ok {
		return found, 1
	}
	return actionTarget{}, 0
}

func findActionPlayerTarget(world ActionWorld, room model.Room, actorID model.PlayerID, target string) (actionTarget, bool) {
	for _, playerID := range room.PlayerIDs {
		if playerID.IsZero() || playerID == actorID {
			continue
		}
		player, ok := world.Player(playerID)
		if !ok {
			continue
		}
		name := strings.TrimSpace(player.DisplayName)
		var nameOK bool
		name, nameOK = legacyLookupName(name, string(player.ID))
		if !nameOK {
			continue
		}
		if actionNameMatches(name, string(player.ID), target) {
			return actionTarget{OK: true, Name: name, PlayerID: player.ID}, true
		}
	}
	return actionTarget{}, false
}

func findActionCreatureTarget(world ActionWorld, room model.Room, actorID model.PlayerID, target string) (actionTarget, bool) {
	for _, creatureID := range room.CreatureIDs {
		if creatureID.IsZero() {
			continue
		}
		creature, ok := world.Creature(creatureID)
		if !ok || creature.PlayerID == actorID {
			continue
		}
		name := strings.TrimSpace(creature.DisplayName)
		var nameOK bool
		name, nameOK = legacyLookupName(name, string(creature.ID))
		if !nameOK {
			continue
		}
		if actionNameMatches(name, string(creature.ID), target) {
			return actionTarget{OK: true, Name: name, PlayerID: creature.PlayerID}, true
		}
	}
	return actionTarget{}, false
}

func actionNameMatches(name, _ string, target string) bool {
	name = strings.TrimSpace(name)
	target = strings.TrimSpace(target)
	return target != "" && legacyASCIIPrefixMatches(name, target)
}

func legacyASCIINameMatches(candidate, target string) bool {
	return legacyLowercizeASCII(candidate, false) == legacyLowercizeASCII(target, false)
}

func legacyASCIIPrefixMatches(candidate, target string) bool {
	candidate = legacyLowercizeASCII(candidate, false)
	target = legacyLowercizeASCII(target, false)
	return candidate != "" && target != "" && strings.HasPrefix(candidate, target)
}

func legacyLookupName(displayName, fallbackID string) (string, bool) {
	if name := strings.TrimSpace(displayName); name != "" {
		return name, true
	}
	fallbackID = strings.TrimSpace(fallbackID)
	if fallbackID == "" || strings.Contains(fallbackID, ":") {
		return "", false
	}
	return fallbackID, true
}

func actionModifierFromArgs(args []string, consumed int) string {
	if consumed < 0 || consumed > len(args) {
		consumed = 0
	}
	return strings.TrimSpace(strings.Join(args[consumed:], " "))
}

func actionPrefix(custom, fallback string) string {
	custom = strings.TrimSpace(custom)
	if custom == "" {
		return fallback
	}
	return custom + " "
}

func actorSubject(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "누군가"
	}
	return name + krtext.Particle(name, '1')
}

func targetObject(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "누군가"
	}
	return name + krtext.Particle(name, '3')
}

func targetWith(name, particle string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "누군가"
	}
	return name + particle
}

func renderActionMessages(commandName, actorName, targetName string, targeted bool, custom string) actionMessages {
	name := strings.TrimSpace(commandName)
	actor := actorSubject(actorName)
	noTarget := func(fallback string, phrase string) actionMessages {
		prefix := actionPrefix(custom, fallback)
		return actionMessages{
			Self: fmt.Sprintf("당신은 %s%s\n", prefix, phrase),
			Room: fmt.Sprintf("%s %s%s\n", actor, prefix, phrase),
		}
	}
	toTarget := func(noTargetFallback, targetFallback string, selfPhrase, targetPhrase, roomPhrase string) actionMessages {
		prefix := actionPrefix(custom, targetFallback)
		if !targeted {
			return noTarget(noTargetFallback, selfPhrase)
		}
		target := targetWith(targetName, "에게")
		return actionMessages{
			Self:   fmt.Sprintf("당신은 %s %s%s\n", target, prefix, selfPhrase),
			Target: fmt.Sprintf("%s 당신에게 %s%s\n", actor, prefix, targetPhrase),
			Room:   fmt.Sprintf("%s %s %s%s\n", actor, target, prefix, roomPhrase),
		}
	}
	toTargetObject := func(noTargetFallback, targetFallback string, selfPhrase, targetPhrase, roomPhrase string) actionMessages {
		prefix := actionPrefix(custom, targetFallback)
		if !targeted {
			return noTarget(noTargetFallback, selfPhrase)
		}
		obj := targetObject(targetName)
		return actionMessages{
			Self:   fmt.Sprintf("당신은 %s %s%s\n", obj, prefix, selfPhrase),
			Target: fmt.Sprintf("%s 당신을 %s%s\n", actor, prefix, targetPhrase),
			Room:   fmt.Sprintf("%s %s %s%s\n", actor, obj, prefix, roomPhrase),
		}
	}

	switch name {
	case "감정표현":
		return actionMessages{Self: "당신은 '감정표현 도움'이라 치는게 좋을겁니다.\n", Room: actor + " 감정표현을 연구합니다.\n"}
	case "보아":
		return toTargetObject("", "", "봅니다.", "봅니다.", "봅니다.")
	case "노려봐":
		return toTargetObject("허공을 뚫어져라 ", "뱁새눈을 하고 ", "노려 봅니다.", "노려 봅니다.", "노려 봅니다.")
	case "끄덕", "응":
		return toTarget("", "", "고개를 끄덕거립니다.", "고개를 끄덕거립니다.", "고개를 끄덕거립니다.")
	case "아니":
		return toTarget("", "", "고개를 가로젓습니다.", "고개를 가로젓습니다.", "고개를 가로젓습니다.")
	case "감", "감사":
		return toTarget("진심으로 ", "진심으로 ", "감사해 합니다.", "감사해 합니다.", "감사해 합니다.")
	case "미소":
		return toTarget("밝은 ", "", "미소를 짓습니다.", "미소를 짓습니다.", "미소를 짓습니다.")
	case "떨어":
		return actionLookTarget(targeted, custom, "무서워서 ", "무서워서 ", actor, targetName, "벌벌 떱니다.")
	case "해":
		if targeted {
			prefix := actionPrefix(custom, "보며 심심해 ")
			return actionMessages{
				Self:   fmt.Sprintf("당신은 %s %s합니다.\n", targetObject(targetName), prefix),
				Target: fmt.Sprintf("%s 당신을 %s합니다.\n", actor, prefix),
				Room:   fmt.Sprintf("%s %s %s합니다.\n", actor, targetObject(targetName), prefix),
			}
		}
		return noTarget("심심해 ", "합니다.")
	case "하품":
		return actionLookTarget(targeted, custom, "자지러지게 ", "", actor, targetName, "하품을 합니다. 아함~")
	case "청혼":
		return toTarget("혼자서 ", "", "청혼을 합니다.", "청혼을 합니다.", "청혼을 합니다.")
	case "웃어":
		return actionLookTarget(targeted, custom, "생긋 ", "생긋 ", actor, targetName, "웃습니다.")
	case "미안":
		return toTarget("", "", "미안해 합니다.", "미안해 합니다.", "미안해 합니다.")
	case "악수":
		if targeted {
			prefix := actionPrefix(custom, "")
			withTarget := targetWith(targetName, krtext.Particle(targetName, '2'))
			return actionMessages{
				Self:   fmt.Sprintf("당신은 %s %s악수를 합니다.\n", withTarget, prefix),
				Target: fmt.Sprintf("%s 당신과 %s악수를 합니다.\n", actor, prefix),
				Room:   fmt.Sprintf("%s %s %s악수를 합니다.\n", actor, withTarget, prefix),
			}
		}
		return actionMessages{
			Self: fmt.Sprintf("당신은 손을 내밀어 %s악수를 청합니다.\n", actionPrefix(custom, "")),
			Room: fmt.Sprintf("%s 손을 내밀어 %s악수를 청합니다.\n", actor, actionPrefix(custom, "")),
		}
	case "하이파이브":
		if targeted {
			prefix := actionPrefix(custom, "손을 높이들어 ")
			withTarget := targetWith(targetName, krtext.Particle(targetName, '2'))
			return actionMessages{
				Self:   fmt.Sprintf("당신은 %s %s하이파이브를 합니다.\n", withTarget, prefix),
				Target: fmt.Sprintf("%s 당신과 %s하이파이브를 합니다.\n", actor, prefix),
				Room:   fmt.Sprintf("%s %s %s하이파이브를 합니다.\n", actor, withTarget, prefix),
			}
		}
		return actionMessages{
			Self: fmt.Sprintf("당신은 손을 들어 허공에다 %s휘젓습니다.\n", actionPrefix(custom, "")),
			Room: fmt.Sprintf("%s 손을 들어 허공에다 %s휘젓습니다.\n", actor, actionPrefix(custom, "")),
		}
	case "박수":
		return toTarget("", "", "박수를 칩니다. 짝짝~", "박수를 칩니다. 짝짝~", "박수를 칩니다. 짝짝~")
	case "흡연", "담배":
		if targeted {
			return toTarget("", "", "담배를 권합니다.", "담배를 권합니다.", "담배를 권합니다.")
		}
		return actionMessages{
			Self: fmt.Sprintf("당신은 담배를 %s피웁니다. 푸우~~~~\n", actionPrefix(custom, "뻐금뻐금 ")),
			Room: fmt.Sprintf("%s 담배를 %s피웁니다. 푸우~~\n", actor, actionPrefix(custom, "뻐금뻐금 ")),
		}
	case "절":
		return toTarget("공손히 ", "공손히 ", "절을 합니다.", "절을 합니다.", "절을 합니다.")
	case "찔러":
		if targeted {
			return toTargetObject("쿡쿡 ", "쿡쿡 ", "찌릅니다.", "찌릅니다.", "찌릅니다.")
		}
		return actionMessages{
			Self: fmt.Sprintf("당신은 손을 올려 허공을 %s찌릅니다.\n", actionPrefix(custom, "쿡쿡 ")),
			Room: fmt.Sprintf("%s 손을 올려 허공을 %s찌릅니다.\n", actor, actionPrefix(custom, "쿡쿡 ")),
		}
	case "춤":
		if targeted {
			prefix := actionPrefix(custom, "신나게 ")
			withTarget := targetWith(targetName, krtext.Particle(targetName, '2'))
			return actionMessages{
				Self:   fmt.Sprintf("당신은 %s %s춤을 춥니다. '대구.부산.찍고~광주.턴!'\n", withTarget, prefix),
				Target: fmt.Sprintf("%s 당신과 %s춤을 춥니다. '대구.부산.찍고~광주.턴!'\n", actor, prefix),
				Room:   fmt.Sprintf("%s %s %s춤을 춥니다. '대구.부산.찍고~광주.턴!'\n", actor, withTarget, prefix),
			}
		}
		return noTarget("혼자서 ", "춤을 춥니다. '대구.부산.찍고~광주.턴!'")
	case "노래":
		if targeted {
			prefix := actionPrefix(custom, "즐겁게 ")
			return actionMessages{
				Self:   fmt.Sprintf("당신은 %s 위해 %s노래를 부릅니다.\n", targetObject(targetName), prefix),
				Target: fmt.Sprintf("%s 당신을 위해 %s노래를 부릅니다.\n", actor, prefix),
				Room:   fmt.Sprintf("%s %s 위해 %s노래를 부릅니다.\n", actor, targetObject(targetName), prefix),
			}
		}
		return noTarget("즐겁게 ", "노래를 부릅니다.")
	case "울어":
		if targeted {
			return toTarget("", "", "눈물을 보입니다.", "눈물을 보입니다.", "눈물을 보입니다.")
		}
		return noTarget("슬프게 ", "웁니다. 아앙~")
	case "달래":
		return toTargetObject("자신을 ", "", "달래 줍니다.", "달래 줍니다.", "달래 줍니다.")
	case "당황":
		return actionLookTarget(targeted, custom, "", "", actor, targetName, "당황해 합니다.")
	case "윙크":
		return toTarget("", "", "윙크를 합니다.", "윙크를 합니다.", "윙크를 합니다.")
	case "뽀뽀":
		return toTarget("자기 손바닥에다 ", "", "뽀뽀를 합니다.", "뽀뽀를 합니다.", "뽀뽀를 합니다.")
	case "바이", "잘가":
		return toTarget("", "", "작별인사를 합니다.", "작별인사를 합니다.", "작별인사를 합니다.")
	case "안녕":
		return toTarget("", "", "인사를 합니다. \"안녕하세요~\"", "인사를 합니다. \"안녕하세요~\"", "인사를 합니다. \"안녕하세요~\"")
	case "설레":
		return actionLookTarget(targeted, custom, "", "", actor, targetName, "마음이 설레입니다.")
	case "놀려":
		return actionLookTarget(targeted, custom, "약오르게 ", "약오르게 ", actor, targetName, "놀립니다. 메롱메롱~~")
	case "생각":
		return noTarget("조심스럽게 ", "생각합니다.")
	case "부끄러":
		return noTarget("얼굴이 빨개져 ", "부끄러워 합니다.")
	case "구걸":
		if targeted {
			return toTarget("", "", "구걸합니다. \"한푼줍쇼~~\"", "구걸합니다. \"한푼줍쇼~~\"", "구걸합니다. \"한푼줍쇼~~\"")
		}
		return actionMessages{
			Self: fmt.Sprintf("당신은 바닥에 엎드려 %s구걸합니다. \"한푼줍쇼~~\"\n", actionPrefix(custom, "")),
			Room: fmt.Sprintf("%s 바닥에 엎드려 %s구걸합니다. \"한푼줍쇼~~\"\n", actor, actionPrefix(custom, "")),
		}
	case "구박":
		if targeted {
			return toTargetObject("마구 ", "마구 ", "구박합니다.", "구박합니다.", "구박합니다.")
		}
		return actionMessages{
			Self: fmt.Sprintf("당신은 스스로를 %s구박합니다.\n", actionPrefix(custom, "마구 ")),
			Room: fmt.Sprintf("%s 스스로를 %s구박합니다.\n", actor, actionPrefix(custom, "마구 ")),
		}
	case "안아", "껴안아":
		if targeted {
			return toTargetObject("허공을 ", "꼭 ", "껴안습니다.", "껴안습니다.", "껴안습니다.")
		}
		return actionMessages{
			Self: fmt.Sprintf("당신은 %s껴안으려 애를 씁니다.\n", actionPrefix(custom, "허공을 ")),
			Room: fmt.Sprintf("%s %s껴안으려 애를 쓰고 있습니다.\n", actor, actionPrefix(custom, "허공을 ")),
		}
	case "니다":
		return actionMessages{Self: "당신은 바보짓을 합니다.\n", Room: actor + " 바보짓을 합니다.\n"}
	default:
		return noTarget("", "감정을 표현합니다.")
	}
}

func actionLookTarget(targeted bool, custom, noTargetFallback, targetFallback, actor, targetName, phrase string) actionMessages {
	prefix := actionPrefix(custom, targetFallback)
	if !targeted {
		prefix = actionPrefix(custom, noTargetFallback)
		return actionMessages{
			Self: fmt.Sprintf("당신은 %s%s\n", prefix, phrase),
			Room: fmt.Sprintf("%s %s%s\n", actor, prefix, phrase),
		}
	}
	obj := targetObject(targetName)
	return actionMessages{
		Self:   fmt.Sprintf("당신은 %s 보고 %s%s\n", obj, prefix, phrase),
		Target: fmt.Sprintf("%s 당신을 보고 %s%s\n", actor, prefix, phrase),
		Room:   fmt.Sprintf("%s %s 보고 %s%s\n", actor, obj, prefix, phrase),
	}
}
