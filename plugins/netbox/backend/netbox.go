// Logic of the NetBox plugin, without build tags so it is testable natively.
package main

import (
	"fmt"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// ref is a nested object in NetBox answers.
type ref struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Location is a NetBox location (room, floor area …).
type Location struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Site *ref   `json:"site"`
}

// Rack is a NetBox rack.
type Rack struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	UHeight    int    `json:"u_height"`
	OuterDepth *int   `json:"outer_depth"`
	OuterUnit  *struct {
		Value string `json:"value"`
	} `json:"outer_unit"`
	Location *ref `json:"location"`
	Site     *ref `json:"site"`
}

// DeviceType carries the height of a device in units.
type DeviceType struct {
	ID           int     `json:"id"`
	Model        string  `json:"model"`
	UHeight      float64 `json:"u_height"`
	FrontPorts   int     `json:"front_port_template_count"`
	Manufacturer *ref    `json:"manufacturer"`
}

// Device is a NetBox device (v4 "role", v3 "device_role").
type Device struct {
	ID         int         `json:"id"`
	Name       string      `json:"name"`
	Role       *ref        `json:"role"`
	DeviceRole *ref        `json:"device_role"`
	DeviceType *DeviceType `json:"device_type"`
	Serial     string      `json:"serial"`
	AssetTag   string      `json:"asset_tag"`
	PrimaryIP  *struct {
		Address string `json:"address"`
	} `json:"primary_ip"`
	Rack     *ref     `json:"rack"`
	Position *float64 `json:"position"`
	Location *ref     `json:"location"`
	Site     *ref     `json:"site"`
	Status   *struct {
		Value string `json:"value"`
	} `json:"status"`
}

func (d Device) role() string {
	if d.Role != nil {
		return d.Role.Slug
	}
	if d.DeviceRole != nil {
		return d.DeviceRole.Slug
	}
	return ""
}

// kindOf maps a device role to a Phoedox kind.
func kindOf(role string) string {
	r := strings.ToLower(role)
	switch {
	case strings.Contains(r, "router"), strings.Contains(r, "firewall"), strings.Contains(r, "gateway"):
		return "router"
	case strings.Contains(r, "switch"):
		return "switch"
	case strings.Contains(r, "access-point"), r == "ap", strings.Contains(r, "wireless"), strings.Contains(r, "wifi"):
		return "access_point"
	case strings.Contains(r, "patch"):
		return "patch_panel"
	case strings.Contains(r, "server"), strings.Contains(r, "hypervisor"), strings.Contains(r, "storage"), strings.Contains(r, "nas"):
		return "server"
	}
	return "device"
}

// mountable kinds may be mounted_in a rack (see the core data model).
var mountable = map[string]bool{"router": true, "switch": true, "server": true, "patch_panel": true, "device": true}

// Page is a NetBox list answer.
type Page[T any] struct {
	Count   int     `json:"count"`
	Next    *string `json:"next"`
	Results []T     `json:"results"`
}

