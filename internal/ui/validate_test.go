package ui

import "testing"

func TestValidateLabel(t *testing.T) {
	valid := []string{"app", "api2", "my-service", "a", "staging-01"}
	for _, l := range valid {
		if err := validateLabel(l); err != nil {
			t.Errorf("validateLabel(%q) rejected a legal label: %v", l, err)
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
			if err := validateLabel(l); err == nil {
				t.Errorf("validateLabel(%q) accepted an illegal label", l)
			}
		})
	}

	t.Run("too long", func(t *testing.T) {
		long := make([]byte, 64)
		for i := range long {
			long[i] = 'a'
		}
		if err := validateLabel(string(long)); err == nil {
			t.Error("accepted a label over 63 characters")
		}
	})
}

// A wildcard would hand the entire zone to this tunnel - the exact behaviour
// the per-route design moved away from. It must not be reachable from a form.
func TestValidateLabelRejectsWildcardExplicitly(t *testing.T) {
	err := validateLabel("*")
	if err == nil {
		t.Fatal("wildcard label must be rejected")
	}
	if got := err.Error(); got == "" {
		t.Error("rejection should explain why")
	}
}
