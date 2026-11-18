package main

import (
	"encoding/json"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// TestDemoData parses the recorded demo answers the screenshots use.
func TestDemoData(t *testing.T) {
	const dir = "../testdata/fake"
	var page StatusPage
	var beats Heartbeats
	for path, out := range map[string]any{"/api/status-page/home": &page, "/api/status-page/heartbeat/home": &beats} {
		body, err := fakeapi.Body(dir, "GET", path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	m := integ.NewMatcher([]pdk.Entity{
		{Kind: "server", Slug: "nas-01", Name: "nas-01", Attrs: map[string]any{"ip": "10.20.0.10"}},
		{Kind: "server", Slug: "pve-01", Name: "pve-01", Attrs: map[string]any{"ip": "10.20.0.11"}},
		{Kind: "vm", Slug: "home-assistant", Name: "Home Assistant", Attrs: map[string]any{"ip": "10.20.0.21"}},
		{Kind: "router", Slug: "router", Name: "Router", Attrs: map[string]any{"ip": "10.20.0.1"}},
		{Kind: "device", Slug: "garden-camera", Name: "Garden camera"},
		{Kind: "service", Slug: "nextcloud", Name: "Nextcloud"},
		{Kind: "contract", Slug: "internet", Name: "Internet"},
	})
	s := summarize(monitors(page, beats, m))
	if len(s.Monitors) != 7 || s.Counts["up"] != 4 || s.Counts["down"] != 1 || s.Counts["pending"] != 1 || s.Counts["maintenance"] != 1 {
		t.Fatalf("%+v", s.Counts)
	}
	first := s.Monitors[0]
	if first.Name != "Home Assistant" || first.Status != "down" || first.Uptime24h == nil || *first.Uptime24h != 97.12 || first.LastCheck != "2026-06-15T07:59:14Z" {
		t.Errorf("%+v", first)
	}
	for _, mon := range s.Monitors {
		if mon.Entry == nil {
			t.Errorf("%s: no entry", mon.Name)
		}
	}
}
