package ui

import (
	"net"
	"strconv"
	"strings"
	"time"
)

// extractPort parses a port number from a target address string.
// Accepts formats like "localhost:3000", "127.0.0.1:8080", "[::1]:3000",
// ":3000", or a bare port "3000". Returns 0 if invalid.
func extractPort(target string) int {
	target = strings.TrimSpace(target)
	if target == "" {
		return 0
	}
	if p, err := strconv.Atoi(target); err == nil && p >= 1 && p <= 65535 {
		return p
	}
	_, portStr, err := net.SplitHostPort(target)
	if err == nil {
		if p, err := strconv.Atoi(portStr); err == nil && p >= 1 && p <= 65535 {
			return p
		}
	}
	if idx := strings.LastIndex(target, ":"); idx >= 0 {
		if p, err := strconv.Atoi(target[idx+1:]); err == nil && p >= 1 && p <= 65535 {
			return p
		}
	}
	return 0
}

// checkPortListening checks whether a local service is actively listening on
// the specified TCP port by attempting a fast loopback dial on IPv4 and IPv6.
// Returns true if a service accepts connection, false if closed or timed out.
func checkPortListening(port int) bool {
	if port < 1 || port > 65535 {
		return false
	}
	timeout := 150 * time.Millisecond

	// Probe IPv4 loopback first.
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), timeout)
	if err == nil {
		conn.Close()
		return true
	}

	// Try IPv6 loopback if IPv4 was not listening.
	conn, err = net.DialTimeout("tcp", net.JoinHostPort("::1", strconv.Itoa(port)), timeout)
	if err == nil {
		conn.Close()
		return true
	}

	return false
}
