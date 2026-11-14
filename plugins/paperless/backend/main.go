//go:build wasip1

// Backend of the Paperless-ngx plugin: searches documents for AI clients.
// Read-only; documents stay in Paperless-ngx.
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

type paperless struct {
	base string
	h    map[string]string
}

func connect() (*paperless, error) {
	raw, err := integ.Setting("url", "e.g. https://paperless.example.org")
	if err != nil {
		return nil, err
	}
	base, err := integ.BaseURL(raw)
	if err != nil {
		return nil, err
	}
	token, err := integ.Setting("token", "the API auth token of a Paperless-ngx user")
	if err != nil {
		return nil, err
	}
	return &paperless{base: base, h: map[string]string{"Authorization": "Token " + strings.TrimSpace(token), "Accept": "application/json"}}, nil
}

func (p *paperless) get(path string) ([]byte, error) {
	res, err := pdk.Fetch(pdk.FetchRequest{URL: p.base + path, Headers: p.h})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, &integ.StatusError{Service: "Paperless-ngx", Status: res.Status}
	}
	return []byte(res.Body), nil
}

func (p *paperless) search(query string, limit int) ([]Doc, int, error) {
	body, err := p.get(searchPath(query, limit))
	if err != nil {
		return nil, 0, err
	}
	return parseDocs(body)
}

// names looks up the names the documents refer to. A failed lookup (e.g.
// the user may not see correspondents) leaves those names out.
func (p *paperless) names(docs []Doc) Names {
	c, t, g := referenced(docs)
	lookup := func(endpoint string, ids []int) map[int]string {
		if len(ids) == 0 {
			return nil
		}
		body, err := p.get(namesPath(endpoint, ids))
		if err == nil {
			var m map[int]string
			if m, err = parseNames(body); err == nil {
				return m
			}
		}
		pdk.Warn("Paperless-ngx " + endpoint + " not available: " + err.Error())
		return nil
	}
	return Names{Correspondents: lookup("correspondents", c), Types: lookup("document_types", t), Tags: lookup("tags", g)}
}

func runTool(tc pdk.ToolCall) (any, error) {
	var in struct {
		Query string `json:"query"`
		Entry string `json:"entry"`
		Limit int    `json:"limit"`
	}
	if len(tc.Args) > 0 {
		if err := json.Unmarshal(tc.Args, &in); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	limit := clampLimit(in.Limit)
	switch tc.Name {
	case "paperless.search":
		q, err := cleanQuery(in.Query)
		if err != nil {
			return nil, err
		}
		p, err := connect()
		if err != nil {
			return nil, err
		}
		docs, total, err := p.search(q, limit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"query": q, "total": total, "documents": results(p.base, docs, p.names(docs))}, nil
	case "paperless.for_entry":
		slug := strings.TrimSpace(in.Entry)
		if slug == "" {
			return nil, fmt.Errorf("entry is empty")
		}
		d, err := pdk.Graph.Get(slug)
		if err != nil {
			return nil, err
		}
		terms := entryTerms(d.Entity)
		if len(terms) == 0 {
			return nil, fmt.Errorf("entry %s has no name, serial number or model to search for", slug)
		}
		p, err := connect()
		if err != nil {
			return nil, err
		}
		var all []Doc
		matched := map[int]string{}
		for _, t := range terms {
			if len(all) >= limit {
				break
			}
			docs, _, err := p.search(t.Query, limit)
			if err != nil {
				return nil, err
			}
			all = merge(all, matched, docs, t.From, limit)
		}
		list := results(p.base, all, p.names(all))
		for i := range list {
			list[i].MatchedBy = matched[list[i].ID]
		}
		return map[string]any{"entry": d.Entity.Slug, "searches": terms, "documents": list}, nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
