//go:build !windows

package config

// Non-Windows builds have no DPAPI equivalent wired up yet. The token is
// stored as-is, protected only by file permissions (0600). macOS Keychain
// and libsecret are the right answers when those platforms are targeted;
// until then secretsAreEncrypted reports false so the UI can say so rather
// than implying protection that is not there.
func protect(plain []byte) ([]byte, error) { return plain, nil }

func unprotect(enc []byte) ([]byte, error) { return enc, nil }

const secretsAreEncrypted = false
