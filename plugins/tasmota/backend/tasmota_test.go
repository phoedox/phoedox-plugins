package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
)

const status0 = `{"Status":{"Module":0,"DeviceName":"Washing Machine","FriendlyName":["Washing Machine"],"Topic":"tasmota_A1B2C3","ButtonTopic":"0",
	"Power":"1","PowerOnState":3,"LedState":1,"LedMask":"FFFF","SaveData":1,"SaveState":1,"SwitchTopic":"0","SwitchMode":[0,0,0,0,0,0,0,0]},
"StatusPRM":{"Baudrate":115200,"SerialConfig":"8N1","GroupTopic":"tasmotas","OtaUrl":"http://ota.tasmota.com/tasmota/release/tasmota.bin.gz","RestartReason":"Software/System restart","Uptime":"3T04:05:06","BootCount":17},
"StatusFWR":{"Version":"14.3.0(release-tasmota)","BuildDateTime":"2024-10-15T13:24:23","Boot":31,"Core":"2_7_7","SDK":"2.2.2-dev(38a443e)","CpuFrequency":80,"Hardware":"ESP8266EX","CR":"401/699"},
"StatusLOG":{"SerialLog":0,"WebLog":2,"MqttLog":0,"SysLog":0,"LogHost":"","LogPort":514,"SSId":["IoT",""],"TelePeriod":300},
"StatusMEM":{"ProgramSize":632,"Free":368,"Heap":25,"ProgramFlashSize":1024,"FlashSize":4096,"FlashChipId":"164020","FlashFrequency":40,"FlashMode":"DOUT"},
"StatusNET":{"Hostname":"washer-2563","IPAddress":"192.168.20.63","Gateway":"192.168.20.1","Subnetmask":"255.255.255.0","DNSServer1":"192.168.20.1","DNSServer2":"0.0.0.0","Mac":"84:F3:EB:A1:B2:C3","Webserver":2,"HTTP_API":1,"WifiConfig":4,"WifiPower":17.0},
"StatusMQT":{"MqttHost":"mqtt.lan","MqttPort":1883,"MqttClientMask":"DVES_%06X","MqttClient":"DVES_A1B2C3","MqttUser":"DVES_USER","MqttCount":1},
"StatusTIM":{"UTC":"2024-11-14T10:00:00","Local":"2024-11-14T11:00:00","StartDST":"2024-03-31T02:00:00","EndDST":"2024-10-27T03:00:00","Timezone":"99"},
"StatusSNS":{"Time":"2024-11-14T11:00:00","ENERGY":{"TotalStartTime":"2023-01-05T18:21:34","Total":512.345,"Yesterday":1.234,"Today":0.456,"Power":1850,"ApparentPower":1900,"ReactivePower":300,"Factor":0.97,"Voltage":229,"Current":8.08}},
"StatusSTS":{"Time":"2024-11-14T11:00:00","Uptime":"3T04:05:06","UptimeSec":273906,"Heap":25,"SleepMode":"Dynamic","Sleep":50,"LoadAvg":19,"MqttCount":1,"POWER":"ON",
	"Wifi":{"AP":1,"SSId":"IoT","BSSId":"AA:BB:CC:DD:EE:FF","Channel":6,"Mode":"11n","RSSI":76,"Signal":-62,"LinkCount":1,"Downtime":"0T00:00:03"}}}`

func TestStatus(t *testing.T) {
	s, err := parseStatus0([]byte(status0))
	if err != nil {
		t.Fatal(err)
	}
	d := Device{Setting: "url", Status: s, Module: parseModule([]byte(`{"Module":{"0":"Athom Plug V2"}}`))}
	ops := graphOps(d)
	op := ops.Ops[0]
	if len(ops.Ops) != 1 || op.Kind != "smart_plug" || op.ID != "tasmota-84f3eba1b2c3" || op.Name != "Washing Machine" {
		t.Fatalf("op: %+v", op)
	}
	want := map[string]any{"model": "Athom Plug V2", "module": "Athom Plug V2", "mac": "84:F3:EB:A1:B2:C3", "ip": "192.168.20.63",
		"hostname": "washer-2563", "firmware": "14.3.0(release-tasmota)", "hardware": "ESP8266EX", "mqtt_topic": "tasmota_A1B2C3"}
	for k, v := range want {
		if op.Attrs[k] != v {
			t.Errorf("attr %s: %v, want %v", k, op.Attrs[k], v)
		}
	}
	r := row(d)
	if r.Power["POWER"] != "ON" || r.Energy.PowerW != float64(1850) || *r.Energy.TodayKWh != 0.456 || *r.Signal != -62 || *r.Quality != 76 || r.Uptime != "3T04:05:06" {
		t.Fatalf("row: %+v %+v", r, r.Energy)
	}
}

