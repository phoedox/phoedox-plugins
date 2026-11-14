// Logic of the Kubernetes plugin, without build tags so it is testable natively.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// systemNamespace is skipped unless include_system is set.
const systemNamespace = "kube-system"

var nsRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

// validNamespace checks a namespace argument (a DNS label).
func validNamespace(ns string) error {
	if !nsRe.MatchString(ns) {
		return fmt.Errorf("namespace %q is not a valid Kubernetes namespace name", ns)
	}
	return nil
}

// clusterName is the cluster part of entry IDs: the setting or the host of
// the API server address.
func clusterName(setting, base string) string {
	if s := integ.Slugify(setting); s != "" {
		return s
	}
	if u, err := url.Parse(base); err == nil {
		if s := integ.Slugify(u.Hostname()); s != "" {
			return s
		}
	}
	return "cluster"
}

// Meta is the object metadata.
type Meta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	UID               string            `json:"uid"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels"`
}

// List is a Kubernetes list answer.
type List[T any] struct {
	Metadata struct {
		Continue string `json:"continue"`
	} `json:"metadata"`
	Items []T `json:"items"`
}

func parseList[T any](body []byte) ([]T, string, error) {
	var l List[T]
	if err := json.Unmarshal(body, &l); err != nil {
		return nil, "", err
	}
	return l.Items, l.Metadata.Continue, nil
}

// Node is a cluster node.
type Node struct {
	Metadata Meta `json:"metadata"`
	Status   struct {
		Capacity  map[string]string `json:"capacity"`
		Addresses []struct {
			Type    string `json:"type"`
			Address string `json:"address"`
		} `json:"addresses"`
		NodeInfo struct {
			OSImage          string `json:"osImage"`
			KernelVersion    string `json:"kernelVersion"`
			KubeletVersion   string `json:"kubeletVersion"`
			ContainerRuntime string `json:"containerRuntimeVersion"`
			Architecture     string `json:"architecture"`
		} `json:"nodeInfo"`
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
	} `json:"status"`
}

func (n Node) address(typ string) string {
	for _, a := range n.Status.Addresses {
		if a.Type == typ {
			return a.Address
		}
	}
	return ""
}

func (n Node) roles() string {
	var r []string
	for k := range n.Metadata.Labels {
		if role, ok := strings.CutPrefix(k, "node-role.kubernetes.io/"); ok && role != "" {
			r = append(r, role)
		}
	}
	sort.Strings(r)
	return strings.Join(r, ", ")
}

func (n Node) ready() (bool, bool) {
	for _, c := range n.Status.Conditions {
		if c.Type == "Ready" {
			return c.Status == "True", true
		}
	}
	return false, false
}

// Namespace is a namespace.
type Namespace struct {
	Metadata Meta `json:"metadata"`
	Status   struct {
		Phase string `json:"phase"`
	} `json:"status"`
}

