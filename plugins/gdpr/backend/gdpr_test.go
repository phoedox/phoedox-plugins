package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

var (
	today = time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)

	support = pdk.Entity{ID: "a1", Kind: "processing_activity", Slug: "customer-support", Name: "Customer support", Attrs: map[string]any{
		"purpose": "Answering support requests | tickets", "legal_basis": "contract", "data_categories": "Name, e-mail address,\nticket content",
		"data_subjects": "Customers", "recipients": "Support team", "third_country_transfer": true, "safeguards": "Standard contractual clauses",
		"retention": "3 years after the last ticket", "toms": "See TOM document", "controller_contact": "ACME GmbH, privacy@acme.example",
		"dpo_contact": "dpo@acme.example", "last_review": "2026-02-01"}}
	payroll = pdk.Entity{ID: "a2", Kind: "processing_activity", Slug: "payroll", Name: "payroll", Attrs: map[string]any{
		"legal_basis": "legal_obligation", "third_country_transfer": "true", "last_review": "2025-10-01"}}
	newsletter = pdk.Entity{ID: "a3", Kind: "processing_activity", Slug: "newsletter", Name: "Newsletter", Attrs: map[string]any{
		"purpose": "Marketing", "legal_basis": "maybe", "retention": "Until unsubscribed"}}

	zammad  = pdk.Entity{ID: "s1", Kind: "service", Slug: "zammad", Name: "Zammad"}
	mail    = pdk.Entity{ID: "s2", Kind: "server", Slug: "mail-01", Name: "mail-01"}
	hetzner = pdk.Entity{ID: "p1", Kind: "data_processor", Slug: "hetzner", Name: "Hetzner Online GmbH", Attrs: map[string]any{
		"country": "Germany", "contract_signed": "2024-01-10", "avv_url": "https://docs.example.org/avv-hetzner.pdf", "contact": "dpa@hetzner.example"}}
	datev = pdk.Entity{ID: "p2", Kind: "data_processor", Slug: "datev", Name: "DATEV eG", Attrs: map[string]any{"country": "Germany"}}
)

func activities() []Activity {
	all := map[string]pdk.Entity{}
	for _, e := range []pdk.Entity{zammad, mail, hetzner, datev} {
		all[e.ID] = e
	}
	details := []pdk.EntityDetail{
		{Entity: support, Edges: []pdk.Edge{
			{From: "a1", Rel: "processed_by", To: "s2"},
			{From: "a1", Rel: "processed_by", To: "s1"},
			{From: "a1", Rel: "processed_by", To: "s1"},         // duplicate
			{From: "a1", Rel: "processor", To: "p1"},            // processor
			{From: "a1", Rel: "depends_on", To: "s2"},           // other relation
			{From: "x9", Rel: "processed_by", To: "a1"},         // incoming
			{From: "a1", Rel: "processed_by", To: "deleted-id"}, // unresolvable
		}},
		{Entity: payroll, Edges: []pdk.Edge{{From: "a2", Rel: "processor", To: "p2"}}},
		{Entity: newsletter},
	}
	return collect(details, func(id string) (pdk.Entity, bool) { e, ok := all[id]; return e, ok })
}

func TestCollect(t *testing.T) {
	acts := activities()
	if len(acts) != 3 || acts[0].Entity.Slug != "customer-support" || acts[1].Entity.Slug != "newsletter" || acts[2].Entity.Slug != "payroll" {
		t.Fatalf("order by name: %+v", acts)
	}
	s := acts[0]
	if len(s.Systems) != 2 || s.Systems[0].Slug != "mail-01" || s.Systems[1].Slug != "zammad" || len(s.Processors) != 1 || s.Processors[0].Slug != "hetzner" {
		t.Fatalf("support: %+v", s)
	}
}

func TestRegister(t *testing.T) {
	md := register(activities(), []pdk.Entity{datev, hetzner}, today)
	for _, want := range []string{
		"# Record of processing activities\n\nArt. 30 GDPR · generated from the knowledge graph on 2026-11-14 · 3 processing activities\n",
		"## Customer support (@customer-support)\n\n| Field | Value |\n|---|---|\n| Purpose of the processing | Answering support requests \\| tickets |\n",
		"| Legal basis | Contract (Art. 6(1)(b) GDPR) |\n",
		"| Categories of personal data | Name, e-mail address, ticket content |\n",
		"| Transfer to a third country | Yes |\n| Safeguards for the transfer | Standard contractual clauses |\n",
		"| Systems | mail-01 (server, @mail-01), Zammad (service, @zammad) |\n| Processors | Hetzner Online GmbH (@hetzner) |\n",
		"## Newsletter (@newsletter)",
		"| Legal basis | maybe |\n",
		"| Transfer to a third country | _not recorded_ |\n",
		"## payroll (@payroll)",
		"| Purpose of the processing | _not recorded_ |\n",
		"| Safeguards for the transfer | _not recorded_ |\n",
		"## Processors\n\n| Processor | Country | Agreement signed | Agreement | Contact | Processing activities |\n|---|---|---|---|---|---|\n" +
			"| DATEV eG (@datev) | Germany | _not recorded_ | _not recorded_ | _not recorded_ | @payroll |\n" +
			"| Hetzner Online GmbH (@hetzner) | Germany | 2024-01-10 | https://docs.example.org/avv-hetzner.pdf | dpa@hetzner.example | @customer-support |\n",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}
	// The newsletter records no transfer, so there is no safeguards row.
	nl := md[strings.Index(md, "## Newsletter"):strings.Index(md, "## payroll")]
	if strings.Contains(nl, "Safeguards") {
		t.Errorf("safeguards without transfer:\n%s", nl)
	}
	empty := register(nil, nil, today)
	if !strings.Contains(empty, "0 processing activities") || !strings.Contains(empty, "No processing activities are recorded yet") || strings.Contains(empty, "## Processors") {
		t.Fatal(empty)
	}
	if one := register(activities()[:1], nil, today); !strings.Contains(one, "· 1 processing activity\n") {
		t.Fatal(one)
	}
}

func TestGaps(t *testing.T) {
	g := gaps(activities(), []pdk.Entity{hetzner, datev}, today)
	if g.Checked != 3 || len(g.Activities) != 2 {
		t.Fatalf("%+v", g)
	}
	nl, pay := g.Activities[0], g.Activities[1]
	if nl.Slug != "newsletter" || !slices.Equal(nl.Missing, []string{"legal_basis", "last_review"}) {
		t.Fatalf("newsletter (unknown legal basis, never reviewed): %+v", nl)
	}
	if pay.Slug != "payroll" || !slices.Equal(pay.Missing, []string{"purpose", "retention", "review_overdue", "safeguards"}) || pay.LastReview != "2025-10-01" {
		t.Fatalf("payroll: %+v", pay)
	}
	if len(g.ProcessorsWithoutAgreement) != 1 || g.ProcessorsWithoutAgreement[0].Slug != "datev" {
		t.Fatalf("processors: %+v", g.ProcessorsWithoutAgreement)
	}
	// Reviewed exactly 12 months ago still counts as reviewed.
	e := pdk.Entity{Attrs: map[string]any{"last_review": "2025-11-14"}}
	if reviewDue(e, today) || !reviewDue(pdk.Entity{Attrs: map[string]any{"last_review": "2025-11-13"}}, today) {
		t.Fatal("12 months")
	}
	if empty := gaps(nil, nil, today); empty.Activities == nil || empty.ProcessorsWithoutAgreement == nil {
		t.Fatal("empty lists must be [] in JSON")
	}
}
