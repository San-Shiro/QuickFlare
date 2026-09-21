//go:build windows

package supervisor

import (
	"fmt"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dbgJob traces job-object setup under QUICKFLARE_DEBUG=1. Failures here are
// survivable - the app still works, it just loses the guarantee - so they are
// traced rather than surfaced.
func dbgJob(format string, args ...any) {
	if os.Getenv("QUICKFLARE_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "[job] "+format+"\n", args...)
	}
}

// Kill-on-close job object: the only cleanup that survives being killed.
//
// Shutdown() tears the connectors down on a normal exit, but "normal" covers
// less than it sounds. An MSI upgrade terminates the running app so it can
// replace the exe, Task Manager terminates it, and a crash takes it out
// entirely - and none of those run a single line of Go. Every one of them
// used to leave cloudflared running: six orphaned connectors accumulated on
// one machine in a single day of reinstalls, each still serving the routes of
// an app that was no longer there.
//
// No amount of deferred cleanup fixes that, because TerminateProcess does not
// ask. A job object does: the children are bound to it, and when the last
// handle to the job closes - which the kernel does when this process dies,
// however it dies - JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE kills everything
// inside it.

var (
	jobOnce   sync.Once
	jobHandle windows.Handle
)

// job returns the process-wide job object, creating it on first use. A zero
// handle means the job could not be set up; callers carry on without it
// rather than refusing to start a tunnel.
func job() windows.Handle {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			dbgJob("CreateJobObject: %v", err)
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		if _, err := windows.SetInformationJobObject(
			h,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		); err != nil {
			dbgJob("SetInformationJobObject: %v", err)
			windows.CloseHandle(h)
			return
		}
		jobHandle = h
	})
	return jobHandle
}

// superviseProcess binds a started child to the job object so it cannot
// outlive this process.
//
// Assignment happens after Start rather than by creating the child suspended:
// cloudflared spawns nothing of its own, so there is no window in which a
// grandchild could escape, and this keeps the exec plumbing ordinary.
func superviseProcess(pid int) {
	j := job()
	if j == 0 {
		return
	}
	// PROCESS_SET_QUOTA and PROCESS_TERMINATE are exactly what
	// AssignProcessToJobObject requires; asking for more would fail against a
	// process we are only allowed to manage.
	h, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		dbgJob("OpenProcess(%d): %v", pid, err)
		return
	}
	defer windows.CloseHandle(h)

	if err := windows.AssignProcessToJobObject(j, h); err != nil {
		dbgJob("AssignProcessToJobObject(%d): %v", pid, err)
	}
}

// killGroup has no Windows meaning: the job object already owns the whole
// tree, and closing it takes every descendant with it.
