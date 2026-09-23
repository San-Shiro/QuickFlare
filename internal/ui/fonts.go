package ui

import (
	_ "embed"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
)

//go:embed assets/fonts/Inter.ttf
var fontInter []byte

//go:embed assets/fonts/JetBrainsMono.ttf
var fontJetBrainsMono []byte

// loadFontCollection builds a typeface collection prioritizing Inter for all UI
// typography and JetBrains Mono for monospace labels (hostnames, ports, tokens),
// with gofont retained as a safe fallback.
func loadFontCollection() []font.FontFace {
	var collection []font.FontFace

	fInter, errInter := opentype.Parse(fontInter)
	fMono, errMono := opentype.Parse(fontJetBrainsMono)

	if errInter == nil {
		for _, tf := range []font.Typeface{"", "Inter", "sans-serif", "Go"} {
			collection = append(collection,
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.Normal}, Face: fInter},
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.Medium}, Face: fInter},
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.SemiBold}, Face: fInter},
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.Bold}, Face: fInter},
			)
		}
	}

	if errMono == nil {
		for _, tf := range []font.Typeface{faceMono, "monospace", "JetBrains Mono", "Go Mono"} {
			collection = append(collection,
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.Normal}, Face: fMono},
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.Medium}, Face: fMono},
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.SemiBold}, Face: fMono},
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.Bold}, Face: fMono},
			)
		}
	}

	// Always append gofont as safe fallback
	collection = append(collection, gofont.Collection()...)
	return collection
}
