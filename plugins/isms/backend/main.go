//go:build wasip1

// Backend of the ISMS plugin: coverage and Statement of Applicability from
// the controls in the knowledge graph, for AI clients. Read-only, no
// network access.
package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

// maxEntries is the most entries of one kind the host returns per query.
const maxEntries = 500

// load reads the controls with the entries they cover.
func load() ([]Control, error) {
	list, err := pdk.Graph.Query(pdk.Query{Kind: controlKind, Limit: maxEntries})
	if err != nil {
		return nil, err
	}
	if len(list) == maxEntries {
		pdk.Warn(fmt.Sprintf("only the first %d controls are included", maxEntries))
	}
	details := make([]pdk.EntityDetail, 0, len(list))
	for _, c := range list {
		d, err := pdk.Graph.Get(c.Slug)
		if err != nil {
			return nil, err
		}
		details = append(details, d)
	}
	known := map[string]pdk.Entity{}
	resolve := func(id string) (pdk.Entity, bool) {
		if e, ok := known[id]; ok {
			return e, true
		}
		d, err := pdk.Graph.Get(id)
		if err != nil {
			return pdk.Entity{}, false
		}
		known[id] = d.Entity
		return d.Entity, true
	}
	return collect(details, resolve), nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	today := time.Now().UTC()
	switch tc.Name {
	case "isms.coverage":
		controls, err := load()
		if err != nil {
			return nil, err
		}
		var entries []pdk.Entity
		for _, k := range assetKinds {
			list, err := pdk.Graph.Query(pdk.Query{Kind: k, Limit: maxEntries})
			if err != nil {
				return nil, err
			}
			entries = append(entries, list...)
		}
		return coverage(controls, entries, today), nil
	case "isms.soa":
		var in struct {
			Framework string `json:"framework"`
		}
		if len(tc.Args) > 0 {
			if err := json.Unmarshal(tc.Args, &in); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		switch in.Framework {
		case "", "iso27001", "bsi", "own":
		default:
			return nil, fmt.Errorf("framework must be iso27001, bsi or own")
		}
		controls, err := load()
		if err != nil {
			return nil, err
		}
		return map[string]any{"markdown": soa(controls, in.Framework, today)}, nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
