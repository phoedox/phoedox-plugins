//go:build wasip1

// Backend of the Portainer plugin: imports environments, stacks and
// containers every hour (and on portainer.sync) and answers
// portainer.containers for AI clients. Read-only.
package main

import (
	"fmt"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport on_event
func onEvent() int32 {
	return pdk.HandleEvent(func(ev pdk.Event) error {
		if ev.Type != "schedule" {
			return nil
		}
		_, err := sync()
		return err
	})
}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

func load() ([]Endpoint, map[int][]Container, []Stack, error) {
	raw, err := integ.Setting("url", "e.g. https://portainer.example.org:9443")
	if err != nil {
		return nil, nil, nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, nil, nil, err
	}
	token, err := integ.Setting("token", "a Portainer access token")
	if err != nil {
		return nil, nil, nil, err
	}
	h := map[string]string{"X-API-Key": token, "Accept": "application/json"}
	var endpoints []Endpoint
	if err := integ.GetJSON("Portainer", base+"/api/endpoints", h, &endpoints); err != nil {
		return nil, nil, nil, err
	}
	containers := map[int][]Container{}
	for _, e := range endpoints {
		if !e.docker() || e.Status != 1 {
			continue
		}
		var list []Container
		if err := integ.GetJSON("Portainer", fmt.Sprintf("%s/api/endpoints/%d/docker/containers/json?all=1", base, e.ID), h, &list); err != nil {
			pdk.Warn(fmt.Sprintf("environment %s: %v", e.Name, err))
			continue
		}
		containers[e.ID] = list
	}
	var stacks []Stack
	if err := integ.GetJSON("Portainer", base+"/api/stacks", h, &stacks); err != nil {
		pdk.Warn("stacks: " + err.Error())
	}
	return endpoints, containers, stacks, nil
}

func sync() (map[string]int, error) {
	endpoints, containers, stacks, err := load()
	if err != nil {
		return nil, err
	}
	n := 0
	for _, list := range containers {
		n += len(list)
	}
	return map[string]int{"environments": len(endpoints), "containers": n, "stacks": len(stacks)}, integ.Apply(graphOps(endpoints, containers, stacks))
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "portainer.sync":
		return sync()
	case "portainer.containers":
		endpoints, containers, _, err := load()
		if err != nil {
			return nil, err
		}
		return statusOf(endpoints, containers), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
