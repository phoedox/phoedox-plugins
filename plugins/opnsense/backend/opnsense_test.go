package main

import "testing"

func TestOPNsense(t *testing.T) {
	if firmwareVersion([]byte(`{"product":{"product_version":"24.7.8"}}`)) != "24.7.8" || firmwareVersion([]byte(`{"product_version":"23.1"}`)) != "23.1" {
		t.Fatal("version")
	}
	ifaces, err := parseIfaces([]byte(`{"rows":[
		{"identifier":"lan","description":"LAN","device":"igb1","status":"up","addr4":"192.168.1.1/24","vlan_tag":null},
		{"identifier":"opt2","description":"IoT","device":"vlan0.30","status":"up","addr4":"10.30.0.1/24","vlan_tag":"30"},
		{"identifier":"wan","description":"WAN","device":"igb0","status":"up","addr4":"","vlan_tag":null},
		{"identifier":"lo0","device":"lo0"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	ops := graphOps("Firewall", "24.7.8", ifaces)
	if ops.Ops[0].Kind != "router" || ops.Ops[0].ID != "opnsense-firewall" || ops.Ops[0].Attrs["ip"] != "192.168.1.1" {
		t.Fatalf("router: %+v", ops.Ops[0])
	}
	iot := ops.Ops[3]
	if iot.Kind != "opnsense_interface" || iot.Name != "Firewall IoT" || iot.Attrs["subnet"] != "10.30.0.0/24" || iot.Attrs["vlan"] != 30 {
		t.Fatalf("iot: %+v", iot)
	}
	if len(ops.Ops) != 7 || ops.Ops[6].Rel != "part_of" {
		t.Fatalf("ops (loopback skipped): %d %+v", len(ops.Ops), ops.Ops)
	}
	leases, err := parseLeases([]byte(`{"rows":[{"address":"192.168.1.50","hwaddr":"aa:bb:cc:dd:ee:ff","hostname":"nas","if_descr":"LAN","state":"active"}]}`))
	if err != nil || leases[0].Hostname != "nas" {
		t.Fatalf("leases: %+v %v", leases, err)
	}
	gws, err := parseGateways([]byte(`{"items":[{"name":"WAN_DHCP","address":"100.64.0.1","status_translated":"Online","delay":"4.1 ms","loss":"0.0 %"}]}`))
	if err != nil || gws[0].Status != "Online" {
		t.Fatalf("gateways: %+v %v", gws, err)
	}
}
