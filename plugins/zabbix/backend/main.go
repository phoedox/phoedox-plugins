//go:build wasip1

// Backend of the Zabbix plugin: current problems and hosts for AI clients
// via JSON-RPC, matched to Phoedox entries. Read-only.
package main

import (
	"fmt"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

type zbx struct {
	url string
	h   map[string]string
	id  int
}

func connect() (*zbx, error) {
	raw, err := integ.Setting("url", "e.g. https://zabbix.example.org")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	token, err := integ.Setting("token", "a Zabbix API token")
	if err != nil {
		return nil, err
	}
	return &zbx{url: apiURL(base), h: map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json-rpc",
		"Accept":        "application/json",
	}}, nil
}

// call sends one JSON-RPC request and decodes its result into out.
func (z *zbx) call(method string, params, out any) error {
	z.id++
	body, err := rpcRequest(method, params, z.id)
	if err != nil {
		return err
	}
	res, err := pdk.Fetch(pdk.FetchRequest{Method: "POST", URL: z.url, Headers: z.h, Body: string(body)})
	if err != nil {
		return err
	}
	if res.Status != 200 {
		return &integ.StatusError{Service: "Zabbix", Status: res.Status}
	}
	return decodeRPC([]byte(res.Body), out)
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "zabbix.problems":
		z, err := connect()
		if err != nil {
			return nil, err
		}
		var probs []Problem
		if err := z.call("problem.get", problemParams(), &probs); err != nil {
			return nil, err
		}
		if len(probs) == 0 {
			return []ProblemRow{}, nil
		}
		var events []Event
		if err := z.call("event.get", eventParams(probs), &events); err != nil {
			return nil, err
		}
		var hosts []Host
		if ids := hostIDs(events); len(ids) > 0 {
			if err := z.call("host.get", hostParams(ids), &hosts); err != nil {
				return nil, err
			}
		}
		return problemRows(probs, events, hosts, integ.GraphMatcher()), nil
	case "zabbix.hosts":
		z, err := connect()
		if err != nil {
			return nil, err
		}
		var hosts []Host
		if err := z.call("host.get", hostParams(nil), &hosts); err != nil {
			return nil, err
		}
		return hostRows(hosts, integ.GraphMatcher()), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
