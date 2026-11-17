//go:build wasip1

package main

import "github.com/phoedox/phoedox-sdk/go/pdk"

func main() {}

//go:wasmexport block_render
func blockRender() int32 { return pdk.HandleBlockRender(render) }

func render(in pdk.BlockRender) (pdk.BlockResult, error) {
	root, ps := Parse(in.Source)
	out := pdk.BlockResult{}
	for _, p := range ps {
		out.Problems = append(out.Problems, pdk.BlockProblem{Line: max(1, p.Line), Message: p.Message, Severity: "error"})
	}
	if len(ps) > 0 {
		return out, nil
	}
	width := in.Width
	if width <= 0 {
		width = 720
	}
	out.SVG = SVG(root, width, nil)
	return out, nil
}
