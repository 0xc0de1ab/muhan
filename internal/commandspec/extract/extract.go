// Package extract reads the legacy C cmdlist initializer into Go command specs.
package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

// Entry is one parsed row from src/global.c's cmdlist[] table.
//
// Number is normalized for special rows: a legacy negative cmdno becomes its
// absolute value and Special is set.
type Entry struct {
	Name       string `json:"name"`
	Number     int    `json:"number"`
	Handler    string `json:"handler"`
	Privileged bool   `json:"privileged"`
	Special    bool   `json:"special"`
}

// CommandSpec returns the registry-facing shape of this entry.
func (e Entry) CommandSpec() commandspec.CommandSpec {
	return commandspec.CommandSpec{
		Name:       e.Name,
		Number:     e.Number,
		Handler:    e.Handler,
		Privileged: e.Privileged,
		Special:    e.Special,
	}
}

// CommandSpecs converts parsed entries to the registry-facing shape.
func CommandSpecs(entries []Entry) []commandspec.CommandSpec {
	specs := make([]commandspec.CommandSpec, len(entries))
	for i, entry := range entries {
		specs[i] = entry.CommandSpec()
	}
	return specs
}

// ExtractFile reads path, decodes it as UTF-8 or legacy Korean text, and
// extracts cmdlist[] entries.
func ExtractFile(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ExtractBytes(path, data)
}

// ExtractRoot reads src/global.c below root.
func ExtractRoot(root string) ([]Entry, error) {
	return ExtractFile(filepath.Join(root, "src", "global.c"))
}

// ExtractBytes decodes C source bytes as UTF-8 or EUC-KR/CP949 and extracts
// cmdlist[] entries.
func ExtractBytes(path string, data []byte) ([]Entry, error) {
	source, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{
		Path:  path,
		Field: "cmdlist source",
	}, data)
	if err != nil {
		source, err = decodeLegacyKoreanPermissive(data)
		if err != nil {
			return nil, err
		}
	}
	return ExtractSource(source)
}

func decodeLegacyKoreanPermissive(data []byte) (string, error) {
	var out strings.Builder
	for i := 0; i < len(data); {
		c := data[i]
		if c <= 0x7f {
			out.WriteByte(c)
			i++
			continue
		}

		if i+1 < len(data) {
			decoded, ok := decodeKoreanPair(data[i : i+2])
			if ok {
				out.WriteString(decoded)
				i += 2
				continue
			}
		}

		if r, size := utf8.DecodeRune(data[i:]); r != utf8.RuneError && size > 1 {
			out.WriteRune(r)
			i += size
			continue
		}

		out.WriteRune(utf8.RuneError)
		i++
	}
	return out.String(), nil
}

func decodeKoreanPair(pair []byte) (string, bool) {
	out, _, err := transform.Bytes(korean.EUCKR.NewDecoder(), pair)
	if err != nil {
		return "", false
	}
	if !utf8.Valid(out) {
		return "", false
	}
	decoded := string(out)
	if decoded == "" || strings.ContainsRune(decoded, utf8.RuneError) {
		return "", false
	}
	return decoded, true
}

var cmdlistStartRE = regexp.MustCompile(`\bcmdlist\s*\[\s*\]\s*=\s*\{`)

// ExtractSource extracts cmdlist[] entries from UTF-8 decoded C source.
func ExtractSource(source string) ([]Entry, error) {
	if !utf8.ValidString(source) {
		return nil, fmt.Errorf("source is not valid UTF-8")
	}

	withoutComments := stripCComments(source)
	loc := cmdlistStartRE.FindStringIndex(withoutComments)
	if loc == nil {
		return nil, fmt.Errorf("cmdlist[] initializer not found")
	}

	start := strings.LastIndex(withoutComments[loc[0]:loc[1]], "{")
	if start < 0 {
		return nil, fmt.Errorf("cmdlist[] initializer opening brace not found")
	}
	start += loc[0]

	body, err := braceBody(withoutComments, start)
	if err != nil {
		return nil, fmt.Errorf("cmdlist[] initializer: %w", err)
	}

	rows, err := topLevelRows(body)
	if err != nil {
		return nil, fmt.Errorf("cmdlist[] rows: %w", err)
	}

	entries := make([]Entry, 0, len(rows))
	for i, row := range rows {
		entry, ok, err := parseRow(row)
		if err != nil {
			return nil, fmt.Errorf("cmdlist[] row %d: %w", i+1, err)
		}
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseRow(row string) (Entry, bool, error) {
	fields, err := splitFields(row)
	if err != nil {
		return Entry{}, false, err
	}
	if len(fields) != 3 {
		return Entry{}, false, fmt.Errorf("got %d field(s), want 3", len(fields))
	}

	name, err := parseCString(fields[0])
	if err != nil {
		return Entry{}, false, fmt.Errorf("name: %w", err)
	}

	rawNumber, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil {
		return Entry{}, false, fmt.Errorf("cmdno: %w", err)
	}

	handler := strings.TrimSpace(fields[2])
	if handler == "" {
		return Entry{}, false, fmt.Errorf("handler is empty")
	}

	if name == "@" && rawNumber == 0 && handler == "0" {
		return Entry{}, false, nil
	}

	entry := Entry{
		Name:       name,
		Number:     rawNumber,
		Handler:    handler,
		Privileged: strings.HasPrefix(name, "*"),
	}
	if rawNumber < 0 {
		entry.Number = -rawNumber
		entry.Special = true
	}
	return entry, true, nil
}

func braceBody(s string, open int) (string, error) {
	if open < 0 || open >= len(s) || s[open] != '{' {
		return "", fmt.Errorf("opening brace index is invalid")
	}

	depth := 0
	inString := false
	inChar := false
	escaped := false

	for i := open; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		if inChar {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '\'':
				inChar = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '\'':
			inChar = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[open+1 : i], nil
			}
			if depth < 0 {
				return "", fmt.Errorf("unexpected closing brace")
			}
		}
	}

	return "", fmt.Errorf("closing brace not found")
}

