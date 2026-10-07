package core

import "testing"

func TestValidateLabel(t *testing.T) {
	valid := []string{"app", "api2", "my-service", "a", "staging-01"}
	for _, l := range valid {
		if err := ValidateLabel(l); err != nil {
			t.Errorf("ValidateLabel(%q) rejected a legal label: %v", l, err)
		}
	}

	invalid := map[string]string{
		"wildcard takes the whole zone": "*",
		"two labels break TLS":          "app.staging",
		"leading hyphen":                "-app",
		"trailing hyphen":               "app-",
		"space":                         "my app",
		"slash":                         "app/admin",
		"at sign":                       "app@host",
		"underscore":                    "my_app",
	}
	for name, l := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := ValidateLabel(l); err == nil {
				t.Errorf("ValidateLabel(%q) accepted an illegal label", l)
			}
		})
	}

	t.Run("too long", func(t *testing.T) {
		long := make([]byte, 64)
		for i := range long {
			long[i] = 'a'
		}
		if err := ValidateLabel(string(long)); err == nil {
			t.Error("accepted a label over 63 characters")
		}
	})
}

// A wildcard would hand the entire zone to this tunnel - the exact behaviour
// the per-route design moved away from. It must not be reachable from a form.
func TestValidateLabelRejectsWildcardExplicitly(t *testing.T) {
	err := ValidateLabel("*")
	if err == nil {
		t.Fatal("wildcard label must be rejected")
	}
	if got := err.Error(); got == "" {
		t.Error("rejection should explain why")
	}
}

func TestNormalizeTarget(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"localhost:3000", "http://localhost:3000"},
		{"127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"http://localhost:3000", "http://localhost:3000"},
		{"https://localhost:8443", "https://localhost:8443"},
		{"tcp://localhost:5432", "tcp://localhost:5432"},
		{"  localhost:9000  ", "http://localhost:9000"},
		{"  https://10.0.0.1:443  ", "https://10.0.0.1:443"},
	}

	for _, tt := range tests {
		got := NormalizeTarget(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeTarget(%q) = %q, want %q", tt.input, got, tt.want)
		}
		// Test idempotency
		if double := NormalizeTarget(got); double != got {
			t.Errorf("NormalizeTarget not idempotent: NormalizeTarget(%q) = %q", got, double)
		}
	}
}
