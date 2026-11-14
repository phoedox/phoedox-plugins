//go:build wasip1

// Backend of the GDPR plugin: builds the record of processing activities
// and its gaps from the knowledge graph for AI clients. Read-only, no
// network access.
package main

import (
	"fmt"
	"time"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

// maxEntries is the most entries of one kind the host returns per query.
const maxEntries = 500

// load reads the processing activities with their systems and processors.
func load() ([]Activity, []pdk.Entity, error) {
	list, err := pdk.Graph.Query(pdk.Query{Kind: activityKind, Limit: maxEntries})
	if err != nil {
		return nil, nil, err
	}
	processors, err := pdk.Graph.Query(pdk.Query{Kind: processorKind, Limit: maxEntries})
	if err != nil {
		return nil, nil, err
	}
	if len(list) == maxEntries {
		pdk.Warn(fmt.Sprintf("only the first %d processing activities are included", maxEntries))
	}
	known := map[string]pdk.Entity{}
	for _, p := range processors {
		known[p.ID] = p
	}
	details := make([]pdk.EntityDetail, 0, len(list))
	for _, a := range list {
		d, err := pdk.Graph.Get(a.Slug)
		if err != nil {
			return nil, nil, err
		}
		details = append(details, d)
	}
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
	return collect(details, resolve), processors, nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "gdpr.register", "gdpr.gaps":
		acts, processors, err := load()
		if err != nil {
			return nil, err
		}
		today := time.Now().UTC()
		if tc.Name == "gdpr.gaps" {
			return gaps(acts, processors, today), nil
		}
		return map[string]any{"markdown": register(acts, processors, today), "activities": len(acts)}, nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
