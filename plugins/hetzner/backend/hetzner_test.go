package main

import (
	"strings"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/integ"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const page1 = `{
  "servers": [
    {
      "id": 42,
      "name": "web-1",
      "status": "running",
      "created": "2025-03-01T10:00:00+00:00",
      "labels": {"env": "prod", "role": "web", "backup": ""},
      "public_net": {
        "ipv4": {"id": 1, "ip": "203.0.113.10", "blocked": false, "dns_ptr": "web-1.example.org"},
        "ipv6": {"id": 2, "ip": "2001:db8:1234::/64", "blocked": false, "dns_ptr": []},
        "floating_ips": [],
        "firewalls": []
      },
      "private_net": [{"network": 7, "ip": "10.0.0.2", "alias_ips": [], "mac_address": "86:00:00:2a:7d:e0"}],
      "server_type": {"id": 22, "name": "cpx21", "description": "CPX 21", "cores": 3, "memory": 4.0, "disk": 80, "cpu_type": "shared", "architecture": "x86"},
      "datacenter": {"id": 3, "name": "fsn1-dc14", "description": "Falkenstein 1 virtual DC 14",
        "location": {"id": 1, "name": "fsn1", "description": "Falkenstein DC Park 1", "country": "DE", "city": "Falkenstein", "network_zone": "eu-central"}},
      "image": {"id": 114690387, "type": "system", "status": "available", "name": "ubuntu-24.04", "description": "Ubuntu 24.04", "os_flavor": "ubuntu", "os_version": "24.04"}
    },
    {
      "id": 43,
      "name": "db-1",
      "status": "off",
      "labels": {},
      "public_net": {"ipv4": null, "ipv6": {"ip": "2001:db8:5678::/64"}},
      "private_net": [],
      "server_type": {"name": "cax11", "cores": 2, "memory": 4, "architecture": "arm"},
      "datacenter": {"name": "nbg1-dc3", "location": {"name": "nbg1"}},
      "image": null
    }
  ],
  "meta": {"pagination": {"page": 1, "per_page": 50, "previous_page": null, "next_page": 2, "last_page": 2, "total_entries": 51}}
}`

func TestServers(t *testing.T) {
	list, next, err := parseServers([]byte(page1))
	if err != nil || next != 2 || len(list) != 2 {
		t.Fatal(list, next, err)
	}
	if _, next, _ := parseServers([]byte(`{"servers":[],"meta":{"pagination":{"page":2,"next_page":null}}}`)); next != 0 {
		t.Fatal("last page", next)
	}
	ops := graphOps(list).Ops
	if len(ops) != 2 {
		t.Fatalf("%+v", ops)
	}
	web := ops[0]
	if web.Kind != "vm" || web.ID != "hetzner-42" || web.Name != "web-1" || web.Attrs["ip"] != "203.0.113.10" || web.Attrs["os"] != "Ubuntu 24.04" ||
		web.Attrs["vcpu"] != 3 || web.Attrs["ram"] != 4.0 || web.Attrs["datacenter"] != "fsn1-dc14" || web.Attrs["location"] != "fsn1" ||
		web.Attrs["status"] != "running" || web.Attrs["server_type"] != "cpx21" || web.Attrs["private_ip"] != "10.0.0.2" ||
		web.Attrs["labels"] != "backup, env=prod, role=web" || web.Attrs["hetzner_id"] != int64(42) {
		t.Fatalf("web: %+v", web)
	}
	db := ops[1]
	if db.ID != "hetzner-43" || db.Attrs["ip"] != nil || db.Attrs["os"] != nil || db.Attrs["ipv6"] != "2001:db8:5678::/64" || db.Attrs["labels"] != nil || db.Attrs["status"] != "off" {
		t.Fatalf("db (no IPv4, deleted image): %+v", db)
	}
	info := serverInfo(list)
	if info[0].IPv4 != "203.0.113.10" || info[0].Labels["env"] != "prod" || info[0].Location != "fsn1" || info[1].IPv4 != "" || len(info[1].PrivateIPs) != 0 {
		t.Fatalf("info: %+v", info)
	}
}

func TestErrorsAndBatches(t *testing.T) {
	err := apiError(401, []byte(`{"error":{"code":"unauthorized","message":"unable to authenticate"}}`))
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatal(err)
	}
	if apiError(502, []byte("<html>")).Error() != "Hetzner Cloud answered HTTP 502" {
		t.Fatal("plain status")
	}
	var ops pdk.GraphOps
	for i := 0; i < 5; i++ {
		ops.Upsert("vm", "x", nil)
	}
	n := 0
	if err := integ.ApplyBatches(ops, 2, func(b *pdk.GraphOps) (map[string]string, error) { n++; return nil, nil }); err != nil || n != 3 {
		t.Fatal(n, err)
	}
}
