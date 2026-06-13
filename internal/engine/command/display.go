package command

import (
	"strings"
	"unicode"

	"github.com/0xc0de1ab/muhan/internal/textfmt"
)

func cleanDisplayText(text string) string {
	return renderDisplayText(text, textfmt.Options{})
}

func cleanDescriptionText(text string) string {
	return renderBlockText(text, textfmt.Options{})
}

func renderDisplayText(text string, opts textfmt.Options) string {
	return strings.TrimSpace(textfmt.RenderLegacyColors(text, opts))
}

func renderBlockText(text string, opts textfmt.Options) string {
	text = textfmt.RenderLegacyColors(text, opts)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return strings.TrimRightFunc(text, unicode.IsSpace)
}

func textOptionsFromContext(ctx *Context) textfmt.Options {
	return textfmt.Options{
		ANSI:   boolContextValue(ctx, ContextANSIKey),
		Bright: boolContextValue(ctx, ContextANSIBrightKey),
	}
}

func boolContextValue(ctx *Context, key string) bool {
	if ctx == nil || ctx.Values == nil {
		return false
	}
	value, ok := ctx.Values[key]
	if !ok {
		return false
	}
	enabled, ok := value.(bool)
	return ok && enabled
}

func colorText(opts textfmt.Options, code string, text string) string {
	if !opts.ANSI || text == "" {
		return text
	}
	rendered, err := (textfmt.Renderer{Options: opts}).Format("%C%s%D", code, text, "0")
	if err != nil {
		return text
	}
	return rendered
}