// graphOps: locations become rooms, racks stand in them, devices are
// mounted in racks at their unit (or stand in their room).
func graphOps(locations []Location, racks []Rack, types map[int]DeviceType, devices []Device) pdk.GraphOps {
	var ops pdk.GraphOps
	rooms := map[int]string{}
	for _, l := range locations {
		rooms[l.ID] = ops.UpsertNamed("room", fmt.Sprintf("nb-loc-%d-%s", l.ID, integ.Slugify(l.Name)), l.Name, map[string]any{})
	}
	rackRefs := map[int]string{}
	for _, r := range racks {
		attrs := map[string]any{"height_units": r.UHeight}
		if r.OuterDepth != nil && r.OuterUnit != nil && r.OuterUnit.Value == "mm" {
			attrs["depth"] = *r.OuterDepth
		}
		rackRefs[r.ID] = ops.UpsertNamed("rack", fmt.Sprintf("nb-rack-%d-%s", r.ID, integ.Slugify(r.Name)), r.Name, attrs)
		if r.Location != nil && rooms[r.Location.ID] != "" {
			ops.Link(rackRefs[r.ID], "located_in", rooms[r.Location.ID], nil)
		}
	}
	for _, d := range devices {
		if d.Name == "" {
			continue // unnamed devices (e.g. blank panels) are not documented
		}
		kind := kindOf(d.role())
		attrs := map[string]any{}
		if kind == "patch_panel" {
			// Phoedox requires the number of ports of a patch panel; without
			// front ports on its device type it becomes a plain device.
			if n := frontPorts(d, types); n > 0 {
				attrs["ports"] = n
			} else {
				kind = "device"
			}
		}
		if d.Serial != "" && kind != "patch_panel" {
			attrs["serial"] = d.Serial
		}
		if d.PrimaryIP != nil && d.PrimaryIP.Address != "" && kind != "patch_panel" {
			attrs["ip"] = integ.StripMask(d.PrimaryIP.Address)
		}
		height := 1.0
		if d.DeviceType != nil {
			t, ok := types[d.DeviceType.ID]
			if !ok {
				t = *d.DeviceType
			}
			if t.UHeight > 0 {
				height = t.UHeight
			}
			if kind != "patch_panel" {
				attrs["model"] = t.Model
				if t.Manufacturer != nil && kind == "device" {
					attrs["manufacturer"] = t.Manufacturer.Name
				}
			}
		}
		if d.Status != nil && d.Status.Value != "" {
			attrs["status"] = d.Status.Value
		}
		ref := ops.UpsertNamed(kind, fmt.Sprintf("nb-dev-%d-%s", d.ID, integ.Slugify(d.Name)), d.Name, attrs)
		switch {
		case d.Rack != nil && rackRefs[d.Rack.ID] != "" && mountable[kind]:
			edge := map[string]any{"height": int(height + 0.5)}
			if d.Position != nil {
				edge["u"] = int(*d.Position)
			}
			ops.Link(ref, "mounted_in", rackRefs[d.Rack.ID], edge)
		case d.Location != nil && rooms[d.Location.ID] != "" && kind != "patch_panel":
			ops.Link(ref, "located_in", rooms[d.Location.ID], nil)
		}
	}
	return ops
}

// frontPorts is the number of front ports of a device's type.
func frontPorts(d Device, types map[int]DeviceType) int {
	if d.DeviceType == nil {
		return 0
	}
	if t, ok := types[d.DeviceType.ID]; ok {
		return t.FrontPorts
	}
	return d.DeviceType.FrontPorts
}

// Found is a search result of netbox.find.
type Found struct {
	Name     string `json:"name"`
	Role     string `json:"role,omitempty"`
	Model    string `json:"model,omitempty"`
	Serial   string `json:"serial,omitempty"`
	IP       string `json:"ip,omitempty"`
	Site     string `json:"site,omitempty"`
	Location string `json:"location,omitempty"`
	Rack     string `json:"rack,omitempty"`
	Position int    `json:"position,omitempty"`
	Status   string `json:"status,omitempty"`
}

func found(devices []Device) []Found {
	out := make([]Found, 0, len(devices))
	for _, d := range devices {
		f := Found{Name: d.Name, Role: d.role(), Serial: d.Serial}
		if d.DeviceType != nil {
			f.Model = d.DeviceType.Model
		}
		if d.PrimaryIP != nil {
			f.IP = d.PrimaryIP.Address
		}
		if d.Site != nil {
			f.Site = d.Site.Name
		}
		if d.Location != nil {
			f.Location = d.Location.Name
		}
		if d.Rack != nil {
			f.Rack = d.Rack.Name
		}
		if d.Position != nil {
			f.Position = int(*d.Position)
		}
		if d.Status != nil {
			f.Status = d.Status.Value
		}
		out = append(out, f)
	}
	return out
}

// Prefix is an IP prefix.
type Prefix struct {
	Prefix      string `json:"prefix"`
	Description string `json:"description"`
	VLAN        *struct {
		VID  int    `json:"vid"`
		Name string `json:"name"`
	} `json:"vlan"`
	Status *struct {
		Value string `json:"value"`
	} `json:"status"`
}

// PrefixInfo is a line of netbox.prefixes.
type PrefixInfo struct {
	Prefix      string `json:"prefix"`
	VLAN        string `json:"vlan,omitempty"`
	Status      string `json:"status,omitempty"`
	Description string `json:"description,omitempty"`
}

func prefixInfo(list []Prefix) []PrefixInfo {
	out := make([]PrefixInfo, 0, len(list))
	for _, p := range list {
		i := PrefixInfo{Prefix: p.Prefix, Description: p.Description}
		if p.VLAN != nil {
			i.VLAN = fmt.Sprintf("%d %s", p.VLAN.VID, p.VLAN.Name)
		}
		if p.Status != nil {
			i.Status = p.Status.Value
		}
		out = append(out, i)
	}
	return out
}
