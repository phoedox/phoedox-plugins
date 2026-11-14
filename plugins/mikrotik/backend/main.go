//go:build wasip1

// Backend of the MikroTik plugin: imports the router every six hours (and on
// mikrotik.sync) through the RouterOS v7 REST API; interfaces and DHCP leases
// for AI clients. Read-only: only GET requests.
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

type ros struct {
	base string
	h    map[string]string
}

func connect() (*ros, error) {
	raw, err := integ.Setting("url", "e.g. https://192.168.88.1")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	user, err := integ.Setting("username", "a RouterOS user with the read and rest-api policies")
	if err != nil {
		return nil, err
	}
	pass, err := integ.Setting("password", "the password of that user")
	if err != nil {
		return nil, err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	return &ros{base: base, h: map[string]string{"Authorization": "Basic " + auth, "Accept": "application/json"}}, nil
}

func (r *ros) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: r.base + "/rest" + path, Headers: r.h})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, httpError(res.Status)
	}
	return []byte(res.Body), nil
}

func sync() (map[string]int, error) {
	r, err := connect()
	if err != nil {
		return nil, err
	}
	identity, err := r.get("/system/identity")
	if err != nil {
		return nil, err
	}
	resource, err := r.get("/system/resource")
	if err != nil {
		return nil, err
	}
	board, _ := r.get("/system/routerboard") // missing on CHR and x86
	sys, err := parseSystem(identity, resource, board)
	if err != nil {
		return nil, err
	}
	var ifaces []Iface
	if body, err := r.get("/interface"); err == nil {
		ifaces, _ = parseInterfaces(body, nil)
	}
	var addrs []Address
	if body, err := r.get("/ip/address"); err == nil {
		addrs, _ = parseAddresses(body)
	}
	ops := graphOps(sys, routerID(sys, ifaces), routerIP(r.base, addrs))
	return map[string]int{"routers": 1}, integ.Apply(ops)
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "mikrotik.sync":
		return sync()
	case "mikrotik.interfaces":
		r, err := connect()
		if err != nil {
			return nil, err
		}
		body, err := r.get("/interface")
		if err != nil {
			return nil, err
		}
		abody, err := r.get("/ip/address")
		if err != nil {
			return nil, err
		}
		addrs, err := parseAddresses(abody)
		if err != nil {
			return nil, err
		}
		return parseInterfaces(body, addrs)
	case "mikrotik.leases":
		r, err := connect()
		if err != nil {
			return nil, err
		}
		body, err := r.get("/ip/dhcp-server/lease")
		if err != nil {
			return nil, err
		}
		return parseLeases(body)
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
