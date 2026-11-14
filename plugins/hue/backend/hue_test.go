package main

import (
	"strings"
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const key = "1028d66426293e821ecfd9ef1a0731df"

const config = `{"name":"Hue Bridge Home","zigbeechannel":25,"bridgeid":"001788FFFE4A1B2C","mac":"00:17:88:4a:1b:2c","dhcp":true,
	"ipaddress":"192.168.1.20","netmask":"255.255.255.0","gateway":"192.168.1.1","proxyaddress":"none","proxyport":0,
	"UTC":"2024-11-14T10:00:00","localtime":"2024-11-14T11:00:00","timezone":"Europe/Berlin","modelid":"BSB002",
	"datastoreversion":"163","swversion":"1967054020","apiversion":"1.65.0","linkbutton":false,"portalservices":true,
	"whitelist":{"` + key + `":{"last use date":"2024-11-14T10:00:00","create date":"2023-01-01T00:00:00","name":"phoedox#server"}}}`

const lights = `{
	"1":{"state":{"on":true,"bri":127,"hue":8418,"sat":140,"effect":"none","xy":[0.4573,0.41],"ct":366,"alert":"select","colormode":"ct","mode":"homeautomation","reachable":true},
		"swupdate":{"state":"noupdates","lastinstall":"2024-06-01T10:00:00"},"type":"Extended color light","name":"Sofa",
		"modelid":"LCT015","manufacturername":"Signify Netherlands B.V.","productname":"Hue color lamp","uniqueid":"00:17:88:01:04:a1:b2:c3-0b","swversion":"1.108.7"},
	"10":{"state":{"on":false,"bri":254,"alert":"none","mode":"homeautomation","reachable":false},"type":"Dimmable light","name":"Hallway",
		"modelid":"LWB010","manufacturername":"Signify Netherlands B.V.","productname":"Hue white lamp","uniqueid":"00:17:88:01:02:d4:e5:f6-0b","swversion":"1.50.2"},
	"2":{"state":{"on":true,"reachable":true},"type":"On/Off plug-in unit","name":"Christmas tree","modelid":"TS011F",
		"manufacturername":"_TZ3000_abc","productname":"On/Off plug","uniqueid":"a4:c1:38:00:11:22:33:44-01","swversion":"0"}}`

const groups = `{
	"1":{"name":"Living room","lights":["1","2"],"sensors":[],"type":"Room","state":{"all_on":false,"any_on":true},"recycle":false,"class":"Living room","action":{"on":true}},
	"2":{"name":"Hallway","lights":["10"],"sensors":[],"type":"Room","class":"Hallway"},
	"3":{"name":"Downstairs","lights":["1","10"],"type":"Zone","class":"Downstairs"},
	"200":{"name":"Entertainment","lights":["1"],"type":"Entertainment","class":"TV"}}`

func TestImport(t *testing.T) {
	c, err := parseConfig([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	ls, err := parseLights([]byte(lights))
	if err != nil || len(ls) != 3 || ls[2].ID != "10" {
		t.Fatalf("lights: %+v %v", ls, err)
	}
	rooms, err := parseRooms([]byte(groups))
	if err != nil || len(rooms) != 2 {
		t.Fatalf("rooms (zones and entertainment areas skipped): %+v %v", rooms, err)
	}
	// "Hallway" exists already (e.g. from Home Assistant) and is reused.
	existing := roomIndex([]pdk.Entity{{Kind: "room", Slug: "ha-hallway", Name: " hallway"}})
	ops := graphOps(c, ls, rooms, existing).Ops
	b := ops[0]
	if b.Kind != "device" || b.ID != "hue-bridge-001788fffe4a1b2c" || b.Name != "Hue Bridge Home" || b.Attrs["model"] != "BSB002" ||
		b.Attrs["ip"] != "192.168.1.20" || b.Attrs["mac"] != "00:17:88:4A:1B:2C" || b.Attrs["firmware"] != "1967054020" || b.Attrs["manufacturer"] != "Signify" {
		t.Fatalf("bridge: %+v", b)
	}
	if ops[1].Kind != "room" || ops[1].ID != "hue-room-1" || ops[1].Name != "Living room" {
		t.Fatalf("room: %+v", ops[1])
	}
	sofa := ops[2]
	if sofa.Kind != "light" || sofa.ID != "hue-light-00-17-88-01-04-a1-b2-c3-0b" || sofa.Attrs["model"] != "LCT015" || sofa.Attrs["reachable"] != true ||
		sofa.Attrs["product"] != "Hue color lamp" || sofa.Attrs["uniqueid"] != "00:17:88:01:04:a1:b2:c3-0b" {
		t.Fatalf("light: %+v", sofa)
	}
	if ops[3].Op != "link" || ops[3].Rel != "located_in" || ops[3].From != sofa.Ref || ops[3].To != ops[1].Ref ||
		ops[4].Rel != "related_to" || ops[4].To != b.Ref {
		t.Fatalf("links: %+v %+v", ops[3], ops[4])
	}
	hall := ops[len(ops)-3]
	if hall.Name != "Hallway" || hall.Attrs["reachable"] != false || ops[len(ops)-2].To != "ha-hallway" {
		t.Fatalf("hallway: %+v %+v", hall, ops[len(ops)-2])
	}
	// bridge + 1 new room + 3 lights + 3 located_in + 3 related_to
	if len(ops) != 11 {
		t.Fatalf("ops: %d", len(ops))
	}
	rows := lightRows(ls, rooms)
	if rows[0].Room != "Living room" || *rows[0].Brightness != 50 || !*rows[0].On || rows[2].Room != "Hallway" || *rows[2].Reachable {
		t.Fatalf("rows: %+v", rows)
	}
}

func TestErrors(t *testing.T) {
	_, err := parseLights([]byte(`[{"error":{"type":1,"address":"/lights","description":"unauthorized user"}}]`))
	if err == nil || !strings.Contains(err.Error(), "username") {
		t.Fatalf("unauthorized: %v", err)
	}
	if _, err := parseConfig([]byte(`[{"error":{"type":4,"address":"/config","description":"method, GET, not available for resource"}}]`)); err == nil {
		t.Fatal("error list accepted")
	}
	if _, err := parseConfig([]byte(`{"name":"router"}`)); err == nil {
		t.Fatal("no bridge accepted")
	}
	if a, err := apiBase("http://192.168.1.20/", key); err != nil || a != "http://192.168.1.20/api/"+key {
		t.Fatal(a, err)
	}
	if _, err := apiBase("https://192.168.1.20", key); err == nil {
		t.Fatal("https accepted")
	}
	if _, err := apiBase("http://192.168.1.20", "../config"); err == nil {
		t.Fatal("path in username accepted")
	}
	msg := redact(`fetch 192.168.1.20: Get "http://192.168.1.20/api/`+key+`/lights": i/o timeout`, key)
	if strings.Contains(msg, key) {
		t.Fatal(msg)
	}
}
