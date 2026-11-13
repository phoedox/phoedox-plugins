// Logic of the Proxmox plugin, without build tags so it is testable natively.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// Resource is one entry of GET /api2/json/cluster/resources.
type Resource struct {
	Type    string  `json:"type"` // node | qemu | lxc | storage | …
	Node    string  `json:"node"`
	VMID    int     `json:"vmid"`
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	MaxCPU  float64 `json:"maxcpu"`
	MaxMem  float64 `json:"maxmem"`
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	Uptime  int64   `json:"uptime"`
	Tags    string  `json:"tags"`
	Pool    string  `json:"pool"`
	Hastate string  `json:"hastate"`
}

func parseResources(body []byte) ([]Resource, error) {
	var r struct {
		Data []Resource `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("unexpected answer from Proxmox: %w", err)
	}
	return r.Data, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	return s
}

func gib(b float64) float64 { return math.Round(b/(1<<30)*10) / 10 }

// graphOps turns nodes into servers and guests into VMs/LXC that run on them.
func graphOps(list []Resource) pdk.GraphOps {
	var ops pdk.GraphOps
	nodes := map[string]string{}
	node := func(n string) string {
		if ref, ok := nodes[n]; ok {
			return ref
		}
		ref := ops.UpsertNamed("server", "pve-"+slugify(n), n, map[string]any{"hostname": n, "os": "Proxmox VE"})
		nodes[n] = ref
		return ref
	}
	for _, r := range list {
		if r.Type == "node" {
			ref := node(r.Node)
			ops.Ops[len(ops.Ops)-1].Attrs["ram"] = gib(r.MaxMem)
			_ = ref
		}
	}
	for _, r := range list {
		var kind, idAttr string
		switch r.Type {
		case "qemu":
			kind, idAttr = "vm", "vmid"
		case "lxc":
			kind, idAttr = "lxc", "ctid"
		default:
			continue
		}
		name := r.Name
		if name == "" {
			name = strconv.Itoa(r.VMID)
		}
		attrs := map[string]any{idAttr: r.VMID, "status": r.Status}
		if kind == "vm" {
			attrs["vcpu"], attrs["ram"] = r.MaxCPU, gib(r.MaxMem)
		}
		if r.Tags != "" {
			attrs["tags"] = strings.ReplaceAll(r.Tags, ";", ", ")
		}
		guest := ops.UpsertNamed(kind, fmt.Sprintf("pve-%d-%s", r.VMID, slugify(name)), name, attrs)
		ops.Link(guest, "runs_on", node(r.Node), nil)
	}
	return ops
}

// Status is a line of pve.status.
type Status struct {
	Type   string  `json:"type"`
	Name   string  `json:"name"`
	Node   string  `json:"node"`
	VMID   int     `json:"vmid,omitempty"`
	Status string  `json:"status"`
	CPU    float64 `json:"cpuPercent"`
	MemGiB float64 `json:"memGiB"`
	MaxGiB float64 `json:"maxMemGiB"`
	Uptime int64   `json:"uptimeSeconds"`
}

func statusOf(list []Resource) []Status {
	out := []Status{}
	for _, r := range list {
		if r.Type != "node" && r.Type != "qemu" && r.Type != "lxc" {
			continue
		}
		name := r.Name
		if r.Type == "node" {
			name = r.Node
		}
		out = append(out, Status{Type: r.Type, Name: name, Node: r.Node, VMID: r.VMID, Status: r.Status,
			CPU: math.Round(r.CPU*1000) / 10, MemGiB: gib(r.Mem), MaxGiB: gib(r.MaxMem), Uptime: r.Uptime})
	}
	return out
}
