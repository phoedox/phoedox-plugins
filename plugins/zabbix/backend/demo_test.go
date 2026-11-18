package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// demoGraph holds the entries the screenshots create (shots.json).
func demoGraph() *integ.Matcher {
	return integ.NewMatcher([]pdk.Entity{
		{Kind: "server", Slug: "nas-01", Name: "nas-01", Attrs: map[string]any{"hostname": "nas-01.home.arpa", "ip": "10.20.0.10"}},
		{Kind: "server", Slug: "pve-01", Name: "pve-01", Attrs: map[string]any{"hostname": "pve-01.home.arpa", "ip": "10.20.0.11"}},
		{Kind: "vm", Slug: "home-assistant", Name: "Home Assistant", Attrs: map[string]any{"ip": "10.20.0.21"}},
		{Kind: "router", Slug: "router", Name: "Router", Attrs: map[string]any{"ip": "10.20.0.1"}},
		{Kind: "device", Slug: "garden-camera", Name: "Garden camera", Attrs: map[string]any{"ip": "10.20.0.42"}},
	})
}

// TestDemoData runs the calls of zabbix.problems against the recorded demo
// answers the screenshots use. All calls go to /api_jsonrpc.php, so the
// recording answers them in order: problem.get, event.get, host.get.
func TestDemoData(t *testing.T) {
	api, err := fakeapi.Load("../testdata/fake")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api)
	defer srv.Close()
	call := func(method string, params, out any) {
		t.Helper()
		req, _ := rpcRequest(method, params, 1)
		res, err := http.Post(apiURL(srv.URL), "application/json-rpc", strings.NewReader(string(req)))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if err := decodeRPC(body, out); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	var probs []Problem
	call("problem.get", problemParams(), &probs)
	var events []Event
	call("event.get", eventParams(probs), &events)
	var hosts []Host
	call("host.get", hostParams(hostIDs(events)), &hosts)
	if len(probs) != 4 || len(events) != 4 || len(hosts) != 5 {
		t.Fatalf("%d problems, %d events, %d hosts", len(probs), len(events), len(hosts))
	}
	rows := problemRows(probs, events, hosts, demoGraph())
	want := []string{"garden-camera", "nas-01", "pve-01", "router"}
	for i, r := range rows {
		if r.Entry == nil || r.Entry.Slug != want[i] {
			t.Errorf("%s: entry %v, want %s", r.Name, r.Entry, want[i])
		}
	}
	if rows[0].Severity != "high" || !rows[2].Resolved || !rows[3].Acknowledged {
		t.Errorf("%+v", rows)
	}
	if hr := hostRows(hosts, demoGraph()); hr[0].Name != "Garden camera" || hr[0].Available != "unavailable" {
		t.Errorf("%+v", hr[0])
	}
}
