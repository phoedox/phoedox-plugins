// Package main is the poll block (ADR-0087): a question, answers as a list
// ("- Zoo"), one vote per reader (or several with "multiple: yes"). The
// votes are kept by the host per reader and only counted, never shown with
// names; block_render draws the question and the answers as SVG without
// votes (the live view shows them). The logic has no build tag so it is
// tested natively; it reads the source like the frontend does.
package main

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Poll is a parsed poll. Results is "always" (shown before voting) or
// "after" (shown once the reader voted, or when the poll is closed).
type Poll struct {
	Question string
	Options  []string
	Multiple bool
	Closed   bool
	Results  string
	ID       string
}

// Problem is a finding in the source; Line counts from 1.
type Problem struct {
	Line    int
	Message string
}

// MaxOptions is the most answers a poll may have.
const MaxOptions = 30

var (
	keyRe    = regexp.MustCompile(`^([A-Za-z]+)\s*:\s*(.*)$`)
	optionRe = regexp.MustCompile(`^[-*+]\s+(.+)$`)
	alwaysRe = regexp.MustCompile(`(?i)always|immer`)
	yesRe    = regexp.MustCompile(`(?i)^(true|yes|ja|1|on)$`)
	keys     = map[string]bool{"question": true, "frage": true, "multiple": true, "closed": true, "results": true, "id": true}
)

// yes reads "true", "yes", "ja", "1" or "on".
func yes(v string) bool { return yesRe.MatchString(strings.TrimSpace(v)) }

// Parse reads a poll; problems name the line they are on. With problems the
// poll is nil.
func Parse(src string) (*Poll, []Problem) {
	p := &Poll{Results: "after"}
	var lines []int // line of each option
	for i, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		n := i + 1
		l := strings.TrimSpace(raw)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if kv := keyRe.FindStringSubmatch(l); kv != nil && keys[strings.ToLower(kv[1])] {
			v := strings.TrimSpace(kv[2])
			switch strings.ToLower(kv[1]) {
			case "question", "frage":
				p.Question = v
			case "multiple":
				p.Multiple = yes(v)
			case "closed":
				p.Closed = yes(v)
			case "results":
				p.Results = "after"
				if alwaysRe.MatchString(v) {
					p.Results = "always"
				}
			case "id":
				p.ID = v
			}
			continue
		}
		// A flag on its own line: "multiple" or "closed".
		switch strings.ToLower(l) {
		case "multiple":
			p.Multiple = true
			continue
		case "closed":
			p.Closed = true
			continue
		}
		if o := optionRe.FindStringSubmatch(l); o != nil {
			p.Options = append(p.Options, strings.TrimSpace(o[1]))
			lines = append(lines, n)
		} else if p.Question == "" {
			p.Question = l
		}
	}
	var ps []Problem
	if p.Question == "" {
		ps = append(ps, Problem{1, "the question is missing (question: …)"})
	}
	if len(p.Options) < 2 {
		ps = append(ps, Problem{1, "a poll needs at least two answers (- …)"})
	}
	if len(p.Options) > MaxOptions {
		ps = append(ps, Problem{lines[MaxOptions], fmt.Sprintf("at most %d answers", MaxOptions)})
	}
	seen := map[string]bool{}
	for i, o := range p.Options {
		if seen[o] {
			ps = append(ps, Problem{lines[i], fmt.Sprintf("the answer %q is there twice", o)})
			break
		}
		seen[o] = true
	}
	if len(ps) > 0 {
		return nil, ps
	}
	return p, nil
}

// num writes a coordinate with at most one decimal.
func num(f float64) string { return strconv.FormatFloat(math.Round(f*10)/10, 'f', -1, 64) }

// SVG draws the question and the answers as empty bars (the votes need the
// live view); width 0 means 420, the poll's size on whiteboards.
func SVG(p *Poll, width int) string {
	if width <= 0 {
		width = 420
	}
	w := float64(max(width, 200))
	n := float64(len(p.Options))
	h := 40 + n*34 + 10
	row := math.Min(40, math.Max(26, (h-40)/math.Max(1, n)))
	esc := html.EscapeString
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s" style="font-family:var(--font-body,system-ui,sans-serif)">`, num(w), num(h), num(w), num(h))
	fmt.Fprintf(&sb, `<text x="6" y="22" style="font-size:16px;font-weight:600;fill:var(--fg)">%s</text>`, esc(p.Question))
	for i, o := range p.Options {
		y := 40 + float64(i)*row
		fmt.Fprintf(&sb, `<rect x="6" y="%s" width="%s" height="%s" rx="6" style="fill:var(--surface2)"/>`, num(y), num(w-12), num(row-6))
		fmt.Fprintf(&sb, `<text x="16" y="%s" style="font-size:13px;fill:var(--fg)">%s</text>`, num(y+(row-6)/2+4.5), esc(o))
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

// sortedProblems puts problems in line order.
func sortedProblems(ps []Problem) []Problem {
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Line < ps[j].Line })
	return ps
}
