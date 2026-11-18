package main

import (
	"strings"
	"testing"
	"time"

	"github.com/phoedox/phoedox-sdk/go/pdk"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
)

const devicesJSON = `{"devices":[
	{"addresses":["100.101.102.103","fd7a:115c:a1e0::1"],"authorized":true,"blocksIncomingConnections":false,
	 "clientVersion":"1.76.6-t2b6b5d7a9-g7f5f3d4b3","created":"2024-01-02T10:00:00Z","expires":"2027-01-01T10:00:00Z",
	 "hostname":"web01","id":"1234567890123456","isExternal":false,"keyExpiryDisabled":false,"lastSeen":"2026-11-14T09:58:00Z",
	 "machineKey":"mkey:aaaa","name":"web01.tail1234.ts.net","nodeId":"nAbCdEf1CNTRL","nodeKey":"nodekey:bbbb","os":"linux",
	 "tags":["tag:server","tag:prod"],"updateAvailable":true,"user":"tagged-devices","connectedToControl":true},
	{"addresses":["fd7a:115c:a1e0::2","100.64.0.7"],"authorized":true,"clientVersion":"1.74.1","expires":"2026-11-01T00:00:00Z",
	 "hostname":"Jans-MacBook-Pro","id":"2234567890123456","nodeId":"nXyZ2CNTRL","keyExpiryDisabled":false,"lastSeen":"2026-11-10T18:00:00Z",
	 "name":"jans-macbook-pro.tail1234.ts.net","os":"macOS","updateAvailable":false,"user":"jan@example.org","connectedToControl":false},
	{"addresses":["100.64.0.9"],"authorized":false,"clientVersion":"1.76.6","expires":"0001-01-01T00:00:00Z","hostname":"NAS",
	 "id":"3234567890123456","keyExpiryDisabled":true,"lastSeen":"2026-11-14T09:59:30Z","name":"nas.tail1234.ts.net","os":"linux"},
	{"addresses":["100.88.1.1"],"hostname":"friend-pc","id":"4234567890123456","isExternal":true,"name":"friend-pc.other.ts.net","os":"windows"},
	{"addresses":["100.64.0.20"],"hostname":"","id":"","nodeId":"nPhone9CNTRL","name":"pixel-8.tail1234.ts.net","os":"android"}]}`

var cands = []pdk.Entity{
	{ID: "e1", Kind: "server", Slug: "pve-web01", Name: "Web 01", Attrs: map[string]any{"hostname": "WEB01"}},
	{ID: "e2", Kind: "vm", Slug: "nas-vm", Name: "nas.lan", Attrs: map[string]any{}},
	{ID: "e3", Kind: "service", Slug: "jans-macbook-pro", Name: "Jans-MacBook-Pro"}, // wrong kind
	{ID: "e4", Kind: "tailscale_node", Slug: "ts-old", Name: "pixel-8"},             // wrong kind
}

