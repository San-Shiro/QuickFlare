package ui

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/png"

	"gioui.org/op/paint"
)

//go:embed assets/logo.png
var logoPNG []byte

// logoOp is the decoded app logo, shared by every frame that draws it.
var logoOp paint.ImageOp

func init() {
	img, _, err := image.Decode(bytes.NewReader(logoPNG))
	if err != nil {
		// Leave logoOp zero; appMark falls back to a plain tile. A bad asset
		// should not stop the app from starting.
		dbg("logo decode failed: %v", err)
		return
	}
	logoOp = paint.NewImageOp(img)
}
