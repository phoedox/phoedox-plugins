// Logic of the Synology plugin, without build tags so it is testable natively.
// DSM answers {"success":bool,"data":…,"error":{"code":n}}; the fields inside
// data differ between DSM versions, so parsing is tolerant.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const session = "Phoedox"

func loginForm(user, pass string) string {
	return url.Values{"api": {"SYNO.API.Auth"}, "version": {"6"}, "method": {"login"}, "account": {user}, "passwd": {pass},
		"session": {session}, "format": {"sid"}}.Encode()
}

func logoutForm(sid string) string {
	return url.Values{"api": {"SYNO.API.Auth"}, "version": {"6"}, "method": {"logout"}, "session": {session}, "_sid": {sid}}.Encode()
}

func callForm(api string, version int, method, sid string) string {
	return url.Values{"api": {api}, "version": {strconv.Itoa(version)}, "method": {method}, "_sid": {sid}}.Encode()
}

// authErrors are the codes of SYNO.API.Auth.
var authErrors = map[int]string{
	400: "the Synology NAS rejected the username or password",
	401: "the DSM account is disabled",
	402: "the DSM account has no permission to sign in",
	403: "the DSM account uses 2-step verification; use a dedicated account without it",
	404: "2-step verification failed; use a dedicated account without 2-step verification",
	406: "DSM enforces 2-step verification for this account; use a dedicated account without it",
	407: "DSM blocked the IP address of the Phoedox server after failed sign-ins; remove it under Control Panel > Security > Protection",
	408: "the password of the DSM account has expired",
	409: "the password of the DSM account has expired and must be changed",
	410: "the password of the DSM account must be changed",
}

// commonErrors are the codes all DSM APIs share.
var commonErrors = map[int]string{
	102: "this DSM version does not offer the requested API",
	103: "this DSM version does not offer the requested method",
	104: "this DSM version does not support the requested API version",
	105: "the DSM account lacks permission; it must be in the administrators group",
	106: "the DSM session timed out",
	107: "the DSM session was interrupted by a duplicate sign-in",
	119: "the DSM session is not valid",
}

// dsmError turns a DSM error code into a clear message; codes 400 and above
// mean something else outside SYNO.API.Auth.
func dsmError(code int, auth bool) error {
	if msg, ok := authErrors[code]; ok && auth {
		return fmt.Errorf("%s (DSM error %d)", msg, code)
	}
	if msg, ok := commonErrors[code]; ok {
		return fmt.Errorf("%s (DSM error %d)", msg, code)
	}
	return fmt.Errorf("the Synology NAS answered with DSM error %d", code)
}

// unwrap checks the envelope and returns data.
func unwrap(body []byte, auth bool) (json.RawMessage, error) {
	var r struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("unexpected answer from Synology DSM (is the url the DSM address?): %w", err)
	}
	if !r.Success {
		code := 100
		if r.Error != nil {
			code = r.Error.Code
		}
		return nil, dsmError(code, auth)
	}
	return r.Data, nil
}

func parseSID(data json.RawMessage) (string, error) {
	var d struct {
		SID string `json:"sid"`
	}
	_ = json.Unmarshal(data, &d)
	if d.SID == "" {
		return "", fmt.Errorf("the Synology NAS sent no session")
	}
	return d.SID, nil
}

// num reads a number that DSM sends as number or string.
func num(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	}
	return 0
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}

// NAS is what the plugin imports.
type NAS struct {
	Hostname, Model, Serial, Version string
	RAMMB                            float64
}

// parseSystem reads SYNO.Core.System info.
func parseSystem(data json.RawMessage) NAS {
	var d map[string]any
	_ = json.Unmarshal(data, &d)
	return NAS{Model: str(d["model"]), Serial: str(d["serial"]), Version: str(d["firmware_ver"]), RAMMB: num(d["ram_size"])}
}

// parseDSMInfo reads SYNO.DSM.Info getinfo (the fallback).
func parseDSMInfo(data json.RawMessage) NAS {
	var d map[string]any
	_ = json.Unmarshal(data, &d)
	v := str(d["version_string"])
	if v == "" {
		v = str(d["version"])
	}
	return NAS{Model: str(d["model"]), Serial: str(d["serial"]), Version: v, RAMMB: num(d["ram"])}
}

// parseHostname reads SYNO.Core.Network get.
func parseHostname(data json.RawMessage) string {
	var d map[string]any
	_ = json.Unmarshal(data, &d)
	if h := str(d["server_name"]); h != "" {
		return h
	}
	return str(d["hostname"])
}

// osName turns "DSM 7.2.1-69057 Update 5" or "7.2.1-69057" into "DSM …".
func osName(v string) string {
	if v == "" || strings.HasPrefix(strings.ToUpper(v), "DSM") {
		return v
	}
	return "DSM " + v
}

