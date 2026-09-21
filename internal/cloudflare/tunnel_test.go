package cloudflare

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeIngressAlwaysEndsWithOneCatchAll(t *testing.T) {
	cases := map[string][]IngressRule{
		"empty":              {},
		"no catch-all":       {{Hostname: "a.example.com", Service: "http://localhost:1"}},
		"already has one":    {{Hostname: "a.example.com", Service: "http://localhost:1"}, {Service: CatchAllService}},
		"several catch-alls": {{Service: CatchAllService}, {Hostname: "a.example.com", Service: "http://localhost:1"}, {Service: CatchAllService}},
	}

	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			got := normalizeIngress(in)
			if len(got) == 0 {
				t.Fatal("expected at least the catch-all")
			}
			last := got[len(got)-1]
			if last.Hostname != "" || last.Service != CatchAllService {
				t.Errorf("last rule is not the catch-all: %+v", last)
			}
			for i, r := range got[:len(got)-1] {
				if r.Hostname == "" {
					t.Errorf("rule %d is a stray catch-all before the end: %+v", i, r)
				}
			}
		})
	}
}

// fakeAPI serves the configurations endpoint and records what was written.
type fakeAPI struct {
	current []IngressRule
	written *TunnelConfigDoc
	puts    int
}

func (f *fakeAPI) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case "GET":
			json.NewEncoder(w).Encode(map[string]any{
				"success": true, "errors": []any{},
				"result": tunnelConfigEnvelope{
					TunnelID: "t1", Version: 3,
					Config: TunnelConfigDoc{Ingress: f.current},
				},
			})
		case "PUT":
			f.puts++
			body, _ := io.ReadAll(r.Body)
			var got struct {
				Config TunnelConfigDoc `json:"config"`
			}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Errorf("bad PUT body: %v", err)
			}
			f.written = &got.Config
			json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": nil})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
}

func TestAddRoutePreservesExistingRules(t *testing.T) {
	f := &fakeAPI{current: []IngressRule{
		{Hostname: "keep.example.com", Service: "http://localhost:1111"},
		{Service: CatchAllService},
	}}
	srv := f.server(t)
	defer srv.Close()

	c := New("tok", "acct", WithBaseURL(srv.URL))
	if err := c.AddRoute(context.Background(), "t1", "new.example.com", "http://localhost:2222"); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}

	if f.puts != 1 {
		t.Fatalf("expected exactly one PUT, got %d", f.puts)
	}
	got := f.written.Ingress
	if len(got) != 3 {
		t.Fatalf("expected 2 routes + catch-all, got %d: %+v", len(got), got)
	}
	if got[0].Hostname != "keep.example.com" {
		t.Errorf("pre-existing rule was dropped: %+v", got)
	}
	if got[1].Hostname != "new.example.com" || got[1].Service != "http://localhost:2222" {
		t.Errorf("new rule wrong: %+v", got[1])
	}
	if got[2].Service != CatchAllService {
		t.Errorf("catch-all not last: %+v", got)
	}
}

func TestAddRouteIsIdempotentForSameHostname(t *testing.T) {
	f := &fakeAPI{current: []IngressRule{
		{Hostname: "app.example.com", Service: "http://localhost:1111"},
		{Service: CatchAllService},
	}}
	srv := f.server(t)
	defer srv.Close()

	c := New("tok", "acct", WithBaseURL(srv.URL))
	if err := c.AddRoute(context.Background(), "t1", "app.example.com", "http://localhost:9999"); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}

	got := f.written.Ingress
	if len(got) != 2 {
		t.Fatalf("expected the hostname replaced, not duplicated: %+v", got)
	}
	if got[0].Service != "http://localhost:9999" {
		t.Errorf("target not updated: %+v", got[0])
	}
}

func TestRemoveRouteDropsOnlyThatHostname(t *testing.T) {
	f := &fakeAPI{current: []IngressRule{
		{Hostname: "a.example.com", Service: "http://localhost:1"},
		{Hostname: "b.example.com", Service: "http://localhost:2"},
		{Service: CatchAllService},
	}}
	srv := f.server(t)
	defer srv.Close()

	c := New("tok", "acct", WithBaseURL(srv.URL))
	if err := c.RemoveRoute(context.Background(), "t1", "a.example.com"); err != nil {
		t.Fatalf("RemoveRoute: %v", err)
	}

	got := f.written.Ingress
	if len(got) != 2 || got[0].Hostname != "b.example.com" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestAPIErrorSurfacesCloudflareCodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"errors":  []APIErrorItem{{Code: 81053, Message: "record already exists"}},
		})
	}))
	defer srv.Close()

	c := New("tok", "acct", WithBaseURL(srv.URL))
	_, err := c.CreateDNSRecord(context.Background(), "z1", DNSRecord{Type: "CNAME", Name: "*"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !IsAlreadyExists(err) {
		t.Errorf("IsAlreadyExists should recognise 81053, got %v", err)
	}
}
