package main

import (
	"strings"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const zonesJSON = `{
  "result": [
    {"id": "023e105f4ecef8ad9ca31a8372d0c353", "name": "example.com", "status": "active", "paused": false, "type": "full",
     "development_mode": 0, "name_servers": ["bob.ns.cloudflare.com", "lola.ns.cloudflare.com"], "original_name_servers": ["ns1.registrar.com"],
     "account": {"id": "01a7362d577a6c3019a474fd6f485823", "name": "Acme Ltd"}, "plan": {"id": "0feeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "name": "Free Website"}},
    {"id": "353c0d8a2713ac9fd8ace4fce501e320", "name": "example.net", "status": "pending", "paused": true, "type": "full",
     "name_servers": ["bob.ns.cloudflare.com"], "plan": {"name": "Pro Website"}}
  ],
  "result_info": {"page": 1, "per_page": 50, "count": 2, "total_count": 2, "total_pages": 1},
  "success": true, "errors": [], "messages": []
}`

const recordsJSON = `{
  "result": [
    {"id": "1", "zone_id": "023e", "zone_name": "example.com", "name": "example.com", "type": "A", "content": "203.0.113.10", "proxiable": true, "proxied": true, "ttl": 1},
    {"id": "2", "name": "www.example.com", "type": "CNAME", "content": "example.com", "proxiable": true, "proxied": true, "ttl": 1},
    {"id": "3", "name": "vpn.example.com", "type": "A", "content": "198.51.100.7", "proxiable": true, "proxied": false, "ttl": 300, "comment": "office"},
    {"id": "4", "name": "v6.example.com", "type": "AAAA", "content": "2001:db8::10", "proxied": false, "ttl": 3600},
    {"id": "5", "name": "shop.example.com", "type": "CNAME", "content": "shops.myshopify.com", "proxied": false, "ttl": 1},
    {"id": "6", "name": "example.com", "type": "MX", "content": "mail.example.com", "priority": 10, "proxiable": false, "ttl": 1},
    {"id": "7", "name": "svc.example.com", "type": "A", "content": "10.9.9.9", "proxied": false, "ttl": 60}
  ],
  "result_info": {"page": 1, "per_page": 100, "count": 7, "total_count": 7, "total_pages": 1},
  "success": true, "errors": [], "messages": []
}`

func TestZones(t *testing.T) {
	zones, err := parsePage[Zone](200, []byte(zonesJSON))
	if err != nil || len(zones.Result) != 2 || zones.ResultInfo.TotalPages != 1 {
		t.Fatal(zones, err)
	}
	recs, err := parsePage[Record](200, []byte(recordsJSON))
	if err != nil || len(recs.Result) != 7 || recs.ResultInfo.TotalCount != 7 {
		t.Fatal(recs, err)
	}
	m := integ.NewMatcher([]pdk.Entity{
		{Kind: "vm", Slug: "hetzner-42", Name: "web-1", Attrs: map[string]any{"ip": "203.0.113.10"}},
		{Kind: "router", Slug: "office-fw", Name: "Office firewall", Attrs: map[string]any{"ip": "198.51.100.7"}},
		{Kind: "server", Slug: "v6-host", Name: "v6 host", Attrs: map[string]any{"ipv6": "2001:db8::10"}},
		{Kind: "service", Slug: "svc", Name: "Some service", Attrs: map[string]any{"ip": "10.9.9.9"}},
	})
	data := []ZoneData{
		{Zone: zones.Result[0], Records: recs.Result, Total: recs.ResultInfo.TotalCount},
		{Zone: zones.Result[1], Total: 250},
	}
	ops := graphOps(data, m).Ops
	z := ops[0]
	if z.Kind != "dns_zone" || z.ID != "cloudflare-023e105f4ecef8ad9ca31a8372d0c353" || z.Name != "example.com" || z.Attrs["status"] != "active" ||
		z.Attrs["plan"] != "Free Website" || z.Attrs["name_servers"] != "bob.ns.cloudflare.com, lola.ns.cloudflare.com" ||
		z.Attrs["records"] != 7 || z.Attrs["proxied_records"] != 2 || z.Attrs["account"] != "Acme Ltd" {
		t.Fatalf("zone: %+v", z)
	}
	// Links sorted by slug: hetzner-42 (apex A and www CNAME), office-fw, v6-host; the service is not a host kind,
	// the external CNAME and the MX record link nothing.
	if len(ops) != 5 {
		t.Fatalf("%d ops: %+v", len(ops), ops)
	}
	if l := ops[1]; l.Op != "link" || l.From != z.Ref || l.Rel != "related_to" || l.To != "hetzner-42" || l.Attrs["records"] != "example.com A, www.example.com CNAME" {
		t.Fatalf("link: %+v", l)
	}
	if ops[2].To != "office-fw" || ops[2].Attrs["records"] != "vpn.example.com A" || ops[3].To != "v6-host" {
		t.Fatalf("links: %+v %+v", ops[2], ops[3])
	}
	if p := ops[4]; p.Kind != "dns_zone" || p.Attrs["records"] != 250 || p.Attrs["paused"] != true || p.Attrs["proxied_records"] != 0 {
		t.Fatalf("paused zone: %+v", p)
	}
	if n := len(graphOps(data, nil).Ops); n != 2 {
		t.Fatalf("without matcher: %d ops", n)
	}
}

func TestTargets(t *testing.T) {
	byName := map[string][]Record{
		"a.example.com": {{Type: "CNAME", Content: "b.example.com."}},
		"b.example.com": {{Type: "A", Content: "192.0.2.1"}, {Type: "A", Content: "192.0.2.2"}},
		"loop.example":  {{Type: "CNAME", Content: "loop.example"}},
		"other.example": {{Type: "TXT", Content: "x"}},
	}
	if got := targets(Record{Type: "CNAME", Content: "A.example.com"}, byName); len(got) != 2 || got[0] != "192.0.2.1" {
		t.Fatal(got)
	}
	if got := targets(Record{Type: "CNAME", Content: "loop.example"}, byName); got != nil {
		t.Fatal("loop", got)
	}
	if got := targets(Record{Type: "TXT", Content: "1.2.3.4"}, byName); got != nil {
		t.Fatal("txt", got)
	}
}

func TestErrorsAndValidation(t *testing.T) {
	_, err := parsePage[Zone](403, []byte(`{"success":false,"errors":[{"code":9109,"message":"Invalid access token"}],"messages":[],"result":null}`))
	if err == nil || err.Error() != "Cloudflare answered HTTP 403 (9109: Invalid access token)" {
		t.Fatal(err)
	}
	if _, err := parsePage[Zone](200, []byte(`{"success":false,"errors":[]}`)); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatal(err)
	}
	if _, err := parsePage[Zone](502, []byte(`<html>`)); err == nil || err.Error() != "Cloudflare answered HTTP 502" {
		t.Fatal(err)
	}
	if n, err := validZone(" Example.COM. "); err != nil || n != "example.com" {
		t.Fatal(n, err)
	}
	for _, bad := range []string{"", "example", "a/b.com", "exa mple.com", "-x.com", "x.com&name=y"} {
		if _, err := validZone(bad); err == nil {
			t.Error("accepted", bad)
		}
	}
	var ops pdk.GraphOps
	ref := ops.Upsert("dns_zone", "z", nil)
	ops.Link(ref, "related_to", "x", nil)
	var second pdk.GraphOp
	_ = integ.ApplyBatches(ops, 1, func(b *pdk.GraphOps) (map[string]string, error) {
		if b.Ops[0].Op == "link" {
			second = b.Ops[0]
		}
		return map[string]string{ref: "z"}, nil
	})
	if second.From != "z" {
		t.Fatalf("ref not replaced: %+v", second)
	}
}

