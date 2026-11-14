// Logic of the UniFi Protect plugin, without build tags so it is testable natively.
package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// apiBase turns the url setting (the console, e.g. https://192.168.1.1, or
// the cloud connector https://api.ui.com/v1/connector/consoles/<id>) into
// the base of the Protect integration API.
func apiBase(raw string) (string, error) {
	base, err := integ.BaseURL(raw)
	if err != nil {
		return "", err
	}
	base = strings.TrimSuffix(strings.TrimSuffix(base, "/v1"), "/proxy/protect/integration")
	return base + "/proxy/protect/integration/v1", nil
}

// Meta is GET /meta/info.
type Meta struct {
	Version string `json:"applicationVersion"`
}

// NVR is GET /nvrs.
type NVR struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"` // model, e.g. UNVR-PRO, UDM-PRO
	MAC     string `json:"mac"`
	ArmMode *struct {
		Status string `json:"status"`
	} `json:"armMode"`
}

// Device is a camera, light or sensor of the integration API.
type Device struct {
	ID            string `json:"id"`
	ModelKey      string `json:"modelKey"`
	State         string `json:"state"` // CONNECTED, CONNECTING, DISCONNECTED
	Name          string `json:"name"`
	Type          string `json:"type"` // model name, e.g. "G4 Bullet"
	MAC           string `json:"mac"`
	IsMicEnabled  *bool  `json:"isMicEnabled"`
	VideoMode     string `json:"videoMode"`
	HDRType       string `json:"hdrType"`
	HasPackageCam *bool  `json:"hasPackageCamera"`
	MountType     string `json:"mountType"`
	BatteryStatus *struct {
		Percentage *float64 `json:"percentage"`
		IsLow      *bool    `json:"isLow"`
	} `json:"batteryStatus"`
	SmartDetect *struct {
		ObjectTypes []string `json:"objectTypes"`
		AudioTypes  []string `json:"audioTypes"`
	} `json:"smartDetectSettings"`
}

func decode[T any](what string, body []byte) (T, error) {
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		return v, fmt.Errorf("unexpected %s from UniFi Protect: %w", what, err)
	}
	return v, nil
}

func parseNVR(body []byte) (NVR, error) {
	// Some releases answer with a list.
	if strings.HasPrefix(strings.TrimSpace(string(body)), "[") {
		list, err := decode[[]NVR]("NVR", body)
		if err != nil || len(list) == 0 {
			return NVR{}, fmt.Errorf("UniFi Protect returned no NVR")
		}
		return list[0], nil
	}
	return decode[NVR]("NVR", body)
}

func parseDevices(what string, body []byte) ([]Device, error) {
	return decode[[]Device](what, body)
}

// cameraState maps the Protect state to the values of the camera kind.
func cameraState(s string) string {
	s = strings.ToLower(s)
	if slices.Contains([]string{"connected", "connecting", "disconnected"}, s) {
		return s
	}
	return ""
}

// graphOps: the NVR (console) as device, cameras as camera, floodlights as
// light and sensors as device, each "related to" the NVR. Slugs use the
// MAC address, so entries survive a re-adoption.
func graphOps(nvr NVR, version string, cams, lights, sensors []Device) pdk.GraphOps {
	var ops pdk.GraphOps
	nattrs := map[string]any{"manufacturer": "Ubiquiti"}
	put(nattrs, "model", nvr.Type)
	put(nattrs, "mac", fmtMAC(nvr.MAC))
	put(nattrs, "protect_version", version)
	nname := nvr.Name
	if nname == "" {
		nname = "UniFi Protect"
	}
	ref := ops.UpsertNamed("device", "protect-nvr-"+key(nvr.MAC, nvr.ID, nname), nname, nattrs)
	add := func(kind, prefix string, d Device, attrs map[string]any) {
		put(attrs, "model", d.Type)
		put(attrs, "mac", fmtMAC(d.MAC))
		name := d.Name
		if name == "" {
			name = strings.TrimSpace(d.Type + " " + strings.ToUpper(macID(d.MAC)))
		}
		r := ops.UpsertNamed(kind, prefix+key(d.MAC, d.ID, name), name, attrs)
		ops.Link(r, "related_to", ref, nil)
	}
	for _, c := range cams {
		a := map[string]any{}
		put(a, "state", cameraState(c.State))
		add("camera", "protect-camera-", c, a)
	}
	for _, l := range lights {
		add("light", "protect-light-", l, map[string]any{"manufacturer": "Ubiquiti"})
	}
	for _, s := range sensors {
		a := map[string]any{"manufacturer": "Ubiquiti"}
		put(a, "mount_type", s.MountType)
		add("device", "protect-sensor-", s, a)
	}
	return ops
}

// CameraRow is what protect.cameras returns per camera.
type CameraRow struct {
	Name        string   `json:"name"`
	State       string   `json:"state"`
	Model       string   `json:"model,omitempty"`
	MAC         string   `json:"mac,omitempty"`
	VideoMode   string   `json:"video_mode,omitempty"`
	HDR         string   `json:"hdr,omitempty"`
	Mic         *bool    `json:"mic_enabled,omitempty"`
	SmartDetect []string `json:"smart_detect,omitempty"`
	AudioDetect []string `json:"audio_detect,omitempty"`
	PackageCam  *bool    `json:"package_camera,omitempty"`
}

func cameraRows(cams []Device) []CameraRow {
	out := make([]CameraRow, 0, len(cams))
	for _, c := range cams {
		r := CameraRow{Name: c.Name, State: strings.ToLower(c.State), Model: c.Type, MAC: fmtMAC(c.MAC), VideoMode: c.VideoMode, HDR: c.HDRType, Mic: c.IsMicEnabled, PackageCam: c.HasPackageCam}
		if c.SmartDetect != nil {
			r.SmartDetect, r.AudioDetect = c.SmartDetect.ObjectTypes, c.SmartDetect.AudioTypes
		}
		out = append(out, r)
	}
	return out
}

func put(m map[string]any, k, v string) {
	if v != "" {
		m[k] = v
	}
}

func key(mac, id, name string) string {
	if m := macID(mac); m != "" {
		return m
	}
	if id != "" {
		return integ.Slugify(id)
	}
	return integ.Slugify(name)
}

func macID(mac string) string {
	return strings.ToLower(strings.NewReplacer(":", "", "-", "").Replace(strings.TrimSpace(mac)))
}

// fmtMAC turns "24A43C3DFEB9" into "24:A4:3C:3D:FE:B9".
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
