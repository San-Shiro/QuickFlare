package ui

import (
	"net"
	"testing"
)

func TestExtractPort(t *testing.T) {
	tests := []struct {
		target string
		want   int
	}{
		{"localhost:3000", 3000},
		{"127.0.0.1:8080", 8080},
		{"[::1]:9000", 9000},
		{":5000", 5000},
		{"4000", 4000},
		{"  localhost:3000  ", 3000},
		{"invalid", 0},
		{"", 0},
		{"localhost:99999", 0},
		{"localhost:-1", 0},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := extractPort(tt.target)
			if got != tt.want {
				t.Errorf("extractPort(%q) = %d, want %d", tt.target, got, tt.want)
			}
		})
	}
}

func TestCheckPortListening(t *testing.T) {
	// Start a real TCP listener on an ephemeral loopback port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test listener: %v", err)
	}
	defer l.Close()

	port := l.Addr().(*net.TCPAddr).Port

	// Verify the open port reports listening
	if !checkPortListening(port) {
		t.Errorf("checkPortListening(%d) = false, want true for open listener", port)
	}

	// Close the listener and verify it reports false
	l.Close()
	if checkPortListening(port) {
		t.Errorf("checkPortListening(%d) = true, want false after listener closed", port)
	}

	// Test boundary / invalid ports
	if checkPortListening(0) {
		t.Errorf("checkPortListening(0) = true, want false")
	}
	if checkPortListening(-1) {
		t.Errorf("checkPortListening(-1) = true, want false")
	}
	if checkPortListening(70000) {
		t.Errorf("checkPortListening(70000) = true, want false")
	}
}
