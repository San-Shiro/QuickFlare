package ui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateHost(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "short hostname is untouched",
			in:   "app.example.com",
			want: "app.example.com",
		},
		{
			name: "exactly at the limit is untouched",
			in:   strings.Repeat("a", truncLimit),
			want: strings.Repeat("a", truncLimit),
		},
		{
			name: "quick tunnel keeps the registrable domain",
			in:   "brave-lion-runs-fast.trycloudflare.com",
			want: "brave-lion…trycloudflare.com",
		},
		{
			name: "one rune over the limit truncates",
			in:   strings.Repeat("b", truncLimit+1),
			want: strings.Repeat("b", truncHead) + "…" + strings.Repeat("b", truncTail),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := truncateHost(c.in); got != c.want {
				t.Errorf("truncateHost(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
		})
	}
}

// The design sizes the hostname column against a 28-rune render. If truncation
// ever produced more, hostnames would overflow the pill.
func TestTruncateHostRendersAt28Runes(t *testing.T) {
	long := "brave-lion-runs-very-fast-indeed.trycloudflare.com"
	got := truncateHost(long)
	if n := utf8.RuneCountInString(got); n != 28 {
		t.Errorf("truncated hostname renders as %d runes, design assumes 28: %q", n, got)
	}
}

// Truncation must split runes, not bytes - a multi-byte hostname must never
// come back as invalid UTF-8.
func TestTruncateHostIsRuneSafe(t *testing.T) {
	in := strings.Repeat("ü", 40)
	got := truncateHost(in)
	if !utf8.ValidString(got) {
		t.Fatalf("truncation produced invalid UTF-8: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != truncHead+1+truncTail {
		t.Errorf("got %d runes, want %d", n, truncHead+1+truncTail)
	}
}
