// Logic of the Portainer plugin, without build tags so it is testable natively.
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// Endpoint is a Portainer environment (GET /api/endpoints).
type Endpoint struct {
	ID     int    `json:"Id"`
	Name   string `json:"Name"`
	Type   int    `json:"Type"`   // 1 Docker, 2 agent, 4 edge agent, 5 Kubernetes …
	Status int    `json:"Status"` // 1 up, 2 down
}

// Docker environments (Kubernetes ones have no Docker API).
func (e Endpoint) docker() bool { return e.Type == 1 || e.Type == 2 || e.Type == 4 }

// Port is a port of a container.
type Port struct {
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

// Container is one entry of /docker/containers/json.
type Container struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
	Ports  []Port            `json:"Ports"`
}

func (c Container) name() string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	if len(c.ID) > 12 {
		return c.ID[:12]
	}
	return c.ID
}

// project is the Compose project (stack) a container belongs to.
func (c Container) project() string { return c.Labels["com.docker.compose.project"] }

func (c Container) ports() string {
	seen := map[string]bool{}
	var out []string
	for _, p := range c.Ports {
		if p.PublicPort == 0 {
			continue
		}
		s := fmt.Sprintf("%d:%d/%s", p.PublicPort, p.PrivatePort, p.Type)
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// Stack is a Portainer stack (GET /api/stacks).
type Stack struct {
	ID         int    `json:"Id"`
	Name       string `json:"Name"`
	EndpointID int    `json:"EndpointId"`
	Status     int    `json:"Status"` // 1 active, 2 inactive
}

// graphOps: environments become servers, stacks stacks that run on their
// server, containers run on their server and are part of their stack.
func graphOps(endpoints []Endpoint, containers map[int][]Container, stacks []Stack) pdk.GraphOps {
	var ops pdk.GraphOps
	servers := map[int]string{}
	names := map[int]string{}
	for _, e := range endpoints {
		if !e.docker() {
			continue
		}
		names[e.ID] = integ.Slugify(e.Name)
		servers[e.ID] = ops.UpsertNamed("server", "portainer-"+names[e.ID], e.Name, map[string]any{"hostname": e.Name})
	}
	stackRefs := map[string]string{}
	stack := func(endpoint int, name, status string) string {
		key := fmt.Sprintf("%d/%s", endpoint, name)
		if ref, ok := stackRefs[key]; ok {
			return ref
		}
		attrs := map[string]any{}
		if status != "" {
			attrs["status"] = status
		}
		ref := ops.UpsertNamed("stack", "portainer-"+names[endpoint]+"-"+integ.Slugify(name), name, attrs)
		// The stack runs on its environment, so Phoedox shows the host's
		// software as stacks with their containers.
		ops.Link(ref, "runs_on", servers[endpoint], nil)
		stackRefs[key] = ref
		return ref
	}
	for _, s := range stacks {
		if _, ok := servers[s.EndpointID]; !ok {
			continue
		}
		// The stack kind knows running and stopped (Portainer: 1 active, 2 inactive).
		status := "running"
		if s.Status == 2 {
			status = "stopped"
		}
		stack(s.EndpointID, s.Name, status)
	}
	ids := make([]int, 0, len(containers))
	for id := range containers {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		server, ok := servers[id]
		if !ok {
			continue
		}
		for _, c := range containers[id] {
			attrs := map[string]any{"image": c.Image, "status": c.State}
			if p := c.ports(); p != "" {
				attrs["ports"] = p
			}
			ref := ops.UpsertNamed("container", "portainer-"+names[id]+"-"+integ.Slugify(c.name()), c.name(), attrs)
			ops.Link(ref, "runs_on", server, nil)
			if p := c.project(); p != "" {
				ops.Link(ref, "part_of", stack(id, p, ""), nil)
			}
		}
	}
	return ops
}

// ContainerStatus is a line of portainer.containers.
type ContainerStatus struct {
	Environment string `json:"environment"`
	Name        string `json:"name"`
	Stack       string `json:"stack,omitempty"`
	Image       string `json:"image"`
	State       string `json:"state"`
	Status      string `json:"status"`
	Ports       string `json:"ports,omitempty"`
}

func statusOf(endpoints []Endpoint, containers map[int][]Container) []ContainerStatus {
	out := []ContainerStatus{}
	for _, e := range endpoints {
		for _, c := range containers[e.ID] {
			out = append(out, ContainerStatus{Environment: e.Name, Name: c.name(), Stack: c.project(), Image: c.Image, State: c.State, Status: c.Status, Ports: c.ports()})
		}
	}
	return out
}
