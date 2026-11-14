// Logic of the Tasmota plugin, without build tags so it is testable natively.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// maxDevices is the number of address settings. net:fetch:config only
// allows hosts of settings named "url" or "*_url", so every device needs a
// setting of its own: url, device2_url … device10_url.
const maxDevices = 10

// slots are the names of the address settings, in order.
func slots() []string {
	out := []string{"url"}
	for i := 2; i <= maxDevices; i++ {
		out = append(out, "device"+strconv.Itoa(i)+"_url")
	}
	return out
}

// commandURL builds /cm?cmnd=…. Tasmota only takes the web password as URL
// parameters; they are added only when a password is set.
func commandURL(base, cmnd, user, pass string) string {
	q := "cmnd=" + escape(cmnd)
	if pass != "" {
		q += "&user=" + escape(user) + "&password=" + escape(pass)
	}
	return base + "/cm?" + q
}

func escape(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }

// redact removes the credentials from a message (the host's fetch errors
// may contain the request URL).
func redact(msg string, secrets ...string) string {
	for _, s := range secrets {
		if s == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, escape(s), "***")
		msg = strings.ReplaceAll(msg, url.QueryEscape(s), "***")
		msg = strings.ReplaceAll(msg, s, "***")
	}
	return msg
}

// Status0 is the part of "Status 0" Phoedox uses.
type Status0 struct {
	Status struct {
		DeviceName   string   `json:"DeviceName"`
		FriendlyName []string `json:"FriendlyName"`
		Topic        string   `json:"Topic"`
		Module       int      `json:"Module"`
	} `json:"Status"`
	FWR struct {
		Version  string `json:"Version"`
		Hardware string `json:"Hardware"`
	} `json:"StatusFWR"`
	NET struct {
		Hostname  string `json:"Hostname"`
		IPAddress string `json:"IPAddress"`
		Mac       string `json:"Mac"`
		Ethernet  *struct {
			IPAddress string `json:"IPAddress"`
			Mac       string `json:"Mac"`
		} `json:"Ethernet"`
	} `json:"StatusNET"`
	SNS map[string]json.RawMessage `json:"StatusSNS"`
	STS map[string]json.RawMessage `json:"StatusSTS"`
}

// errAuth is the answer of a device with a web password when none (or a
// wrong one) was given.
var errAuth = errors.New("the Tasmota device asks for its web password: set the settings password (and username if not admin)")

func parseStatus0(body []byte) (Status0, error) {
	var w struct {
		Warning string `json:"WARNING"`
	}
	if json.Unmarshal(body, &w) == nil && strings.Contains(strings.ToLower(w.Warning), "password") {
		return Status0{}, errAuth
	}
	var s Status0
	if err := json.Unmarshal(body, &s); err != nil {
		return s, fmt.Errorf("unexpected answer from Tasmota: %w", err)
	}
	if s.NET.Mac == "" && s.FWR.Version == "" {
		return s, errors.New("the address does not answer like a Tasmota device")
	}
	return s, nil
}

var moduleRe = regexp.MustCompile(`^\d+\s*\((.*)\)$`)

// parseModule reads the answer of "Module": {"Module":{"0":"Athom Plug V2"}}
// (current) or {"Module":"1 (Sonoff Basic)"} (older releases).
func parseModule(body []byte) string {
	var r struct {
		Module json.RawMessage `json:"Module"`
	}
	if json.Unmarshal(body, &r) != nil {
		return ""
	}
	var m map[string]string
	if json.Unmarshal(r.Module, &m) == nil {
		for _, v := range m {
			return v
		}
	}
	var s string
	if json.Unmarshal(r.Module, &s) == nil {
		if x := moduleRe.FindStringSubmatch(s); x != nil {
			return x[1]
		}
		return s
	}
	return ""
}

// Energy is the energy meter of a device (only devices with metering).
type Energy struct {
	PowerW   any      `json:"power_w"` // number, or a list for several channels
	TodayKWh *float64 `json:"today_kwh,omitempty"`
	YestKWh  *float64 `json:"yesterday_kwh,omitempty"`
	TotalKWh *float64 `json:"total_kwh,omitempty"`
	VoltageV any      `json:"voltage_v,omitempty"`
	CurrentA any      `json:"current_a,omitempty"`
}