// TestDemoData parses the recorded demo answers the screenshots use.
func TestDemoData(t *testing.T) {
	get := func(p string) []byte {
		body, err := fakeapi.Body("../testdata/fake", "GET", strings.TrimPrefix(api, "https://api.cloudflare.com")+p)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	zones, err := parsePage[Zone](200, get("/zones?per_page=50&page=1"))
	if err != nil || len(zones.Result) != 3 {
		t.Fatalf("%v %+v", err, zones)
	}
	var data []ZoneData
	for _, z := range zones.Result {
		recs, err := parsePage[Record](200, get("/zones/"+z.ID+"/dns_records?per_page=100&page=1"))
		if err != nil || len(recs.Result) == 0 {
			t.Fatalf("%s: %v", z.Name, err)
		}
		data = append(data, ZoneData{Zone: z, Records: recs.Result, Total: recs.ResultInfo.TotalCount})
	}
	m := integ.NewMatcher([]pdk.Entity{
		{Slug: "gateway", Kind: "router", Name: "Gateway", Attrs: map[string]any{"ip": "198.51.100.23"}},
		{Slug: "vps-fra", Kind: "server", Name: "vps-fra", Attrs: map[string]any{"ip": "192.0.2.80"}},
		{Slug: "mail", Kind: "server", Name: "mail", Attrs: map[string]any{"ip": "192.0.2.81"}},
	})
	ops := graphOps(data, m)
	links := 0
	for _, o := range ops.Ops {
		if o.Op == "link" {
			links++
		}
	}
	if len(ops.Ops)-links != 3 || links != 5 || ops.Ops[0].Attrs["proxied_records"] != 7 {
		t.Fatalf("ops: %+v", ops.Ops)
	}
}
