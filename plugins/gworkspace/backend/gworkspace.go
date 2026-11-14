// Logic of the Google Workspace plugin, without build tags so it is
// testable natively.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const (
	adminHost = "admin.googleapis.com"
	devices   = "https://" + adminHost + "/admin/directory/v1/customer/my_customer/devices/"
	// The fields are listed so that no user e-mail addresses, recent users
	// or annotated users are read at all.
	chromeFields = "nextPageToken,chromeosdevices(deviceId,serialNumber,status,lastSync,model,osVersion,orgUnitPath,annotatedAssetId,macAddress)"
	mobileFields = "nextPageToken,mobiledevices(resourceId,serialNumber,status,lastSync,model,os,type,name,manufacturer,brand)"
)

// chromeURL and mobileURL are the pages of the device listings.
func chromeURL(pageToken string) string {
	return devices + "chromeos?" + pageQuery(chromeFields, "200", pageToken)
}

func mobileURL(pageToken string) string {
	return devices + "mobile?" + pageQuery(mobileFields, "100", pageToken)
}

func pageQuery(fields, maxResults, pageToken string) string {
	v := url.Values{"projection": {"BASIC"}, "maxResults": {maxResults}, "fields": {fields}}
	if pageToken != "" {
		v.Set("pageToken", pageToken)
	}
	return v.Encode()
}

