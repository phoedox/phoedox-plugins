package main

import (
	"net/url"
	"strings"
	"testing"
)

// Answers of the Directory API with the plugin's fields selection.
const chromeJSON = `{
  "chromeosdevices": [
    {"deviceId": "4f1c3a2b-0d9e-4c8b-a7f6-5e4d3c2b1a09", "serialNumber": "5CD1234XYZ", "status": "ACTIVE", "lastSync": "2026-11-13T07:41:22.315Z",
     "model": "HP Chromebook x360 14c", "osVersion": "130.0.6723.126", "orgUnitPath": "/Schools/Library", "annotatedAssetId": "LIB-CB-07", "macAddress": "a4b1c2d3e4f5"},
    {"deviceId": "9a8b7c6d-5e4f-4a3b-2c1d-0e9f8a7b6c5d", "serialNumber": "NXH8Z0001", "status": "DEPROVISIONED", "lastSync": "1970-01-01T00:00:00.000Z",
     "model": "Acer Chromebook 314", "orgUnitPath": "/"}
  ],
  "nextPageToken": "CAEaIQoKMjAyNi0xMS0xMw"
}`

const mobileJSON = `{
  "mobiledevices": [
    {"resourceId": "AFiQxQ8Wn1H0bR9iYq4h", "serialNumber": "R58N123ABC", "status": "APPROVED", "lastSync": "2026-11-14T06:02:10.000Z",
     "model": "Pixel 8", "os": "Android 15", "type": "ANDROID", "name": ["Max Mustermann"], "manufacturer": "Google", "brand": "google"},
    {"resourceId": "AFiQxQ9-x7", "status": "PENDING", "model": "iPhone", "os": "iOS 18.1.1", "type": "IOS_SYNC", "name": ["max@acme.example"], "brand": "Apple"}
  ]
}`

func TestDevices(t *testing.T) {
	chrome, next, err := parseChrome([]byte(chromeJSON))
	if err != nil || len(chrome) != 2 || next != "CAEaIQoKMjAyNi0xMS0xMw" {
		t.Fatal(chrome, next, err)
	}
	mobile, next, err := parseMobile([]byte(mobileJSON))
	if err != nil || len(mobile) != 2 || next != "" {
		t.Fatal(mobile, next, err)
	}
	list := append(fromChrome(chrome), fromMobile(mobile)...)
	ops := graphOps(list).Ops
	if len(ops) != 4 {
		t.Fatalf("%+v", ops)
	}
	cb := ops[0]
	if cb.Kind != "device" || cb.ID != "gws-cros-4f1c3a2b-0d9e-4c8b-a7f6-5e4d3c2b1a09" || cb.Name != "LIB-CB-07" || cb.Attrs["serial"] != "5CD1234XYZ" ||
		cb.Attrs["model"] != "HP Chromebook x360 14c" || cb.Attrs["os"] != "ChromeOS" || cb.Attrs["os_version"] != "130.0.6723.126" || cb.Attrs["status"] != "active" ||
		cb.Attrs["last_sync"] != "2026-11-13T07:41:22Z" || cb.Attrs["org_unit"] != "/Schools/Library" || cb.Attrs["mac"] != "a4:b1:c2:d3:e4:f5" || cb.Attrs["asset_tag"] != "LIB-CB-07" {
		t.Fatalf("chromebook: %+v", cb)
	}
	if old := ops[1]; old.Name != "Acer Chromebook 314 NXH8Z0001" || old.Attrs["last_sync"] != nil || old.Attrs["status"] != "deprovisioned" {
		t.Fatalf("deprovisioned: %+v", old)
	}
	px := ops[2]
	if px.ID != "gws-mobile-afiqxq8wn1h0br9iyq4h" || px.Name != "Pixel 8 R58N123ABC" || px.Attrs["os"] != "Android" || px.Attrs["os_version"] != "15" ||
		px.Attrs["manufacturer"] != "Google" || px.Attrs["user"] != "Max Mustermann" || px.Attrs["status"] != "approved" {
		t.Fatalf("pixel: %+v", px)
	}
	ip := ops[3]
	if ip.Name != "iPhone" || ip.Attrs["os"] != "iOS" || ip.Attrs["os_version"] != "18.1.1" || ip.Attrs["manufacturer"] != "Apple" || ip.Attrs["user"] != nil {
		t.Fatalf("iphone (e-mail as owner name is dropped): %+v", ip)
	}
	for _, o := range ops {
		for k, v := range o.Attrs {
			if s, _ := v.(string); strings.Contains(s, "@") {
				t.Fatalf("e-mail address in attrs: %s=%v", k, v)
			}
		}
	}
	if f := filter(list, "library"); len(f) != 1 || f[0].Platform != "chromeos" {
		t.Fatalf("filter: %+v", f)
	}
	if f := filter(list, ""); len(f) != 4 || f[3].Platform != "ios_sync" {
		t.Fatalf("all: %+v", f)
	}
}

func TestURLs(t *testing.T) {
	u, err := url.Parse(chromeURL("tok/en=="))
	if err != nil || u.Host != "admin.googleapis.com" || u.Path != "/admin/directory/v1/customer/my_customer/devices/chromeos" {
		t.Fatal(u, err)
	}
	q := u.Query()
	if q.Get("pageToken") != "tok/en==" || q.Get("projection") != "BASIC" || q.Get("maxResults") != "200" || strings.Contains(q.Get("fields"), "recentUsers") || strings.Contains(q.Get("fields"), "annotatedUser") {
		t.Fatal(q)
	}
	m, _ := url.Parse(mobileURL(""))
	if m.Path != "/admin/directory/v1/customer/my_customer/devices/mobile" || m.Query().Has("pageToken") || strings.Contains(m.Query().Get("fields"), "email") {
		t.Fatal(m)
	}
	if err := apiError(403, []byte(`{"error":{"code":403,"message":"Not Authorized to access this resource/api","errors":[{"message":"Not Authorized to access this resource/api","domain":"global","reason":"forbidden"}],"status":"PERMISSION_DENIED"}}`)); err.Error() != "Google Admin SDK answered HTTP 403 (PERMISSION_DENIED); admin_email must be an administrator who may view devices" {
		t.Fatal(err)
	}
	if a, b := splitOS("Windows"); a != "Windows" || b != "" {
		t.Fatal(a, b)
	}
	if a, b := splitOS("2.3.5"); a != "2.3.5" || b != "" {
		t.Fatal(a, b)
	}
	if formatMAC("not-a-mac") != "not-a-mac" {
		t.Fatal("mac")
	}
}
