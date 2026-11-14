//go:build wasip1

// Backend of the Checkmk plugin: service problems and host states for AI
// clients via the REST API, matched to Phoedox entries. Read-only.
package main

import (
	"fmt"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

type cmk struct {
	api string
	h   map[string]string
}

func connect() (*cmk, error) {
	raw, err := integ.Setting("url", "the site address, e.g. https://monitoring.example.org/mysite")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	user, err := integ.Setting("username", "an automation user")
	if err != nil {
		return nil, err
	}
	secret, err := integ.Setting("secret", "the automation secret of that user")
	if err != nil {
		return nil, err
	}
	user, secret = strings.TrimSpace(user), strings.TrimSpace(secret)
	if strings.ContainsAny(user, " \t") {
		return nil, fmt.Errorf("the setting username must not contain spaces")
	}
	return &cmk{api: apiBase(base), h: map[string]string{"Authorization": "Bearer " + user + " " + secret, "Accept": "application/json"}}, nil
}

func (c *cmk) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: c.api + path, Headers: c.h})
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
	case "checkmk.problems":
		c, err := connect()
		if err != nil {
			return nil, err
		}
		body, err := c.get(problemsPath())
		if err != nil {
			return nil, err
		}
		list, err := parseServices(body)
		if err != nil {
			return nil, err
		}
		return problemRows(list, integ.GraphMatcher()), nil
	case "checkmk.hosts":
		c, err := connect()
		if err != nil {
			return nil, err
		}
		body, err := c.get(hostsPath())
		if err != nil {
			return nil, err
		}
		list, err := parseHosts(body)
		if err != nil {
			return nil, err
		}
		return hostRows(list, integ.GraphMatcher()), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
