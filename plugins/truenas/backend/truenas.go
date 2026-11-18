// Logic of the TrueNAS plugin, without build tags so it is testable natively.
// Fields differ between TrueNAS CORE and SCALE releases, so parsing is
// tolerant (numbers may come as strings, fields may be missing).
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

// secureURL checks the address and insists on https: TrueNAS revokes an API
// key that is sent over plain HTTP.
func secureURL(raw string) (string, error) {
	base, err := integ.BaseURL(raw)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(base, "https://") {
		return "", fmt.Errorf("the setting url must start with https:// (TrueNAS revokes API keys sent over plain HTTP)")
	}
	return base, nil
}

// httpError turns a non-200 answer into a clear message.
func httpError(status int) error {
	switch status {
	case 401:
		return fmt.Errorf("TrueNAS rejected the API key (wrong, expired or revoked)")
	case 403:
		return fmt.Errorf("the user of the API key may not read this (it needs the Read-Only Administrator role)")
	case 404:
		return fmt.Errorf("TrueNAS has no REST API v2.0 at this address")
	}
	return &integ.StatusError{Service: "TrueNAS", Status: status}
}

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

func flag(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return integ.Truthy(x)
	}
	return false
}

// placeholder tells whether a DMI value is a vendor placeholder.
func placeholder(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "to be filled by o.e.m.", "default string", "system serial number", "system product name", "system manufacturer", "not specified",
		"not applicable", "none", "n/a", "0", "0123456789", "123456789", "unknown":
		return true
	}
	return false
}

// System is what /system/info tells.
type System struct {
	Hostname, Version, Product, Manufacturer, CPU, Serial string
	RAM                                                   float64 // bytes
	Cores                                                 int
}

func parseSystem(body []byte) (System, error) {
	var d map[string]any
	if err := json.Unmarshal(body, &d); err != nil {
		return System{}, fmt.Errorf("unexpected answer from TrueNAS: %w", err)
	}
	s := System{Hostname: str(d["hostname"]), Version: str(d["version"]), CPU: str(d["model"]), RAM: num(d["physmem"]), Cores: int(num(d["cores"]))}
	if v := str(d["system_product"]); !placeholder(v) {
		s.Product = v
	}
	if v := str(d["system_manufacturer"]); !placeholder(v) {
		s.Manufacturer = v
	}
	if v := str(d["system_serial"]); !placeholder(v) {
		s.Serial = v
	}
	return s, nil
}

// osName turns "TrueNAS-SCALE-24.04.2", "TrueNAS-13.0-U6.1" or "25.04.0"
// into "TrueNAS SCALE 24.04.2", "TrueNAS 13.0-U6.1", "TrueNAS 25.04.0".
func osName(v string) string {
	if v == "" {
		return ""
	}
	rest, ok := strings.CutPrefix(v, "TrueNAS-")
	if !ok {
		rest, _ = strings.CutPrefix(v, "TrueNAS ")
	}
	for _, ed := range []string{"SCALE", "CORE", "ENTERPRISE"} {
		if r, ok := strings.CutPrefix(rest, ed+"-"); ok {
			return "TrueNAS " + ed + " " + r
		}
	}
	return "TrueNAS " + rest
}

// graphOps: the system as server, keyed by its serial number (else hostname).
func graphOps(s System, base string) pdk.GraphOps {
	var ops pdk.GraphOps
	attrs := map[string]any{}
	if s.Hostname != "" {
		attrs["hostname"] = s.Hostname
	}
	if os := osName(s.Version); os != "" {
		attrs["os"] = os
	}
	if s.Product != "" {
		attrs["model"] = s.Product
	}
	if s.Manufacturer != "" {
		attrs["manufacturer"] = s.Manufacturer
	}
	if s.Serial != "" {
		attrs["serial"] = s.Serial
	}
	if s.CPU != "" {
		attrs["cpu"] = s.CPU
	}
	if s.Cores > 0 {
		attrs["cores"] = s.Cores
	}
	if s.RAM > 0 {
		attrs["ram"] = integ.GiB(s.RAM)
	}
	if u, err := url.Parse(base); err == nil {
		// A loopback address (a tunnel) is not the address of the system.
		if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !ip.IsLoopback() {
			attrs["ip"] = ip.String()
		}
	}
	name, key := s.Hostname, s.Serial
	if name == "" {
		name = "TrueNAS"
	}
	if key == "" {
		key = name
	}
	ops.UpsertNamed("server", "truenas-"+integ.Slugify(key), name, attrs)
	return ops
}