func TestDeviceWithoutMeter(t *testing.T) {
	s, err := parseStatus0([]byte(`{"Status":{"Module":1,"DeviceName":"Tasmota","FriendlyName":["Tasmota","Tasmota2"],"Topic":"sonoff"},
		"StatusFWR":{"Version":"9.5.0(tasmota)","Hardware":"ESP8266EX"},
		"StatusNET":{"Hostname":"","IPAddress":"0.0.0.0","Mac":"","Ethernet":{"IPAddress":"10.0.0.9","Mac":"A8:48:FA:00:11:22"}},
		"StatusSNS":{"Time":"2024-11-14T11:00:00"},"StatusSTS":{"POWER1":"OFF","POWER2":"ON","POWERONSTATE":"x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	op := graphOps(Device{Status: s, Module: parseModule([]byte(`{"Module":"1 (Sonoff Basic)"}`))}).Ops[0]
	if op.Kind != "device" || op.ID != "tasmota-a848fa001122" || op.Name != "Tasmota2" || op.Attrs["ip"] != "10.0.0.9" || op.Attrs["model"] != "Sonoff Basic" {
		t.Fatalf("op: %+v", op)
	}
	if p := s.power(); len(p) != 2 || p["POWER2"] != "ON" {
		t.Fatalf("power: %v", p)
	}
}

func TestErrors(t *testing.T) {
	if _, err := parseStatus0([]byte(`{"WARNING":"Need user=<username>&password=<password>"}`)); err != errAuth {
		t.Fatalf("auth: %v", err)
	}
	if _, err := parseStatus0([]byte(`{"Command":"Unknown"}`)); err == nil {
		t.Fatal("unknown answer accepted")
	}
	if _, err := parseStatus0([]byte(`<html>`)); err == nil {
		t.Fatal("html accepted")
	}
	if parseModule([]byte(`nonsense`)) != "" {
		t.Fatal("module")
	}
	if _, err := deviceBase("device3_url", "192.168.1.60"); err == nil || !strings.Contains(err.Error(), "device3_url") {
		t.Fatalf("base: %v", err)
	}
}

func TestCredentials(t *testing.T) {
	if u := commandURL("http://10.0.0.5", "Status 0", "admin", ""); u != "http://10.0.0.5/cm?cmnd=Status%200" {
		t.Fatal(u)
	}
	u := commandURL("http://10.0.0.5", "Status 0", "admin", "p&s w+rd")
	if u != "http://10.0.0.5/cm?cmnd=Status%200&user=admin&password=p%26s%20w%2Brd" {
		t.Fatal(u)
	}
	msg := `fetch 10.0.0.5: Get "` + u + `": dial tcp 10.0.0.5:80: connect: no route to host`
	if r := redact(msg, "p&s w+rd"); strings.Contains(r, "p%26s") || !strings.Contains(r, "no route to host") {
		t.Fatal(r)
	}
}

func TestManifestSlots(t *testing.T) {
	data, err := os.ReadFile("../phoedox-plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := pdk.ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range slots() {
		if !slices.Contains(m.Permissions, "secrets:"+s) || (s != "url" && !strings.HasSuffix(s, "_url")) {
			t.Errorf("slot %s not declared", s)
		}
	}
}

// TestDemoData parses the recorded demo answers the screenshots use.
func TestDemoData(t *testing.T) {
	kinds := map[string]int{}
	for _, slug := range []string{"washer", "dryer", "dishwasher", "aquarium", "ledstrip", "heatpump", "fan"} {
		body, err := fakeapi.Body("../testdata/fake", "GET", "/"+slug+"/cm?cmnd=Status%200")
		if err != nil {
			t.Fatal(err)
		}
		s, err := parseStatus0(body)
		if err != nil {
			t.Fatal(slug, err)
		}
		if body, err = fakeapi.Body("../testdata/fake", "GET", "/"+slug+"/cm?cmnd=Module"); err != nil {
			t.Fatal(err)
		}
		d := Device{Setting: "url", Status: s, Module: parseModule(body)}
		op := graphOps(d).Ops[0]
		if d.Module == "" || op.Attrs["ip"] == nil || op.Attrs["mac"] == "" {
			t.Fatalf("%s: %+v", slug, op)
		}
		kinds[op.Kind]++
	}
	if kinds["smart_plug"] != 3 || kinds["device"] != 4 {
		t.Fatalf("kinds: %v", kinds)
	}
}
