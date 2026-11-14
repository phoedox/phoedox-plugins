package integ

import (
	"slices"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// MaxOps is the host's limit of graph operations per apply call.
const MaxOps = 500

// ApplyBatches applies graph operations in batches of at most size. "$n"
// references of entities created in earlier batches are replaced with the
// slugs the host returned, so links may cross batch borders.
func ApplyBatches(ops pdk.GraphOps, size int, apply func(*pdk.GraphOps) (map[string]string, error)) error {
	known := map[string]string{}
	for start := 0; start < len(ops.Ops); start += size {
		batch := pdk.GraphOps{Ops: slices.Clone(ops.Ops[start:min(start+size, len(ops.Ops))])}
		for i := range batch.Ops {
			if s, ok := known[batch.Ops[i].From]; ok {
				batch.Ops[i].From = s
			}
			if s, ok := known[batch.Ops[i].To]; ok {
				batch.Ops[i].To = s
			}
		}
		refs, err := apply(&batch)
		if err != nil {
			return err
		}
		for k, v := range refs {
			known[k] = v
		}
	}
	return nil
}
