// Logic of the GDPR plugin, without build tags so it is testable natively.
// It only structures what users entered: field labels are factual, there is
// no legal text and no legal assessment.
package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const (
	activityKind  = "processing_activity"
	processorKind = "data_processor"
	relSystem     = "processed_by"
	relProcessor  = "processor"
	notRecorded   = "not recorded"
)

// Activity is a processing activity with the entries it links to.
type Activity struct {
	Entity     pdk.Entity
	Systems    []pdk.Entity
	Processors []pdk.Entity
}

// collect resolves the outgoing processed_by and processor edges of the
// activities. resolve returns the entry for an entity ID.
func collect(details []pdk.EntityDetail, resolve func(id string) (pdk.Entity, bool)) []Activity {
	out := make([]Activity, 0, len(details))
	for _, d := range details {
		a := Activity{Entity: d.Entity}
		seen := map[string]bool{}
		for _, e := range d.Edges {
			if e.From != d.Entity.ID || seen[e.Rel+"\x00"+e.To] || (e.Rel != relSystem && e.Rel != relProcessor) {
				continue
			}
			seen[e.Rel+"\x00"+e.To] = true
			t, ok := resolve(e.To)
			if !ok {
				continue
			}
			if e.Rel == relProcessor {
				a.Processors = append(a.Processors, t)
			} else {
				a.Systems = append(a.Systems, t)
			}
		}
		slices.SortFunc(a.Systems, func(x, y pdk.Entity) int {
			return cmp.Or(cmp.Compare(x.Kind, y.Kind), byName(x, y))
		})
		slices.SortFunc(a.Processors, byName)
		out = append(out, a)
	}
	slices.SortFunc(out, func(x, y Activity) int { return byName(x.Entity, y.Entity) })
	return out
}

func byName(x, y pdk.Entity) int {
	return cmp.Or(cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name)), cmp.Compare(x.Slug, y.Slug))
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

// flag reads a yes/no attribute: true, false or unknown.
func flag(e pdk.Entity, k string) (value, known bool) {
	switch v := e.Attrs[k].(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "1":
			return true, true
		case "false", "no", "0":
			return false, true
		}
	}
	return false, false
}

// date reads a date attribute (YYYY-MM-DD).
func date(e pdk.Entity, k string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, text(e, k))
	return t, err == nil
}

// reviewDue tells whether the last review is missing or older than 12
// months.
func reviewDue(e pdk.Entity, today time.Time) bool {
	t, ok := date(e, "last_review")
	y, m, d := today.Date()
	return !ok || t.Before(time.Date(y-1, m, d, 0, 0, 0, 0, time.UTC))
}

// legalBasis names the options of Art. 6(1) GDPR.
var legalBasis = map[string]string{
	"consent":              "Consent (Art. 6(1)(a) GDPR)",
	"contract":             "Contract (Art. 6(1)(b) GDPR)",
	"legal_obligation":     "Legal obligation (Art. 6(1)(c) GDPR)",
	"vital_interests":      "Vital interests (Art. 6(1)(d) GDPR)",
	"public_task":          "Public task (Art. 6(1)(e) GDPR)",
	"legitimate_interests": "Legitimate interests (Art. 6(1)(f) GDPR)",
}

// cell makes a value safe for one Markdown table cell.
func cell(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "_" + notRecorded + "_"
	}
	return strings.ReplaceAll(s, "|", `\|`)
}

// ref names an entry with a mention, e.g. "Zammad (@zammad)".
func ref(e pdk.Entity) string {
	return strings.Join(strings.Fields(e.Name), " ") + " (@" + e.Slug + ")"
}

func refs(list []pdk.Entity, kind bool) string {
	parts := make([]string, len(list))
	for i, e := range list {
		parts[i] = ref(e)
		if kind {
			parts[i] = strings.Join(strings.Fields(e.Name), " ") + " (" + e.Kind + ", @" + e.Slug + ")"
		}
	}
	return strings.Join(parts, ", ")
}

