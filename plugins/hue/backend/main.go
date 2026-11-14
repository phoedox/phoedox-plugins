//go:build wasip1

// Backend of the Philips Hue plugin: imports the bridge, its rooms and
// lights every six hours (and on hue.sync); light states for AI clients.
// Read-only: only GET requests to API v1, nothing is switched.
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

type bridge struct {
	api, user string
}

func connect() (*bridge, error) {
	raw, err := integ.Setting("url", "the bridge address, e.g. http://192.168.1.20")
	if err != nil {
		return nil, err
	}
	user, err := integ.Setting("username", "an application key of the bridge")
	if err != nil {
		return nil, err
	}
	api, err := apiBase(raw, user)
	if err != nil {
		return nil, err
	}
	return &bridge{api: api, user: user}, nil
}

// get fetches a path below /api/<username>; the key never appears in errors.
func (b *bridge) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: b.api + path, Headers: map[string]string{"Accept": "application/json"}})
	if err != nil {
		return nil, errors.New(redact(err.Error(), b.user))
	}
	if res.Status != 200 {
		return nil, &integ.StatusError{Service: "Hue bridge", Status: res.Status}
	}
	return []byte(res.Body), nil
}

func (b *bridge) lightsAndRooms() ([]Light, []Group, error) {
	body, err := b.get("/lights")
	if err != nil {
		return nil, nil, err
	}
	lights, err := parseLights(body)
	if err != nil {
		return nil, nil, err
	}
	if body, err = b.get("/groups"); err != nil {
		return nil, nil, err
	}
	rooms, err := parseRooms(body)
	return lights, rooms, err
}

func sync() (map[string]int, error) {
	b, err := connect()
	if err != nil {
		return nil, err
	}
	body, err := b.get("/config")
	if err != nil {
		return nil, err
	}
	cfg, err := parseConfig(body)
	if err != nil {
		return nil, err
	}
	lights, rooms, err := b.lightsAndRooms()
	if err != nil {
		return nil, err
	}
	existing, err := pdk.Graph.Query(pdk.Query{Kind: "room", Limit: 500})
	if err != nil {
		return nil, err
	}
	if err := integ.Apply(graphOps(cfg, lights, rooms, roomIndex(existing))); err != nil {
		return nil, err
	}
	return map[string]int{"rooms": len(rooms), "lights": len(lights)}, nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "hue.sync":
		return sync()
	case "hue.lights":
		b, err := connect()
		if err != nil {
			return nil, err
		}
		lights, rooms, err := b.lightsAndRooms()
		if err != nil {
			return nil, err
		}
		return lightRows(lights, rooms), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
