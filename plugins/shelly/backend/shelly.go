// Logic of the Shelly plugin, without build tags so it is testable natively.
package main

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"net/netip"
	"net/url"
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

// Info is the answer of GET /shelly (Gen1 and Gen2+; no login needed).
type Info struct {
	// Gen2+
	ID    string  `json:"id"`
	Name  *string `json:"name"`
	Model string  `json:"model"`
	Gen   int     `json:"gen"`
	Ver   string  `json:"ver"`
	App   string  `json:"app"`
	// Gen1
	Type string `json:"type"`
	FW   string `json:"fw"`
	// both
	MAC string `json:"mac"`
}

func parseInfo(body []byte) (Info, error) {
	var i Info
	if err := json.Unmarshal(body, &i); err != nil || i.MAC == "" {
		return i, errors.New("the address does not answer like a Shelly device")
	}
	if i.Gen == 0 {
		i.Gen = 1
	}
	return i, nil
}

func (i Info) model() string {
	if i.Model != "" {
		return i.Model
	}
	return i.Type
}

func (i Info) firmware() string {
	if i.Ver != "" {
		return i.Ver
	}
	return i.FW
}

// Channel is one output or meter of a device.
type Channel struct {
	Type      string   `json:"type"` // switch, pm1, cover, light, em, em1 (Gen2+); relay, emeter (Gen1)
	ID        int      `json:"id"`
	On        *bool    `json:"on,omitempty"`
	PowerW    *float64 `json:"power_w,omitempty"`
	EnergyKWh *float64 `json:"energy_kwh,omitempty"`
	VoltageV  *float64 `json:"voltage_v,omitempty"`
	TempC     *float64 `json:"temperature_c,omitempty"`
}

// Status is the part of the device status Phoedox uses.
type Status struct {
	Channels []Channel
	IP       string
	SSID     string
	RSSI     *int
	UptimeS  int
	Update   string   // available firmware version
	TempC    *float64 // device temperature (Gen1)
}

// metered reports whether the device switches a load and measures its power
// (then it is imported as a smart plug).
func (s Status) metered() bool {
	for _, c := range s.Channels {
		if (c.Type == "switch" || c.Type == "relay") && c.PowerW != nil {
			return true
		}
	}
	return false
}

type gen2Comp struct {
	ID            int      `json:"id"`
	Output        *bool    `json:"output"`
	APower        *float64 `json:"apower"`
	ActPower      *float64 `json:"act_power"`
	TotalActPower *float64 `json:"total_act_power"`
	Voltage       *float64 `json:"voltage"`
	AVoltage      *float64 `json:"a_voltage"`
	AEnergy       *struct {
		Total float64 `json:"total"` // Wh
	} `json:"aenergy"`
	Temperature *struct {
		TC *float64 `json:"tC"`
	} `json:"temperature"`
}

var gen2Types = []string{"switch", "pm1", "cover", "light", "em1", "em"}

// parseGen2Status reads /rpc/Shelly.GetStatus.
func parseGen2Status(body []byte) (Status, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return Status{}, fmt.Errorf("unexpected status from Shelly: %w", err)
	}
	var s Status
	for key, v := range raw {
		typ, _, ok := strings.Cut(key, ":")
		if !ok || !slices.Contains(gen2Types, typ) {
			continue
		}
		var c gen2Comp
		if json.Unmarshal(v, &c) != nil {
			continue
		}
		ch := Channel{Type: typ, ID: c.ID, On: c.Output, PowerW: firstOf(c.APower, c.ActPower, c.TotalActPower), VoltageV: firstOf(c.Voltage, c.AVoltage)}
		if c.AEnergy != nil {
			ch.EnergyKWh = ptr(round3(c.AEnergy.Total / 1000))
		}
		if c.Temperature != nil {
			ch.TempC = c.Temperature.TC
		}
		s.Channels = append(s.Channels, ch)
	}
	sortChannels(s.Channels)
	var r struct {
		Wifi struct {
			StaIP string `json:"sta_ip"`
			SSID  string `json:"ssid"`
			RSSI  *int   `json:"rssi"`
		} `json:"wifi"`
		Eth struct {
			IP string `json:"ip"`
		} `json:"eth"`
		Sys struct {
			Uptime           int `json:"uptime"`
			AvailableUpdates struct {
				Stable struct {
					Version string `json:"version"`
				} `json:"stable"`
			} `json:"available_updates"`
		} `json:"sys"`
	}
	_ = json.Unmarshal(body, &r)
	s.IP, s.SSID, s.RSSI = r.Wifi.StaIP, r.Wifi.SSID, r.Wifi.RSSI
	if s.IP == "" {
		s.IP = r.Eth.IP
	}
	s.UptimeS, s.Update = r.Sys.Uptime, r.Sys.AvailableUpdates.Stable.Version
	return s, nil
}

