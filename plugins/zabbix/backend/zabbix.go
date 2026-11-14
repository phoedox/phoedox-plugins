// Logic of the Zabbix plugin, without build tags so it is testable natively.
// Zabbix speaks JSON-RPC 2.0 at /api_jsonrpc.php; since 6.4 an API token
// goes into the Authorization header (no "auth" field in the request).
package main

import (
	"encoding/json"
	"fmt"
	"github.com/phoedox/phoedox-plugins/internal/integ"
	"sort"
	"strconv"
	"strings"
	"time"
)

// hostLimit bounds zabbix.hosts.
const hostLimit = 2000

// apiURL accepts the frontend address with or without /api_jsonrpc.php.
func apiURL(base string) string {
	if strings.HasSuffix(base, "/api_jsonrpc.php") {
		return base
	}
	return base + "/api_jsonrpc.php"
}

func rpcRequest(method string, params any, id int) ([]byte, error) {
	return json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params, "id": id})
}

// RPCError is a JSON-RPC error answer.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

func (e *RPCError) Error() string {
	msg := strings.TrimSuffix(strings.TrimSpace(e.Message), ".")
	if d := strings.TrimSpace(e.Data); d != "" {
		if len(d) > 300 {
			d = d[:300] + "…"
		}
		msg += ": " + d
	}
	return "Zabbix: " + msg
}

