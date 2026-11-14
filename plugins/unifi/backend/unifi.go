// Logic of the UniFi plugin, without build tags so it is testable natively.
// It uses the official UniFi Network integration API (v1).
package main

import (
	"slices"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// Site is a UniFi site.
type Site struct {
	ID                string `json:"id"`
	InternalReference string `json:"internalReference"`
	Name              string `json:"name"`
}

// Device is a UniFi device (list entry merged with its details).
type Device struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Model           string   `json:"model"`
	MacAddress      string   `json:"macAddress"`
	IPAddress       string   `json:"ipAddress"`
	State           string   `json:"state"`
	Features        []string `json:"features"`
	FirmwareVersion string   `json:"firmwareVersion"`
	Uplink          *struct {
		DeviceID string `json:"deviceId"`
	} `json:"uplink"`
}

// Client is a connected client.
type Client struct {
	Type           string `json:"type"` // WIRED | WIRELESS | VPN | …
	Name           string `json:"name"`
	IPAddress      string `json:"ipAddress"`
	MacAddress     string `json:"macAddress"`
	ConnectedAt    string `json:"connectedAt"`
	UplinkDeviceID string `json:"uplinkDeviceId"`
}

// page is the envelope of list answers.
type page[T any] struct {
	Data       []T `json:"data"`
	TotalCount int `json:"totalCount"`
}

// pickSite chooses the configured site (by name or reference) or the first.
func pickSite(sites []Site, want string) (Site, bool) {
	want = strings.TrimSpace(want)
	for _, s := range sites {
		if want == "" || strings.EqualFold(s.Name, want) || strings.EqualFold(s.InternalReference, want) || s.ID == want {
			return s, true
		}
	}
	return Site{}, false
}

// gateways are the consoles and gateways among UniFi models.
var gatewayPrefixes = []string{"UDM", "UDR", "UDW", "UXG", "UCG", "USG", "UX", "EFG"}

// kindOf maps a device to a Phoedox kind.
func kindOf(d Device) string {
	m := strings.ToUpper(d.Model)
	for _, p := range gatewayPrefixes {
		if strings.HasPrefix(m, p) {
			return "router"
		}
	}
	if slices.Contains(d.Features, "switching") {
		return "switch"
	}
	if slices.Contains(d.Features, "accessPoint") {
		return "access_point"
	}
	return "switch"
}

func deviceName(d Device) string {
	if d.Name != "" {
		return d.Name
	}
	if d.Model != "" {
		return d.Model + " " + d.MacAddress
	}
	return d.MacAddress
}

// graphOps: devices with their uplinks ("wired to"); gateways have none.
func graphOps(devices []Device) pdk.GraphOps {
	var ops pdk.GraphOps
	refs := map[string]string{}
	kinds := map[string]string{}
	for _, d := range devices {
		kind := kindOf(d)
		attrs := map[string]any{"model": d.Model, "mac": strings.ToLower(d.MacAddress), "state": strings.ToLower(d.State)}
		if d.IPAddress != "" {
			attrs["ip"] = d.IPAddress
		}
		if d.FirmwareVersion != "" {
			attrs["firmware"] = d.FirmwareVersion
		}
		if kind == "switch" {
			attrs["managed"] = true
		}
		id := "unifi-" + integ.Slugify(strings.ReplaceAll(d.MacAddress, ":", ""))
		refs[d.ID] = ops.UpsertNamed(kind, id, deviceName(d), attrs)
		kinds[d.ID] = kind
	}
	for _, d := range devices {
		if d.Uplink == nil || d.Uplink.DeviceID == "" || kinds[d.ID] == "router" {
			continue
		}
		to, ok := refs[d.Uplink.DeviceID]
		if !ok || (kinds[d.Uplink.DeviceID] != "switch" && kinds[d.Uplink.DeviceID] != "router") {
			continue // wireless mesh uplinks (AP to AP) are not cables
		}
		ops.Link(refs[d.ID], "wired_to", to, nil)
	}
	return ops
}

// ClientInfo is a line of unifi.clients.
type ClientInfo struct {
	Name        string `json:"name"`
	IP          string `json:"ip,omitempty"`
	MAC         string `json:"mac"`
	Type        string `json:"type"`
	Via         string `json:"via,omitempty"`
	ConnectedAt string `json:"connectedAt,omitempty"`
}

func clientInfo(clients []Client, devices []Device) []ClientInfo {
	names := map[string]string{}
	for _, d := range devices {
		names[d.ID] = deviceName(d)
	}
	out := make([]ClientInfo, 0, len(clients))
	for _, c := range clients {
		name := c.Name
		if name == "" {
			name = c.MacAddress
		}
		out = append(out, ClientInfo{Name: name, IP: c.IPAddress, MAC: strings.ToLower(c.MacAddress), Type: strings.ToLower(c.Type), Via: names[c.UplinkDeviceID], ConnectedAt: c.ConnectedAt})
	}
	return out
}
