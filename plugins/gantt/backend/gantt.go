// Package main is the schedule (Gantt) block (ADR-0086): one line per item,
// "Name [id]: start, end or duration, …", drawn as SVG by block_render. The
// logic has no build tag so it is tested natively.
package main

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const day = 24 * time.Hour

// Item is one bar or milestone.
type Item struct {
	Name, ID  string
	Section   int
	Start     time.Time
	End       time.Time // exclusive
	Milestone bool
	Done      bool
	Progress  int // -1 = none
	Color     string
	After     []string
	resolved  bool
}

// Gantt is a parsed schedule.
type Gantt struct {
	Title    string
	Sections []string
	Items    []*Item
	Today    bool
	Board    string
}

// Problem is a finding in the source; Line counts from 1.
type Problem struct {
	Line    int
	Message string
}

var (
	colors   = []string{"blue", "green", "orange", "purple", "pink", "red", "yellow", "gray"}
	lineRe   = regexp.MustCompile(`^(.+?)(?:\s*\[([A-Za-z0-9-]+)\])?\s*:\s*(.+)$`)
	keyRe    = regexp.MustCompile(`(?i)^(title|titel|board)\s*:\s*(.*)$`)
	secRe    = regexp.MustCompile(`(?i)^(?:##\s+|section\s+|abschnitt\s+)(.+)$`)
	rangeRe  = regexp.MustCompile(`\s+(?:–|—|-|\.\.|bis|to)\s+(\d{4}-)`)
	durRe    = regexp.MustCompile(`(?i)^(\d+(?:[.,]\d+)?)\s*([dtwm])$`)
	pctRe    = regexp.MustCompile(`^(\d{1,3})\s*%$`)
	doneRe   = regexp.MustCompile(`(?i)^(done|fertig|erledigt)$`)
	afterRe  = regexp.MustCompile(`(?i)^(?:after|nach)\s+(.+)$`)
	todayRe  = regexp.MustCompile(`(?i)^(today|heute)$`)
	isColour = map[string]bool{}
)

func init() {
	for _, c := range colors {
		isColour[c] = true
	}
}

func date(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	return t, err == nil
}

func duration(s string) (time.Duration, bool) {
	m := durRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	n, _ := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
	mult := map[string]float64{"d": 1, "t": 1, "w": 7, "m": 30}[strings.ToLower(m[2])]
	return time.Duration(n * mult * float64(day)), true
}

