package command

import (
	"strings"

	"muhan/internal/world/model"
)

const (
	ContextActiveActorIDsKey = "command.activeActorIDs"
	ContextANSIKey           = "command.ansi"
	ContextANSIBrightKey     = "command.ansiBright"
	ContextPendingLineKey    = "command.pendingLineHandler"
	ContextRoomBroadcastKey  = "command.roomBroadcast"
	ContextShopSellBonusKey  = "command.shopSellBonus"
)

type PendingLineHandler func(*Context, string) (Status, error)
type RoomBroadcastFunc func(roomID model.RoomID, excludeSessionID string, text string) error

func (c *Context) WriteString(text string) {
	if c == nil || text == "" {
		return
	}
	c.Output = append(c.Output, text)
}

func (c *Context) OutputString() string {
	if c == nil || len(c.Output) == 0 {
		return ""
	}
	return strings.Join(c.Output, "")
}

func SetPendingLineHandler(ctx *Context, handler PendingLineHandler) bool {
	if ctx == nil || ctx.Values == nil {
		return false
	}
	setter, ok := ctx.Values[ContextPendingLineKey].(func(PendingLineHandler))
	if !ok || setter == nil {
		return false
	}
	setter(handler)
	return true
}

func ClearPendingLineHandler(ctx *Context) bool {
	return SetPendingLineHandler(ctx, nil)
}

func roomBroadcast(ctx *Context, roomID model.RoomID, text string) error {
	if ctx == nil || ctx.Values == nil || roomID.IsZero() || text == "" {
		return nil
	}
	fn, ok := ctx.Values[ContextRoomBroadcastKey].(RoomBroadcastFunc)
	if !ok || fn == nil {
		return nil
	}
	return fn(roomID, ctx.SessionID, text)
}

func activeActorIDSet(ctx *Context) (map[string]struct{}, bool) {
	if ctx == nil || ctx.Values == nil {
		return nil, false
	}
	provider, ok := ctx.Values[ContextActiveActorIDsKey].(func() []string)
	if !ok || provider == nil {
		return nil, false
	}

	ids := provider()
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			set[id] = struct{}{}
		}
	}
	return set, true
}
