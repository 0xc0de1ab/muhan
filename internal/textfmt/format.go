package textfmt

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"muhan/internal/krtext"
)

// Resolver supplies rendered names for world-backed values that textfmt cannot
// know about yet. The verb is one of m, M, i, or I, and count is the numeric
// prefix from formats such as %2M.
type Resolver interface {
	ResolveText(verb byte, value any, count int) (string, error)
}

type Options struct {
	ANSI   bool
	Bright bool
}

type Renderer struct {
	Options  Options
	Resolver Resolver
}

var legacyColorCodes = map[string]string{
	"검": "30",
	"빨": "31",
	"초": "32",
	"녹": "32",
	"노": "33",
	"파": "34",
	"보": "35",
	"청": "36",
	"하": "36",
	"흰": "37",
}

func Format(format string, args ...any) (string, error) {
	return Renderer{}.Format(format, args...)
}

func RenderLegacyColors(text string, opts Options) string {
	if !strings.Contains(text, "{") && !strings.Contains(text, "}") {
		return text
	}
	var b strings.Builder
	changed := false
	for i := 0; i < len(text); {
		if text[i] == '{' {
			if code, end, ok := legacyColorCodeAt(text, i+1); ok {
				if !changed {
					b.Grow(len(text))
					b.WriteString(text[:i])
					changed = true
				}
				if opts.ANSI {
					b.WriteString(ansiSequence(opts, code))
				}
				i = end
				continue
			}
		}
		if text[i] == '}' {
			if changed {
				if opts.ANSI {
					b.WriteString(ansiResetSequence(opts, "37"))
				}
				i++
				continue
			}
		}
		if changed {
			b.WriteByte(text[i])
		}
		i++
	}
	if !changed {
		return text
	}
	return b.String()
}

func legacyColorCodeAt(text string, start int) (string, int, bool) {
	for token, code := range legacyColorCodes {
		if strings.HasPrefix(text[start:], token) {
			return code, start + len(token), true
		}
	}
	return "", 0, false
}

func (r Renderer) Format(format string, args ...any) (string, error) {
	var b strings.Builder
	argIndex := 0
	lastRendered := ""
	hasLastRendered := false

	for i := 0; i < len(format); {
		if format[i] != '%' {
			b.WriteByte(format[i])
			i++
			continue
		}

		if i+1 >= len(format) {
			return "", fmt.Errorf("dangling %% at byte %d", i)
		}
		if format[i+1] == '%' {
			b.WriteByte('%')
			i += 2
			continue
		}

		if verb, count, end, ok := customVerb(format, i); ok {
			if verb == 'j' {
				kind, err := consumeParticleKind(args, &argIndex, i)
				if err != nil {
					return "", err
				}
				if !hasLastRendered {
					return "", fmt.Errorf("%%j at byte %d has no previous rendered text", i)
				}
				b.WriteString(krtext.Particle(particleReferenceText(lastRendered), kind))
				i = end
				continue
			}

			value, err := consumeArg(args, &argIndex, verb, i)
			if err != nil {
				return "", err
			}

			var rendered string
			switch verb {
			case 'S':
				rendered = stringify(value)
				lastRendered = rendered
				hasLastRendered = true
			case 'C':
				rendered = ansiSequence(r.Options, value)
			case 'D':
				rendered = ansiResetSequence(r.Options, value)
			case 'm', 'M', 'i', 'I':
				if r.Resolver == nil {
					return "", fmt.Errorf("%%%c at byte %d requires a Resolver", verb, i)
				}
				rendered, err = r.Resolver.ResolveText(verb, value, count)
				if err != nil {
					return "", err
				}
				lastRendered = rendered
				hasLastRendered = true
			}

			b.WriteString(rendered)
			i = end
			continue
		}

		spec, end, starArgs, err := standardSpec(format, i)
		if err != nil {
			return "", err
		}
		values := make([]any, 0, starArgs+1)
		for n := 0; n < starArgs; n++ {
			value, err := consumeArg(args, &argIndex, '*', i)
			if err != nil {
				return "", err
			}
			values = append(values, value)
		}
		value, err := consumeArg(args, &argIndex, spec[len(spec)-1], i)
		if err != nil {
			return "", err
		}
		values = append(values, value)
		fmt.Fprintf(&b, goSpec(spec), values...)
		i = end
	}

	return b.String(), nil
}

func customVerb(format string, start int) (verb byte, count int, end int, ok bool) {
	i := start + 1
	for i < len(format) && format[i] >= '0' && format[i] <= '9' {
		count = count*10 + int(format[i]-'0')
		i++
	}
	if i >= len(format) || !isCustomVerb(format[i]) {
		return 0, 0, 0, false
	}
	return format[i], count, i + 1, true
}

func isCustomVerb(verb byte) bool {
	switch verb {
	case 'S', 'j', 'C', 'D', 'm', 'M', 'i', 'I':
		return true
	default:
		return false
	}
}

func standardSpec(format string, start int) (string, int, int, error) {
	i := start + 1
	starArgs := 0
	for i < len(format) && strings.ContainsRune("#0+- ", rune(format[i])) {
		i++
	}
	if i < len(format) && format[i] == '*' {
		starArgs++
		i++
	} else {
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			i++
		}
	}
	if i < len(format) && format[i] == '.' {
		i++
		if i < len(format) && format[i] == '*' {
			starArgs++
			i++
		} else {
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
		}
	}
	for i < len(format) && strings.ContainsRune("hlLztj", rune(format[i])) {
		i++
	}
	if i >= len(format) {
		return "", 0, 0, fmt.Errorf("unterminated format specifier at byte %d", start)
	}
	return format[start : i+1], i + 1, starArgs, nil
}

