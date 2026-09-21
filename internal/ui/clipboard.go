package ui

import (
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
)

// writeClipboard copies text via Gio's input router. It must be called with a
// live frame context - the command is queued on the frame, not executed
// immediately, which is why copying happens during input handling rather than
// from a background goroutine.
func writeClipboard(gtx layout.Context, s string) {
	gtx.Execute(clipboard.WriteCmd{
		Type: "application/text",
		Data: io.NopCloser(strings.NewReader(s)),
	})
}
