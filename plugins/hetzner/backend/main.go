//go:build wasip1

// Backend of the Hetzner Cloud plugin: imports the servers of a project
// every six hours (and on hetzner.sync) and lists them for AI clients.
// Read-only.
package main

import (
	"fmt"
	"strings"

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

// servers reads all servers, 50 per page (at most 2500).
func servers() ([]Server, error) {
	token, err := integ.Setting("token", "a read-only API token of the Hetzner Cloud project")
	if err != nil {
		return nil, err
	}
	h := map[string]string{"Authorization": "Bearer " + strings.TrimSpace(token), "Accept": "application/json"}
	var all []Server
	for page := 1; page > 0 && page <= 50; {
		res, err := pdk.Fetch(pdk.FetchRequest{URL: fmt.Sprintf("https://api.hetzner.cloud/v1/servers?page=%d&per_page=50", page), Headers: h})
		if err != nil {
			return nil, err
		}
		if res.Status != 200 {
			return nil, apiError(res.Status, []byte(res.Body))
		}
		list, next, err := parseServers([]byte(res.Body))
		if err != nil {
			return nil, fmt.Errorf("unexpected answer from Hetzner Cloud: %w", err)
		}
		all = append(all, list...)
		if next <= page {
			break
		}
		page = next
	}
	return all, nil
}

func sync() (map[string]int, error) {
	list, err := servers()
	if err != nil {
		return nil, err
	}
	err = integ.ApplyBatches(graphOps(list), 500, func(b *pdk.GraphOps) (map[string]string, error) {
		r, err := pdk.Graph.Apply(b)
		return r.Refs, err
	})
	return map[string]int{"servers": len(list)}, err
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "hetzner.sync":
		return sync()
	case "hetzner.servers":
		list, err := servers()
		if err != nil {
			return nil, err
		}
		return serverInfo(list), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
