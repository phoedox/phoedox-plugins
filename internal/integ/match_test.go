// Tests of match.go; the same file is used by the monitoring plugins.
package integ

import (
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func TestMatcher(t *testing.T) {
	m := NewMatcher([]pdk.Entity{
		{Kind: "server", Slug: "nas", Name: "NAS", Attrs: map[string]any{"hostname": "nas.home.lan", "ip": "192.168.1.10/24"}},
		{Kind: "service", Slug: "nextcloud", Name: "Nextcloud", Attrs: map[string]any{"hostname": "cloud.example.org"}},
		{Kind: "switch", Slug: "core-switch", Name: "Core Switch", Attrs: map[string]any{"hostname": "CORE-SW.home.lan"}},
		{Kind: "server", Slug: "web01", Name: "web01", Attrs: map[string]any{"ip": []any{"10.0.0.5", "fd00::5"}}},
		{Kind: "server", Slug: "app-a", Name: "App A", Attrs: map[string]any{"hostname": "app.a.example.org"}},
		{Kind: "server", Slug: "app-b", Name: "App B", Attrs: map[string]any{"hostname": "app.b.example.org"}},
	})
	cases := map[string]string{
		"10.0.0.5:9100":            "web01",
		"[fd00::5]:9100":           "web01",
		"https://10.0.0.5/metrics": "web01",
		"WEB01.example.org.":       "web01",
		"192.168.1.10":             "nas",
		"nas":                      "nas",
		"core switch":              "core-switch",
		"core-switch":              "core-switch",
		"App.A.Example.org":        "app-a",
		"app":                      "", // ambiguous short name
		"10.0.0.6":                 "",
		"":                         "",
	}
	for in, want := range cases {
		got := ""
		if e := m.Match(in); e != nil {
			got = e.Slug
		}
		if got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	// Addresses win over names.
	if e := m.Match("Nextcloud", "192.168.1.10"); e == nil || e.Slug != "nas" {
		t.Fatalf("priority: %+v", e)
	}
	// Lookup is used for misses, at most once per name.
	calls := 0
	m.Lookup = func(q string) []pdk.Entity {
		calls++
		if q == "db01" {
			return []pdk.Entity{{Kind: "server", Slug: "db01", Name: "DB01"}}
		}
		return nil
	}
	if e := m.Match("db01.example.org"); e == nil || e.Slug != "db01" {
		t.Fatalf("lookup: %+v", e)
	}
	m.Match("zz.example.org")
	m.Match("zz.example.org")
	if calls != 2 {
		t.Fatalf("lookups: %d", calls)
	}
	var nilM *Matcher
	if nilM.Match("x") != nil {
		t.Fatal("nil matcher")
	}
}
