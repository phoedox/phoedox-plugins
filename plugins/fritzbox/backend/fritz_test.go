package main

import (
	"strings"
	"testing"
)

func TestDigest(t *testing.T) {
	// RFC 2617, section 3.5.
	d, err := parseChallenge(`Digest realm="testrealm@host.com", qop="auth,auth-int", nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093", opaque="5ccc069c403ebaf9f0171e9517f40e41"`)
	if err != nil {
		t.Fatal(err)
	}
	d.Qop = "auth"
	h := d.Authorization("Mufasa", "Circle Of Life", "GET", "/dir/index.html", "0a4f113b")
	if !strings.Contains(h, `response="6629fae49393a05397450978507c4ef1"`) || !strings.Contains(h, "nc=00000001") {
		t.Fatalf("digest: %s", h)
	}
	if _, err := parseChallenge(`Basic realm="x"`); err == nil {
		t.Fatal("basic accepted")
	}
}

func TestSOAPAndHosts(t *testing.T) {
	if !strings.Contains(envelope("urn:dslforum-org:service:DeviceInfo:1", "GetInfo"), `<u:GetInfo xmlns:u="urn:dslforum-org:service:DeviceInfo:1"/>`) {
		t.Fatal("envelope")
	}
	v, err := soapValues(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>
		<u:GetInfoResponse xmlns:u="urn:dslforum-org:service:DeviceInfo:1"><NewModelName>FRITZ!Box 7590</NewModelName>
		<NewSoftwareVersion>154.07.57</NewSoftwareVersion><NewSerialNumber>ABC123</NewSerialNumber></u:GetInfoResponse></s:Body></s:Envelope>`)
	if err != nil || v["NewModelName"] != "FRITZ!Box 7590" || v["NewSoftwareVersion"] != "154.07.57" {
		t.Fatalf("values: %v %v", v, err)
	}
	if _, err := soapValues(`<s:Envelope><s:Body><s:Fault><detail><UPnPError><errorCode>401</errorCode><errorDescription>Invalid Action</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`); err == nil || !strings.Contains(err.Error(), "Invalid Action") {
		t.Fatalf("fault: %v", err)
	}
	hosts, err := parseHosts(`<List><Item><IPAddress>192.168.178.20</IPAddress><MACAddress>AA:BB:CC:DD:EE:01</MACAddress><HostName>nas</HostName><Active>1</Active><InterfaceType>Ethernet</InterfaceType></Item>
		<Item><IPAddress>192.168.178.30</IPAddress><MACAddress>AA:BB:CC:DD:EE:02</MACAddress><HostName>phone</HostName><Active>1</Active><InterfaceType>802.11</InterfaceType></Item>
		<Item><MACAddress>AA:BB:CC:DD:EE:03</MACAddress><HostName>old</HostName><Active>0</Active><InterfaceType>Ethernet</InterfaceType></Item></List>`)
	if err != nil || len(hosts) != 3 || hosts[0].Interface != "lan" || hosts[1].Interface != "wifi" || hosts[2].Online {
		t.Fatalf("hosts: %+v %v", hosts, err)
	}
	ops := graphOps(Info{Model: "FRITZ!Box 7590", Firmware: "154.07.57"}, hosts)
	if ops.Ops[0].ID != "fritzbox-fritz-box-7590" || ops.Ops[0].Kind != "router" {
		t.Fatalf("box: %+v", ops.Ops[0])
	}
	if len(ops.Ops) != 5 || ops.Ops[1].ID != "fritz-aabbccddee01" || ops.Ops[2].Rel != "wired_to" || ops.Ops[4].Rel != "uplink" {
		t.Fatalf("hosts ops: %+v", ops.Ops)
	}
	if only := graphOps(Info{Model: "FRITZ!Box 6660 Cable"}, nil); len(only.Ops) != 1 {
		t.Fatal("hosts imported without asking")
	}
}
