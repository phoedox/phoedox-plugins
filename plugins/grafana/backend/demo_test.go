package main

import (
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// TestDemoData parses the recorded demo answers the screenshots use.
func TestDemoData(t *testing.T) {
	const dir = "../testdata/fake"
	body, err := fakeapi.Body(dir, "GET", searchPath(""))
	if err != nil {
		t.Fatal(err)
	}
	dash, err := parseDashboards(body, "https://grafana.home.arpa")
	if err != nil || len(dash) != 5 {
		t.Fatalf("%v %d", err, len(dash))
	}
	if dash[1].Title != "Homelab nodes" || dash[1].Folder != "Homelab" || dash[1].URL != "https://grafana.home.arpa/d/homelab-nodes/homelab-nodes" {
		t.Errorf("%+v", dash[1])
	}

	body, err = fakeapi.Body(dir, "GET", alertsPath)
	if err != nil {
		t.Fatal(err)
	}
	alerts, err := parseAlerts(body)
	if err != nil || len(alerts) != 4 {
		t.Fatalf("%v %d", err, len(alerts))
	}
	m := integ.NewMatcher([]pdk.Entity{
		{Kind: "server", Slug: "nas-01", Name: "nas-01", Attrs: map[string]any{"hostname": "nas-01.home.arpa", "ip": "10.20.0.10"}},
		{Kind: "vm", Slug: "home-assistant", Name: "Home Assistant", Attrs: map[string]any{"ip": "10.20.0.21"}},
		{Kind: "appliance", Slug: "chest-freezer", Name: "Chest freezer"},
	})
	rows := alertRows(alerts, m)
	want := []string{"home-assistant", "chest-freezer", "nas-01"}
	if len(rows) != len(want) {
		t.Fatalf("pending alert not dropped: %+v", rows)
	}
	for i, r := range rows {
		if r.Entry == nil || r.Entry.Slug != want[i] {
			t.Errorf("%s: entry %v, want %s", r.Name, r.Entry, want[i])
		}
	}
}
