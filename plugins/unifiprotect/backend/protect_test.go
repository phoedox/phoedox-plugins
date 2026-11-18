package main

import (
	"os"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
)

const cameras = `[
 {"id":"66d025b301ebc903e80003ea","modelKey":"camera","state":"CONNECTED","name":"Front Door","type":"G4 Doorbell Pro","guid":"x","mac":"F4E2C6A1B2C3",
  "isMicEnabled":true,"osdSettings":{"isNameEnabled":true,"isDateEnabled":true,"isLogoEnabled":false,"isDebugEnabled":false,"overlayLocation":"topLeft"},
  "ledSettings":{"isEnabled":true},"lcdMessage":{},"micVolume":80,"activePatrolSlot":null,"videoMode":"default","hdrType":"auto",
  "featureFlags":{"supportFullHdSnapshot":true,"hasHdr":true,"smartDetectTypes":["person","vehicle","package"],"smartDetectAudioTypes":[],"videoModes":["default"],"hasMic":true,"hasLedStatus":true,"hasSpeaker":true},
  "smartDetectSettings":{"objectTypes":["person","package"],"audioTypes":[]},"hasPackageCamera":true},
 {"id":"66d025b301ebc903e80003eb","modelKey":"camera","state":"DISCONNECTED","name":null,"type":"G5 Bullet","guid":"y","mac":"F4E2C6D4E5F6",
  "isMicEnabled":false,"videoMode":"highFps","hdrType":"off","smartDetectSettings":{"objectTypes":[],"audioTypes":[]},"hasPackageCamera":false},
 {"id":"66d025b301ebc903e80003ec","modelKey":"camera","state":"UPDATING","name":"Garage","type":"G4 Instant","mac":"F4E2C6000001"}]`

const nvr = `{"id":"66d025b301ebc903e80003e0","modelKey":"nvr","name":"Home NVR","type":"UNVR","guid":"z","mac":"D021F98B6AED",
	"doorbellSettings":{"defaultMessageText":"Welcome"},"armMode":{"status":"disabled","armProfileId":null}}`

func TestImport(t *testing.T) {
	cams, err := parseDevices("camera list", []byte(cameras))
	if err != nil || len(cams) != 3 {
		t.Fatalf("cameras: %v", err)
	}
	n, err := parseNVR([]byte(nvr))
	if err != nil || n.Type != "UNVR" {
		t.Fatalf("nvr: %+v %v", n, err)
	}
	lights, _ := parseDevices("lights", []byte(`[{"id":"l1","modelKey":"light","state":"CONNECTED","name":"Driveway","type":"UP FloodLight","mac":"7483C2AABBCC","isLightOn":false}]`))
	sensors, _ := parseDevices("sensors", []byte(`[{"id":"s1","modelKey":"sensor","state":"CONNECTED","name":"Back Door","type":"UP Sense","mac":"7483C2DDEEFF","mountType":"door","batteryStatus":{"percentage":90,"isLow":false}}]`))
	ops := graphOps(n, "6.1.79", cams, lights, sensors).Ops
	if ops[0].Kind != "device" || ops[0].ID != "protect-nvr-d021f98b6aed" || ops[0].Name != "Home NVR" || ops[0].Attrs["model"] != "UNVR" ||
		ops[0].Attrs["mac"] != "D0:21:F9:8B:6A:ED" || ops[0].Attrs["protect_version"] != "6.1.79" {
		t.Fatalf("nvr op: %+v", ops[0])
	}
	door := ops[1]
	if door.Kind != "camera" || door.ID != "protect-camera-f4e2c6a1b2c3" || door.Name != "Front Door" || door.Attrs["model"] != "G4 Doorbell Pro" ||
		door.Attrs["state"] != "connected" || door.Attrs["mac"] != "F4:E2:C6:A1:B2:C3" {
		t.Fatalf("camera: %+v", door)
	}
	if ops[2].Op != "link" || ops[2].From != door.Ref || ops[2].Rel != "related_to" || ops[2].To != ops[0].Ref {
		t.Fatalf("link: %+v", ops[2])
	}
	if ops[3].Name != "G5 Bullet F4E2C6D4E5F6" || ops[3].Attrs["state"] != "disconnected" {
		t.Fatalf("unnamed camera: %+v", ops[3])
	}
	if _, ok := ops[5].Attrs["state"]; ok {
		t.Fatalf("unknown state must be left out (enum): %+v", ops[5])
	}
	if ops[7].Kind != "light" || ops[7].ID != "protect-light-7483c2aabbcc" || ops[9].Kind != "device" || ops[9].Attrs["mount_type"] != "door" {
		t.Fatalf("light/sensor: %+v %+v", ops[7], ops[9])
	}
	if len(ops) != 11 {
		t.Fatalf("ops: %d", len(ops))
	}
	rows := cameraRows(cams)
	if rows[0].State != "connected" || len(rows[0].SmartDetect) != 2 || !*rows[0].Mic || rows[1].VideoMode != "highFps" {
		t.Fatalf("rows: %+v", rows)
	}
}

