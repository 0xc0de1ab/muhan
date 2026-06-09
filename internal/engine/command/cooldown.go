package command

import "fmt"

func renderPleaseWait(seconds int64) string {
	if seconds <= 1 {
		return "1초만 기다리세요.\n"
	}
	return fmt.Sprintf("%d초동안 기다리세요.\n", seconds)
}