func TestParseAndGraph(t *testing.T) {
	devs, err := parseDevices([]byte(devicesJSON))
	if err != nil || len(devs) != 5 {
		t.Fatalf("devices: %d %v", len(devs), err)
	}
	ops, links := graphOps(devs, newMatcher(cands))
	// 4 own devices (shared one left out) + 2 links (web01 exact via attrs.hostname, NAS via the short name of nas.lan).
	if links != 2 || len(ops.Ops) != 6 {
		t.Fatalf("ops: %d links %d: %+v", len(ops.Ops), links, ops.Ops)
	}
	web := ops.Ops[0]
	if web.Kind != "tailscale_node" || web.ID != "ts-1234567890123456" || web.Name != "web01" {
		t.Fatalf("web01: %+v", web)
	}
	want := map[string]any{"hostname": "web01", "os": "linux", "addresses": "100.101.102.103, fd7a:115c:a1e0::1", "tailscale_ip": "100.101.102.103",
		"client_version": "1.76.6", "last_seen": "2026-11-14", "update_available": true, "tags": "tag:server, tag:prod", "authorized": true,
		"dns_name": "web01.tail1234.ts.net"}
	for k, v := range want {
		if web.Attrs[k] != v {
			t.Errorf("attr %s = %v, want %v", k, web.Attrs[k], v)
		}
	}
	for k := range web.Attrs {
		if strings.Contains(k, "key") || k == "user" {
			t.Errorf("attr %s must not be stored", k)
		}
	}
	if l := ops.Ops[1]; l.Op != "link" || l.From != web.Ref || l.Rel != "related_to" || l.To != "pve-web01" {
		t.Fatalf("link web01: %+v", l)
	}
	mac := ops.Ops[2]
	if mac.Attrs["tailscale_ip"] != "100.64.0.7" || ops.Ops[3].Op != "upsert" {
		t.Fatalf("macbook (no host kind with that name, IPv4 picked): %+v %+v", mac, ops.Ops[3])
	}
	nas := ops.Ops[3]
	if nas.Name != "NAS" || nas.Attrs["authorized"] != false || ops.Ops[4].To != "nas-vm" || ops.Ops[4].From != nas.Ref {
		t.Fatalf("nas: %+v %+v", nas, ops.Ops[4])
	}
	phone := ops.Ops[5]
	if phone.ID != "ts-nphone9cntrl" || phone.Name != "pixel-8" || phone.Attrs["tags"] != nil {
		t.Fatalf("phone (node ID and DNS name fallback): %+v", phone)
	}
	if _, err := parseDevices([]byte(`<html>`)); err == nil {
		t.Fatal("accepted HTML")
	}
}

func TestMatch(t *testing.T) {
	dev := func(h string, addrs ...string) Device { return Device{Hostname: h, Addresses: addrs} }
	m := newMatcher(cands)
	if ref, ok := match(dev("web01"), m); !ok || ref != "pve-web01" {
		t.Fatal(ref, ok)
	}
	// Entries of other kinds are never linked, even with the same name.
	if ref, ok := match(dev("Jans-MacBook-Pro"), m); ok {
		t.Fatal("linked a service", ref)
	}
	if _, ok := match(dev(""), m); ok {
		t.Fatal("matched an empty hostname")
	}
	if _, ok := match(dev("localhost"), newMatcher([]pdk.Entity{{Kind: "server", Slug: "localhost", Name: "localhost"}})); ok {
		t.Fatal("matched localhost")
	}
	// A Tailscale address stored on the entry matches too.
	if ref, ok := match(dev("raspberrypi", "100.64.0.42"), newMatcher([]pdk.Entity{{Kind: "device", Slug: "pi-garage", Name: "Garage Pi", Attrs: map[string]any{"ip": "100.64.0.42"}}})); !ok || ref != "pi-garage" {
		t.Fatal(ref, ok)
	}
	// Two candidates with the same short name: ambiguous, no link.
	amb := []pdk.Entity{{Kind: "server", Slug: "a", Name: "db.site-a"}, {Kind: "lxc", Slug: "b", Name: "x", Attrs: map[string]any{"hostname": "db.site-b"}}}
	if ref, ok := match(dev("db"), newMatcher(amb)); ok {
		t.Fatal("linked an ambiguous host", ref)
	}
	// An exact name beats short ones; duplicates of one entry are fine.
	amb = append(amb, pdk.Entity{Kind: "device", Slug: "c", Name: "DB"}, pdk.Entity{Kind: "device", Slug: "c", Name: "DB"})
	if ref, ok := match(dev("db"), newMatcher(amb)); !ok || ref != "c" {
		t.Fatal(ref, ok)
	}
	// A service named like the device does not shadow the server.
	mixed := []pdk.Entity{{Kind: "service", Slug: "git-svc", Name: "git"}, {Kind: "vm", Slug: "git-vm", Name: "Git VM", Attrs: map[string]any{"hostname": "git"}}}
	if ref, ok := match(dev("git"), newMatcher(mixed)); !ok || ref != "git-vm" {
		t.Fatal(ref, ok)
	}
}

