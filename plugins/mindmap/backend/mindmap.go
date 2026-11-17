// Package main is the mind map block (ADR-0087): an indented outline (spaces,
// tabs or "- " lists) becomes a map with the first line in the middle and its
// branches to both sides, drawn as SVG by block_render. The logic has no build
// tag so it is tested natively; it follows frontend/index.ts (the interactive
// view) so both draw the same picture.
package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxNodes is the most items a mind map may have.
const MaxNodes = 400

// Node is one item of the outline; ID is its path ("0", "0.1", "0.1.2").
type Node struct {
	Text     string
	Children []*Node
	ID       string
}

// Problem is a finding in the source; Line counts from 1.
type Problem struct {
	Line    int
	Message string
}

var (
	listRe = regexp.MustCompile(`^[-*+]\s+`)
	// escaper writes the same entities as the frontend.
	escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
)

// indent counts leading spaces, a tab counting as two.
func indent(l string) int {
	n := 0
	for _, r := range l {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 2
		default:
			return n
		}
	}
	return n
}

func clean(l string) string { return listRe.ReplaceAllString(strings.TrimSpace(l), "") }

// Parse reads an outline: the first line is the root, deeper indented lines
// are children of the line above them. Empty lines and # comments are skipped.
func Parse(src string) (*Node, []Problem) {
	type line struct {
		n    int
		text string
	}
	var lines []line
	for i, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(raw)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		lines = append(lines, line{i + 1, raw})
	}
	if len(lines) == 0 {
		return nil, []Problem{{1, "the mind map is empty: write the centre on the first line and its branches indented below it"}}
	}
	if len(lines) > MaxNodes {
		return nil, []Problem{{lines[MaxNodes].n, fmt.Sprintf("a mind map has at most %d items, this one has %d", MaxNodes, len(lines))}}
	}
	root := &Node{Text: clean(lines[0].text), ID: "0"}
	type open struct {
		node *Node
		ind  int
	}
	stack := []open{{root, -1}}
	for _, l := range lines[1:] {
		ind := indent(l.text)
		for len(stack) > 1 && stack[len(stack)-1].ind >= ind {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1].node
		node := &Node{Text: clean(l.text), ID: parent.ID + "." + strconv.Itoa(len(parent.Children))}
		parent.Children = append(parent.Children, node)
		stack = append(stack, open{node, ind})
	}
	return root, nil
}

func count(n *Node) int {
	c := 1
	for _, k := range n.Children {
		c += count(k)
	}
	return c
}

var branchColors = []string{"blue", "green", "orange", "purple", "pink", "red", "yellow"}

const (
	fontSize = 13.0
	rowH     = 30.0
	gapX     = 36.0
)

var (
	wideRe   = regexp.MustCompile(`[MWmw@%]`)
	upperRe  = regexp.MustCompile(`[A-ZÄÖÜ0-9]`)
	narrowRe = regexp.MustCompile(`[iljtf.,:;'|!]`)
)

// TextWidth is a rough text width without fonts: wide letters count more.
func TextWidth(s string, size float64) float64 {
	w := 0.0
	for _, r := range s {
		c := string(r)
		switch {
		case wideRe.MatchString(c):
			w += 0.9
		case upperRe.MatchString(c):
			w += 0.68
		case narrowRe.MatchString(c):
			w += 0.3
		default:
			w += 0.55
		}
	}
	return w * size
}

// Placed is a node with its box. Side is -1 (left), 0 (root) or 1 (right);
// Hidden counts the items of a collapsed branch.
type Placed struct {
	Node       *Node
	X, Y, W, H float64
	Side       int
	Color      string
	Depth      int
	Parent     *Placed
	Hidden     int
}

// Layout places the map: the root in the middle, branches split by size to
// the right and left. Branches whose ID is in collapsed hide their children.
func Layout(root *Node, collapsed map[string]bool) (nodes []*Placed, width, height float64) {
	kids := func(n *Node) []*Node {
		if collapsed[n.ID] {
			return nil
		}
		return n.Children
	}
	var leaves func(n *Node) int
	leaves = func(n *Node) int {
		ks := kids(n)
		if len(ks) == 0 {
			return 1
		}
		s := 0
		for _, c := range ks {
			s += leaves(c)
		}
		return s
	}
	branches := root.Children
	// Balance: fill the right side up to half of all leaves, the rest goes left.
	total := 0
	for _, b := range branches {
		total += leaves(b)
	}
	var right, left []*Node
	acc := 0
	for _, b := range branches {
		if float64(acc) < float64(total)/2 || len(right) == 0 {
			right = append(right, b)
			acc += leaves(b)
		} else {
			left = append(left, b)
		}
	}
	boxW := func(n *Node, d int) float64 {
		if d == 0 {
			return math.Min(240, TextWidth(n.Text, 16)+32)
		}
		return math.Min(240, TextWidth(n.Text, fontSize)+20)
	}
	// Column widths per side and depth.
	type col struct{ side, depth int }
	colW := map[col]float64{}
	var measure func(n *Node, d, side int)
	measure = func(n *Node, d, side int) {
		k := col{side, d}
		colW[k] = math.Max(colW[k], boxW(n, d))
		for _, c := range kids(n) {
			measure(c, d+1, side)
		}
	}
	for _, b := range right {
		measure(b, 1, 1)
	}
	for _, b := range left {
		measure(b, 1, -1)
	}
	rootW, rootH := boxW(root, 0), 40.0
	colX := func(side, d int) float64 {
		x := rootW/2 + gapX
		for i := 1; i < d; i++ {
			x += colW[col{side, i}] + gapX
		}
		return x
	}
	var place func(n *Node, d, side int, top float64, color string, parent *Placed) float64
	place = func(n *Node, d, side int, top float64, color string, parent *Placed) float64 {
		h := float64(leaves(n)) * rowH
		w := boxW(n, d)
		off := colX(side, d)
		x := off
		if side == -1 {
			x = -off - w
		}
		p := &Placed{Node: n, X: x, Y: top + h/2 - 12, W: w, H: 24, Side: side, Color: color, Depth: d, Parent: parent}
		if collapsed[n.ID] {
			p.Hidden = count(n) - 1
		}
		nodes = append(nodes, p)
		y := top
		for _, c := range kids(n) {
			y += place(c, d+1, side, y, color, p)
		}
		return h
	}
	rp := &Placed{Node: root, X: -rootW / 2, Y: -rootH / 2, W: rootW, H: rootH, Color: "gray"}
	nodes = append(nodes, rp)
	for _, s := range []struct {
		list []*Node
		side int
	}{{right, 1}, {left, -1}} {
		h := 0.0
		for _, b := range s.list {
			h += float64(leaves(b)) * rowH
		}
		y := -h / 2
		for _, b := range s.list {
			i := 0
			for i < len(branches) && branches[i] != b {
				i++
			}
			y += place(b, 1, s.side, y, branchColors[i%len(branchColors)], rp)
		}
	}
	// Shift into positive coordinates with a margin.
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, n := range nodes {
		minX, minY = math.Min(minX, n.X), math.Min(minY, n.Y)
		maxX, maxY = math.Max(maxX, n.X+n.W), math.Max(maxY, n.Y+n.H)
	}
	minX, minY, maxX, maxY = minX-8, minY-8, maxX+8, maxY+8
	for _, n := range nodes {
		n.X -= minX
		n.Y -= minY
	}
	return nodes, maxX - minX, maxY - minY
}

