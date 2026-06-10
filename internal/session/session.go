package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"muhan/internal/protocol"
)

const (
	DefaultReadBufferSize = 128
	MaxInputLineBytes     = 512
)

type Option func(*Session)

func WithReadBufferSize(size int) Option {
	return func(s *Session) {
		s.readBufferSize = size
	}
}

type Session struct {
	id       ID
	conn     net.Conn
	state    *protocol.SessionState
	events   chan<- Event
	commands <-chan Command

	readBufferSize int

	mu          sync.Mutex
	closeOnce   sync.Once
	closedEvent sync.Once
	closed      atomic.Bool
}

func New(id ID, conn net.Conn, events chan<- Event, commands <-chan Command, opts ...Option) (*Session, error) {
	if id == "" {
		return nil, errors.New("session id is required")
	}
	if conn == nil {
		return nil, errors.New("connection is required")
	}
	if events == nil {
		return nil, errors.New("events channel is required")
	}
	if commands == nil {
		return nil, errors.New("commands channel is required")
	}

	s := &Session{
		id:             id,
		conn:           conn,
		state:          protocol.NewSessionState(),
		events:         events,
		commands:       commands,
		readBufferSize: DefaultReadBufferSize,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	if s.readBufferSize <= 0 {
		s.readBufferSize = DefaultReadBufferSize
	}
	return s, nil
}

func (s *Session) ID() ID {
	if s == nil {
		return ""
	}
	return s.id
}

func (s *Session) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	readerDone := make(chan error, 1)
	writerDone := make(chan error, 1)
	go func() {
		readerDone <- s.readLoop(ctx)
	}()
	go func() {
		writerDone <- s.writeLoop(ctx)
	}()

	var err error
	readerClosed := false
	writerClosed := false
	select {
	case err = <-readerDone:
		readerClosed = true
	case err = <-writerDone:
		writerClosed = true
	case <-ctx.Done():
		err = ctx.Err()
	}

	cancel()
	s.closeConn()

	if !readerClosed {
		readerErr := <-readerDone
		if err == nil {
			err = readerErr
		}
	}
	if !writerClosed {
		writerErr := <-writerDone
		if err == nil {
			err = writerErr
		}
	}
	if isExpectedClose(err) {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (s *Session) readLoop(ctx context.Context) error {
	buf := make([]byte, s.readBufferSize)
	for {
		n, err := s.conn.Read(buf)
		if n > 0 {
			lines := s.feedInput(buf[:n])
			for _, line := range lines {
				line = truncateUTF8(line, MaxInputLineBytes)
				if !s.emit(ctx, Event{SessionID: s.id, Kind: EventLine, Line: line}) {
					return ctx.Err()
				}
			}
		}
		if err != nil {
			return s.handleIOError(ctx, err)
		}
	}
}

func (s *Session) writeLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case cmd, ok := <-s.commands:
			if !ok {
				s.closeState()
				s.closeConn()
				s.emitClosed(ctx)
				return nil
			}
			if err := s.applyCommand(ctx, cmd); err != nil {
				return err
			}
			if cmd.Close {
				return nil
			}
		}
	}
}

func (s *Session) applyCommand(ctx context.Context, cmd Command) error {
	var out []byte
	s.mu.Lock()
	if cmd.SetCallback != nil {
		s.state.SetCallback(cmd.SetCallback.Name, cmd.SetCallback.Param)
	}
	if cmd.Write != "" {
		s.state.AppendOutput(cmd.Write)
	}
	if cmd.Prompt != "" {
		s.state.AppendOutput(cmd.Prompt)
	}
	out = s.state.DrainAllOutput()
	if cmd.Close {
		s.state.Close()
	}
	s.mu.Unlock()

	if len(out) > 0 {
		if err := writeAll(s.conn, out); err != nil {
			return s.handleIOError(ctx, err)
		}
	}
	if cmd.Close {
		s.closeConn()
		s.emitClosed(ctx)
		return nil
	}
	return nil
}

func (s *Session) feedInput(data []byte) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.FeedInput(data)
}

func (s *Session) closeState() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Close()
}

func (s *Session) closeConn() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		_ = s.conn.Close()
	})
}

func (s *Session) handleIOError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if isExpectedClose(err) || s.closed.Load() {
		s.closeState()
		s.emitClosed(ctx)
		return nil
	}
	s.closeState()
	s.emit(ctx, Event{SessionID: s.id, Kind: EventError, Err: err})
	s.emitClosed(ctx)
	return err
}

func (s *Session) emitClosed(ctx context.Context) {
	s.closedEvent.Do(func() {
		s.emit(ctx, Event{SessionID: s.id, Kind: EventClosed})
	})
}

func (s *Session) emit(ctx context.Context, event Event) bool {
	select {
	case s.events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func isExpectedClose(err error) bool {
	return err == nil ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, context.Canceled) ||
		err.Error() == "io: read/write on closed pipe"
}

func (e Event) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("%s session %s", e.Kind, e.SessionID)
	}
	return fmt.Sprintf("%s session %s: %v", e.Kind, e.SessionID, e.Err)
}

// truncateUTF8 returns s truncated to at most maxBytes bytes, cutting at a
// valid UTF-8 boundary so that no rune is split.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk backwards from the cut point to find a valid UTF-8 boundary.
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}
