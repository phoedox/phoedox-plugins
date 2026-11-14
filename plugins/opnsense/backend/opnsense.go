// Logic of the OPNsense plugin, without build tags so it is testable natively.
package main

import (
	"encoding/json"
	"net/netip"
	"strconv"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// firmwareVersion reads the version from /api/core/firmware/status (the
// answer differs between releases).
func firmwareVersion(body []byte) string {
	var r struct {
		ProductVersion string `json:"product_version"`
		Product        struct {
			ProductVersion string `json:"product_version"`
		} `json:"product"`
	}
	_ = json.Unmarshal(body, &r)
	if r.Product.ProductVersion != "" {
		return r.Product.ProductVersion
	}
	return r.ProductVersion
}

// Iface is a row of /api/interfaces/overview/interfacesInfo.
type Iface struct {
	Identifier  string `json:"identifier"`
	Description string `json:"description"`
	Device      string `json:"device"`
	Status      string `json:"status"`
	Addr4       string `json:"addr4"`
	VLANTag     any    `json:"vlan_tag"`
	Enabled     any    `json:"enabled"`
}

func (i Iface) vlan() int {
	switch v := i.VLANTag.(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

// subnet turns "192.168.1.1/24" into "192.168.1.0/24".
func subnet(cidr string) string {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return ""
	}
	return p.Masked().String()
}

func parseIfaces(body []byte) ([]Iface, error) {
	var r struct {
		Rows []Iface `json:"rows"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r.Rows, nil
}

// graphOps: the firewall as router, its interfaces part of it; the router
// gets the LAN address.
func graphOps(name, version string, ifaces []Iface) pdk.GraphOps {
	var ops pdk.GraphOps
	attrs := map[string]any{"model": "OPNsense", "firmware": version}
	for _, i := range ifaces {
		if i.Identifier == "lan" && i.Addr4 != "" {
			attrs["ip"] = integ.StripMask(i.Addr4)
		}
	}
	base := "opnsense-" + integ.Slugify(name)
	fw := ops.UpsertNamed("router", base, name, attrs)
	for _, i := range ifaces {
		if i.Identifier == "" || i.Identifier == "lo0" || strings.HasPrefix(i.Device, "lo") {
			continue
		}
		label := i.Description
		if label == "" {
			label = strings.ToUpper(i.Identifier)
		}
		a := map[string]any{"identifier": i.Identifier, "device": i.Device}
		if i.Status != "" {
			a["status"] = i.Status
		}
		if i.Addr4 != "" {
			a["ipv4"] = integ.StripMask(i.Addr4)
			if s := subnet(i.Addr4); s != "" {
				a["subnet"] = s
			}
		}
		if v := i.vlan(); v > 0 {
			a["vlan"] = v
		}
		ref := ops.UpsertNamed("opnsense_interface", base+"-"+integ.Slugify(i.Identifier), name+" "+label, a)
		ops.Link(ref, "part_of", fw, nil)
	}
	return ops
}

// Lease is a DHCPv4 lease.
type Lease struct {
	Address   string `json:"address"`
	MAC       string `json:"hwaddr"`
	Hostname  string `json:"hostname"`
	Interface string `json:"if_descr"`
	State     string `json:"state"`
	Type      string `json:"type"`
}

func parseLeases(body []byte) ([]Lease, error) {
	var r struct {
		Rows []Lease `json:"rows"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r.Rows, nil
}

// Gateway is a gateway status.
type Gateway struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Status  string `json:"status_translated"`
	Delay   string `json:"delay"`
	Loss    string `json:"loss"`
}

func parseGateways(body []byte) ([]Gateway, error) {
	var r struct {
		Items []Gateway `json:"items"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r.Items, nil
}