// Parse reads a schedule; problems name the line they are on.
func Parse(src string) (*Gantt, []Problem) {
	g := &Gantt{}
	var ps []Problem
	section := -1
	type pend struct {
		it  *Item
		end *time.Time
		dur *time.Duration
	}
	var pending []pend
	for i, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		n := i + 1
		l := strings.TrimSpace(raw)
		if l == "" || (strings.HasPrefix(l, "#") && !strings.HasPrefix(l, "## ")) {
			continue
		}
		if m := keyRe.FindStringSubmatch(l); m != nil {
			if strings.EqualFold(m[1], "board") {
				g.Board = strings.TrimSpace(m[2])
			} else {
				g.Title = strings.TrimSpace(m[2])
			}
			continue
		}
		if todayRe.MatchString(l) {
			g.Today = true
			continue
		}
		if m := secRe.FindStringSubmatch(l); m != nil {
			g.Sections = append(g.Sections, strings.TrimSpace(m[1]))
			section = len(g.Sections) - 1
			continue
		}
		m := lineRe.FindStringSubmatch(l)
		if m == nil {
			ps = append(ps, Problem{n, fmt.Sprintf("unknown line %q (expected \"Name: start, duration\")", l)})
			continue
		}
		if section < 0 {
			g.Sections = append(g.Sections, "")
			section = 0
		}
		it := &Item{Name: strings.TrimSpace(m[1]), ID: m[2], Section: section, Progress: -1}
		parts := strings.Split(rangeRe.ReplaceAllString(m[3], ", $1"), ",")
		first := strings.TrimSpace(parts[0])
		if a := afterRe.FindStringSubmatch(first); a != nil {
			it.After = strings.Fields(a[1])
		} else if t, ok := date(first); ok {
			it.Start = t
		} else {
			ps = append(ps, Problem{n, fmt.Sprintf("%q: %q is neither a date (YYYY-MM-DD) nor \"after id\"", it.Name, first)})
			continue
		}
		p := pend{it: it}
		bad := false
		for _, part := range parts[1:] {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if t, ok := date(part); ok && p.end == nil && p.dur == nil {
				p.end = &t
			} else if d, ok := duration(part); ok && p.end == nil && p.dur == nil {
				p.dur = &d
			} else if doneRe.MatchString(part) {
				it.Done = true
			} else if pm := pctRe.FindStringSubmatch(part); pm != nil {
				v, _ := strconv.Atoi(pm[1])
				it.Progress = min(100, v)
			} else if isColour[strings.ToLower(part)] {
				it.Color = strings.ToLower(part)
			} else {
				ps = append(ps, Problem{n, fmt.Sprintf("%q: %q is not a date, a duration (3d, 2w), done, a percentage or a colour", it.Name, part)})
				bad = true
			}
		}
		if bad {
			continue
		}
		g.Items = append(g.Items, it)
		pending = append(pending, p)
		if p.end != nil && len(it.After) == 0 && p.end.Before(it.Start) {
			ps = append(ps, Problem{n, fmt.Sprintf("%q ends before it starts", it.Name)})
		}
	}
	byID := map[string]*Item{}
	for _, it := range g.Items {
		if it.ID != "" {
			byID[it.ID] = it
		}
	}
	finish := func(p pend) {
		it := p.it
		switch {
		case p.end != nil:
			it.End = p.end.Add(day) // an end date includes that day
		case p.dur != nil:
			it.End = it.Start.Add(*p.dur)
		default:
			it.End, it.Milestone = it.Start, true
		}
		it.resolved = true
	}
	for round := 0; round <= len(pending); round++ {
		for _, p := range pending {
			it := p.it
			if it.resolved {
				continue
			}
			if len(it.After) == 0 {
				finish(p)
				continue
			}
			ready := true
			var start time.Time
			for _, a := range it.After {
				d := byID[a]
				if d == nil || !d.resolved {
					ready = false
					break
				}
				if d.End.After(start) {
					start = d.End
				}
			}
			if ready {
				it.Start = start
				finish(p)
			}
		}
	}
	for _, p := range pending {
		for _, a := range p.it.After {
			if byID[a] == nil {
				ps = append(ps, Problem{0, fmt.Sprintf("%q follows %q, but there is no such id", p.it.Name, a)})
			}
		}
		if !p.it.resolved && len(ps) == 0 {
			ps = append(ps, Problem{0, fmt.Sprintf("%q depends on itself in a circle", p.it.Name)})
		}
	}
	if len(g.Items) == 0 && g.Board == "" && len(ps) == 0 {
		ps = append(ps, Problem{1, "the schedule has no items"})
	}
	return g, ps
}

