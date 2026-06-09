package protocol

import "unicode/utf8"

const DefaultInputLimit = 1024

// LineDiscipline converts terminal byte input into complete UTF-8 command
// lines. It keeps C io.c's user-visible behavior: CR/LF terminates a command,
// DEL and BS delete one buffered input unit, control bytes are ignored, and
// Telnet option negotiation bytes are stripped before command parsing.
type LineDiscipline struct {
	limit       int
	buffer      []rune
	bufferBytes int
	pendingUTF8 []byte
	pendingIAC  bool
	skipTelnet  int
	sawCR       bool
	invalid     int
}

type LineOption func(*LineDiscipline)

func WithInputLimit(limit int) LineOption {
	return func(l *LineDiscipline) {
		l.limit = limit
	}
}

func NewLineDiscipline(opts ...LineOption) *LineDiscipline {
	l := &LineDiscipline{limit: DefaultInputLimit}
	for _, opt := range opts {
		if opt != nil {
			opt(l)
		}
	}
	if l.limit <= 0 {
		l.limit = DefaultInputLimit
	}
	return l
}

// Feed adds a socket byte chunk and returns every complete command line made
// available by the chunk. Incomplete UTF-8 sequences are retained for the next
// call; invalid UTF-8 bytes are discarded and counted.
func (l *LineDiscipline) Feed(data []byte) []string {
	if l == nil {
		return nil
	}

	var lines []string
	for i := 0; i < len(data); i++ {
		b := data[i]
		if l.skipTelnet > 0 {
			l.skipTelnet--
			continue
		}
		if l.pendingIAC {
			l.pendingIAC = false
			if isTelnetOptionCommand(b) {
				if i+1 < len(data) {
					i++
				} else {
					l.skipTelnet = 1
				}
			}
			continue
		}
		if b == 0xff {
			if i+1 < len(data) && isTelnetOptionCommand(data[i+1]) {
				if i+2 < len(data) {
					i += 2
				} else {
					i++
					l.skipTelnet = 1
				}
			} else if i+1 == len(data) {
				l.pendingIAC = true
			}
			continue
		}

		switch b {
		case '\r', '\n':
			if b == '\n' && l.sawCR {
				l.sawCR = false
				continue
			}
			lines = append(lines, string(l.buffer))
			l.resetLine()
			l.sawCR = b == '\r'
		case '\b', 0x7f:
			l.sawCR = false
			l.pendingUTF8 = l.pendingUTF8[:0]
			l.backspace()
		default:
			l.sawCR = false
			if b < 32 || (b == 155 && len(l.pendingUTF8) == 0) {
				continue
			}
			l.acceptUTF8Byte(b)
		}
	}
	return lines
}

func (l *LineDiscipline) Buffered() string {
	if l == nil {
		return ""
	}
	return string(l.buffer)
}

func (l *LineDiscipline) InvalidUTF8Bytes() int {
	if l == nil {
		return 0
	}
	return l.invalid
}

func (l *LineDiscipline) resetLine() {
	l.buffer = l.buffer[:0]
	l.bufferBytes = 0
	l.pendingUTF8 = l.pendingUTF8[:0]
}

func (l *LineDiscipline) backspace() {
	if len(l.buffer) == 0 {
		return
	}
	last := l.buffer[len(l.buffer)-1]
	l.buffer = l.buffer[:len(l.buffer)-1]
	l.bufferBytes -= runeLen(last)
	if l.bufferBytes < 0 {
		l.bufferBytes = 0
	}
}

func (l *LineDiscipline) acceptUTF8Byte(b byte) {
	l.pendingUTF8 = append(l.pendingUTF8, b)
	if !utf8.FullRune(l.pendingUTF8) {
		return
	}

	r, size := utf8.DecodeRune(l.pendingUTF8)
	if r == utf8.RuneError && size == 1 {
		l.invalid++
		copy(l.pendingUTF8, l.pendingUTF8[1:])
		l.pendingUTF8 = l.pendingUTF8[:len(l.pendingUTF8)-1]
		return
	}

	l.appendRune(r)
	copy(l.pendingUTF8, l.pendingUTF8[size:])
	l.pendingUTF8 = l.pendingUTF8[:len(l.pendingUTF8)-size]
}

func (l *LineDiscipline) appendRune(r rune) {
	n := runeLen(r)
	for len(l.buffer) > 0 && l.bufferBytes+n > l.limit {
		l.bufferBytes -= runeLen(l.buffer[0])
		copy(l.buffer, l.buffer[1:])
		l.buffer = l.buffer[:len(l.buffer)-1]
	}
	if n > l.limit {
		return
	}
	l.buffer = append(l.buffer, r)
	l.bufferBytes += n
}

func runeLen(r rune) int {
	if n := utf8.RuneLen(r); n > 0 {
		return n
	}
	return len(string(r))
}

func isTelnetOptionCommand(b byte) bool {
	switch b {
	case 0xfb, 0xfc, 0xfd, 0xfe:
		return true
	default:
		return false
	}
}
