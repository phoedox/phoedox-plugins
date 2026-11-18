package main

import (
	"encoding/json"
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
)

func TestHomeAssistant(t *testing.T) {
	body := []byte(`[{"area":"Keller","id":"a1b2c3d4e5","name":"Switch","manufacturer":"Ubiquiti","model":"USW-8"},
		{"area":"Küche","id":"ffee0011","name":null,"manufacturer":null,"model":null},
		{"area":"Keller","id":"0099aa","name":"NAS","manufacturer":"Synology","model":"DS920+"}]`)
	devs, err := parseDevices(body)
	if err != nil || len(devs) != 3 {
		t.Fatal(err)
	}
	ops := graphOps(devs)
	rooms, devices, links := 0, 0, 0
	for _, o := range ops.Ops {
		switch {
		case o.Op == "upsert" && o.Kind == "room":
			rooms++
		case o.Op == "upsert" && o.Kind == "device":
			devices++
		case o.Op == "link":
			links++
		}
	}
	if rooms != 2 || devices != 3 || links != 3 {
		t.Fatalf("rooms %d devices %d links %d", rooms, devices, links)
	}
	if ops.Ops[0].ID != "ha-keller" || ops.Ops[1].ID != "ha-switch-a1b2c3" {
		t.Fatalf("slugs: %+v %+v", ops.Ops[0], ops.Ops[1])
	}
	if slugify("Küche & Flur") != "kueche-flur" {
		t.Fatal(slugify("Küche & Flur"))
	}
	if _, err := validEntities([]string{"light.flur", "rm -rf"}); err == nil {
		t.Fatal("bad entity id must fail")
	}
}

func TestControls(t *testing.T) {
	devs, _ := parseDevices([]byte(`[{"area":"Flur","id":"abc123","name":"Steckdose Router","entities":["sensor.steckdose_power","switch.steckdose_router"]}]`))
	ops := graphOps(devs)
	attrs := ops.Ops[1].Attrs
	if ops.Ops[1].Kind != "smart_plug" {
		t.Fatalf("a device with a switch is a smart plug: %+v", ops.Ops[1])
	}
	if attrs["ha_entity"] != "switch.steckdose_router" || attrs["ha_entities"] != "sensor.steckdose_power, switch.steckdose_router" {
		t.Fatalf("binding: %+v", attrs)
	}
	if primaryEntity([]string{"sensor.x", "light.decke", "switch.a"}) != "switch.a" || primaryEntity([]string{"sensor.x"}) != "" {
		t.Fatal("primary entity")
	}
	for _, c := range []CallArgs{{EntityID: "sensor.temp", Action: "on"}, {EntityID: "switch.a", Action: "explode"}, {EntityID: "switch.a; rm", Action: "on"}} {
		if _, _, err := c.service(); err == nil {
			t.Errorf("%+v must fail", c)
		}
	}
	if d, s, err := (CallArgs{EntityID: "light.flur", Action: "toggle"}).service(); err != nil || d != "light" || s != "toggle" {
		t.Fatal(d, s, err)
	}
	if (RestartArgs{}).pause() != 10 || (RestartArgs{Seconds: 1}).pause() != 3 || (RestartArgs{Seconds: 99}).pause() != 30 {
		t.Fatal("pause bounds")
	}
}

func TestReadings(t *testing.T) {
	owners := Owners([]pdk.Entity{
		{Slug: "ha-keller", Attrs: map[string]any{"ha_entities": "sensor.keller_temperatur, switch.keller_licht, bad id"}},
		{Slug: "ha-flur", Attrs: map[string]any{}},
	})
	if len(owners) != 2 || owners["switch.keller_licht"] != "ha-keller" {
		t.Fatalf("owners: %v", owners)
	}
	got := Readings([]State{
		{EntityID: "sensor.keller_temperatur", State: "18.5", Attributes: map[string]any{"unit_of_measurement": "°C"}},
		{EntityID: "switch.keller_licht", State: "on"},
		{EntityID: "sensor.keller_temperatur", State: "unavailable"},
		{EntityID: "sensor.other", State: "3"},
	}, owners)
	if len(got) != 2 || got[0].Value != 18.5 || got[0].Unit != "°C" || got[0].Entity != "ha-keller" || got[1].Value != 1 || got[1].Metric != "switch.keller_licht" {
		t.Fatalf("readings: %+v", got)
	}
}

func TestBridge(t *testing.T) {
	name := "Plug"
	ops := graphOpsVia([]Device{{ID: "abcdef12", Name: &name, Area: "Flur", Entities: []string{"switch.plug"}}}, "agent-1")
	if len(ops.Ops) != 3 || ops.Ops[1].Attrs["bridge_agent"] != "agent-1" || ops.Ops[1].Attrs["ha_entity"] != "switch.plug" {
		t.Fatalf("ops: %+v", ops.Ops)
	}
	if direct := graphOps([]Device{{ID: "abcdef12", Name: &name, Area: "Flur", Entities: []string{"switch.plug"}}}); direct.Ops[1].Attrs["bridge_agent"] != nil {
		t.Fatalf("direct import names an agent: %+v", direct.Ops[1])
	}
	var b Bridge
	_ = json.Unmarshal([]byte(`{"agent":"a","states":[{"entity_id":"sensor.t","state":"21.5","unit":"°C"},{"entity_id":"switch.plug","state":"off"}]}`), &b)
	rs := Readings(b.BridgeStates(), map[string]string{"sensor.t": "x", "switch.plug": "y"})
	if len(rs) != 2 || rs[0].Unit != "°C" || rs[0].Value != 21.5 || rs[1].Value != 0 {
		t.Fatalf("bridged readings: %+v", rs)
	}
}

// TestDemoData parses the recorded demo answers the screenshots use.
func TestDemoData(t *testing.T) {
	body, err := fakeapi.Body("../testdata/fake", "POST", "/api/template")
	if err != nil {
		t.Fatal(err)
	}
	devs, err := parseDevices(body)
	if err != nil || len(devs) != 13 {
		t.Fatalf("%v %d", err, len(devs))
	}
	ops := graphOps(devs)
	if len(ops.Ops) != 7+13+13 {
		t.Fatalf("ops: %d", len(ops.Ops))
	}
	var entries []pdk.Entity
	for _, o := range ops.Ops {
		if o.Op == "upsert" {
			entries = append(entries, pdk.Entity{Slug: o.ID, Attrs: o.Attrs})
		}
	}
	if body, err = fakeapi.Body("../testdata/fake", "GET", "/api/states"); err != nil {
		t.Fatal(err)
	}
	var states []State
	if err := json.Unmarshal(body, &states); err != nil {
		t.Fatal(err)
	}
	if rs := Readings(states, Owners(entries)); len(rs) != 23 {
		t.Fatalf("readings: %d", len(rs))
	}
	if body, err = fakeapi.Body("../testdata/fake", "GET", "/api/states/switch.rack_plug"); err != nil {
		t.Fatal(err)
	}
	var s State
	if err := json.Unmarshal(body, &s); err != nil || s.State != "on" {
		t.Fatalf("%v %+v", err, s)
	}
}
