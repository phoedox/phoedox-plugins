package main

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// demoDay is the day the screenshots are taken (scripts/shots.mjs).
var demoDay = time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

// TestDemoData checks the record of processing activities that the
// screenshots show (shots.json): the register and the gaps it has on purpose.
func TestDemoData(t *testing.T) {
	ents, edges := demoGraph(t)
	ds, resolve := details(ents, edges, activityKind)
	acts := collect(ds, resolve)
	var procs []pdk.Entity
	for _, e := range ents {
		if e.Kind == processorKind {
			procs = append(procs, e)
		}
	}
	if len(acts) != 4 || len(procs) != 2 {
		t.Fatalf("%d activities, %d processors", len(acts), len(procs))
	}
	if a := acts[0]; a.Entity.Slug != "customer-invoicing" || len(a.Systems) != 3 || len(a.Processors) != 1 {
		t.Errorf("%+v", a)
	}
	g := gaps(acts, procs, demoDay)
	missing := map[string][]string{}
	for _, x := range g.Activities {
		missing[x.Slug] = x.Missing
	}
	if len(g.Activities) != 2 || !slices.Contains(missing["newsletter"], "review_overdue") || !slices.Contains(missing["contact-form"], "retention") {
		t.Errorf("gaps: %+v", g.Activities)
	}
	if len(g.ProcessorsWithoutAgreement) != 1 || g.ProcessorsWithoutAgreement[0].Slug != "example-backup-cloud" {
		t.Errorf("processors: %+v", g.ProcessorsWithoutAgreement)
	}
	if md := register(acts, procs, demoDay); !strings.Contains(md, "Customer invoicing") || !strings.Contains(md, "Example Mail Hosting") {
		t.Errorf("register:\n%s", md)
	}
}

// demoGraph reads the entries and relations that shots.json creates for the
// screenshots, with the slug as ID.
func demoGraph(t *testing.T) ([]pdk.Entity, []pdk.Edge) {
	t.Helper()
	raw, err := os.ReadFile("../shots.json")
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Setup []struct {
			Path string          `json:"path"`
			Body json.RawMessage `json:"body"`
		} `json:"setup"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	var ents []pdk.Entity
	var edges []pdk.Edge
	for _, s := range plan.Setup {
		switch {
		case strings.HasSuffix(s.Path, "/entities"):
			var e pdk.Entity
			if err := json.Unmarshal(s.Body, &e); err != nil {
				t.Fatal(err)
			}
			e.ID = e.Slug
			ents = append(ents, e)
		case strings.HasSuffix(s.Path, "/edges"):
			var e pdk.Edge
			if err := json.Unmarshal(s.Body, &e); err != nil {
				t.Fatal(err)
			}
			edges = append(edges, e)
		}
	}
	return ents, edges
}

// details returns the entries of a kind with their edges, and a resolver.
func details(ents []pdk.Entity, edges []pdk.Edge, kind string) ([]pdk.EntityDetail, func(string) (pdk.Entity, bool)) {
	var out []pdk.EntityDetail
	for _, e := range ents {
		if e.Kind != kind {
			continue
		}
		d := pdk.EntityDetail{Entity: e}
		for _, x := range edges {
			if x.From == e.ID || x.To == e.ID {
				d.Edges = append(d.Edges, x)
			}
		}
		out = append(out, d)
	}
	return out, func(id string) (pdk.Entity, bool) {
		i := slices.IndexFunc(ents, func(e pdk.Entity) bool { return e.ID == id })
		if i < 0 {
			return pdk.Entity{}, false
		}
		return ents[i], true
	}
}
