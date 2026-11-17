package main

import (
	"strings"
	"testing"
	"time"
)

const example = `title: Bathroom
today
## Planning
Quotes [q]: 2026-01-05, 2w, done
Choose [c]: after q, 3d, 50%
## Work
Demolition: 2026-02-02 – 2026-02-06, red
Done: 2026-03-20`

func TestParse(t *testing.T) {
	g, ps := Parse(example)
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	q, c, d, m := g.Items[0], g.Items[1], g.Items[2], g.Items[3]
	if !q.Done || q.End.Sub(q.Start) != 14*day {
		t.Errorf("quotes: %+v", q)
	}
	if !c.Start.Equal(q.End) || c.Progress != 50 {
		t.Errorf("after: %+v", c)
	}
	if d.End.Sub(d.Start) != 5*day || d.Color != "red" {
		t.Errorf("range: %+v", d)
	}
	if !m.Milestone || len(g.Sections) != 2 || !g.Today || g.Title != "Bathroom" {
		t.Errorf("milestone/sections: %+v %+v", m, g)
	}
	for src, want := range map[string]string{
		"X: after y, 2d":                     "no such id",
		"A [a]: after b, 1d\nB [b]: after a, 1d": "circle",
		"A: tomorrow":                         "neither a date",
		"A: 2026-01-01, soon":                 "not a date, a duration",
		"A: 2026-01-10, 2026-01-05":           "ends before",
		"just text":                           "unknown line",
	} {
		_, ps := Parse(src)
		if len(ps) == 0 || !strings.Contains(ps[0].Message, want) {
			t.Errorf("%q: %v", src, ps)
		}
	}
	if _, ps := Parse("board: roadmap"); len(ps) > 0 {
		t.Errorf("a board alone is fine: %v", ps)
	}
}

func TestSVG(t *testing.T) {
	g, _ := Parse(example)
	svg := SVG(g, 700, "de", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	for _, want := range []string{"<svg", "marker-end", "heute", "PLANNING", "Feb.", "var(--tc-red)", "Quotes: 5. Jan."} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(svg, "<script") {
		t.Error("no scripts")
	}
	if !strings.Contains(SVG(&Gantt{}, 400, "en", time.Now()), "No items") {
		t.Error("empty schedule")
	}
}