func TestParsing(t *testing.T) {
	for in, want := range map[string]string{
		"https://192.168.1.1":                               "https://192.168.1.1/proxy/protect/integration/v1",
		"https://192.168.1.1/proxy/protect/integration/v1/": "https://192.168.1.1/proxy/protect/integration/v1",
		"https://api.ui.com/v1/connector/consoles/ABC:123":  "https://api.ui.com/v1/connector/consoles/ABC:123/proxy/protect/integration/v1",
	} {
		if got, err := apiBase(in); err != nil || got != want {
			t.Errorf("%s: %s %v", in, got, err)
		}
	}
	if _, err := apiBase("ftp://x"); err == nil {
		t.Fatal("ftp accepted")
	}
	if n, err := parseNVR([]byte(`[` + nvr + `]`)); err != nil || n.Name != "Home NVR" {
		t.Fatalf("nvr list: %+v %v", n, err)
	}
	if _, err := parseNVR([]byte(`[]`)); err == nil {
		t.Fatal("empty nvr list accepted")
	}
	if _, err := parseDevices("camera list", []byte(`{"error":"Unauthorized","name":"UNAUTHORIZED"}`)); err == nil {
		t.Fatal("error object accepted")
	}
}

// TestCameraSchema: UniFi Protect and Frigate both contribute the kind
// camera; the host uses the first active plugin's schema, so both must be
// identical.
func TestCameraSchema(t *testing.T) {
	a, err := os.ReadFile("../schemas/camera.json")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("../../frigate/schemas/camera.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("schemas/camera.json differs between unifiprotect and frigate")
	}
	data, _ := os.ReadFile("../phoedox-plugin.json")
	m, err := pdk.ParseManifest(data)
	if err != nil || len(m.Contributes.Kinds) != 1 || m.Contributes.Kinds[0].ID != "camera" {
		t.Fatalf("manifest: %v", err)
	}
}

// TestDemoData parses the recorded demo answers the screenshots use.
func TestDemoData(t *testing.T) {
	base, _ := apiBase("https://unifi.example.org")
	get := func(p string) []byte {
		body, err := fakeapi.Body("../testdata/fake", "GET", strings.TrimPrefix(base, "https://unifi.example.org")+p)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	cams, err := parseDevices("camera list", get("/cameras"))
	if err != nil || len(cams) != 6 {
		t.Fatalf("%v %d", err, len(cams))
	}
	nvr, err := parseNVR(get("/nvrs"))
	if err != nil || nvr.Type != "UNVR" {
		t.Fatalf("%v %+v", err, nvr)
	}
	meta, err := decode[Meta]("version", get("/meta/info"))
	if err != nil {
		t.Fatal(err)
	}
	lights, err := parseDevices("lights", get("/lights"))
	if err != nil {
		t.Fatal(err)
	}
	sensors, err := parseDevices("sensors", get("/sensors"))
	if err != nil {
		t.Fatal(err)
	}
	ops := graphOps(nvr, meta.Version, cams, lights, sensors).Ops
	if len(ops) != 1+(6+1+3)*2 || ops[0].Attrs["protect_version"] != "6.0.53" {
		t.Fatalf("ops: %d %+v", len(ops), ops[0])
	}
	if rows := cameraRows(cams); rows[4].State != "disconnected" || len(rows[0].SmartDetect) != 2 {
		t.Fatalf("rows: %+v", rows)
	}
}
