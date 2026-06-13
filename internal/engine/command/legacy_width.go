package command

import (
	"strings"

	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
)

func legacyLeftWidthBytes(text string, width int) string {
	return legacyByteWidthLabel(text, width, false)
}

func legacyFixedByteLabel(text string, width int) string {
	return legacyByteWidthLabel(strings.TrimSpace(text), width, true)
}

func legacyByteLen(text string) int {
	encoded, err := legacykr.EncodeEUCKR(text)
	if err != nil {
		return len([]byte(text))
	}
	return len(encoded)
}

func legacyByteWidthLabel(text string, width int, truncate bool) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range text {
		part := string(r)
		size := len(part)
		if encoded, err := legacykr.EncodeEUCKR(part); err == nil {
			size = len(encoded)
		}
		if truncate && used+size > width {
			break
		}
		b.WriteRune(r)
		used += size
	}
	for used < width {
		b.WriteByte(' ')
		used++
	}
	return b.String()
}

func legacyTruncateBytes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range text {
		part := string(r)
		size := len(part)
		if encoded, err := legacykr.EncodeEUCKR(part); err == nil {
			size = len(encoded)
		}
		if used+size > limit {
			break
		}
		b.WriteRune(r)
		used += size
	}
	return b.String()
}