// parseGen1Status reads /status of a Gen1 device.
func parseGen1Status(body []byte) (Status, error) {
	var r struct {
		WifiSta struct {
			IP   string `json:"ip"`
			SSID string `json:"ssid"`
			RSSI *int   `json:"rssi"`
		} `json:"wifi_sta"`
		Relays []struct {
			IsOn *bool `json:"ison"`
		} `json:"relays"`
		Meters []struct {
			Power *float64 `json:"power"`
			Total *float64 `json:"total"` // watt-minutes
		} `json:"meters"`
		EMeters []struct {
			Power   *float64 `json:"power"`
			Voltage *float64 `json:"voltage"`
			Total   *float64 `json:"total"` // Wh
		} `json:"emeters"`
		Temperature *float64 `json:"temperature"`
		Tmp         *struct {
			TC *float64 `json:"tC"`
		} `json:"tmp"`
		Update struct {
			HasUpdate  bool   `json:"has_update"`
			NewVersion string `json:"new_version"`
		} `json:"update"`
		Uptime int `json:"uptime"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return Status{}, fmt.Errorf("unexpected status from Shelly: %w", err)
	}
	s := Status{IP: r.WifiSta.IP, SSID: r.WifiSta.SSID, RSSI: r.WifiSta.RSSI, UptimeS: r.Uptime, TempC: r.Temperature}
	if s.TempC == nil && r.Tmp != nil {
		s.TempC = r.Tmp.TC
	}
	if r.Update.HasUpdate {
		s.Update = r.Update.NewVersion
	}
	for i, rel := range r.Relays {
		ch := Channel{Type: "relay", ID: i, On: rel.IsOn}
		if i < len(r.Meters) {
			ch.PowerW = r.Meters[i].Power
			if t := r.Meters[i].Total; t != nil {
				ch.EnergyKWh = ptr(round3(*t / 60000))
			}
		}
		s.Channels = append(s.Channels, ch)
	}
	for i, m := range r.EMeters {
		ch := Channel{Type: "emeter", ID: i, PowerW: m.Power, VoltageV: m.Voltage}
		if m.Total != nil {
			ch.EnergyKWh = ptr(round3(*m.Total / 1000))
		}
		s.Channels = append(s.Channels, ch)
	}
	return s, nil
}

// Settings1 is the part of a Gen1 /settings answer Phoedox uses.
type Settings1 struct {
	Name   *string `json:"name"`
	Device struct {
		Hostname string `json:"hostname"`
	} `json:"device"`
}

func parseSettings1(body []byte) Settings1 {
	var s Settings1
	_ = json.Unmarshal(body, &s)
	return s
}

// Device is everything read from one Shelly.
type Device struct {
	Setting  string // name of the address setting
	Host     string // host of the configured address
	Info     Info
	Status   Status
	Name     string // Gen1: from /settings
	Hostname string // Gen1: from /settings
}

func (d Device) name() string {
	if d.Info.Name != nil && strings.TrimSpace(*d.Info.Name) != "" {
		return strings.TrimSpace(*d.Info.Name)
	}
	if strings.TrimSpace(d.Name) != "" {
		return strings.TrimSpace(d.Name)
	}
	label := d.Info.App
	if label == "" {
		label = d.Info.model()
	}
	mac := macID(d.Info.MAC)
	return strings.TrimSpace("Shelly " + label + " " + strings.ToUpper(mac[max(0, len(mac)-6):]))
}

func (d Device) hostname() string {
	if d.Info.Gen >= 2 {
		return d.Info.ID // the Gen2+ device id is its mDNS host name
	}
	return d.Hostname
}

func (d Device) ip() string {
	if d.Status.IP != "" {
		return d.Status.IP
	}
	if a, err := netip.ParseAddr(d.Host); err == nil {
		return a.String()
	}
	return ""
}

// graphOps imports a device as a smart plug (switch with power metering) or
// a device. Slugs are "shelly-<mac>" so a device keeps its entry when its
// address changes.
func graphOps(d Device) pdk.GraphOps {
	var ops pdk.GraphOps
	kind := "device"
	if d.Status.metered() {
		kind = "smart_plug"
	}
	attrs := map[string]any{"manufacturer": "Shelly", "mac": fmtMAC(d.Info.MAC), "shelly_gen": d.Info.Gen}
	set := func(k, v string) {
		if v != "" {
			attrs[k] = v
		}
	}
	set("model", d.Info.model())
	set("ip", d.ip())
	set("firmware", d.Info.firmware())
	set("shelly_app", d.Info.App)
	set("hostname", d.hostname())
	ops.UpsertNamed(kind, "shelly-"+macID(d.Info.MAC), d.name(), attrs)
	return ops
}

// Row is what shelly.status returns per device.
type Row struct {
	Setting  string    `json:"setting"`
	Name     string    `json:"name,omitempty"`
	Model    string    `json:"model,omitempty"`
	Gen      int       `json:"gen,omitempty"`
	IP       string    `json:"ip,omitempty"`
	Firmware string    `json:"firmware,omitempty"`
	Update   string    `json:"update_available,omitempty"`
	SSID     string    `json:"wifi_ssid,omitempty"`
	RSSI     *int      `json:"wifi_rssi_dbm,omitempty"`
	UptimeS  int       `json:"uptime_s,omitempty"`
	TempC    *float64  `json:"temperature_c,omitempty"`
	Channels []Channel `json:"channels,omitempty"`
	Error    string    `json:"error,omitempty"`
}

func row(d Device) Row {
	return Row{Setting: d.Setting, Name: d.name(), Model: d.Info.model(), Gen: d.Info.Gen, IP: d.ip(), Firmware: d.Info.firmware(),
		Update: d.Status.Update, SSID: d.Status.SSID, RSSI: d.Status.RSSI, UptimeS: d.Status.UptimeS, TempC: d.Status.TempC, Channels: d.Status.Channels}
}

// --- digest authentication (Gen2+) ---

// digestAuth answers a "WWW-Authenticate: Digest …" challenge (RFC 7616,
// SHA-256 or MD5, qop=auth) for one request.
func digestAuth(challenge, method, uri, user, pass, cnonce string) (string, error) {
	p, ok := parseChallenge(challenge)
	if !ok || p["nonce"] == "" || p["realm"] == "" {
		return "", errors.New("Shelly sent an unknown login challenge")
	}
	var h func() hash.Hash
	alg := p["algorithm"]
	switch strings.ToUpper(alg) {
	case "", "MD5":
		h = md5.New
	case "SHA-256":
		h = sha256.New
	default:
		return "", fmt.Errorf("Shelly asks for the unsupported digest algorithm %s", alg)
	}
	sum := func(s string) string {
		x := h()
		x.Write([]byte(s))
		return hex.EncodeToString(x.Sum(nil))
	}
	ha1 := sum(user + ":" + p["realm"] + ":" + pass)
	ha2 := sum(method + ":" + uri)
	const nc = "00000001"
	var resp string
	qop := ""
	if q := p["qop"]; q != "" {
		qop = "auth"
		if !slices.Contains(strings.Split(strings.ReplaceAll(q, " ", ""), ","), "auth") {
			return "", errors.New("Shelly asks for an unsupported digest qop")
		}
		resp = sum(ha1 + ":" + p["nonce"] + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
	} else {
		resp = sum(ha1 + ":" + p["nonce"] + ":" + ha2)
	}
	b := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`, user, p["realm"], p["nonce"], uri, resp)
	if alg != "" {
		b += ", algorithm=" + alg
	}
	if qop != "" {
		b += fmt.Sprintf(`, qop=%s, nc=%s, cnonce="%s"`, qop, nc, cnonce)
	}
	if o := p["opaque"]; o != "" {
		b += fmt.Sprintf(`, opaque="%s"`, o)
	}
	return b, nil
}

