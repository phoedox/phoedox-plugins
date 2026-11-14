//go:build wasip1

// Backend of the UniFi plugin: imports devices every six hours (and on
// unifi.sync) and lists clients for AI clients. Read-only.
package main

import (
	"fmt"
	"net/url"

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

type api struct {
	base string
	h    map[string]string
	site Site
}

func connect() (*api, error) {
	raw, err := integ.Setting("url", "the UniFi console, e.g. https://192.168.1.1")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	key, err := integ.Setting("apikey", "an API key from UniFi Network → Settings → Control Plane → Integrations")
	if err != nil {
		return nil, err
	}
	a := &api{base: base + "/proxy/network/integration/v1", h: map[string]string{"X-API-KEY": key, "Accept": "application/json"}}
	var sites page[Site]
	if err := integ.GetJSON("UniFi", a.base+"/sites?limit=200", a.h, &sites); err != nil {
		return nil, err
	}
	site, ok := pickSite(sites.Data, integ.Optional("site"))
	if !ok {
		return nil, fmt.Errorf("UniFi site %q not found", integ.Optional("site"))
	}
	a.site = site
	return a, nil
}

func (a *api) devices() ([]Device, error) {
	var list page[Device]
	if err := integ.GetJSON("UniFi", a.base+"/sites/"+url.PathEscape(a.site.ID)+"/devices?limit=200", a.h, &list); err != nil {
		return nil, err
	}
	// Details carry the uplink and the firmware.
	for i, d := range list.Data {
		var full Device
		if err := integ.GetJSON("UniFi", a.base+"/sites/"+url.PathEscape(a.site.ID)+"/devices/"+url.PathEscape(d.ID), a.h, &full); err == nil {
			list.Data[i] = full
		}
	}
	return list.Data, nil
}

func sync() (map[string]int, error) {
	a, err := connect()
	if err != nil {
		return nil, err
	}
	devices, err := a.devices()
	if err != nil {
		return nil, err
	}
	return map[string]int{"devices": len(devices)}, integ.Apply(graphOps(devices))
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "unifi.sync":
		return sync()
	case "unifi.clients":
		a, err := connect()
		if err != nil {
			return nil, err
		}
		var clients page[Client]
		if err := integ.GetJSON("UniFi", a.base+"/sites/"+url.PathEscape(a.site.ID)+"/clients?limit=500", a.h, &clients); err != nil {
			return nil, err
		}
		var devices page[Device]
		_ = integ.GetJSON("UniFi", a.base+"/sites/"+url.PathEscape(a.site.ID)+"/devices?limit=200", a.h, &devices)
		return clientInfo(clients.Data, devices.Data), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
