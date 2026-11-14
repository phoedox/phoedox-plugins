package main

import (
	"net/url"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// A search answer of Paperless-ngx 2.x (with fields=…, so no content).
const searchJSON = `{
  "count": 3,
  "next": null,
  "previous": null,
  "all": [412, 77, 90],
  "results": [
    {"id": 412, "correspondent": 5, "document_type": 2, "title": "Invoice 2024-1187 Synology DS923+", "tags": [1, 7], "created": "2024-03-18", "archive_serial_number": 1187,
     "__search_hit__": {"score": 9.2, "highlights": "Serial <span>2290PDN123456</span>", "rank": 0}},
    {"id": 77, "correspondent": null, "document_type": 3, "title": "DS923+ hardware installation guide", "tags": [], "created": "2023-01-02T00:00:00+01:00", "archive_serial_number": null},
    {"id": 90, "correspondent": 5, "document_type": null, "title": "Delivery note", "tags": [7, 99], "created": "2024-03-15"}
  ]
}`

func TestSearch(t *testing.T) {
	docs, total, err := parseDocs([]byte(searchJSON))
	if err != nil || total != 3 || len(docs) != 3 {
		t.Fatal(docs, total, err)
	}
	c, ty, tags := referenced(docs)
	if len(c) != 1 || c[0] != 5 || len(ty) != 2 || len(tags) != 3 || tags[2] != 99 {
		t.Fatal(c, ty, tags)
	}
	corr, err := parseNames([]byte(`{"count":1,"next":null,"results":[{"id":5,"slug":"acme-it","name":"ACME IT GmbH","match":"","matching_algorithm":6,"document_count":40}]}`))
	if err != nil || corr[5] != "ACME IT GmbH" {
		t.Fatal(corr, err)
	}
	names := Names{Correspondents: corr, Types: map[int]string{2: "Invoice", 3: "Manual"}, Tags: map[int]string{1: "Hardware", 7: "IT"}}
	list := results("https://paperless.example.org", docs, names)
	inv := list[0]
	if inv.Title != "Invoice 2024-1187 Synology DS923+" || inv.Correspondent != "ACME IT GmbH" || inv.DocumentType != "Invoice" || inv.Created != "2024-03-18" ||
		strings.Join(inv.Tags, ",") != "Hardware,IT" || *inv.ASN != 1187 || inv.URL != "https://paperless.example.org/documents/412/details" {
		t.Fatalf("invoice: %+v", inv)
	}
	if m := list[1]; m.Created != "2023-01-02" || m.Correspondent != "" || m.DocumentType != "Manual" || m.ASN != nil {
		t.Fatalf("manual (datetime, no correspondent): %+v", m)
	}
	if d := list[2]; len(d.Tags) != 1 || d.DocumentType != "" {
		t.Fatalf("unknown tag left out: %+v", d)
	}
	// Without names (lookup failed) the documents are still listed.
	if l := results("https://p", docs, Names{}); l[0].Correspondent != "" || len(l[0].Tags) != 0 {
		t.Fatalf("%+v", l[0])
	}
	if _, _, err := parseDocs([]byte("<html>")); err == nil {
		t.Fatal("html accepted")
	}
}

func TestPaths(t *testing.T) {
	u, err := url.Parse(searchPath(`"2290PDN123456" & more`, 10))
	if err != nil || u.Path != "/api/documents/" {
		t.Fatal(u, err)
	}
	q := u.Query()
	if q.Get("query") != `"2290PDN123456" & more` || q.Get("page_size") != "10" || strings.Contains(q.Get("fields"), "content") || !strings.Contains(q.Get("fields"), "title") {
		t.Fatal(q)
	}
	if p := namesPath("tags", []int{1, 7, 99}); p != "/api/tags/?id__in=1%2C7%2C99&page_size=3" {
		t.Fatal(p)
	}
	if clampLimit(0) != 10 || clampLimit(500) != 50 || clampLimit(5) != 5 {
		t.Fatal("limit")
	}
	if q, err := cleanQuery("  warranty \n ThinkPad "); err != nil || q != "warranty ThinkPad" {
		t.Fatal(q, err)
	}
	if _, err := cleanQuery("  "); err == nil {
		t.Fatal("empty query")
	}
	if _, err := cleanQuery(strings.Repeat("x", 201)); err == nil {
		t.Fatal("long query")
	}
}

func TestEntryTerms(t *testing.T) {
	e := pdk.Entity{Slug: "nas-01", Name: "NAS 01", Attrs: map[string]any{"serial": `2290PDN"123456`, "model": "DS923+", "asset_tag": "INV-0042", "inventory_number": "inv-0042", "ip": "10.0.0.5", "height_units": 2.0}}
	terms := entryTerms(e)
	if len(terms) != 4 || terms[0] != (Term{`"2290PDN 123456"`, "serial"}) || terms[1] != (Term{`"INV-0042"`, "asset_tag"}) || terms[2] != (Term{`"DS923+"`, "model"}) || terms[3].From != "name" {
		t.Fatalf("%+v", terms)
	}
	if len(entryTerms(pdk.Entity{Name: "ab"})) != 0 {
		t.Fatal("too short")
	}
	var all []Doc
	matched := map[int]string{}
	all = merge(all, matched, []Doc{{ID: 1}, {ID: 2}}, "serial", 3)
	all = merge(all, matched, []Doc{{ID: 2}, {ID: 3}, {ID: 4}}, "model", 3)
	if len(all) != 3 || matched[2] != "serial" || matched[3] != "model" || matched[4] != "" {
		t.Fatal(all, matched)
	}
}
