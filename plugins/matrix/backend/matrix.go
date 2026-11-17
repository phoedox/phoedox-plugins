// Package main is the decision matrix block (ADR-0087): options scored
// against weighted criteria ("criteria: Price*3, Noise*2", then
// "- Option: 4, 3"), drawn as SVG by block_render. The logic has no build tag
// so it is tested natively; it reads the source like the frontend does.
package main

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Criterion is one column with its weight (1 when none is given).
type Criterion struct {
	Name   string
	Weight float64
}

// Option is one row with a score per criterion.
type Option struct {
	Name   string
	Scores []float64
}

// Matrix is a parsed decision matrix; Max is the best possible score.
type Matrix struct {
	Title    string
	Criteria []Criterion
	Options  []Option
	Max      float64
}

// Problem is a finding in the source; Line counts from 1.
type Problem struct {
	Line    int
	Message string
}

var (
	keyRe    = regexp.MustCompile(`^([A-Za-z]+)\s*:\s*(.*)$`)
	optionRe = regexp.MustCompile(`^[-*+]\s+(.+?)\s*:\s*(.+)$`)
	weightRe = regexp.MustCompile(`^(.*?)\s*[*×x]\s*(\d+(?:[.,]\d+)?)$`)
	scaleRe  = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*$`)
	scoreSep = regexp.MustCompile(`[,;]`)
	keys     = map[string]bool{"title": true, "titel": true, "criteria": true, "kriterien": true, "scale": true, "skala": true}
)

// number reads "3", "2.5" or "2,5".
func number(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(s), ",", ".", 1), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// Parse reads a matrix; problems name the line they are on. With problems
// the matrix is nil.
func Parse(src string) (*Matrix, []Problem) {
	m := &Matrix{}
	var ps []Problem
	var lines []int // line of each option
	scaleMax := 0.0
	for i, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		n := i + 1
		l := strings.TrimSpace(raw)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if kv := keyRe.FindStringSubmatch(l); kv != nil && keys[strings.ToLower(kv[1])] {
			v := strings.TrimSpace(kv[2])
			switch strings.ToLower(kv[1]) {
			case "title", "titel":
				m.Title = v
			case "scale", "skala":
				scaleMax = 0
				if s := scaleRe.FindStringSubmatch(v); s != nil {
					scaleMax, _ = number(s[1])
				}
			default:
				m.Criteria = nil
				for _, c := range strings.Split(v, ",") {
					c = strings.TrimSpace(c)
					if c == "" {
						continue
					}
					cr := Criterion{Name: c, Weight: 1}
					if w := weightRe.FindStringSubmatch(c); w != nil {
						cr.Name = strings.TrimSpace(w[1])
						cr.Weight, _ = number(w[2])
					}
					m.Criteria = append(m.Criteria, cr)
				}
			}
			continue
		}
		o := optionRe.FindStringSubmatch(l)
		if o == nil {
			continue
		}
		opt := Option{Name: o[1]}
		bad := ""
		for _, part := range scoreSep.Split(o[2], -1) {
			part = strings.TrimSpace(part)
			v, ok := number(part)
			if !ok && bad == "" {
				if part == "" {
					bad = "an empty score"
				} else {
					bad = strconv.Quote(part)
				}
			}
			opt.Scores = append(opt.Scores, v)
		}
		if bad != "" {
			ps = append(ps, Problem{n, fmt.Sprintf("%q: scores must be numbers, %s is not one", opt.Name, bad)})
		}
		m.Options = append(m.Options, opt)
		lines = append(lines, n)
	}
	if len(m.Criteria) == 0 {
		ps = append(ps, Problem{1, `the criteria are missing (criteria: Price*3, Noise*2)`})
	}
	if len(m.Options) < 2 {
		ps = append(ps, Problem{1, `a matrix needs at least two options (- Name: 3, 4)`})
	}
	for i, o := range m.Options {
		if len(m.Criteria) > 0 && len(o.Scores) != len(m.Criteria) {
			ps = append(ps, Problem{lines[i], fmt.Sprintf("%q needs %d scores, one per criterion (it has %d)", o.Name, len(m.Criteria), len(o.Scores))})
		}
	}
	if len(ps) > 0 {
		return nil, ps
	}
	m.Max = scaleMax
	if m.Max == 0 {
		m.Max = 5
		for _, o := range m.Options {
			for _, s := range o.Scores {
				m.Max = math.Max(m.Max, s)
			}
		}
	}
	return m, nil
}

// Scores is each option's share of the best possible result (0–1) with the
// given weights (nil: the matrix's own).
func (m *Matrix) Scores(weights []float64) []float64 {
	if weights == nil {
		for _, c := range m.Criteria {
			weights = append(weights, c.Weight)
		}
	}
	wsum := 0.0
	for _, w := range weights {
		wsum += w
	}
	if wsum == 0 {
		wsum = 1
	}
	out := make([]float64, len(m.Options))
	for i, o := range m.Options {
		s := 0.0
		for j, x := range o.Scores {
			if j < len(weights) {
				s += x * weights[j]
			}
		}
		out[i] = s / (wsum * m.Max)
	}
	return out
}

// num writes a coordinate with at most one decimal.
func num(f float64) string { return strconv.FormatFloat(math.Round(f*10)/10, 'f', -1, 64) }

// SVG draws the totals as bars, the best option in green; width 0 means 720.
func SVG(m *Matrix, width int) string {
	if width <= 0 {
		width = 720
	}
	w := float64(max(width, 320))
	esc := html.EscapeString
	top := 6.0
	if m.Title != "" {
		top = 34
	}
	n := float64(len(m.Options))
	h := top + n*34 + 8
	row := math.Min(38, math.Max(24, (h-top-6)/n))
	longest := 0
	for _, o := range m.Options {
		longest = max(longest, utf8.RuneCountInString(o.Name))
	}
	label := math.Min(w*0.4, 12+float64(longest)*7)
	scores := m.Scores(nil)
	best := math.Inf(-1)
	for _, s := range scores {
		best = math.Max(best, s)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s" style="font-family:var(--font-body,system-ui,sans-serif)">`, num(w), num(h), num(w), num(h))
	if m.Title != "" {
		fmt.Fprintf(&sb, `<text x="6" y="22" style="font-size:16px;font-weight:600;fill:var(--fg)">%s</text>`, esc(m.Title))
	}
	for i, o := range m.Options {
		y := top + float64(i)*row
		bw := math.Max(2, (w-label-60)*scores[i])
		c, bold := "gray", ""
		if scores[i] == best {
			c, bold = "green", ";font-weight:600"
		}
		pct := int(math.Round(scores[i] * 100))
		fmt.Fprintf(&sb, `<g><title>%s: %d %%</title>`, esc(o.Name), pct)
		fmt.Fprintf(&sb, `<text x="6" y="%s" style="font-size:13px;fill:var(--fg)%s">%s</text>`, num(y+row/2+4), bold, esc(o.Name))
		fmt.Fprintf(&sb, `<rect x="%s" y="%s" width="%s" height="%s" rx="5" style="fill:var(--tc-%s)"/>`, num(label), num(y+4), num(bw), num(row-10), c)
		fmt.Fprintf(&sb, `<text x="%s" y="%s" style="font-size:12px;fill:var(--muted)">%d %%</text></g>`, num(label+bw+6), num(y+row/2+4), pct)
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

// sortedProblems puts problems in line order.
func sortedProblems(ps []Problem) []Problem {
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Line < ps[j].Line })
	return ps
}
