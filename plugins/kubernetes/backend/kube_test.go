package main

import (
	"errors"
	"testing"
	"time"

	"github.com/phoedox/phoedox-plugins/internal/integ"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const nodesJSON = `{"kind":"NodeList","apiVersion":"v1","metadata":{"resourceVersion":"123"},"items":[
 {"metadata":{"name":"cp-1","uid":"0b6f","labels":{"node-role.kubernetes.io/control-plane":"","kubernetes.io/os":"linux"}},
  "status":{"capacity":{"cpu":"4","memory":"8148696Ki","pods":"110"},
   "addresses":[{"type":"InternalIP","address":"10.0.0.10"},{"type":"Hostname","address":"cp-1.lan"}],
   "nodeInfo":{"osImage":"Ubuntu 24.04.1 LTS","kernelVersion":"6.8.0-45-generic","kubeletVersion":"v1.31.2","containerRuntimeVersion":"containerd://1.7.22","architecture":"amd64"},
   "conditions":[{"type":"MemoryPressure","status":"False"},{"type":"Ready","status":"True"}]}},
 {"metadata":{"name":"worker-1"},"status":{"capacity":{"cpu":"3920m","memory":"16Gi"},"addresses":[{"type":"InternalIP","address":"10.0.0.11"}],"conditions":[{"type":"Ready","status":"Unknown"}]}}]}`

const deploymentsJSON = `{"kind":"DeploymentList","metadata":{"continue":"eyJ2IjoibWV0YS"},"items":[
 {"metadata":{"name":"web","namespace":"shop","uid":"a1"},"spec":{"replicas":3,"template":{"spec":{"containers":[{"name":"web","image":"ghcr.io/acme/shop:1.4.2"},{"name":"proxy","image":"envoyproxy/envoy:v1.31"}]}}},"status":{"replicas":3,"readyReplicas":2}},
 {"metadata":{"name":"coredns","namespace":"kube-system"},"spec":{"replicas":2,"template":{"spec":{"containers":[{"image":"registry.k8s.io/coredns/coredns:v1.11.3"}]}}},"status":{"readyReplicas":2}},
 {"metadata":{"name":"worker","namespace":"shop"},"spec":{"template":{"spec":{"containers":[{"image":"ghcr.io/acme/worker:2"}]}}},"status":{}}]}`

const statefulJSON = `{"items":[{"metadata":{"name":"postgres","namespace":"db"},"spec":{"replicas":1,"template":{"spec":{"containers":[{"image":"postgres:17"}]}}},"status":{"readyReplicas":1}}]}`

const daemonJSON = `{"items":[{"metadata":{"name":"node-exporter","namespace":"monitoring"},"spec":{"template":{"spec":{"containers":[{"image":"prom/node-exporter:v1.8.2"}]}}},"status":{"desiredNumberScheduled":2,"numberReady":1}}]}`

const namespacesJSON = `{"items":[{"metadata":{"name":"shop"},"status":{"phase":"Active"}},{"metadata":{"name":"db"},"status":{"phase":"Active"}},{"metadata":{"name":"empty"},"status":{"phase":"Active"}},{"metadata":{"name":"kube-system"},"status":{"phase":"Active"}}]}`

func fixtures(t *testing.T) ([]Node, []Namespace, []Workload) {
	t.Helper()
	nodes, next, err := parseList[Node]([]byte(nodesJSON))
	if err != nil || next != "" || len(nodes) != 2 {
		t.Fatal(nodes, next, err)
	}
	ns, _, err := parseList[Namespace]([]byte(namespacesJSON))
	if err != nil {
		t.Fatal(err)
	}
	var all []Workload
	for _, f := range []struct{ kind, body string }{{"Deployment", deploymentsJSON}, {"StatefulSet", statefulJSON}, {"DaemonSet", daemonJSON}} {
		w, next, err := parseWorkloads(f.kind, []byte(f.body))
		if err != nil {
			t.Fatal(err)
		}
		if f.kind == "Deployment" && next != "eyJ2IjoibWV0YS" {
			t.Fatalf("continue token: %q", next)
		}
		all = append(all, w...)
	}
	return nodes, ns, all
}

func TestGraphOps(t *testing.T) {
	nodes, ns, workloads := fixtures(t)
	ops := graphOps("prod", nodes, ns, workloads, false).Ops
	cp := ops[0]
	if cp.Kind != "server" || cp.ID != "k8s-prod-cp-1" || cp.Name != "cp-1" || cp.Attrs["hostname"] != "cp-1.lan" || cp.Attrs["ip"] != "10.0.0.10" ||
		cp.Attrs["os"] != "Ubuntu 24.04.1 LTS" || cp.Attrs["kubelet_version"] != "v1.31.2" || cp.Attrs["cpu"] != 4.0 || cp.Attrs["ram"] != 7.8 ||
		cp.Attrs["roles"] != "control-plane" || cp.Attrs["ready"] != true || cp.Attrs["cluster"] != "prod" {
		t.Fatalf("control plane: %+v", cp)
	}
	if w := ops[1]; w.Attrs["cpu"] != 3.92 || w.Attrs["ram"] != 16.0 || w.Attrs["ready"] != false || w.Attrs["hostname"] != "worker-1" || w.Attrs["os"] != nil {
		t.Fatalf("worker: %+v", w)
	}
	// Namespaces sorted: db, monitoring, shop; kube-system and empty skipped.
	want := []struct{ op, kind, id string }{
		{"upsert", "stack", "k8s-prod-db"},
		{"upsert", "container", "k8s-prod-db-sts-postgres"},
		{"link", "", ""},
		{"upsert", "stack", "k8s-prod-monitoring"},
		{"upsert", "container", "k8s-prod-monitoring-ds-node-exporter"},
		{"link", "", ""},
		{"upsert", "stack", "k8s-prod-shop"},
		{"upsert", "container", "k8s-prod-shop-deploy-web"},
		{"link", "", ""},
		{"upsert", "container", "k8s-prod-shop-deploy-worker"},
		{"link", "", ""},
	}
	rest := ops[2:]
	if len(rest) != len(want) {
		t.Fatalf("%d ops: %+v", len(rest), rest)
	}
	for i, w := range want {
		if rest[i].Op != w.op || rest[i].Kind != w.kind || rest[i].ID != w.id {
			t.Fatalf("op %d: %+v, want %+v", i, rest[i], w)
		}
	}
	if s := rest[0]; s.Attrs["status"] != "active" || s.Attrs["namespace"] != "db" {
		t.Fatalf("stack: %+v", s)
	}
	if l := rest[2]; l.Rel != "part_of" || l.From != rest[1].Ref || l.To != rest[0].Ref {
		t.Fatalf("link: %+v", l)
	}
	ds := rest[4]
	if ds.Attrs["workload"] != "DaemonSet" || ds.Attrs["replicas_ready"] != 1 || ds.Attrs["replicas_desired"] != 2 {
		t.Fatalf("daemonset: %+v", ds)
	}
	web := rest[7]
	if web.Attrs["image"] != "ghcr.io/acme/shop:1.4.2" || web.Attrs["images"] != "ghcr.io/acme/shop:1.4.2, envoyproxy/envoy:v1.31" || web.Attrs["replicas_ready"] != 2 || web.Attrs["replicas_desired"] != 3 {
		t.Fatalf("deployment: %+v", web)
	}
	if wk := rest[9]; wk.Attrs["replicas_desired"] != 1 || wk.Attrs["replicas_ready"] != 0 || wk.Attrs["images"] != nil {
		t.Fatalf("default replicas: %+v", wk)
	}

	withSystem := graphOps("prod", nil, ns, workloads, true).Ops
	found := false
	for _, o := range withSystem {
		if o.ID == "k8s-prod-kube-system-deploy-coredns" {
			found = true
		}
	}
	if !found {
		t.Fatal("include_system should import kube-system")
	}
}

func TestQuantity(t *testing.T) {
	for in, want := range map[string]float64{"4": 4, "3920m": 3.92, "16Gi": 16 << 30, "8148696Ki": 8148696 * 1024, "1G": 1e9, "500M": 5e8, "2e3": 2000} {
		if got, ok := parseQuantity(in); !ok || got != want {
			t.Errorf("%s: %v %v", in, got, ok)
		}
	}
	for _, bad := range []string{"", "abc", "Gi"} {
		if _, ok := parseQuantity(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestPodsAndEvents(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	pods, _, err := parseList[Pod]([]byte(`{"items":[
	 {"metadata":{"name":"web-7d4b9c-x2x9k","namespace":"shop","creationTimestamp":"2026-11-10T09:30:00Z"},"spec":{"nodeName":"worker-1"},
	  "status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":2},{"ready":false,"restartCount":5,"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}},
	 {"metadata":{"name":"coredns-1","namespace":"kube-system","creationTimestamp":"2026-11-14T11:59:30Z"},"status":{"phase":"Running"}},
	 {"metadata":{"name":"job-1","namespace":"batch","creationTimestamp":"2026-11-14T11:47:00Z"},"status":{"phase":"Pending","reason":"Unschedulable"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	info := podInfo(pods, false, now)
	if len(info) != 2 {
		t.Fatalf("kube-system pods should be skipped: %+v", info)
	}
	if p := info[0]; p.Restarts != 7 || p.Ready != "1/2" || p.Reason != "CrashLoopBackOff" || p.Node != "worker-1" || p.Age != "4d2h" {
		t.Fatalf("pod: %+v", p)
	}
	if p := info[1]; p.Ready != "0/0" || p.Reason != "Unschedulable" || p.Age != "13m" {
		t.Fatalf("pending pod: %+v", p)
	}
	if all := podInfo(pods, true, now); len(all) != 3 || all[1].Age != "30s" {
		t.Fatalf("with system: %+v", all)
	}

	events, _, err := parseList[Event]([]byte(`{"items":[
	 {"metadata":{"namespace":"shop","creationTimestamp":"2026-11-14T11:00:00Z"},"type":"Warning","reason":"BackOff","message":"Back-off restarting failed container web","count":12,"involvedObject":{"kind":"Pod","name":"web-7d4b9c-x2x9k"},"firstTimestamp":"2026-11-14T10:00:00Z","lastTimestamp":"2026-11-14T11:58:00Z"},
	 {"metadata":{"namespace":"shop"},"type":"Warning","reason":"FailedScheduling","message":"0/2 nodes are available","involvedObject":{"kind":"Pod","name":"job-1"},"eventTime":"2026-11-14T11:30:00.000000Z","series":{"count":4,"lastObservedTime":"2026-11-14T11:59:00.000000Z"}},
	 {"metadata":{"namespace":"shop"},"type":"Warning","reason":"Old","involvedObject":{"kind":"Pod","name":"x"},"lastTimestamp":"2026-11-14T09:00:00Z"},
	 {"metadata":{"namespace":"shop"},"type":"Normal","reason":"Pulled","involvedObject":{"kind":"Pod","name":"x"},"lastTimestamp":"2026-11-14T11:59:00Z"},
	 {"metadata":{"namespace":"kube-system"},"type":"Warning","reason":"Unhealthy","involvedObject":{"kind":"Pod","name":"y"},"lastTimestamp":"2026-11-14T11:59:00Z"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	w := warnings(events, false, now)
	if len(w) != 2 || w[0].Reason != "FailedScheduling" || w[0].Count != 4 || w[1].Object != "Pod/web-7d4b9c-x2x9k" || w[1].Count != 12 {
		t.Fatalf("warnings: %+v", w)
	}
}

func TestHelpers(t *testing.T) {
	if clusterName("Prod EU", "https://x:6443") != "prod-eu" || clusterName("", "https://k8s.example.org:6443") != "k8s-example-org" {
		t.Fatal("cluster name")
	}
	if validNamespace("shop-1") != nil || validNamespace("Shop") == nil || validNamespace("a/b") == nil || validNamespace("") == nil {
		t.Fatal("namespace validation")
	}
	if age(90*time.Minute) != "1h30m" || age(-time.Second) != "0s" {
		t.Fatal("age")
	}
}

func TestApplyBatches(t *testing.T) {
	var ops pdk.GraphOps
	a := ops.Upsert("stack", "a", nil)
	b := ops.Upsert("container", "b", nil)
	ops.Link(b, "part_of", a, nil)
	var calls [][]pdk.GraphOp
	err := integ.ApplyBatches(ops, 2, func(g *pdk.GraphOps) (map[string]string, error) {
		calls = append(calls, g.Ops)
		refs := map[string]string{}
		for _, o := range g.Ops {
			if o.Ref != "" {
				refs[o.Ref] = o.ID
			}
		}
		return refs, nil
	})
	if err != nil || len(calls) != 2 || calls[1][0].From != "b" || calls[1][0].To != "a" {
		t.Fatalf("%v %+v", err, calls)
	}
	if err := integ.ApplyBatches(ops, 500, func(*pdk.GraphOps) (map[string]string, error) { return nil, errors.New("denied") }); err == nil {
		t.Fatal("error lost")
	}
}
