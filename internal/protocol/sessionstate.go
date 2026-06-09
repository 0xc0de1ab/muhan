package protocol

type CallbackMode struct {
	Name  string
	Param byte
}

type SessionState struct {
	input     *LineDiscipline
	output    *OutputBuffer
	lines     []string
	closed    bool
	interrupt bool
	callback  CallbackMode
}

func NewSessionState() *SessionState {
	return &SessionState{
		input:  NewLineDiscipline(),
		output: NewOutputBuffer(),
	}
}

func (s *SessionState) FeedInput(data []byte) []string {
	if s == nil || s.closed {
		return nil
	}
	lines := s.input.Feed(data)
	if len(lines) > 0 {
		s.lines = append(s.lines, lines...)
		s.interrupt = true
	} else {
		s.interrupt = false
	}
	return append([]string(nil), lines...)
}

func (s *SessionState) NextLine() (string, bool) {
	if s == nil || len(s.lines) == 0 {
		return "", false
	}
	line := s.lines[0]
	copy(s.lines, s.lines[1:])
	s.lines = s.lines[:len(s.lines)-1]
	return line, true
}

func (s *SessionState) PendingLines() int {
	if s == nil {
		return 0
	}
	return len(s.lines)
}

func (s *SessionState) InterruptReady() bool {
	if s == nil {
		return false
	}
	return s.interrupt
}

func (s *SessionState) AppendOutput(text string) int {
	if s == nil || s.closed {
		return 0
	}
	return s.output.AppendString(text)
}

func (s *SessionState) ShouldFlush() bool {
	if s == nil {
		return false
	}
	return s.interrupt || s.output.HighWater()
}

func (s *SessionState) DrainOutput(n int) []byte {
	if s == nil {
		return nil
	}
	return s.output.Drain(n)
}

func (s *SessionState) DrainAllOutput() []byte {
	if s == nil {
		return nil
	}
	return s.output.DrainAll()
}

func (s *SessionState) SetCallback(name string, param byte) {
	if s == nil {
		return
	}
	s.callback = CallbackMode{Name: name, Param: param}
}

func (s *SessionState) Callback() CallbackMode {
	if s == nil {
		return CallbackMode{}
	}
	return s.callback
}

func (s *SessionState) Close() {
	if s == nil {
		return
	}
	s.closed = true
}

func (s *SessionState) Closed() bool {
	if s == nil {
		return true
	}
	return s.closed
}