func topLevelRows(body string) ([]string, error) {
	var rows []string
	depth := 0
	rowStart := -1
	inString := false
	inChar := false
	escaped := false

	for i := 0; i < len(body); i++ {
		c := body[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		if inChar {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '\'':
				inChar = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '\'':
			inChar = true
		case '{':
			if depth == 0 {
				rowStart = i + 1
			}
			depth++
		case '}':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unexpected closing brace")
			}
			if depth == 0 {
				rows = append(rows, body[rowStart:i])
				rowStart = -1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unterminated row initializer")
	}
	return rows, nil
}

func splitFields(row string) ([]string, error) {
	var fields []string
	start := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	inString := false
	inChar := false
	escaped := false

	for i := 0; i < len(row); i++ {
		c := row[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		if inChar {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '\'':
				inChar = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '\'':
			inChar = true
		case '(':
			parenDepth++
		case ')':
			if parenDepth == 0 {
				return nil, fmt.Errorf("unexpected )")
			}
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth == 0 {
				return nil, fmt.Errorf("unexpected ]")
			}
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			if braceDepth == 0 {
				return nil, fmt.Errorf("unexpected }")
			}
			braceDepth--
		case ',':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				fields = append(fields, strings.TrimSpace(row[start:i]))
				start = i + 1
			}
		}
	}
	if inString {
		return nil, fmt.Errorf("unterminated string literal")
	}
	if inChar {
		return nil, fmt.Errorf("unterminated character literal")
	}
	if parenDepth != 0 || bracketDepth != 0 || braceDepth != 0 {
		return nil, fmt.Errorf("unbalanced delimiters")
	}
	fields = append(fields, strings.TrimSpace(row[start:]))
	return fields, nil
}

func parseCString(field string) (string, error) {
	rest := strings.TrimSpace(field)
	if rest == "" {
		return "", fmt.Errorf("empty string literal")
	}

	var out strings.Builder
	for rest != "" {
		if rest[0] != '"' {
			return "", fmt.Errorf("expected string literal, got %q", rest)
		}
		body, next, err := consumeCStringLiteral(rest)
		if err != nil {
			return "", err
		}
		decoded, err := decodeCStringBody(body)
		if err != nil {
			return "", err
		}
		out.WriteString(decoded)
		rest = strings.TrimSpace(rest[next:])
	}
	return out.String(), nil
}

func consumeCStringLiteral(s string) (body string, next int, err error) {
	escaped := false
	for i := 1; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '"':
			return s[1:i], i + 1, nil
		}
	}
	return "", 0, fmt.Errorf("unterminated string literal")
}

func decodeCStringBody(body string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' {
			out.WriteByte(c)
			continue
		}
		if i+1 >= len(body) {
			return "", fmt.Errorf("trailing backslash in string literal")
		}
		i++
		switch esc := body[i]; esc {
		case 'a':
			out.WriteByte('\a')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'v':
			out.WriteByte('\v')
		case '\\', '\'', '"', '?':
			out.WriteByte(esc)
		case 'x', 'X':
			value, next, err := consumeHexEscape(body, i+1)
			if err != nil {
				return "", err
			}
			out.WriteByte(byte(value))
			i = next - 1
		default:
			if esc >= '0' && esc <= '7' {
				value, next := consumeOctalEscape(body, i)
				out.WriteByte(byte(value))
				i = next - 1
				continue
			}
			return "", fmt.Errorf("unsupported escape \\%c", esc)
		}
	}
	return out.String(), nil
}

func consumeHexEscape(s string, start int) (int64, int, error) {
	if start >= len(s) || !isHexDigit(s[start]) {
		return 0, 0, fmt.Errorf("hex escape has no digits")
	}
	i := start
	for i < len(s) && isHexDigit(s[i]) {
		i++
	}
	value, err := strconv.ParseInt(s[start:i], 16, 32)
	if err != nil {
		return 0, 0, err
	}
	return value, i, nil
}

func consumeOctalEscape(s string, start int) (int64, int) {
	i := start
	for i < len(s) && i < start+3 && s[i] >= '0' && s[i] <= '7' {
		i++
	}
	value, _ := strconv.ParseInt(s[start:i], 8, 32)
	return value, i
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func stripCComments(s string) string {
	var out strings.Builder
	out.Grow(len(s))

	inString := false
	inChar := false
	escaped := false

	for i := 0; i < len(s); {
		c := s[i]
		if inString {
			out.WriteByte(c)
			i++
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		if inChar {
			out.WriteByte(c)
			i++
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '\'':
				inChar = false
			}
			continue
		}

		switch {
		case c == '"':
			inString = true
			out.WriteByte(c)
			i++
		case c == '\'':
			inChar = true
			out.WriteByte(c)
			i++
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			i += 2
			for i < len(s) && s[i] != '\n' {
				i++
			}
			if i < len(s) {
				out.WriteByte('\n')
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			i += 2
			for i < len(s) {
				if s[i] == '*' && i+1 < len(s) && s[i+1] == '/' {
					i += 2
					break
				}
				if s[i] == '\n' {
					out.WriteByte('\n')
				}
				i++
			}
		default:
			out.WriteByte(c)
			i++
		}
	}

	return out.String()
}
