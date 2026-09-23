package ipc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/San-Shiro/QuickFlare/internal/config"
)

const (
	tokenHeader   = "X-QuickFlare-Token"
	ipcFileName   = "ipc.json"
	statusTimeout = 2 * time.Second
)

// Info is written to ipc.json by the running QuickFlare tray server.
type Info struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
}

// StatusData is returned by GET /api/status.
type StatusData struct {
	OK            bool   `json:"ok"`
	PID           int    `json:"pid"`
	Running       bool   `json:"running"`
	Disabled      bool   `json:"disabled"`
	RoutesCount   int    `json:"routes_count"`
	EngineVersion string `json:"engine_version"`
}

// Handlers defines the callbacks invoked when IPC commands arrive.
type Handlers struct {
	OnQuit   func()
	OnPause  func() error
	OnResume func() error
	OnReload func() error
	OnOpen   func() error
	OnStatus func() StatusData
}

// Server provides authenticated loopback control for QuickFlare.
type Server struct {
	handlers Handlers
	listener net.Listener
	server   *http.Server
	infoPath string
	info     Info
	mu       sync.Mutex
	closed   bool
}

// ipcPath returns the path to ipc.json.
func ipcPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ipcFileName), nil
}

// StartServer starts the loopback HTTP server, writes ipc.json, and returns the Server.
func StartServer(h Handlers) (*Server, error) {
	path, err := ipcPath()
	if err != nil {
		return nil, fmt.Errorf("locate ipc path: %w", err)
	}

	// Listen on 127.0.0.1:0 for dynamic port assignment
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("bind loopback: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port

	// Generate 32-byte cryptographically secure token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		ln.Close()
		return nil, fmt.Errorf("generate secret token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	info := Info{
		PID:       os.Getpid(),
		Port:      port,
		Token:     token,
		CreatedAt: time.Now().UTC(),
	}

	raw, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		ln.Close()
		return nil, fmt.Errorf("marshal ipc info: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		ln.Close()
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	if err := os.WriteFile(path, raw, 0600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("write %s: %w", path, err)
	}

	s := &Server{
		handlers: h,
		listener: ln,
		infoPath: path,
		info:     info,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/quit", s.authMiddleware(s.handleQuit))
	mux.HandleFunc("/api/pause", s.authMiddleware(s.handlePause))
	mux.HandleFunc("/api/resume", s.authMiddleware(s.handleResume))
	mux.HandleFunc("/api/reload", s.authMiddleware(s.handleReload))
	mux.HandleFunc("/api/open", s.authMiddleware(s.handleOpen))
	mux.HandleFunc("/api/status", s.authMiddleware(s.handleStatus))

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		_ = s.server.Serve(ln)
	}()

	return s, nil
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqToken := r.Header.Get(tokenHeader)
		if reqToken == "" || reqToken != s.info.Token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "shutting down"})

	if s.handlers.OnQuit != nil {
		go func() {
			time.Sleep(100 * time.Millisecond)
			s.handlers.OnQuit()
		}()
	}
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var err error
	if s.handlers.OnPause != nil {
		err = s.handlers.OnPause()
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "paused": true})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var err error
	if s.handlers.OnResume != nil {
		err = s.handlers.OnResume()
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "resumed": true})
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var err error
	if s.handlers.OnReload != nil {
		err = s.handlers.OnReload()
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "reloaded": true})
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var err error
	if s.handlers.OnOpen != nil {
		err = s.handlers.OnOpen()
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "opened": true})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var st StatusData
	if s.handlers.OnStatus != nil {
		st = s.handlers.OnStatus()
	}
	st.OK = true
	st.PID = s.info.PID
	st.Running = true

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// Close stops the HTTP server and deletes ipc.json.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_ = s.server.Shutdown(ctx)
	_ = s.listener.Close()
	_ = os.Remove(s.infoPath)
	return nil
}

// Client interacts with a running QuickFlare tray instance.
type Client struct {
	info   Info
	client *http.Client
}

// Discover connects to the active QuickFlare instance if running.
func Discover() (*Client, error) {
	path, err := ipcPath()
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("quickflare is not running")
	}
	if err != nil {
		return nil, fmt.Errorf("read ipc file: %w", err)
	}

	var info Info
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, fmt.Errorf("parse ipc file: %w", err)
	}

	if !isProcessAlive(info.PID) {
		// Stale file from an ungraceful exit
		_ = os.Remove(path)
		return nil, errors.New("quickflare is not running (stale session cleaned up)")
	}

	c := &Client{
		info: info,
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
	}

	// Verify health via status
	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()

	if _, err := c.Status(ctx); err != nil {
		return nil, fmt.Errorf("quickflare process found (PID %d) but IPC not responding: %w", info.PID, err)
	}

	return c, nil
}

// PID returns the server's process ID.
func (c *Client) PID() int {
	return c.info.PID
}

func (c *Client) post(ctx context.Context, endpoint string) error {
	url := "http://127.0.0.1:" + strconv.Itoa(c.info.Port) + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set(tokenHeader, c.info.Token)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ipc error (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// Quit requests the running QuickFlare tray application to exit gracefully.
func (c *Client) Quit(ctx context.Context) error {
	return c.post(ctx, "/api/quit")
}

// Pause pauses the route forwarding connector in the running tray.
func (c *Client) Pause(ctx context.Context) error {
	return c.post(ctx, "/api/pause")
}

// Resume resumes the route forwarding connector in the running tray.
func (c *Client) Resume(ctx context.Context) error {
	return c.post(ctx, "/api/resume")
}

// Reload requests the running tray application to reload routes & settings from disk.
func (c *Client) Reload(ctx context.Context) error {
	return c.post(ctx, "/api/reload")
}

// Open toggles or brings the running tray panel to the front.
func (c *Client) Open(ctx context.Context) error {
	return c.post(ctx, "/api/open")
}

// Status fetches real-time status from the running tray application.
func (c *Client) Status(ctx context.Context) (*StatusData, error) {
	url := "http://127.0.0.1:" + strconv.Itoa(c.info.Port) + "/api/status"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(tokenHeader, c.info.Token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ipc error (status %d): %s", resp.StatusCode, string(body))
	}

	var data StatusData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// isProcessAlive checks if a process with the given PID is running.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		return isWindowsProcessAlive(pid)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, sending signal 0 checks for process existence without killing it.
	return proc.Signal(syscallSignalZero()) == nil
}
