package main

import "testing"

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
