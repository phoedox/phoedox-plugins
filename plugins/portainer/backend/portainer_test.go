package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
)

func TestPortainer(t *testing.T) {
	var endpoints []Endpoint
	_ = json.Unmarshal([]byte(`[{"Id":1,"Name":"docker-01","Type":2,"Status":1},{"Id":3,"Name":"k8s","Type":5,"Status":1}]`), &endpoints)
	var list []Container
	_ = json.Unmarshal([]byte(`[
		{"Id":"abc","Names":["/paperless-web-1"],"Image":"paperless-ngx:2.13","State":"running","Status":"Up 3 days",
		 "Labels":{"com.docker.compose.project":"paperless"},"Ports":[{"PrivatePort":8000,"PublicPort":8010,"Type":"tcp"},{"PrivatePort":8000,"PublicPort":8010,"Type":"tcp"}]},
		{"Id":"0123456789abcdef","Names":[],"Image":"busybox","State":"exited"}]`), &list)
	var stacks []Stack
	_ = json.Unmarshal([]byte(`[{"Id":7,"Name":"paperless","EndpointId":1,"Status":1},{"Id":8,"Name":"elsewhere","EndpointId":9,"Status":1}]`), &stacks)

	ops := graphOps(endpoints, map[int][]Container{1: list}, stacks)
	if ops.Ops[0].Kind != "server" || ops.Ops[0].ID != "portainer-docker-01" {
		t.Fatalf("server: %+v", ops.Ops[0])
	}
	if ops.Ops[1].Kind != "stack" || ops.Ops[1].ID != "portainer-docker-01-paperless" || ops.Ops[1].Attrs["status"] != "running" {
		t.Fatalf("stack: %+v", ops.Ops[1])
	}
	if l := ops.Ops[2]; l.Rel != "runs_on" || l.From != ops.Ops[1].Ref || l.To != ops.Ops[0].Ref {
		t.Fatalf("stack on server: %+v", l)
	}
	c := ops.Ops[3]
	if c.Kind != "container" || c.Name != "paperless-web-1" || c.Attrs["ports"] != "8010:8000/tcp" || c.Attrs["status"] != "running" {
		t.Fatalf("container: %+v", c)
	}
	if ops.Ops[4].Rel != "runs_on" || ops.Ops[5].Rel != "part_of" || ops.Ops[5].To != ops.Ops[1].Ref {
		t.Fatalf("links: %+v %+v", ops.Ops[4], ops.Ops[5])
	}
	if ops.Ops[6].Name != "0123456789ab" {
		t.Fatalf("unnamed: %+v", ops.Ops[6])
	}
	if st := statusOf(endpoints, map[int][]Container{1: list}); len(st) != 2 || st[0].Stack != "paperless" {
		t.Fatalf("status: %+v", st)
	}
}

// TestDemoData parses the recorded demo answers the screenshots use.
func TestDemoData(t *testing.T) {
	read := func(p string, out any) {
		t.Helper()
		body, err := fakeapi.Body("../testdata/fake", "GET", p)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	var endpoints []Endpoint
	read("/api/endpoints", &endpoints)
	containers := map[int][]Container{}
	for _, e := range endpoints {
		if !e.docker() || e.Status != 1 {
			continue
		}
		var list []Container
		read(fmt.Sprintf("/api/endpoints/%d/docker/containers/json?all=1", e.ID), &list)
		containers[e.ID] = list
	}
	var stacks []Stack
	read("/api/stacks", &stacks)
	if len(endpoints) != 3 || len(containers) != 2 || len(stacks) != 4 {
		t.Fatalf("%d endpoints, %d docker environments, %d stacks", len(endpoints), len(containers), len(stacks))
	}
	count := map[string]int{}
	for _, o := range graphOps(endpoints, containers, stacks).Ops {
		count[o.Kind+o.Rel]++
		if o.Kind == "stack" && o.ID == "portainer-docker-01-photoprism" && o.Attrs["status"] != "stopped" {
			t.Errorf("inactive stack: %+v", o)
		}
	}
	if count["server"] != 2 || count["stack"] != 4 || count["container"] != 11 || count["runs_on"] != 15 || count["part_of"] != 10 {
		t.Fatalf("ops: %v", count)
	}
	if st := statusOf(endpoints, containers); len(st) != 11 || st[0].Ports != "8010:8000/tcp" {
		t.Fatalf("status: %+v", st[0])
	}
}
