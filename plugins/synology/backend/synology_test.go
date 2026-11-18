package main

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
)

func TestForms(t *testing.T) {
	v, err := url.ParseQuery(loginForm("phoedox", "p&ss=w rd+ü"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Get("api") != "SYNO.API.Auth" || v.Get("method") != "login" || v.Get("version") != "6" || v.Get("format") != "sid" ||
		v.Get("account") != "phoedox" || v.Get("passwd") != "p&ss=w rd+ü" {
		t.Fatalf("login form: %v", v)
	}
	v, _ = url.ParseQuery(callForm("SYNO.Core.System", 1, "info", "abc"))
	if v.Get("api") != "SYNO.Core.System" || v.Get("version") != "1" || v.Get("method") != "info" || v.Get("_sid") != "abc" {
		t.Fatalf("call form: %v", v)
	}
	v, _ = url.ParseQuery(logoutForm("abc"))
	if v.Get("method") != "logout" || v.Get("_sid") != "abc" {
		t.Fatalf("logout form: %v", v)
	}
}

func TestLoginAndErrors(t *testing.T) {
	data, err := unwrap([]byte(`{"data":{"did":"x","is_portal_port":false,"sid":"Wm9vYmFy"},"success":true}`), true)
	if err != nil {
		t.Fatal(err)
	}
	if sid, err := parseSID(data); err != nil || sid != "Wm9vYmFy" {
		t.Fatal(sid, err)
	}
	if _, err := parseSID([]byte(`{}`)); err == nil {
		t.Fatal("accepted a missing sid")
	}
	cases := map[string]string{
		`{"error":{"code":400},"success":false}`:                                                 "username or password",
		`{"error":{"code":403,"errors":{"token":"x","types":[{"type":"otp"}]}},"success":false}`: "2-step verification",
		`{"error":{"code":406},"success":false}`:                                                 "enforces 2-step",
		`{"error":{"code":407},"success":false}`:                                                 "blocked the IP address",
		`{"error":{"code":401},"success":false}`:                                                 "disabled",
		`{"success":false}`:                                                                      "DSM error 100",
	}
	for body, want := range cases {
		_, err := unwrap([]byte(body), true)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", body, err, want)
		}
	}
	// Outside SYNO.API.Auth, 400+ are not sign-in errors; 105 is a permission error.
	if _, err := unwrap([]byte(`{"error":{"code":403},"success":false}`), false); err == nil || strings.Contains(err.Error(), "2-step") {
		t.Fatal(err)
	}
	if _, err := unwrap([]byte(`{"error":{"code":105},"success":false}`), false); err == nil || !strings.Contains(err.Error(), "administrators") {
		t.Fatal(err)
	}
	if _, err := unwrap([]byte(`<html>login</html>`), false); err == nil {
		t.Fatal("accepted HTML")
	}
}

func TestSystemAndGraph(t *testing.T) {
	data, _ := unwrap([]byte(`{"data":{"cpu_clock_speed":2600,"cpu_cores":"4","cpu_family":"Ryzen","cpu_series":"R1600","cpu_vendor":"AMD",
		"enabled_ntp":true,"firmware_date":"2024/05/21","firmware_ver":"DSM 7.2.1-69057 Update 5","model":"DS923+","ram_size":4096,
		"serial":"2150ABN123456","sys_temp":42,"up_time":"123:45:6"},"success":true}`), false)
	n := parseSystem(data)
	n.Hostname = parseHostname([]byte(`{"dns_primary":"192.168.1.1","gateway":"192.168.1.1","server_name":"nas01","v6gateway":""}`))
	if n.Model != "DS923+" || n.Serial != "2150ABN123456" || n.RAMMB != 4096 || n.Hostname != "nas01" {
		t.Fatalf("nas: %+v", n)
	}
	ops := graphOps(n, "https://192.168.1.20:5001")
	if len(ops.Ops) != 1 {
		t.Fatalf("ops: %+v", ops.Ops)
	}
	op := ops.Ops[0]
	if op.Kind != "server" || op.ID != "synology-2150abn123456" || op.Name != "nas01" {
		t.Fatalf("server: %+v", op)
	}
	want := map[string]any{"hostname": "nas01", "model": "DS923+", "os": "DSM 7.2.1-69057 Update 5", "serial": "2150ABN123456", "ram": 4.0,
		"ip": "192.168.1.20", "manufacturer": "Synology"}
	for k, v := range want {
		if op.Attrs[k] != v {
			t.Errorf("attr %s = %v, want %v", k, op.Attrs[k], v)
		}
	}
	for k := range op.Attrs {
		if k == "password" || k == "sid" || k == "username" {
			t.Fatal("secret in attrs")
		}
	}
}

func TestDSMInfoFallback(t *testing.T) {
	n := parseDSMInfo([]byte(`{"codepage":"enu","model":"DS218+","ram":"2048","serial":"1780PDN012345","temperature":38,"temperature_warn":false,
		"uptime":86400,"version":"42962","version_string":"DSM 6.2.4-25556 Update 7"}`))
	if n.Model != "DS218+" || n.RAMMB != 2048 || n.Version != "DSM 6.2.4-25556 Update 7" {
		t.Fatalf("dsm info: %+v", n)
	}
	if osName("7.1-42661") != "DSM 7.1-42661" || osName("") != "" {
		t.Fatal("os name")
	}
	if parseHostname([]byte(`{"hostname":"Archive"}`)) != "Archive" || parseHostname([]byte(`null`)) != "" {
		t.Fatal("hostname")
	}
	// Without hostname the model names the entry; without serial, model and hostname key it.
	op := graphOps(NAS{Model: "DS218+"}, "https://nas.example.org").Ops[0]
	if op.Name != "Synology DS218+" || op.ID != "synology-ds218" || op.Attrs["ip"] != nil {
		t.Fatalf("fallback: %+v", op)
	}
	if op := graphOps(NAS{}, "x").Ops[0]; op.ID != "synology-nas" || op.Name != "Synology NAS" {
		t.Fatalf("empty: %+v", op)
	}
}

func TestStorage(t *testing.T) {
	data, err := unwrap([]byte(`{"success":true,"data":{
		"disks":[
			{"id":"sata1","name":"Drive 1","longName":"Drive 1","vendor":"WDC","model":"WD40EFRX-68N32N0","diskType":"SATA","size_total":"4000787030016",
			 "status":"normal","smart_status":"normal","temp":35,"used_by":"reuse_1","serial":"WD-WCC7K0000000"},
			{"id":"sata2","name":"Drive 2","vendor":"Seagate","model":"ST4000VN008","diskType":"SATA","size_total":4000787030016,"status":"crashed","smart_status":"failing","temp":"41","used_by":"reuse_1"}],
		"storagePools":[{"id":"reuse_1","num_id":1,"status":"degraded","device_type":"shr_with_1_disk_protect","size":{"total":"3995903488000","used":"3995903488000"},"disks":["sata1","sata2"]}],
		"volumes":[{"id":"volume_1","display_name":"Volume 1","vol_path":"/volume1","status":"degraded","fs_type":"btrfs","pool_path":"reuse_1",
			"size":{"free_inode":"0","total":"3839700664320","total_device":"3995903488000","used":"1073741824000"}},
			{"id":"volume_2","vol_path":"/volume2","status":"normal","fs_type":"ext4","size":{}}]}}`), false)
	if err != nil {
		t.Fatal(err)
	}
	s, err := parseStorage(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Pools) != 1 || s.Pools[0].Status != "degraded" || s.Pools[0].RAID != "shr_with_1_disk_protect" || s.Pools[0].TotalGiB != 3721.5 || len(s.Pools[0].Disks) != 2 {
		t.Fatalf("pools: %+v", s.Pools)
	}
	v := s.Volumes[0]
	if v.Name != "Volume 1" || v.Path != "/volume1" || v.FS != "btrfs" || v.UsedGiB != 1000 || v.TotalGiB != 3576 || v.FreeGiB != 2576 || v.UsedPercent != 28 {
		t.Fatalf("volume: %+v", v)
	}
	if s.Volumes[1].TotalGiB != 0 || s.Volumes[1].UsedPercent != 0 {
		t.Fatalf("volume without size: %+v", s.Volumes[1])
	}
	d1, d2 := s.Disks[0], s.Disks[1]
	if d1.Name != "Drive 1" || d1.SizeGiB != 3726 || d1.TempC != 35 || d1.SMART != "normal" || d1.Pool != "reuse_1" {
		t.Fatalf("disk 1: %+v", d1)
	}
	if d2.Status != "crashed" || d2.SMART != "failing" || d2.TempC != 41 || d2.SizeGiB != 3726 {
		t.Fatalf("disk 2 (numbers as strings and numbers): %+v", d2)
	}
	empty, err := parseStorage([]byte(`{}`))
	if err != nil || empty.Pools == nil || empty.Volumes == nil || empty.Disks == nil {
		t.Fatalf("empty storage must give empty lists: %+v %v", empty, err)
	}
	if _, err := parseStorage([]byte(`{"volumes":"x"}`)); err == nil {
		t.Fatal("accepted a broken answer")
	}
}

// TestDemoData parses the recorded demo answers the screenshots use. DSM
// answers every call on /webapi/entry.cgi, so routes.json plays them as a
// sequence: login, SYNO.Core.System info, SYNO.Core.Network get, logout.
func TestDemoData(t *testing.T) {
	const dir = "../testdata/fake"
	body, err := fakeapi.Body(dir, "POST", "/webapi/entry.cgi")
	if err != nil {
		t.Fatal(err)
	}
	data, err := unwrap(body, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseSID(data); err != nil {
		t.Fatal(err)
	}
	api, err := fakeapi.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	read := func(file string) json.RawMessage {
		t.Helper()
		body, err := api.Content(fakeapi.Route{File: file})
		if err != nil {
			t.Fatal(err)
		}
		data, err := unwrap(body, false)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		return data
	}
	n := parseSystem(read("system.json"))
	n.Hostname = parseHostname(read("network.json"))
	op := graphOps(n, "http://127.0.0.1:5000").Ops[0]
	if op.ID != "synology-demo-2150abn0042" || op.Name != "nas" || op.Attrs["os"] != "DSM 7.2.2-72806 Update 4" || op.Attrs["ram"] != 8.0 || op.Attrs["ip"] != nil {
		t.Fatalf("nas: %+v", op)
	}
	st, err := parseStorage(read("storage.json"))
	if err != nil || len(st.Pools) != 1 || len(st.Volumes) != 1 || len(st.Disks) != 4 || st.Volumes[0].UsedPercent != 40.1 {
		t.Fatalf("storage: %+v %v", st, err)
	}
}
