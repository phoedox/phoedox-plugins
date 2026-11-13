package main

import (
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func TestParseNUT(t *testing.T) {
	r, err := parseNUT([]byte(`{"battery.charge":"87","battery.runtime":1260,"ups.status":"OL CHRG"}`))
	if err != nil || r.Charge != 87 || r.Runtime != 21 || r.Status != "online" {
		t.Fatalf("%+v %v", r, err)
	}
	if r, _ := parseNUT([]byte(`{"battery.charge":12,"ups.status":"OB LB"}`)); r.Status != "batterie_schwach" {
		t.Fatalf("%+v", r)
	}
	if _, err := parseNUT([]byte(`{"ups.status":"OL"}`)); err == nil {
		t.Fatal("charge is required")
	}
	if nutName(pdk.Entity{Slug: "usv-keller"}) != "usv-keller" || nutName(pdk.Entity{Slug: "x", Attrs: map[string]any{"nut_name": "ups1"}}) != "ups1" {
		t.Fatal("nut name")
	}
	s := summarize([]pdk.Entity{{Kind: "ups", Slug: "a", Attrs: map[string]any{"ladung": 80.0}}, {Kind: "device"}})
	if len(s) != 1 || s[0].Charge != 80.0 {
		t.Fatalf("%+v", s)
	}
}
