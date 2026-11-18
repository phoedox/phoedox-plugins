package main

import (
	"strings"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
)

const systemInfoJSON = `{"version":"TrueNAS-SCALE-24.04.2","buildtime":{"$date":1719240000000},"hostname":"truenas","physmem":34359738368,
	"model":"AMD Ryzen 5 5600G with Radeon Graphics","cores":12,"physical_cores":6,"loadavg":[0.1,0.2,0.3],"uptime":"3 days, 2:01:00",
	"uptime_seconds":266460.5,"system_serial":"A1-23456","system_product":"TRUENAS-MINI-3.0-X+","system_product_version":"1.0",
	"license":null,"boottime":{"$date":1730000000000},"datetime":{"$date":1730266460000},"timezone":"Europe/Berlin",
	"system_manufacturer":"iXsystems","ecc_memory":true}`

func TestSystemAndGraph(t *testing.T) {
	s, err := parseSystem([]byte(systemInfoJSON))
	if err != nil {
		t.Fatal(err)
	}
	ops := graphOps(s, "https://192.168.1.30")
	if len(ops.Ops) != 1 {
		t.Fatalf("ops: %+v", ops.Ops)
	}
	op := ops.Ops[0]
	if op.Kind != "server" || op.ID != "truenas-a1-23456" || op.Name != "truenas" {
		t.Fatalf("server: %+v", op)
	}
	want := map[string]any{"hostname": "truenas", "os": "TrueNAS SCALE 24.04.2", "model": "TRUENAS-MINI-3.0-X+", "manufacturer": "iXsystems",
		"serial": "A1-23456", "cpu": "AMD Ryzen 5 5600G with Radeon Graphics", "cores": 12, "ram": 32.0, "ip": "192.168.1.30"}
	for k, v := range want {
		if op.Attrs[k] != v {
			t.Errorf("attr %s = %v, want %v", k, op.Attrs[k], v)
		}
	}
}

func TestPlaceholdersAndVersions(t *testing.T) {
	// A self-built box: DMI placeholders are dropped, the hostname keys the entry.
	s, err := parseSystem([]byte(`{"version":"TrueNAS-13.0-U6.1","hostname":"backup","physmem":"17179869184","model":"Intel(R) Xeon(R) E-2224",
		"system_serial":"To Be Filled By O.E.M.","system_product":"Default string","system_manufacturer":"System manufacturer"}`))
	if err != nil {
		t.Fatal(err)
	}
	op := graphOps(s, "https://backup.example.org").Ops[0]
	if op.ID != "truenas-backup" || op.Attrs["serial"] != nil || op.Attrs["model"] != nil || op.Attrs["manufacturer"] != nil || op.Attrs["os"] != "TrueNAS 13.0-U6.1" || op.Attrs["ram"] != 16.0 || op.Attrs["ip"] != nil {
		t.Fatalf("diy: %+v", op)
	}
	for in, want := range map[string]string{"25.04.0": "TrueNAS 25.04.0", "TrueNAS-CORE-13.3": "TrueNAS CORE 13.3", "TrueNAS 25.10.1": "TrueNAS 25.10.1", "": ""} {
		if got := osName(in); got != want {
			t.Errorf("osName(%q) = %q, want %q", in, got, want)
		}
	}
	if op := graphOps(System{}, "x").Ops[0]; op.ID != "truenas-truenas" || op.Name != "TrueNAS" {
		t.Fatalf("empty: %+v", op)
	}
	if _, err := parseSystem([]byte(`[]`)); err == nil {
		t.Fatal("accepted a list")
	}
}

func TestPools(t *testing.T) {
	pools, err := parsePools([]byte(`[
		{"id":1,"name":"tank","guid":"1234","path":"/mnt/tank","status":"ONLINE","healthy":true,"warning":false,"status_code":"HEALTHY",
		 "size":3985729650688,"allocated":1099511627776,"free":2886217998912,"freeing":0,"fragmentation":"3","topology":{"data":[]}},
		{"id":2,"name":"scratch","path":"/mnt/scratch","status":"DEGRADED","healthy":false,"size":"1099511627776","allocated":"0","free":"1099511627776"},
		{"id":3,"name":"old","status":"ONLINE","healthy":true}]`))
	if err != nil || len(pools) != 3 {
		t.Fatalf("pools: %+v %v", pools, err)
	}
	if p := pools[0]; p.Name != "tank" || !p.Healthy || p.SizeGiB != 3712 || p.AllocatedGiB != 1024 || p.FreeGiB != 2688 || p.UsedPercent != 27.6 {
		t.Fatalf("tank: %+v", p)
	}
	if p := pools[1]; p.Healthy || p.Status != "DEGRADED" || p.SizeGiB != 1024 || p.UsedPercent != 0 {
		t.Fatalf("scratch (numbers as strings): %+v", p)
	}
	if p := pools[2]; p.SizeGiB != 0 || !p.Healthy {
		t.Fatalf("old (CORE without sizes): %+v", p)
	}
	if _, err := parsePools([]byte(`{"message":"Not authenticated"}`)); err == nil {
		t.Fatal("accepted an error object")
	}
}

