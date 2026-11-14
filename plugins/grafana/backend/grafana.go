// Logic of the Grafana plugin, without build tags so it is testable
// natively. Dashboards come from /api/search, firing alerts from the
// Prometheus-compatible rules API of Grafana-managed alerting.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
)

const alertsPath = "/api/prometheus/grafana/api/v1/alerts"

// searchPath lists up to 500 dashboards, optionally filtered.
func searchPath(query string) string {
	p := "/api/search?type=dash-db&limit=500"
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 200 {
		query = string([]rune(query)[:200])
	}
	if query != "" {
		p += "&query=" + url.QueryEscape(query)
	}
	return p
}

// apiError reads Grafana's {"message": …} error body.
func apiError(status int, body []byte) error {
	if status == 401 {
		return fmt.Errorf("Grafana rejected the token (HTTP 401)")
	}
	var e struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &e)
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		return &integ.StatusError{Service: "Grafana", Status: status}
	}
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return fmt.Errorf("Grafana answered HTTP %d: %s", status, msg)
}

// Dashboard is a row of grafana.dashboards.
type Dashboard struct {
	Title  string   `json:"title"`
	Folder string   `json:"folder"`
	URL    string   `json:"url"`
	Tags   []string `json:"tags"`
}

// absURL joins the base address and a path Grafana returns; the path may
// already carry the sub path of the base address.
func absURL(base, p string) string {
	if p == "" || strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if u, err := url.Parse(base); err == nil && u.Path != "" && u.Path != "/" {
		sub := strings.TrimRight(u.Path, "/")
		if p == sub || strings.HasPrefix(p, sub+"/") {
			p = strings.TrimPrefix(p, sub)
		}
	}
	return base + p
}

func parseDashboards(body []byte, base string) ([]Dashboard, error) {
	var list []struct {
		Title       string   `json:"title"`
		URL         string   `json:"url"`
		Type        string   `json:"type"`
		Tags        []string `json:"tags"`
		FolderTitle string   `json:"folderTitle"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("unexpected answer from Grafana: %w", err)
	}
	out := make([]Dashboard, 0, len(list))
	for _, d := range list {
		if d.Type != "" && d.Type != "dash-db" {
			continue
		}
		row := Dashboard{Title: d.Title, Folder: d.FolderTitle, URL: absURL(base, d.URL), Tags: d.Tags}
		if row.Folder == "" {
			row.Folder = "General"
		}
		if row.Tags == nil {
			row.Tags = []string{}
		}
		out = append(out, row)
	}
	return out, nil
}

// Alert is an alert of the rules API.
type Alert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
	ActiveAt    string            `json:"activeAt"`
	Value       string            `json:"value"`
}

func parseAlerts(body []byte) ([]Alert, error) {
	var r struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			Alerts []Alert `json:"alerts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("unexpected answer from Grafana: %w", err)
	}
	if r.Status != "" && r.Status != "success" {
		msg := r.Error
		if msg == "" {
			msg = r.Status
		}
		return nil, fmt.Errorf("Grafana could not list alerts: %s", msg)
	}
	return r.Data.Alerts, nil
}

// firing tells whether a state is firing: "Alerting", "Alerting (NoData)",
// "Alerting (Error)" or "firing".
func firing(state string) bool {
	s := strings.ToLower(strings.TrimSpace(state))
	return strings.HasPrefix(s, "alerting") || strings.HasPrefix(s, "firing")
}

// hostLabels are the labels that name the monitored host, best first.
var hostLabels = []string{"instance", "host", "hostname", "nodename", "node", "server"}

// AlertRow is a row of grafana.alerts.
type AlertRow struct {
	Name     string       `json:"name"`
	Severity string       `json:"severity,omitempty"`
	Instance string       `json:"instance,omitempty"`
	Folder   string       `json:"folder,omitempty"`
	State    string       `json:"state"`
	Since    string       `json:"since,omitempty"`
	Summary  string       `json:"summary,omitempty"`
	Entry    *integ.Entry `json:"entry,omitempty"`
}

func alertRows(list []Alert, m *integ.Matcher) []AlertRow {
	out := []AlertRow{}
	for _, a := range list {
		if !firing(a.State) {
			continue
		}
		row := AlertRow{Name: a.Labels["alertname"], Severity: a.Labels["severity"], Folder: a.Labels["grafana_folder"], State: a.State, Summary: a.Annotations["summary"]}
		if row.Summary == "" {
			row.Summary = a.Annotations["description"]
		}
		if !strings.HasPrefix(a.ActiveAt, "0001-01-01") {
			row.Since = a.ActiveAt
		}
		var cands []string
		for _, l := range hostLabels {
			if v := a.Labels[l]; v != "" {
				cands = append(cands, v)
				if row.Instance == "" {
					row.Instance = v
				}
			}
		}
		row.Entry = m.Match(cands...)
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Since > out[j].Since })
	return out
}