func TestChunk(t *testing.T) {
	var ops pdk.GraphOps
	for i := 0; i < 5; i++ {
		ref := ops.UpsertNamed("tailscale_node", "ts-x", "x", nil)
		ops.Link(ref, "related_to", "y", nil)
	}
	ops.UpsertNamed("tailscale_node", "ts-z", "z", nil)
	batches := chunk(ops, 5)
	if len(batches) != 3 || len(batches[0].Ops) != 4 || len(batches[1].Ops) != 4 || len(batches[2].Ops) != 3 {
		t.Fatalf("batches: %+v", batches)
	}
	for _, b := range batches {
		refs := map[string]bool{}
		for _, o := range b.Ops {
			if o.Op == "upsert" {
				refs[o.Ref] = true
			} else if !refs[o.From] {
				t.Fatalf("link %s separated from its upsert", o.From)
			}
		}
	}
	if len(chunk(pdk.GraphOps{}, 5)) != 0 || len(chunk(ops, 500)) != 1 {
		t.Fatal("edge cases")
	}
}

func TestDeviceInfo(t *testing.T) {
	devs, _ := parseDevices([]byte(devicesJSON))
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	list := deviceInfo(devs, now)
	if len(list) != 5 {
		t.Fatalf("list: %+v", list)
	}
	// Online first (web01 by connectedToControl, NAS by last seen 30 s ago), then by name.
	names := []string{}
	for _, in := range list {
		names = append(names, in.Name)
	}
	if strings.Join(names, ",") != "NAS,web01,friend-pc,Jans-MacBook-Pro,pixel-8" {
		t.Fatal(names)
	}
	nas, web, friend, mac := list[0], list[1], list[2], list[3]
	if !nas.Online || nas.KeyExpiry != "" || !nas.KeyExpiryDisabled || nas.Authorized {
		t.Fatalf("nas: %+v", nas)
	}
	if !web.Online || web.KeyExpired || web.KeyExpiry == "" || web.User != "tagged-devices" || !web.UpdateAvailable || web.ClientVersion != "1.76.6" {
		t.Fatalf("web01: %+v", web)
	}
	if !friend.Shared || friend.Online {
		t.Fatalf("shared device: %+v", friend)
	}
	if mac.Online || !mac.KeyExpired || mac.TailscaleIP != "100.64.0.7" || mac.LastSeen != "2026-11-10T18:00:00Z" {
		t.Fatalf("macbook: %+v", mac)
	}
}

func TestSettingsAndErrors(t *testing.T) {
	for in, want := range map[string]string{"": "-", " - ": "-", "example.com": "example.com", "T1a2b3CNTRL": "T1a2b3CNTRL", "jan@example.org": "jan@example.org"} {
		if got, err := tailnetSetting(in); err != nil || got != want {
			t.Errorf("tailnetSetting(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"../devices", "a/b", "a?b", "x y", "a..b"} {
		if _, err := tailnetSetting(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if !strings.Contains(httpError(401).Error(), "API key") || !strings.Contains(httpError(404).Error(), "tailnet") {
		t.Fatal("messages")
	}
	if httpError(500).Error() != "Tailscale answered HTTP 500" {
		t.Fatal(httpError(500))
	}
}

// TestDemoData parses the recorded demo answer the screenshots use.
func TestDemoData(t *testing.T) {
	body, err := fakeapi.Body("../testdata/fake", "GET", "/api/v2/tailnet/-/devices")
	if err != nil {
		t.Fatal(err)
	}
	devs, err := parseDevices(body)
	if err != nil || len(devs) != 11 {
		t.Fatalf("%v %d", err, len(devs))
	}
	m := newMatcher([]pdk.Entity{
		{Slug: "homeserver", Kind: "server", Name: "homeserver", Attrs: map[string]any{"hostname": "homeserver.home.arpa", "ip": "10.20.0.10"}},
		{Slug: "pihole", Kind: "lxc", Name: "pihole", Attrs: map[string]any{"ip": "10.20.0.53"}},
		{Slug: "home-assistant", Kind: "vm", Name: "home-assistant", Attrs: map[string]any{"ip": "10.20.0.20"}},
	})
	ops, links := graphOps(devs, m)
	if len(ops.Ops) != 10+3 || links != 3 {
		t.Fatalf("ops %d links %d", len(ops.Ops), links)
	}
	info := deviceInfo(devs, time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC))
	if !info[0].Online || info[len(info)-1].Online || info[0].TailscaleIP == "" {
		t.Fatalf("info: %+v", info)
	}
}
