// Package legacykr contains import-only helpers for legacy Muhan data encoded
// as EUC-KR/CP949 bytes.
package legacykr

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

// Context identifies where a conversion failed while importing legacy data.
type Context struct {
	Path  string
	Field string
}

// ConversionError wraps codec failures with file/field context.
type ConversionError struct {
	Op     string
	Path   string
	Field  string
	Offset int
	Sample []byte
	Err    error
}

func (e *ConversionError) Error() string {
	var b strings.Builder
	if e.Op != "" {
		b.WriteString(e.Op)
	} else {
		b.WriteString("convert")
	}
	b.WriteString(" legacy Korean text")
	if e.Path != "" {
		b.WriteString(" path=")
		b.WriteString(fmt.Sprintf("%q", e.Path))
	}
	if e.Field != "" {
		b.WriteString(" field=")
		b.WriteString(fmt.Sprintf("%q", e.Field))
	}
	if e.Offset >= 0 {
		b.WriteString(fmt.Sprintf(" offset=%d", e.Offset))
	}
	if len(e.Sample) > 0 {
		b.WriteString(fmt.Sprintf(" sample=% X", e.Sample))
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

func (e *ConversionError) Unwrap() error {
	return e.Err
}

// DecodeEUCKR decodes legacy EUC-KR/CP949 bytes into a UTF-8 Go string.
func DecodeEUCKR(b []byte) (string, error) {
	return DecodeEUCKRContext(Context{}, b)
}

// DecodeEUCKRContext decodes legacy EUC-KR/CP949 bytes with file/field context.
func DecodeEUCKRContext(ctx Context, b []byte) (string, error) {
	b = StripTerminalControls(b)
	if off := firstInvalidCP949Byte(b); off >= 0 {
		return "", conversionError(ctx, "decode", off, b, fmt.Errorf("malformed CP949 byte sequence"))
	}

	out, _, err := transform.Bytes(korean.EUCKR.NewDecoder(), b)
	if err != nil {
		return "", conversionError(ctx, "decode", -1, b, err)
	}
	if off := firstRuneErrorOffset(string(out)); off >= 0 {
		return "", conversionError(ctx, "decode", off, b, fmt.Errorf("decoder emitted replacement rune"))
	}
	return string(out), nil
}

// EncodeEUCKR encodes a UTF-8 Go string as legacy EUC-KR/CP949 bytes.
func EncodeEUCKR(s string) ([]byte, error) {
	return EncodeEUCKRContext(Context{}, s)
}

// EncodeEUCKRContext encodes a UTF-8 Go string with file/field context.
func EncodeEUCKRContext(ctx Context, s string) ([]byte, error) {
	if !utf8.ValidString(s) {
		return nil, conversionError(ctx, "encode", -1, []byte(s), fmt.Errorf("input is not valid UTF-8"))
	}
	out, _, err := transform.Bytes(korean.EUCKR.NewEncoder(), []byte(s))
	if err != nil {
		return nil, conversionError(ctx, "encode", -1, []byte(s), err)
	}
	return out, nil
}

// DecodePathBytes decodes an OS path represented as raw legacy bytes.
//
// Slash bytes are ASCII and are preserved by the codec, so this can be used on
// either a full relative path or a single path component.
func DecodePathBytes(path []byte) (string, error) {
	return DecodeEUCKRContext(Context{Path: string(path), Field: "path"}, path)
}

// ValidUTF8OrDecode returns b as a string when it is already valid UTF-8;
// otherwise it decodes b as EUC-KR/CP949.
func ValidUTF8OrDecode(b []byte) (string, error) {
	return ValidUTF8OrDecodeContext(Context{}, b)
}

// ValidUTF8OrDecodeContext is ValidUTF8OrDecode with file/field context.
func ValidUTF8OrDecodeContext(ctx Context, b []byte) (string, error) {
	if utf8.Valid(b) {
		return string(b), nil
	}
	return DecodeEUCKRContext(ctx, b)
}

// TrimCString returns bytes up to the first NUL. Legacy fixed-size C string
// fields are often NUL-padded and should be trimmed before text conversion.
func TrimCString(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

// DecodeCStringEUCKR trims a fixed-size C string and decodes it.
func DecodeCStringEUCKR(b []byte) (string, error) {
	return DecodeEUCKR(TrimCString(b))
}

// DecodeCStringEUCKRContext trims a fixed-size C string and decodes it with
// file/field context.
func DecodeCStringEUCKRContext(ctx Context, b []byte) (string, error) {
	return DecodeEUCKRContext(ctx, TrimCString(b))
}

// StripTerminalControls removes legacy terminal control sequences that were
// sometimes persisted inside text fields before the EUC-KR bytes.
func StripTerminalControls(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	var out []byte
	for i := 0; i < len(b); {
		switch b[i] {
		case 0x9b:
			end := csiEnd(b, i+1)
			if end < 0 {
				return appendControlCopy(out, b, i, i+1)
			}
			if out == nil {
				out = make([]byte, 0, len(b))
				out = append(out, b[:i]...)
			}
			i = end
			continue
		case 0x1b:
			if i+1 < len(b) && b[i+1] == '[' {
				end := csiEnd(b, i+2)
				if end >= 0 {
					if out == nil {
						out = make([]byte, 0, len(b))
						out = append(out, b[:i]...)
					}
					i = end
					continue
				}
			}
		}
		if out != nil {
			out = append(out, b[i])
		}
		i++
	}
	if out == nil {
		return b
	}
	return out
}

func csiEnd(b []byte, start int) int {
	for i := start; i < len(b); i++ {
		if b[i] >= 0x40 && b[i] <= 0x7e {
			return i + 1
		}
	}
	return -1
}

func appendControlCopy(out []byte, b []byte, start, end int) []byte {
	if out == nil {
		out = make([]byte, 0, len(b))
		out = append(out, b[:start]...)
	}
	return append(out, b[start:end]...)
}

func conversionError(ctx Context, op string, offset int, input []byte, err error) error {
	return &ConversionError{
		Op:     op,
		Path:   ctx.Path,
		Field:  ctx.Field,
		Offset: offset,
		Sample: sampleAround(input, offset),
		Err:    err,
	}
}

func sampleAround(input []byte, offset int) []byte {
	if len(input) == 0 {
		return nil
	}
	if offset < 0 || offset >= len(input) {
		offset = 0
	}
	start := offset - 4
	if start < 0 {
		start = 0
	}
	end := offset + 8
	if end > len(input) {
		end = len(input)
	}
	out := make([]byte, end-start)
	copy(out, input[start:end])
	return out
}

func firstRuneErrorOffset(s string) int {
	for off, r := range s {
		if r == utf8.RuneError {
			return off
		}
	}
	return -1
}

// firstInvalidCP949Byte catches malformed byte shapes before the transformer
// has a chance to replace them. It intentionally accepts CP949's broader trail
// byte ranges because old Korean Linux/Windows datasets often mix EUC-KR and
// CP949 filenames.
func firstInvalidCP949Byte(b []byte) int {
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c <= 0x7f {
			continue
		}
		if c == 0x80 || c == 0xff {
			return i
		}
		if i+1 >= len(b) {
			return i
		}
		t := b[i+1]
		if !isCP949Trail(t) {
			return i + 1
		}
		i++
	}
	return -1
}

func isCP949Trail(b byte) bool {
	switch {
	case b >= 0x41 && b <= 0x5a:
		return true
	case b >= 0x61 && b <= 0x7a:
		return true
	case b >= 0x81 && b <= 0xfe:
		return true
	default:
		return false
	}
}