func goSpec(spec string) string {
	if len(spec) < 2 {
		return spec
	}

	verb := spec[len(spec)-1]
	body := strings.TrimRight(spec[1:len(spec)-1], "hlLztj")
	if verb == 'u' {
		verb = 'd'
	}
	return "%" + body + string(verb)
}

func consumeArg(args []any, index *int, verb byte, pos int) (any, error) {
	if *index >= len(args) {
		return nil, fmt.Errorf("%%%c at byte %d missing argument", verb, pos)
	}
	value := args[*index]
	*index = *index + 1
	return value, nil
}

func consumeParticleKind(args []any, index *int, pos int) (byte, error) {
	value, err := consumeArg(args, index, 'j', pos)
	if err != nil {
		return 0, err
	}
	switch v := value.(type) {
	case byte:
		if v <= 4 {
			return '0' + v, nil
		}
		return v, nil
	case rune:
		if v >= 0 && v <= 4 {
			return '0' + byte(v), nil
		}
		if v >= 0 && v <= 255 {
			return byte(v), nil
		}
	case int:
		if v >= 0 && v <= 4 {
			return '0' + byte(v), nil
		}
		if v >= 0 && v <= 255 {
			return byte(v), nil
		}
	case string:
		if len(v) > 0 {
			return v[0], nil
		}
	case fmt.Stringer:
		s := v.String()
		if len(s) > 0 {
			return s[0], nil
		}
	}
	return 0, fmt.Errorf("%%j particle kind must be byte-like, got %T", value)
}

func stringify(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func particleReferenceText(s string) string {
	s = stripANSISequences(s)
	for {
		before := s
		s = strings.TrimRightFunc(s, unicode.IsSpace)
		s = trimRightFuncExceptClosingPair(s, unicode.IsPunct)
		if trimmed, ok := trimTrailingParenthetical(s); ok {
			s = trimmed
			continue
		}
		s = strings.TrimRightFunc(s, func(r rune) bool {
			return unicode.IsSpace(r) || unicode.IsPunct(r)
		})
		if s == before {
			return s
		}
	}
}

func stripANSISequences(s string) string {
	var b strings.Builder
	changed := false
	for i := 0; i < len(s); {
		if s[i] != '\x1b' {
			if changed {
				b.WriteByte(s[i])
			}
			i++
			continue
		}

		end, ok := ansiEscapeEnd(s, i)
		if !ok {
			if changed {
				b.WriteByte(s[i])
			}
			i++
			continue
		}
		if !changed {
			b.Grow(len(s))
			b.WriteString(s[:i])
			changed = true
		}
		i = end
	}
	if !changed {
		return s
	}
	return b.String()
}

func ansiEscapeEnd(s string, start int) (int, bool) {
	if start+1 >= len(s) {
		return 0, false
	}

	switch s[start+1] {
	case '[':
		for i := start + 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1, true
			}
		}
		return len(s), true
	case ']':
		for i := start + 2; i < len(s); i++ {
			if s[i] == '\a' {
				return i + 1, true
			}
			if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2, true
			}
		}
		return len(s), true
	default:
		if s[start+1] >= 0x40 && s[start+1] <= 0x5f {
			return start + 2, true
		}
	}
	return 0, false
}

func trimRightFuncExceptClosingPair(s string, f func(rune) bool) string {
	for len(s) > 0 {
		r, size := utf8.DecodeLastRuneInString(s)
		if !f(r) || isClosingPairRune(r) {
			return s
		}
		s = s[:len(s)-size]
	}
	return s
}

func isClosingPairRune(r rune) bool {
	switch r {
	case ')', ']', '}', '）', '］', '｝':
		return true
	default:
		return false
	}
}

func trimTrailingParenthetical(s string) (string, bool) {
	pairs := []struct {
		open  rune
		close rune
	}{
		{'(', ')'},
		{'[', ']'},
		{'{', '}'},
		{'（', '）'},
		{'［', '］'},
		{'｛', '｝'},
	}
	for _, pair := range pairs {
		if !strings.HasSuffix(s, string(pair.close)) {
			continue
		}
		start, ok := matchingTrailingOpen(s, pair.open, pair.close)
		if !ok {
			return s, false
		}
		return strings.TrimRightFunc(s[:start], unicode.IsSpace), true
	}
	return s, false
}

func matchingTrailingOpen(s string, open, close rune) (int, bool) {
	depth := 0
	for i := len(s); i > 0; {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		i -= size
		switch r {
		case close:
			depth++
		case open:
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func ansiSequence(opts Options, value any) string {
	if !opts.ANSI {
		return ""
	}

	color := stringify(value)
	if color == "" {
		return ""
	}
	if _, err := strconv.Atoi(color); err == nil {
		color += "m"
	}

	bright := 0
	if opts.Bright && leadingColorCode(color) != 37 {
		bright = 1
	}
	return fmt.Sprintf("\x1b[%d;%s", bright, color)
}

func ansiResetSequence(opts Options, value any) string {
	if !opts.ANSI {
		return ""
	}

	color := stringify(value)
	if color == "" {
		color = "0"
	}
	return ansiSequence(opts, color)
}

func leadingColorCode(s string) int {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return -1
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil {
		return -1
	}
	return n
}
