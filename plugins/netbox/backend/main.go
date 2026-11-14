//go:build wasip1

// Backend of the NetBox plugin: imports locations, racks and devices every
// six hours (and on netbox.sync); search and prefixes for AI clients. Read-only.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
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

type nb struct {
	base string
	h    map[string]string
}

func connect() (*nb, error) {
	raw, err := integ.Setting("url", "e.g. https://netbox.example.org")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	token, err := integ.Setting("token", "a read-only NetBox API token")
	if err != nil {
		return nil, err
	}
	return &nb{base: base, h: map[string]string{"Authorization": "Token " + token, "Accept": "application/json"}}, nil
}

// list reads up to 5000 objects, 500 per page.
func list[T any](n *nb, path string) ([]T, error) {
	var out []T
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	for offset := 0; offset < 5000; offset += 500 {
		var p Page[T]
		if err := integ.GetJSON("NetBox", fmt.Sprintf("%s/api/%s%slimit=500&offset=%d", n.base, path, sep, offset), n.h, &p); err != nil {
			return nil, err
		}
		out = append(out, p.Results...)
		if p.Next == nil || len(p.Results) == 0 {
			break
		}
	}
	return out, nil
}

func sync() (map[string]int, error) {
	n, err := connect()
	if err != nil {
		return nil, err
	}
	locations, err := list[Location](n, "dcim/locations/")
	if err != nil {
		return nil, err
	}
	racks, err := list[Rack](n, "dcim/racks/")
	if err != nil {
		return nil, err
	}
	dts, err := list[DeviceType](n, "dcim/device-types/")
	if err != nil {
		return nil, err
	}
	types := map[int]DeviceType{}
	for _, t := range dts {
		types[t.ID] = t
	}
	devices, err := list[Device](n, "dcim/devices/")
	if err != nil {
		return nil, err
	}
	return map[string]int{"locations": len(locations), "racks": len(racks), "devices": len(devices)}, integ.Apply(graphOps(locations, racks, types, devices))
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "netbox.sync":
		return sync()
	case "netbox.find":
		var in struct {
			Q string `json:"q"`
		}
		_ = json.Unmarshal(tc.Args, &in)
		if strings.TrimSpace(in.Q) == "" {
			return nil, fmt.Errorf("q is required")
		}
		n, err := connect()
		if err != nil {
			return nil, err
		}
		var p Page[Device]
		if err := integ.GetJSON("NetBox", n.base+"/api/dcim/devices/?limit=50&q="+url.QueryEscape(in.Q), n.h, &p); err != nil {
			return nil, err
		}
		return found(p.Results), nil
	case "netbox.prefixes":
		n, err := connect()
		if err != nil {
			return nil, err
		}
		l, err := list[Prefix](n, "ipam/prefixes/")
		if err != nil {
			return nil, err
		}
		return prefixInfo(l), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
