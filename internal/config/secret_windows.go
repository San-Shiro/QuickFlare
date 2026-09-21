//go:build windows

package config

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	crypt32            = windows.NewLazySystemDLL("crypt32.dll")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procCryptProtect   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotect = crypt32.NewProc("CryptUnprotectData")
	procLocalFree      = kernel32.NewProc("LocalFree")
)

// dataBlob is Win32 DATA_BLOB.
type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(b []byte) dataBlob {
	if len(b) == 0 {
		return dataBlob{}
	}
	return dataBlob{cbData: uint32(len(b)), pbData: &b[0]}
}

// bytes copies the blob's contents out before it is freed.
func (b dataBlob) bytes() []byte {
	if b.cbData == 0 || b.pbData == nil {
		return nil
	}
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	return out
}

func (b dataBlob) free() {
	if b.pbData != nil {
		procLocalFree.Call(uintptr(unsafe.Pointer(b.pbData)))
	}
}

// entropy is mixed into the DPAPI blob so that another program running as the
// same Windows user cannot decrypt the token just by asking DPAPI nicely.
//
// It is compiled into the binary, so it is not a secret from anyone holding
// the executable - it raises the bar from "any process on this account reads
// it for free" to "must know this value", nothing more. The real protection
// is DPAPI itself: the ciphertext is bound to the Windows account, so copying
// the config file to another machine or user yields nothing.
var entropy = []byte("quickflare.cloudflare.api-token.v1")

// protect encrypts plaintext for the current Windows user.
func protect(plain []byte) ([]byte, error) {
	in := newBlob(plain)
	ent := newBlob(entropy)
	var out dataBlob

	ret, _, err := procCryptProtect.Call(
		uintptr(unsafe.Pointer(&in)),
		0, // description
		uintptr(unsafe.Pointer(&ent)),
		0, // reserved
		0, // prompt struct
		0, // flags
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer out.free()
	return out.bytes(), nil
}

// unprotect decrypts a blob written by protect on this machine and account.
func unprotect(enc []byte) ([]byte, error) {
	in := newBlob(enc)
	ent := newBlob(entropy)
	var out dataBlob

	ret, _, err := procCryptUnprotect.Call(
		uintptr(unsafe.Pointer(&in)),
		0, // description out
		uintptr(unsafe.Pointer(&ent)),
		0, // reserved
		0, // prompt struct
		0, // flags
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer out.free()
	return out.bytes(), nil
}

// secretsAreEncrypted reports whether stored secrets get real protection on
// this platform, so the UI can be honest about it.
const secretsAreEncrypted = true