// decodeRPC decodes the result of a JSON-RPC answer or returns its error.
func decodeRPC(body []byte, out any) error {
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *RPCError       `json:"error"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("unexpected answer from Zabbix: %w", err)
	}
	if r.Error != nil {
		return r.Error
	}
	if len(r.Result) == 0 {
		return fmt.Errorf("unexpected answer from Zabbix: no result")
	}
	if err := json.Unmarshal(r.Result, out); err != nil {
		return fmt.Errorf("unexpected answer from Zabbix: %w", err)
	}
	return nil
}

// flexInt accepts a number or a numeric string (Zabbix sends strings).
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*f = flexInt(n)
	return nil
}

func problemParams() map[string]any {
	return map[string]any{
		"output":    []string{"eventid", "objectid", "clock", "name", "severity", "acknowledged", "suppressed", "r_eventid", "r_clock"},
		"recent":    true,
		"sortfield": []string{"eventid"},
		"sortorder": "DESC",
		"limit":     100,
	}
}

// Problem is a row of problem.get.
type Problem struct {
	EventID      string  `json:"eventid"`
	Clock        flexInt `json:"clock"`
	Name         string  `json:"name"`
	Severity     flexInt `json:"severity"`
	Acknowledged flexInt `json:"acknowledged"`
	Suppressed   flexInt `json:"suppressed"`
	REventID     string  `json:"r_eventid"`
	RClock       flexInt `json:"r_clock"`
}

// eventParams asks for the hosts of the problem events (problem.get has no
// selectHosts).
func eventParams(probs []Problem) map[string]any {
	ids := make([]string, 0, len(probs))
	for _, p := range probs {
		ids = append(ids, p.EventID)
	}
	return map[string]any{"eventids": ids, "output": []string{"eventid"}, "selectHosts": []string{"hostid"}}
}

// Event is a row of event.get with its hosts.
type Event struct {
	EventID string `json:"eventid"`
	Hosts   []struct {
		HostID string `json:"hostid"`
	} `json:"hosts"`
}

func hostIDs(events []Event) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range events {
		for _, h := range e.Hosts {
			if h.HostID != "" && !seen[h.HostID] {
				seen[h.HostID] = true
				out = append(out, h.HostID)
			}
		}
	}
	return out
}

// hostParams reads hosts with their interfaces (all hosts when ids is nil).
func hostParams(ids []string) map[string]any {
	p := map[string]any{
		"output":           []string{"hostid", "host", "name", "status"},
		"selectInterfaces": []string{"type", "main", "useip", "ip", "dns", "available"},
		"sortfield":        "name",
	}
	if ids != nil {
		p["hostids"] = ids
	} else {
		p["limit"] = hostLimit
	}
	return p
}

// Host is a row of host.get.
type Host struct {
	HostID     string  `json:"hostid"`
	Host       string  `json:"host"`
	Name       string  `json:"name"`
	Status     flexInt `json:"status"`
	Interfaces []Iface `json:"interfaces"`
}

// Iface is a host interface (type 1 agent, 2 SNMP, 3 IPMI, 4 JMX).
type Iface struct {
	Type      flexInt `json:"type"`
	Main      flexInt `json:"main"`
	UseIP     flexInt `json:"useip"`
	IP        string  `json:"ip"`
	DNS       string  `json:"dns"`
	Available flexInt `json:"available"`
}

// main is the main interface, the agent's first.
func (h Host) main() *Iface {
	var best *Iface
	for i := range h.Interfaces {
		f := &h.Interfaces[i]
		if f.Main != 1 {
			continue
		}
		if best == nil || (f.Type == 1 && best.Type != 1) {
			best = f
		}
	}
	if best == nil && len(h.Interfaces) > 0 {
		best = &h.Interfaces[0]
	}
	return best
}

// availability sums up the main interfaces (0 unknown, 1 available,
// 2 unavailable): any unavailable one makes the host unavailable.
func (h Host) availability() string {
	state := "unknown"
	for _, f := range h.Interfaces {
		if f.Main != 1 {
			continue
		}
		switch f.Available {
		case 2:
			return "unavailable"
		case 1:
			state = "available"
		}
	}
	return state
}

func (h Host) candidates() []string {
	c := []string{h.Host, h.Name}
	for _, f := range h.Interfaces {
		c = append(c, f.DNS, f.IP)
	}
	return c
}

var severities = []string{"not classified", "information", "warning", "average", "high", "disaster"}

func severityName(s flexInt) string {
	if s >= 0 && int(s) < len(severities) {
		return severities[s]
	}
	return "unknown"
}

func unixTime(s flexInt) string {
	if s <= 0 {
		return ""
	}
	return time.Unix(int64(s), 0).UTC().Format(time.RFC3339)
}

// ProblemRow is a row of zabbix.problems.
type ProblemRow struct {
	EventID      string       `json:"eventId"`
	Name         string       `json:"name"`
	Severity     string       `json:"severity"`
	Acknowledged bool         `json:"acknowledged"`
	Suppressed   bool         `json:"suppressed,omitempty"`
	Since        string       `json:"since"`
	Resolved     bool         `json:"resolved"`
	ResolvedAt   string       `json:"resolvedAt,omitempty"`
	Hosts        []string     `json:"hosts"`
	Entry        *integ.Entry `json:"entry,omitempty"`
}

func problemRows(probs []Problem, events []Event, hosts []Host, m *integ.Matcher) []ProblemRow {
	byID := map[string]Host{}
	for _, h := range hosts {
		byID[h.HostID] = h
	}
	hostsOf := map[string][]Host{}
	for _, e := range events {
		for _, h := range e.Hosts {
			if host, ok := byID[h.HostID]; ok {
				hostsOf[e.EventID] = append(hostsOf[e.EventID], host)
			}
		}
	}
	out := make([]ProblemRow, 0, len(probs))
	for _, p := range probs {
		row := ProblemRow{
			EventID:      p.EventID,
			Name:         p.Name,
			Severity:     severityName(p.Severity),
			Acknowledged: p.Acknowledged == 1,
			Suppressed:   p.Suppressed == 1,
			Since:        unixTime(p.Clock),
			Resolved:     p.REventID != "" && p.REventID != "0",
			Hosts:        []string{},
		}
		if row.Resolved {
			row.ResolvedAt = unixTime(p.RClock)
		}
		for _, h := range hostsOf[p.EventID] {
			row.Hosts = append(row.Hosts, displayName(h))
			if row.Entry == nil {
				row.Entry = m.Match(h.candidates()...)
			}
		}
		out = append(out, row)
	}
	return out
}

func displayName(h Host) string {
	if h.Name != "" {
		return h.Name
	}
	return h.Host
}

// HostRow is a row of zabbix.hosts.
type HostRow struct {
	Host      string       `json:"host"`
	Name      string       `json:"name"`
	Status    string       `json:"status"`
	Available string       `json:"available"`
	IP        string       `json:"ip,omitempty"`
	DNS       string       `json:"dns,omitempty"`
	Entry     *integ.Entry `json:"entry,omitempty"`
}

func hostRows(hosts []Host, m *integ.Matcher) []HostRow {
	out := make([]HostRow, 0, len(hosts))
	for _, h := range hosts {
		row := HostRow{Host: h.Host, Name: h.Name, Status: "enabled", Available: h.availability(), Entry: m.Match(h.candidates()...)}
		if h.Status == 1 {
			row.Status = "disabled"
		}
		if f := h.main(); f != nil {
			row.IP, row.DNS = f.IP, f.DNS
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}
