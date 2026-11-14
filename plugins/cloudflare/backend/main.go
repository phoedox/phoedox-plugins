//go:build wasip1

// Backend of the Cloudflare plugin: imports the DNS zones every six hours
// (and on cloudflare.sync), links them to the entries their records point to
// and lists the DNS records of a zone for AI clients. Read-only.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func main() {}

//go:wasmexport on_event
func onEvent() int32 {
	return pdk.HandleEvent(func(ev pdk.Event) error {
		if ev.Type != "schedule" {
			return nil
		}
		_, err := sync()
		return err
	})
}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

func headers() (map[string]string, error) {
	token, err := integ.Setting("token", "an API token with Zone:Read and DNS:Read")
	if err != nil {
		return nil, err
	}
	return map[string]string{"Authorization": "Bearer " + strings.TrimSpace(token), "Accept": "application/json"}, nil
}

// pages reads up to max pages of a list (path has a query string).
func pages[T any](h map[string]string, path string, max int) ([]T, int, error) {
	var out []T
	total := 0
	for page := 1; page <= max; page++ {
		res, err := pdk.Fetch(pdk.FetchRequest{URL: fmt.Sprintf("%s%s&page=%d", api, path, page), Headers: h})
		if err != nil {
			return nil, 0, err
		}
		e, err := parsePage[T](res.Status, []byte(res.Body))
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e.Result...)
		total = e.ResultInfo.TotalCount
		if page >= e.ResultInfo.TotalPages || len(e.Result) == 0 {
			break
		}
	}
	return out, total, nil
}

// records reads up to 2000 records of a zone.
func records(h map[string]string, zoneID string) ([]Record, int, error) {
	return pages[Record](h, "/zones/"+url.PathEscape(zoneID)+"/dns_records?per_page=100", 20)
}

func sync() (map[string]int, error) {
	h, err := headers()
	if err != nil {
		return nil, err
	}
	zones, _, err := pages[Zone](h, "/zones?per_page=50", 20)
	if err != nil {
		return nil, err
	}
	data := make([]ZoneData, 0, len(zones))
	n := 0
	for _, z := range zones {
		recs, total, err := records(h, z.ID)
		if err != nil {
			return nil, fmt.Errorf("zone %s: %w", z.Name, err)
		}
		data = append(data, ZoneData{Zone: z, Records: recs, Total: total})
		n += len(recs)
	}
	ops := graphOps(data, integ.GraphMatcher())
	err = integ.ApplyBatches(ops, 500, func(b *pdk.GraphOps) (map[string]string, error) {
		r, err := pdk.Graph.Apply(b)
		return r.Refs, err
	})
	return map[string]int{"zones": len(zones), "records": n}, err
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "cloudflare.sync":
		return sync()
	case "cloudflare.records":
		var in struct {
			Zone string `json:"zone"`
		}
		_ = json.Unmarshal(tc.Args, &in)
		name, err := validZone(in.Zone)
		if err != nil {
			return nil, err
		}
		h, err := headers()
		if err != nil {
			return nil, err
		}
		zones, _, err := pages[Zone](h, "/zones?per_page=5&name="+url.QueryEscape(name), 1)
		if err != nil {
			return nil, err
		}
		if len(zones) == 0 {
			return nil, fmt.Errorf("zone %s not found (or the token cannot read it)", name)
		}
		recs, _, err := records(h, zones[0].ID)
		if err != nil {
			return nil, err
		}
		if recs == nil {
			recs = []Record{}
		}
		return recs, nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}