// parseChallenge splits `Digest a="x, y", b=z` into its parameters.
func parseChallenge(s string) (map[string]string, bool) {
	scheme, rest, _ := strings.Cut(strings.TrimSpace(s), " ")
	if !strings.EqualFold(scheme, "Digest") {
		return nil, false
	}
	out := map[string]string{}
	for rest = strings.TrimSpace(rest); rest != ""; rest = strings.TrimLeft(strings.TrimSpace(rest), ",") {
		rest = strings.TrimSpace(rest)
		k, v, ok := strings.Cut(rest, "=")
		if !ok {
			break
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, `"`) {
			end := strings.Index(v[1:], `"`)
			if end < 0 {
				return nil, false
			}
			out[k], rest = v[1:end+1], v[end+2:]
			continue
		}
		val, after, _ := strings.Cut(v, ",")
		out[k], rest = strings.TrimSpace(val), after
	}
	return out, true
}

// --- helpers ---

// deviceBase checks a configured device address.
func deviceBase(setting, raw string) (base, host string, err error) {
	base, err = integ.BaseURL(raw)
	if err != nil {
		return "", "", fmt.Errorf("the setting %s must be an address like http://192.168.1.50", setting)
	}
	u, _ := url.Parse(base)
	return base, u.Hostname(), nil
}

// macID turns "A8:03:2A:B1:23:45" into "a8032ab12345".
func macID(mac string) string {
	return strings.ToLower(strings.NewReplacer(":", "", "-", "", ".", "").Replace(strings.TrimSpace(mac)))
}

// fmtMAC turns "A8032AB12345" into "A8:03:2A:B1:23:45".
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

func sortChannels(cs []Channel) {
	slices.SortFunc(cs, func(a, b Channel) int {
		if c := strings.Compare(a.Type, b.Type); c != 0 {
			return c
		}
		return a.ID - b.ID
	})
}

func firstOf(vs ...*float64) *float64 {
	for _, v := range vs {
		if v != nil {
			return v
		}
	}
	return nil
}

func ptr[T any](v T) *T { return &v }

func round3(f float64) float64 {
	v, _ := strconv.ParseFloat(strconv.FormatFloat(f, 'f', 3, 64), 64)
	return v
}
