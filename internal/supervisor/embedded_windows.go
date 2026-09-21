//go:build windows

package supervisor

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

//go:embed assets/cloudflared.exe.gz
var embeddedCloudflaredGz []byte

// EmbeddedCloudflaredVersion is the version of cloudflared embedded in QuickFlare.
const EmbeddedCloudflaredVersion = "2026.9.1"

// embeddedExpectedSize is the uncompressed byte size of cloudflared.exe.
const embeddedExpectedSize int64 = 54976432

var (
	extractMu     sync.Mutex
	extractedPath string
)

// EnsureEmbeddedBinary extracts the embedded cloudflared executable to
// %LOCALAPPDATA%\QuickFlare\bin\cloudflared.exe if not already extracted.
func EnsureEmbeddedBinary() (string, error) {
	extractMu.Lock()
	defer extractMu.Unlock()

	if extractedPath != "" {
		if st, err := os.Stat(extractedPath); err == nil && st.Size() == embeddedExpectedSize {
			return extractedPath, nil
		}
	}

	localApp := os.Getenv("LOCALAPPDATA")
	if localApp == "" {
		localApp = os.Getenv("APPDATA")
	}
	if localApp == "" {
		var err error
		localApp, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("locate local app data: %w", err)
		}
	}

	binDir := filepath.Join(localApp, "QuickFlare", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("create bin dir: %w", err)
	}

	target := filepath.Join(binDir, "cloudflared.exe")
	if st, err := os.Stat(target); err == nil && st.Size() == embeddedExpectedSize {
		extractedPath = target
		return target, nil
	}

	if len(embeddedCloudflaredGz) == 0 {
		return "", fmt.Errorf("embedded cloudflared binary is empty")
	}

	gzReader, err := gzip.NewReader(bytes.NewReader(embeddedCloudflaredGz))
	if err != nil {
		return "", fmt.Errorf("open compressed cloudflared: %w", err)
	}
	defer gzReader.Close()

	tmpFile := filepath.Join(binDir, fmt.Sprintf("cloudflared.tmp.%d", os.Getpid()))
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return "", fmt.Errorf("create temporary cloudflared: %w", err)
	}

	n, copyErr := io.Copy(f, gzReader)
	closeErr := f.Close()

	if copyErr != nil {
		_ = os.Remove(tmpFile)
		return "", fmt.Errorf("decompress cloudflared: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpFile)
		return "", fmt.Errorf("flush temporary cloudflared: %w", closeErr)
	}
	if n != embeddedExpectedSize {
		_ = os.Remove(tmpFile)
		return "", fmt.Errorf("decompressed cloudflared size mismatch: got %d, expected %d", n, embeddedExpectedSize)
	}

	_ = os.Remove(target)
	if err := os.Rename(tmpFile, target); err != nil {
		// If rename failed because the target is locked/running, try using it if it exists.
		if st, statErr := os.Stat(target); statErr == nil && st.Size() == embeddedExpectedSize {
			_ = os.Remove(tmpFile)
			extractedPath = target
			return target, nil
		}
		_ = os.Remove(tmpFile)
		return "", fmt.Errorf("install cloudflared: %w", err)
	}

	extractedPath = target
	return target, nil
}
