// Logic of the ISMS plugin, without build tags so it is testable natively.
// It ships no catalogue: references and titles are what users entered.
package main

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const (
	controlKind = "control"
	relCovers   = "covers"
	notSet      = "not_set"
	notRecorded = "not recorded"
	notApplic   = "not_applicable"
)

// assetKinds are the entries every ISMS should cover with a control.
var assetKinds = []string{"server", "vm", "service"}

// frameworks in the order of the Statement of Applicability.
var frameworks = []struct{ ID, Label string }{
	{"iso27001", "ISO/IEC 27001"},
	{"bsi", "BSI IT-Grundschutz"},
	{"own", "Own controls"},
	{notSet, "Without framework"},
}

var statusLabel = map[string]string{
	"not_started":    "Not started",
	"planned":        "Planned",
	"partial":        "Partially implemented",
	"implemented":    "Implemented",
	"not_applicable": "Not applicable",
}

// Control is a control with the entries it covers.
type Control struct {
	Entity pdk.Entity
	Covers []pdk.Entity
}

func (c Control) framework() string {
	f := text(c.Entity, "framework")
	for _, x := range frameworks {
		if x.ID == f {
			return f
		}
	}
	return notSet
}

func (c Control) status() string {
	if s := text(c.Entity, "status"); statusLabel[s] != "" {
		return s
	}
	return notSet
}

func (c Control) title() string {
	if t := text(c.Entity, "title"); t != "" {
		return t
	}
	return c.Entity.Name
}

// collect resolves the outgoing covers edges of the controls. resolve
// returns the entry for an entity ID.
func collect(details []pdk.EntityDetail, resolve func(id string) (pdk.Entity, bool)) []Control {
	out := make([]Control, 0, len(details))
	for _, d := range details {
		c := Control{Entity: d.Entity}
		seen := map[string]bool{}
		for _, e := range d.Edges {
			if e.From != d.Entity.ID || e.Rel != relCovers || seen[e.To] {
				continue
			}
			seen[e.To] = true
			if t, ok := resolve(e.To); ok {
				c.Covers = append(c.Covers, t)
			}
		}
		slices.SortFunc(c.Covers, func(x, y pdk.Entity) int {
			return cmp.Or(cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name)), cmp.Compare(x.Slug, y.Slug))
		})
		out = append(out, c)
	}
	slices.SortFunc(out, func(x, y Control) int {
		return cmp.Or(compareRef(text(x.Entity, "ref"), text(y.Entity, "ref")), cmp.Compare(x.Entity.Slug, y.Entity.Slug))
	})
	return out
}

// compareRef orders references naturally: A.5.2 before A.5.10, and
// references before controls without one.
func compareRef(a, b string) int {
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	pa, pb := chunks(strings.ToLower(a)), chunks(strings.ToLower(b))
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		var c int
		if ea == nil && eb == nil {
			c = cmp.Compare(na, nb)
		} else {
			c = cmp.Compare(pa[i], pb[i])
		}
		if c != 0 {
			return c
		}
	}
	return cmp.Compare(len(pa), len(pb))
}

// chunks splits "ops.1.1.2.a3" into runs of digits and of other characters.
func chunks(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if i > 0 && unicode.IsDigit(r) != unicode.IsDigit(rune(s[i-1])) {
			out = append(out, s[start:i])
			start = i
		}
	}
	return append(out, s[start:])
}

