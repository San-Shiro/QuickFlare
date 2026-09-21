//go:build !windows

package supervisor

import (
	"fmt"
)

// EmbeddedCloudflaredVersion is the version of cloudflared embedded in QuickFlare.
const EmbeddedCloudflaredVersion = "2026.9.1"

// EnsureEmbeddedBinary is a no-op on non-Windows platforms.
func EnsureEmbeddedBinary() (string, error) {
	return "", fmt.Errorf("inbuilt cloudflared is only packaged for Windows")
}
