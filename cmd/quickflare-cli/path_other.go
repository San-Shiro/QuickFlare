//go:build !windows

package main

import (
	"context"
	"errors"
)

func cmdPath(ctx context.Context, args []string) error {
	return errors.New("the 'path' command is Windows-specific; on Linux/Unix, install the package (.deb/.rpm) or add the directory to your shell profile (~/.bashrc, ~/.zshrc)")
}
