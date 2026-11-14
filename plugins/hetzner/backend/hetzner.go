// Logic of the Hetzner Cloud plugin, without build tags so it is testable
// natively.
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// Server is a server of GET /v1/servers.
type Server struct {
	ID        int64             `json:"id"`
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Created   string            `json:"created"`
	Labels    map[string]string `json:"labels"`
	PublicNet struct {
		IPv4 *struct {
			IP string `json:"ip"`
		} `json:"ipv4"`
		IPv6 *struct {
			IP string `json:"ip"`
		} `json:"ipv6"`
	} `json:"public_net"`
	PrivateNet []struct {
		IP string `json:"ip"`
	} `json:"private_net"`
	ServerType struct {
		Name         string  `json:"name"`
		Cores        int     `json:"cores"`
		Memory       float64 `json:"memory"`
		Disk         float64 `json:"disk"`
		Architecture string  `json:"architecture"`
	} `json:"server_type"`
	Datacenter struct {
		Name     string `json:"name"`
		Location struct {
			Name        string `json:"name"`
			City        string `json:"city"`
			NetworkZone string `json:"network_zone"`
		} `json:"location"`
	} `json:"datacenter"`
	Image *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		OSFlavor    string `json:"os_flavor"`
		OSVersion   string `json:"os_version"`
	} `json:"image"`
}

func (s Server) ipv4() string {
	if s.PublicNet.IPv4 != nil {
		return s.PublicNet.IPv4.IP
	}
	return ""
}

func (s Server) ipv6() string {
	if s.PublicNet.IPv6 != nil {
		return s.PublicNet.IPv6.IP
	}
	return ""
}

func (s Server) privateIPs() []string {
	var out []string
	for _, n := range s.PrivateNet {
		if n.IP != "" {
			out = append(out, n.IP)
		}
	}
	return out
}

func (s Server) os() string {
	if s.Image == nil {
		return "" // the image was deleted
	}
	if s.Image.Description != "" {
		return s.Image.Description
	}
	return strings.TrimSpace(s.Image.OSFlavor + " " + s.Image.OSVersion)
}

func (s Server) labels() string {
	keys := make([]string, 0, len(s.Labels))
	for k := range s.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k
		if v := s.Labels[k]; v != "" {
			parts[i] += "=" + v
		}
	}
	return strings.Join(parts, ", ")
}

// parseServers reads a page and returns the next page number (0 when done).
func parseServers(body []byte) ([]Server, int, error) {
	var r struct {
		Servers []Server `json:"servers"`
		Meta    struct {
			Pagination struct {
				NextPage *int `json:"next_page"`
			} `json:"pagination"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, 0, err
	}
	next := 0
	if r.Meta.Pagination.NextPage != nil {
		next = *r.Meta.Pagination.NextPage
	}
	return r.Servers, next, nil
}

// apiError reads the error code of a Hetzner error answer.
func apiError(status int, body []byte) error {
	var r struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &r)
	if r.Error.Code != "" {
		return fmt.Errorf("Hetzner Cloud answered HTTP %d (%s)", status, r.Error.Code)
	}
	return fmt.Errorf("Hetzner Cloud answered HTTP %d", status)
}

// graphOps: every server as a VM, keyed by its server ID.
func graphOps(servers []Server) pdk.GraphOps {
	var ops pdk.GraphOps
	for _, s := range servers {
		if s.ID == 0 {
			continue
		}
		attrs := map[string]any{"hetzner_id": s.ID}
		set := func(k, v string) {
			if v != "" {
				attrs[k] = v
			}
		}
		set("ip", s.ipv4())
		set("ipv6", s.ipv6())
		set("private_ip", strings.Join(s.privateIPs(), ", "))
		set("os", s.os())
		set("status", s.Status)
		set("server_type", s.ServerType.Name)
		set("datacenter", s.Datacenter.Name)
		set("location", s.Datacenter.Location.Name)
		set("labels", s.labels())
		if s.ServerType.Cores > 0 {
			attrs["vcpu"] = s.ServerType.Cores
		}
		if s.ServerType.Memory > 0 {
			attrs["ram"] = s.ServerType.Memory
		}
		if s.ServerType.Disk > 0 {
			attrs["disk"] = s.ServerType.Disk
		}
		name := s.Name
		if name == "" {
			name = fmt.Sprintf("Hetzner server %d", s.ID)
		}
		ops.UpsertNamed("vm", fmt.Sprintf("hetzner-%d", s.ID), name, attrs)
	}
	return ops
}

// ServerInfo is a server for AI clients.
type ServerInfo struct {
	ID         int64             `json:"id"`
	Name       string            `json:"name"`
	Status     string            `json:"status"`
	ServerType string            `json:"server_type"`
	Location   string            `json:"location"`
	Image      string            `json:"image,omitempty"`
	IPv4       string            `json:"ipv4,omitempty"`
	IPv6       string            `json:"ipv6,omitempty"`
	PrivateIPs []string          `json:"private_ips,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Created    string            `json:"created,omitempty"`
}

func serverInfo(servers []Server) []ServerInfo {
	out := make([]ServerInfo, 0, len(servers))
	for _, s := range servers {
		out = append(out, ServerInfo{ID: s.ID, Name: s.Name, Status: s.Status, ServerType: s.ServerType.Name, Location: s.Datacenter.Location.Name,
			Image: s.os(), IPv4: s.ipv4(), IPv6: s.ipv6(), PrivateIPs: s.privateIPs(), Labels: s.Labels, Created: s.Created})
	}
	return out
}
