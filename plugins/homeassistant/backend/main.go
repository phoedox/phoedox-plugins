//go:build wasip1

// Backend of the Home Assistant plugin: imports areas and devices every six
// hours (and on ha.sync), answers ha.state, and switches (ha.call,
// ha.restart) – control tools that people confirm when an AI asks.
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/phoedox/phoedox-sdk/go/pdk"

	"github.com/phoedox/phoedox-plugins/internal/integ"
)

func main() {}

//go:wasmexport on_event
func onEvent() int32 {
	return pdk.HandleEvent(func(ev pdk.Event) error {
		if ev.Type == "agent.homeassistant" {
			return bridged(ev.Data)
		}
		if ev.Type != "schedule" {
			return nil
		}
		if _, _, err := settings(); err != nil {
			return nil // connected through an agent (or not set up yet)
		}
		// Every 5 minutes: record states; every 6 hours: import devices.
		var last time.Time
		if _, err := pdk.KV.Get("last_sync", &last); err == nil && time.Since(last) < 6*time.Hour {
			_, err := record()
			return err
		}
		if _, err := sync(); err != nil {
			return err
		}
		_ = pdk.KV.Set("last_sync", time.Now())
		_, err := record()
		return err
	})
}

//go:wasmexport tool
func tool() int32 { return pdk.HandleTool(runTool) }

func settings() (base, token string, err error) {
	if base, err = pdk.Secret("url"); err != nil {
		return "", "", fmt.Errorf("set the setting url (e.g. https://ha.example.org), or, if a Phoedox agent connects Home Assistant, switch with the agent actions on the entry: %w", err)
	}
	if token, err = pdk.Secret("token"); err != nil {
		return "", "", fmt.Errorf("set the setting token (a long-lived access token): %w", err)
	}
	return strings.TrimRight(base, "/"), token, nil
}

