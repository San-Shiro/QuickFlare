// Package supervisor runs and watches the cloudflared child process.
package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// State is the connector's lifecycle state.
type State int

const (
	StateStopped State = iota
	StateStarting
	StateConnected
	StateReconnecting
	StateFailed
)

func (s State) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateStarting:
		return "starting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	case StateFailed:
		return "failed"
	}
	return "unknown"
}

// Status is a snapshot of the connector.
type Status struct {
	State       State
	Connections int
	Err         error
	Since       time.Time
}

// Tunnel supervises one cloudflared process running a named tunnel.
type Tunnel struct {
	binPath string

	mu     sync.RWMutex
	status Status
	cancel context.CancelFunc

	logs   chan string
	onStat func(Status)
}

// NewTunnel returns a supervisor for the cloudflared at binPath.
func NewTunnel(binPath string) *Tunnel {
	return &Tunnel{
		binPath: binPath,
		status:  Status{State: StateStopped, Since: time.Now()},
		logs:    make(chan string, 256),
	}
}

// OnStatus registers a callback fired on every state change. It runs on the
// supervisor's goroutine, so it must not block.
func (t *Tunnel) OnStatus(f func(Status)) { t.onStat = f }

// Logs streams cloudflared's stderr. Lines are dropped rather than queued when
// nobody is reading, so a chatty connector can never stall the process.
func (t *Tunnel) Logs() <-chan string { return t.logs }

// Status returns the current snapshot.
func (t *Tunnel) Status() Status {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

func (t *Tunnel) setState(s State, conns int, err error) {
	t.mu.Lock()
	changed := t.status.State != s || t.status.Connections != conns
	t.status = Status{State: s, Connections: conns, Err: err, Since: time.Now()}
	snap := t.status
	t.mu.Unlock()

	if changed && t.onStat != nil {
		t.onStat(snap)
	}
}

// Start launches cloudflared and keeps it running until Stop or ctx ends.
// It returns once the process has been spawned, not once it is connected;
// watch Status for that.
func (t *Tunnel) Start(ctx context.Context, token string) error {
	if token == "" {
		return errors.New("supervisor: empty tunnel token")
	}
	t.mu.Lock()
	if t.cancel != nil {
		t.mu.Unlock()
		return errors.New("supervisor: already running")
	}
	runCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	t.mu.Unlock()

	go t.run(runCtx, token)
	return nil
}

// Stop terminates the process and stops restarting it.
func (t *Tunnel) Stop() {
	t.mu.Lock()
	cancel := t.cancel
	t.cancel = nil
	t.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	t.setState(StateStopped, 0, nil)
}

// run owns the restart loop.
func (t *Tunnel) run(ctx context.Context, token string) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		err := t.runOnce(ctx, token)
		if ctx.Err() != nil {
			return
		}

		t.setState(StateReconnecting, 0, err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// runOnce spawns cloudflared and blocks until it exits.
func (t *Tunnel) runOnce(ctx context.Context, token string) error {
	port, err := freePort()
	if err != nil {
		return fmt.Errorf("allocate metrics port: %w", err)
	}
	metrics := fmt.Sprintf("127.0.0.1:%d", port)

	// --no-autoupdate matters: an auto-update restarts the process underneath
	// us, which reads as an unexplained disconnect.
	cmd := exec.CommandContext(ctx, t.binPath,
		"tunnel",
		"--no-autoupdate",
		"--metrics", metrics,
		"run",
		"--token", token,
	)
	configureProc(cmd)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	t.setState(StateStarting, 0, nil)
	if err := cmd.Start(); err != nil {
		t.setState(StateFailed, 0, err)
		return fmt.Errorf("start cloudflared: %w", err)
	}
	// Bound to the kill-on-close job before anything else, so even an
	// immediate hard kill of this process takes the connector with it.
	superviseProcess(cmd.Process.Pid)

	go t.pumpLogs(stderr)

	pollCtx, stopPoll := context.WithCancel(ctx)
	defer stopPoll()
	go t.pollReady(pollCtx, metrics)

	return cmd.Wait()
}

// pumpLogs forwards stderr lines, dropping them if nobody is listening.
func (t *Tunnel) pumpLogs(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		select {
		case t.logs <- sc.Text():
		default:
		}
	}
}

// readyResponse is what cloudflared's /ready endpoint returns.
type readyResponse struct {
	Status           int `json:"status"`
	ReadyConnections int `json:"readyConnections"`
}

// pollReady tracks connectivity via the metrics endpoint.
//
// The alternative - matching "Registered tunnel connection" in stderr - breaks
// whenever cloudflared rewords a log line. /ready is a real interface and
// reports the connection count, which the log lines do not.
func (t *Tunnel) pollReady(ctx context.Context, metricsAddr string) {
	url := "http://" + metricsAddr + "/ready"
	client := &http.Client{Timeout: 2 * time.Second}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			// Metrics server not up yet, or the process is going down.
			continue
		}
		var ready readyResponse
		dec := json.NewDecoder(io.LimitReader(resp.Body, 64<<10))
		decErr := dec.Decode(&ready)
		resp.Body.Close()
		if decErr != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK && ready.ReadyConnections > 0 {
			t.setState(StateConnected, ready.ReadyConnections, nil)
		} else {
			t.setState(StateStarting, ready.ReadyConnections, nil)
		}
	}
}

// freePort reserves an ephemeral port and releases it. There is a small race
// before cloudflared binds it; on collision the connector restarts and draws
// a new one.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// FindBinary locates cloudflared: application directory first, then the
// inbuilt embedded binary, then PATH and candidate directories.
func FindBinary() (string, error) {
	name := "cloudflared"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	// 1. Next to the running executable (e.g. MSI installation folder)
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}

	// 2. Inbuilt embedded binary (auto-extracted to LocalAppData)
	if p, err := EnsureEmbeddedBinary(); err == nil && p != "" {
		return p, nil
	}

	// 3. Fallback: PATH
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}

	// 4. Fallback: Common install directories
	for _, dir := range candidateDirs() {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("cloudflared engine not available")
}

// EngineVersion returns the version of the cloudflared binary.
func EngineVersion(binPath string) string {
	if binPath != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, "--version")
		configureProc(cmd)
		if out, err := cmd.Output(); err == nil {
			line := strings.TrimSpace(string(out))
			if strings.HasPrefix(line, "cloudflared version ") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					return parts[2]
				}
			}
			if line != "" {
				return line
			}
		}
	}
	return EmbeddedCloudflaredVersion
}