// Workload is a Deployment, StatefulSet or DaemonSet. List items carry no
// kind, so Kind is set by the caller.
type Workload struct {
	Kind     string `json:"-"`
	Metadata Meta   `json:"metadata"`
	Spec     struct {
		Replicas *int `json:"replicas"`
		Template struct {
			Spec struct {
				Containers []struct {
					Name  string `json:"name"`
					Image string `json:"image"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas          int `json:"readyReplicas"`
		NumberReady            int `json:"numberReady"`
		DesiredNumberScheduled int `json:"desiredNumberScheduled"`
	} `json:"status"`
}

func (w Workload) replicas() (ready, desired int) {
	if w.Kind == "DaemonSet" {
		return w.Status.NumberReady, w.Status.DesiredNumberScheduled
	}
	desired = 1 // the API default when spec.replicas is unset
	if w.Spec.Replicas != nil {
		desired = *w.Spec.Replicas
	}
	return w.Status.ReadyReplicas, desired
}

// parseWorkloads reads a list of apps/v1 workloads of one kind.
func parseWorkloads(kind string, body []byte) ([]Workload, string, error) {
	items, next, err := parseList[Workload](body)
	for i := range items {
		items[i].Kind = kind
	}
	return items, next, err
}

var kindShort = map[string]string{"Deployment": "deploy", "StatefulSet": "sts", "DaemonSet": "ds"}

var (
	binarySuffix  = map[string]float64{"Ki": 1 << 10, "Mi": 1 << 20, "Gi": 1 << 30, "Ti": 1 << 40, "Pi": 1 << 50, "Ei": 1 << 60}
	decimalSuffix = map[byte]float64{'n': 1e-9, 'u': 1e-6, 'm': 1e-3, 'k': 1e3, 'M': 1e6, 'G': 1e9, 'T': 1e12, 'P': 1e15, 'E': 1e18}
)

// parseQuantity reads a Kubernetes quantity like "4", "3920m", "16Gi" or
// "16323484Ki".
func parseQuantity(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	mult := 1.0
	if len(s) > 2 && s[len(s)-1] == 'i' {
		if m, ok := binarySuffix[s[len(s)-2:]]; ok {
			s, mult = s[:len(s)-2], m
		}
	} else if m, ok := decimalSuffix[s[len(s)-1]]; ok {
		s, mult = s[:len(s)-1], m
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f * mult, true
}

// graphOps: nodes as servers, namespaces with workloads as stacks, workloads
// as containers part of their namespace.
func graphOps(cluster string, nodes []Node, namespaces []Namespace, workloads []Workload, includeSystem bool) pdk.GraphOps {
	var ops pdk.GraphOps
	base := "k8s-" + cluster
	for _, n := range nodes {
		if n.Metadata.Name == "" {
			continue
		}
		info := n.Status.NodeInfo
		attrs := map[string]any{"cluster": cluster, "hostname": n.Metadata.Name}
		if h := n.address("Hostname"); h != "" {
			attrs["hostname"] = h
		}
		set := func(k, v string) {
			if v != "" {
				attrs[k] = v
			}
		}
		set("ip", n.address("InternalIP"))
		set("external_ip", n.address("ExternalIP"))
		set("os", info.OSImage)
		set("kernel", info.KernelVersion)
		set("kubelet_version", info.KubeletVersion)
		set("container_runtime", info.ContainerRuntime)
		set("architecture", info.Architecture)
		set("roles", n.roles())
		if cpu, ok := parseQuantity(n.Status.Capacity["cpu"]); ok {
			attrs["cpu"] = math.Round(cpu*100) / 100
		}
		if mem, ok := parseQuantity(n.Status.Capacity["memory"]); ok {
			attrs["ram"] = integ.GiB(mem)
		}
		if ready, ok := n.ready(); ok {
			attrs["ready"] = ready
		}
		ops.UpsertNamed("server", base+"-"+integ.Slugify(n.Metadata.Name), n.Metadata.Name, attrs)
	}

	phase := map[string]string{}
	for _, ns := range namespaces {
		phase[ns.Metadata.Name] = ns.Status.Phase
	}
	byNS := map[string][]Workload{}
	for _, w := range workloads {
		ns := w.Metadata.Namespace
		if ns == "" || w.Metadata.Name == "" || (ns == systemNamespace && !includeSystem) {
			continue
		}
		byNS[ns] = append(byNS[ns], w)
	}
	names := make([]string, 0, len(byNS))
	for ns := range byNS {
		names = append(names, ns)
	}
	sort.Strings(names)
	for _, ns := range names {
		attrs := map[string]any{"cluster": cluster, "namespace": ns}
		if p := phase[ns]; p != "" {
			attrs["status"] = strings.ToLower(p)
		}
		stack := ops.UpsertNamed("stack", base+"-"+integ.Slugify(ns), ns, attrs)
		for _, w := range byNS[ns] {
			ready, desired := w.replicas()
			a := map[string]any{"cluster": cluster, "namespace": ns, "workload": w.Kind, "replicas_ready": ready, "replicas_desired": desired}
			var images []string
			for _, c := range w.Spec.Template.Spec.Containers {
				if c.Image != "" && !slices.Contains(images, c.Image) {
					images = append(images, c.Image)
				}
			}
			if len(images) > 0 {
				a["image"] = images[0]
			}
			if len(images) > 1 {
				a["images"] = strings.Join(images, ", ")
			}
			short := kindShort[w.Kind]
			if short == "" {
				short = integ.Slugify(w.Kind)
			}
			ref := ops.UpsertNamed("container", base+"-"+integ.Slugify(ns)+"-"+short+"-"+integ.Slugify(w.Metadata.Name), w.Metadata.Name, a)
			ops.Link(ref, "part_of", stack, nil)
		}
	}
	return ops
}

// Pod is a pod.
type Pod struct {
	Metadata Meta `json:"metadata"`
	Spec     struct {
		NodeName string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		Phase             string `json:"phase"`
		Reason            string `json:"reason"`
		ContainerStatuses []struct {
			Ready        bool `json:"ready"`
			RestartCount int  `json:"restartCount"`
			State        struct {
				Waiting *struct {
					Reason string `json:"reason"`
				} `json:"waiting"`
			} `json:"state"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

// PodInfo is a pod for AI clients.
type PodInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Phase     string `json:"phase"`
	Reason    string `json:"reason,omitempty"`
	Ready     string `json:"ready"`
	Restarts  int    `json:"restarts"`
	Node      string `json:"node,omitempty"`
	Age       string `json:"age"`
}

// age formats a duration like kubectl (45s, 12m, 5h3m, 4d2h).
func age(d time.Duration) string {
	switch {
	case d < 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}

// podInfo summarizes pods; kube-system only with includeSystem.
func podInfo(pods []Pod, includeSystem bool, now time.Time) []PodInfo {
	out := []PodInfo{}
	for _, p := range pods {
		if p.Metadata.Namespace == systemNamespace && !includeSystem {
			continue
		}
		i := PodInfo{Namespace: p.Metadata.Namespace, Name: p.Metadata.Name, Phase: p.Status.Phase, Reason: p.Status.Reason, Node: p.Spec.NodeName}
		ready := 0
		for _, c := range p.Status.ContainerStatuses {
			i.Restarts += c.RestartCount
			if c.Ready {
				ready++
			}
			if c.State.Waiting != nil && c.State.Waiting.Reason != "" && i.Reason == "" {
				i.Reason = c.State.Waiting.Reason // e.g. CrashLoopBackOff
			}
		}
		i.Ready = fmt.Sprintf("%d/%d", ready, len(p.Status.ContainerStatuses))
		if !p.Metadata.CreationTimestamp.IsZero() {
			i.Age = age(now.Sub(p.Metadata.CreationTimestamp))
		}
		out = append(out, i)
	}
	return out
}

// Event is a core/v1 event.
type Event struct {
	Metadata       Meta   `json:"metadata"`
	Type           string `json:"type"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	Count          int    `json:"count"`
	InvolvedObject struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"involvedObject"`
	FirstTimestamp *time.Time `json:"firstTimestamp"`
	LastTimestamp  *time.Time `json:"lastTimestamp"`
	EventTime      *time.Time `json:"eventTime"`
	Series         *struct {
		Count            int        `json:"count"`
		LastObservedTime *time.Time `json:"lastObservedTime"`
	} `json:"series"`
}

// last is the time the event was last seen.
func (e Event) last() time.Time {
	for _, t := range []*time.Time{e.LastTimestamp, seriesTime(e), e.EventTime, e.FirstTimestamp, &e.Metadata.CreationTimestamp} {
		if t != nil && !t.IsZero() {
			return *t
		}
	}
	return time.Time{}
}

func seriesTime(e Event) *time.Time {
	if e.Series == nil {
		return nil
	}
	return e.Series.LastObservedTime
}

// EventInfo is a warning event for AI clients.
type EventInfo struct {
	Namespace string    `json:"namespace"`
	Object    string    `json:"object"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Count     int       `json:"count"`
	Last      time.Time `json:"last"`
}

// warnings keeps warning events seen in the last hour, newest first (at
// most 200).
func warnings(events []Event, includeSystem bool, now time.Time) []EventInfo {
	out := []EventInfo{}
	for _, e := range events {
		if e.Type != "Warning" || (e.Metadata.Namespace == systemNamespace && !includeSystem) {
			continue
		}
		last := e.last()
		if last.IsZero() || now.Sub(last) > time.Hour {
			continue
		}
		count := e.Count
		if e.Series != nil && e.Series.Count > count {
			count = e.Series.Count
		}
		if count == 0 {
			count = 1
		}
		msg := strings.TrimSpace(e.Message)
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		out = append(out, EventInfo{Namespace: e.Metadata.Namespace, Object: e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name, Reason: e.Reason, Message: msg, Count: count, Last: last})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Last.After(out[j].Last) })
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}
