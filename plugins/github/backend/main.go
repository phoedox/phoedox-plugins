//go:build wasip1

// Backend of the GitHub plugin: imports repositories every six hours (and on
// github.sync) and lists open issues and pull requests for AI clients.
package main

import (
	"fmt"
	"strings"

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

func get(path string) ([]byte, error) {
	h := map[string]string{"Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"}
	if token, err := pdk.Secret("token"); err == nil && token != "" {
		h["Authorization"] = "Bearer " + token
	}
	res, err := pdk.Fetch(pdk.FetchRequest{URL: "https://api.github.com" + path, Headers: h})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, fmt.Errorf("GitHub answered HTTP %d", res.Status)
	}
	return []byte(res.Body), nil
}

func owner() (string, error) {
	o, err := pdk.Secret("owner")
	if err != nil {
		return "", fmt.Errorf("set the setting owner (organization or account): %w", err)
	}
	o = strings.TrimSpace(o)
	return o, validName(o)
}

func sync() (map[string]int, error) {
	o, err := owner()
	if err != nil {
		return nil, err
	}
	// Organization first, then a user account; at most 300 repositories.
	var repos []Repo
	for page := 1; page <= 3; page++ {
		body, err := get(fmt.Sprintf("/orgs/%s/repos?per_page=100&page=%d&sort=pushed", o, page))
		if err != nil && page == 1 {
			body, err = get(fmt.Sprintf("/users/%s/repos?per_page=100&page=%d&sort=pushed", o, page))
		}
		if err != nil {
			return nil, err
		}
		list, err := parseRepos(body)
		if err != nil {
			return nil, err
		}
		repos = append(repos, list...)
		if len(list) < 100 {
			break
		}
	}
	ops := graphOps(o, repos)
	if len(ops.Ops) > 0 {
		if _, err := pdk.Graph.Apply(&ops); err != nil {
			return nil, err
		}
	}
	return map[string]int{"repositories": len(ops.Ops)}, nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "github.sync":
		return sync()
	case "github.open":
		var args struct {
			Repo string `json:"repo"`
		}
		_ = jsonUnmarshal(tc.Args, &args)
		if err := validName(args.Repo); err != nil {
			return nil, err
		}
		o, err := owner()
		if err != nil {
			return nil, err
		}
		body, err := get(fmt.Sprintf("/repos/%s/%s/issues?state=open&per_page=50", o, args.Repo))
		if err != nil {
			return nil, err
		}
		return openItems(body)
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
