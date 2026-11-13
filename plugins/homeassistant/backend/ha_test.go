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
