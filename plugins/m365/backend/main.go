//go:build wasip1

// Backend of the Microsoft 365 plugin: signs in with client credentials,
// imports the Intune managed devices every six hours (and on m365.sync) and
// lists devices and licenses for AI clients. Read-only.
package main

import (
	"encoding/json"
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

// maxPages bounds the device pages (Intune returns up to 1000 per page).
const maxPages = 100

type msgraph struct{ h map[string]string }

func connect() (*msgraph, error) {
	tenant, err := integ.Setting("tenant_id", "the directory (tenant) ID")
	if err != nil {
		return nil, err
	}
	clientID, err := integ.Setting("client_id", "the application (client) ID of the app registration")
	if err != nil {
		return nil, err
	}
	secret, err := integ.Setting("client_secret", "a client secret of the app registration")
	if err != nil {
		return nil, err
	}
	tenant = strings.TrimSpace(tenant)
	if err := validTenant(tenant); err != nil {
		return nil, err
	}
	res, err := pdk.Fetch(pdk.FetchRequest{
		Method:  "POST",
		URL:     tokenURL(tenant),
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"},
		Body:    tokenBody(strings.TrimSpace(clientID), strings.TrimSpace(secret)),
	})
	if err != nil {
		return nil, err
	}
	token, err := parseToken(res.Status, []byte(res.Body))
	if err != nil {
		return nil, err
	}
	return &msgraph{h: map[string]string{"Authorization": "Bearer " + token, "Accept": "application/json"}}, nil
}

func (g *msgraph) get(link, permission string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: link, Headers: g.h})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, graphError(res.Status, []byte(res.Body), permission)
	}
	return []byte(res.Body), nil
}

// devices lists the managed devices, following @odata.nextLink on Microsoft
// Graph only.
func (g *msgraph) devices() ([]Device, error) {
	var all []Device
	link := devicesURL()
	for page := 0; page < maxPages; page++ {
		body, err := g.get(link, "DeviceManagementManagedDevices.Read.All")
		if err != nil {
			return nil, err
		}
		list, next, err := parseDevices(body)
		if err != nil {
			return nil, err
		}
		all = append(all, list...)
		var ok bool
		if link, ok = safeNext(next); !ok {
			break
		}
	}
	return all, nil
}

func sync() (map[string]int, error) {
	g, err := connect()
	if err != nil {
		return nil, err
	}
	devices, err := g.devices()
	if err != nil {
		return nil, err
	}
	ops := graphOps(devices)
	return map[string]int{"devices": len(ops.Ops)}, integ.Apply(ops)
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "m365.sync":
		return sync()
	case "m365.devices":
		var in struct {
			Q string `json:"q"`
		}
		if len(tc.Args) > 0 {
			if err := json.Unmarshal(tc.Args, &in); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		g, err := connect()
		if err != nil {
			return nil, err
		}
		devices, err := g.devices()
		if err != nil {
			return nil, err
		}
		return deviceInfo(devices, in.Q), nil
	case "m365.licenses":
		g, err := connect()
		if err != nil {
			return nil, err
		}
		body, err := g.get(skusURL(), "Organization.Read.All")
		if err != nil {
			return nil, err
		}
		return parseLicenses(body)
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
