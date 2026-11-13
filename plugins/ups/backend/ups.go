// Package logic of the UPS plugin, kept free of WASM so it can be tested
// natively.
package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// NUTBase is the HTTP API of the NUT web front end (PeaNUT style:
// GET /api/v1/devices/<name> returns NUT variables as JSON).
const NUTBase = "http://nut.lan/api/v1/devices/"

// Reading is what the plugin stores on a UPS entry.
type Reading struct {
	Charge  float64 `json:"ladung"`
	Runtime float64 `json:"laufzeit"` // minutes
	Status  string  `json:"status"`
}

// parseNUT reads NUT variables ("battery.charge", "battery.runtime" in
// seconds, "ups.status" like "OL CHRG"). Values may be strings or numbers.
func parseNUT(body []byte) (Reading, error) {
	var vars map[string]any
	if err := json.Unmarshal(body, &vars); err != nil {
		return Reading{}, fmt.Errorf("NUT answer is not JSON: %w", err)
	}
	num := func(k string) (float64, bool) {
		switch v := vars[k].(type) {
		case float64:
			return v, true
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			return f, err == nil
		}
		return 0, false
	}
	var r Reading
	var ok bool
	if r.Charge, ok = num("battery.charge"); !ok {
		return r, fmt.Errorf("NUT answer has no battery.charge")
	}
	if sec, ok := num("battery.runtime"); ok {
		r.Runtime = float64(int(sec/60*10+0.5)) / 10
	}
	r.Status = status(fmt.Sprint(vars["ups.status"]))
	return r, nil
}

// status maps NUT flags to the select values of the kind.
func status(flags string) string {
	f := strings.Fields(flags)
	has := func(s string) bool {
		for _, x := range f {
			if x == s {
				return true
			}
		}
		return false
	}
	switch {
	case has("LB"):
		return "batterie_schwach"
	case has("OB"):
		return "batterie"
	case has("OL"):
		return "online"
	}
	return "unbekannt"
}

// nutName is the device name in NUT: attribute nut_name or the slug.
func nutName(e pdk.Entity) string {
	if n, _ := e.Attrs["nut_name"].(string); n != "" {
		return n
	}
	return e.Slug
}
