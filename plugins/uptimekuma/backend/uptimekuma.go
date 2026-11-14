// Logic of the Uptime Kuma plugin, without build tags so it is testable
// natively. Uptime Kuma has no full REST API; the public status page JSON
// (/api/status-page/<slug> and /api/status-page/heartbeat/<slug>) carries
// monitors, groups, heartbeats and uptime.
package main

import (
	"encoding/json"
	"github.com/phoedox/phoedox-plugins/internal/integ"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

var slugRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func validSlug(s string) bool { return slugRe.MatchString(s) }

// StatusPage is the answer of /api/status-page/<slug>.
type StatusPage struct {
	Config struct {
		Title string `json:"title"`
	} `json:"config"`
	Groups []struct {
		Name     string    `json:"name"`
		Monitors []kumaMon `json:"monitorList"`
	} `json:"publicGroupList"`
}

type kumaMon struct {
	ID       flexID `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	Hostname string `json:"hostname"`
}

// flexID accepts a number or a string.
type flexID string

func (f *flexID) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*f = flexID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexID(n.String())
	return nil
}

// Heartbeats is the answer of /api/status-page/heartbeat/<slug>.
type Heartbeats struct {
	List   map[string][]Beat  `json:"heartbeatList"`
	Uptime map[string]float64 `json:"uptimeList"`
}

// Beat is one heartbeat.
type Beat struct {
	Status int    `json:"status"`
	Time   string `json:"time"`
	Msg    string `json:"msg"`
}

// Monitor is a tool row.
type Monitor struct {
	Name      string       `json:"name"`
	Group     string       `json:"group,omitempty"`
	Type      string       `json:"type,omitempty"`
	Status    string       `json:"status"`
	Message   string       `json:"message,omitempty"`
	LastCheck string       `json:"lastCheck,omitempty"`
	Uptime24h *float64     `json:"uptime24h,omitempty"`
	Entry     *integ.Entry `json:"entry,omitempty"`
}

// statusName maps a heartbeat status.
func statusName(s int) string {
	switch s {
	case 0:
		return "down"
	case 1:
		return "up"
	case 2:
		return "pending"
	case 3:
		return "maintenance"
	}
	return "unknown"
}

// latest picks the newest heartbeat (Uptime Kuma sends them oldest first,
// but the order is not relied on).
func latest(list []Beat) *Beat {
	var best *Beat
	for i := range list {
		if best == nil || list[i].Time >= best.Time {
			best = &list[i]
		}
	}
	return best
}

// isoTime turns "2024-05-01 12:00:00.123" (UTC) into RFC 3339.
func isoTime(s string) string {
	for _, layout := range []string{"2006-01-02 15:04:05.999", "2006-01-02 15:04:05", time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}

var bracket = regexp.MustCompile(`\s*[\(\[].*?[\)\]]\s*`)

func monitors(page StatusPage, beats Heartbeats, m *integ.Matcher) []Monitor {
	var out []Monitor
	seen := map[flexID]bool{}
	for _, g := range page.Groups {
		for _, km := range g.Monitors {
			if seen[km.ID] {
				continue
			}
			seen[km.ID] = true
			row := Monitor{Name: km.Name, Group: g.Name, Type: km.Type, Status: "unknown"}
			if b := latest(beats.List[string(km.ID)]); b != nil {
				row.Status = statusName(b.Status)
				row.Message = b.Msg
				row.LastCheck = isoTime(b.Time)
			}
			if u, ok := beats.Uptime[string(km.ID)+"_24"]; ok {
				p := math.Round(u*10000) / 100
				row.Uptime24h = &p
			}
			row.Entry = m.Match(km.Hostname, km.URL, km.Name, strings.TrimSpace(bracket.ReplaceAllString(km.Name, " ")))
			out = append(out, row)
		}
	}
	return out
}

func notUp(list []Monitor) []Monitor {
	out := []Monitor{}
	for _, m := range list {
		if m.Status != "up" {
			out = append(out, m)
		}
	}
	return out
}

// Summary is the tool answer: counts per status and the monitors, down first.
type Summary struct {
	Counts   map[string]int `json:"counts"`
	Monitors []Monitor      `json:"monitors"`
}

var order = map[string]int{"down": 0, "pending": 1, "unknown": 2, "maintenance": 3, "up": 4}

func summarize(list []Monitor) Summary {
	s := Summary{Counts: map[string]int{}, Monitors: list}
	if s.Monitors == nil {
		s.Monitors = []Monitor{}
	}
	for _, m := range list {
		s.Counts[m.Status]++
	}
	sort.SliceStable(s.Monitors, func(i, j int) bool { return order[s.Monitors[i].Status] < order[s.Monitors[j].Status] })
	return s
}
