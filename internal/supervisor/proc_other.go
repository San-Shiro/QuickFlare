//go:build !windows

package supervisor

import "os/exec"

func configureProc(cmd *exec.Cmd) {}

// superviseProcess is the Windows job-object binding; see job_windows.go.
// On Unix the process group and context cancellation already cover this.
func superviseProcess(pid int) {}

func candidateDirs() []string {
	return []string{"/usr/local/bin", "/usr/bin", "/opt/homebrew/bin"}
}
