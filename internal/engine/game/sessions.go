package game

import (
	"context"
	"errors"
	"fmt"
	"sort"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/session"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	// ContextActiveSessionsKey stores func() []ActiveSession in command context values.
	ContextActiveSessionsKey = "game.activeSessions"
	// ContextSessionActorsKey stores func() map[session.ID]string in command context values.
	ContextSessionActorsKey = "game.sessionActors"
	// ContextSendToSessionKey stores func(session.ID, session.Command) error in command context values.
	ContextSendToSessionKey = "game.sendToSession"
	// ContextBroadcastKey stores func(session.Command) error in command context values.
	ContextBroadcastKey = "game.broadcast"
	// ContextBroadcastExceptKey stores func(session.ID, session.Command) error in command context values.
	ContextBroadcastExceptKey = "game.broadcastExcept"
)

// ActiveSession is a point-in-time snapshot of a registered session.
type ActiveSession struct {
	ID      session.ID
	ActorID string
}

type commandTarget struct {
	id       session.ID
	commands chan<- session.Command
}

// ActiveSessions returns registered sessions sorted by session ID.
func (l *Loop) ActiveSessions() []ActiveSession {
	if l == nil {
		return nil
	}

	l.mu.RLock()
	snapshot := make([]ActiveSession, 0, len(l.sessions))
	for id, b := range l.sessions {
		snapshot = append(snapshot, ActiveSession{ID: id, ActorID: b.actorID})
	}
	l.mu.RUnlock()

	sort.Slice(snapshot, func(i, j int) bool {
		return snapshot[i].ID < snapshot[j].ID
	})
	return snapshot
}

// ActiveSessionActors returns a snapshot keyed by session ID for command handlers.
func (l *Loop) ActiveSessionActors() map[session.ID]string {
	snapshot := l.ActiveSessions()
	if len(snapshot) == 0 {
		return map[session.ID]string{}
	}

	actors := make(map[session.ID]string, len(snapshot))
	for _, active := range snapshot {
		actors[active.ID] = active.ActorID
	}
	return actors
}

// SendToSession sends a command to the currently registered session ID.
func (l *Loop) SendToSession(ctx context.Context, id session.ID, cmd session.Command) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil {
		return errors.New("game: nil loop")
	}
	if isNoopCommand(cmd) {
		return nil
	}

	b, ok := l.sessionBinding(id)
	if !ok {
		return fmt.Errorf("%w %q", ErrSessionNotFound, id)
	}
	return sendCommand(ctx, b.commands, cmd)
}

// Broadcast sends a command to every currently registered session.
func (l *Loop) Broadcast(ctx context.Context, cmd session.Command) error {
	return l.broadcastTargets(ctx, l.commandTargets(""), cmd)
}

// BroadcastExcept sends a command to every registered session except excluded.
func (l *Loop) BroadcastExcept(ctx context.Context, excluded session.ID, cmd session.Command) error {
	return l.broadcastTargets(ctx, l.commandTargets(excluded), cmd)
}

func (l *Loop) newCommandContext(ctx context.Context, id session.ID, actorID string) *enginecmd.Context {
	values := make(map[string]any, len(l.values)+8)
	for key, value := range l.values {
		values[key] = value
	}
	values[ContextActiveSessionsKey] = func() []ActiveSession {
		return l.ActiveSessions()
	}
	values[ContextSessionActorsKey] = func() map[session.ID]string {
		return l.ActiveSessionActors()
	}
	values[enginecmd.ContextActiveActorIDsKey] = func() []string {
		active := l.ActiveSessions()
		ids := make([]string, 0, len(active))
		for _, session := range active {
			if session.ActorID != "" {
				ids = append(ids, session.ActorID)
			}
		}
		return ids
	}
	values[ContextSendToSessionKey] = func(id session.ID, cmd session.Command) error {
		return l.SendToSession(ctx, id, cmd)
	}
	values[ContextBroadcastKey] = func(cmd session.Command) error {
		return l.Broadcast(ctx, cmd)
	}
	values[ContextBroadcastExceptKey] = func(excluded session.ID, cmd session.Command) error {
		return l.BroadcastExcept(ctx, excluded, cmd)
	}
	if l.roomWorld != nil {
		values[enginecmd.ContextRoomBroadcastKey] = enginecmd.RoomBroadcastFunc(func(roomID model.RoomID, excluded string, text string) error {
			return l.RoomBroadcast(ctx, roomID, session.ID(excluded), text)
		})
	}
	values[enginecmd.ContextPendingLineKey] = func(handler enginecmd.PendingLineHandler) {
		l.setPendingLineHandler(id, handler)
	}

	return &enginecmd.Context{
		SessionID: string(id),
		ActorID:   actorID,
		Values:    values,
	}
}

// RoomBroadcast sends text to active sessions whose bound players are in roomID.
func (l *Loop) RoomBroadcast(ctx context.Context, roomID model.RoomID, excluded session.ID, text string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil {
		return errors.New("game: nil loop")
	}
	if l.roomWorld == nil || roomID.IsZero() || text == "" {
		return nil
	}
	for _, active := range l.ActiveSessions() {
		if active.ID == excluded || active.ActorID == "" {
			continue
		}
		player, ok := l.roomWorld.Player(model.PlayerID(active.ActorID))
		if !ok || player.RoomID != roomID {
			continue
		}
		err := l.SendToSession(ctx, active.ID, session.Command{Write: text})
		if errors.Is(err, ErrSessionNotFound) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *Loop) sessionBinding(id session.ID) (binding, bool) {
	if l == nil {
		return binding{}, false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	b, ok := l.sessions[id]
	return b, ok
}

func (l *Loop) commandTargets(excluded session.ID) []commandTarget {
	if l == nil {
		return nil
	}

	l.mu.RLock()
	targets := make([]commandTarget, 0, len(l.sessions))
	for id, b := range l.sessions {
		if id == excluded {
			continue
		}
		targets = append(targets, commandTarget{id: id, commands: b.commands})
	}
	l.mu.RUnlock()

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].id < targets[j].id
	})
	return targets
}

func (l *Loop) broadcastTargets(ctx context.Context, targets []commandTarget, cmd session.Command) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil {
		return errors.New("game: nil loop")
	}
	if isNoopCommand(cmd) {
		return nil
	}
	for _, target := range targets {
		if err := sendCommand(ctx, target.commands, cmd); err != nil {
			return fmt.Errorf("game: send to session %q: %w", target.id, err)
		}
	}
	return nil
}
