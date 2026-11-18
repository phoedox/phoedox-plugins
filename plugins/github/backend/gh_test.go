package main

import (
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
)

func TestGitHub(t *testing.T) {
	repos, err := parseRepos([]byte(`[
		{"name":"phoedox","html_url":"https://github.com/phoedox/phoedox","language":"Go","visibility":"private","stargazers_count":3,"pushed_at":"2026-11-13T22:00:00Z","license":{"spdx_id":"AGPL-3.0"}},
		{"name":"fork-of-x","fork":true},
		{"name":"phoedox-sdk","html_url":"https://github.com/phoedox/phoedox-sdk","language":"TypeScript","visibility":"public","license":{"spdx_id":"NOASSERTION"}}]`))
	if err != nil {
		t.Fatal(err)
	}
	ops := graphOps("phoedox", repos)
	if len(ops.Ops) != 2 || ops.Ops[0].ID != "gh-phoedox-phoedox" || ops.Ops[0].Name != "phoedox/phoedox" || ops.Ops[0].Attrs["license"] != "AGPL-3.0" || ops.Ops[0].Attrs["pushed_at"] != "2026-11-13" {
		t.Fatalf("ops: %+v", ops.Ops)
	}
	if _, ok := ops.Ops[1].Attrs["license"]; ok {
		t.Fatal("NOASSERTION is no license")
	}
	items, err := openItems([]byte(`[{"number":5,"title":"Bug","html_url":"u","user":{"login":"jan"},"labels":[{"name":"bug"}]},{"number":6,"title":"PR","pull_request":{}}]`))
	if err != nil || len(items) != 2 || items[0].Kind != "issue" || items[1].Kind != "pull" || items[0].Labels[0] != "bug" {
		t.Fatalf("items: %+v", items)
	}
	if validName("../etc") == nil {
		t.Fatal("bad name")
	}
}

// TestDemoData parses the recorded demo answers (for screenshots once the API
// address can be pointed at the fake API).
func TestDemoData(t *testing.T) {
	body, err := fakeapi.Body("../testdata/fake", "GET", "/orgs/maple-labs/repos?per_page=100&page=1&sort=pushed")
	if err != nil {
		t.Fatal(err)
	}
	repos, err := parseRepos(body)
	if err != nil || len(repos) != 7 {
		t.Fatal(err, len(repos))
	}
	ops := graphOps("maple-labs", repos).Ops
	if len(ops) != 6 || ops[2].Name != "maple-labs/maple-ui" || ops[2].Attrs["license"] != "MIT" || ops[2].Attrs["stars"] != 48 {
		t.Fatalf("ops: %+v", ops)
	}
	body, err = fakeapi.Body("../testdata/fake", "GET", "/repos/maple-labs/shop/issues?state=open&per_page=50")
	if err != nil {
		t.Fatal(err)
	}
	if items, err := openItems(body); err != nil || len(items) != 2 || items[1].Kind != "pull" {
		t.Fatalf("items: %+v %v", items, err)
	}
}
