//go:build wasip1

// Backend of the Proxmox VE plugin: imports nodes and guests every hour (and
// on pve.sync) and answers pve.status for AI clients. Read-only.
package main

import (
	"fmt"
	"strings"

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

func resources() ([]Resource, error) {
	base, err := pdk.Secret("url")
	if err != nil {
		return nil, fmt.Errorf("set the setting url (e.g. https://pve.example.org:8006): %w", err)
	}
	token, err := pdk.Secret("token")
	if err != nil {
		return nil, fmt.Errorf("set the setting token (USER@REALM!ID=SECRET): %w", err)
	}
	res, err := pdk.Fetch(pdk.FetchRequest{URL: strings.TrimRight(base, "/") + "/api2/json/cluster/resources",
		Headers: map[string]string{"Authorization": "PVEAPIToken=" + token}})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, fmt.Errorf("Proxmox answered HTTP %d", res.Status)
	}
	return parseResources([]byte(res.Body))
}

func sync() (map[string]int, error) {
	list, err := resources()
	if err != nil {
		return nil, err
	}
	ops := graphOps(list)
	if len(ops.Ops) > 0 {
		if _, err := pdk.Graph.Apply(&ops); err != nil {
			return nil, err
		}
	}
	return map[string]int{"resources": len(list)}, nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "pve.sync":
		return sync()
	case "pve.status":
		list, err := resources()
		if err != nil {
			return nil, err
		}
		return statusOf(list), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
