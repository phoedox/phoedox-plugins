package main

import (
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// Device Builder (2026.6+): runtime state flattened into each entry.
const builderDevices = `{"configured":[
 {"name":"kitchen-sensor","friendly_name":"Kitchen Sensor","configuration":"kitchen-sensor.yaml","comment":"BME280 above the fridge","area":"Kitchen",
  "board_id":"esp32dev","target_platform":"esp32","address":"kitchen-sensor.local","ip":"192.168.20.71","web_port":80,"current_version":"2026.10.2",
  "loaded_integrations":["api","esp32","logger","ota","sensor","web_server","wifi"],"update_available":true,"uses_mqtt":false,"mac_address":"94:C9:60:1F:8C:F1",
  "state":"online","active_source":"mdns","ip_addresses":["192.168.20.71"],"offline_seconds":null,"deployed_version":"2026.8.0","network":"wifi"},
 {"name":"garage-door","friendly_name":"","configuration":"garage-door.yaml","comment":null,"area":"kitchen ","board_id":"","target_platform":"esp8266",
  "address":"garage-door.local","ip":"","ip_addresses":[],"mac_address":"","state":"offline","deployed_version":"2026.10.2","current_version":"2026.10.2",
  "loaded_integrations":["api","esp8266"],"uses_deep_sleep":true},
 {"name":"office-plug","friendly_name":"Office Plug","configuration":"office-plug.yaml","area":"Office","target_platform":"bk72xx","address":"192.168.20.80",
  "state":"unknown","deployed_version":"","current_version":"2026.10.2","loaded_integrations":[]}],
 "importable":[{"name":"m5stack-atom-echo","friendly_name":"M5Stack Atom Echo","package_import_url":"github://esphome/wake-word-voice-assistants/m5stack-atom-echo.yaml",
  "project_name":"m5stack.atom-echo-wake-word-voice-assistant","project_version":"24.7.24","network":"wifi","ignored":false}]}`

// Legacy dashboard (before 2026.6).
const legacyDevices = `{"configured":[{"name":"living-room","friendly_name":"Living Room","configuration":"living-room.yaml",
 "loaded_integrations":["api","esp32","wifi"],"deployed_version":"2024.11.2","current_version":"2025.2.0","path":"/config/living-room.yaml",
 "comment":null,"address":"living-room.local","web_port":null,"target_platform":"ESP32"}],"importable":[]}`

func TestBuilder(t *testing.T) {
	d, err := parseDevices([]byte(builderDevices))
	if err != nil || len(d.Configured) != 3 || len(d.Importable) != 1 {
		t.Fatalf("%+v %v", d, err)
	}
	existing := roomIndex([]pdk.Entity{{Kind: "room", Slug: "office", Name: "Office"}})
	ops := graphOps(d.Configured, existing).Ops
	k := ops[0]
	if k.Kind != "device" || k.ID != "esphome-kitchen-sensor" || k.Name != "Kitchen Sensor" || k.Attrs["model"] != "esp32dev" || k.Attrs["platform"] != "esp32" ||
		k.Attrs["ip"] != "192.168.20.71" || k.Attrs["mac"] != "94:C9:60:1F:8C:F1" || k.Attrs["hostname"] != "kitchen-sensor.local" ||
		k.Attrs["firmware"] != "2026.8.0" || k.Attrs["comment"] != "BME280 above the fridge" || k.Attrs["esphome_config"] != "kitchen-sensor.yaml" {
		t.Fatalf("kitchen: %+v", k)
	}
	if ops[1].Kind != "room" || ops[1].ID != "esphome-room-kitchen" || ops[2].Rel != "located_in" || ops[2].To != ops[1].Ref {
		t.Fatalf("room: %+v %+v", ops[1], ops[2])
	}
	// The second device's area "kitchen " is the same room, created once.
	g := ops[3]
	if g.Name != "garage-door" || g.Attrs["model"] != "esp8266" || g.Attrs["ip"] != nil || ops[4].To != ops[1].Ref {
		t.Fatalf("garage: %+v %+v", g, ops[4])
	}
	// "Office" exists already; the address is an IP.
	if ops[5].Attrs["ip"] != "192.168.20.80" || ops[6].To != "office" || len(ops) != 7 {
		t.Fatalf("office: %+v", ops[5:])
	}
	o := overview(d, map[string]*bool{"office-plug.yaml": ptr(true)})
	if !*o.Devices[0].Online || !o.Devices[0].UpdateAvailable || !o.Devices[0].WebServer || *o.Devices[1].Online || o.Devices[1].UpdateAvailable ||
		!*o.Devices[2].Online || len(o.Discovered) != 1 || o.Discovered[0].ProjectName == "" {
		t.Fatalf("overview: %+v", o)
	}
}

func TestLegacy(t *testing.T) {
	d, err := parseDevices([]byte(legacyDevices))
	if err != nil {
		t.Fatal(err)
	}
	ops := graphOps(d.Configured, nil).Ops
	if len(ops) != 1 || ops[0].Attrs["model"] != "ESP32" || ops[0].Attrs["firmware"] != "2024.11.2" || ops[0].Attrs["mac"] != nil {
		t.Fatalf("ops: %+v", ops)
	}
	ping := parsePing([]byte(`{"living-room.yaml":null}`))
	o := overview(d, ping)
	if o.Devices[0].Online != nil || !o.Devices[0].UpdateAvailable {
		t.Fatalf("overview: %+v", o.Devices[0])
	}
	if len(parsePing([]byte(`<html>`))) != 0 {
		t.Fatal("ping html")
	}
}

func TestErrors(t *testing.T) {
	for _, b := range []string{`<!doctype html><html>`, `{"error":"x"}`, `[]`} {
		if _, err := parseDevices([]byte(b)); err == nil {
			t.Errorf("accepted %s", b)
		}
	}
}
