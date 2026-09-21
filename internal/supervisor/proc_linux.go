//go:build linux

package supervisor

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Linux's answer to the Windows job object.
//
// job_windows.go exists because TerminateProcess runs no cleanup, so a killed
// QuickFlare used to leave its connectors running - six of them accumulated
// on one machine in a day. Linux has the same hole: SIGKILL is not catchable,
// and a crash or `systemctl kill` runs no deferred code.
//
// Two settings close it, and they do different jobs:
//
//   - Pdeathsig asks the kernel to SIGKILL this child when its parent dies.
//     This is the half that survives a SIGKILL of the parent, which nothing
//     in userspace can.
//   - Setpgid puts the child in its own process group, so a Ctrl-C in the
//     terminal is delivered to quickflare alone rather than to quickflare and
//     cloudflared at once. The connector then stops in a defined order -
//     supervisor first, child second - instead of both racing the same
//     signal.
//
// One caveat worth knowing: Pdeathsig is tied to the *thread* that forked,
// not to the process. If the Go runtime retires that thread while the child
// is still running, the child is signalled early. The connector is started
// once from a long-lived supervisor goroutine and the runtime does not retire
// threads eagerly, so this has not been observed - but it is a real edge, and
// it is the reason context cancellation still does the ordinary shutdown
// rather than this being the only mechanism.
func configureProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
}

// superviseProcess is a no-op here. The guarantee is established at fork time
// by configureProc, rather than applied to an already-running process as it
// has to be on Windows.
func superviseProcess(pid int) {}

// candidateDirs is where a distro or a manual install is likely to have put
// cloudflared, checked after PATH.
func candidateDirs() []string {
	dirs := []string{
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
		"/opt/cloudflared",
		"/snap/bin",
	}
	// A per-user install, for anyone without root on the box - which is a
	// normal way to run this.
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "bin"),
		)
	}
	return dirs
}
