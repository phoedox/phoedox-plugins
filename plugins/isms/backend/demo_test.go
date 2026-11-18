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

// TestDemoData checks the controls that the screenshots show (shots.json):
// the coverage and the Statement of Applicability.
func TestDemoData(t *testing.T) {
	ents, edges := demoGraph(t)
	ds, resolve := details(ents, edges, "control")
	controls := collect(ds, resolve)
	if len(controls) != 7 {
		t.Fatalf("%d controls", len(controls))
	}
	cv := coverage(controls, ents, demoDay)
	slugs := func(list []ControlRef) []string {
		var out []string
		for _, c := range list {
			out = append(out, c.Slug)
		}
		return out
	}
	if got := slugs(cv.WithoutEvidence); !slices.Contains(got, "mfa") || slices.Contains(got, "dev") || slices.Contains(got, "backup") {
		t.Errorf("without evidence: %v", got)
	}
	if got := slugs(cv.NotReviewed); !slices.Contains(got, "updates") || slices.Contains(got, "backup") {
		t.Errorf("not reviewed: %v", got)
	}
	if cv.EntriesChecked != 5 || len(cv.Uncovered) != 0 {
		t.Errorf("checked %d, uncovered %+v", cv.EntriesChecked, cv.Uncovered)
	}
	if md := soa(controls, "", demoDay); !strings.Contains(md, "A.8.13") || !strings.Contains(md, "Nightly backups with an offsite copy") {
		t.Errorf("soa:\n%s", md)
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