func TestShares(t *testing.T) {
	shares, err := parseShares(
		[]byte(`[{"id":1,"purpose":"DEFAULT_SHARE","path":"/mnt/tank/media","name":"media","comment":"Movies","enabled":true,"ro":false,"browsable":true},
			{"id":2,"path":"/mnt/tank/archive","name":"archive","enabled":false,"readonly":true}]`),
		[]byte(`[{"id":1,"path":"/mnt/tank/backup","comment":"","enabled":true,"ro":true,"networks":["10.0.0.0/24"],"hosts":[]},
			{"id":2,"paths":["/mnt/tank/a","/mnt/tank/b"],"comment":"legacy","enabled":true,"hosts":["10.0.0.5"]}]`))
	if err != nil || len(shares) != 4 {
		t.Fatalf("shares: %+v %v", shares, err)
	}
	if s := shares[0]; s.Protocol != "smb" || s.Name != "media" || s.Path != "/mnt/tank/media" || !s.Enabled || s.ReadOnly || s.Comment != "Movies" {
		t.Fatalf("smb: %+v", s)
	}
	if s := shares[1]; s.Enabled || !s.ReadOnly {
		t.Fatalf("smb archive: %+v", s)
	}
	if s := shares[2]; s.Protocol != "nfs" || s.Name != "/mnt/tank/backup" || !s.ReadOnly || len(s.Networks) != 1 || s.Hosts != nil {
		t.Fatalf("nfs: %+v", s)
	}
	if s := shares[3]; s.Path != "/mnt/tank/a, /mnt/tank/b" || s.Name != "legacy" || s.Hosts[0] != "10.0.0.5" {
		t.Fatalf("nfs core: %+v", s)
	}
	if _, err := parseShares([]byte(`[]`), []byte(`x`)); err == nil {
		t.Fatal("accepted a broken NFS list")
	}
}

func TestURLAndErrors(t *testing.T) {
	if u, err := secureURL("https://truenas.example.org/"); err != nil || u != "https://truenas.example.org" {
		t.Fatal(u, err)
	}
	if _, err := secureURL("http://truenas.example.org"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatal("accepted plain http", err)
	}
	if !strings.Contains(httpError(401).Error(), "API key") || !strings.Contains(httpError(403).Error(), "Read-Only Administrator") {
		t.Fatal("messages")
	}
	if httpError(502).Error() != "TrueNAS answered HTTP 502" {
		t.Fatal(httpError(502))
	}
}

// TestDemoData parses the recorded demo answers (for screenshots once the
// fake API can serve https, which the plugin insists on).
func TestDemoData(t *testing.T) {
	get := func(p string) []byte {
		t.Helper()
		body, err := fakeapi.Body("../testdata/fake", "GET", "/api/v2.0"+p)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	s, err := parseSystem(get("/system/info"))
	if err != nil {
		t.Fatal(err)
	}
	op := graphOps(s, "https://127.0.0.1:8443").Ops[0]
	if op.ID != "truenas-demo-a1-30512" || op.Attrs["os"] != "TrueNAS 25.04.1" || op.Attrs["ram"] != 64.0 || op.Attrs["ip"] != nil {
		t.Fatalf("server: %+v", op)
	}
	pools, err := parsePools(get("/pool"))
	if err != nil || len(pools) != 2 || !pools[0].Healthy || pools[0].UsedPercent != 41.1 {
		t.Fatalf("pools: %+v %v", pools, err)
	}
	shares, err := parseShares(get("/sharing/smb"), get("/sharing/nfs"))
	if err != nil || len(shares) != 5 || shares[3].Name != "Media library for jellyfin" || !shares[3].ReadOnly {
		t.Fatalf("shares: %+v %v", shares, err)
	}
}
