package command

import (
	"fmt"
	"strings"
)

func renderLegacyViewFile(ctx *Context, content string, pendingError string) (Status, error) {
	read := &legacyViewFileReadState{text: content, pendingError: pendingError}
	status, err := read.render(ctx)
	if err != nil {
		return status, err
	}
	if status == StatusDefault {
		return StatusDoPrompt, nil
	}
	return status, nil
}

// RenderLegacyViewFile renders text with the legacy C view_file paging surface.
func RenderLegacyViewFile(ctx *Context, content string, pendingError string) (Status, error) {
	return renderLegacyViewFile(ctx, content, pendingError)
}

// LegacyViewFileContinuePrompt is the prompt emitted by C view_file pagination.
const LegacyViewFileContinuePrompt = postReadContinuePrompt

// LegacyViewFilePage splits content using the same byte/line page boundaries as C view_file.
func LegacyViewFilePage(content string, start int) (string, int) {
	return postReadPage(content, start)
}

type legacyViewFileReadState struct {
	text         string
	next         int
	pendingError string
}

func (s *legacyViewFileReadState) handleLine(ctx *Context, line string) (Status, error) {
	if strings.HasPrefix(line, ".") {
		ClearPendingLineHandler(ctx)
		ctx.WriteString("중단합니다.\n")
		return StatusDefault, nil
	}
	return s.render(ctx)
}

func (s *legacyViewFileReadState) render(ctx *Context) (Status, error) {
	if s == nil || s.next >= len(s.text) {
		ClearPendingLineHandler(ctx)
		return StatusDefault, nil
	}

	page, next := postReadPage(s.text, s.next)
	s.next = next
	ctx.WriteString(page)
	if s.next < len(s.text) {
		ctx.WriteString(postReadContinuePrompt)
		if !SetPendingLineHandler(ctx, s.handleLine) {
			if s.pendingError == "" {
				s.pendingError = "파일 읽기 상태를 시작할 수 없습니다"
			}
			return StatusDefault, fmt.Errorf("%s", s.pendingError)
		}
		return StatusDoPrompt, nil
	}

	ClearPendingLineHandler(ctx)
	return StatusDefault, nil
}
