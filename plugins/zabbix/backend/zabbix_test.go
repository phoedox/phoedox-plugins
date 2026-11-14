package main

import (
	"encoding/json"
	"github.com/phoedox/phoedox-plugins/internal/integ"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const problemsAnswer = `{"jsonrpc":"2.0","result":[
  {"eventid":"4711","source":"0","object":"0","objectid":"23001","clock":"1794643200","ns":"0","r_eventid":"0","r_clock":"0","r_ns":"0","correlationid":"0","userid":"0","name":"High CPU utilization (over 90% for 5m)","acknowledged":"0","severity":"3","opdata":"","suppressed":"0","urls":[]},
  {"eventid":"4702","objectid":"23010","clock":"1794639600","r_eventid":"4709","r_clock":"1794642000","name":"Unavailable by ICMP ping","acknowledged":"1","severity":"4","suppressed":"0"},
  {"eventid":"4690","objectid":"23020","clock":"1794636000","r_eventid":"0","name":"Template problem without host","acknowledged":"0","severity":"9"}
],"id":1}`

const eventsAnswer = `{"jsonrpc":"2.0","result":[
  {"eventid":"4711","hosts":[{"hostid":"10084"}]},
  {"eventid":"4702","hosts":[{"hostid":"10101"}]}
],"id":2}`

const hostsAnswer = `{"jsonrpc":"2.0","result":[
  {"hostid":"10084","host":"web01","name":"Web server 01","status":"0","interfaces":[
    {"type":"1","main":"1","useip":"1","ip":"10.0.0.5","dns":"","available":"1"},
    {"type":"2","main":"1","useip":"1","ip":"10.0.0.5","dns":"","available":"0"}]},
  {"hostid":"10101","host":"nas","name":"nas","status":"0","interfaces":[
    {"type":"2","main":"1","useip":"0","ip":"","dns":"nas.home.lan","available":"2"}]},
  {"hostid":"10200","host":"old-db","name":"Alte Datenbank","status":"1","interfaces":[]}
],"id":3}`

func graph() *integ.Matcher {
	return integ.NewMatcher([]pdk.Entity{
		{Kind: "server", Slug: "web-01", Name: "Web 01", Attrs: map[string]any{"ip": "10.0.0.5"}},
		{Kind: "server", Slug: "nas", Name: "Synology", Attrs: map[string]any{"hostname": "NAS.home.lan"}},
	})
}

func TestProblems(t *testing.T) {
	var probs []Problem
	var events []Event
	var hosts []Host
	for body, out := range map[string]any{problemsAnswer: &probs, eventsAnswer: &events, hostsAnswer: &hosts} {
		if err := decodeRPC([]byte(body), out); err != nil {
			t.Fatal(err)
		}
	}
	if ids := hostIDs(events); len(ids) != 2 || ids[0] != "10084" {
		t.Fatalf("host ids: %v", ids)
	}
	rows := problemRows(probs, events, hosts, graph())
	if len(rows) != 3 {
		t.Fatal(len(rows))
	}
	cpu := rows[0]
	if cpu.Severity != "average" || cpu.Acknowledged || cpu.Resolved || cpu.Since != "2026-11-14T08:00:00Z" || cpu.Hosts[0] != "Web server 01" || cpu.Entry == nil || cpu.Entry.Slug != "web-01" {
		t.Fatalf("cpu: %+v %+v", cpu, cpu.Entry)
	}
	ping := rows[1]
	if ping.Severity != "high" || !ping.Acknowledged || !ping.Resolved || ping.ResolvedAt != "2026-11-14T07:40:00Z" || ping.Entry == nil || ping.Entry.Slug != "nas" {
		t.Fatalf("ping: %+v %+v", ping, ping.Entry)
	}
	if rows[2].Severity != "unknown" || len(rows[2].Hosts) != 0 || rows[2].Entry != nil || rows[2].Hosts == nil {
		t.Fatalf("no host: %+v", rows[2])
	}
	for i, want := range []string{"not classified", "information", "warning", "average", "high", "disaster"} {
		if severityName(flexInt(i)) != want {
			t.Fatal(i)
		}
	}
}

func TestHosts(t *testing.T) {
	var hosts []Host
	if err := decodeRPC([]byte(hostsAnswer), &hosts); err != nil {
		t.Fatal(err)
	}
	rows := hostRows(hosts, graph())
	if rows[0].Name != "Alte Datenbank" || rows[0].Status != "disabled" || rows[0].Available != "unknown" || rows[0].Entry != nil {
		t.Fatalf("disabled: %+v", rows[0])
	}
	if rows[1].Host != "nas" || rows[1].Available != "unavailable" || rows[1].DNS != "nas.home.lan" || rows[1].Entry.Slug != "nas" {
		t.Fatalf("nas: %+v", rows[1])
	}
	if rows[2].Status != "enabled" || rows[2].Available != "available" || rows[2].IP != "10.0.0.5" || rows[2].Entry.Slug != "web-01" {
		t.Fatalf("web: %+v", rows[2])
	}
}

func TestRPC(t *testing.T) {
	body, err := rpcRequest("problem.get", problemParams(), 7)
	if err != nil {
		t.Fatal(err)
	}
	var req map[string]any
	_ = json.Unmarshal(body, &req)
	params := req["params"].(map[string]any)
	if req["jsonrpc"] != "2.0" || req["method"] != "problem.get" || req["id"] != float64(7) || req["auth"] != nil || params["recent"] != true || params["sortorder"] != "DESC" || params["limit"] != float64(100) {
		t.Fatalf("request: %s", body)
	}
	if p := hostParams([]string{"1"}); p["limit"] != nil || p["hostids"] == nil {
		t.Fatalf("host params by id: %v", p)
	}
	if apiURL("https://z.example.org/zabbix") != "https://z.example.org/zabbix/api_jsonrpc.php" || apiURL("https://z/api_jsonrpc.php") != "https://z/api_jsonrpc.php" {
		t.Fatal("api url")
	}
	var out []Host
	err = decodeRPC([]byte(`{"jsonrpc":"2.0","error":{"code":-32602,"message":"Invalid params.","data":"Not authorized."},"id":1}`), &out)
	if err == nil || err.Error() != "Zabbix: Invalid params: Not authorized." {
		t.Fatalf("rpc error: %v", err)
	}
	err = decodeRPC([]byte(`{"jsonrpc":"2.0","error":{"code":-32500,"message":"Application error.","data":"`+strings.Repeat("x", 500)+`"},"id":1}`), &out)
	if err == nil || len(err.Error()) > 340 {
		t.Fatalf("long error not cut: %d", len(err.Error()))
	}
	for _, bad := range []string{`<html>`, `{"jsonrpc":"2.0","id":1}`, `{"result":{"x":1}}`} {
		if decodeRPC([]byte(bad), &out) == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	var f flexInt
	if json.Unmarshal([]byte(`"12"`), &f) != nil || f != 12 || json.Unmarshal([]byte(`3`), &f) != nil || f != 3 || json.Unmarshal([]byte(`"x"`), &f) == nil {
		t.Fatal("flexInt")
	}
}
