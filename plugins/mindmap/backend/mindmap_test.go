package main

import (
	"strings"
	"testing"
)

const example = "Moving\n  Flat\n    Give notice\n    Handover\n  Internet\n    Contract\n  - Boxes\n    - Kitchen"

func TestParse(t *testing.T) {
	root, ps := Parse(example)
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	var names []string
	for _, c := range root.Children {
		names = append(names, c.Text)
	}
	if root.Text != "Moving" || strings.Join(names, ",") != "Flat,Internet,Boxes" {
		t.Errorf("branches: %q %v", root.Text, names)
	}
	if c := root.Children[1].Children[0]; c.Text != "Contract" || c.ID != "0.1.0" {
		t.Errorf("child: %+v", c)
	}
	if k := root.Children[2].Children; len(k) != 1 || k[0].Text != "Kitchen" {
		t.Errorf("list items: %+v", k)
	}
	// Tabs count as two spaces, comments and empty lines are skipped.
	root, _ = Parse("# notes\r\n- Centre\r\n\tA\r\n\t\tA1\r\n\r\n\tB")
	if root.Text != "Centre" || len(root.Children) != 2 || root.Children[0].Children[0].Text != "A1" {
		t.Errorf("tabs: %+v", root)
	}
	// A shallower line after a deep one goes back up.
	root, _ = Parse("R\n    deep\n  mid")
	if len(root.Children) != 2 {
		t.Errorf("dedent: %+v", root.Children)
	}
}

func TestProblems(t *testing.T) {
	for _, src := range []string{"", "  \n", "# only a comment"} {
		_, ps := Parse(src)
		if len(ps) != 1 || ps[0].Line != 1 || !strings.Contains(ps[0].Message, "empty") {
			t.Errorf("%q: %v", src, ps)
		}
	}
	src := "Root\n# comment\n" + strings.Repeat("  item\n", MaxNodes)
	root, ps := Parse(src)
	if root != nil || len(ps) != 1 || !strings.Contains(ps[0].Message, "at most 400 items, this one has 401") || ps[0].Line != MaxNodes+2 {
		t.Errorf("too many: %v", ps)
	}
	if _, ps := Parse("Root\n" + strings.Repeat("  item\n", MaxNodes-1)); len(ps) > 0 {
		t.Errorf("400 items are fine: %v", ps)
	}
}

func TestLayout(t *testing.T) {
	root, _ := Parse(example)
	nodes, w, h := Layout(root, nil)
	if len(nodes) != 8 || w <= 0 || h <= 0 {
		t.Fatalf("nodes %d, size %g×%g", len(nodes), w, h)
	}
	sides := map[int]bool{}
	for _, n := range nodes {
		if n.Depth == 1 {
			sides[n.Side] = true
		}
		if n.X < 0 || n.Y < 0 || n.X+n.W > w || n.Y+n.H > h {
			t.Errorf("%q outside the picture", n.Node.Text)
		}
	}
	if !sides[1] || !sides[-1] {
		t.Errorf("branches on both sides: %v", sides)
	}
	for _, a := range nodes {
		for _, b := range nodes {
			if a != b && a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H {
				t.Errorf("%q overlaps %q", a.Node.Text, b.Node.Text)
			}
		}
	}
	c, _, _ := Layout(root, map[string]bool{"0.0": true})
	if len(c) != len(nodes)-2 {
		t.Errorf("collapsed: %d nodes", len(c))
	}
	for _, n := range c {
		if n.Node.ID == "0.0" && n.Hidden != 2 {
			t.Errorf("hidden: %d", n.Hidden)
		}
	}
	// A single branch goes right; long texts are capped at 240 px.
	one, _ := Parse("R\n  " + strings.Repeat("long ", 80))
	ns, _, _ := Layout(one, nil)
	if ns[1].Side != 1 || ns[1].W != 240 {
		t.Errorf("single branch: %+v", ns[1])
	}
}

func TestSVG(t *testing.T) {
	root, _ := Parse(example)
	svg := SVG(root, 720, nil)
	for _, want := range []string{"<svg", `aria-label="Moving"`, "var(--tc-blue)", "var(--tb-green)", "var(--surface2)", ">Give notice</text>"} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(svg, "<script") || strings.Contains(svg, "aria-expanded") {
		t.Error("static picture only")
	}
	if !strings.Contains(SVG(root, 720, map[string]bool{"0.0": true}), ">2</text></g>") {
		t.Error("badge of a collapsed branch")
	}
	if small := SVG(root, 200, nil); !strings.Contains(small, `width="200"`) {
		t.Errorf("shrinks to the width: %.140s", small)
	}
	evil, _ := Parse(`<script>alert("x")</script>` + "\n  a & b\n  " + strings.Repeat("x", 70))
	s := SVG(evil, 720, nil)
	if strings.Contains(s, "<script") || !strings.Contains(s, "a &amp; b") || !strings.Contains(s, strings.Repeat("x", 58)+"…<") {
		t.Errorf("escaping and shortening: %s", s)
	}
}