// apiError names the status of an Admin SDK answer.
func apiError(status int, body []byte) error {
	var r struct {
		Error struct {
			Status string `json:"status"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &r)
	msg := fmt.Sprintf("Google Admin SDK answered HTTP %d", status)
	if s := r.Error.Status; s != "" && len(s) < 64 {
		msg += " (" + s + ")"
	}
	if status == 403 {
		msg += "; admin_email must be an administrator who may view devices"
	}
	return fmt.Errorf("%s", msg)
}

// ChromeOS is a ChromeOS device.
type ChromeOS struct {
	DeviceID    string `json:"deviceId"`
	Serial      string `json:"serialNumber"`
	Status      string `json:"status"`
	LastSync    string `json:"lastSync"`
	Model       string `json:"model"`
	OSVersion   string `json:"osVersion"`
	OrgUnitPath string `json:"orgUnitPath"`
	AssetID     string `json:"annotatedAssetId"`
	MAC         string `json:"macAddress"`
}

// Mobile is a mobile device. Name holds the display names of its owners.
type Mobile struct {
	ResourceID   string   `json:"resourceId"`
	Serial       string   `json:"serialNumber"`
	Status       string   `json:"status"`
	LastSync     string   `json:"lastSync"`
	Model        string   `json:"model"`
	OS           string   `json:"os"`
	Type         string   `json:"type"`
	Name         []string `json:"name"`
	Manufacturer string   `json:"manufacturer"`
	Brand        string   `json:"brand"`
}

func parseChrome(body []byte) ([]ChromeOS, string, error) {
	var r struct {
		Devices []ChromeOS `json:"chromeosdevices"`
		Next    string     `json:"nextPageToken"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, "", fmt.Errorf("unexpected answer from Google Admin SDK: %w", err)
	}
	return r.Devices, r.Next, nil
}

func parseMobile(body []byte) ([]Mobile, string, error) {
	var r struct {
		Devices []Mobile `json:"mobiledevices"`
		Next    string   `json:"nextPageToken"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, "", fmt.Errorf("unexpected answer from Google Admin SDK: %w", err)
	}
	return r.Devices, r.Next, nil
}

var (
	macRe     = regexp.MustCompile(`^[0-9a-fA-F]{12}$`)
	osVerRe   = regexp.MustCompile(`^(.*?)\s+v?(\d[\w.]*)$`)
	hasLetter = regexp.MustCompile(`[A-Za-z]`)
)

// formatMAC turns "a4b1c2d3e4f5" into "a4:b1:c2:d3:e4:f5".
func formatMAC(s string) string {
	if !macRe.MatchString(s) {
		return s
	}
	s = strings.ToLower(s)
	parts := make([]string, 6)
	for i := range parts {
		parts[i] = s[2*i : 2*i+2]
	}
	return strings.Join(parts, ":")
}

// splitOS turns "Android 14" into ("Android", "14").
func splitOS(s string) (string, string) {
	s = strings.TrimSpace(s)
	if m := osVerRe.FindStringSubmatch(s); m != nil && hasLetter.MatchString(m[1]) {
		return m[1], m[2]
	}
	return s, ""
}

// syncTime is a last sync in UTC (RFC 3339), or "" for never.
func syncTime(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil || t.Year() < 2000 {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// Device is a device of either listing, as stored and listed.
type Device struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Platform     string `json:"platform"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	Serial       string `json:"serial,omitempty"`
	OS           string `json:"os,omitempty"`
	OSVersion    string `json:"os_version,omitempty"`
	Status       string `json:"status,omitempty"`
	LastSync     string `json:"last_sync,omitempty"`
	OrgUnit      string `json:"org_unit,omitempty"`
	User         string `json:"user,omitempty"`
	MAC          string `json:"-"`
	AssetID      string `json:"asset_id,omitempty"`
}

func label(model, serial, fallback string) string {
	switch {
	case model != "" && serial != "":
		return model + " " + serial
	case model != "":
		return model
	case serial != "":
		return serial
	}
	return fallback
}

// fromChrome converts ChromeOS devices.
func fromChrome(list []ChromeOS) []Device {
	var out []Device
	for _, c := range list {
		id := integ.Slugify(c.DeviceID)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(c.AssetID)
		if name == "" {
			name = label(c.Model, c.Serial, c.DeviceID)
		}
		out = append(out, Device{Slug: "gws-cros-" + id, Name: name, Platform: "chromeos", Model: c.Model, Serial: c.Serial,
			OS: "ChromeOS", OSVersion: c.OSVersion, Status: strings.ToLower(c.Status), LastSync: syncTime(c.LastSync),
			OrgUnit: c.OrgUnitPath, MAC: formatMAC(c.MAC), AssetID: c.AssetID})
	}
	return out
}

// fromMobile converts mobile devices; of the owners only the first display
// name is kept.
func fromMobile(list []Mobile) []Device {
	var out []Device
	for _, m := range list {
		id := integ.Slugify(m.ResourceID)
		if id == "" {
			continue
		}
		os, ver := splitOS(m.OS)
		maker := m.Manufacturer
		if maker == "" {
			maker = m.Brand
		}
		user := ""
		if len(m.Name) > 0 && !strings.Contains(m.Name[0], "@") {
			user = strings.TrimSpace(m.Name[0])
		}
		out = append(out, Device{Slug: "gws-mobile-" + id, Name: label(m.Model, m.Serial, m.ResourceID), Platform: strings.ToLower(m.Type),
			Manufacturer: maker, Model: m.Model, Serial: m.Serial, OS: os, OSVersion: ver, Status: strings.ToLower(m.Status),
			LastSync: syncTime(m.LastSync), User: user})
	}
	return out
}

// graphOps: every device as a device.
func graphOps(list []Device) pdk.GraphOps {
	var ops pdk.GraphOps
	for _, d := range list {
		attrs := map[string]any{}
		set := func(k, s string) {
			if s = strings.TrimSpace(s); s != "" {
				attrs[k] = s
			}
		}
		set("manufacturer", d.Manufacturer)
		set("model", d.Model)
		set("serial", d.Serial)
		set("mac", d.MAC)
		set("os", d.OS)
		set("os_version", d.OSVersion)
		set("status", d.Status)
		set("last_sync", d.LastSync)
		set("org_unit", d.OrgUnit)
		set("asset_tag", d.AssetID)
		set("user", d.User)
		set("managed_by", "Google Workspace")
		ops.UpsertNamed("device", d.Slug, d.Name, attrs)
	}
	return ops
}

// filter keeps the devices matching q (all for an empty q).
func filter(list []Device, q string) []Device {
	q = strings.ToLower(strings.TrimSpace(q))
	out := []Device{}
	for _, d := range list {
		if q == "" || strings.Contains(strings.ToLower(strings.Join([]string{d.Name, d.Serial, d.Model, d.OrgUnit}, "\n")), q) {
			out = append(out, d)
		}
	}
	return out
}
