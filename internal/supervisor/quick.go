package supervisor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sync"
	"time"
)

// quickURLPattern matches the hostname cloudflared prints when it opens a
// quick tunnel.
var quickURLPattern = regexp.MustCompile(`https://[a-z0-9][a-z0-9-]*\.trycloudflare\.com`)

// QuickWaitTimeout is how long to wait for cloudflared to report a URL.
const QuickWaitTimeout = 45 * time.Second

// Quick is one anonymous tunnel from cloudflared's TryCloudflare service.
//
// These need no account, no domain and no DNS: cloudflared hands back a random
// hostname on trycloudflare.com. That also sets the limits - the URL is not
// yours, it changes on every restart, and anyone holding the link is in.
// Expiry is simply the lifetime of the process, which is why Close is the
// whole revocation story.
//
// Cloudflare caps a quick tunnel at 200 concurrent in-flight requests and does
// not support SSE on them; they are a debug aid, not a production target.
type Quick struct {
	Port    int
	Started time.Time

	mu     sync.RWMutex
	url    string
	err    error
	cancel context.CancelFunc
	done   chan struct{}
}

// URL returns the public address, empty until cloudflared reports one.
func (q *Quick) URL() string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.url
}

// Err reports why the tunnel stopped, if it did.
func (q *Quick) Err() error {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.err
}

// Close tears the tunnel down. The link dies with the process.
func (q *Quick) Close() {
	q.mu.Lock()
	cancel := q.cancel
	q.cancel = nil
	q.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Done is closed when the tunnel process exits.
func (q *Quick) Done() <-chan struct{} { return q.done }

// StartQuick opens an anonymous tunnel to a local port and blocks until
// cloudflared reports the public URL or the wait times out.
func StartQuick(ctx context.Context, binPath string, port int) (*Quick, error) {
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("supervisor: invalid port %d", port)
	}

	runCtx, cancel := context.WithCancel(ctx)
	q := &Quick{
		Port:    port,
		Started: time.Now(),
		cancel:  cancel,
		done:    make(chan struct{}),
	}

	target := fmt.Sprintf("http://localhost:%d", port)
	cmd := exec.CommandContext(runCtx, binPath,
		"tunnel",
		"--no-autoupdate",
		"--url", target,
	)
	configureProc(cmd)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start quick tunnel: %w", err)
	}
	superviseProcess(cmd.Process.Pid)

	found := make(chan string, 1)
	go scanForURL(stderr, found)

	go func() {
		waitErr := cmd.Wait()
		q.mu.Lock()
		if q.err == nil {
			q.err = waitErr
		}
		q.mu.Unlock()
		close(q.done)
	}()

	select {
	case url := <-found:
		q.mu.Lock()
		q.url = url
		q.mu.Unlock()
		return q, nil

	case <-q.done:
		q.Close()
		return nil, fmt.Errorf("cloudflared exited before reporting a URL: %w", q.Err())

	case <-runCtx.Done():
		q.Close()
		return nil, runCtx.Err()

	case <-time.After(QuickWaitTimeout):
		q.Close()
		return nil, errors.New("timed out waiting for a trycloudflare.com URL")
	}
}

// scanForURL watches stderr for the public hostname. cloudflared prints it
// inside a box of ASCII art rather than in any structured form, so matching
// the hostname pattern is the only option available.
func scanForURL(r io.Reader, found chan<- string) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	sent := false
	for sc.Scan() {
		if sent {
			continue // drain, so cloudflared never blocks writing stderr
		}
		if m := quickURLPattern.FindString(sc.Text()); m != "" {
			found <- m
			sent = true
		}
	}
}
