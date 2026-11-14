package main

import (
	"strings"
	"testing"
)

const (
	identityJSON = `{"name":"core-rtr"}`
	resourceJSON = `{"architecture-name":"arm64","board-name":"hAP ax^3","build-time":"2024-10-29 13:23:46","cpu":"ARM64","cpu-count":"4",
		"cpu-load":"1","free-memory":"880279552","platform":"MikroTik","total-memory":"1073741824","uptime":"2w3d4h","version":"7.16.1 (stable)"}`
	routerboardJSON = `{"board-name":"hAP ax^3","current-firmware":"7.16.1","factory-firmware":"7.10","firmware-type":"ipq5000",
		"model":"C53UiG+5HPaxD2HPaxD","routerboard":"true","serial-number":"HEX0A1B2C3D","upgrade-firmware":"7.16.1"}`
	interfacesJSON = `[
		{".id":"*1","name":"ether1","type":"ether","mac-address":"48:A9:8A:01:02:03","running":"true","disabled":"false","mtu":"1500","actual-mtu":"1500","comment":"WAN"},
		{".id":"*2","name":"ether2","type":"ether","mac-address":"48:A9:8A:01:02:04","running":"false","disabled":"false","mtu":"1500"},
		{".id":"*9","name":"bridge","type":"bridge","mac-address":"48:A9:8A:01:02:04","running":"true","disabled":"false","actual-mtu":"1500"},
		{".id":"*A","name":"lo","type":"loopback","mac-address":"00:00:00:00:00:00","running":"true","disabled":"false"}]`
	addressesJSON = `[
		{".id":"*1","address":"100.70.1.2/24","interface":"ether1","network":"100.70.1.0","dynamic":"true","disabled":"false","invalid":"false"},
		{".id":"*2","address":"192.168.88.1/24","interface":"bridge","network":"192.168.88.0","dynamic":"false","disabled":"false","invalid":"false"},
		{".id":"*3","address":"10.9.9.1/24","interface":"bridge","dynamic":"false","disabled":"true","invalid":"false"},
		{".id":"*4","address":"10.8.8.1/24","interface":"ether9","dynamic":"false","disabled":"false","invalid":"true"}]`
	leasesJSON = `[
		{".id":"*1","address":"192.168.88.254","mac-address":"AA:BB:CC:DD:EE:01","host-name":"nas","server":"defconf","status":"bound","dynamic":"true","disabled":"false","expires-after":"8m12s","last-seen":"1m48s"},
		{".id":"*2","address":"192.168.88.10","active-address":"192.168.88.10","mac-address":"AA:BB:CC:DD:EE:02","server":"defconf","status":"waiting","dynamic":"false","disabled":"false","last-seen":"never","comment":"printer"}]`
)

func TestSystemAndGraph(t *testing.T) {
	sys, err := parseSystem([]byte(identityJSON), []byte(resourceJSON), []byte(routerboardJSON))
	if err != nil {
		t.Fatal(err)
	}
	if sys.Identity != "core-rtr" || sys.Board != "hAP ax^3" || sys.Serial != "HEX0A1B2C3D" || sys.Version != "7.16.1 (stable)" || sys.RAM != 1<<30 {
		t.Fatalf("system: %+v", sys)
	}
	ifaces, err := parseInterfaces([]byte(interfacesJSON), nil)
	if err != nil {
		t.Fatal(err)
	}
	addrs, err := parseAddresses([]byte(addressesJSON))
	if err != nil || len(addrs) != 3 {
		t.Fatalf("addresses (invalid dropped): %+v %v", addrs, err)
	}
	id := routerID(sys, ifaces)
	if id != "mikrotik-hex0a1b2c3d" {
		t.Fatal(id)
	}
	ops := graphOps(sys, id, routerIP("https://router.example.org", addrs))
	if len(ops.Ops) != 1 {
		t.Fatalf("ops: %+v", ops.Ops)
	}
	op := ops.Ops[0]
	if op.Kind != "router" || op.ID != "mikrotik-hex0a1b2c3d" || op.Name != "core-rtr" {
		t.Fatalf("router: %+v", op)
	}
	want := map[string]any{"model": "hAP ax^3", "product_code": "C53UiG+5HPaxD2HPaxD", "firmware": "RouterOS 7.16.1 (stable)", "serial": "HEX0A1B2C3D",
		"manufacturer": "MikroTik", "architecture": "arm64", "ram": 1.0, "ip": "192.168.88.1"}
	for k, v := range want {
		if op.Attrs[k] != v {
			t.Errorf("attr %s = %v, want %v", k, op.Attrs[k], v)
		}
	}
	// An IP address in the url wins over the router's own addresses.
	if ip := routerIP("https://10.0.0.1:8443", addrs); ip != "10.0.0.1" {
		t.Fatal(ip)
	}
}

