package app

import "strings"

// scrollTextModel holds the title + body content for a scrollable read-only
// screen (card detail, users, tags, account info). The actual viewport lives
// on appModel so it is shared with the list screens; this struct is just the
// data payload.
type scrollTextModel struct {
	title   string
	content string
}

func newScrollTextModel(title, content string) scrollTextModel {
	return scrollTextModel{title: title, content: content}
}

// padRight left-justifies s in a field of the given width (byte width, like the
// rest of the app which assumes mostly-ASCII values).
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
