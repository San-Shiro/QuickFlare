package ui

import (
	"fmt"
	"os"
)

// debugEnabled turns on placement tracing via QUICKFLARE_DEBUG=1. Window
// placement is the one part of this app that cannot be reasoned about from the
// source alone, so it gets a trace switch rather than print statements that
// come and go.
var debugEnabled = os.Getenv("QUICKFLARE_DEBUG") == "1"

func dbg(format string, args ...any) {
	if debugEnabled {
		fmt.Fprintf(os.Stderr, "[place] "+format+"\n", args...)
	}
}
