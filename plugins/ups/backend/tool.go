package main

import (
	"encoding/json"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

type toolArgs struct {
	UPS string `json:"ups"`
}

func decodeArgs(raw json.RawMessage) (toolArgs, error) {
	var a toolArgs
	if len(raw) == 0 {
		return a, nil
	}
	return a, json.Unmarshal(raw, &a)
}

// UPSStatus is one line of the ups.status tool.
type UPSStatus struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Charge   any    `json:"chargePercent,omitempty"`
	Runtime  any    `json:"runtimeMinutes,omitempty"`
	Status   any    `json:"status,omitempty"`
	Polled   any    `json:"polledAt,omitempty"`
	Capacity any    `json:"capacityVA,omitempty"`
}

func summarize(list []pdk.Entity) []UPSStatus {
	out := []UPSStatus{}
	for _, e := range list {
		if e.Kind != "ups" {
			continue
		}
		out = append(out, UPSStatus{Slug: e.Slug, Name: e.Name, Charge: e.Attrs["ladung"], Runtime: e.Attrs["laufzeit"],
			Status: e.Attrs["status"], Polled: e.Attrs["abgefragt"], Capacity: e.Attrs["kapazitaet"]})
	}
	return out
}
