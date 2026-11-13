//go:build wasip1

// Backend of the Home Assistant plugin: imports areas and devices every six
// hours (and on ha.sync) and answers ha.state for AI clients.
package main

import (
	"encoding/json"
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

func settings() (base, token string, err error) {
	if base, err = pdk.Secret("url"); err != nil {
		return "", "", fmt.Errorf("set the setting url (e.g. https://ha.example.org): %w", err)
	}
	if token, err = pdk.Secret("token"); err != nil {
		return "", "", fmt.Errorf("set the setting token (a long-lived access token): %w", err)
	}
	return strings.TrimRight(base, "/"), token, nil
}

func call(method, path, body string) ([]byte, error) {
	base, token, err := settings()
	if err != nil {
		return nil, err
	}
	res, err := pdk.Fetch(pdk.FetchRequest{Method: method, URL: base + path, Body: body,
		Headers: map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"}})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, fmt.Errorf("Home Assistant answered HTTP %d", res.Status)
	}
	return []byte(res.Body), nil
}

func sync() (map[string]int, error) {
	tpl, _ := json.Marshal(map[string]string{"template": registryTemplate})
	body, err := call("POST", "/api/template", string(tpl))
	if err != nil {
		return nil, err
	}
	devices, err := parseDevices(body)
	if err != nil {
		return nil, err
	}
	ops := graphOps(devices)
	if len(ops.Ops) > 0 {
		if _, err := pdk.Graph.Apply(&ops); err != nil {
			return nil, err
		}
	}
	return map[string]int{"devices": len(devices)}, nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "ha.sync":
		return sync()
	case "ha.state":
		var args struct {
			Entities []string `json:"entities"`
		}
		_ = json.Unmarshal(tc.Args, &args)
		list, err := validEntities(args.Entities)
		if err != nil {
			return nil, err
		}
		out := []State{}
		for _, e := range list {
			body, err := call("GET", "/api/states/"+e, "")
			var s State
			if err == nil {
				err = json.Unmarshal(body, &s)
			}
			if err != nil {
				s = State{EntityID: e, Error: err.Error()}
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
