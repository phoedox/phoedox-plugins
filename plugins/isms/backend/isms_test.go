package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

var today = time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)

func ctl(id, slug, framework, ref, status string, attrs map[string]any) pdk.Entity {
	a := map[string]any{"framework": framework, "ref": ref, "status": status}
	for k, v := range attrs {
		a[k] = v
	}
	return pdk.Entity{ID: id, Kind: "control", Slug: slug, Name: ref + " " + slug, Attrs: a}
}

var (
	web  = pdk.Entity{ID: "e1", Kind: "server", Slug: "web-01", Name: "web-01"}
	db   = pdk.Entity{ID: "e2", Kind: "vm", Slug: "db-01", Name: "db-01"}
	wiki = pdk.Entity{ID: "e3", Kind: "service", Slug: "wiki", Name: "Wiki"}
	nas  = pdk.Entity{ID: "e4", Kind: "server", Slug: "nas", Name: "NAS"}
	sw   = pdk.Entity{ID: "e5", Kind: "switch", Slug: "core-sw", Name: "core-sw"}
)

func controls() []Control {
	all := map[string]pdk.Entity{}
	for _, e := range []pdk.Entity{web, db, wiki, nas, sw} {
		all[e.ID] = e
	}
	details := []pdk.EntityDetail{
		{Entity: ctl("c1", "iso-a-5-15", "iso27001", "A.5.15", "implemented", map[string]any{"title": "Access rules | roles", "owner": "IT lead",
			"evidence": "https://wiki.example.org/access", "last_review": "2026-03-01", "justification": "Required by customer contracts"}),
			Edges: []pdk.Edge{{From: "c1", Rel: "covers", To: "e1"}, {From: "c1", Rel: "covers", To: "e5"}, {From: "c1", Rel: "covers", To: "e1"}, {From: "x", Rel: "covers", To: "c1"}}},
		{Entity: ctl("c2", "iso-a-5-2", "iso27001", "A.5.2", "partial", map[string]any{"last_review": "2025-01-01"}),
			Edges: []pdk.Edge{{From: "c2", Rel: "covers", To: "e2"}, {From: "c2", Rel: "related_to", To: "e3"}}},
		{Entity: ctl("c3", "iso-a-7-4", "iso27001", "A.7.4", "not_applicable", map[string]any{"justification": "No own premises", "last_review": "2026-06-01"}),
			Edges: []pdk.Edge{{From: "c3", Rel: "covers", To: "e4"}}},
		{Entity: ctl("c4", "ops-1-1-2-a3", "bsi", "OPS.1.1.2.A3", "planned", nil)},
		{Entity: ctl("c5", "ops-1-1-2-a10", "bsi", "OPS.1.1.2.A10", "", map[string]any{"evidence": "Ticket #42"})},
		{Entity: pdk.Entity{ID: "c6", Kind: "control", Slug: "backup-check", Name: "Monthly restore test", Attrs: map[string]any{"framework": "own", "status": "implemented", "evidence": "Restore log", "last_review": "2026-10-01"}},
			Edges: []pdk.Edge{{From: "c6", Rel: "covers", To: "e3"}, {From: "c6", Rel: "covers", To: "gone"}}},
	}
	return collect(details, func(id string) (pdk.Entity, bool) { e, ok := all[id]; return e, ok })
}

func TestCollect(t *testing.T) {
	cs := controls()
	var order []string
	for _, c := range cs {
		order = append(order, c.Entity.Slug)
	}
	// Natural order of references; the control without a reference is last.
	if !slices.Equal(order, []string{"iso-a-5-2", "iso-a-5-15", "iso-a-7-4", "ops-1-1-2-a3", "ops-1-1-2-a10", "backup-check"}) {
		t.Fatal(order)
	}
	if c := cs[1]; len(c.Covers) != 2 || c.Covers[0].Slug != "core-sw" || c.Covers[1].Slug != "web-01" {
		t.Fatalf("covers: %+v", c.Covers)
	}
	if len(cs[0].Covers) != 1 || len(cs[5].Covers) != 1 {
		t.Fatal("only covers edges, resolvable targets")
	}
	if compareRef("A.5.10", "A.5.9") <= 0 || compareRef("A.5.9", "A.5.10") >= 0 || compareRef("a.5.1", "A.5.1") != 0 || compareRef("OPS.1.1.2.A3", "OPS.1.1") <= 0 {
		t.Fatal("compareRef")
	}
}

