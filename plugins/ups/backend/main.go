//go:build wasip1

// Backend of the UPS monitoring plugin. Every 60 s it asks the NUT HTTP API
// for each UPS entry of the project and stores charge, runtime and status.
package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport on_event
func onEvent() int32 { return pdk.HandleEvent(poll) }

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

func poll(ev pdk.Event) error {
	if ev.Type != "schedule" {
		return nil
	}
	list, err := pdk.Graph.Query(pdk.Query{Kind: "ups", Limit: 200})
	if err != nil {
		return err
	}
	auth := ""
	if pw, err := pdk.Secret("nut_password"); err == nil {
		auth = "Basic " + base64.StdEncoding.EncodeToString([]byte("monuser:"+pw))
	} else if !pdk.IsCode(err, "not_found") {
		return err
	}
	var ops pdk.GraphOps
	failed := 0
	for _, e := range list {
		req := pdk.FetchRequest{URL: NUTBase + nutName(e), Headers: map[string]string{"Accept": "application/json"}}
		if auth != "" {
			req.Headers["Authorization"] = auth
		}
		res, err := pdk.Fetch(req)
		if err == nil && res.Status != 200 {
			err = fmt.Errorf("HTTP %d", res.Status)
		}
		var r Reading
		if err == nil {
			r, err = parseNUT([]byte(res.Body))
		}
		if err != nil {
			pdk.Warn(fmt.Sprintf("%s: %v", e.Slug, err))
			failed++
			continue
		}
		ops.Upsert("ups", e.Slug, map[string]any{"ladung": r.Charge, "laufzeit": r.Runtime, "status": r.Status, "abgefragt": time.Now().UTC().Format(time.RFC3339)})
	}
	if len(ops.Ops) > 0 {
		if _, err := pdk.Graph.Apply(&ops); err != nil {
			return err
		}
	}
	if failed > 0 && failed == len(list) {
		return errors.New("no UPS answered")
	}
	return nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	if tc.Name != "ups.status" {
		return nil, fmt.Errorf("unknown tool %s", tc.Name)
	}
	args, err := decodeArgs(tc.Args)
	if err != nil {
		return nil, err
	}
	var list []pdk.Entity
	if args.UPS != "" {
		d, err := pdk.Graph.Get(args.UPS)
		if err != nil {
			return nil, err
		}
		list = []pdk.Entity{d.Entity}
	} else if list, err = pdk.Graph.Query(pdk.Query{Kind: "ups"}); err != nil {
		return nil, err
	}
	return summarize(list), nil
}
