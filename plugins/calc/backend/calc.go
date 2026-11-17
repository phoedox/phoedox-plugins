// Package main is the calculator block (ADR-0087): inputs and formulas inside
// a page, one line per value, "Label (unit) = number, formula or
// @entry.attribute", drawn as SVG by block_render. The logic has no build tag
// so it is tested natively.
package main

import (
	"fmt"
	"html"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Entity is the entry attribute an input is bound to ("= @nas.power_w").
type Entity struct{ Slug, Attr string }

// Row is one value of the calculator.
type Row struct {
	Line   int // in the source, from 1
	Label  string
	Name   string // the name formulas use
	Unit   string
	Expr   string
	Input  bool
	Value  float64 // inputs: the number of the source (0 for entry inputs)
	Entity *Entity
	node   *Node
	err    error // the formula cannot be read
}

// Calc is a parsed calculator.
type Calc struct {
	Title string
	Rows  []*Row
}

// Problem is a finding in the source; Line counts from 1.
type Problem struct {
	Line    int
	Message string
}

var (
	keyRe     = regexp.MustCompile(`^([A-Za-z]+)\s*:\s*(.*)$`)
	unitRe    = regexp.MustCompile(`^(.*?)\s*\(([^)]*)\)\s*$`)
	nonNameRe = regexp.MustCompile(`[^\p{L}\p{N}_]+`)
	numRe     = regexp.MustCompile(`^-?[0-9][0-9 ]*(?:[.,][0-9]+)?$`)
	entityRe  = regexp.MustCompile(`^@([a-z0-9-]+)\.([A-Za-z0-9_]+)$`)
)

// CalcName turns a label into the name formulas use and its unit:
// "Cost per year (€)" → Cost_per_year, €.
func CalcName(label string) (name, unit string) {
	base := label
	if m := unitRe.FindStringSubmatch(label); m != nil {
		base, unit = m[1], strings.TrimSpace(m[2])
	}
	return strings.Trim(nonNameRe.ReplaceAllString(strings.TrimSpace(base), "_"), "_"), unit
}

// Parse reads a calculator; problems name the line they are on.
func Parse(src string) (*Calc, []Problem) {
	c := &Calc{}
	var ps []Problem
	byName := map[string]*Row{}
	for i, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		n := i + 1
		l := strings.TrimSpace(raw)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if m := keyRe.FindStringSubmatch(l); m != nil && (strings.EqualFold(m[1], "title") || strings.EqualFold(m[1], "titel")) {
			c.Title = strings.TrimSpace(m[2])
			continue
		}
		eq := strings.Index(l, "=")
		if eq <= 0 {
			ps = append(ps, Problem{n, fmt.Sprintf("%q: a line needs label = value or formula", l)})
			continue
		}
		label, expr := strings.TrimSpace(l[:eq]), strings.TrimSpace(l[eq+1:])
		name, unit := CalcName(label)
		if name == "" || expr == "" {
			ps = append(ps, Problem{n, fmt.Sprintf("%q: a line needs label = value or formula", l)})
			continue
		}
		if byName[name] != nil {
			ps = append(ps, Problem{n, fmt.Sprintf("the name %q is there twice", name)})
		}
		r := &Row{Line: n, Label: label, Name: name, Unit: unit, Expr: expr}
		byName[name] = r
		c.Rows = append(c.Rows, r)
		if numRe.MatchString(expr) {
			r.Input = true
			r.Value, _ = strconv.ParseFloat(strings.Replace(strings.ReplaceAll(expr, " ", ""), ",", ".", 1), 64)
		} else if m := entityRe.FindStringSubmatch(expr); m != nil {
			r.Input, r.Entity = true, &Entity{Slug: m[1], Attr: m[2]}
		} else if r.node, r.err = ParseFormula(expr); r.err != nil {
			ps = append(ps, Problem{n, fmt.Sprintf("the formula of %q cannot be read: %v", name, r.err)})
		}
	}
	for _, r := range c.Rows {
		if r.node == nil {
			continue
		}
		for _, u := range r.node.names(map[string]bool{}, nil) {
			if byName[u] == nil {
				ps = append(ps, Problem{r.Line, fmt.Sprintf("%q uses %q, which does not exist (names are labels with _ for spaces, without the unit)", r.Name, u)})
			}
		}
	}
	if len(c.Rows) == 0 {
		ps = append(ps, Problem{1, "the calculator is empty"})
	}
	if len(ps) == 0 {
		if r := cycle(c, byName); r != nil {
			ps = append(ps, Problem{r.Line, fmt.Sprintf("%q depends on itself", r.Name)})
		}
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Line < ps[j].Line })
	return c, ps
}

// cycle returns a row whose formula (through others) uses itself.
func cycle(c *Calc, byName map[string]*Row) *Row {
	state := map[*Row]int{} // 1 = on the path, 2 = done
	var visit func(*Row) *Row
	visit = func(r *Row) *Row {
		if r == nil {
			return nil
		}
		switch state[r] {
		case 1:
			return r
		case 2:
			return nil
		}
		state[r] = 1
		if r.node != nil {
			for _, u := range r.node.names(map[string]bool{}, nil) {
				if hit := visit(byName[u]); hit != nil {
					return hit
				}
			}
		}
		state[r] = 2
		return nil
	}
	for _, r := range c.Rows {
		if hit := visit(r); hit != nil {
			return hit
		}
	}
	return nil
}

