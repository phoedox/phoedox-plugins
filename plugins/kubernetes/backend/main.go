//go:build wasip1

// Backend of the Kubernetes plugin: imports nodes, namespaces and workloads
// every hour (and on k8s.sync); pods and warning events for AI clients.
// Read-only.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

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

type kube struct {
	base, cluster string
	system        bool
	h             map[string]string
}

func connect() (*kube, error) {
	raw, err := integ.Setting("url", "the API server, e.g. https://k8s.example.org:6443")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	token, err := integ.Setting("token", "the bearer token of a read-only service account")
	if err != nil {
		return nil, err
	}
	return &kube{
		base:    base,
		cluster: clusterName(integ.Optional("cluster"), base),
		system:  integ.Truthy(integ.Optional("include_system")),
		h:       map[string]string{"Authorization": "Bearer " + strings.TrimSpace(token), "Accept": "application/json"},
	}, nil
}

// list reads a collection in pages of 500 (at most 10000 objects).
func (k *kube) list(path string, page func(body []byte) (string, error)) error {
	next := ""
	for i := 0; i < 20; i++ {
		u := k.base + path + "?limit=500"
		if strings.Contains(path, "?") {
			u = k.base + path + "&limit=500"
		}
		if next != "" {
			u += "&continue=" + url.QueryEscape(next)
		}
		res, err := pdk.Fetch(pdk.FetchRequest{URL: u, Headers: k.h})
		if err != nil {
			return err
		}
		if res.Status != 200 {
			return &integ.StatusError{Service: "Kubernetes", Status: res.Status}
		}
		next, err = page([]byte(res.Body))
		if err != nil {
			return fmt.Errorf("unexpected answer from Kubernetes: %w", err)
		}
		if next == "" {
			return nil
		}
	}
	return nil
}

func listAll[T any](k *kube, path string) ([]T, error) {
	var out []T
	err := k.list(path, func(body []byte) (string, error) {
		items, next, err := parseList[T](body)
		out = append(out, items...)
		return next, err
	})
	return out, err
}

func sync() (map[string]int, error) {
	k, err := connect()
	if err != nil {
		return nil, err
	}
	nodes, err := listAll[Node](k, "/api/v1/nodes")
	if err != nil {
		return nil, err
	}
	namespaces, err := listAll[Namespace](k, "/api/v1/namespaces")
	if err != nil {
		return nil, err
	}
	var workloads []Workload
	for _, w := range []struct{ kind, path string }{
		{"Deployment", "/apis/apps/v1/deployments"},
		{"StatefulSet", "/apis/apps/v1/statefulsets"},
		{"DaemonSet", "/apis/apps/v1/daemonsets"},
	} {
		err := k.list(w.path, func(body []byte) (string, error) {
			items, next, err := parseWorkloads(w.kind, body)
			workloads = append(workloads, items...)
			return next, err
		})
		if err != nil {
			return nil, err
		}
	}
	ops := graphOps(k.cluster, nodes, namespaces, workloads, k.system)
	err = integ.ApplyBatches(ops, 500, func(b *pdk.GraphOps) (map[string]string, error) {
		r, err := pdk.Graph.Apply(b)
		return r.Refs, err
	})
	return map[string]int{"nodes": len(nodes), "workloads": len(workloads)}, err
}

// scope returns the path prefix for an optional namespace argument.
func scope(args json.RawMessage) (prefix string, explicit bool, err error) {
	var in struct {
		Namespace string `json:"namespace"`
	}
	_ = json.Unmarshal(args, &in)
	ns := strings.TrimSpace(in.Namespace)
	if ns == "" {
		return "/api/v1", false, nil
	}
	if err := validNamespace(ns); err != nil {
		return "", false, err
	}
	return "/api/v1/namespaces/" + ns, true, nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "k8s.sync":
		return sync()
	case "k8s.pods", "k8s.events":
		prefix, explicit, err := scope(tc.Args)
		if err != nil {
			return nil, err
		}
		k, err := connect()
		if err != nil {
			return nil, err
		}
		system := k.system || explicit // a namespace asked for by name is shown
		if tc.Name == "k8s.pods" {
			pods, err := listAll[Pod](k, prefix+"/pods")
			if err != nil {
				return nil, err
			}
			return podInfo(pods, system, time.Now()), nil
		}
		events, err := listAll[Event](k, prefix+"/events?fieldSelector="+url.QueryEscape("type=Warning"))
		if err != nil {
			return nil, err
		}
		return warnings(events, system, time.Now()), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
