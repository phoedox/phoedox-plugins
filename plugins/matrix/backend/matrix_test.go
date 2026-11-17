package main

import (
	"math"
	"strings"
	"testing"
)

const example = `title: Which NAS?
# a comment
criteria: Price*3, Noise × 2, Storage x1.5, Power
scale: 1-10
- Synology: 3, 4, 3, 4
- QNAP: 4; 3; 4; 3
* Self-built <b>: 10, 2, 5, 2`

func TestParse(t *testing.T) {
	m, ps := Parse(example)
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	want := []Criterion{{"Price", 3}, {"Noise", 2}, {"Storage", 1.5}, {"Power", 1}}
	if len(m.Criteria) != len(want) {
		t.Fatalf("criteria: %+v", m.Criteria)
	}
	for i, c := range want {
		if m.Criteria[i] != c {
			t.Errorf("criterion %d: %+v, want %+v", i, m.Criteria[i], c)
		}
	}
	if m.Title != "Which NAS?" || m.Max != 10 || len(m.Options) != 3 {
		t.Errorf("matrix: %+v", m)
	}
	if o := m.Options[1]; o.Name != "QNAP" || len(o.Scores) != 4 || o.Scores[0] != 4 {
		t.Errorf("semicolons: %+v", o)
	}
	// German keys, no scale: the highest score (at least 5) is the best possible one.
	m, ps = Parse("Titel: NAS\nKriterien: Preis*2, Lärm\n- A: 7, 1\n- B: 2, 3")
	if len(ps) > 0 || m.Title != "NAS" || m.Max != 7 || m.Criteria[0].Name != "Preis" {
		t.Errorf("german: %+v %v", m, ps)
	}
	m, _ = Parse("criteria: a, b\n- A: 1, 2\n- B: 2, 1")
	if m.Max != 5 {
		t.Errorf("max at least 5: %v", m.Max)
	}
	m, _ = Parse("criteria: a\nskala: 0 bis 2,5\n- A: 1\n- B: 2")
	if m.Max != 2.5 {
		t.Errorf("scale with comma: %v", m.Max)
	}
}

func TestProblems(t *testing.T) {
	for src, want := range map[string]Problem{
		"- A: 1\n- B: 2":                          {1, "criteria are missing"},
		"criteria: a, b\n- A: 1, 2":               {1, "at least two options"},
		"criteria: a, b\n- A: 1, x\n- B: 1, 2":    {2, `"A": scores must be numbers, "x" is not one`},
		"criteria: a, b\n- A: 1, \n- B: 1, 2":     {2, "an empty score"},
		"criteria: a, b\n- A: 1, 2\n\n- B: 1":     {4, `"B" needs 2 scores, one per criterion (it has 1)`},
		"- A: 1, 2, 3\n- B: 1, 2\ncriteria: a, b": {1, `"A" needs 2 scores`},
		"criteria: a\n- A: NaN\n- B: 1":           {2, "scores must be numbers"},
	} {
		m, ps := Parse(src)
		if m != nil || len(ps) == 0 {
			t.Errorf("%q: no problem", src)
			continue
		}
		ps = sortedProblems(ps)
		if ps[0].Line != want.Line || !strings.Contains(ps[0].Message, want.Message) {
			t.Errorf("%q: %+v, want %+v", src, ps, want)
		}
	}
}

func TestScores(t *testing.T) {
	m, _ := Parse("criteria: a*3, b*1\nscale: 1-5\n- A: 5, 1\n- B: 1, 5")
	s := m.Scores(nil)
	if math.Abs(s[0]-16.0/20) > 1e-9 || math.Abs(s[1]-8.0/20) > 1e-9 {
		t.Errorf("weighted: %v", s)
	}
	s = m.Scores([]float64{1, 3})
	if s[1] <= s[0] {
		t.Errorf("other weights: %v", s)
	}
	if s = m.Scores([]float64{0, 0}); s[0] != 0 || s[1] != 0 {
		t.Errorf("no weights: %v", s)
	}
}

func TestSVG(t *testing.T) {
	m, _ := Parse(example)
	svg := SVG(m, 600)
	for _, want := range []string{`<svg`, `viewBox="0 0 600 144"`, "Which NAS?", "var(--tc-green)", "var(--tc-gray)", "var(--fg)", "var(--muted)", "var(--font-body", "Self-built &lt;b&gt;", " %</text>"} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %q in %s", want, svg)
		}
	}
	if strings.Contains(svg, "<b>") || strings.Contains(svg, "<script") {
		t.Error("text must be escaped")
	}
	if strings.Count(svg, "var(--tc-green)") != 1 {
		t.Error("one best option")
	}
	if !strings.Contains(SVG(m, 0), `width="720"`) {
		t.Error("default width")
	}
	m.Title = ""
	if !strings.Contains(SVG(m, 500), `height="116"`) {
		t.Error("height without title")
	}
}
