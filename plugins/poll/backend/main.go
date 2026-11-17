//go:build wasip1

package main

import "github.com/phoedox/phoedox-sdk/go/pdk"

func main() {}

//go:wasmexport block_render
func blockRender() int32 { return pdk.HandleBlockRender(render) }

func render(in pdk.BlockRender) (pdk.BlockResult, error) {
	p, ps := Parse(in.Source)
	out := pdk.BlockResult{}
	for _, x := range sortedProblems(ps) {
		out.Problems = append(out.Problems, pdk.BlockProblem{Line: max(1, x.Line), Message: x.Message, Severity: "error"})
	}
	if len(ps) > 0 {
		return out, nil
	}
	out.SVG = SVG(p, in.Width)
	return out, nil
}
