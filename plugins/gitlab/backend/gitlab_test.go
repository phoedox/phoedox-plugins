package main

import "testing"

func TestGitLab(t *testing.T) {
	list, err := parseProjects([]byte(`[
		{"path_with_namespace":"acme/infra/ansible","web_url":"https://gitlab.com/acme/infra/ansible","visibility":"private","default_branch":"main","topics":["ops","ansible"],"star_count":2,"open_issues_count":5,"last_activity_at":"2026-11-10T08:00:00.000Z"},
		{"path_with_namespace":"acme/infra/fork","forked_from_project":{"id":1}},
		{"path_with_namespace":"acme/infra/docs","visibility":"public","archived":true}]`))
	if err != nil {
		t.Fatal(err)
	}
	ops := graphOps(list)
	if len(ops.Ops) != 2 || ops.Ops[0].ID != "gl-acme-infra-ansible" || ops.Ops[0].Attrs["topics"] != "ops, ansible" || ops.Ops[0].Attrs["last_activity"] != "2026-11-10" || ops.Ops[0].Attrs["open_issues"] != 5 {
		t.Fatalf("ops: %+v", ops.Ops)
	}
	if _, ok := ops.Ops[1].Attrs["open_issues"]; ok || ops.Ops[1].Attrs["archived"] != true {
		t.Fatalf("docs: %+v", ops.Ops[1])
	}
	items, err := parseItems("merge_request", []byte(`[{"iid":4,"title":"Draft: upgrade","web_url":"u","draft":true,"author":{"username":"jan"},"labels":["deps"]}]`))
	if err != nil || items[0].Author != "jan" || !items[0].Draft || items[0].Kind != "merge_request" {
		t.Fatalf("items: %+v %v", items, err)
	}
	for _, bad := range []string{"../x", "a//b", "a/../b", "", "/a"} {
		if validPath(bad) == nil {
			t.Fatal("accepted", bad)
		}
	}
	if validPath("acme/infra/ansible") != nil {
		t.Fatal("rejected a good path")
	}
}
