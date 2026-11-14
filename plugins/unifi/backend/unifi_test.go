package main

import (
	"encoding/json"
	"testing"
)

func TestUniFi(t *testing.T) {
	sites := []Site{{ID: "1", InternalReference: "default", Name: "Default"}, {ID: "2", InternalReference: "x7", Name: "Office"}}
	if s, ok := pickSite(sites, ""); !ok || s.ID != "1" {
		t.Fatal("default site")
	}
	if s, ok := pickSite(sites, "office"); !ok || s.ID != "2" {
		t.Fatal("named site")
	}
	if _, ok := pickSite(sites, "nope"); ok {
		t.Fatal("unknown site")
	}
	var devices []Device
	_ = json.Unmarshal([]byte(`[
		{"id":"g","name":"Dream Machine","model":"UDM-Pro","macAddress":"AA:BB:CC:00:00:01","ipAddress":"192.168.1.1","state":"ONLINE","features":["switching","gateway"],"firmwareVersion":"4.1.13"},
		{"id":"s","name":"","model":"USW-Lite-8-PoE","macAddress":"AA:BB:CC:00:00:02","ipAddress":"192.168.1.2","state":"ONLINE","features":["switching"],"uplink":{"deviceId":"g"}},
		{"id":"a","name":"AP Hall","model":"U6-Lite","macAddress":"AA:BB:CC:00:00:03","state":"ONLINE","features":["accessPoint"],"uplink":{"deviceId":"s"}},
		{"id":"m","name":"AP Garden","model":"U6-Mesh","macAddress":"AA:BB:CC:00:00:04","features":["accessPoint"],"uplink":{"deviceId":"a"}}]`), &devices)
	ops := graphOps(devices)
	if ops.Ops[0].Kind != "router" || ops.Ops[0].ID != "unifi-aabbcc000001" || ops.Ops[0].Attrs["firmware"] != "4.1.13" {
		t.Fatalf("gateway: %+v", ops.Ops[0])
	}
	if ops.Ops[1].Kind != "switch" || ops.Ops[1].Name != "USW-Lite-8-PoE AA:BB:CC:00:00:02" || ops.Ops[1].Attrs["managed"] != true {
		t.Fatalf("switch: %+v", ops.Ops[1])
	}
	if ops.Ops[2].Kind != "access_point" || ops.Ops[3].Kind != "access_point" {
		t.Fatalf("aps: %+v", ops.Ops[2:4])
	}
	links := ops.Ops[4:]
	if len(links) != 2 || links[0].From != ops.Ops[1].Ref || links[0].To != ops.Ops[0].Ref || links[1].From != ops.Ops[2].Ref {
		t.Fatalf("links (mesh uplink skipped): %+v", links)
	}
	cl := clientInfo([]Client{{Type: "WIRELESS", MacAddress: "11:22:33:44:55:66", IPAddress: "192.168.1.50", UplinkDeviceID: "a"}}, devices)
	if cl[0].Name != "11:22:33:44:55:66" || cl[0].Via != "AP Hall" || cl[0].Type != "wireless" {
		t.Fatalf("clients: %+v", cl)
	}
}
