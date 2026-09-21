package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// dnsFake serves the records endpoint from an in-memory zone.
type dnsFake struct {
	records []DNSRecord
	created []DNSRecord
	updated []DNSRecord
}

func (f *dnsFake) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case "GET":
			q := r.URL.Query()
			var hits []DNSRecord
			for _, rec := range f.records {
				if rec.Type == q.Get("type") && rec.Name == q.Get("name") {
					hits = append(hits, rec)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true, "errors": []any{}, "result": hits,
			})
		case "POST":
			var rec DNSRecord
			json.NewDecoder(r.Body).Decode(&rec)
			rec.ID = "new-id"
			f.created = append(f.created, rec)
			json.NewEncoder(w).Encode(map[string]any{
				"success": true, "errors": []any{}, "result": rec,
			})
		case "PUT":
			var rec DNSRecord
			json.NewDecoder(r.Body).Decode(&rec)
			f.updated = append(f.updated, rec)
			json.NewEncoder(w).Encode(map[string]any{
				"success": true, "errors": []any{}, "result": rec,
			})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
}

func TestEnsureRouteCNAMECreatesWhenFree(t *testing.T) {
	f := &dnsFake{}
	srv := f.server(t)
	defer srv.Close()

	c := New("tok", "acct", WithBaseURL(srv.URL))
	rec, err := c.EnsureRouteCNAME(context.Background(), "z1", "app.example.com", "tunnel-uuid")
	if err != nil {
		t.Fatalf("EnsureRouteCNAME: %v", err)
	}
	if rec.Content != "tunnel-uuid.cfargotunnel.com" {
		t.Errorf("wrong target: %q", rec.Content)
	}
	if !rec.Proxied {
		t.Error("record must be proxied - an unproxied CNAME to cfargotunnel does not route")
	}
	if len(f.created) != 1 {
		t.Fatalf("expected exactly one record created, got %d", len(f.created))
	}
}

// The safety-critical case: a name already serving something else must not be
// silently repointed at our tunnel.
func TestEnsureRouteCNAMERefusesToHijackExistingRecord(t *testing.T) {
	for _, existing := range []DNSRecord{
		{Type: "A", Name: "app.example.com", Content: "203.0.113.5"},
		{Type: "CNAME", Name: "app.example.com", Content: "someone-elses-site.example.net"},
		{Type: "AAAA", Name: "app.example.com", Content: "2001:db8::1"},
	} {
		t.Run(existing.Type, func(t *testing.T) {
			f := &dnsFake{records: []DNSRecord{existing}}
			srv := f.server(t)
			defer srv.Close()

			c := New("tok", "acct", WithBaseURL(srv.URL))
			_, err := c.EnsureRouteCNAME(context.Background(), "z1", "app.example.com", "tunnel-uuid")
			if err == nil {
				t.Fatal("expected a conflict error, got nil")
			}
			if !strings.Contains(err.Error(), existing.Type) {
				t.Errorf("error should say which record type is in the way, got: %v", err)
			}
			if len(f.created) != 0 {
				t.Error("must not create a record over an existing one")
			}
		})
	}
}

// Re-running a finished route is safe: the record is already ours.
func TestEnsureRouteCNAMEIsIdempotent(t *testing.T) {
	f := &dnsFake{records: []DNSRecord{{
		ID: "existing", Type: "CNAME", Name: "app.example.com",
		Content: "tunnel-uuid.cfargotunnel.com", Proxied: true,
	}}}
	srv := f.server(t)
	defer srv.Close()

	c := New("tok", "acct", WithBaseURL(srv.URL))
	rec, err := c.EnsureRouteCNAME(context.Background(), "z1", "app.example.com", "tunnel-uuid")
	if err != nil {
		t.Fatalf("re-running should succeed: %v", err)
	}
	if rec.ID != "existing" {
		t.Errorf("should have adopted the existing record, got %+v", rec)
	}
	if len(f.created) != 0 {
		t.Error("should not have created a duplicate")
	}
}

