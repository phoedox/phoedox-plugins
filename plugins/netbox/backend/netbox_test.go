package main

import (
	"encoding/json"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
)

func TestNetBox(t *testing.T) {
	var locations []Location
	_ = json.Unmarshal([]byte(`[{"id":3,"name":"Server room","site":{"id":1,"name":"HQ"}}]`), &locations)
	var racks []Rack
	_ = json.Unmarshal([]byte(`[{"id":7,"name":"R1","u_height":42,"outer_depth":1000,"outer_unit":{"value":"mm"},"location":{"id":3}}]`), &racks)
	types := map[int]DeviceType{10: {ID: 10, Model: "R750", UHeight: 2, Manufacturer: &ref{Name: "Dell"}}}
	var devices []Device
	_ = json.Unmarshal([]byte(`[
		{"id":1,"name":"esx01","role":{"slug":"hypervisor"},"device_type":{"id":10,"model":"R750"},"serial":"ABC","primary_ip":{"address":"10.0.0.11/24"},"rack":{"id":7,"name":"R1"},"position":20,"status":{"value":"active"}},
		{"id":2,"name":"core-sw","device_role":{"slug":"core-switch"},"device_type":{"id":11,"model":"CRS326"},"rack":{"id":7},"position":40},
		{"id":3,"name":"ap-hall","role":{"slug":"access-point"},"location":{"id":3},"rack":{"id":7}},
		{"id":4,"name":"","role":{"slug":"blank"}},
		{"id":5,"name":"ups","role":{"slug":"power"},"device_type":{"id":12,"model":"SMT1500","manufacturer":{"name":"APC"}}}]`), &devices)
	ops := graphOps(locations, racks, types, devices)
	if ops.Ops[0].Kind != "room" || ops.Ops[1].Kind != "rack" || ops.Ops[1].Attrs["height_units"] != 42 || ops.Ops[1].Attrs["depth"] != 1000 || ops.Ops[2].Rel != "located_in" {
		t.Fatalf("room and rack: %+v", ops.Ops[:3])
	}
	esx := ops.Ops[3]
	if esx.Kind != "server" || esx.Attrs["ip"] != "10.0.0.11" || esx.Attrs["model"] != "R750" || esx.ID != "nb-dev-1-esx01" {
		t.Fatalf("server: %+v", esx)
	}
	if m := ops.Ops[4]; m.Rel != "mounted_in" || m.Attrs["u"] != 20 || m.Attrs["height"] != 2 {
		t.Fatalf("mount: %+v", m)
	}
	if ops.Ops[5].Kind != "switch" || ops.Ops[6].Attrs["height"] != 1 {
		t.Fatalf("switch (v3 role, default height): %+v %+v", ops.Ops[5], ops.Ops[6])
	}
	if ops.Ops[7].Kind != "access_point" || ops.Ops[8].Rel != "located_in" {
		t.Fatalf("access points stand in rooms: %+v %+v", ops.Ops[7], ops.Ops[8])
	}
	if ups := ops.Ops[9]; ups.Kind != "device" || ups.Attrs["manufacturer"] != "APC" || len(ops.Ops) != 10 {
		t.Fatalf("device: %+v (%d ops)", ups, len(ops.Ops))
	}
	f := found(devices[:1])
	if f[0].Rack != "R1" || f[0].Position != 20 || f[0].Role != "hypervisor" {
		t.Fatalf("found: %+v", f)
	}
	var prefixes []Prefix
	_ = json.Unmarshal([]byte(`[{"prefix":"10.30.0.0/24","description":"IoT","vlan":{"vid":30,"name":"iot"},"status":{"value":"active"}}]`), &prefixes)
	if p := prefixInfo(prefixes); p[0].VLAN != "30 iot" || p[0].Status != "active" {
		t.Fatalf("prefixes: %+v", p)
	}
}

// TestDemoData parses the recorded demo answers the screenshots use.
func TestDemoData(t *testing.T) {
	get := func(p string, out any) {
		body, err := fakeapi.Body("../testdata/fake", "GET", "/api/"+p+"?limit=500&offset=0")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatal(p, err)
		}
	}
	var locations Page[Location]
	var racks Page[Rack]
	var dts Page[DeviceType]
	var devices Page[Device]
	var prefixes Page[Prefix]
	get("dcim/locations/", &locations)
	get("dcim/racks/", &racks)
	get("dcim/device-types/", &dts)
	get("dcim/devices/", &devices)
	get("ipam/prefixes/", &prefixes)
	types := map[int]DeviceType{}
	for _, d := range dts.Results {
		types[d.ID] = d
	}
	ops := graphOps(locations.Results, racks.Results, types, devices.Results)
	kinds, mounted := map[string]int{}, 0
	for _, o := range ops.Ops {
		if o.Op == "upsert" {
			kinds[o.Kind]++
		} else if o.Rel == "mounted_in" {
			mounted++
		}
	}
	if kinds["room"] != 3 || kinds["rack"] != 1 || kinds["server"] != 3 || kinds["router"] != 1 || kinds["switch"] != 2 ||
		kinds["access_point"] != 2 || kinds["patch_panel"] != 1 || kinds["device"] != 1 || mounted != 7 {
		t.Fatalf("kinds %v mounted %d", kinds, mounted)
	}
	if p := prefixInfo(prefixes.Results); len(p) != 5 || p[1].VLAN != "30 IoT" {
		t.Fatalf("prefixes: %+v", p)
	}
	if f := found(devices.Results); f[4].Rack != "Rack 1" || f[4].Position != 16 {
		t.Fatalf("found: %+v", f[4])
	}
}

// TestPatchPanelPorts: Phoedox requires the ports of a patch panel; a
// patch panel type without front ports becomes a device.
func TestPatchPanelPorts(t *testing.T) {
	rack := []Rack{{ID: 1, Name: "R1", UHeight: 12}}
	types := map[int]DeviceType{1: {ID: 1, Model: "PP-24", UHeight: 1, FrontPorts: 24}, 2: {ID: 2, Model: "Keystone 6", UHeight: 1}}
	pos := 12.0
	devices := []Device{
		{ID: 1, Name: "patch-a", Role: &ref{Slug: "patch-panel"}, DeviceType: &DeviceType{ID: 1}, Rack: &ref{ID: 1}, Position: &pos},
		{ID: 2, Name: "patch-b", Role: &ref{Slug: "patch-panel"}, DeviceType: &DeviceType{ID: 2}, Rack: &ref{ID: 1}},
	}
	ops := graphOps(nil, rack, types, devices).Ops
	if ops[1].Kind != "patch_panel" || ops[1].Attrs["ports"] != 24 || ops[3].Kind != "device" || ops[3].Attrs["ports"] != nil {
		t.Fatalf("ops: %+v", ops)
	}
}
