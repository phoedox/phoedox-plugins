package main

import (
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const demoDir = "../testdata/fake"

// demoGraph holds the entries the screenshots create (shots.json).
func demoGraph() *integ.Matcher {
	return integ.NewMatcher([]pdk.Entity{
		{Kind: "server", Slug: "nas-01", Name: "nas-01", Attrs: map[string]any{"hostname": "nas-01.home.arpa", "ip": "10.20.0.10"}},
		{Kind: "server", Slug: "pve-01", Name: "pve-01", Attrs: map[string]any{"hostname": "pve-01.home.arpa", "ip": "10.20.0.11"}},
		{Kind: "vm", Slug: "home-assistant", Name: "Home Assistant", Attrs: map[string]any{"ip": "10.20.0.21"}},
		{Kind: "router", Slug: "router", Name: "Router", Attrs: map[string]any{"ip": "10.20.0.1"}},
	})
}

// TestDemoData parses the recorded demo answers the screenshots use.
func TestDemoData(t *testing.T) {
	api := apiBase("/home")
	body, err := fakeapi.Body(demoDir, "GET", api+problemsPath())
	if err != nil {
		t.Fatal(err)
	}
	list, err := parseServices(body)
	if err != nil || len(list) != 4 {
		t.Fatalf("%v %d", err, len(list))
	}
	rows := problemRows(list, demoGraph())
	if len(rows) != 4 || rows[0].State != "CRIT" || rows[3].State != "WARN" {
		t.Fatalf("%+v", rows)
	}
	for _, r := range rows {
		if r.Entry == nil || r.Since == "" {
			t.Errorf("%s/%s: entry %v, since %q", r.Host, r.Service, r.Entry, r.Since)
		}
	}
	if rows[0].Entry.Slug != "home-assistant" || !rows[0].Acknowledged {
		t.Errorf("first problem: %+v", rows[0])
	}

	body, err = fakeapi.Body(demoDir, "GET", api+hostsPath())
	if err != nil {
		t.Fatal(err)
	}
	hosts, err := parseHosts(body)
	if err != nil || len(hosts) != 5 {
		t.Fatalf("%v %d", err, len(hosts))
	}
	hr := hostRows(hosts, demoGraph())
	if hr[0].Name != "cam-garden" || hr[0].State != "DOWN" || hr[0].Entry != nil {
		t.Errorf("%+v", hr[0])
	}
}
