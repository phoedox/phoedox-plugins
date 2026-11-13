package main

import "testing"

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
