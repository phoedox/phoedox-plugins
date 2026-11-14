//go:build wasip1

// Backend of the Tailscale plugin: imports the devices of a tailnet every six
// hours (and on tailscale.sync) and links them to known hosts; device list for
// AI clients. Read-only towards Tailscale: only GET requests.
package main

import (
	"fmt"
	"net/url"
	"time"

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

const api = "https://api.tailscale.com/api/v2"

func devices() ([]Device, error) {
	tailnet, err := tailnetSetting(integ.Optional("tailnet"))
	if err != nil {
		return nil, err
	}
	key, err := integ.Setting("apikey", "a Tailscale API access token")
	if err != nil {
		return nil, err
	}
	res, err := pdk.Fetch(pdk.FetchRequest{URL: api + "/tailnet/" + url.PathEscape(tailnet) + "/devices",
		Headers: map[string]string{"Authorization": "Bearer " + key, "Accept": "application/json"}})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, httpError(res.Status)
	}
	return parseDevices([]byte(res.Body))
}

// matcher indexes the entries a node may belong to (kinds server, vm, lxc,
// device). If a kind has more entries than one query returns, unmatched
// hostnames are also searched on demand.
func matcher() (*integ.Matcher, error) {
	var all []pdk.Entity
	truncated := false
	for _, kind := range hostKinds {
		l, err := pdk.Graph.Query(pdk.Query{Kind: kind, Limit: queryLimit})
		if err != nil {
			return nil, err
		}
		truncated = truncated || len(l) >= queryLimit
		all = append(all, l...)
	}
	m := newMatcher(all)
	if truncated {
		m.Lookup = func(q string) []pdk.Entity {
			l, _ := pdk.Graph.Query(pdk.Query{Q: q, Limit: 50})
			return hostEntries(l)
		}
	}
	return m, nil
}

func sync() (map[string]int, error) {
	devs, err := devices()
	if err != nil {
		return nil, err
	}
	m, err := matcher()
	if err != nil {
		return nil, err
	}
	ops, links := graphOps(devs, m)
	for _, batch := range chunk(ops, maxOps) {
		if err := integ.Apply(batch); err != nil {
			return nil, err
		}
	}
	return map[string]int{"nodes": len(ops.Ops) - links, "linked": links}, nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "tailscale.sync":
		return sync()
	case "tailscale.devices":
		devs, err := devices()
		if err != nil {
			return nil, err
		}
		return deviceInfo(devs, time.Now()), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
