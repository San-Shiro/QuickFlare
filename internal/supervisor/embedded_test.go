package supervisor

import (
	"os"
	"testing"
)

func TestFindBinary(t *testing.T) {
	p, err := FindBinary()
	if err != nil {
		t.Fatalf("FindBinary failed: %v", err)
	}
	if p == "" {
		t.Fatal("FindBinary returned empty path")
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat on found binary failed: %v", err)
	}
	if st.IsDir() {
		t.Fatalf("binary %s is a directory", p)
	}

	ver := EngineVersion(p)
	if ver == "" {
		t.Fatal("EngineVersion returned empty string")
	}
	t.Logf("Found cloudflared at %s (version: %s)", p, ver)
}
