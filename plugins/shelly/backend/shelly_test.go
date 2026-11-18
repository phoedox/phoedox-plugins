package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
)

const gen2Info = `{"name":"Kitchen Plug","id":"shellyplusplugs-e86beae12345","mac":"E86BEAE12345","slot":0,"model":"SNPL-00112EU","gen":2,
	"fw_id":"20240625-122245/1.3.3-gbdfd9b3","ver":"1.3.3","app":"PlusPlugS","auth_en":true,"auth_domain":"shellyplusplugs-e86beae12345"}`

const gen2Status = `{"ble":{},"cloud":{"connected":false},"mqtt":{"connected":false},
	"switch:0":{"id":0,"source":"init","output":true,"apower":42.7,"voltage":231.4,"current":0.21,
		"aenergy":{"total":12345.678,"by_minute":[1,2,3],"minute_ts":1731577200},"temperature":{"tC":38.2,"tF":100.8}},
	"sys":{"mac":"E86BEAE12345","restart_required":false,"uptime":86400,"available_updates":{"stable":{"version":"1.4.4"}}},
	"wifi":{"sta_ip":"192.168.20.41","status":"got ip","ssid":"IoT","rssi":-61},"ws":{"connected":false}}`

func TestGen2(t *testing.T) {
	info, err := parseInfo([]byte(gen2Info))
	if err != nil || info.Gen != 2 || info.model() != "SNPL-00112EU" || info.firmware() != "1.3.3" {
		t.Fatalf("info: %+v %v", info, err)
	}
	st, err := parseGen2Status([]byte(gen2Status))
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Channels) != 1 || *st.Channels[0].PowerW != 42.7 || *st.Channels[0].EnergyKWh != 12.346 || *st.Channels[0].TempC != 38.2 || !*st.Channels[0].On {
		t.Fatalf("channels: %+v", st.Channels)
	}
	if st.IP != "192.168.20.41" || *st.RSSI != -61 || st.Update != "1.4.4" || !st.metered() {
		t.Fatalf("status: %+v", st)
	}
	d := Device{Setting: "url", Host: "shelly-kitchen.lan", Info: info, Status: st}
	ops := graphOps(d)
	op := ops.Ops[0]
	if len(ops.Ops) != 1 || op.Kind != "smart_plug" || op.ID != "shelly-e86beae12345" || op.Name != "Kitchen Plug" {
		t.Fatalf("op: %+v", op)
	}
	want := map[string]any{"manufacturer": "Shelly", "model": "SNPL-00112EU", "mac": "E8:6B:EA:E1:23:45", "ip": "192.168.20.41",
		"firmware": "1.3.3", "shelly_gen": 2, "shelly_app": "PlusPlugS", "hostname": "shellyplusplugs-e86beae12345"}
	for k, v := range want {
		if op.Attrs[k] != v {
			t.Errorf("attr %s: %v, want %v", k, op.Attrs[k], v)
		}
	}
	r := row(d)
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"power_w":42.7`) || r.Update != "1.4.4" {
		t.Fatalf("row: %s", b)
	}
}

func TestGen2WithoutMetering(t *testing.T) {
	// Plus i4 (inputs only) and Plus 2PM as cover: no switch with power.
	st, err := parseGen2Status([]byte(`{"input:0":{"id":0,"state":false},"cover:0":{"id":0,"state":"stopped","apower":0,"current_pos":50},
		"wifi":{"sta_ip":null,"status":"disconnected"},"eth":{"ip":"10.0.0.7"},"sys":{"uptime":5}}`))
	if err != nil || st.metered() || len(st.Channels) != 1 || st.Channels[0].Type != "cover" || st.IP != "10.0.0.7" {
		t.Fatalf("%+v %v", st, err)
	}
	info, _ := parseInfo([]byte(`{"name":null,"id":"shellyplus2pm-a0b1c2d3e4f5","mac":"A0B1C2D3E4F5","model":"SNSW-102P16EU","gen":2,"ver":"1.4.4","app":"Plus2PM"}`))
	op := graphOps(Device{Host: "192.168.1.9", Info: info, Status: st}).Ops[0]
	if op.Kind != "device" || op.Name != "Shelly Plus2PM D3E4F5" || op.Attrs["ip"] != "10.0.0.7" {
		t.Fatalf("op: %+v", op)
	}
}

func TestGen1(t *testing.T) {
	info, err := parseInfo([]byte(`{"type":"SHPLG-S","mac":"C45BBE6A1B2C","auth":true,"fw":"20230913-112003/v1.14.0-gcb84623","num_outputs":1,"num_meters":1}`))
	if err != nil || info.Gen != 1 || info.model() != "SHPLG-S" {
		t.Fatalf("info: %+v %v", info, err)
	}
	st, err := parseGen1Status([]byte(`{"wifi_sta":{"connected":true,"ssid":"IoT","ip":"192.168.20.52","rssi":-70},
		"relays":[{"ison":false,"has_timer":false,"overpower":false,"source":"http"}],
		"meters":[{"power":0.0,"overpower":0.0,"is_valid":true,"timestamp":1731577200,"counters":[0,0,0],"total":600000}],
		"temperature":24.31,"overtemperature":false,"update":{"status":"pending","has_update":true,"new_version":"20230913-112003/v1.14.0","old_version":"20221027-091427/v1.12.1"},
		"uptime":3600}`))
	if err != nil || !st.metered() || *st.Channels[0].EnergyKWh != 10 || *st.TempC != 24.31 || st.Update == "" || *st.RSSI != -70 {
		t.Fatalf("status: %+v %v", st, err)
	}
	s := parseSettings1([]byte(`{"device":{"type":"SHPLG-S","mac":"C45BBE6A1B2C","hostname":"shellyplug-s-6A1B2C"},"name":"Desk Lamp"}`))
	d := Device{Info: info, Status: st, Name: *s.Name, Hostname: s.Device.Hostname}
	op := graphOps(d).Ops[0]
	if op.Kind != "smart_plug" || op.ID != "shelly-c45bbe6a1b2c" || op.Name != "Desk Lamp" || op.Attrs["hostname"] != "shellyplug-s-6A1B2C" || op.Attrs["shelly_gen"] != 1 {
		t.Fatalf("op: %+v", op)
	}
	// Shelly EM: energy meters only.
	st, _ = parseGen1Status([]byte(`{"relays":[{"ison":true}],"emeters":[{"power":512.3,"voltage":229.8,"total":1500}]}`))
	if st.metered() || len(st.Channels) != 2 || *st.Channels[1].EnergyKWh != 1.5 {
		t.Fatalf("em: %+v", st)
	}
}

func TestErrors(t *testing.T) {
	if _, err := parseInfo([]byte(`<html>router login</html>`)); err == nil {
		t.Fatal("html accepted")
	}
	if _, err := parseInfo([]byte(`{"name":"x"}`)); err == nil {
		t.Fatal("missing mac accepted")
	}
	if _, err := parseGen2Status([]byte(`[]`)); err == nil {
		t.Fatal("array accepted")
	}
	if _, _, err := deviceBase("device2_url", "http://admin:pw@192.168.1.5"); err == nil || strings.Contains(err.Error(), "pw") {
		t.Fatalf("credentials in url: %v", err)
	}
	if b, h, err := deviceBase("url", "http://192.168.1.5/ "); err != nil || b != "http://192.168.1.5" || h != "192.168.1.5" {
		t.Fatal(b, h, err)
	}
}

func TestDigest(t *testing.T) {
	ch := `Digest qop="auth", realm="shellyplus1pm-a8032ab12345", nonce="60dc59c6", algorithm=SHA-256`
	got, err := digestAuth(ch, "GET", "/rpc/Shelly.GetStatus", "admin", "secret", "0a4f113b")
	if err != nil {
		t.Fatal(err)
	}
	h := func(s string) string { x := sha256.Sum256([]byte(s)); return hex.EncodeToString(x[:]) }
	ha1 := h("admin:shellyplus1pm-a8032ab12345:secret")
	ha2 := h("GET:/rpc/Shelly.GetStatus")
	want := h(ha1 + ":60dc59c6:00000001:0a4f113b:auth:" + ha2)
	if !strings.Contains(got, `response="`+want+`"`) || !strings.Contains(got, `realm="shellyplus1pm-a8032ab12345"`) || strings.Contains(got, "secret") {
		t.Fatalf("digest: %s", got)
	}
	if _, err := digestAuth(`Basic realm="x"`, "GET", "/", "admin", "x", "1"); err == nil {
		t.Fatal("basic challenge accepted")
	}
	if _, err := digestAuth(`Digest realm="x", nonce="1", algorithm=SHA-512-256`, "GET", "/", "admin", "x", "1"); err == nil {
		t.Fatal("unknown algorithm accepted")
	}
	p, ok := parseChallenge(`Digest realm="a, b", nonce=abc, qop="auth,auth-int"`)
	if !ok || p["realm"] != "a, b" || p["nonce"] != "abc" || p["qop"] != "auth,auth-int" {
		t.Fatalf("challenge: %v", p)
	}
}

// TestManifestSlots: every address setting is declared as a secret, so the
// host lets the plugin read it and net:fetch:config allows its host.
func TestManifestSlots(t *testing.T) {
	data, err := os.ReadFile("../phoedox-plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := pdk.ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range slots() {
		if !slices.Contains(m.Permissions, "secrets:"+s) || (s != "url" && !strings.HasSuffix(s, "_url")) {
			t.Errorf("slot %s not declared", s)
		}
	}
	if len(slots()) != maxDevices || fmtMAC("e8-6b-ea-e1-23-45") != "E8:6B:EA:E1:23:45" {
		t.Fatal("helpers")
	}
}

// TestDemoData parses the recorded demo answers the screenshots use.
func TestDemoData(t *testing.T) {
	get := func(p string) []byte {
		body, err := fakeapi.Body("../testdata/fake", "GET", p)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	kinds := map[string]int{}
	for _, slug := range []string{"rack-plug", "media-plug", "main-meter", "garden-pump", "shutters", "basement-climate", "garage-door", "coffee-plug"} {
		info, err := parseInfo(get("/" + slug + "/shelly"))
		if err != nil {
			t.Fatal(slug, err)
		}
		d := Device{Setting: "url", Host: "127.0.0.1", Info: info}
		if info.Gen >= 2 {
			d.Status, err = parseGen2Status(get("/" + slug + "/rpc/Shelly.GetStatus"))
		} else {
			d.Status, err = parseGen1Status(get("/" + slug + "/status"))
			s := parseSettings1(get("/" + slug + "/settings"))
			d.Name, d.Hostname = *s.Name, s.Device.Hostname
		}
		if err != nil {
			t.Fatal(slug, err)
		}
		op := graphOps(d).Ops[0]
		if op.Attrs["ip"] == "127.0.0.1" || op.Attrs["ip"] == nil {
			t.Fatalf("%s: ip %v", slug, op.Attrs["ip"])
		}
		kinds[op.Kind]++
	}
	if kinds["smart_plug"] != 4 || kinds["device"] != 4 {
		t.Fatalf("kinds: %v", kinds)
	}
}
