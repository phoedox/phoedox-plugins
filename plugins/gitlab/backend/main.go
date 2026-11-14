//go:build wasip1

// Backend of the GitLab plugin: imports the projects of a group every six
// hours (and on gitlab.sync) and lists open issues and merge requests for AI
// clients. Read-only.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
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

type gl struct {
	api, group string
	h          map[string]string
}

func connect() (*gl, error) {
	raw, err := integ.Setting("url", "e.g. https://gitlab.com")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	group, err := integ.Setting("group", "the group path, e.g. acme/infra")
	if err != nil {
		return nil, err
	}
	group = strings.Trim(strings.TrimSpace(group), "/")
	if err := validPath(group); err != nil {
		return nil, err
	}
	h := map[string]string{"Accept": "application/json"}
	if token := integ.Optional("token"); token != "" {
		h["PRIVATE-TOKEN"] = token
	}
	return &gl{api: base + "/api/v4", group: group, h: h}, nil
}

func (g *gl) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: g.api + path, Headers: g.h})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, &integ.StatusError{Service: "GitLab", Status: res.Status}
	}
	return []byte(res.Body), nil
}

func sync() (map[string]int, error) {
	g, err := connect()
	if err != nil {
		return nil, err
	}
	var all []Project
	for page := 1; page <= 10; page++ {
		body, err := g.get(fmt.Sprintf("/groups/%s/projects?include_subgroups=true&per_page=100&page=%d&order_by=last_activity_at", url.PathEscape(g.group), page))
		if err != nil {
			return nil, err
		}
		list, err := parseProjects(body)
		if err != nil {
			return nil, err
		}
		all = append(all, list...)
		if len(list) < 100 {
			break
		}
	}
	ops := graphOps(all)
	return map[string]int{"projects": len(ops.Ops)}, integ.Apply(ops)
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "gitlab.sync":
		return sync()
	case "gitlab.open":
		var in struct {
			Project string `json:"project"`
		}
		_ = json.Unmarshal(tc.Args, &in)
		g, err := connect()
		if err != nil {
			return nil, err
		}
		p := strings.Trim(strings.TrimSpace(in.Project), "/")
		if !strings.HasPrefix(p, g.group+"/") {
			p = g.group + "/" + p // only projects of the configured group
		}
		if err := validPath(p); err != nil {
			return nil, err
		}
		id := url.PathEscape(p)
		body, err := g.get("/projects/" + id + "/issues?state=opened&per_page=50")
		if err != nil {
			return nil, err
		}
		issues, err := parseItems("issue", body)
		if err != nil {
			return nil, err
		}
		body, err = g.get("/projects/" + id + "/merge_requests?state=opened&per_page=50")
		if err != nil {
			return nil, err
		}
		mrs, err := parseItems("merge_request", body)
		if err != nil {
			return nil, err
		}
		return append(issues, mrs...), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
