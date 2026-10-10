package ui

import (
	"testing"
	"time"

	"github.com/San-Shiro/QuickFlare/internal/config"
	"github.com/San-Shiro/QuickFlare/internal/core"
)

func TestDeleteRouteEnqueuesAndRemovesFromUIImmediately(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	p := &Panel{
		routes: []Route{
			{Route: core.Route{Hostname: "api1.example.com", Target: "localhost:3000", ZoneID: "z1"}},
			{Route: core.Route{Hostname: "api2.example.com", Target: "localhost:3001", ZoneID: "z1"}},
			{Route: core.Route{Hostname: "api3.example.com", Target: "localhost:3002", ZoneID: "z1"}},
		},
		tunnelID:   "tun-123",
		deleteWake: make(chan struct{}, 1),
		deleteStop: make(chan struct{}),
	}

	// 1. Delete first route
	p.deleteRoute("api1.example.com")

	if len(p.routes) != 2 {
		t.Fatalf("expected 2 routes in UI list, got %d", len(p.routes))
	}
	if p.routes[0].Hostname != "api2.example.com" || p.routes[1].Hostname != "api3.example.com" {
		t.Errorf("unexpected remaining routes: %+v", p.routes)
	}

	p.deleteMu.Lock()
	qLen := len(p.deleteQueue)
	var firstJob config.StoredDeletionJob
	if qLen > 0 {
		firstJob = p.deleteQueue[0]
	}
	p.deleteMu.Unlock()

	if qLen != 1 {
		t.Fatalf("expected deleteQueue length 1, got %d", qLen)
	}
	if firstJob.Hostname != "api1.example.com" || firstJob.ZoneID != "z1" || firstJob.TunnelID != "tun-123" {
		t.Errorf("unexpected job fields: %+v", firstJob)
	}

	// 2. Delete second route while first is still in queue
	p.deleteRoute("api2.example.com")

	if len(p.routes) != 1 {
		t.Fatalf("expected 1 route in UI list, got %d", len(p.routes))
	}
	if p.routes[0].Hostname != "api3.example.com" {
		t.Errorf("unexpected remaining route: %s", p.routes[0].Hostname)
	}

	p.deleteMu.Lock()
	qLen = len(p.deleteQueue)
	jobs := make([]config.StoredDeletionJob, qLen)
	copy(jobs, p.deleteQueue)
	p.deleteMu.Unlock()

	if qLen != 2 {
		t.Fatalf("expected deleteQueue length 2, got %d", qLen)
	}
	if jobs[0].Hostname != "api1.example.com" || jobs[1].Hostname != "api2.example.com" {
		t.Errorf("expected FIFO order in queue, got %+v", jobs)
	}

	// Verify status message indicates queue count
	if p.status != "Queued api2 for removal (2 in queue)" {
		t.Errorf("unexpected status string: %q", p.status)
	}

	// Verify persistence in config.json
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}
	if len(cfg.PendingDeletions) != 2 {
		t.Fatalf("expected 2 pending deletions in config, got %d", len(cfg.PendingDeletions))
	}
	if cfg.PendingDeletions[0].Hostname != "api1.example.com" || cfg.PendingDeletions[1].Hostname != "api2.example.com" {
		t.Errorf("unexpected pending deletions in config: %+v", cfg.PendingDeletions)
	}
}

func TestDeleteQueueRestoresOnLaunch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Pre-seed config with 2 pending deletions
	cfg := &config.Config{
		Domain: "example.com",
		PendingDeletions: []config.StoredDeletionJob{
			{Hostname: "stale1.example.com", ZoneID: "z1", TunnelID: "tun-99", CreatedAt: time.Now()},
			{Hostname: "stale2.example.com", ZoneID: "z1", TunnelID: "tun-99", CreatedAt: time.Now()},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("cfg.Save failed: %v", err)
	}

	p := &Panel{
		deleteWake: make(chan struct{}, 1),
		deleteStop: make(chan struct{}),
	}
	p.restore()

	p.deleteMu.Lock()
	restoredCount := len(p.deleteQueue)
	p.deleteMu.Unlock()

	if restoredCount != 2 {
		t.Fatalf("expected 2 restored jobs in queue, got %d", restoredCount)
	}

	// Check wake channel received signal
	select {
	case <-p.deleteWake:
		// OK
	default:
		t.Errorf("expected worker to be signaled after restore")
	}
}

func TestDeleteWorkerDrainsQueueFIFO(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	p := &Panel{
		deleteWake: make(chan struct{}, 1),
		deleteStop: make(chan struct{}),
		post:       make(chan func(), 32),
	}

	// Drain post channel in background
	go func() {
		for {
			select {
			case <-p.deleteStop:
				return
			case f := <-p.post:
				f()
			}
		}
	}()

	p.deleteMu.Lock()
	p.deleteQueue = []config.StoredDeletionJob{
		{Hostname: "q1.example.com", ZoneID: "z1", TunnelID: "tun-1", CreatedAt: time.Now()},
		{Hostname: "q2.example.com", ZoneID: "z1", TunnelID: "tun-1", CreatedAt: time.Now()},
	}
	p.deleteMu.Unlock()

	p.deleteWG.Add(1)
	go p.runDeleteWorker()

	// Wake worker
	p.signalDeleteWorker()

	// Wait for queue to drain
	deadline := time.Now().Add(5 * time.Second)
	var drained bool
	for time.Now().Before(deadline) {
		p.deleteMu.Lock()
		count := len(p.deleteQueue)
		p.deleteMu.Unlock()
		if count == 0 {
			drained = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !drained {
		p.deleteMu.Lock()
		t.Fatalf("expected queue to drain, remaining: %+v", p.deleteQueue)
		p.deleteMu.Unlock()
	}

	// Shutdown worker
	p.Shutdown()

	// Verify config reflects empty queue
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}
	if len(cfg.PendingDeletions) != 0 {
		t.Errorf("expected 0 pending deletions after drain, got %d", len(cfg.PendingDeletions))
	}
}
