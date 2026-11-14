package main

import (
	"strings"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const searchAnswer = `[
  {"id": 12, "uid": "node-web01", "title": "Node Exporter Full", "uri": "db/node-exporter-full", "url": "/grafana/d/node-web01/node-exporter-full", "slug": "", "type": "dash-db", "tags": ["linux", "prometheus"], "isStarred": false, "folderId": 3, "folderUid": "infra", "folderTitle": "Infrastructure", "folderUrl": "/grafana/dashboards/f/infra/infrastructure", "sortMeta": 0},
  {"id": 13, "uid": "home", "title": "Home", "uri": "db/home", "url": "/grafana/d/home/home", "type": "dash-db", "tags": [], "isStarred": true},
  {"id": 3, "uid": "infra", "title": "Infrastructure", "url": "/grafana/dashboards/f/infra/infrastructure", "type": "dash-folder"}
]`

const alertsAnswer = `{"status": "success", "data": {"alerts": [
  {"labels": {"alertname": "HostDown", "grafana_folder": "Infrastructure", "instance": "10.0.0.5:9100", "job": "node", "severity": "critical"},
   "annotations": {"summary": "web01 is not reachable", "__orgId__": "1"},
   "state": "Alerting", "activeAt": "2026-11-14T07:55:00Z", "value": "0e+00"},
  {"labels": {"alertname": "DiskFull", "hostname": "nas.home.lan", "severity": "warning"},
   "annotations": {"description": "Disk on nas above 90 %"},
   "state": "Alerting (NoData)", "activeAt": "2026-11-14T08:10:00Z", "value": ""},
  {"labels": {"alertname": "HighLoad", "instance": "db01:9100"},
   "annotations": {}, "state": "Pending", "activeAt": "2026-11-14T08:12:00Z"},
  {"labels": {"alertname": "Watchdog"}, "annotations": {}, "state": "Normal", "activeAt": "0001-01-01T00:00:00Z"},
  {"labels": {"alertname": "Unknown host", "instance": "printer:9100"}, "annotations": {}, "state": "firing", "activeAt": "0001-01-01T00:00:00Z"}
]}}`

func TestDashboards(t *testing.T) {
	list, err := parseDashboards([]byte(searchAnswer), "https://mon.example.org/grafana")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("folder not dropped: %+v", list)
	}
	d := list[0]
	if d.Title != "Node Exporter Full" || d.Folder != "Infrastructure" || d.URL != "https://mon.example.org/grafana/d/node-web01/node-exporter-full" || len(d.Tags) != 2 {
		t.Fatalf("dashboard: %+v", d)
	}
	if list[1].Folder != "General" || list[1].Tags == nil {
		t.Fatalf("general: %+v", list[1])
	}
	for base, p := range map[string]string{
		"https://g.example.org":         "/d/a/b",
		"https://g.example.org/grafana": "/d/a/b",
	} {
		if got := absURL(base, p); got != base+"/d/a/b" {
			t.Errorf("%s %s: %s", base, p, got)
		}
	}
	if absURL("https://g.example.org/graf", "/grafana/d/x") != "https://g.example.org/graf/grafana/d/x" || absURL("https://g", "https://other/d/x") != "https://other/d/x" {
		t.Fatal("prefix must match a whole path segment")
	}
	if searchPath("") != "/api/search?type=dash-db&limit=500" || searchPath(" node & co ") != "/api/search?type=dash-db&limit=500&query=node+%26+co" {
		t.Fatal(searchPath(" node & co "))
	}
	if len(searchPath(strings.Repeat("ä", 300))) > 1300 {
		t.Fatal("query not cut")
	}
	if _, err := parseDashboards([]byte(`{"message":"x"}`), "https://g"); err == nil {
		t.Fatal("object accepted")
	}
}

func TestAlerts(t *testing.T) {
	list, err := parseAlerts([]byte(alertsAnswer))
	if err != nil {
		t.Fatal(err)
	}
	m := integ.NewMatcher([]pdk.Entity{
		{Kind: "server", Slug: "web01", Name: "web01", Attrs: map[string]any{"ip": "10.0.0.5"}},
		{Kind: "server", Slug: "nas", Name: "NAS", Attrs: map[string]any{"hostname": "nas.home.lan"}},
		{Kind: "server", Slug: "db01", Name: "db01"},
	})
	rows := alertRows(list, m)
	if len(rows) != 3 {
		t.Fatalf("pending and normal not dropped: %+v", rows)
	}
	disk, down, printer := rows[0], rows[1], rows[2]
	if disk.Name != "DiskFull" || disk.Summary != "Disk on nas above 90 %" || disk.Instance != "nas.home.lan" || disk.Entry == nil || disk.Entry.Slug != "nas" {
		t.Fatalf("disk: %+v %+v", disk, disk.Entry)
	}
	if down.Severity != "critical" || down.Folder != "Infrastructure" || down.Since != "2026-11-14T07:55:00Z" || down.Summary != "web01 is not reachable" || down.Entry == nil || down.Entry.Slug != "web01" {
		t.Fatalf("down: %+v %+v", down, down.Entry)
	}
	if printer.Since != "" || printer.Entry != nil || printer.State != "firing" {
		t.Fatalf("printer: %+v", printer)
	}
	if rows := alertRows(nil, m); rows == nil {
		t.Fatal("empty list must encode as []")
	}
}

func TestErrors(t *testing.T) {
	if _, err := parseAlerts([]byte(`{"status":"error","errorType":"server_error","error":"rule store unavailable"}`)); err == nil || !strings.Contains(err.Error(), "rule store unavailable") {
		t.Fatal(err)
	}
	if _, err := parseAlerts([]byte(`not json`)); err == nil {
		t.Fatal("garbage accepted")
	}
	if err := apiError(401, []byte(`{"message":"invalid API key"}`)); !strings.Contains(err.Error(), "token") {
		t.Fatal(err)
	}
	if err := apiError(403, []byte(`{"message":"You'll need additional permissions to perform this action. Permissions needed: alert.rules:read"}`)); !strings.HasPrefix(err.Error(), "Grafana answered HTTP 403: You'll need") {
		t.Fatal(err)
	}
	if se, ok := apiError(500, nil).(*integ.StatusError); !ok || se.Status != 500 {
		t.Fatal("status error")
	}
}
