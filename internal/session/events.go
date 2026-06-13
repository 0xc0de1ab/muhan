package session

import "github.com/0xc0de1ab/muhan/internal/protocol"

type ID string

type EventKind string

const (
	EventLine   EventKind = "line"
	EventClosed EventKind = "closed"
	EventError  EventKind = "error"
)

type Event struct {
	SessionID ID
	Kind      EventKind
	Line      string
	Err       error
}

type CallbackMode = protocol.CallbackMode

type Command struct {
	Write       string
	Prompt      string
	Close       bool
	SetCallback *CallbackMode
}
