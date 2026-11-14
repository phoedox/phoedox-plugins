// Logic of the ESPHome plugin, without build tags so it is testable natively.
//
// ESPHome devices themselves cannot be read cleanly over HTTP: the native
// API is protobuf over TCP, and the web_server component only has
// per-entity endpoints and an endless event stream. The ESPHome dashboard
// (Device Builder) knows every configured device and answers GET /devices
// and GET /ping with JSON (the endpoints Home Assistant's ESPHome
// integration uses), so the plugin reads the dashboard.
package main

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// Device is a configured device of GET /devices (legacy dashboard and
// Device Builder; the newer fields are empty with the legacy dashboard).
type Device struct {
	Name           string   `json:"name"`
	FriendlyName   string   `json:"friendly_name"`
	Configuration  string   `json:"configuration"`
	Comment        *string  `json:"comment"`
	Area           string   `json:"area"`
	BoardID        string   `json:"board_id"`
	Platform       string   `json:"target_platform"`
	Address        string   `json:"address"`
	IP             string   `json:"ip"`
	IPAddresses    []string `json:"ip_addresses"`
	MAC            string   `json:"mac_address"`
	WebPort        *int     `json:"web_port"`
	Deployed       string   `json:"deployed_version"`
	Current        string   `json:"current_version"`
	UpdateFlag     *bool    `json:"update_available"`
	State          string   `json:"state"` // online, offline, unknown (Device Builder)
	Integrations   []string `json:"loaded_integrations"`
	UsesMQTT       bool     `json:"uses_mqtt"`
	UsesDeepSleep  bool     `json:"uses_deep_sleep"`
	OfflineSeconds *float64 `json:"offline_seconds"`
}

// Importable is a discovered device that is not adopted yet.
type Importable struct {
	Name         string `json:"name"`
	FriendlyName string `json:"friendly_name"`
	ProjectName  string `json:"project_name"`
	ProjectVer   string `json:"project_version"`
	Network      string `json:"network"`
}

// Devices is GET /devices.
type Devices struct {
	Configured []Device     `json:"configured"`
	Importable []Importable `json:"importable"`
}

func parseDevices(body []byte) (Devices, error) {
	var d Devices
	if err := json.Unmarshal(body, &d); err != nil {
		return d, fmt.Errorf("unexpected device list from the ESPHome dashboard: %w", err)
	}
	if d.Configured == nil && d.Importable == nil {
		return d, fmt.Errorf("the address does not answer like the ESPHome dashboard (no device list at /devices)")
	}
	return d, nil
}

// parsePing reads GET /ping: {"kitchen.yaml": true, "garage.yaml": null}.
func parsePing(body []byte) map[string]*bool {
	m := map[string]*bool{}
	_ = json.Unmarshal(body, &m)
	return m
}

func (d Device) ip() string {
	for _, c := range append([]string{d.IP}, append(d.IPAddresses, d.Address)...) {
		if a, err := netip.ParseAddr(c); err == nil && !a.IsLoopback() {
			return a.String()
		}
	}
	return ""
}

func (d Device) label() string {
	if strings.TrimSpace(d.FriendlyName) != "" {
		return strings.TrimSpace(d.FriendlyName)
	}
	return d.Name
}

func (d Device) updateAvailable() bool {
	if d.UpdateFlag != nil {
		return *d.UpdateFlag
	}
	return d.Deployed != "" && d.Current != "" && d.Deployed != d.Current
}

// roomIndex maps lower-case names of existing rooms to their slugs, so an
// ESPHome area joins a room that already exists instead of adding one.
func roomIndex(rooms []pdk.Entity) map[string]string {
	out := map[string]string{}
	for _, r := range rooms {
		k := normName(r.Name)
		if _, ok := out[k]; !ok && k != "" && r.Slug != "" {
			out[k] = r.Slug
		}
	}
	return out
}

