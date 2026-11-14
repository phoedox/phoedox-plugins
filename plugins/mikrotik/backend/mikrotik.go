// Logic of the MikroTik plugin, without build tags so it is testable natively.
// The RouterOS REST API answers with JSON objects whose values are strings
// ("true", "1073741824"), so parsing goes through tolerant helpers.
package main

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// httpError turns a non-200 answer into a clear message.
func httpError(status int) error {
	switch status {
	case 401:
		return fmt.Errorf("the MikroTik router rejected the username or password")
	case 403:
		return fmt.Errorf("the RouterOS user may not use the REST API (its group needs the read and rest-api policies)")
	case 404:
		return fmt.Errorf("the MikroTik router has no REST API at this address (RouterOS v7 with the www-ssl service is needed)")
	}
	return &integ.StatusError{Service: "MikroTik", Status: status}
}

type obj map[string]any

func (o obj) str(key string) string {
	switch v := o[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

func (o obj) flag(key string) bool {
	switch v := o[key].(type) {
	case bool:
		return v
	case string:
		return integ.Truthy(v)
	}
	return false
}

func (o obj) num(key string) float64 {
	f, _ := strconv.ParseFloat(o.str(key), 64)
	return f
}

func parseObj(body []byte) (obj, error) {
	var o obj
	if err := json.Unmarshal(body, &o); err != nil {
		return nil, fmt.Errorf("unexpected answer from MikroTik: %w", err)
	}
	return o, nil
}

func parseList(body []byte) ([]obj, error) {
	var l []obj
	if err := json.Unmarshal(body, &l); err != nil {
		return nil, fmt.Errorf("unexpected answer from MikroTik: %w", err)
	}
	return l, nil
}

// System is what /system/identity, /system/resource and /system/routerboard
// tell about the router.
type System struct {
	Identity string
	Board    string // marketing name, e.g. "hAP ax^3"
	Model    string // product code from the routerboard, e.g. "C53UiG+5HPaxD2HPaxD"
	Serial   string
	Version  string // e.g. "7.16.1 (stable)"
	Arch     string
	RAM      float64 // bytes
}

// parseSystem reads the three answers; routerboard may be nil (CHR, x86).
func parseSystem(identity, resource, routerboard []byte) (System, error) {
	id, err := parseObj(identity)
	if err != nil {
		return System{}, err
	}
	res, err := parseObj(resource)
	if err != nil {
		return System{}, err
	}
	s := System{Identity: id.str("name"), Board: res.str("board-name"), Version: res.str("version"), Arch: res.str("architecture-name"), RAM: res.num("total-memory")}
	if len(routerboard) > 0 {
		if rb, err := parseObj(routerboard); err == nil && rb.flag("routerboard") {
			s.Model, s.Serial = rb.str("model"), rb.str("serial-number")
			if s.Board == "" {
				s.Board = rb.str("board-name")
			}
		}
	}
	return s, nil
}

// routerID is a stable slug: the serial number, else the MAC address of the
// first Ethernet port, else the identity name.
func routerID(s System, ifaces []Iface) string {
	if s.Serial != "" {
		return "mikrotik-" + integ.Slugify(s.Serial)
	}
	var macs []Iface
	for _, i := range ifaces {
		if i.Type == "ether" && i.MAC != "" && i.MAC != "00:00:00:00:00:00" {
			macs = append(macs, i)
		}
	}
	if len(macs) > 0 {
		sort.Slice(macs, func(a, b int) bool { return macs[a].Name < macs[b].Name })
		return "mikrotik-" + integ.Slugify(macs[0].MAC)
	}
	name := s.Identity
	if name == "" {
		name = "router"
	}
	return "mikrotik-" + integ.Slugify(name)
}

// routerIP is the address Phoedox talks to, if it is an IP address, else the
// first static private IPv4 address of the router.
func routerIP(base string, addrs []Address) string {
	if u, err := url.Parse(base); err == nil {
		if ip, err := netip.ParseAddr(u.Hostname()); err == nil {
			return ip.String()
		}
	}
	for _, a := range addrs {
		ip, err := netip.ParseAddr(integ.StripMask(a.Address))
		if err == nil && ip.Is4() && ip.IsPrivate() && !a.Dynamic && !a.Disabled {
			return ip.String()
		}
	}
	return ""
}

// graphOps: the router as kind router.
func graphOps(s System, id, ip string) pdk.GraphOps {
	var ops pdk.GraphOps
	attrs := map[string]any{"manufacturer": "MikroTik"}
	switch {
	case s.Board != "":
		attrs["model"] = s.Board
	case s.Model != "":
		attrs["model"] = s.Model
	}
	if s.Model != "" && s.Model != attrs["model"] {
		attrs["product_code"] = s.Model
	}
	if s.Version != "" {
		attrs["firmware"] = "RouterOS " + s.Version
	}
	if s.Serial != "" {
		attrs["serial"] = s.Serial
	}
	if s.Arch != "" {
		attrs["architecture"] = s.Arch
	}
	if s.RAM > 0 {
		attrs["ram"] = integ.GiB(s.RAM)
	}
	if ip != "" {
		attrs["ip"] = ip
	}
	name := s.Identity
	if name == "" {
		name = "MikroTik"
	}
	ops.UpsertNamed("router", id, name, attrs)
	return ops
}

// Address is a row of /ip/address.
type Address struct {
	Address   string
	Interface string
	Dynamic   bool
	Disabled  bool
}

func parseAddresses(body []byte) ([]Address, error) {
	l, err := parseList(body)
	if err != nil {
		return nil, err
	}
	out := make([]Address, 0, len(l))
	for _, o := range l {
		if o.flag("invalid") {
			continue
		}
		out = append(out, Address{Address: o.str("address"), Interface: o.str("interface"), Dynamic: o.flag("dynamic"), Disabled: o.flag("disabled")})
	}
	return out, nil
}

// Iface is an interface with its addresses.
type Iface struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Running   bool     `json:"running"`
	Disabled  bool     `json:"disabled"`
	MAC       string   `json:"mac,omitempty"`
	MTU       string   `json:"mtu,omitempty"`
	Comment   string   `json:"comment,omitempty"`
	Addresses []string `json:"addresses,omitempty"`
}

// parseInterfaces reads /interface and attaches the enabled addresses.
func parseInterfaces(body []byte, addrs []Address) ([]Iface, error) {
	l, err := parseList(body)
	if err != nil {
		return nil, err
	}
	byIface := map[string][]string{}
	for _, a := range addrs {
		if !a.Disabled && a.Address != "" {
			byIface[a.Interface] = append(byIface[a.Interface], a.Address)
		}
	}
	out := make([]Iface, 0, len(l))
	for _, o := range l {
		mtu := o.str("actual-mtu")
		if mtu == "" {
			mtu = o.str("mtu")
		}
		name := o.str("name")
		out = append(out, Iface{Name: name, Type: o.str("type"), Running: o.flag("running"), Disabled: o.flag("disabled"),
			MAC: strings.ToLower(o.str("mac-address")), MTU: mtu, Comment: o.str("comment"), Addresses: byIface[name]})
	}
	return out, nil
}

// Lease is a DHCP lease of /ip/dhcp-server/lease.
type Lease struct {
	Address      string `json:"address"`
	MAC          string `json:"mac"`
	Hostname     string `json:"hostname,omitempty"`
	Status       string `json:"status,omitempty"`
	Server       string `json:"server,omitempty"`
	Dynamic      bool   `json:"dynamic"`
	Disabled     bool   `json:"disabled,omitempty"`
	ExpiresAfter string `json:"expires_after,omitempty"`
	LastSeen     string `json:"last_seen,omitempty"`
	Comment      string `json:"comment,omitempty"`
}

func parseLeases(body []byte) ([]Lease, error) {
	l, err := parseList(body)
	if err != nil {
		return nil, err
	}
	out := make([]Lease, 0, len(l))
	for _, o := range l {
		addr := o.str("active-address")
		if addr == "" {
			addr = o.str("address")
		}
		mac := o.str("active-mac-address")
		if mac == "" {
			mac = o.str("mac-address")
		}
		out = append(out, Lease{Address: addr, MAC: strings.ToLower(mac), Hostname: o.str("host-name"), Status: o.str("status"), Server: o.str("server"),
			Dynamic: o.flag("dynamic"), Disabled: o.flag("disabled"), ExpiresAfter: o.str("expires-after"), LastSeen: o.str("last-seen"), Comment: o.str("comment")})
	}
	return out, nil
}
