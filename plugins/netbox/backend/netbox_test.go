package main

import (
	"encoding/json"
	"testing"
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