func TestHostnameConflictReportsFreeNames(t *testing.T) {
	f := &dnsFake{records: []DNSRecord{
		{Type: "CNAME", Name: "taken.example.com", Content: "elsewhere"},
	}}
	srv := f.server(t)
	defer srv.Close()

	c := New("tok", "acct", WithBaseURL(srv.URL))
	got, err := c.HostnameConflict(context.Background(), "z1", "free.example.com")
	if err != nil {
		t.Fatalf("HostnameConflict: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for a free hostname, got %+v", got)
	}
}

// A record pointing at a different tunnel is almost always ours from before:
// the tunnel is looked up by name, so renaming it - or losing it - mints a
// new id while the old DNS keeps pointing at the previous one. Refusing
// those would strand every route the moment the tunnel changed.
func TestEnsureRouteCNAMEAdoptsRecordFromAnotherTunnel(t *testing.T) {
	stale := "bd8d3676-1262-495b-aaaa-000000000000.cfargotunnel.com"
	f := &dnsFake{records: []DNSRecord{{
		ID: "stale", Type: "CNAME", Name: "app.example.com",
		Content: stale, Proxied: true, Comment: RecordComment,
	}}}
	srv := f.server(t)
	defer srv.Close()

	c := New("tok", "acct", WithBaseURL(srv.URL))
	rec, err := c.EnsureRouteCNAME(context.Background(), "z1", "app.example.com", "new-tunnel")
	if err != nil {
		t.Fatalf("a stale tunnel record should be repointed, not refused: %v", err)
	}
	if rec.Content != "new-tunnel.cfargotunnel.com" {
		t.Errorf("record should now point at the current tunnel, got %q", rec.Content)
	}
	if len(f.updated) != 1 {
		t.Fatalf("expected the existing record to be updated, got %d updates", len(f.updated))
	}
	if len(f.created) != 0 {
		t.Error("should reuse the existing record rather than creating a duplicate")
	}
}

// Pointing at a tunnel is not proof of ownership. A hand-written cloudflared
// config, another tool, or the same domain driven from a second machine all
// produce tunnel records that are not ours - repointing them would hijack a
// working route.
func TestEnsureRouteCNAMERefusesForeignTunnelRecord(t *testing.T) {
	f := &dnsFake{records: []DNSRecord{{
		ID: "theirs", Type: "CNAME", Name: "app.example.com",
		Content: "somebody-elses-tunnel.cfargotunnel.com", Proxied: true,
		// No marker: not ours.
	}}}
	srv := f.server(t)
	defer srv.Close()

	c := New("tok", "acct", WithBaseURL(srv.URL))
	_, err := c.EnsureRouteCNAME(context.Background(), "z1", "app.example.com", "our-tunnel")
	if err == nil {
		t.Fatal("a tunnel record we did not create must not be repointed")
	}
	if len(f.updated) != 0 {
		t.Error("must not modify a record it does not own")
	}
	if len(f.created) != 0 {
		t.Error("must not create a competing record")
	}
}

// Records written under the old project name are still ours.
func TestEnsureRouteCNAMEAdoptsLegacyMarkedRecord(t *testing.T) {
	f := &dnsFake{records: []DNSRecord{{
		ID: "legacy", Type: "CNAME", Name: "app.example.com",
		Content: "old-tunnel.cfargotunnel.com", Proxied: true,
		Comment: legacyRecordComments[0],
	}}}
	srv := f.server(t)
	defer srv.Close()

	c := New("tok", "acct", WithBaseURL(srv.URL))
	if _, err := c.EnsureRouteCNAME(context.Background(), "z1", "app.example.com", "new-tunnel"); err != nil {
		t.Fatalf("a record from a previous version is still ours: %v", err)
	}
	if len(f.updated) != 1 {
		t.Error("expected the legacy record to be repointed")
	}
}

func TestOwnedByUs(t *testing.T) {
	cases := map[string]struct {
		rec  *DNSRecord
		want bool
	}{
		"current marker": {&DNSRecord{Comment: RecordComment}, true},
		"legacy marker":  {&DNSRecord{Comment: legacyRecordComments[0]}, true},
		"no comment":     {&DNSRecord{}, false},
		"other tool":     {&DNSRecord{Comment: "Managed by something else"}, false},
		"nil":            {nil, false},
	}
	for name, c := range cases {
		if got := OwnedByUs(c.rec); got != c.want {
			t.Errorf("%s: OwnedByUs = %v, want %v", name, got, c.want)
		}
	}
}
