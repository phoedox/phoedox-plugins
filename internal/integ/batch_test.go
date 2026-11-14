package integ

import (
	"errors"
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

func TestApplyBatches(t *testing.T) {
	var ops pdk.GraphOps
	a := ops.UpsertNamed("server", "a", "A", nil)
	b := ops.UpsertNamed("vm", "b", "B", nil)
	ops.Link(b, "runs_on", a, nil)
	var seen [][]pdk.GraphOp
	err := ApplyBatches(ops, 2, func(g *pdk.GraphOps) (map[string]string, error) {
		seen = append(seen, g.Ops)
		refs := map[string]string{}
		for _, o := range g.Ops {
			if o.Ref != "" {
				refs[o.Ref] = o.ID
			}
		}
		return refs, nil
	})
	if err != nil || len(seen) != 2 || seen[1][0].From != "b" || seen[1][0].To != "a" {
		t.Fatalf("batches: %+v %v", seen, err)
	}
	if ApplyBatches(ops, MaxOps, func(*pdk.GraphOps) (map[string]string, error) { return nil, errors.New("denied") }) == nil {
		t.Fatal("error swallowed")
	}
}
