//go:build wasip1

// Backend of the ESPHome plugin: imports the devices the ESPHome dashboard
// (Device Builder) knows every six hours (and on esphome.sync); device list
// with online state and versions for AI clients. Read-only: only GET
// requests to /devices and /ping; nothing is compiled, flashed or switched.
package main

import (
	"encoding/base64"
	"errors"
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

type dashboard struct {
	base string
	h    map[string]string
}

func connect() (*dashboard, error) {
	raw, err := integ.Setting("url", "the ESPHome dashboard, e.g. http://192.168.1.40:6052")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	d := &dashboard{base: base, h: map[string]string{"Accept": "application/json"}}
	user, pass := integ.Optional("username"), integ.Optional("password")
	if user != "" || pass != "" {
		d.h["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	}
	return d, nil
}

func (d *dashboard) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: d.base + path, Headers: d.h})
	if err != nil {
		return nil, err
	}
	switch res.Status {
	case 200:
		return []byte(res.Body), nil
	case 401:
		return nil, errors.New("the ESPHome dashboard rejected the login (settings username and password)")
	case 301, 302, 303, 307:
		return nil, errors.New("the ESPHome dashboard redirects to its login page: this needs the Device Builder (ESPHome 2026.6 or later), which accepts username and password directly; the older dashboard only works without a password")
	case 404:
		return nil, errors.New("the address has no ESPHome device list at /devices (check the setting url; the Home Assistant add-on must expose its port)")
	}
	return nil, &integ.StatusError{Service: "ESPHome dashboard", Status: res.Status}
}

func (d *dashboard) devices() (Devices, error) {
	body, err := d.get("/devices")
	if err != nil {
		return Devices{}, err
	}
	return parseDevices(body)
}

func sync() (map[string]int, error) {
	d, err := connect()
	if err != nil {
		return nil, err
	}
	devs, err := d.devices()
	if err != nil {
		return nil, err
	}
	rooms, err := pdk.Graph.Query(pdk.Query{Kind: "room", Limit: 500})
	if err != nil {
		return nil, err
	}
	if err := integ.Apply(graphOps(devs.Configured, roomIndex(rooms))); err != nil {
		return nil, err
	}
	return map[string]int{"devices": len(devs.Configured)}, nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "esphome.sync":
		return sync()
	case "esphome.devices":
		d, err := connect()
		if err != nil {
			return nil, err
		}
		devs, err := d.devices()
		if err != nil {
			return nil, err
		}
		ping := map[string]*bool{}
		if b, err := d.get("/ping"); err == nil {
			ping = parsePing(b)
		}
		return overview(devs, ping), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