// Values computes all rows: inputs (overridden by inputs, which a reader
// changed) and formulas; failed values are *FormulaError.
func Values(c *Calc, inputs map[string]float64) map[string]any {
	out := map[string]any{}
	rows := map[string]*Row{}
	for _, r := range c.Rows {
		rows[r.Name] = r
	}
	busy := map[string]bool{}
	var get Getter
	get = func(name string) (any, error) {
		if v, ok := out[name]; ok {
			if e, ok := v.(*FormulaError); ok {
				return nil, e
			}
			return v, nil
		}
		r := rows[name]
		if r == nil {
			return nil, ferr("name")
		}
		if busy[name] {
			return nil, ferr("cycle")
		}
		busy[name] = true
		defer delete(busy, name)
		var v any
		var err error
		switch {
		case r.Input:
			if x, ok := inputs[name]; ok {
				v = x
			} else {
				v = r.Value
			}
		case r.node == nil:
			err = r.err
			if err == nil {
				err = ferr("syntax")
			}
		default:
			v, err = r.node.Eval(get)
		}
		if err != nil {
			fe, ok := err.(*FormulaError)
			if !ok {
				fe = ferr("value")
			}
			out[name] = fe
			return nil, fe
		}
		out[name] = v
		return v, nil
	}
	for _, r := range c.Rows {
		_, _ = get(r.Name)
	}
	return out
}

func lang(locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "de") {
		return "de"
	}
	return "en"
}

// FormatValue shows a value like the core's formatValue: numbers with at
// most 2 decimals (none from 100 on) in the reader's language, ✓/✗ for
// comparisons and #DIV/0, #CYCLE or #ERROR for failed formulas.
func FormatValue(v any, locale string) string {
	de := lang(locale) == "de"
	switch x := v.(type) {
	case *FormulaError:
		switch {
		case x.Code == "div0":
			return "#DIV/0"
		case x.Code == "cycle" && de:
			return "#ZYKLUS"
		case x.Code == "cycle":
			return "#CYCLE"
		case de:
			return "#FEHLER"
		}
		return "#ERROR"
	case float64:
		frac := 2
		if math.Abs(x) >= 100 {
			frac = 0
		}
		return formatNumber(x, frac, de)
	case bool:
		if x {
			return "✓"
		}
		return "✗"
	case string:
		return x
	}
	return ""
}

// formatNumber is Intl.NumberFormat(locale, {maximumFractionDigits: frac})
// for en and de: grouping by thousands, halves rounded away from zero on the
// exact binary value, trailing zeros dropped.
func formatNumber(x float64, frac int, de bool) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "∞"
	case math.IsInf(x, -1):
		return "-∞"
	}
	group, point := ",", "."
	if de {
		group, point = ".", ","
	}
	f := new(big.Float).SetPrec(256).SetFloat64(math.Abs(x))
	f.Mul(f, new(big.Float).SetPrec(256).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(frac)), nil)))
	n, _ := f.Int(nil)
	if rest := new(big.Float).SetPrec(256).Sub(f, new(big.Float).SetPrec(256).SetInt(n)); rest.Cmp(big.NewFloat(0.5)) >= 0 {
		n.Add(n, big.NewInt(1))
	}
	digits := n.String()
	for len(digits) <= frac {
		digits = "0" + digits
	}
	ip, fp := digits[:len(digits)-frac], strings.TrimRight(digits[len(digits)-frac:], "0")
	var sb strings.Builder
	if math.Signbit(x) {
		sb.WriteString("-")
	}
	for i, d := range ip {
		if i > 0 && (len(ip)-i)%3 == 0 {
			sb.WriteString(group)
		}
		sb.WriteRune(d)
	}
	if fp != "" {
		sb.WriteString(point + fp)
	}
	return sb.String()
}

func px(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }

// SVG draws the calculator with its starting values: a label on the left,
// the value with its unit on the right, results in bold. Inputs from entries
// show "–" (the picture does not read the graph).
func SVG(c *Calc, width int, locale string) string {
	if width <= 0 {
		width = 480
	}
	w := float64(max(width, 240))
	esc := html.EscapeString
	vals := Values(c, nil)
	top := 6.0
	if c.Title != "" {
		top = 34
	}
	h := top + float64(len(c.Rows))*28 + 6
	row := math.Min(30, math.Max(22, (h-top-4)/math.Max(1, float64(len(c.Rows)))))
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s" style="font-family:var(--font-body,system-ui,sans-serif)">`, px(w), px(h), px(w), px(h))
	if c.Title != "" {
		fmt.Fprintf(&sb, `<text x="6" y="22" style="font-size:16px;font-weight:600;fill:var(--fg)">%s</text>`, esc(c.Title))
	}
	for i, r := range c.Rows {
		y := top + float64(i)*row + row/2 + 4
		v := "–"
		if r.Entity == nil {
			v = FormatValue(vals[r.Name], locale)
		}
		if r.Unit != "" {
			v += " " + r.Unit
		}
		color, weight := "fg", ";font-weight:600"
		if r.Input {
			color, weight = "muted", ""
		}
		fmt.Fprintf(&sb, `<text x="6" y="%s" style="font-size:13px;fill:var(--%s)">%s</text><text x="%s" y="%s" text-anchor="end" style="font-size:13px;fill:var(--fg)%s">%s</text>`,
			px(y), color, esc(r.Label), px(w-6), px(y), weight, esc(v))
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}