func (s Status0) energy() *Energy {
	raw, ok := s.SNS["ENERGY"]
	if !ok {
		return nil
	}
	var e struct {
		Power     any      `json:"Power"`
		Today     *float64 `json:"Today"`
		Yesterday *float64 `json:"Yesterday"`
		Total     *float64 `json:"Total"`
		Voltage   any      `json:"Voltage"`
		Current   any      `json:"Current"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return nil
	}
	return &Energy{PowerW: e.Power, TodayKWh: e.Today, YestKWh: e.Yesterday, TotalKWh: e.Total, VoltageV: e.Voltage, CurrentA: e.Current}
}

// power returns the relay states: {"POWER": "ON"} or {"POWER1": …, "POWER2": …}.
func (s Status0) power() map[string]string {
	out := map[string]string{}
	for k, v := range s.STS {
		if k != "POWER" && !(strings.HasPrefix(k, "POWER") && isDigits(k[5:])) {
			continue
		}
		var st string
		if json.Unmarshal(v, &st) == nil {
			out[k] = st
		}
	}
	return out
}

type wifi struct {
	SSID   string `json:"SSId"`
	RSSI   *int   `json:"RSSI"`   // quality in percent
	Signal *int   `json:"Signal"` // dBm
}

func (s Status0) wifi() wifi {
	var w wifi
	_ = json.Unmarshal(s.STS["Wifi"], &w)
	return w
}

func (s Status0) uptime() string {
	var u string
	_ = json.Unmarshal(s.STS["Uptime"], &u)
	return u
}

func (s Status0) name() string {
	for _, n := range append([]string{s.Status.DeviceName}, s.Status.FriendlyName...) {
		if n = strings.TrimSpace(n); n != "" && n != "Tasmota" {
			return n
		}
	}
	if s.NET.Hostname != "" {
		return s.NET.Hostname
	}
	return "Tasmota " + strings.ToUpper(macID(s.mac()))
}

func (s Status0) mac() string {
	if s.NET.Mac == "" && s.NET.Ethernet != nil {
		return s.NET.Ethernet.Mac
	}
	return s.NET.Mac
}

func (s Status0) ip() string {
	ip := s.NET.IPAddress
	if (ip == "" || ip == "0.0.0.0") && s.NET.Ethernet != nil {
		ip = s.NET.Ethernet.IPAddress
	}
	if a, err := netip.ParseAddr(ip); err == nil && !a.IsUnspecified() {
		return ip
	}
	return ""
}

// Device is everything read from one Tasmota device.
type Device struct {
	Setting string
	Status  Status0
	Module  string
}

// graphOps imports a device as a smart plug (relay with an energy meter) or
// a device. Slugs are "tasmota-<mac>".
func graphOps(d Device) pdk.GraphOps {
	var ops pdk.GraphOps
	s := d.Status
	kind := "device"
	if s.energy() != nil && len(s.power()) > 0 {
		kind = "smart_plug"
	}
	attrs := map[string]any{"mac": fmtMAC(s.mac())}
	set := func(k, v string) {
		if v != "" {
			attrs[k] = v
		}
	}
	set("model", d.Module)
	set("module", d.Module)
	set("ip", s.ip())
	set("hostname", s.NET.Hostname)
	set("firmware", s.FWR.Version)
	set("hardware", s.FWR.Hardware)
	set("mqtt_topic", s.Status.Topic)
	id := macID(s.mac())
	if id == "" {
		id = integ.Slugify(s.NET.Hostname + " " + s.name())
	}
	ops.UpsertNamed(kind, "tasmota-"+id, s.name(), attrs)
	return ops
}

// Row is what tasmota.status returns per device.
type Row struct {
	Setting  string            `json:"setting"`
	Name     string            `json:"name,omitempty"`
	Module   string            `json:"module,omitempty"`
	IP       string            `json:"ip,omitempty"`
	Firmware string            `json:"firmware,omitempty"`
	Power    map[string]string `json:"power,omitempty"`
	Energy   *Energy           `json:"energy,omitempty"`
	SSID     string            `json:"wifi_ssid,omitempty"`
	Quality  *int              `json:"wifi_quality_pct,omitempty"`
	Signal   *int              `json:"wifi_signal_dbm,omitempty"`
	Uptime   string            `json:"uptime,omitempty"`
	Error    string            `json:"error,omitempty"`
}

func row(d Device) Row {
	s := d.Status
	w := s.wifi()
	return Row{Setting: d.Setting, Name: s.name(), Module: d.Module, IP: s.ip(), Firmware: s.FWR.Version, Power: s.power(),
		Energy: s.energy(), SSID: w.SSID, Quality: w.RSSI, Signal: w.Signal, Uptime: s.uptime()}
}

// deviceBase checks a configured device address.
func deviceBase(setting, raw string) (string, error) {
	base, err := integ.BaseURL(raw)
	if err != nil {
		return "", fmt.Errorf("the setting %s must be an address like http://192.168.1.60", setting)
	}
	return base, nil
}

func isDigits(s string) bool {
	return s != "" && !slices.ContainsFunc([]rune(s), func(r rune) bool { return r < '0' || r > '9' })
}

// macID turns "84:F3:EB:AB:C1:23" into "84f3ebabc123".
func macID(mac string) string {
	return strings.ToLower(strings.NewReplacer(":", "", "-", "").Replace(strings.TrimSpace(mac)))
}

func fmtMAC(mac string) string {
	m := strings.ToUpper(macID(mac))
	if len(m) != 12 {
		return strings.ToUpper(mac)
	}
	parts := make([]string, 6)
	for i := range parts {
		parts[i] = m[2*i : 2*i+2]
	}
	return strings.Join(parts, ":")
}