func TestCoverage(t *testing.T) {
	cv := coverage(controls(), []pdk.Entity{web, db, wiki, nas, sw}, today)
	if cv.Controls != 6 || cv.ByFramework["iso27001"]["implemented"] != 1 || cv.ByFramework["iso27001"]["partial"] != 1 || cv.ByFramework["iso27001"]["not_applicable"] != 1 ||
		cv.ByFramework["bsi"]["planned"] != 1 || cv.ByFramework["bsi"]["not_set"] != 1 || cv.ByFramework["own"]["implemented"] != 1 {
		t.Fatalf("counts: %+v", cv.ByFramework)
	}
	slugs := func(l []ControlRef) []string {
		var out []string
		for _, c := range l {
			out = append(out, c.Slug)
		}
		return out
	}
	if got := slugs(cv.WithoutEvidence); !slices.Equal(got, []string{"iso-a-5-2", "ops-1-1-2-a3"}) {
		t.Fatalf("without evidence (not applicable excluded): %v", got)
	}
	if got := slugs(cv.NotReviewed); !slices.Equal(got, []string{"iso-a-5-2", "ops-1-1-2-a3", "ops-1-1-2-a10"}) {
		t.Fatalf("not reviewed: %v", got)
	}
	// The NAS is only covered by a control that is not applicable; the
	// switch is not an asset kind.
	if cv.EntriesChecked != 4 || len(cv.Uncovered) != 1 || cv.Uncovered[0].Slug != "nas" {
		t.Fatalf("uncovered: %+v (%d)", cv.Uncovered, cv.EntriesChecked)
	}
	if e := coverage(nil, nil, today); e.WithoutEvidence == nil || e.NotReviewed == nil || e.Uncovered == nil {
		t.Fatal("empty lists must be [] in JSON")
	}
}

func TestSoA(t *testing.T) {
	md := soa(controls(), "", today)
	for _, want := range []string{
		"# Statement of Applicability\n\nGenerated from the knowledge graph on 2026-11-14 · 6 controls\n",
		"\n## ISO/IEC 27001\n\n3 controls · 2 applicable · 1 implemented\n\n| Reference | Title | Applicable | Status | Justification | Owner | Evidence | Last review | Covers |\n|---|---|---|---|---|---|---|---|---|\n" +
			"| A.5.2 | A.5.2 iso-a-5-2 (@iso-a-5-2) | Yes | Partially implemented | _not recorded_ | _not recorded_ | _not recorded_ | 2025-01-01 | @db-01 |\n" +
			"| A.5.15 | Access rules \\| roles (@iso-a-5-15) | Yes | Implemented | Required by customer contracts | IT lead | https://wiki.example.org/access | 2026-03-01 | @core-sw, @web-01 |\n" +
			"| A.7.4 | A.7.4 iso-a-7-4 (@iso-a-7-4) | No | Not applicable | No own premises |",
		"\n## BSI IT-Grundschutz\n\n2 controls · 2 applicable · 0 implemented\n",
		"| OPS.1.1.2.A10 | OPS.1.1.2.A10 ops-1-1-2-a10 (@ops-1-1-2-a10) | _not recorded_ | _not recorded_ |",
		"\n## Own controls\n\n1 control · 1 applicable · 1 implemented\n",
		"| _not recorded_ | Monthly restore test (@backup-check) | Yes | Implemented |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}
	if strings.Index(md, "## ISO") > strings.Index(md, "## BSI") || strings.Contains(md, "Without framework") {
		t.Fatal("order of sections")
	}
	bsi := soa(controls(), "bsi", today)
	if !strings.Contains(bsi, "· 2 controls\n") || strings.Contains(bsi, "## ISO") || !strings.Contains(bsi, "## BSI") {
		t.Fatal(bsi)
	}
	if e := soa(nil, "", today); !strings.Contains(e, "· 0 controls\n") || !strings.Contains(e, "No controls are recorded yet") {
		t.Fatal(e)
	}
}