// graphOps: the NAS as server, keyed by its serial number.
func graphOps(n NAS, base string) pdk.GraphOps {
	var ops pdk.GraphOps
	attrs := map[string]any{"manufacturer": "Synology"}
	name := n.Hostname
	if n.Hostname != "" {
		attrs["hostname"] = n.Hostname
	}
	if n.Model != "" {
		attrs["model"] = n.Model
		if name == "" {
			name = "Synology " + n.Model
		}
	}
	if name == "" {
		name = "Synology NAS"
	}
	if os := osName(n.Version); os != "" {
		attrs["os"] = os
	}
	if n.Serial != "" {
		attrs["serial"] = n.Serial
	}
	if n.RAMMB > 0 {
		attrs["ram"] = math.Round(n.RAMMB/1024*10) / 10
	}
	if u, err := url.Parse(base); err == nil {
		if ip, err := netip.ParseAddr(u.Hostname()); err == nil {
			attrs["ip"] = ip.String()
		}
	}
	key := n.Serial
	if key == "" {
		key = n.Model + " " + n.Hostname
	}
	if strings.TrimSpace(key) == "" {
		key = "nas"
	}
	ops.UpsertNamed("server", "synology-"+integ.Slugify(key), name, attrs)
	return ops
}

// Storage is the answer of synology.storage.
type Storage struct {
	Pools   []Pool   `json:"pools"`
	Volumes []Volume `json:"volumes"`
	Disks   []Disk   `json:"disks"`
}

// Pool is a storage pool.
type Pool struct {
	ID       string   `json:"id"`
	Status   string   `json:"status,omitempty"`
	RAID     string   `json:"raid,omitempty"`
	TotalGiB float64  `json:"total_gib,omitempty"`
	UsedGiB  float64  `json:"used_gib,omitempty"`
	Disks    []string `json:"disks,omitempty"`
}

// Volume is a volume with its usage.
type Volume struct {
	ID          string  `json:"id"`
	Name        string  `json:"name,omitempty"`
	Path        string  `json:"path,omitempty"`
	Status      string  `json:"status,omitempty"`
	FS          string  `json:"fs_type,omitempty"`
	Pool        string  `json:"pool,omitempty"`
	TotalGiB    float64 `json:"total_gib,omitempty"`
	UsedGiB     float64 `json:"used_gib,omitempty"`
	FreeGiB     float64 `json:"free_gib,omitempty"`
	UsedPercent float64 `json:"used_percent,omitempty"`
}

// Disk is a drive.
type Disk struct {
	ID      string  `json:"id"`
	Name    string  `json:"name,omitempty"`
	Vendor  string  `json:"vendor,omitempty"`
	Model   string  `json:"model,omitempty"`
	Type    string  `json:"type,omitempty"`
	SizeGiB float64 `json:"size_gib,omitempty"`
	Status  string  `json:"status,omitempty"`
	SMART   string  `json:"smart_status,omitempty"`
	TempC   float64 `json:"temperature_c,omitempty"`
	Pool    string  `json:"pool,omitempty"`
}

type size struct {
	Total any `json:"total"`
	Used  any `json:"used"`
}

// parseStorage reads SYNO.Storage.CGI.Storage load_info.
func parseStorage(data json.RawMessage) (Storage, error) {
	var d struct {
		Pools []struct {
			ID         string   `json:"id"`
			Status     string   `json:"status"`
			DeviceType string   `json:"device_type"`
			RAIDType   string   `json:"raidType"`
			Size       size     `json:"size"`
			Disks      []string `json:"disks"`
		} `json:"storagePools"`
		Volumes []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			Desc        string `json:"desc"`
			Path        string `json:"vol_path"`
			Status      string `json:"status"`
			FS          string `json:"fs_type"`
			Pool        string `json:"pool_path"`
			Size        size   `json:"size"`
		} `json:"volumes"`
		Disks []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			LongName    string `json:"longName"`
			Vendor      string `json:"vendor"`
			Model       string `json:"model"`
			DiskType    string `json:"diskType"`
			SizeTotal   any    `json:"size_total"`
			Status      string `json:"status"`
			SmartStatus string `json:"smart_status"`
			Temp        any    `json:"temp"`
			UsedBy      string `json:"used_by"`
		} `json:"disks"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return Storage{}, fmt.Errorf("unexpected storage answer from Synology DSM: %w", err)
	}
	out := Storage{Pools: []Pool{}, Volumes: []Volume{}, Disks: []Disk{}}
	for _, p := range d.Pools {
		raid := p.DeviceType
		if raid == "" {
			raid = p.RAIDType
		}
		out.Pools = append(out.Pools, Pool{ID: p.ID, Status: p.Status, RAID: raid, TotalGiB: integ.GiB(num(p.Size.Total)), UsedGiB: integ.GiB(num(p.Size.Used)), Disks: p.Disks})
	}
	for _, v := range d.Volumes {
		name := v.DisplayName
		if name == "" {
			name = v.Desc
		}
		vol := Volume{ID: v.ID, Name: name, Path: v.Path, Status: v.Status, FS: v.FS, Pool: v.Pool}
		total, used := num(v.Size.Total), num(v.Size.Used)
		if total > 0 {
			vol.TotalGiB, vol.UsedGiB, vol.FreeGiB = integ.GiB(total), integ.GiB(used), integ.GiB(total-used)
			vol.UsedPercent = math.Round(used/total*1000) / 10
		}
		out.Volumes = append(out.Volumes, vol)
	}
	for _, k := range d.Disks {
		name := k.LongName
		if name == "" {
			name = k.Name
		}
		out.Disks = append(out.Disks, Disk{ID: k.ID, Name: name, Vendor: k.Vendor, Model: k.Model, Type: k.DiskType, SizeGiB: integ.GiB(num(k.SizeTotal)),
			Status: k.Status, SMART: k.SmartStatus, TempC: num(k.Temp), Pool: k.UsedBy})
	}
	return out, nil
}
