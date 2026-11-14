//go:build wasip1

// Backend of the Uptime Kuma plugin: live monitor state from a public status
// page for AI clients, matched to Phoedox entries. Read-only.
package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "uptimekuma.status", "uptimekuma.down":
		monitors, err := load()
		if err != nil {
			return nil, err
		}
		if tc.Name == "uptimekuma.down" {
			monitors = notUp(monitors)
		}
		return summarize(monitors), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}

func load() ([]Monitor, error) {
	raw, err := integ.Setting("url", "e.g. https://status.example.org")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	slug, err := integ.Setting("status_page", "the slug of a status page, e.g. infra")
	if err != nil {
		return nil, err
	}
	slug = strings.TrimSpace(slug)
	if !validSlug(slug) {
		return nil, fmt.Errorf("the setting status_page must be the slug of a status page (letters, digits, - and _)")
	}
	h := map[string]string{"Accept": "application/json"}
	var page StatusPage
	if err := integ.GetJSON("Uptime Kuma", base+"/api/status-page/"+url.PathEscape(slug), h, &page); err != nil {
		return nil, notFound(err)
	}
	var beats Heartbeats
	if err := integ.GetJSON("Uptime Kuma", base+"/api/status-page/heartbeat/"+url.PathEscape(slug), h, &beats); err != nil {
		return nil, notFound(err)
	}
	return monitors(page, beats, integ.GraphMatcher()), nil
}

func notFound(err error) error {
	if se, ok := err.(*integ.StatusError); ok && se.Status == 404 {
		return fmt.Errorf("Uptime Kuma has no published status page with the configured slug")
	}
	return err
}