func TestRouterIDFallbacks(t *testing.T) {
	// CHR: no routerboard, so no serial; the first ether port's MAC is used.
	sys, err := parseSystem([]byte(`{"name":"chr"}`), []byte(`{"board-name":"CHR","version":"7.15"}`), []byte(`{"routerboard":"false"}`))
	if err != nil || sys.Serial != "" || sys.Board != "CHR" {
		t.Fatalf("chr: %+v %v", sys, err)
	}
	ifaces, _ := parseInterfaces([]byte(`[{"name":"ether2","type":"ether","mac-address":"0C:00:00:00:00:02"},{"name":"ether1","type":"ether","mac-address":"0C:00:00:00:00:01"}]`), nil)
	if id := routerID(sys, ifaces); id != "mikrotik-0c-00-00-00-00-01" {
		t.Fatal(id)
	}
	if id := routerID(sys, nil); id != "mikrotik-chr" {
		t.Fatal(id)
	}
	if id := routerID(System{}, nil); id != "mikrotik-router" {
		t.Fatal(id)
	}
	// Missing routerboard answer (nil) is fine, a broken one too.
	if _, err := parseSystem([]byte(`{"name":"x"}`), []byte(`{}`), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSystem([]byte(`{"name":"x"}`), []byte(`{}`), []byte(`<html>`)); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSystem([]byte(`nope`), []byte(`{}`), nil); err == nil {
		t.Fatal("accepted a broken identity")
	}
	ops := graphOps(System{}, "mikrotik-router", "")
	if ops.Ops[0].Name != "MikroTik" || len(ops.Ops[0].Attrs) != 1 {
		t.Fatalf("empty: %+v", ops.Ops[0])
	}
}

func TestInterfacesAndLeases(t *testing.T) {
	addrs, _ := parseAddresses([]byte(addressesJSON))
	ifaces, err := parseInterfaces([]byte(interfacesJSON), addrs)
	if err != nil || len(ifaces) != 4 {
		t.Fatalf("ifaces: %+v %v", ifaces, err)
	}
	e1, br := ifaces[0], ifaces[2]
	if !e1.Running || e1.MAC != "48:a9:8a:01:02:03" || e1.Comment != "WAN" || e1.MTU != "1500" || len(e1.Addresses) != 1 || e1.Addresses[0] != "100.70.1.2/24" {
		t.Fatalf("ether1: %+v", e1)
	}
	if ifaces[1].Running || len(br.Addresses) != 1 || br.Addresses[0] != "192.168.88.1/24" {
		t.Fatalf("ether2/bridge (disabled address dropped): %+v %+v", ifaces[1], br)
	}
	leases, err := parseLeases([]byte(leasesJSON))
	if err != nil || len(leases) != 2 {
		t.Fatalf("leases: %+v %v", leases, err)
	}
	if l := leases[0]; l.Address != "192.168.88.254" || l.MAC != "aa:bb:cc:dd:ee:01" || l.Hostname != "nas" || l.Status != "bound" || !l.Dynamic || l.ExpiresAfter != "8m12s" {
		t.Fatalf("lease 0: %+v", l)
	}
	if l := leases[1]; l.Dynamic || l.Comment != "printer" || l.Hostname != "" {
		t.Fatalf("lease 1: %+v", l)
	}
	if _, err := parseLeases([]byte(`{"error":401,"message":"Unauthorized"}`)); err == nil {
		t.Fatal("accepted an error object as lease list")
	}
}

func TestHTTPError(t *testing.T) {
	if !strings.Contains(httpError(401).Error(), "username or password") || !strings.Contains(httpError(403).Error(), "rest-api") {
		t.Fatal("auth messages")
	}
	if httpError(500).Error() != "MikroTik answered HTTP 500" {
		t.Fatal(httpError(500))
	}
}
