//go:build wasip1

// Backend of the Azure plugin: signs in with client credentials and imports
// the virtual machines of a subscription every six hours (and on
// azure.sync); lists them for AI clients. Read-only.
package main

import (
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

type arm struct {
	subscription string
	h            map[string]string
}

func connect() (*arm, error) {
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
	sub, err := integ.Setting("subscription_id", "the subscription ID")
	if err != nil {
		return nil, err
	}
	tenant, sub = strings.TrimSpace(tenant), strings.TrimSpace(sub)
	if err := validTenant(tenant); err != nil {
		return nil, err
	}
	if err := validSubscription(sub); err != nil {
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
	return &arm{subscription: sub, h: map[string]string{"Authorization": "Bearer " + token, "Accept": "application/json"}}, nil
}

// vms lists the VMs of the subscription, following nextLink on ARM only (at
// most 50 pages).
func (a *arm) vms(statusOnly bool) ([]VM, error) {
	var all []VM
	link := listURL(a.subscription, statusOnly)
	for page := 0; page < 50; page++ {
		res, err := pdk.Fetch(pdk.FetchRequest{URL: link, Headers: a.h})
		if err != nil {
			return nil, err
		}
		if res.Status != 200 {
			return nil, armError(res.Status, []byte(res.Body))
		}
		list, next, err := parseVMs([]byte(res.Body))
		if err != nil {
			return nil, fmt.Errorf("unexpected answer from Azure Resource Manager: %w", err)
		}
		all = append(all, list...)
		var ok bool
		if link, ok = safeNext(next); !ok {
			break
		}
	}
	return all, nil
}

// list reads the VMs with their power state (left out if the status listing
// fails).
func list() ([]VM, error) {
	a, err := connect()
	if err != nil {
		return nil, err
	}
	vms, err := a.vms(false)
	if err != nil {
		return nil, err
	}
	if status, err := a.vms(true); err == nil {
		mergeStatus(vms, status)
	} else {
		pdk.Warn("Azure power states not available: " + err.Error())
	}
	return vms, nil
}

func sync() (map[string]int, error) {
	vms, err := list()
	if err != nil {
		return nil, err
	}
	err = integ.ApplyBatches(graphOps(vms), 500, func(b *pdk.GraphOps) (map[string]string, error) {
		r, err := pdk.Graph.Apply(b)
		return r.Refs, err
	})
	return map[string]int{"vms": len(vms)}, err
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "azure.sync":
		return sync()
	case "azure.vms":
		vms, err := list()
		if err != nil {
			return nil, err
		}
		return vmInfo(vms), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