// num writes a coordinate with at most two decimals.
func num(f float64) string {
	return strconv.FormatFloat(math.Round(f*100)/100, 'f', -1, 64)
}

// label shortens long texts to 59 characters with an ellipsis.
func label(s string) string {
	if utf8.RuneCountInString(s) > 60 {
		return string([]rune(s)[:58]) + "…"
	}
	return s
}

// SVG draws the map; width is the space available (the picture shrinks to fit,
// never grows). Colours come from the theme's CSS variables.
func SVG(root *Node, width int, collapsed map[string]bool) string {
	nodes, w, h := Layout(root, collapsed)
	vw, vh := math.Ceil(w)+40, math.Ceil(h) // 20px each side for the "+n" badges of collapsed branches
	scale := 1.0
	if width > 0 && float64(width) < vw {
		scale = float64(width) / vw
	}
	esc := escaper.Replace
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-20 0 %s %s" width="%s" height="%s" role="img" aria-label="%s" style="font-family:var(--font-body,system-ui,sans-serif);max-width:100%%;height:auto">`,
		num(vw), num(vh), num(math.Ceil(vw*scale)), num(math.Ceil(vh*scale)), esc(root.Text))
	for _, n := range nodes {
		p := n.Parent
		if p == nil {
			continue
		}
		x1, x2 := p.X, n.X+n.W
		if n.Side == 1 {
			x1, x2 = p.X+p.W, n.X
		}
		y1, y2 := p.Y+p.H/2, n.Y+n.H/2
		mx := (x1 + x2) / 2
		sw := "1.5"
		if n.Depth == 1 {
			sw = "2.5"
		}
		fmt.Fprintf(&sb, `<path d="M%s %s C%s %s %s %s %s %s" fill="none" style="stroke:var(--tc-%s)" stroke-width="%s"/>`,
			num(x1), num(y1), num(mx), num(y1), num(mx), num(y2), num(x2), num(y2), n.Color, sw)
	}
	for _, n := range nodes {
		isRoot := n.Depth == 0
		fill, stroke, rx, sw, size, dy, weight := "var(--surface)", "var(--tc-"+n.Color+")", "8", "1", fontSize, 4.5, ""
		if n.Depth == 1 {
			fill, weight = "var(--tb-"+n.Color+")", "font-weight:600;"
		}
		if isRoot {
			fill, stroke, rx, sw, size, dy, weight = "var(--surface2)", "var(--line)", "12", "1.5", 16, 5.5, "font-weight:600;"
		}
		fmt.Fprintf(&sb, `<g><rect x="%s" y="%s" width="%s" height="%s" rx="%s" style="fill:%s;stroke:%s" stroke-width="%s"/>`,
			num(n.X), num(n.Y), num(n.W), num(n.H), rx, fill, stroke, sw)
		fmt.Fprintf(&sb, `<text x="%s" y="%s" text-anchor="middle" style="font-size:%spx;%sfill:var(--fg)">%s</text>`,
			num(n.X+n.W/2), num(n.Y+n.H/2+dy), num(size), weight, esc(label(n.Node.Text)))
		if n.Hidden > 0 {
			cx := n.X + n.W + 10
			if n.Side == -1 {
				cx = n.X - 10
			}
			badge := strconv.Itoa(n.Hidden)
			if n.Hidden > 99 {
				badge = "99+"
			}
			fmt.Fprintf(&sb, `<g><circle cx="%s" cy="%s" r="9" style="fill:var(--tc-%s)"/><text x="%s" y="%s" text-anchor="middle" style="font-size:10px;font-weight:600;fill:var(--bg)">%s</text></g>`,
				num(cx), num(n.Y+n.H/2), n.Color, num(cx), num(n.Y+n.H/2+3.5), badge)
		}
		sb.WriteString(`</g>`)
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}
