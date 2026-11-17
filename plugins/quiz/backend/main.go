//go:build wasip1

package main

import "github.com/phoedox/phoedox-sdk/go/pdk"

func main() {}

//go:wasmexport block_render
func blockRender() int32 { return pdk.HandleBlockRender(render) }

func render(in pdk.BlockRender) (pdk.BlockResult, error) {
	z, ps := Parse(in.Source)
	out := pdk.BlockResult{}
	for _, p := range sortedProblems(ps) {
		out.Problems = append(out.Problems, pdk.BlockProblem{Line: max(1, p.Line), Message: p.Message, Severity: "error"})
	}
	if len(ps) > 0 {
		return out, nil
	}
	out.SVG = SVG(z, in.Width)
	return out, nil
}
