package main

import (
	"testing"

	"github.com/phoedox/phoedox-plugins/internal/fakeapi"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// TestDemoData runs the searches of paperless.for_entry for the demo entry
// against the recorded answers the screenshots use.
func TestDemoData(t *testing.T) {
	const dir = "../testdata/fake"
	get := func(path string) []byte {
		t.Helper()
		body, err := fakeapi.Body(dir, "GET", path)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	nas := pdk.Entity{Kind: "device", Slug: "nas-01", Name: "nas-01", Attrs: map[string]any{"model": "HomeStore 4", "serial": "HS4-2291-0457"}}
	var all []Doc
	matched := map[int]string{}
	for _, term := range entryTerms(nas) {
		docs, _, err := parseDocs(get(searchPath(term.Query, defaultLimit)))
		if err != nil {
			t.Fatal(err)
		}
		all = merge(all, matched, docs, term.From, defaultLimit)
	}
	if len(all) != 4 {
		t.Fatalf("%d documents", len(all))
	}
	c, ty, tags := referenced(all)
	var n Names
	var err error
	if n.Correspondents, err = parseNames(get(namesPath("correspondents", c))); err != nil {
		t.Fatal(err)
	}
	if n.Types, err = parseNames(get(namesPath("document_types", ty))); err != nil {
		t.Fatal(err)
	}
	if n.Tags, err = parseNames(get(namesPath("tags", tags))); err != nil {
		t.Fatal(err)
	}
	list := results("https://paperless.home.arpa", all, n)
	want := []struct{ title, typ, from string }{
		{"Invoice HomeStore 4 NAS", "Invoice", "serial"},
		{"Warranty card HomeStore 4", "Warranty", "serial"},
		{"HomeStore 4 user manual", "Manual", "model"},
		{"Replacement hard drive for nas-01", "Invoice", "name"},
	}
	for i, w := range want {
		r := list[i]
		if r.Title != w.title || r.DocumentType != w.typ || matched[r.ID] != w.from || r.Correspondent == "" || len(r.Tags) == 0 {
			t.Errorf("%d: %+v (found by %s), want %+v", i, r, matched[r.ID], w)
		}
	}
	if list[0].URL != "https://paperless.home.arpa/documents/412/details" {
		t.Errorf("url %s", list[0].URL)
	}
}
