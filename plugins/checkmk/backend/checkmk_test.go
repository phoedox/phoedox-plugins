package main

import (
	"net/url"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const servicesAnswer = `{
  "links": [{"domainType": "link", "rel": "self", "href": "https://mon.example.org/mysite/check_mk/api/1.0/domain-types/service/collections/all", "method": "GET", "type": "application/json"}],
  "id": "service",
  "domainType": "service",
  "value": [
    {"links": [], "domainType": "service", "id": "web01:Filesystem /", "title": "Filesystem / on web01",
     "extensions": {"host_name": "web01", "host_address": "10.0.0.5", "description": "Filesystem /", "state": 1, "plugin_output": "Used: 85.2% - 17.0 GiB of 20.0 GiB (warn/crit at 80.0%/90.0% used)WARN", "last_state_change": 1794643200, "acknowledged": 0}},
    {"links": [], "domainType": "service", "id": "nas:PING", "title": "PING on nas",
     "extensions": {"host_name": "nas", "host_address": "192.168.1.10", "description": "PING", "state": 2, "plugin_output": "CRIT - 192.168.1.10: rta nan, lost 100%", "last_state_change": 1794639600.0, "acknowledged": 1}},
    {"links": [], "domainType": "service", "id": "printer:SNMP Info", "title": "SNMP Info on printer",
     "extensions": {"host_name": "printer", "host_address": "192.168.1.30", "description": "SNMP Info", "state": 3, "plugin_output": "Cannot fetch SNMP data", "last_state_change": 0, "acknowledged": 0}},
    {"links": [], "domainType": "service", "id": "x", "extensions": {"host_name": "ok-host", "description": "CPU load", "state": 0}}
  ]
}`

const hostsAnswer = `{"id": "host", "domainType": "host", "links": [], "value": [
  {"links": [], "domainType": "host", "id": "web01", "title": "web01", "extensions": {"name": "web01", "address": "10.0.0.5", "state": 0}},
  {"links": [], "domainType": "host", "id": "nas", "title": "nas", "extensions": {"name": "nas", "address": "192.168.1.10", "state": 1}},
  {"links": [], "domainType": "host", "id": "branch-rtr", "title": "branch-rtr", "extensions": {"name": "branch-rtr", "address": "rtr.branch.example.org", "state": 2}}
]}`

func graph() *integ.Matcher {
	return integ.NewMatcher([]pdk.Entity{
		{Kind: "server", Slug: "web-01", Name: "Web 01", Attrs: map[string]any{"ip": "10.0.0.5"}},
		{Kind: "server", Slug: "nas", Name: "NAS"},
		{Kind: "router", Slug: "branch-router", Name: "Branch router", Attrs: map[string]any{"hostname": "RTR.branch.example.org"}},
	})
}

func TestProblems(t *testing.T) {
	list, err := parseServices([]byte(servicesAnswer))
	if err != nil || len(list) != 4 {
		t.Fatal(list, err)
	}
	rows := problemRows(list, graph())
	if len(rows) != 3 {
		t.Fatalf("OK service not dropped: %+v", rows)
	}
	crit, unknown, warn := rows[0], rows[1], rows[2]
	if crit.Host != "nas" || crit.Service != "PING" || crit.State != "CRIT" || !crit.Acknowledged || crit.Since != "2026-11-14T07:00:00Z" || crit.Entry == nil || crit.Entry.Slug != "nas" {
		t.Fatalf("crit: %+v %+v", crit, crit.Entry)
	}
	if unknown.State != "UNKNOWN" || unknown.Since != "" || unknown.Entry != nil {
		t.Fatalf("unknown: %+v", unknown)
	}
	if warn.State != "WARN" || warn.Acknowledged || warn.Entry == nil || warn.Entry.Slug != "web-01" || !strings.HasPrefix(warn.Output, "Used: 85.2%") {
		t.Fatalf("warn: %+v", warn)
	}
}

func TestHosts(t *testing.T) {
	list, err := parseHosts([]byte(hostsAnswer))
	if err != nil {
		t.Fatal(err)
	}
	rows := hostRows(list, graph())
	want := []struct{ name, state, slug string }{{"branch-rtr", "UNREACHABLE", "branch-router"}, {"nas", "DOWN", "nas"}, {"web01", "UP", "web-01"}}
	for i, w := range want {
		r := rows[i]
		if r.Name != w.name || r.State != w.state || r.Entry == nil || r.Entry.Slug != w.slug {
			t.Fatalf("%d: %+v %+v", i, r, r.Entry)
		}
	}
	if hostState(7) != "UNKNOWN" || serviceState(0) != "OK" || serviceState(9) != "UNKNOWN" {
		t.Fatal("states")
	}
}

func TestRequests(t *testing.T) {
	for _, in := range []string{"https://mon.example.org/mysite", "https://mon.example.org/mysite/check_mk", "https://mon.example.org/mysite/check_mk/api/1.0"} {
		if apiBase(in) != "https://mon.example.org/mysite/check_mk/api/1.0" {
			t.Fatal(in, apiBase(in))
		}
	}
	u, err := url.Parse(problemsPath())
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Path != "/domain-types/service/collections/all" || q.Get("query") != `{"op":"!=","left":"state","right":"0"}` || len(q["columns"]) != 7 || q["columns"][0] != "host_name" {
		t.Fatalf("problems path: %s", problemsPath())
	}
	if hostsPath() != "/domain-types/host/collections/all?columns=name&columns=address&columns=state" {
		t.Fatal(hostsPath())
	}
}

func TestErrors(t *testing.T) {
	if _, err := parseServices([]byte(`<html>login</html>`)); err == nil {
		t.Fatal("html accepted")
	}
	if l, err := parseHosts([]byte(`{"value":[]}`)); err != nil || l == nil || len(l) != 0 {
		t.Fatal("empty", l, err)
	}
	err := apiError(400, []byte(`{"title":"Bad Request","status":400,"detail":"These fields have problems: columns","fields":{}}`))
	if err.Error() != "Checkmk answered HTTP 400: Bad Request: These fields have problems: columns" {
		t.Fatal(err)
	}
	err = apiError(401, []byte(`{"title":"Unauthorized","status":401,"detail":"You need to be authenticated to use the REST API."}`))
	if !strings.Contains(err.Error(), "username or secret") {
		t.Fatal(err)
	}
	if se, ok := apiError(502, []byte(`Bad gateway`)).(*integ.StatusError); !ok || se.Status != 502 {
		t.Fatal("plain status error")
	}
}