var months = map[string][]string{
	"de": {"Jan.", "Feb.", "März", "Apr.", "Mai", "Juni", "Juli", "Aug.", "Sept.", "Okt.", "Nov.", "Dez."},
	"en": {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
}

func monthName(m time.Month, locale string) string {
	if n, ok := months[locale]; ok {
		return n[m-1]
	}
	return months["en"][m-1]
}

func dayLabel(t time.Time, locale string) string {
	if locale == "de" {
		return fmt.Sprintf("%d. %s", t.Day(), monthName(t.Month(), locale))
	}
	return fmt.Sprintf("%s %d", monthName(t.Month(), locale), t.Day())
}

type tick struct {
	t     time.Time
	label string
}

// ticks chooses days, weeks or months so labels keep about gap pixels apart.
func ticks(a, b time.Time, px float64, locale string) []tick {
	fits := max(1, int(px/70))
	days := b.Sub(a).Hours() / 24
	var out []tick
	if days <= 90 {
		step := 30
		for _, s := range []int{1, 2, 7, 14, 30} {
			if days/float64(s) <= float64(fits) {
				step = s
				break
			}
		}
		for t := a.Truncate(day); !t.After(b); t = t.AddDate(0, 0, step) {
			out = append(out, tick{t, dayLabel(t, locale)})
		}
		return out
	}
	months := days / 30.4
	step := 120
	for _, s := range []int{1, 2, 3, 6, 12, 24, 60} {
		if months/float64(s) <= float64(fits) {
			step = s
			break
		}
	}
	t := time.Date(a.Year(), a.Month(), 1, 0, 0, 0, 0, time.UTC)
	if t.Before(a) {
		t = t.AddDate(0, 1, 0)
	}
	if step >= 12 {
		t = time.Date(a.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	first := true
	for ; !t.After(b); t = t.AddDate(0, step, 0) {
		label := monthName(t.Month(), locale)
		if step >= 12 {
			label = strconv.Itoa(t.Year())
		} else if t.Month() == 1 || first {
			label += fmt.Sprintf(" %02d", t.Year()%100)
		}
		out = append(out, tick{t, label})
		first = false
	}
	return out
}

const rowH, headH = 26.0, 30.0

// SVG draws the schedule; now is "today".
func SVG(g *Gantt, width int, locale string, now time.Time) string {
	w := float64(max(width, 360))
	esc := html.EscapeString
	if len(g.Items) == 0 {
		return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %g 40" width="%g" height="40"><text x="6" y="24" style="font-size:13px;fill:var(--muted)">%s</text></svg>`, w, w, esc(text(locale, "empty")))
	}
	labelW := 120.0
	for _, it := range g.Items {
		labelW = max(labelW, float64(len([]rune(it.Name)))*7+24)
	}
	labelW = min(labelW, math.Max(120, w*0.3))
	left, right := labelW+8, w-14
	a, b := g.Items[0].Start, g.Items[0].End
	for _, it := range g.Items {
		if it.Start.Before(a) {
			a = it.Start
		}
		if it.End.After(b) {
			b = it.End
		}
	}
	if g.Today {
		if now.Before(a) {
			a = now
		}
		if now.After(b) {
			b = now
		}
	}
	pad := time.Duration(math.Max(float64(day), float64(b.Sub(a))*0.02))
	a, b = a.Add(-pad), b.Add(pad)
	x := func(t time.Time) float64 { return left + (right-left)*float64(t.Sub(a))/float64(b.Sub(a)) }
	top := headH
	if g.Title != "" {
		top += 30
	}
	type row struct {
		section string
		item    *Item
		color   string
	}
	var rows []row
	for si, s := range g.Sections {
		var its []*Item
		for _, it := range g.Items {
			if it.Section == si {
				its = append(its, it)
			}
		}
		if len(its) == 0 {
			continue
		}
		c := colors[si%len(colors)]
		if s != "" {
			rows = append(rows, row{section: s, color: c})
		}
		for _, it := range its {
			ic := c
			if it.Color != "" {
				ic = it.Color
			}
			rows = append(rows, row{item: it, color: ic})
		}
	}
	h := top + float64(len(rows))*rowH + 12
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %g %g" width="%g" height="%g" style="font-family:var(--font-body,system-ui,sans-serif)">`, w, h, w, h)
	sb.WriteString(`<defs><marker id="ga" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0 L8 4 L0 8 z" style="fill:var(--muted)"/></marker></defs>`)
	if g.Title != "" {
		fmt.Fprintf(&sb, `<text x="6" y="20" style="font-size:16px;font-weight:600;fill:var(--fg)">%s</text>`, esc(g.Title))
	}
	for _, tk := range ticks(a, b, right-left, locale) {
		tx := x(tk.t)
		fmt.Fprintf(&sb, `<line x1="%.1f" x2="%.1f" y1="%g" y2="%g" style="stroke:var(--line)"/><text x="%.1f" y="%g" text-anchor="middle" style="font-size:11px;fill:var(--muted)">%s</text>`, tx, tx, top-4, h-12, tx, top-10, esc(tk.label))
	}
	type pos struct{ x0, x1, y float64 }
	at := map[*Item]pos{}
	maxChars := int(labelW / 7)
	for i, r := range rows {
		ry := top + float64(i)*rowH
		cy := ry + rowH/2
		if r.item == nil {
			fmt.Fprintf(&sb, `<text x="6" y="%.1f" style="font-size:12px;font-weight:700;fill:var(--muted);letter-spacing:.02em">%s</text>`, cy+4.5, esc(strings.ToUpper(r.section)))
			continue
		}
		it := r.item
		x0 := x(it.Start)
		x1 := math.Max(x(it.End), x0+3)
		at[it] = pos{x0, x1, cy}
		name := []rune(it.Name)
		if len(name) > maxChars {
			name = append(name[:maxChars-1], '…')
		}
		op := ""
		if it.Done {
			op = ";opacity:.6"
		}
		fmt.Fprintf(&sb, `<text x="14" y="%.1f" style="font-size:13px;fill:var(--fg)%s">%s</text>`, cy+4.5, op, esc(string(name)))
		tip := it.Name + ": " + dayLabel(it.Start, locale)
		if !it.Milestone {
			tip += " – " + dayLabel(it.End.Add(-day), locale)
		}
		if it.Progress >= 0 {
			tip += fmt.Sprintf(" (%d %%)", it.Progress)
		}
		if it.Done {
			tip += " – " + text(locale, "done")
		}
		if it.Milestone {
			fmt.Fprintf(&sb, `<path d="M%.1f %.1f L%.1f %.1f L%.1f %.1f L%.1f %.1f Z" style="fill:var(--tc-%s)"><title>%s</title></path>`, x0, cy-8, x0+8, cy, x0, cy+8, x0-8, cy, r.color, esc(tip))
			continue
		}
		bop := ""
		if it.Done {
			bop = ";opacity:.45"
		}
		fmt.Fprintf(&sb, `<g><title>%s</title><rect x="%.1f" y="%.1f" width="%.1f" height="%g" rx="4" style="fill:var(--tb-%s);stroke:var(--tc-%s)%s"/>`, esc(tip), x0, ry+6, x1-x0, rowH-12, r.color, r.color, bop)
		if it.Progress > 0 {
			fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%g" rx="4" style="fill:var(--tc-%s)%s"/>`, x0, ry+6, (x1-x0)*float64(it.Progress)/100, rowH-12, r.color, bop)
		}
		if it.Done {
			fmt.Fprintf(&sb, `<path d="M%.1f %.1f l3 3 l6 -7" fill="none" style="stroke:var(--good)" stroke-width="2"/>`, x1+5, ry+13)
		}
		sb.WriteString(`</g>`)
	}
	byID := map[string]*Item{}
	for _, it := range g.Items {
		if it.ID != "" {
			byID[it.ID] = it
		}
	}
	for _, it := range g.Items {
		to, ok := at[it]
		if !ok {
			continue
		}
		for _, id := range it.After {
			from, ok := at[byID[id]]
			if !ok {
				continue
			}
			mx := math.Max(from.x1+6, to.x0-8)
			fmt.Fprintf(&sb, `<path d="M%.1f %.1f H%.1f V%.1f H%.1f" fill="none" style="stroke:var(--muted)" stroke-width="1.2" marker-end="url(#ga)"/>`, from.x1, from.y, mx, to.y, to.x0-2)
		}
	}
	if g.Today && !now.Before(a) && !now.After(b) {
		tx := x(now)
		fmt.Fprintf(&sb, `<line x1="%.1f" x2="%.1f" y1="%g" y2="%g" style="stroke:var(--bad)" stroke-width="1.5" stroke-dasharray="4 3"/><text x="%.1f" y="%g" style="font-size:11px;fill:var(--bad)">%s</text>`, tx, tx, top-4, h-12, tx+4, h-14, esc(text(locale, "today")))
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

func text(locale, key string) string {
	de := map[string]string{"today": "heute", "done": "erledigt", "empty": "Keine Einträge mit Datum."}
	en := map[string]string{"today": "today", "done": "done", "empty": "No items with dates."}
	if locale == "de" {
		return de[key]
	}
	return en[key]
}

// sortedProblems puts problems in line order.
func sortedProblems(ps []Problem) []Problem {
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Line < ps[j].Line })
	return ps
}
