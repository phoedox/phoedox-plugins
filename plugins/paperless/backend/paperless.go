// Logic of the Paperless-ngx plugin, without build tags so it is testable
// natively. Documents stay in Paperless-ngx: the tools return references
// (title, correspondent, type, date, tags, link), never the content.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const (
	defaultLimit = 10
	maxLimit     = 50
	maxQuery     = 200
	// docFields keeps the answer small: no OCR content.
	docFields = "id,title,correspondent,document_type,tags,created,archive_serial_number"
)

// clampLimit applies the default and the maximum.
func clampLimit(n int) int {
	if n <= 0 {
		return defaultLimit
	}
	return min(n, maxLimit)
}

// cleanQuery trims a search and rejects empty or overlong ones.
func cleanQuery(q string) (string, error) {
	q = strings.Join(strings.Fields(q), " ")
	if q == "" {
		return "", fmt.Errorf("query is empty")
	}
	if len(q) > maxQuery {
		return "", fmt.Errorf("query is longer than %d characters", maxQuery)
	}
	return q, nil
}

// searchPath is the first page of a full-text search.
func searchPath(query string, limit int) string {
	return "/api/documents/?" + url.Values{
		"query":            {query},
		"page_size":        {strconv.Itoa(limit)},
		"fields":           {docFields},
		"truncate_content": {"true"},
	}.Encode()
}

// namesPath looks up the names of correspondents, document types or tags.
func namesPath(endpoint string, ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return "/api/" + endpoint + "/?" + url.Values{"id__in": {strings.Join(parts, ",")}, "page_size": {strconv.Itoa(max(len(ids), 1))}}.Encode()
}

// Doc is a document of a Paperless-ngx search answer.
type Doc struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	Correspondent *int   `json:"correspondent"`
	DocumentType  *int   `json:"document_type"`
	Tags          []int  `json:"tags"`
	Created       string `json:"created"`
	ASN           *int   `json:"archive_serial_number"`
}

// parseDocs reads a page of documents and the total number of hits.
func parseDocs(body []byte) ([]Doc, int, error) {
	var r struct {
		Count   int   `json:"count"`
		Results []Doc `json:"results"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, 0, fmt.Errorf("unexpected answer from Paperless-ngx: %w", err)
	}
	return r.Results, r.Count, nil
}

// parseNames reads a list of correspondents, document types or tags.
func parseNames(body []byte) (map[int]string, error) {
	var r struct {
		Results []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := map[int]string{}
	for _, x := range r.Results {
		out[x.ID] = x.Name
	}
	return out, nil
}

// Names resolves the IDs in documents.
type Names struct {
	Correspondents, Types, Tags map[int]string
}

// referenced lists the IDs the documents use, per lookup endpoint.
func referenced(docs []Doc) (correspondents, types, tags []int) {
	add := func(list []int, id *int) []int {
		if id != nil && !slices.Contains(list, *id) {
			list = append(list, *id)
		}
		return list
	}
	for _, d := range docs {
		correspondents = add(correspondents, d.Correspondent)
		types = add(types, d.DocumentType)
		for _, t := range d.Tags {
			tags = add(tags, &t)
		}
	}
	slices.Sort(correspondents)
	slices.Sort(types)
	slices.Sort(tags)
	return
}

// Result is a document for AI clients.
type Result struct {
	ID            int      `json:"id"`
	Title         string   `json:"title"`
	Correspondent string   `json:"correspondent,omitempty"`
	DocumentType  string   `json:"document_type,omitempty"`
	Created       string   `json:"created,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	ASN           *int     `json:"archive_serial_number,omitempty"`
	URL           string   `json:"url"`
	MatchedBy     string   `json:"matched_by,omitempty"`
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// results turns documents into results with names and links.
func results(base string, docs []Doc, n Names) []Result {
	out := make([]Result, 0, len(docs))
	name := func(m map[int]string, id *int) string {
		if id == nil {
			return ""
		}
		return m[*id]
	}
	for _, d := range docs {
		r := Result{ID: d.ID, Title: d.Title, ASN: d.ASN, URL: fmt.Sprintf("%s/documents/%d/details", base, d.ID),
			Correspondent: name(n.Correspondents, d.Correspondent), DocumentType: name(n.Types, d.DocumentType),
			Created: dateRe.FindString(d.Created)}
		for _, t := range d.Tags {
			if s := n.Tags[t]; s != "" {
				r.Tags = append(r.Tags, s)
			}
		}
		out = append(out, r)
	}
	return out
}

// Term is one search for an entry: the text and what it came from.
type Term struct {
	Query string `json:"query"`
	From  string `json:"from"`
}

// entryTerms builds phrase searches from an entry: identifiers first (they
// are the most specific), then the model, then the name.
func entryTerms(e pdk.Entity) []Term {
	var out []Term
	seen := map[string]bool{}
	add := func(from string, v any) {
		s, ok := v.(string)
		if !ok {
			return
		}
		s = strings.Join(strings.Fields(strings.NewReplacer(`"`, " ", `\`, " ").Replace(s)), " ")
		if len(s) < 3 || len(s) > 100 || seen[strings.ToLower(s)] {
			return
		}
		seen[strings.ToLower(s)] = true
		out = append(out, Term{Query: `"` + s + `"`, From: from})
	}
	for _, a := range []string{"serial", "serial_number", "asset_tag", "inventory_number", "model"} {
		add(a, e.Attrs[a])
	}
	add("name", e.Name)
	return out
}

// merge appends documents not seen before, up to limit, and tells what
// matched them.
func merge(all []Doc, matched map[int]string, docs []Doc, from string, limit int) []Doc {
	for _, d := range docs {
		if len(all) >= limit {
			break
		}
		if _, ok := matched[d.ID]; ok {
			continue
		}
		matched[d.ID] = from
		all = append(all, d)
	}
	return all
}
