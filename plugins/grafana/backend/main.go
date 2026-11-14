//go:build wasip1

// Backend of the Grafana plugin: dashboards and firing alerts for AI
// clients via the HTTP API, alerts matched to Phoedox entries. Read-only.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

type gf struct {
	base string
	h    map[string]string
}

func connect() (*gf, error) {
	raw, err := integ.Setting("url", "e.g. https://grafana.example.org")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	token, err := integ.Setting("token", "a service account token with the Viewer role")
	if err != nil {
		return nil, err
	}
	return &gf{base: base, h: map[string]string{"Authorization": "Bearer " + token, "Accept": "application/json"}}, nil
}

func (g *gf) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: g.base + path, Headers: g.h})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, apiError(res.Status, []byte(res.Body))
	}
	return []byte(res.Body), nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "grafana.dashboards":
		var in struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(tc.Args, &in)
		g, err := connect()
		if err != nil {
			return nil, err
		}
		body, err := g.get(searchPath(in.Query))
		if err != nil {
			return nil, err
		}
		return parseDashboards(body, g.base)
	case "grafana.alerts":
		g, err := connect()
		if err != nil {
			return nil, err
		}
		body, err := g.get(alertsPath)
		if err != nil {
			return nil, err
		}
		list, err := parseAlerts(body)
		if err != nil {
			return nil, err
		}
		return alertRows(list, integ.GraphMatcher()), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
