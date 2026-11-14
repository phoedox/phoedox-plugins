package main

import (
	"encoding/json"
	"testing"
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
	if ops.Ops[1].Kind != "stack" || ops.Ops[1].ID != "portainer-docker-01-paperless" || ops.Ops[1].Attrs["status"] != "active" {
		t.Fatalf("stack: %+v", ops.Ops[1])
	}
	c := ops.Ops[2]
	if c.Kind != "container" || c.Name != "paperless-web-1" || c.Attrs["ports"] != "8010:8000/tcp" || c.Attrs["status"] != "running" {
		t.Fatalf("container: %+v", c)
	}
	if ops.Ops[3].Rel != "runs_on" || ops.Ops[4].Rel != "part_of" || ops.Ops[4].To != ops.Ops[1].Ref {
		t.Fatalf("links: %+v %+v", ops.Ops[3], ops.Ops[4])
	}
	if ops.Ops[5].Name != "0123456789ab" {
		t.Fatalf("unnamed: %+v", ops.Ops[5])
	}
	if st := statusOf(endpoints, map[int][]Container{1: list}); len(st) != 2 || st[0].Stack != "paperless" {
		t.Fatalf("status: %+v", st)
	}
}
