package main

import (
	"encoding/json"
	"github.com/phoedox/phoedox-plugins/internal/integ"
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const samplePage = `{
  "config": {"slug": "infra", "title": "Infrastructure"},
  "incident": null,
  "publicGroupList": [
    {"id": 1, "name": "Servers", "weight": 1, "monitorList": [
      {"id": 1, "name": "NAS (ping)", "sendUrl": 0, "type": "ping"},
      {"id": 2, "name": "Nextcloud", "sendUrl": 1, "type": "http", "url": "https://cloud.example.org/status.php"},
      {"id": 3, "name": "Backup job", "sendUrl": 0, "type": "push"}
    ]},
    {"id": 2, "name": "Network", "weight": 2, "monitorList": [
      {"id": "4", "name": "core-sw", "type": "ping"},
      {"id": 1, "name": "NAS (ping)", "type": "ping"}
    ]}
  ],
  "maintenanceList": []
}`

const sampleBeats = `{
  "heartbeatList": {
    "1": [
      {"status": 1, "time": "2026-11-14 08:00:00.000", "msg": "", "ping": 2},
      {"status": 0, "time": "2026-11-14 08:01:00.512", "msg": "Request timed out", "ping": null}
    ],
    "2": [{"status": 1, "time": "2026-11-14 08:01:10", "msg": "200 - OK", "ping": 87}],
    "4": [{"status": 3, "time": "2026-11-14 08:00:30.000", "msg": "Maintenance window"}]
  },
  "uptimeList": {"1_24": 0.98765, "2_24": 1, "4_24": 0.5}
}`

func entities() []pdk.Entity {
	return []pdk.Entity{
		{Kind: "server", Slug: "nas", Name: "NAS", Attrs: map[string]any{"hostname": "nas.home.lan", "ip": "192.168.1.10/24"}},
		{Kind: "service", Slug: "nextcloud", Name: "Nextcloud", Attrs: map[string]any{"hostname": "cloud.example.org"}},
		{Kind: "switch", Slug: "core-switch", Name: "Core Switch", Attrs: map[string]any{"hostname": "CORE-SW.home.lan"}},
	}
}

func TestStatus(t *testing.T) {
	var page StatusPage
	var beats Heartbeats
	if err := json.Unmarshal([]byte(samplePage), &page); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(sampleBeats), &beats); err != nil {
		t.Fatal(err)
	}
	list := monitors(page, beats, integ.NewMatcher(entities()))
	if len(list) != 4 {
		t.Fatalf("duplicate monitor not skipped: %+v", list)
	}
	nas := list[0]
	if nas.Status != "down" || nas.Message != "Request timed out" || nas.LastCheck != "2026-11-14T08:01:00Z" || nas.Group != "Servers" || *nas.Uptime24h != 98.77 {
		t.Fatalf("nas: %+v", nas)
	}
	if nas.Entry == nil || nas.Entry.Slug != "nas" {
		t.Fatalf("nas entry (name without bracket): %+v", nas.Entry)
	}
	nc := list[1]
	if nc.Status != "up" || *nc.Uptime24h != 100 || nc.Entry == nil || nc.Entry.Slug != "nextcloud" || nc.LastCheck != "2026-11-14T08:01:10Z" {
		t.Fatalf("nextcloud: %+v %+v", nc, nc.Entry)
	}
	if list[2].Status != "unknown" || list[2].Uptime24h != nil || list[2].Entry != nil {
		t.Fatalf("push monitor without beats: %+v", list[2])
	}
	if sw := list[3]; sw.Status != "maintenance" || sw.Entry == nil || sw.Entry.Slug != "core-switch" {
		t.Fatalf("switch by short host name: %+v", sw)
	}
	s := summarize(list)
	if s.Counts["down"] != 1 || s.Counts["up"] != 1 || s.Monitors[0].Status != "down" || s.Monitors[3].Status != "up" {
		t.Fatalf("summary: %+v", s)
	}
	down := summarize(notUp(list))
	if len(down.Monitors) != 3 || down.Counts["up"] != 0 {
		t.Fatalf("down: %+v", down)
	}
	if empty := summarize(notUp(nil)); empty.Monitors == nil {
		t.Fatal("empty list must encode as []")
	}
}

func TestStatusMapping(t *testing.T) {
	for in, want := range map[int]string{0: "down", 1: "up", 2: "pending", 3: "maintenance", 9: "unknown"} {
		if statusName(in) != want {
			t.Fatal(in)
		}
	}
	if !validSlug("infra-2") || validSlug("../x") || validSlug("") {
		t.Fatal("slug")
	}
	if isoTime("garbage") != "garbage" {
		t.Fatal("time fallback")
	}
	var bad StatusPage
	if json.Unmarshal([]byte(`{"publicGroupList":[{"monitorList":[{"id":{}}]}]}`), &bad) == nil {
		t.Fatal("bad id accepted")
	}
}
