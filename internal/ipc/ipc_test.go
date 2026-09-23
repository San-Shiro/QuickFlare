package ipc

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestIPCServerAndClient(t *testing.T) {
	// Sandbox config directory for the test so we don't overwrite user's ipc.json
	tmp := t.TempDir()
	t.Setenv("APPDATA", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	var (
		paused   atomic.Bool
		reloaded atomic.Bool
		opened   atomic.Bool
		quitCh   = make(chan struct{}, 1)
	)

	server, err := StartServer(Handlers{
		OnQuit: func() {
			select {
			case quitCh <- struct{}{}:
			default:
			}
		},
		OnPause: func() error {
			paused.Store(true)
			return nil
		},
		OnResume: func() error {
			paused.Store(false)
			return nil
		},
		OnReload: func() error {
			reloaded.Store(true)
			return nil
		},
		OnOpen: func() error {
			opened.Store(true)
			return nil
		},
		OnStatus: func() StatusData {
			return StatusData{
				Disabled:      paused.Load(),
				RoutesCount:   3,
				EngineVersion: "2026.9.1",
			}
		},
	})
	if err != nil {
		t.Fatalf("StartServer failed: %v", err)
	}
	defer server.Close()

	// Verify ipc.json was created
	path, err := ipcPath()
	if err != nil {
		t.Fatalf("ipcPath failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected ipc.json to exist at %s: %v", path, err)
	}

	// Client Discover
	client, err := Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if client.PID() != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), client.PID())
	}

	ctx := context.Background()

	// Test Status
	st, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("client.Status failed: %v", err)
	}
	if !st.OK || !st.Running || st.RoutesCount != 3 || st.EngineVersion != "2026.9.1" {
		t.Errorf("unexpected status: %+v", st)
	}

	// Test Pause & Resume
	if err := client.Pause(ctx); err != nil {
		t.Fatalf("client.Pause failed: %v", err)
	}
	if !paused.Load() {
		t.Errorf("expected paused=true")
	}

	if err := client.Resume(ctx); err != nil {
		t.Fatalf("client.Resume failed: %v", err)
	}
	if paused.Load() {
		t.Errorf("expected paused=false")
	}

	// Test Reload
	if err := client.Reload(ctx); err != nil {
		t.Fatalf("client.Reload failed: %v", err)
	}
	if !reloaded.Load() {
		t.Errorf("expected reloaded=true")
	}

	// Test Open
	if err := client.Open(ctx); err != nil {
		t.Fatalf("client.Open failed: %v", err)
	}
	if !opened.Load() {
		t.Errorf("expected opened=true")
	}

	// Test unauthorized request
	url := fmt.Sprintf("http://127.0.0.1:%d/api/pause", client.info.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unauthorized request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized without token, got %d", resp.StatusCode)
	}

	// Test Quit
	if err := client.Quit(ctx); err != nil {
		t.Fatalf("client.Quit failed: %v", err)
	}
	select {
	case <-quitCh:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for OnQuit callback")
	}

	// Test Close removes ipc.json
	if err := server.Close(); err != nil {
		t.Fatalf("server.Close failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected ipc.json to be deleted after Close, stat err=%v", err)
	}
}
