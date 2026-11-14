//go:build wasip1

// Backend of the OPNsense plugin: imports the firewall and its interfaces
// every six hours (and on opnsense.sync); DHCP leases and gateway status for
// AI clients. Read-only.
package main

import (
	"encoding/base64"
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

type fw struct {
	base string
	h    map[string]string
}

func connect() (*fw, error) {
	raw, err := integ.Setting("url", "e.g. https://fw.example.org")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	key, err := integ.Setting("key", "the API key")
	if err != nil {
		return nil, err
	}
	secret, err := integ.Setting("secret", "the API secret")
	if err != nil {
		return nil, err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(key + ":" + secret))
	return &fw{base: base, h: map[string]string{"Authorization": "Basic " + auth, "Accept": "application/json"}}, nil
}

func (f *fw) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: f.base + path, Headers: f.h})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, &integ.StatusError{Service: "OPNsense", Status: res.Status}
	}
	return []byte(res.Body), nil
}

func sync() (map[string]int, error) {
	f, err := connect()
	if err != nil {
		return nil, err
	}
	status, err := f.get("/api/core/firmware/status")
	if err != nil {
		return nil, err
	}
	body, err := f.get("/api/interfaces/overview/interfacesInfo")
	if err != nil {
		return nil, err
	}
	ifaces, err := parseIfaces(body)
	if err != nil {
		return nil, fmt.Errorf("unexpected interface list from OPNsense: %w", err)
	}
	name := integ.Optional("name")
	if name == "" {
		name = "OPNsense"
	}
	return map[string]int{"interfaces": len(ifaces)}, integ.Apply(graphOps(name, firmwareVersion(status), ifaces))
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "opnsense.sync":
		return sync()
	case "opnsense.leases":
		f, err := connect()
		if err != nil {
			return nil, err
		}
		body, err := f.get("/api/dhcpv4/leases/searchLease")
		if err != nil {
			return nil, err
		}
		return parseLeases(body)
	case "opnsense.gateways":
		f, err := connect()
		if err != nil {
			return nil, err
		}
		body, err := f.get("/api/routes/gateway/status")
		if err != nil {
			return nil, err
		}
		return parseGateways(body)
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