func normName(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

// graphOps imports configured devices as device ("esphome-<name>"), "located
// in" the room of their area (an existing room with that name, else a new
// "esphome-room-<area>").
func graphOps(devs []Device, existing map[string]string) pdk.GraphOps {
	var ops pdk.GraphOps
	rooms := map[string]string{}
	for _, d := range devs {
		if d.Name == "" {
			continue
		}
		attrs := map[string]any{"esphome_config": d.Configuration}
		set := func(k, v string) {
			if v != "" {
				attrs[k] = v
			}
		}
		model := d.BoardID
		if model == "" {
			model = d.Platform
		}
		set("model", model)
		set("platform", d.Platform)
		set("ip", d.ip())
		set("mac", fmtMAC(d.MAC))
		set("hostname", d.Address)
		set("firmware", d.Deployed)
		if d.Comment != nil {
			set("comment", strings.TrimSpace(*d.Comment))
		}
		ref := ops.UpsertNamed("device", "esphome-"+integ.Slugify(d.Name), d.label(), attrs)
		area := strings.TrimSpace(d.Area)
		if area == "" {
			continue
		}
		room, ok := rooms[normName(area)]
		if !ok {
			if room, ok = existing[normName(area)]; !ok {
				room = ops.UpsertNamed("room", "esphome-room-"+integ.Slugify(area), area, map[string]any{})
			}
			rooms[normName(area)] = room
		}
		ops.Link(ref, "located_in", room, nil)
	}
	return ops
}

// Row is what esphome.devices returns per device.
type Row struct {
	Name            string   `json:"name"`
	FriendlyName    string   `json:"friendly_name,omitempty"`
	Online          *bool    `json:"online,omitempty"`
	Address         string   `json:"address,omitempty"`
	IP              string   `json:"ip,omitempty"`
	Platform        string   `json:"platform,omitempty"`
	Board           string   `json:"board,omitempty"`
	Area            string   `json:"area,omitempty"`
	Deployed        string   `json:"deployed_version,omitempty"`
	Current         string   `json:"esphome_version,omitempty"`
	UpdateAvailable bool     `json:"update_available"`
	WebServer       bool     `json:"web_server"`
	MQTT            bool     `json:"mqtt,omitempty"`
	DeepSleep       bool     `json:"deep_sleep,omitempty"`
	Integrations    []string `json:"integrations,omitempty"`
}

// Overview is what esphome.devices returns.
type Overview struct {
	Devices    []Row        `json:"devices"`
	Discovered []Importable `json:"discovered_not_adopted,omitempty"`
}

func overview(d Devices, ping map[string]*bool) Overview {
	out := Overview{Devices: []Row{}, Discovered: d.Importable}
	for _, c := range d.Configured {
		r := Row{Name: c.Name, FriendlyName: c.FriendlyName, Address: c.Address, IP: c.ip(), Platform: c.Platform, Board: c.BoardID, Area: c.Area,
			Deployed: c.Deployed, Current: c.Current, UpdateAvailable: c.updateAvailable(), MQTT: c.UsesMQTT, DeepSleep: c.UsesDeepSleep,
			WebServer: slices.Contains(c.Integrations, "web_server"), Integrations: c.Integrations}
		switch c.State {
		case "online":
			r.Online = ptr(true)
		case "offline":
			r.Online = ptr(false)
		default:
			r.Online = ping[c.Configuration]
		}
		out.Devices = append(out.Devices, r)
	}
	return out
}

func ptr[T any](v T) *T { return &v }

func macID(mac string) string {
	return strings.ToLower(strings.NewReplacer(":", "", "-", "").Replace(strings.TrimSpace(mac)))
}

func fmtMAC(mac string) string {
	m := strings.ToUpper(macID(mac))
	if len(m) != 12 {
		return strings.ToUpper(strings.TrimSpace(mac))
	}
	parts := make([]string, 6)
	for i := range parts {
		parts[i] = m[2*i : 2*i+2]
	}
	return strings.Join(parts, ":")
}
