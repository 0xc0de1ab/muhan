package protocol

const DefaultOutputLimit = 8192

type OutputBuffer struct {
	limit int
	buf   []byte
}

type OutputOption func(*OutputBuffer)

func WithOutputLimit(limit int) OutputOption {
	return func(b *OutputBuffer) {
		b.limit = limit
	}
}

func NewOutputBuffer(opts ...OutputOption) *OutputBuffer {
	b := &OutputBuffer{limit: DefaultOutputLimit}
	for _, opt := range opts {
		if opt != nil {
			opt(b)
		}
	}
	if b.limit <= 0 {
		b.limit = DefaultOutputLimit
	}
	return b
}

func (b *OutputBuffer) AppendString(s string) int {
	if b == nil {
		return 0
	}

	written := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			b.appendByte('\r')
			written++
		}
		b.appendByte(s[i])
		written++
	}
	return written
}

func (b *OutputBuffer) Len() int {
	if b == nil {
		return 0
	}
	return len(b.buf)
}

func (b *OutputBuffer) Empty() bool {
	return b.Len() == 0
}

func (b *OutputBuffer) HighWater() bool {
	if b == nil {
		return false
	}
	return len(b.buf) > (b.limit*3)/4
}

func (b *OutputBuffer) Drain(n int) []byte {
	if b == nil || n <= 0 || len(b.buf) == 0 {
		return nil
	}
	if n > len(b.buf) {
		n = len(b.buf)
	}
	out := append([]byte(nil), b.buf[:n]...)
	copy(b.buf, b.buf[n:])
	b.buf = b.buf[:len(b.buf)-n]
	return out
}

func (b *OutputBuffer) DrainAll() []byte {
	if b == nil || len(b.buf) == 0 {
		return nil
	}
	return b.Drain(len(b.buf))
}

func (b *OutputBuffer) appendByte(v byte) {
	if b.limit <= 0 {
		return
	}
	if len(b.buf) == b.limit {
		copy(b.buf, b.buf[1:])
		b.buf[len(b.buf)-1] = v
		return
	}
	b.buf = append(b.buf, v)
}