// text reads a text attribute.
func text(e pdk.Entity, k string) string {
	switch v := e.Attrs[k].(type) {
	case string:
		return strings.TrimSpace(v)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

// reviewDue tells whether the last review is missing or older than 12
// months.
func reviewDue(e pdk.Entity, today time.Time) bool {
	t, err := time.Parse(time.DateOnly, text(e, "last_review"))
	y, m, d := today.Date()
	return err != nil || t.Before(time.Date(y-1, m, d, 0, 0, 0, 0, time.UTC))
}

// ControlRef names a control.
type ControlRef struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Framework  string `json:"framework"`
	Ref        string `json:"ref,omitempty"`
	Status     string `json:"status"`
	LastReview string `json:"last_review,omitempty"`
}

// EntryRef names an entry.
type EntryRef struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Coverage is the result of isms.coverage.
type Coverage struct {
	Controls    int                       `json:"controls"`
	ByFramework map[string]map[string]int `json:"by_framework"`
	// Controls (except not applicable ones) without evidence.
	WithoutEvidence []ControlRef `json:"without_evidence"`
	// Controls without a review in the last 12 months.
	NotReviewed []ControlRef `json:"not_reviewed"`
	// Servers, VMs and services no applicable control covers.
	Uncovered      []EntryRef `json:"uncovered"`
	EntriesChecked int        `json:"entries_checked"`
}

func controlRef(c Control) ControlRef {
	return ControlRef{Slug: c.Entity.Slug, Name: c.Entity.Name, Framework: c.framework(), Ref: text(c.Entity, "ref"), Status: c.status(), LastReview: text(c.Entity, "last_review")}
}

// coverage counts controls and finds the gaps. Controls marked not
// applicable do not cover entries.
func coverage(controls []Control, entries []pdk.Entity, today time.Time) Coverage {
	cv := Coverage{Controls: len(controls), ByFramework: map[string]map[string]int{}, WithoutEvidence: []ControlRef{}, NotReviewed: []ControlRef{}, Uncovered: []EntryRef{}}
	covered := map[string]bool{}
	for _, c := range controls {
		f, s := c.framework(), c.status()
		if cv.ByFramework[f] == nil {
			cv.ByFramework[f] = map[string]int{}
		}
		cv.ByFramework[f][s]++
		if s != notApplic && text(c.Entity, "evidence") == "" {
			cv.WithoutEvidence = append(cv.WithoutEvidence, controlRef(c))
		}
		if reviewDue(c.Entity, today) {
			cv.NotReviewed = append(cv.NotReviewed, controlRef(c))
		}
		if s != notApplic {
			for _, e := range c.Covers {
				covered[e.ID] = true
			}
		}
	}
	for _, e := range entries {
		if !slices.Contains(assetKinds, e.Kind) {
			continue
		}
		cv.EntriesChecked++
		if !covered[e.ID] {
			cv.Uncovered = append(cv.Uncovered, EntryRef{Slug: e.Slug, Name: e.Name, Kind: e.Kind})
		}
	}
	slices.SortFunc(cv.Uncovered, func(x, y EntryRef) int {
		return cmp.Or(cmp.Compare(x.Kind, y.Kind), cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name)))
	})
	return cv
}

// cell makes a value safe for one Markdown table cell.
func cell(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "_" + notRecorded + "_"
	}
	return strings.ReplaceAll(s, "|", `\|`)
}

// soa writes the Statement of Applicability as Markdown, optionally for one
// framework.
func soa(controls []Control, only string, today time.Time) string {
	var b strings.Builder
	b.WriteString("# Statement of Applicability\n\n")
	n := 0
	for _, c := range controls {
		if only == "" || c.framework() == only {
			n++
		}
	}
	fmt.Fprintf(&b, "Generated from the knowledge graph on %s · %d %s\n", today.Format(time.DateOnly), n, plural(n))
	if n == 0 {
		b.WriteString("\nNo controls are recorded yet. Create entries of the kind control with framework, reference and your own title, and link them with covers to the entries they protect.\n")
		return b.String()
	}
	for _, f := range frameworks {
		if only != "" && f.ID != only {
			continue
		}
		var list []Control
		applicable, implemented := 0, 0
		for _, c := range controls {
			if c.framework() != f.ID {
				continue
			}
			list = append(list, c)
			if c.status() != notApplic {
				applicable++
			}
			if c.status() == "implemented" {
				implemented++
			}
		}
		if len(list) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n%d %s · %d applicable · %d implemented\n\n", f.Label, len(list), plural(len(list)), applicable, implemented)
		b.WriteString("| Reference | Title | Applicable | Status | Justification | Owner | Evidence | Last review | Covers |\n|---|---|---|---|---|---|---|---|---|\n")
		for _, c := range list {
			s := c.status()
			appl := ""
			switch s {
			case notApplic:
				appl = "No"
			case notSet:
			default:
				appl = "Yes"
			}
			covers := make([]string, len(c.Covers))
			for i, e := range c.Covers {
				covers[i] = "@" + e.Slug
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
				cell(text(c.Entity, "ref")), cell(c.title()+" (@"+c.Entity.Slug+")"), cell(appl), cell(statusLabel[s]),
				cell(text(c.Entity, "justification")), cell(text(c.Entity, "owner")), cell(text(c.Entity, "evidence")),
				cell(text(c.Entity, "last_review")), cell(strings.Join(covers, ", ")))
		}
	}
	return b.String()
}

func plural(n int) string {
	if n == 1 {
		return "control"
	}
	return "controls"
}
