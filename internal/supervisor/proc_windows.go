//go:build windows

package supervisor

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// createNoWindow suppresses the console window that cloudflared would
// otherwise flash up on every start - unacceptable for a tray app.
const createNoWindow = 0x08000000

func configureProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

func candidateDirs() []string {
	var dirs []string
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if v := os.Getenv(env); v != "" {
			dirs = append(dirs, filepath.Join(v, "cloudflared"))
		}
	}
	return dirs
}
