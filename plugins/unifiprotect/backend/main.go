//go:build wasip1

// Backend of the UniFi Protect plugin: imports the NVR, cameras, floodlights
// and sensors through the official integration API every six hours (and on
// protect.sync); camera status for AI clients. Read-only: only GET requests.
package main

import (
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

type protect struct {
	base string
	h    map[string]string
}

func connect() (*protect, error) {
	raw, err := integ.Setting("url", "the console, e.g. https://unifi.example.org")
	if err != nil {
		return nil, err
	}
	base, err := apiBase(raw)
	if err != nil {
		return nil, err
	}
	key, err := integ.Setting("apikey", "an API key from UniFi OS > Control Plane > Integrations")
	if err != nil {
		return nil, err
	}
	return &protect{base: base, h: map[string]string{"X-API-KEY": key, "Accept": "application/json"}}, nil
}

var errNotFound = errors.New("not found")

func (p *protect) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: p.base + path, Headers: p.h})
	if err != nil {
		return nil, err
	}
	switch res.Status {
	case 200:
		return []byte(res.Body), nil
	case 401, 403:
		return nil, errors.New("UniFi Protect rejected the API key (setting apikey)")
	case 404:
		return nil, errNotFound
	}
	return nil, &integ.StatusError{Service: "UniFi Protect", Status: res.Status}
}

func (p *protect) cameras() ([]Device, error) {
	body, err := p.get("/cameras")
	if errors.Is(err, errNotFound) {
		return nil, errors.New("the console has no UniFi Protect integration API (Protect 5.3 or later; check the setting url)")
	}
	if err != nil {
		return nil, err
	}
	return parseDevices("camera list", body)
}

// optional reads lights or sensors; older releases do not have them.
func (p *protect) optional(path string) ([]Device, error) {
	body, err := p.get(path)
	if errors.Is(err, errNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseDevices(path[1:], body)
}

func sync() (map[string]int, error) {
	p, err := connect()
	if err != nil {
		return nil, err
	}
	cams, err := p.cameras()
	if err != nil {
		return nil, err
	}
	body, err := p.get("/nvrs")
	if err != nil {
		return nil, err
	}
	nvr, err := parseNVR(body)
	if err != nil {
		return nil, err
	}
	version := ""
	if b, err := p.get("/meta/info"); err == nil {
		if m, err := decode[Meta]("version", b); err == nil {
			version = m.Version
		}
	}
	lights, err := p.optional("/lights")
	if err != nil {
		return nil, err
	}
	sensors, err := p.optional("/sensors")
	if err != nil {
		return nil, err
	}
	if err := integ.Apply(graphOps(nvr, version, cams, lights, sensors)); err != nil {
		return nil, err
	}
	return map[string]int{"cameras": len(cams), "lights": len(lights), "sensors": len(sensors)}, nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "protect.sync":
		return sync()
	case "protect.cameras":
		p, err := connect()
		if err != nil {
			return nil, err
		}
		cams, err := p.cameras()
		if err != nil {
			return nil, err
		}
		return cameraRows(cams), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
