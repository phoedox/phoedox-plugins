// Logic of the Checkmk plugin, without build tags so it is testable
// natively. The REST API lives at <site>/check_mk/api/1.0; the monitoring
// tables (Livestatus) are the collections of the domain types service and
// host, each row's columns in "extensions".
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/phoedox/phoedox-plugins/internal/integ"
)

// apiBase accepts the site address with or without /check_mk[/api/1.0].
func apiBase(base string) string {
	base = strings.TrimSuffix(base, "/check_mk/api/1.0")
	base = strings.TrimSuffix(base, "/check_mk")
	return base + "/check_mk/api/1.0"
}

func columns(cols ...string) string {
	var b strings.Builder
	for _, c := range cols {
		b.WriteString("&columns=" + url.QueryEscape(c))
	}
	return b.String()
}

// problemsPath lists services whose state is not OK.
func problemsPath() string {
	q := `{"op":"!=","left":"state","right":"0"}`
	return "/domain-types/service/collections/all?query=" + url.QueryEscape(q) +
		columns("host_name", "host_address", "description", "state", "plugin_output", "last_state_change", "acknowledged")
}

func hostsPath() string {
	return "/domain-types/host/collections/all?" + strings.TrimPrefix(columns("name", "address", "state"), "&")
}

// apiError reads the problem details Checkmk sends with errors.
func apiError(status int, body []byte) error {
	var p struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(body, &p)
	msg := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(p.Title), ".") + ": " + strings.TrimSpace(p.Detail))
	msg = strings.Trim(msg, ": ")
	if status == 401 {
		return fmt.Errorf("Checkmk rejected username or secret (HTTP 401)")
	}
	if msg == "" {
		return &integ.StatusError{Service: "Checkmk", Status: status}
	}
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return fmt.Errorf("Checkmk answered HTTP %d: %s", status, msg)
}

// collection is the answer of a .../collections/all request.
type collection[T any] struct {
	Value []struct {
		Extensions T `json:"extensions"`
	} `json:"value"`
}

func parse[T any](body []byte) ([]T, error) {
	var c collection[T]
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, fmt.Errorf("unexpected answer from Checkmk: %w", err)
	}
	out := make([]T, 0, len(c.Value))
	for _, v := range c.Value {
		out = append(out, v.Extensions)
	}
	return out, nil
}

// Service holds the columns of a service row.
type Service struct {
	HostName        string  `json:"host_name"`
	HostAddress     string  `json:"host_address"`
	Description     string  `json:"description"`
	State           int     `json:"state"`
	PluginOutput    string  `json:"plugin_output"`
	LastStateChange float64 `json:"last_state_change"`
	Acknowledged    int     `json:"acknowledged"`
}

func parseServices(body []byte) ([]Service, error) { return parse[Service](body) }

// Host holds the columns of a host row.
type Host struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	State   int    `json:"state"`
}

func parseHosts(body []byte) ([]Host, error) { return parse[Host](body) }

func serviceState(s int) string {
	switch s {
	case 0:
		return "OK"
	case 1:
		return "WARN"
	case 2:
		return "CRIT"
	case 3:
		return "UNKNOWN"
	}
	return "UNKNOWN"
}

func hostState(s int) string {
	switch s {
	case 0:
		return "UP"
	case 1:
		return "DOWN"
	case 2:
		return "UNREACHABLE"
	}
	return "UNKNOWN"
}

// ProblemRow is a row of checkmk.problems.
type ProblemRow struct {
	Host         string       `json:"host"`
	Service      string       `json:"service"`
	State        string       `json:"state"`
	Output       string       `json:"output,omitempty"`
	Since        string       `json:"since,omitempty"`
	Acknowledged bool         `json:"acknowledged"`
	Entry        *integ.Entry `json:"entry,omitempty"`
}

// severity orders problems: CRIT, UNKNOWN, WARN.
var severity = map[int]int{2: 0, 3: 1, 1: 2}

func problemRows(list []Service, m *integ.Matcher) []ProblemRow {
	sort.SliceStable(list, func(i, j int) bool {
		if severity[list[i].State] != severity[list[j].State] {
			return severity[list[i].State] < severity[list[j].State]
		}
		return strings.ToLower(list[i].HostName) < strings.ToLower(list[j].HostName)
	})
	out := make([]ProblemRow, 0, len(list))
	for _, s := range list {
		if s.State == 0 {
			continue
		}
		row := ProblemRow{Host: s.HostName, Service: s.Description, State: serviceState(s.State), Output: s.PluginOutput, Acknowledged: s.Acknowledged > 0}
		if s.LastStateChange > 0 {
			row.Since = time.Unix(int64(s.LastStateChange), 0).UTC().Format(time.RFC3339)
		}
		row.Entry = m.Match(s.HostAddress, s.HostName)
		out = append(out, row)
	}
	return out
}

// HostRow is a row of checkmk.hosts.
type HostRow struct {
	Name    string       `json:"name"`
	Address string       `json:"address,omitempty"`
	State   string       `json:"state"`
	Entry   *integ.Entry `json:"entry,omitempty"`
}

func hostRows(list []Host, m *integ.Matcher) []HostRow {
	out := make([]HostRow, 0, len(list))
	for _, h := range list {
		out = append(out, HostRow{Name: h.Name, Address: h.Address, State: hostState(h.State), Entry: m.Match(h.Address, h.Name)})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}