func call(method, path, body string) ([]byte, error) {
	base, token, err := settings()
	if err != nil {
		return nil, err
	}
	res, err := pdk.Fetch(pdk.FetchRequest{Method: method, URL: base + path, Body: body,
		Headers: map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"}})
	if err != nil {
		return nil, err
	}
	if res.Status != 200 {
		return nil, fmt.Errorf("Home Assistant answered HTTP %d", res.Status)
	}
	return []byte(res.Body), nil
}

func sync() (map[string]int, error) {
	tpl, _ := json.Marshal(map[string]string{"template": registryTemplate})
	body, err := call("POST", "/api/template", string(tpl))
	if err != nil {
		return nil, err
	}
	devices, err := parseDevices(body)
	if err != nil {
		return nil, err
	}
	if err := integ.Apply(graphOps(devices)); err != nil {
		return nil, err
	}
	return map[string]int{"devices": len(devices)}, nil
}

// bridged imports devices and records states an agent read from Home
// Assistant in its network; the devices are switched through that agent.
func bridged(data []byte) error {
	var b Bridge
	if err := json.Unmarshal(data, &b); err != nil {
		return fmt.Errorf("unexpected data from the agent: %w", err)
	}
	if len(b.Devices) > 0 && string(b.Devices) != "null" {
		devices, err := parseDevices(b.Devices)
		if err != nil {
			return err
		}
		if err := integ.Apply(graphOpsVia(devices, b.Agent)); err != nil {
			return err
		}
	}
	var entries []pdk.Entity
	for _, kind := range []string{"device", "smart_plug", "light"} {
		list, err := pdk.Graph.Query(pdk.Query{Kind: kind, Limit: 1000})
		if err != nil {
			return err
		}
		entries = append(entries, list...)
	}
	rs := Readings(b.BridgeStates(), Owners(entries))
	for len(rs) > 0 {
		n := min(len(rs), 500)
		if _, err := pdk.Graph.Record(rs[:n]); err != nil {
			return err
		}
		rs = rs[n:]
	}
	return nil
}

// record stores the states of the entities listed on imported entries.
func record() (pdk.RecordResult, error) {
	var entries []pdk.Entity
	for _, kind := range []string{"device", "smart_plug", "light"} {
		list, err := pdk.Graph.Query(pdk.Query{Kind: kind, Limit: 1000})
		if err != nil {
			return pdk.RecordResult{}, err
		}
		entries = append(entries, list...)
	}
	owners := Owners(entries)
	if len(owners) == 0 {
		return pdk.RecordResult{}, nil
	}
	body, err := call("GET", "/api/states", "")
	if err != nil {
		return pdk.RecordResult{}, err
	}
	var states []State
	if err := json.Unmarshal(body, &states); err != nil {
		return pdk.RecordResult{}, fmt.Errorf("unexpected answer from Home Assistant: %w", err)
	}
	var total pdk.RecordResult
	rs := Readings(states, owners)
	for len(rs) > 0 {
		n := min(len(rs), 500)
		r, err := pdk.Graph.Record(rs[:n])
		if err != nil {
			return total, err
		}
		total.Recorded += r.Recorded
		total.Skipped += r.Skipped
		rs = rs[n:]
	}
	return total, nil
}

func runTool(tc pdk.ToolCall) (any, error) {
	switch tc.Name {
	case "ha.sync":
		return sync()
	case "ha.record":
		return record()
	case "ha.state":
		var args struct {
			Entities []string `json:"entities"`
		}
		_ = json.Unmarshal(tc.Args, &args)
		list, err := validEntities(args.Entities)
		if err != nil {
			return nil, err
		}
		out := []State{}
		for _, e := range list {
			body, err := call("GET", "/api/states/"+e, "")
			var s State
			if err == nil {
				err = json.Unmarshal(body, &s)
			}
			if err != nil {
				s = State{EntityID: e, Error: err.Error()}
			}
			out = append(out, s)
		}
		return out, nil
	case "ha.controls":
		var args struct {
			Entity string `json:"entity"`
		}
		_ = json.Unmarshal(tc.Args, &args)
		return controls(args.Entity)
	case "ha.call":
		var args CallArgs
		_ = json.Unmarshal(tc.Args, &args)
		domain, svc, err := args.service()
		if err != nil {
			return nil, err
		}
		if err := service(domain, svc, args.EntityID); err != nil {
			return nil, err
		}
		return currentState(args.EntityID), nil
	case "ha.restart":
		var args RestartArgs
		_ = json.Unmarshal(tc.Args, &args)
		if !Switchable(args.EntityID) {
			return nil, fmt.Errorf("%q is not a switch", args.EntityID)
		}
		domain := strings.SplitN(args.EntityID, ".", 2)[0]
		if err := service(domain, "turn_off", args.EntityID); err != nil {
			return nil, err
		}
		time.Sleep(time.Duration(args.pause()) * time.Second)
		if err := service(domain, "turn_on", args.EntityID); err != nil {
			return nil, fmt.Errorf("switched off, but switching on again failed – switch it on in Home Assistant: %w", err)
		}
		return currentState(args.EntityID), nil
	}
	return nil, fmt.Errorf("unknown tool %s", tc.Name)
}

func service(domain, svc, entity string) error {
	body, _ := json.Marshal(map[string]string{"entity_id": entity})
	_, err := call("POST", "/api/services/"+domain+"/"+svc, string(body))
	return err
}

func currentState(entity string) State {
	s := State{EntityID: entity}
	body, err := call("GET", "/api/states/"+entity, "")
	if err == nil {
		err = json.Unmarshal(body, &s)
	}
	if err != nil {
		s.Error = err.Error()
	}
	s.Attributes = nil
	return s
}

// controls finds the switch of an entry and the switch it is powered by.
func controls(slug string) (Controls, error) {
	var out Controls
	d, err := pdk.Graph.Get(slug)
	if err != nil {
		return out, err
	}
	out.Entry = d.Entity.Name
	if a, _ := d.Entity.Attrs["bridge_agent"].(string); a != "" {
		return out, nil // switched through the agent with signed jobs
	}
	if c := controlOf(d.Entity); c != nil {
		c.State = currentState(c.EntityID).State
		out.Self = c
	}
	for _, e := range d.Edges {
		if e.Rel != "powered_by" || (e.From != d.Entity.ID && e.From != d.Entity.Slug) {
			continue
		}
		if p, err := pdk.Graph.Get(e.To); err == nil {
			if c := controlOf(p.Entity); c != nil {
				c.State = currentState(c.EntityID).State
				out.Power = c
				break
			}
		}
	}
	return out, nil
}