// register writes the record of processing activities as Markdown.
func register(acts []Activity, processors []pdk.Entity, today time.Time) string {
	var b strings.Builder
	b.WriteString("# Record of processing activities\n\n")
	fmt.Fprintf(&b, "Art. 30 GDPR · generated from the knowledge graph on %s · %d processing %s\n\n", today.Format(time.DateOnly), len(acts), plural(len(acts), "activity", "activities"))
	if len(acts) == 0 {
		b.WriteString("No processing activities are recorded yet. Create entries of the kind processing_activity and link them to their systems (processed_by) and processors (processor).\n")
	}
	for _, a := range acts {
		e := a.Entity
		fmt.Fprintf(&b, "## %s\n\n", ref(e))
		b.WriteString("| Field | Value |\n|---|---|\n")
		row := func(label, value string) { fmt.Fprintf(&b, "| %s | %s |\n", label, cell(value)) }
		row("Purpose of the processing", text(e, "purpose"))
		basis := text(e, "legal_basis")
		if l, ok := legalBasis[basis]; ok {
			basis = l
		}
		row("Legal basis", basis)
		row("Categories of personal data", text(e, "data_categories"))
		row("Categories of data subjects", text(e, "data_subjects"))
		row("Categories of recipients", text(e, "recipients"))
		transfer := ""
		if v, ok := flag(e, "third_country_transfer"); ok {
			transfer = "No"
			if v {
				transfer = "Yes"
			}
		}
		row("Transfer to a third country", transfer)
		if v, _ := flag(e, "third_country_transfer"); v || text(e, "safeguards") != "" {
			row("Safeguards for the transfer", text(e, "safeguards"))
		}
		row("Time limits for erasure", text(e, "retention"))
		row("Technical and organisational measures", text(e, "toms"))
		row("Controller", text(e, "controller_contact"))
		row("Data protection officer", text(e, "dpo_contact"))
		row("Last review", text(e, "last_review"))
		row("Systems", refs(a.Systems, true))
		row("Processors", refs(a.Processors, false))
		b.WriteString("\n")
	}
	if len(processors) > 0 {
		used := map[string][]string{}
		for _, a := range acts {
			for _, p := range a.Processors {
				used[p.ID] = append(used[p.ID], "@"+a.Entity.Slug)
			}
		}
		list := slices.Clone(processors)
		slices.SortFunc(list, byName)
		b.WriteString("## Processors\n\n| Processor | Country | Agreement signed | Agreement | Contact | Processing activities |\n|---|---|---|---|---|---|\n")
		for _, p := range list {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", cell(ref(p)), cell(text(p, "country")), cell(text(p, "contract_signed")),
				cell(text(p, "avv_url")), cell(text(p, "contact")), cell(strings.Join(used[p.ID], ", ")))
		}
	}
	return b.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Gap is a processing activity with missing or outdated fields.
type Gap struct {
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	Missing    []string `json:"missing"`
	LastReview string   `json:"last_review,omitempty"`
}

// Ref names an entry.
type Ref struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// Gaps is the result of gdpr.gaps.
type Gaps struct {
	Checked    int   `json:"checked"`
	Activities []Gap `json:"activities"`
	// Processors without a valid date of the data processing agreement.
	ProcessorsWithoutAgreement []Ref `json:"processors_without_agreement"`
}

// gaps finds activities without purpose, legal basis or retention, without
// a review in the last 12 months, or with a third-country transfer but no
// safeguards; and processors without an agreement date.
func gaps(acts []Activity, processors []pdk.Entity, today time.Time) Gaps {
	g := Gaps{Checked: len(acts), Activities: []Gap{}, ProcessorsWithoutAgreement: []Ref{}}
	for _, a := range acts {
		e := a.Entity
		var missing []string
		for _, k := range []string{"purpose", "legal_basis", "retention"} {
			if text(e, k) == "" {
				missing = append(missing, k)
			}
		}
		if _, ok := legalBasis[text(e, "legal_basis")]; !ok && text(e, "legal_basis") != "" {
			missing = append(missing, "legal_basis")
		}
		if _, ok := date(e, "last_review"); !ok {
			missing = append(missing, "last_review")
		} else if reviewDue(e, today) {
			missing = append(missing, "review_overdue")
		}
		if v, _ := flag(e, "third_country_transfer"); v && text(e, "safeguards") == "" {
			missing = append(missing, "safeguards")
		}
		if len(missing) > 0 {
			g.Activities = append(g.Activities, Gap{Slug: e.Slug, Name: e.Name, Missing: missing, LastReview: text(e, "last_review")})
		}
	}
	list := slices.Clone(processors)
	slices.SortFunc(list, byName)
	for _, p := range list {
		if _, ok := date(p, "contract_signed"); !ok {
			g.ProcessorsWithoutAgreement = append(g.ProcessorsWithoutAgreement, Ref{Slug: p.Slug, Name: p.Name})
		}
	}
	return g
}
