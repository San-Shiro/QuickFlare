package ui

// Middle-truncation for public hostnames, per the design spec.
//
// Head and tail are not arbitrary. A quick tunnel's hostname is three random
// words plus "trycloudflare.com", which overflows the 196dp hostname column;
// truncating the head would lose the only part that distinguishes one tunnel
// from another, and truncating the tail would lose the registrable domain.
// Keeping 17 trailing runes is exactly "trycloudflare.com", so the domain
// always survives intact while the leading random word stays scannable.
const (
	truncHead  = 10
	truncTail  = 17
	truncLimit = 29
)

// truncateHost shortens a hostname from the middle, leaving the registrable
// domain intact. It operates on runes, never bytes, so a multi-byte character
// is never split into mojibake.
func truncateHost(host string) string {
	r := []rune(host)
	if len(r) <= truncLimit {
		return host
	}
	// U+2026 HORIZONTAL ELLIPSIS is one rune, so the result renders as 28.
	return string(r[:truncHead]) + "…" + string(r[len(r)-truncTail:])
}
