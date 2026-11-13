package main

import "testing"

func TestProxmox(t *testing.T) {
	body := []byte(`{"data":[
		{"type":"node","node":"pve1","status":"online","maxmem":68719476736,"maxcpu":16,"cpu":0.05,"mem":21474836480,"uptime":1000},
		{"type":"qemu","node":"pve1","vmid":101,"name":"Home Assistant","status":"running","maxcpu":2,"maxmem":4294967296,"tags":"smarthome;prod"},
		{"type":"lxc","node":"pve1","vmid":200,"name":"paperless","status":"stopped","maxmem":2147483648},
		{"type":"storage","node":"pve1","storage":"local"}]}`)
	list, err := parseResources(body)
	if err != nil || len(list) != 4 {
		t.Fatal(err)
	}
	ops := graphOps(list)
	if ops.Ops[0].Kind != "server" || ops.Ops[0].ID != "pve-pve1" || ops.Ops[0].Attrs["ram"] != 64.0 {
		t.Fatalf("node: %+v", ops.Ops[0])
	}
	if ops.Ops[1].Kind != "vm" || ops.Ops[1].ID != "pve-101-home-assistant" || ops.Ops[1].Attrs["tags"] != "smarthome, prod" || ops.Ops[1].Attrs["ram"] != 4.0 {
		t.Fatalf("vm: %+v", ops.Ops[1])
	}
	if ops.Ops[2].Op != "link" || ops.Ops[2].Rel != "runs_on" || ops.Ops[2].To != ops.Ops[0].Ref {
		t.Fatalf("link: %+v", ops.Ops[2])
	}
	if ops.Ops[3].Kind != "lxc" || ops.Ops[3].Attrs["ctid"] != 200 {
		t.Fatalf("lxc: %+v", ops.Ops[3])
	}
	if st := statusOf(list); len(st) != 3 || st[0].CPU != 5 || st[0].MemGiB != 20 {
		t.Fatalf("status: %+v", st)
	}
}