// Pool is a ZFS pool.
type Pool struct {
	Name         string  `json:"name"`
	Status       string  `json:"status,omitempty"`
	Healthy      bool    `json:"healthy"`
	Path         string  `json:"path,omitempty"`
	SizeGiB      float64 `json:"size_gib,omitempty"`
	AllocatedGiB float64 `json:"allocated_gib,omitempty"`
	FreeGiB      float64 `json:"free_gib,omitempty"`
	UsedPercent  float64 `json:"used_percent,omitempty"`
}

func parsePools(body []byte) ([]Pool, error) {
	var l []map[string]any
	if err := json.Unmarshal(body, &l); err != nil {
		return nil, fmt.Errorf("unexpected pool list from TrueNAS: %w", err)
	}
	out := make([]Pool, 0, len(l))
	for _, d := range l {
		p := Pool{Name: str(d["name"]), Status: str(d["status"]), Healthy: flag(d["healthy"]), Path: str(d["path"])}
		size, alloc, free := num(d["size"]), num(d["allocated"]), num(d["free"])
		if size > 0 {
			p.SizeGiB, p.AllocatedGiB, p.FreeGiB = integ.GiB(size), integ.GiB(alloc), integ.GiB(free)
			p.UsedPercent = math.Round(alloc/size*1000) / 10
		}
		out = append(out, p)
	}
	return out, nil
}

// Share is an SMB or NFS share.
type Share struct {
	Protocol string   `json:"protocol"` // smb | nfs
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Enabled  bool     `json:"enabled"`
	ReadOnly bool     `json:"read_only"`
	Comment  string   `json:"comment,omitempty"`
	Networks []string `json:"networks,omitempty"`
	Hosts    []string `json:"hosts,omitempty"`
}

func strs(v any) []string {
	l, _ := v.([]any)
	var out []string
	for _, x := range l {
		if s := str(x); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseShares(smb, nfs []byte) ([]Share, error) {
	var sl, nl []map[string]any
	if err := json.Unmarshal(smb, &sl); err != nil {
		return nil, fmt.Errorf("unexpected SMB share list from TrueNAS: %w", err)
	}
	if err := json.Unmarshal(nfs, &nl); err != nil {
		return nil, fmt.Errorf("unexpected NFS share list from TrueNAS: %w", err)
	}
	out := make([]Share, 0, len(sl)+len(nl))
	readOnly := func(d map[string]any) bool { return flag(d["ro"]) || flag(d["readonly"]) }
	for _, d := range sl {
		out = append(out, Share{Protocol: "smb", Name: str(d["name"]), Path: str(d["path"]), Enabled: flag(d["enabled"]), ReadOnly: readOnly(d), Comment: str(d["comment"])})
	}
	for _, d := range nl {
		path := str(d["path"])
		if path == "" { // CORE: several paths per export
			path = strings.Join(strs(d["paths"]), ", ")
		}
		name := str(d["comment"])
		if name == "" {
			name = path
		}
		out = append(out, Share{Protocol: "nfs", Name: name, Path: path, Enabled: flag(d["enabled"]), ReadOnly: readOnly(d), Comment: str(d["comment"]),
			Networks: strs(d["networks"]), Hosts: strs(d["hosts"])})
	}
	return out, nil
}
