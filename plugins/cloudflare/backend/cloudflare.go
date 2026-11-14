// Logic of the Cloudflare plugin, without build tags so it is testable
// natively.
package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const api = "https://api.cloudflare.com/client/v4"

var zoneRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9-]{2,63}$`)

// validZone checks a zone name argument.
func validZone(name string) (string, error) {
	n := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	if len(n) > 253 || !zoneRe.MatchString(n) {
		return "", fmt.Errorf("zone %q is not a domain name like example.com", name)
	}
	return n, nil
}

// envelope is the common answer format of the API.
type envelope[T any] struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result     []T `json:"result"`
	ResultInfo struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
		TotalCount int `json:"total_count"`
	} `json:"result_info"`
}

// parsePage reads a list answer. status is the HTTP status; errors name the
// Cloudflare error code and message (they never contain the token).
func parsePage[T any](status int, body []byte) (envelope[T], error) {
	var e envelope[T]
	err := json.Unmarshal(body, &e)
	if status == 200 && err == nil && e.Success {
		return e, nil
	}
	msg := fmt.Sprintf("Cloudflare answered HTTP %d", status)
	if len(e.Errors) > 0 {
		m := e.Errors[0].Message
		if len(m) > 200 {
			m = m[:200] + "…"
		}
		msg += fmt.Sprintf(" (%d: %s)", e.Errors[0].Code, m)
	} else if status == 200 {
		msg = "unexpected answer from Cloudflare"
	}
	return e, fmt.Errorf("%s", msg)
}

// Zone is a zone of GET /zones.
type Zone struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Paused      bool     `json:"paused"`
	Type        string   `json:"type"`
	NameServers []string `json:"name_servers"`
	Plan        struct {
		Name string `json:"name"`
	} `json:"plan"`
	Account struct {
		Name string `json:"name"`
	} `json:"account"`
}

// Record is a DNS record.
type Record struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
	Comment string `json:"comment,omitempty"`
}

// ZoneData is a zone with its records. Total is the record count Cloudflare
// reports (it can exceed len(Records) when not all pages were read).
type ZoneData struct {
	Zone    Zone
	Records []Record
	Total   int
}

// linkKinds are the kinds a zone is linked to.
var linkKinds = []string{"server", "vm", "lxc", "router"}

// targets resolves the addresses a record points to: A and AAAA directly,
// CNAME through the A/AAAA records of the account's zones (up to five hops).
func targets(r Record, byName map[string][]Record) []string {
	switch r.Type {
	case "A", "AAAA":
		return []string{r.Content}
	case "CNAME":
		name := strings.TrimSuffix(strings.ToLower(r.Content), ".")
		for hop := 0; hop < 5; hop++ {
			next := ""
			var out []string
			for _, t := range byName[name] {
				switch t.Type {
				case "A", "AAAA":
					out = append(out, t.Content)
				case "CNAME":
					next = strings.TrimSuffix(strings.ToLower(t.Content), ".")
				}
			}
			if len(out) > 0 || next == "" {
				return out
			}
			name = next
		}
	}
	return nil
}

// graphOps: every zone as dns_zone, related to the entries (server, vm, lxc,
// router) its records point to. match may be nil (no links).
func graphOps(zones []ZoneData, match *integ.Matcher) pdk.GraphOps {
	byName := map[string][]Record{}
	for _, z := range zones {
		for _, r := range z.Records {
			n := strings.ToLower(r.Name)
			byName[n] = append(byName[n], r)
		}
	}
	var ops pdk.GraphOps
	for _, z := range zones {
		if z.Zone.ID == "" {
			continue
		}
		proxied := 0
		for _, r := range z.Records {
			if r.Proxied {
				proxied++
			}
		}
		attrs := map[string]any{"name": z.Zone.Name, "records": max(z.Total, len(z.Records)), "proxied_records": proxied, "paused": z.Zone.Paused}
		set := func(k, v string) {
			if v != "" {
				attrs[k] = v
			}
		}
		set("status", z.Zone.Status)
		set("plan", z.Zone.Plan.Name)
		set("name_servers", strings.Join(z.Zone.NameServers, ", "))
		set("type", z.Zone.Type)
		set("account", z.Zone.Account.Name)
		ref := ops.UpsertNamed("dns_zone", "cloudflare-"+integ.Slugify(z.Zone.ID), z.Zone.Name, attrs)
		if match == nil {
			continue
		}
		linked := map[string][]string{}
		for _, r := range z.Records {
			for _, ip := range targets(r, byName) {
				e := match.Match(ip)
				if e == nil || !slices.Contains(linkKinds, e.Kind) {
					continue
				}
				label := r.Name + " " + r.Type
				if !slices.Contains(linked[e.Slug], label) {
					linked[e.Slug] = append(linked[e.Slug], label)
				}
				break
			}
		}
		slugs := make([]string, 0, len(linked))
		for s := range linked {
			slugs = append(slugs, s)
		}
		sort.Strings(slugs)
		for _, s := range slugs {
			labels := linked[s]
			sort.Strings(labels)
			if len(labels) > 20 {
				labels = append(labels[:20], fmt.Sprintf("… %d more", len(linked[s])-20))
			}
			ops.Link(ref, "related_to", s, map[string]any{"records": strings.Join(labels, ", ")})
		}
	}
	return ops
}
