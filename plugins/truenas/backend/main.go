//go:build wasip1

// Backend of the TrueNAS plugin: imports the system every six hours (and on
// truenas.sync) through the REST API v2.0; pools and shares for AI clients.
// Read-only: only GET requests.
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

type tn struct {
	base string
	h    map[string]string
}

func connect() (*tn, error) {
	raw, err := integ.Setting("url", "e.g. https://truenas.example.org")
	if err != nil {
		return nil, err
	}
	base, err := secureURL(raw)
	if err != nil {
		return nil, err
	}
	key, err := integ.Setting("apikey", "an API key of a user with the Read-Only Administrator role")
	if err != nil {
		return nil, err
	}
	return &tn{base: base, h: map[string]string{"Authorization": "Bearer " + key, "Accept": "application/json"}}, nil
}

func (t *tn) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: t.base + "/api/v2.0" + path, Headers: t.h})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, httpError(res.Status)
	}
	return []byte(res.Body), nil
}

func sync() (map[string]int, error) {
	t, err := connect()
	if err != nil {
		return nil, err
	}
	body, err := t.get("/system/info")
	if err != nil {
		return nil, err
	}
	sys, err := parseSystem(body)
	if err != nil {
		return nil, err
	}
	return map[string]int{"servers": 1}, integ.Apply(graphOps(sys, t.base))
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "truenas.sync":
		return sync()
	case "truenas.pools":
		t, err := connect()
		if err != nil {
			return nil, err
		}
		body, err := t.get("/pool")
		if err != nil {
			return nil, err
		}
		return parsePools(body)
	case "truenas.shares":
		t, err := connect()
		if err != nil {
			return nil, err
		}
		smb, err := t.get("/sharing/smb")
		if err != nil {
			return nil, err
		}
		nfs, err := t.get("/sharing/nfs")
		if err != nil {
			return nil, err
		}
		return parseShares(smb, nfs)
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
